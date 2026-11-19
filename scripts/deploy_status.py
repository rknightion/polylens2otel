"""Read exact publication and runtime evidence for a protected PolyLens revision."""

import base64
from datetime import datetime, timezone
import hashlib
import json
import math
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request


REPO = "rknightion/polylens2otel"
IMAGE = f"ghcr.io/{REPO}"
PUBLISHER = "rknightion/.github/.github/workflows/container-publish.yml"
EDGE_PUBLISHER_PIN = "25774fddcf21225cd249db33e61f1c3fa092da83"
ISSUER = "https://token.actions.githubusercontent.com"
SIGNATURE_TYPE = "https://sigstore.dev/cosign/sign/v1"
REGISTRY = f"https://ghcr.io/v2/{REPO}"
SHA = re.compile(r"[0-9a-f]{40}")
PRERELEASE_TAG = re.compile(r"v[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}")
MEDIA = ", ".join(
    f"application/vnd.{name}"
    for name in ("oci.image.index.v1+json", "oci.image.manifest.v1+json",
                 "docker.distribution.manifest.list.v2+json", "docker.distribution.manifest.v2+json")
)
MAX_BODY = 2 * 1024 * 1024
MAX_AGE_SECONDS = 90

# The one bootstrap receipt predates the verifier. Keep it on its own immutable
# artifacts and actual observation, even after a later revision deploys.
BOOTSTRAP_SHA = "7997ff791b0bb6e128bf170bd339cbc38767a7ae"
BOOTSTRAP_DIGEST = "sha256:810e8a5542015a326690849ee46ad95f6cb1127d741cc205ebfdebff01c98de6"
BOOTSTRAP_TAG = "v0.3.1-rc.149"
BOOTSTRAP_AT = "2026-11-18T16:01:33Z"
BOOTSTRAP_RUNS = {"ci.yml": 44333075330, "release-please.yml": 44333075428, "auto-rc.yml": 44333625309}


class Unread(Exception):
    """There is insufficient exact deployment evidence."""


class Unhealthy(Exception):
    """The exact publication or deployed revision explicitly failed."""


def require(condition, message):
    if not condition:
        raise Unread(message)


def utc(value):
    if not isinstance(value, str) or not value.endswith("Z"):
        raise Unread("provider did not return a UTC timestamp")
    return datetime.fromisoformat(value)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def fetch(url, headers=None, registry_blob=False):
    opener = urllib.request.build_opener(NoRedirect())
    request = urllib.request.Request(url, headers=headers or {})
    try:
        response = opener.open(request, timeout=5)
    except urllib.error.HTTPError as error:
        location = error.headers.get("Location", "")
        target = urllib.parse.urlsplit(location)
        if (not registry_blob or error.code != 307 or target.scheme != "https"
                or target.netloc != "pkg-containers.githubusercontent.com"):
            raise
        # GHCR may hand a config blob to its CDN. Its signed URL already grants
        # access, so the registry token is never forwarded.
        response = opener.open(urllib.request.Request(location), timeout=5)
    with response:
        require('rel="next"' not in response.headers.get("Link", ""), "provider returned a partial page")
        body = response.read(MAX_BODY + 1)
        require(len(body) <= MAX_BODY, "provider response exceeded 2 MiB")
        return body, response.headers.get("Docker-Content-Digest")


def github(path):
    result = subprocess.run([
        "gh", "api", "--hostname", "github.com", "-H", "Accept: application/vnd.github+json",
        "-H", "X-GitHub-Api-Version: 2022-11-28", f"repos/{REPO}{path}",
    ], capture_output=True, timeout=8, check=False,
        env={**os.environ, "GH_PROMPT_DISABLED": "1", "GH_DEBUG": ""})
    require(result.returncode == 0 and len(result.stdout) <= MAX_BODY, "GitHub evidence is unavailable")
    return json.loads(result.stdout)


def complete_page(page, key):
    rows = page[key]
    require(isinstance(rows, list) and page["total_count"] == len(rows) < 100,
            "provider returned an incomplete result list")
    return rows


def succeeded(item, reason, cutoff):
    require(item["status"] == "completed", reason)
    if cutoff:
        require(utc(item.get("completed_at", item.get("updated_at"))) <= cutoff,
                "record completed after the historical observation")
    if item["conclusion"] in {"failure", "timed_out", "startup_failure"}:
        raise Unhealthy(reason)
    require(item["conclusion"] == "success", reason)


def workflow(file, event, sha, required_jobs, cutoff):
    if cutoff:
        run = github(f"/actions/runs/{BOOTSTRAP_RUNS[file]}/attempts/1")
    else:
        rows = complete_page(github(
            f"/actions/workflows/{file}/runs?event={event}&head_sha={sha}&per_page=100"
        ), "workflow_runs")
        numbers = {row["run_number"] for row in rows}
        require(rows and len(numbers) == len(rows), "matching workflow run is missing or ambiguous")
        run = max(rows, key=lambda row: row["run_number"])
    require(run["path"] == f".github/workflows/{file}" and run["event"] == event
            and run["head_branch"] == "main" and run["head_sha"] == sha
            and run["repository"]["full_name"] == run["head_repository"]["full_name"] == REPO,
            "workflow run does not belong to the exact protected revision")
    succeeded(run, "matching workflow did not complete successfully", cutoff)
    run_id, attempt = run["id"], run["run_attempt"]
    require(type(run_id) is int and run_id > 0 and type(attempt) is int and attempt > 0
            and (not cutoff or (run_id == BOOTSTRAP_RUNS[file] and attempt == 1)),
            "matching workflow attempt is invalid")
    rows = complete_page(github(f"/actions/runs/{run_id}/attempts/{attempt}/jobs?per_page=100"), "jobs")
    jobs = {job["name"]: job for job in rows}
    require(len(jobs) == len(rows) and all(job["run_id"] == run_id and job["run_attempt"] == attempt
                                          and job["head_sha"] == sha for job in rows),
            "workflow jobs do not belong to one exact attempt")
    for name in required_jobs:
        require(name in jobs, "required workflow job is missing")
        succeeded(jobs[name], "required workflow job did not complete successfully", cutoff)
    return run, jobs


def verify_ci(sha, cutoff):
    run, jobs = workflow("ci.yml", "push", sha, ("ci-success",), cutoff)
    job_id = jobs["ci-success"]["id"]
    rows = complete_page(github(f"/commits/{sha}/check-runs?filter=all&per_page=100"), "check_runs")
    matches = [row for row in rows if row["name"] == "ci-success" and row["app"]["id"] == 15368]
    if cutoff:
        matches = [row for row in matches if row["id"] == job_id]
    require(matches, "Actions-owned ci-success is missing")
    check = max(matches, key=lambda row: row["id"])
    require(check["id"] == job_id and check["check_suite"]["id"] == run["check_suite_id"]
            and check["head_sha"] == sha, "ci-success does not belong to the exact CI attempt")
    succeeded(check, "Actions-owned ci-success did not pass", cutoff)


def publisher(sha, cutoff, rc=False):
    prefix = "publish / image" if rc else "edge (:main image) / image"
    jobs = tuple(f"{prefix} / {name}" for name in (
        "build (linux/amd64)", "build (linux/arm64)", "merge + sign + sbom",
    ))
    if rc:
        workflow("auto-rc.yml", "workflow_run", sha, ("rc / guard", "rc / tag + prerelease", *jobs), cutoff)
    else:
        workflow("release-please.yml", "push", sha, jobs, cutoff)


def signer_identity(sha):
    receipt = github(f"/contents/.github/workflows/publish.yml?ref={sha}")
    body = base64.b64decode("".join(receipt["content"].split()), validate=True).decode()
    refs = re.findall(rf"^\s+uses: {re.escape(PUBLISHER)}@([0-9a-f]{{40}})(?: +#.*)?$", body, re.MULTILINE)
    require(len(refs) == 1, "protected publisher pin is missing or ambiguous")
    return f"https://github.com/{PUBLISHER}@{refs[0]}"


def edge_version(sha, identity):
    require(identity == f"https://github.com/{PUBLISHER}@{EDGE_PUBLISHER_PIN}",
            "the signed Edge publisher's version rules are unverified")
    commit = github(f"/git/commits/{sha}")
    tree_sha = commit["tree"]["sha"]
    require(commit["sha"] == sha and SHA.fullmatch(tree_sha), "the Edge checkout commit is invalid")
    tree = github(f"/git/trees/{tree_sha}?recursive=1")
    entries = tree["tree"]
    require(tree["sha"] == tree_sha and tree["truncated"] is False and isinstance(entries, list) and entries,
            "the complete Edge checkout tree is unavailable")
    # The pinned publisher starts at depth one with no tags or submodules.
    # Below 2**14 objects, Git's automatic abbreviation begins at seven.
    require(all(entry["type"] in {"blob", "tree"} and SHA.fullmatch(entry["sha"]) for entry in entries),
            "the Edge checkout contains an unsupported object")
    objects = {sha, tree_sha, *(entry["sha"] for entry in entries)}
    version = sha[:7]
    require(len(objects) < 2**14 and not any(oid != sha and oid.startswith(version) for oid in objects),
            "the checkout does not establish a unique seven-character Edge version")
    return version


def tag_points_to(tag, sha):
    ref = github(f"/git/ref/tags/{tag}")["object"]
    if ref["type"] == "tag":
        require(SHA.fullmatch(ref["sha"]), "release tag object is invalid")
        ref = github(f"/git/tags/{ref['sha']}")["object"]
    return ref["type"] == "commit" and ref["sha"] == sha


def verify_bootstrap(sha, cutoff):
    pull = github("/pulls/174")
    require(pull["merged"] is True and pull["merge_commit_sha"] == sha
            and pull["base"]["ref"] == "main" and pull["base"]["repo"]["full_name"] == REPO
            and utc(pull["merged_at"]) < cutoff, "historical merge record is missing or inconsistent")
    release = github(f"/releases/tags/{BOOTSTRAP_TAG}")
    require(release["prerelease"] is True and release["draft"] is False
            and release["tag_name"] == BOOTSTRAP_TAG and utc(release["published_at"]) <= cutoff,
            "historical prerelease is missing or was published later")
    require(tag_points_to(BOOTSTRAP_TAG, sha), "historical tag no longer binds the expected SHA")


def prerelease_version(registry, sha, digest):
    # The publisher cuts at most one RC for a SHA. The current release is near
    # the top of this bounded read; absent evidence leaves the result unread.
    releases = github("/releases?per_page=30")
    matches = [release for release in releases
               if release["prerelease"] is True and release["draft"] is False
               and release["target_commitish"] == sha and PRERELEASE_TAG.fullmatch(release["tag_name"])]
    require(len(matches) == 1 and utc(matches[0]["published_at"]) <= datetime.now(timezone.utc),
            "the signed rolling image lacks one exact published prerelease")
    tag = matches[0]["tag_name"]
    version = tag.removeprefix("v")
    require(tag_points_to(tag, sha) and registry.object("manifests", version)[1] == digest,
            "the immutable prerelease tag does not bind the signed rolling image")
    return version


class Registry:
    def __init__(self):
        raw, _ = fetch(f"https://ghcr.io/token?service=ghcr.io&scope=repository:{REPO}:pull")
        token = json.loads(raw)["token"]
        require(isinstance(token, str) and token and not any(c.isspace() for c in token),
                "public registry token is malformed")
        self.headers = {"Authorization": f"Bearer {token}", "Accept": MEDIA}

    def object(self, kind, ref):
        raw, reported = fetch(f"{REGISTRY}/{kind}/{ref}", self.headers, registry_blob=kind == "blobs")
        digest = f"sha256:{hashlib.sha256(raw).hexdigest()}"
        if DIGEST.fullmatch(ref):
            require(ref == digest, "registry object did not match its requested content digest")
        require(not reported or reported == digest, "registry content digest does not match the response")
        return json.loads(raw), digest

    def image(self, ref, sha):
        manifest, digest = self.object("manifests", ref)
        platforms = {}
        for descriptor in manifest["manifests"]:
            platform = descriptor["platform"]
            arch = platform["architecture"]
            if platform["os"] != "linux" or arch not in {"amd64", "arm64"}:
                continue
            require(arch not in platforms and DIGEST.fullmatch(descriptor["digest"]),
                    "image index has an invalid or ambiguous platform descriptor")
            child, _ = self.object("manifests", descriptor["digest"])
            config_digest = child["config"]["digest"]
            require(DIGEST.fullmatch(config_digest), "platform config digest is invalid")
            config, _ = self.object("blobs", config_digest)
            require(config["architecture"] == arch and config["os"] == "linux"
                    and config["config"]["Labels"]["org.opencontainers.image.revision"] == sha,
                    "platform image does not describe the expected source revision")
            platforms[arch] = config_digest
        require(set(platforms) == {"amd64", "arm64"}, "image index is missing a required platform")
        return digest


def signature_matches(digest, sha, identity, event, cutoff=None):
    image = f"{IMAGE}@{digest}"
    claims = [
        "--certificate-identity", identity, "--certificate-oidc-issuer", ISSUER,
        "--certificate-github-workflow-repository", REPO,
        "--certificate-github-workflow-ref", "refs/heads/main",
        "--certificate-github-workflow-sha", sha,
        "--certificate-github-workflow-trigger", event,
    ]
    with tempfile.TemporaryDirectory(prefix="polylens-signature-") as scratch:
        names = ("PATH", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR")
        env = {key: os.environ[key] for key in names if key in os.environ}
        env.update(DOCKER_CONFIG=scratch, TUF_ROOT=str(Path(scratch, "tuf")))
        return verify_signature_bundles(image, digest.removeprefix("sha256:"), claims, cutoff, Path(scratch), env)


def certificate_expires_by(bundle, cutoff, env):
    material = bundle["verificationMaterial"]
    if not material.get("timestampVerificationData", {}).get("rfc3161Timestamps"):
        return False
    certificate = base64.b64decode(material["certificate"]["rawBytes"], validate=True)
    validity = subprocess.run(["openssl", "x509", "-inform", "DER", "-dateopt", "iso_8601",
                               "-enddate", "-noout"], input=certificate, capture_output=True,
                              timeout=3, env=env, check=False)
    if validity.returncode or not validity.stdout.startswith(b"notAfter="):
        return False
    return utc(validity.stdout.decode().strip().removeprefix("notAfter=")) <= cutoff


def verify_signature_bundles(image, digest, claims, cutoff, scratch, env):
    response = subprocess.run(["cosign", "download", "signature", "--timeout", "15s", image],
                              capture_output=True, timeout=18, env=env, check=False)
    if response.returncode or len(response.stdout) > MAX_BODY:
        return False
    timestamp_flags = ["--use-signed-timestamps"] if cutoff else []
    for line in response.stdout.splitlines():
        bundle = json.loads(line)
        envelope = bundle.get("dsseEnvelope", {})
        if not envelope:
            continue
        payload = json.loads(base64.b64decode(envelope["payload"], validate=True))
        if payload["predicateType"] != SIGNATURE_TYPE:
            continue
        subjects = payload["subject"]
        if (len(subjects) != 1 or subjects[0].get("name") not in (None, "", IMAGE)
                or subjects[0]["digest"].get("sha256") != digest):
            continue
        if cutoff and not certificate_expires_by(bundle, cutoff, env):
            continue
        path = scratch / "signature.json"
        path.write_bytes(line)
        # Verify this exact image-signature statement, not other attestations
        # for the same digest. For historical evidence, authenticate the
        # timestamp at which its cutoff-bounded certificate was valid.
        result = subprocess.run([
            "cosign", "verify-blob-attestation", "--timeout", "20s", "--bundle", str(path),
            "--digestAlg", "sha256", "--digest", digest,
            "--type", SIGNATURE_TYPE, "--check-claims=true", *timestamp_flags, *claims,
        ], capture_output=True, timeout=23, env=env, check=False)
        if result.returncode == 0:
            return True
    return False


def prometheus(config, metric, at, version=None):
    origin = urllib.parse.urlsplit(config["origin"])
    require(origin.scheme == "https" and origin.hostname and not origin.username and not origin.password
            and origin.path in ("", "/") and not origin.query and not origin.fragment,
            "Grafana configuration must name a plain HTTPS origin")
    selector = {"service_name": "polylens2otel", "job": config["job"], "tenant_id": config["tenant_id"]}
    if version is not None:
        selector["service_version"] = version
    require(all(isinstance(value, str) and value and len(value) <= 256 for value in selector.values()),
            "Grafana deployment selector is incomplete")
    labels = ",".join(f"{key}={json.dumps(value, ensure_ascii=False)}" for key, value in selector.items())
    request = metric.format(labels="{" + labels + "}")
    token_file = Path(config["token_file"])
    require(token_file.is_file() and token_file.stat().st_size <= 4096, "Grafana read-token file is unavailable")
    token = token_file.read_text().strip()
    require(token and not any(char.isspace() for char in token), "Grafana read token is malformed")
    uid = urllib.parse.quote(config["datasource_uid"], safe="")
    require(uid and len(uid) <= 256, "Grafana datasource is missing")
    query = urllib.parse.urlencode({"query": request, "time": at.isoformat()})
    raw, _ = fetch(f"https://{origin.netloc}/api/datasources/proxy/uid/{uid}/api/v1/query?{query}",
                   {"Authorization": f"Bearer {token}", "Accept": "application/json"})
    reply = json.loads(raw)
    require(reply["status"] == "success" and not reply.get("warnings")
            and reply["data"]["resultType"] == "vector", "Grafana response is incomplete or ambiguous")
    rows = reply["data"]["result"]
    require(isinstance(rows, list) and all(len(row["value"]) == 2
            and abs(float(row["value"][0]) - at.timestamp()) <= 1 for row in rows),
            "Grafana response does not match the requested observation time")
    return rows


def sample_age(row, at):
    stamp = float(row["value"][1])
    age = at.timestamp() - stamp
    require(math.isfinite(age) and 0 <= age <= MAX_AGE_SECONDS, "runtime sample is stale or out of range")
    return age


def live_health(config, sha, at, expected_version, *, version_kind):
    builds = prometheus(config, "timestamp(polylens2otel_build_info{labels})", at)
    require(len(builds) == 1 and builds[0]["metric"]["commit"] == sha, "exact deployed revision is missing or overlapping")
    build = builds[0]["metric"]
    version = build["version"]
    require(isinstance(version, str) and version and build["service_version"] == version,
            "runtime version labels disagree")
    require(version == expected_version, f"runtime does not report the {version_kind}")
    age = sample_age(builds[0], at)
    metrics = ("polylens2otel_collector_availability{labels}", "timestamp(polylens2otel_collector_availability{labels})")
    values, stamps = (prometheus(config, metric, at, build["version"]) for metric in metrics)
    names = ("job", "service_name", "service_version", "tenant_id")
    by_id = {row["metric"]["collector_id"]: row for row in values}
    stamps_by_id = {row["metric"]["collector_id"]: row for row in stamps}
    require(by_id and len(by_id) == len(values) and len(stamps_by_id) == len(stamps)
            and by_id.keys() == stamps_by_id.keys() and all(isinstance(key, str) and key for key in by_id),
            "collector samples are missing or ambiguous")
    for rows in (values, stamps):
        require(all(all(row["metric"][key] == build[key] for key in names) for row in rows),
                "collector samples do not share the exact deployment scope")
    require(all(build[key] == config[key] for key in ("job", "tenant_id"))
            and build["service_name"] == "polylens2otel", "runtime sample does not match the configured deployment")
    oldest = max(sample_age(row, at) for row in stamps)
    availability = [float(row["value"][1]) for row in values]
    require(all(value in (0, 1) for value in availability), "collector availability is malformed")
    if 0 in availability:
        raise Unhealthy("the exact deployed revision has unavailable collectors")
    errors = prometheus(config, "sum(increase(polylens2otel_http_5xx_total{labels}[5m]))", at, build["version"])
    require(len(errors) <= 1, "HTTP error evidence is ambiguous")
    count = float(errors[0]["value"][1]) if errors else None
    require(count is None or (math.isfinite(count) and count >= 0), "HTTP error count is malformed")
    return {"runtime_version": version, "build_age_seconds": round(age, 3), "oldest_collector_age_seconds": round(oldest, 3),
            "available_collectors": len(values), "http_5xx_increase": count}


def protected(sha):
    branch = github("/branches/main")
    return branch["protected"] is True and branch["commit"]["sha"] == sha


def verify(sha, mode, config):
    cutoff = utc(BOOTSTRAP_AT) if mode == "landing" and sha == BOOTSTRAP_SHA else None
    if cutoff:
        verify_bootstrap(sha, cutoff)
    else:
        require(protected(sha), "expected revision is no longer protected main")
    verify_ci(sha, cutoff)
    publisher(sha, cutoff)
    registry = Registry()
    tag = BOOTSTRAP_TAG.removeprefix("v") if cutoff else "main"
    digest = registry.image(tag, sha)
    identity = signer_identity(sha)
    if cutoff:
        require(digest == BOOTSTRAP_DIGEST, "historical image no longer binds the recorded digest")
    if cutoff or not signature_matches(digest, sha, identity, "push"):
        publisher(sha, cutoff, rc=True)
        require(signature_matches(digest, sha, identity, "workflow_run", cutoff),
                "no trusted image signature was verified")
        expected_version = tag if cutoff else prerelease_version(registry, sha, digest)
        version_kind = "historical runtime version" if cutoff else "signed prerelease version"
    else:
        expected_version = edge_version(sha, identity)
        version_kind = "signed Edge version"
    at = cutoff or datetime.now(timezone.utc)
    health = live_health(config, sha, at, expected_version, version_kind=version_kind)
    if not cutoff:
        require(registry.object("manifests", tag)[1] == digest and protected(sha),
                "protected main or image tag moved during verification")
    return {"state": "verified", "mode": "historical" if cutoff else "current",
            "revision": sha, "image_digest": digest, "image_digest_source": "registry",
            "observed_at": at.isoformat(), **health}


def main(args):
    try:
        require(1 <= len(args) <= 2 and SHA.fullmatch(args[0])
                and (len(args) == 1 or args[1] in {"current", "landing"}), "invalid deployment verification arguments")
        config_path = Path(os.environ["PL2O_DEPLOY_VERIFY_CONFIG"])
        require(config_path.is_file() and config_path.stat().st_size <= 4096,
                "deployment verification configuration is unavailable")
        config = json.loads(config_path.read_text())
        print(json.dumps(verify(args[0], args[1] if len(args) == 2 else "current", config)))
        return 0
    except Unhealthy as error:
        print(json.dumps({"state": "failed", "reason": str(error)}))
        return 1
    except Unread as error:
        print(json.dumps({"state": "unread", "reason": str(error)}))
    except (AttributeError, KeyError, IndexError, OSError, TypeError, ValueError, subprocess.SubprocessError):
        print(json.dumps({"state": "unread", "reason": "deployment evidence could not be read"}))
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
