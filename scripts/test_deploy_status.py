"""Controls for the protected deployment verification command."""

import base64
import contextlib
from datetime import datetime
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import urllib.error
import urllib.parse

import deploy_status as deploy


class EntryPoint(unittest.TestCase):
    def test_invalid_revision_is_unread_at_the_just_entry_point(self):
        fixture = "synthetic-invalid-input"
        result = subprocess.run(
            ["just", "deploy-status", fixture],
            cwd=Path(__file__).resolve().parents[1],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 2)
        self.assertIn("invalid deployment verification arguments", result.stdout)
        self.assertNotIn(fixture, result.stdout + result.stderr)


class DeploymentEvidence(unittest.TestCase):
    revision = deploy.BOOTSTRAP_SHA
    original = "2026-11-18T15:15:00Z"

    def setUp(self):
        scratch = Path(self.enterContext(tempfile.TemporaryDirectory()))
        token = scratch / "token"
        token.write_text("synthetic-read-token\n")
        self.config = {"origin": "https://example.invalid", "datasource_uid": "monitor",
                       "job": "synthetic-exporter", "tenant_id": "synthetic-deployment", "token_file": str(token)}
        config_file = scratch / "config.json"
        config_file.write_text(json.dumps(self.config))
        self.enterContext(patch.dict(os.environ, {"PL2O_DEPLOY_VERIFY_CONFIG": str(config_file)}))
        self.github = {}
        self.registry = {}
        self.labels = {"job": self.config["job"], "tenant_id": self.config["tenant_id"],
                       "service_name": "polylens2otel", "service_version": deploy.BOOTSTRAP_TAG[1:]}
        self.metric_revision = self.revision
        self.availability = 1
        self.age = 4
        self.evaluation_offset = 0
        self.error_count = None
        self.stamp_labels = dict(self.labels)
        self.observed_at = []
        self.branch = {"protected": True, "commit": {"sha": self.revision}}
        self.github["/branches/main"] = self.branch
        self.setup_publication()
        self.digest = self.make_index(self.revision)
        self.enterContext(patch.object(deploy, "fetch", self.fetch))
        self.enterContext(patch.object(deploy.subprocess, "run", self.run_gh))
        self.signature = self.enterContext(patch.object(deploy, "signature_matches", return_value=True))
        self.enterContext(patch.object(deploy, "BOOTSTRAP_DIGEST", self.digest))

    def item(self, **fields):
        return {"status": "completed", "conclusion": "success", "completed_at": self.original, **fields}

    def setup_publication(self):
        repo = {"full_name": deploy.REPO}
        arch_jobs = ("build (linux/amd64)", "build (linux/arm64)", "merge + sign + sbom")
        for file, event, job_names in (
            ("ci.yml", "push", ("ci-success",)),
            ("release-please.yml", "push", tuple(f"edge (:main image) / image / {name}" for name in arch_jobs)),
            ("auto-rc.yml", "workflow_run", ("rc / guard", "rc / tag + prerelease",
                                            *(f"publish / image / {name}" for name in arch_jobs))),
        ):
            run_id = deploy.BOOTSTRAP_RUNS[file]
            run = self.item(id=run_id, run_number=1, run_attempt=1, head_sha=self.revision, event=event,
                            path=f".github/workflows/{file}", head_branch="main",
                            repository=repo, head_repository=repo, check_suite_id=3)
            jobs = [self.item(id=run_id + index, run_id=run_id, run_attempt=1, name=name, head_sha=self.revision)
                    for index, name in enumerate(job_names)]
            self.github[f"/actions/workflows/{file}/runs?event={event}&head_sha={self.revision}&per_page=100"] = {
                "total_count": 1, "workflow_runs": [run]}
            self.github[f"/actions/runs/{run_id}/attempts/1"] = run
            self.github[f"/actions/runs/{run_id}/attempts/1/jobs?per_page=100"] = {
                "total_count": len(jobs), "jobs": jobs}
        self.check = self.item(id=deploy.BOOTSTRAP_RUNS["ci.yml"], app={"id": 15368}, name="ci-success",
                               check_suite={"id": 3}, head_sha=self.revision)
        self.github[f"/commits/{self.revision}/check-runs?filter=all&per_page=100"] = {
            "total_count": 1, "check_runs": [self.check]}
        content = f"  uses: {deploy.PUBLISHER}@{'a' * 40} # v1\n".encode()
        self.github[f"/contents/.github/workflows/publish.yml?ref={self.revision}"] = {
            "content": base64.b64encode(content).decode()}
        self.github["/pulls/174"] = {
            "merged": True, "merge_commit_sha": self.revision, "merged_at": self.original,
            "base": {"ref": "main", "repo": repo}}
        self.github[f"/releases/tags/{deploy.BOOTSTRAP_TAG}"] = {
            "prerelease": True, "draft": False, "tag_name": deploy.BOOTSTRAP_TAG, "published_at": self.original}
        self.github[f"/git/ref/tags/{deploy.BOOTSTRAP_TAG}"] = {
            "object": {"type": "commit", "sha": self.revision}}

    def registry_object(self, kind, data):
        raw = json.dumps(data, sort_keys=True).encode()
        digest = f"sha256:{hashlib.sha256(raw).hexdigest()}"
        self.registry[f"/{kind}/{digest}"] = raw, digest
        return digest

    def make_index(self, arm_revision):
        children = []
        for arch, revision in (("amd64", self.revision), ("arm64", arm_revision)):
            config = self.registry_object("blobs", {
                "architecture": arch, "os": "linux",
                "config": {"Labels": {"org.opencontainers.image.revision": revision}}})
            manifest = self.registry_object("manifests", {"config": {"digest": config}})
            children.append({"platform": {"architecture": arch, "os": "linux"}, "digest": manifest})
        digest = self.registry_object("manifests", {"manifests": children})
        self.registry["/manifests/main"] = self.registry[f"/manifests/{digest}"]
        self.registry[f"/manifests/{deploy.BOOTSTRAP_TAG[1:]}"] = self.registry[f"/manifests/{digest}"]
        return digest

    def prometheus(self, request):
        args = urllib.parse.parse_qs(request.query)
        query = args["query"][0]
        when = datetime.fromisoformat(args["time"][0]).timestamp()
        self.observed_at.append(when)
        self.assertIn('service_name="polylens2otel"', query)
        self.assertIn('tenant_id="synthetic-deployment"', query)
        if "build_info" in query:
            labels = {**self.labels, "commit": self.metric_revision, "version": self.labels["service_version"]}
            value = when - self.age
        else:
            self.assertIn(f'service_version="{self.labels["service_version"]}"', query)
            labels = {**self.labels, "collector_id": "synthetic"}
            if "timestamp(" in query:
                labels.update(self.stamp_labels)
                value = when - self.age
            elif "availability" in query:
                value = self.availability
            else:
                labels, value = {}, self.error_count
        rows = [] if value is None else [{"metric": labels, "value": [when + self.evaluation_offset, str(value)]}]
        return {"status": "success", "data": {"resultType": "vector", "result": rows}}

    def run_gh(self, args, **kwargs):
        self.assertEqual(args[:4], ["gh", "api", "--hostname", "github.com"])
        prefix = f"repos/{deploy.REPO}"
        self.assertTrue(args[-1].startswith(prefix))
        body = json.dumps(self.github[args[-1].removeprefix(prefix)]).encode()
        return subprocess.CompletedProcess(args, 0, stdout=body)

    def fetch(self, url, headers=None, registry_blob=False):
        if url.startswith(deploy.REGISTRY):
            self.assertEqual(headers["Authorization"], "Bearer synthetic-public-token")
            relative = url.removeprefix(deploy.REGISTRY)
            self.assertEqual(registry_blob, relative.startswith("/blobs/"))
            return self.registry[relative]
        if url.startswith("https://ghcr.io/token?"):
            self.assertIsNone(headers)
            return b'{"token":"synthetic-public-token"}', None
        request = urllib.parse.urlsplit(url)
        self.assertEqual(request.hostname, "example.invalid")
        self.assertEqual(request.path, "/api/datasources/proxy/uid/monitor/api/v1/query")
        self.assertEqual(headers["Authorization"], "Bearer synthetic-read-token")
        self.assertNotIn("synthetic-read-token", url)
        return json.dumps(self.prometheus(request)).encode(), None

    def run_command(self, expected_code, mode="current"):
        with contextlib.redirect_stdout(io.StringIO()) as sink:
            code = deploy.main([self.revision, mode])
        self.assertEqual(code, expected_code, sink.getvalue())
        self.assertNotIn("synthetic-read-token", sink.getvalue())
        self.assertNotIn("synthetic-deployment", sink.getvalue())
        return json.loads(sink.getvalue())

    def test_healthy_exact_protected_deployment(self):
        result = self.run_command(0)
        self.assertEqual((result["state"], result["mode"], result["revision"], result["image_digest"],
                          result["available_collectors"], result["build_age_seconds"], result["http_5xx_increase"]),
                         ("verified", "current", self.revision, self.digest, 1, 4, None))
        self.error_count = 0
        self.assertEqual(self.run_command(0)["http_5xx_increase"], 0)

    def test_explicit_unhealthy_exact_collector(self):
        self.availability = 0
        self.assertIn("unavailable collectors", self.run_command(1)["reason"])

    def test_explicit_failed_exact_publication(self):
        run_id = deploy.BOOTSTRAP_RUNS["release-please.yml"]
        self.github[f"/actions/runs/{run_id}/attempts/1"]["conclusion"] = "failure"
        self.assertEqual(self.run_command(1)["state"], "failed")

    def test_missing_selected_publisher(self):
        run_id = deploy.BOOTSTRAP_RUNS["release-please.yml"]
        self.github[f"/actions/runs/{run_id}/attempts/1/jobs?per_page=100"] = {"total_count": 0, "jobs": []}
        self.assertIn("required workflow job is missing", self.run_command(2)["reason"])

    def test_wrong_ci_provenance_or_oci_arm_revision(self):
        self.check["app"]["id"] = 42
        self.assertIn("Actions-owned ci-success is missing", self.run_command(2)["reason"])
        self.check["app"]["id"] = 15368
        self.make_index("b" * 40)
        self.assertIn("platform image does not describe the expected source revision", self.run_command(2)["reason"])

    def test_wrong_deployed_revision_or_signature_is_unread(self):
        self.metric_revision = "b" * 40
        self.assertIn("exact deployed revision", self.run_command(2)["reason"])
        self.metric_revision = self.revision
        self.signature.return_value = False
        self.assertIn("no trusted image signature", self.run_command(2)["reason"])

    def test_stale_or_wrong_observation_is_unread(self):
        self.age = 91
        self.assertIn("sample is stale", self.run_command(2)["reason"])
        self.age = 4
        self.evaluation_offset = 2
        self.assertIn("requested observation time", self.run_command(2)["reason"])

    def test_collector_timestamps_must_share_the_deployment(self):
        self.stamp_labels["service_version"] = "0.0.1-test"
        self.assertIn("exact deployment scope", self.run_command(2)["reason"])

    def test_historical_bootstrap_uses_only_its_own_observation(self):
        self.branch["commit"]["sha"] = "b" * 40
        self.assertIn("no longer protected main", self.run_command(2)["reason"])
        result = self.run_command(0, "landing")
        cutoff = deploy.utc(deploy.BOOTSTRAP_AT)
        self.assertEqual(self.observed_at, [cutoff.timestamp()] * 4)
        self.assertEqual(result["mode"], "historical")
        self.assertEqual(self.signature.call_args.args[3:], ("workflow_run", cutoff))

    def test_historical_bootstrap_rejects_another_version_of_the_same_sha(self):
        self.labels["service_version"] = "0.0.1-test"
        self.stamp_labels["service_version"] = "0.0.1-test"
        self.assertIn("historical runtime version", self.run_command(2, "landing")["reason"])

    def test_later_workflow_or_moved_tag_cannot_verify_bootstrap(self):
        run_id = deploy.BOOTSTRAP_RUNS["auto-rc.yml"]
        run = self.github[f"/actions/runs/{run_id}/attempts/1"]
        run["completed_at"] = "2026-11-18T16:02:00Z"
        self.assertIn("completed after the historical observation", self.run_command(2, "landing")["reason"])
        run["completed_at"] = self.original
        self.github[f"/git/ref/tags/{deploy.BOOTSTRAP_TAG}"]["object"]["sha"] = "b" * 40
        self.assertIn("historical tag no longer binds", self.run_command(2, "landing")["reason"])


class Transport(unittest.TestCase):
    def test_redirect_does_not_send_grafana_credential_elsewhere(self):
        url = "https://example.invalid/api/v1/query"
        error = urllib.error.HTTPError(url, 307, "Redirect", {"Location": "https://elsewhere.invalid"}, None)
        with patch("urllib.request.build_opener") as build_opener:
            build_opener.return_value.open.side_effect = error
            with self.assertRaises(urllib.error.HTTPError):
                deploy.fetch(url, {"Authorization": "Bearer synthetic"})
            self.assertEqual(build_opener.return_value.open.call_count, 1)

    def test_config_blob_redirect_does_not_forward_registry_credential(self):
        url = f"{deploy.REGISTRY}/blobs/sha256:synthetic"
        cdn = "https://pkg-containers.githubusercontent.com/synthetic"
        error = urllib.error.HTTPError(url, 307, "Redirect", {"Location": cdn}, None)
        with patch("urllib.request.build_opener") as build_opener:
            opener = build_opener.return_value.open
            response = opener.return_value
            response.headers = {}
            response.read.return_value = b"{}"
            opener.side_effect = [error, response]
            deploy.fetch(url, {"Authorization": "Bearer synthetic"}, registry_blob=True)
            redirect = opener.call_args.args[0]
            self.assertEqual(redirect.full_url, cdn)
            self.assertFalse(redirect.has_header("Authorization"))


class ImageSignature(unittest.TestCase):
    def make_bundle(self, predicate, name=None):
        subject = {"digest": {"sha256": deploy.BOOTSTRAP_DIGEST[7:]}}
        if name is not None:
            subject["name"] = name
        payload = {"predicateType": predicate, "subject": [subject]}
        bundle = {"dsseEnvelope": {"payload": base64.b64encode(json.dumps(payload).encode()).decode()}}
        return json.dumps(bundle).encode()

    def signature_matches(self, bundle, expected, verifier_result=0):
        def run(args, **kwargs):
            if args[1:3] == ["download", "signature"]:
                self.assertEqual(args[-1], f"{deploy.IMAGE}@{deploy.BOOTSTRAP_DIGEST}")
                return subprocess.CompletedProcess(args, 0, stdout=bundle)
            if args[1] == "verify":
                image_response = [{"critical": {"image": {"docker-manifest-digest": deploy.BOOTSTRAP_DIGEST}}}]
                return subprocess.CompletedProcess(args, 0, stdout=json.dumps(image_response).encode())
            self.assertEqual(args[1], "verify-blob-attestation")
            self.assertIn(deploy.SIGNATURE_TYPE, args)
            self.assertIn(deploy.BOOTSTRAP_DIGEST[7:], args)
            self.assertIn("--check-claims=true", args)
            self.assertEqual(Path(args[args.index("--bundle") + 1]).read_bytes(), bundle)
            return subprocess.CompletedProcess(args, verifier_result, stdout=b"")

        with patch.object(deploy.subprocess, "run", side_effect=run) as calls:
            self.assertEqual(deploy.signature_matches(deploy.BOOTSTRAP_DIGEST, deploy.BOOTSTRAP_SHA,
                                                      "synthetic-identity", "push"), expected)
        return calls

    def test_provenance_for_the_digest_is_not_an_image_signature(self):
        provenance = self.make_bundle("https://slsa.dev/provenance/v1", deploy.IMAGE)
        self.signature_matches(provenance, False).assert_called_once()

    def test_cosign_must_verify_the_expected_image_signature_itself(self):
        signature = self.make_bundle(deploy.SIGNATURE_TYPE)
        self.assertEqual(self.signature_matches(signature, True).call_count, 2)
        self.assertEqual(self.signature_matches(signature, False, verifier_result=1).call_count, 2)
        other_image = self.make_bundle(deploy.SIGNATURE_TYPE, "ghcr.io/synthetic/image")
        self.signature_matches(other_image, False).assert_called_once()
