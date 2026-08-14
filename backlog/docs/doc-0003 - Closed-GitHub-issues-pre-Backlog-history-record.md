---
id: doc-0003
title: Closed GitHub issues (pre-Backlog history record)
type: other
created_date: '2026-08-14 16:58'
updated_date: '2026-08-14 17:03'
---
> **This is the record of the work tracked on GitHub Issues before polylens2otel moved to
> Backlog.md on 2026-08-14. The issues were DELETED from GitHub on that date and there is no JSON
> archive — Rob's explicit call. `gh issue view <N>` will 404 and nothing else holds their bodies,
> so this document is not an index into a record, it *is* the record.** Everything load-bearing that
> was in those 49 issues and 81 comments — frozen decisions with their reasons, corrections, live
> evidence, closing SHAs — is reproduced below. Anything not here is gone.
>
> **Redaction.** The line this repo already draws is *identifiers and addresses out, device names
> in*: `deskie` and `extra` appear in tracked fixtures and tests, so keeping them here preserves
> correlation with the code. Tenant, collection and policy IDs, MAC addresses, private and external
> IP addresses, the deployment host name and every credential value are replaced with a placeholder
> (`<tenant-id>`, `<deploy-host>`) or omitted entirely. One real value maps to one placeholder
> throughout, including inside quoted material.
>
> **Why these were not imported as tasks.** Backlog IDs follow creation order, so an imported task
> could never carry the `#NNN` this history already cites from commit messages and code comments.
> Keeping GitHub numbers as the only ID space over this history is what keeps those references
> resolvable. Forty-five `Done` rows would also drown the board's only real signal — what is left.
> **Cite closed work as `#NNN`; cite new work as `plo-NNNN`.**
>
> The two pieces of genuinely open work at migration time became `plo-0001` and `plo-0002`.

Three waves ran between 2026-08-12 and 2026-08-13, all planned by Claude and executed by Codex, all
committing straight to `main`. The repo went from empty to v0.2.0 released, deployed, and emitting
metrics, logs, traces and profiles to Grafana Cloud.

---

## Frozen product decisions — still binding

These were answered by Rob before each wave and marked "do not reopen". They are the most expensive
content here to re-derive, and several are non-obvious.

### Wave 1 (#1), D1-D20

**D1 Shape.** Single static Go binary, OTLP **push only**. No Prometheus pull endpoint, no HA, no
leader election, single instance.

**D2 Identity.** Module `github.com/rknightion/polylens2otel`, binary and entrypoint
`cmd/polylens2otel`, image `ghcr.io/rknightion/polylens2otel`.

**D3 Dependencies.** koanf v2 with yaml/file/env/structs providers; `go.opentelemetry.io/otel` and
the OTLP metric/log/trace http+grpc exporters in **lockstep** — bumped together, never
independently, Renovate-grouped; `github.com/grafana/pyroscope-go`; `github.com/coder/websocket` for
the subscription transport. A hand-rolled RFC 7616 Digest client was preferred over a dead
dependency in a public repo.

**D4 Config.** koanf, precedence defaults < YAML < env. Prefix `PL2O_`, double-underscore nesting
(`otlp.endpoint` → `PL2O_OTLP__ENDPOINT`). Secrets are **env-only, never in YAML**.

**D5 Licence: AGPL-3.0.** Rob's call; graph2otel is the precedent, not the Apache-2.0 siblings.

**D6 Multi-tenant from day one.** `lens.tenants` optional; empty means discover via
`{ tenants { id name } }`. `tenant.id` is stamped on **every** signal at the Emitter boundary, never
by a collector.

**D7 Syslog is out of scope, deliberately.** The deployment host's Alloy Fleet Management pipeline
already parses the handsets' syslog into Loki, including a stage that fixes Poly's lying wire
severity. `docs/` points at it and says polylens2otel does not duplicate it. **Do not build a syslog
receiver** — its absence is a decision, not a gap.

**D8 Read-only, enforced by a gate.** See the *Wave operating model* doc; the reasons are recorded
there.

**D9 Namespaces.** `polylens.*` for Lens-sourced, `polyphone.*` for phone-REST-sourced,
`polylens2otel.*` for self-observability. A collector emitting outside its source's namespace is a
bug. OTLP→Prometheus normalization is real: gauges gain `_ratio`/`_seconds`/`_percent`, counters
gain `_total`, and dashboards and alerts use the **normalized** names.

**D10 The metric set**, frozen. Resource attributes `device.id`, `device.name`, `device.mac`,
`device.model`, `site.name`, `tenant.id` (`net.host.ip` was removed in wave 2 — see #33).

| Metric | Type | Source |
|---|---|---|
| `polylens.device.connected` | gauge 0/1 | poll + stream |
| `polylens.device.last_detected_seconds` | gauge (age) | poll |
| `polylens.device.last_config_request_seconds` | gauge (age) | poll |
| `polylens.device.firmware_info` | gauge=1, labels `version`,`build` | poll |
| `polylens.device.firmware_current` | gauge 0/1 vs `availableProductSoftwareByPid` | poll |
| `polylens.device.active_calls` | gauge, label `state` | poll |
| `polylens.stream.connected` | gauge 0/1 | ws engine |
| `polyphone.uptime_seconds` | gauge | `network/stats` |
| `polyphone.network.packets` | **counter**, label `direction=rx\|tx` | `network/stats` |
| `polyphone.line.registered` | gauge 0/1, labels `line`,`label`,`sip_address` | `lineInfo` |
| `polyphone.lines_total` | gauge | `lineInfo` |
| `polyphone.config_param_source` | gauge=1, labels `param`,`source` | `config/get` |
| `polyphone.api_state` | gauge, label `state` | probe result |

`RxPackets`/`TxPackets` are monotonic since boot and **reset on reboot** — emitted as OTel counters
so the backend handles the reset; do not compute a delta.

**Logs:** one OTLP log line per CDR row. Log attributes are Loki **structured metadata**, not stream
labels — only `service_name` is a stream label, so `{event_name="polylens.cdr"}` matches zero rows
silently. Always `{service_name="polylens2otel"} | event_name=…`. This is the most common way to
build an alert that never fires.

**D11 Self-observability is a first-class requirement**, explicitly asked for. `polylens2otel.*`:
`build_info`; `collector.duration`, `collector.availability`, `collector.expected_interval` per
collector ID; `http.client.request.duration` labelled `source=lens|phone`; `http_4xx`, `http_5xx`;
`api.unexpected` (a response shape the code did not anticipate — report, never drop);
`auth.token.refresh` and `auth.token.expires_seconds`; `stream.reconnects`, `stream.messages`,
`stream.last_message_seconds`; `ingest.emitted_points`, `ingest.source_records`;
`checkpoint.persist.errors`. Plus traces: one span per collector run, one child span per outbound
HTTP call. Pyroscope is wired but inert unless credentialed.

**D12 `polyphone.api_state` replaces a boolean `reachable`**, because a boolean conflates two
genuinely different failures. Four states, exactly one emitted as 1 per device:

| `state` | Means |
|---|---|
| `ok` | TLS connected, cert CN matched the device MAC, digest accepted, 2xx |
| `api_disabled` | TLS connected and cert matched, but the management path 404s **unauthenticated** |
| `auth_failed` | reached and identified, digest rejected → 401 |
| `unreachable` | no TCP/TLS connection |

`api_disabled` is the state a boolean would have called "unreachable", sending an operator to check
the network for a config problem. **404 before auth means `api_disabled`; 401 means `auth_failed`.**

**D13 `config_param_source` allowlist**, six by default and **never** all 5047 parameters:
`reg.1.address`, `reg.2.address`, `reg.1.label`, `device.syslog.serverName`,
`tcpIpApp.sntp.address`, `softkey.1.enable`. `reg.1.address` flipping from `source=config` to
`source=default` is the single unambiguous signal for a policy-replacement wipe.

**D14 Phone target discovery.** Candidates come from Lens `deviceSearch.internalIp`; a **static
per-device override** (`phone.targets`) takes precedence, and without it one of the two real devices
is undiscoverable because Lens reports a dead address for it. **The phone's own TLS certificate is
the identity proof** — its CN is the device MAC; connect, read the peer certificate, and require
`CN == device MAC` before sending any credential. A mismatch classifies `unreachable` and emits
`api.unexpected`; it does **not** authenticate. The chain is self-signed, so chain verification is
off by default with a `phone.tls.ca_file` escape hatch, but **CN identity checking is not optional
and has no off switch** — "skip verification" and "verify identity without a public CA" are
different things and only the second ships. Discovery **never scans**.

**D15 Cardinality.** No central limiter; the licence and its observable expiry are in the *Wave
operating model* doc. The standing rule: per-device aggregates go to metrics, per-call and
per-parameter-value data goes to logs. `cardinality.max_devices` (default 500) logs loudly and emits
`api.unexpected` above threshold rather than silently exploding a bill.

**D16 Timestamps.** The Emitter only sets non-zero timestamps. A record with no parseable event time
is **dropped**, never stamped on arrival — stamping would silently claim it happened now. CDR rows
carry `startTime`/`endTime` and **no duration field**, so duration is computed. The CDR `id` field
returns `null`, so dedupe is on `deviceId + startTime`.

**D17 Checkpoints.** File-based, namespaced per tenant + collector, under `state.dir` (default
`/var/lib/polylens2otel`). **Fail fast at startup if the path is unwritable.**

**D18 Release wiring**, and it is a gate rather than a list of jobs:

```text
release-please.yml  on: push to main
  job release-please  → mints the broker token, maintains the release PR, may cut a release
  job publish         → if release_created == true   → ./.github/workflows/publish.yml
  job edge            → if release_created != true   → ./.github/workflows/publish.yml (tag "")
  job binaries        → if release_created == true   → shared binaries.yml (GoReleaser, --skip=docker)
```

**`publish.yml` is `workflow_call` + `workflow_dispatch` ONLY — it must not have its own `push`
trigger.** `release_created` is what stops `publish` and `edge` both firing on one push; adding a
push trigger puts two builds on every commit to `main` racing for the same `:main` tag. That is a
real defect that looks like a working pipeline.

Runtime secrets on the deployment host are a root-only `0600` `.env` beside `compose.yml`. **There
is no OpenBao-to-compose path and one must not be invented here** — CI credentials and runtime
credentials are separate mechanisms and stay that way. Release-please mints its own token per run
from the broker; `permission-set:` and `role:` are both `release-please-polylens2otel` (they match
for release-please and deliberately differ for docs-sync).

**D19 Public-repo hygiene from day one**, even while private. `scripts/check-secret-hygiene.sh`
runs in CI.

**D20 Commits.** Conventional Commits, straight to `main`, pushed immediately, no branches, no PRs,
green state between units of work.

### Wave 2 (#31) additions

1. First release is **v0.1.0**, not 1.0.0.
2. Deployment-host compose ownership is `rob:rob`, `.env` `0600`.
3. `phone.auth.from_lens_policy` implemented after the gate, default **false** (wave 3 flipped it).
4. Process telemetry emitted before tenant discovery has no `tenant.id`; the `_discovery` sentinel
   was deleted rather than faked.
5. `polylens2otel.startup` is a stable, documented public log event.
6. Docs sync uses permission-set `docs-sync` and role `docs-sync-polylens2otel` — these deliberately
   differ, and the hub dispatch needs **`contents: write`**, not `actions: write`.
7. Public CodeQL/Scorecard jobs; the private-repository disposition branches were removed.
8. State-enum gauges emit the full enum every cycle: current=1, every non-current member=0.
9. Metric series identities use stable attributes only; `net.host.ip` must not key a device series.
10. Existing `testdata/` fixtures are immutable; healthy fixtures are added alongside.
11. The documented phone password is stale; only the environment-held live credential is used and it
    is never exposed.

### Wave 3 (#47) additions

**D1 No credential rotation** — wave 2's report called it blocking and was **wrong in its severity**.
Traced facts, so nobody reopens it: exactly two parameters in the local config archive carry values
(`device.auth.localAdminPassword`, `device.auth.localUserPassword`); every `reg.*.auth.password` and
`reg.*.auth.userId` is **empty**, so no SIP credential was ever exposed. All 544 objects in the
repo's full history were scanned against both values and the live credential appears **nowhere** —
the only match is a 3-digit legacy default matching incidentally inside commit metadata and object
hashes, which is not a leak. The exposure is confined to a session transcript inside the
tailnet-only private Forgejo backup, the same trust boundary the value already occupies. Nothing
reached a public surface.

**D3** `0.2.0` was cut from the already-proven head **before** any wave-3 commit, so the release
ships a verified state and exists even if later lanes park.

**D4 `1.0.0` remains forbidden.** See the *Wave operating model* doc for the two conditions and
their current state.

**D5 The deployment host follows `:main`, not a release digest** — Rob: *"<deploy-host> should follow
:main so it's always on dev"*. This retired wave 2's digest-pin criterion. Knowingly accepted consequence:
a broken `main` now reaches the live service automatically.

**D6** A one-purpose write licence on `rknightion/.github` for the actionlint installer fix and its
same-pattern sweep. **That licence ended when the sweep landed** and does not extend to future waves.

**D9 `phone.auth.from_lens_policy` defaults to `true`** in all deployments — safe only because the
wave-2 fallback exists, which is why all four fallback paths had to be proven.

**D10 GHCR cleanup prunes untagged manifests only.** Never a release manifest, never `buildcache-*`,
and `main-<sha>` tags are retained because the deployment host depends on `:main`.

**F1** The configurable `config_params` allowlist **already existed and shipped in v0.1.0**
(`PhoneConfig.ConfigParams`, six defaults, `PL2O_PHONE__CONFIG_PARAMS`, documented). The only thing
missing was a cardinality guard, limit **50**. Named in advance as a re-implementation trap.

**F2** `(*Client).NetworkInfo` and `(*Client).CallLogs` were fully implemented with nothing calling
them — dead surface against endpoints already proven to return 200. Activating them was the whole of
two wave-3 lanes.

---

## The Lens winning-policy contract (#40) — live-proven, undocumented upstream

Context7's Poly Lens corpus has **no** documentation for effective policy inheritance, so the live
read-only schema and the Lens UI are authoritative. This chain cost real API calls to derive.

**Do not call `getPolicies(deviceId: ...)` to find the winner — it returns an empty list for both
governed devices.** Ask the parameter resolver which source won for the specific parameter:

```graphql
query WinningPolicySource($tenantID: String!, $deviceID: String!) {
  getDeviceParametersExtended(
    tenantId: $tenantID
    deviceId: $deviceID
    scope: DEVICE
    sendOnlyChanged: false
  ) { name policyDeploymentScope collectionId collectionName }
}
```

Select the row whose `name` is `device.auth.localAdminPassword`; its `policyDeploymentScope` and
`collectionId` are the backend-resolved winning source. Then resolve the policy ID without
requesting values, and only then read that one policy:

```graphql
query WinningPolicies($tenantID: String!, $groupID: String!) {
  getPolicies(tenantId: $tenantID, groupId: $groupID, policyScope: FAMILY_GROUP) {
    policyId configurationAttributes { name }
  }
}

query PhoneLocalAdminPassword($policyID: String!) {
  getPolicyById(policyId: $policyID) { configurationAttributes { name currentValue } }
}
```

**The live schema field is `currentValue`, not `value`.**

Precedence, high to low: individual device → device user group → site → device group → account. At
account, site, group and user-group levels a device-**model** policy overrides the corresponding
device-**family** policy; the API spells the variants `DEVICE_MODEL`/`FAMILY_MODEL`,
`SITE`/`FAMILY_SITE`, `GROUP`/`FAMILY_GROUP`. **The selector already returns the winning variant, so
the exporter must not recreate precedence locally** — map the returned source to its lookup, reject
zero or ambiguous candidates, and fall back to the configured password.

Proven against both devices: winning scope `FAMILY_GROUP`, one candidate policy each, selected value
matching the runtime credential in-process, never emitted.

## Live Lens GraphQL variable types — the canary's frozen contract

Device and tenant IDs are `String!`; firmware `pid` is `ID!`. The three historical wrong types
(`active_calls` `ID!`, CDR tenant `ID!`, firmware `String!`) are negative controls the canary must
**fail** on.

The exact working CDR argument list:

```graphql
meetingRecordLists(
  tenantId: ["<tenant-id>"]
  meetingRecordType: [CDR]
  timeRange: { relativeRange: { increment: DAY, value: 2 } }
  first: 100
)
```

## The websocket contract (#2)

A named `subscription DevStream` over `graphql-transport-ws`. Two header-only attempts returned
`subscription 401`; **the reproducible live sequence requires the bearer on both the HTTP upgrade
and in `connection_init.payload`.** It upgraded with HTTP 101, received `connection_ack`, accepted
`modelId`/`siteId`/`roomId`, and held 15s with no data frame — expected edge-triggered behaviour, so
the stream is a latency supplement to polling and never a replacement. One probe overran its hold
because ping traffic reset the socket timeout; a wall-clock probe is the correct measurement.

---

## Wave 1 — initial implementation (#1, closed 2026-08-12)

Sixteen lanes L0-L15, final head `b81e8a6`, 23 commits after the initial commit. Every exact-SHA
workflow green; the release edge path built both architectures, merged/signed/SBOM'd the image and
published Helm.

| # | Lane | Closed by | Substance |
|---|---|---|---|
| #2 | L0 freeze skeleton and seams | `ffe7235` | Captured every live fixture before fan-out. Contention measured: a ninth collector touches exactly two files. Commit signing failed once because the encrypted SSH key could not prompt non-interactively; a one-command `commit.gpgsign=false` override was used without changing config. Follow-up `fb6b411` fixed a hygiene regex that falsely classified Go prose and its own YAML canary as credentials. |
| #3 | L1 read-only Lens GraphQL client | `09d87dd` | JSON OAuth token bodies with caching, lowercase bearer semantics, retained-but-redacted 4xx bodies, stable `pageSize`, bounded rate-limit retry, and mutation rejection **before transport**. |
| #4 | L2 read-only phone REST client | `f359e28` | Digest reads, POST-only `config/get`, mandatory certificate-CN identity before credentials, and the 404/`api_disabled` vs 401/`auth_failed` split, asserted against both device fixtures. |
| #5 | L3 CI, release-please, publishing | `9bbafd2` + `1740852`, `796485a`, `afe18e7` | **Reopened twice.** Live runs exposed private-repository contracts local validation could not prove: the shared Scorecard job omitted `contents:read`, Docker Security omitted `actions:read`, SARIF upload and build attestations were unavailable, and the release caller had not delegated `actions:read` to nested publish jobs. Scanners were localized to run and retain SARIF without Code Security; CodeQL took an explicit private-phase skip. Also fixed four zizmor ref-version comment mismatches. |
| #6 | L4 container, Helm, compose | `cf255d8` | `helm lint` 0 failed; image built as UID/GID 65532. Noted: the build downloaded base layers despite the child no-network contract. |
| #7 | L5 config documentation generator | `9b0057a` | Covers all 46 frozen leaf keys, preserves env-only secrets; the deliberate-drift check failed as intended before regeneration. |
| #8 | L6 Lens metric collectors | `66de137` | Fixed-clock timestamp ages, empty active-call behaviour, firmware comparison, cardinality guard. |
| #9 | L7 named Lens websocket stream | `cf432ff` | 50 race-enabled runs. Proves the exact named subscription, one all-device subscription, ACK-timeout/close reconnects, quiet-stream stability. |
| #10 | L8 CDR logs and watermarks | `43d766c` | Two real sanitized rows emit with computed 9s/16s durations, `deviceId+startTime` dedupe, restart-surviving tenant-hashed watermark, invalid-time **drop**. |
| #11 | L9 phone metrics, both broken states | `0f11d4d` | Per-instance: `deskie` API-on but unregistered with default config; `extra` emits exactly `api_disabled` and **no** authenticated metrics. |
| #12 | L10 target resolution, credential safety | `6ee257c` + `b5b5c9b` | Static override precedence, no scanning, real TLS CN mismatch rejected before any Digest request, redaction proven **under every `fmt` verb**. `b5b5c9b` replaced five live private-IP test literals with RFC 5737 documentation addresses. |
| #13 | L11 self-observability | `400b415` | Build info, scheduler duration/availability/interval, source-tagged HTTP duration/4xx/5xx, child-span transport instrumentation. |
| #14 | L12 dashboards and alerts as code | `527a82c`, seam `1a70d67` | 29 metrics accounted for, 3 catalog-resolved alert expressions. Waivers empty; stale or blank waivers fail the gate. |
| #15 | L13 public documentation | `46cd6ee`, gate `3e986d9` | 19 documented commands across 9 files, all verified to exist. Docs state the read-only boundary, the CN check, no scanning, no call-quality data, the deliberate syslog exclusion and the structured-metadata LogQL route. |
| #16 | L14 integration and exact-SHA CI | `b2fb09d` | First `make check` failed lint with 22 findings (17 staticcheck capitalization, 3 gosec, bodyclose, errcheck), all fixed. Every workflow green at the exact SHA; broker mint succeeded. |
| #17 | L15 deploy and prove Grafana Cloud data | `b81e8a6` (closed manually — external state) | Deployed from GHCR `:main`; container running, restart count 0, `.env` `0600`, state dir `65532:65532` `0750`. **Live startup exposed the three GraphQL variable-type defects** fixtures had hidden; boundary tests failed first for the right reason, then the minimal fix landed. Grafana Cloud returned both Lens devices, both phones, build info, availability=1 for all eight collectors, and the startup log. A post-deployment ownership drift (compose and `.env` had become `rob:rob`) was caught and restored in the completion audit. |

**The acceptance evidence that became impossible.** Wave 1 was designed around a broken fleet — one
handset unregistered with a factory-placeholder line, the other with its REST API wiped off. Both
were **repaired externally before deployment**, so the `registered=0`, `reg.1.address source=default`
and `api_disabled` assertions were recorded as **not proven due to fleet drift** rather than
converted into passes or into exporter failures. The degraded fixtures captured in L0 are the only
surviving coverage of that state.

## Wave 2 — stabilise and release v0.1.0 (#31, closed 2026-08-12)

Two phases with a hard gate; no phase-2 work started until all seven gate items passed.

**The two live defects that motivated the wave** are described in the *Wave operating model* doc
(§2): the state-enum gauge that never zeroed superseded members, and `net.host.ip` keying a metric
series so a two-device fleet counted three.

| # | Lane | Closed by | Substance |
|---|---|---|---|
| #33 | P1-L1 state enums and stable identity | `418edbe` | Failing-first tests reproduced **both** defects before the fix. The `api_disabled` alert remains correct and now clears. |
| #34 | P1-L2 real commit and build date | `c1d9358` | Local binary proved non-placeholder values. No local-Docker workaround was added (the dev machine has no Docker); image proof was deferred to the gate. |
| #35 | P1-L3 public security jobs and docs sync | `41fd3e9` | Private-disposition branches removed; zizmor `No findings to report (34 suppressed)`. |
| #36 | P1-L4 reset release baseline to 0.1.0 | `768be49` + `345fd53` | **`bump-minor-pre-major` alone did not work** — a live run kept the release PR at 1.0.0. The documented one-shot `release-as: 0.1.0` did, rewriting the PR with branch manifest `{".":"0.1.0"}`. The one-shot override was removed after the release merged (`cc530bf`). |
| #37 | P1-L5 validate Grafana consumers | no file change | Accepted with **no owned-file changes** — the alert expression was already correct under a one-hot enum and no generated consumer referenced `net.host.ip`. A lane correctly closing with an empty diff. |
| #38 | P1-L6 document corrected contracts | `d9b6e1e` | Full-enum emission, stable-identity-only series, the startup event and image build metadata all frozen in docs. |
| #39 | GATE release v0.1.0 and prove live | `6f6bb22` | All seven items passed. Every launch-open PR resolved: #19, #21, #24-#27, #29, #30 merged; #28 applied directly as `37bb19b` and closed as conflicting; the stale 1.0.0 release PR #22 closed and replaced. Release PR #45 merged as `772feff`; **v0.1.0 published with 15 assets**. Live counts came from the changed release build, not a restart. |
| #40 | P2-L7 winning-policy selection | `3d6923f` | The contract is reproduced in full above. |
| #41 | P2-L8 opt-in Lens-policy phone auth | `d660a90` | Implements source → policy ID → `getPolicyById`. A temporary live run used a **distinct fallback canary** so the fallback path could not silently pass. Formatting tests cover all `fmt` verbs and **found and fixed a `%p` backing-string leak**. |
| #42 | P2-L9 Lens variable-type drift canary | `773a6ec` | Passes the three correct live types and fails all three historical wrong ones under an env flag. |
| #43 | P2-L10 prove trace and profile retrieval | `1962243` | Tempo returned a trace rooted at `collector.phone.lines` with 12 `http.client.request` child spans; Pyroscope returned a non-empty CPU profile containing `internal/collector.(*Scheduler).loop`. **Backend retrieval, not exporter configuration** — configured exporters, successful writes and empty results are explicitly not proof. Durable record: `docs/verification.md`. |
| #44 | P2-L11 healthy fixtures, all-state assertions | `63f5b70` | Six sanitized healthy fixtures added **beside** the degraded ones; all seven pre-existing fixtures matched their pre-lane SHA-256 exactly. Both devices expose four API records representing two logical SIP registrations. |

## Wave 3 — ship 0.2.0, harden CI, activate phone telemetry (#47)

Twelve lanes plus L0. Final head `ad36b32`. **The wave closed truthfully partial**: every lane
landed except P4-L10 and P4-L12, which are blocked on a time condition, not on work. That remainder
is now `plo-0001`.

| # | Lane | Closed by | Substance |
|---|---|---|---|
| #50 | L0 pre-fan-out pass | `4ea038f` (closed manually) | Froze semconv names and the registry call list, made nil collector registration safe (test failed first with `registered collectors = 1; want 0`), added both 5m interval keys, SHA-256 baselined all 21 starting fixtures and captured four new sanitized per-device fixtures. Re-checked the 1.0.0 conditions: `{"tenant_count":1,"cdr_rows_in_two_day_first_page":9}`. **GitHub did not auto-close it** because the commit carried literal `\n` before the `Closes #50` footer. |
| #52 | P1-L1 publish v0.2.0 | `82a47ce` | PR #46 merged **unchanged** before any wave-3 commit; `v0.2.0` resolves to that exact SHA with all 15 assets; PR #62 opened proposing 0.3.0. |
| #59 | P1-L2 deployment host to `:main` | closed manually (external-only) | One-line compose change to `:main` with no digest. The running image changed unattended under Watchtower with restart count 0 and **no manual pull** — the activation recreate was explicitly not claimed as unattended proof. |
| #56 | P2-L3 stream reconnect shutdown race | `f2f6882` | `stream.go` consults `ctx.Err()` before returning transport errors, so a cancelled context is a clean shutdown even when the transport fails concurrently. 20 race-enabled repetitions. No wait/retry/relaxed-assertion false pass. |
| #48 | P2-L4 harden network installers | `b5e4225`, `902bf45` | Bounded retry with backoff or checksum pinning for every in-scope install; both Syft `go install` precommands replaced with a retried SHA-256-verified archive download. No action SHA pin loosened. |
| #53 | P2-L5 shared actionlint workflow | shared `328bc72`, caller `9c3bce4` | Replaced a pipe-to-shell installer with a retried checksum-verified download, after a 13-file sweep in which every inspected file including clean ones was enumerated. Proven by three consecutive real actionlint runs at three different SHAs. |
| #57 | P3-L6 phone call records and counters | `bfc090f` | One OTLP log per call row plus a `direction=placed\|received\|missed` counter, reusing the CDR checkpoint mechanism. Empty call logs produce zero records and no error **without a skip**. |
| #58 | P3-L7 phone network info | `bfc090f` | `polyphone.network.info` as gauge 1 with `dhcp_enabled`, `dhcp_server`, `default_gateway`, `subnet_mask`, `boot_server_option`. Missing payload fields produce **empty labels, not a missing series**, and the phone host IP is deliberately not a label. |
| #55 | P3-L8 config_params disposition + guard | `33c448b` | Branch A confirmed: the feature shipped in v0.1.0 and was **not** rebuilt. Added the missing limit-50 validation and a non-default two-item collector proof. |
| #51 | P3-L9 default from_lens_policy true | `ad36b32` | **Reopened after a completion audit.** The first closure relaxed `Config.Validate` while startup still constructed and queried Lens — a validation-only false pass. The corrective commit makes the static phone-only path real: Lens collectors and streaming are absent when both Lens credentials are omitted, static target keys supply the mandatory 12-hex-digit certificate MAC identity, and the configured-password fallback stays live. |
| #54 | P4-L11 catalog, docs, dashboard | `93b9570` | Single late integration pass for every wave-3 signal: catalog, docs, generated dashboard, configuration docs. |
| #61 | fix(config): document wave-3 intervals | `af6fd5d` | Repaired the exact-head CI failure the L0 interval keys introduced — a config-surface change without regeneration is a red generated-doc job. |
| #49 | P4-L10 guarded GHCR cleanup | `a660762` + `7642e48` | First dry run **failed** because the Octokit pagination route was called with the wrong signature. Second succeeded with deletion disabled. See `plo-0001` — this issue's resume boundary is the task. |
| #60 | P4-L12 ten-item integrated gate | open at migration | Items 1-8 PASS at final head, 9-10 PARTIAL. Became `plo-0001`. |

**Wave-3 final state**, at `ad36b32`: local and `origin/main` identical, fresh `make check` green,
every exact-head workflow successful, the deployment host updated unattended with restart count 0,
and live queries returning two connected devices, two phone API states, two `ok` and zero
`auth_failed` at the exact version, two complete network-info devices, one exact build-info series,
and nine real call records across both devices in a sanitized Loki projection.

## Post-wave-3 fixes

| # | Title | Closed by | Substance |
|---|---|---|---|
| #65 | fix(phone): call-log timestamps in the phone timezone | `8dd886d` + `6539c7b` | Grafana rejected recent call records as `timestamp too new`. Phone `callLogs` rows are offset-free local wall-clock and were parsed as UTC, shifting each instant an hour forward during BST. Fixed by reading each phone's `tcpIpApp.sntp.olsonTimezoneID` through the existing read-only `config/get` and parsing in that location, failing **closed** on a missing or invalid zone rather than guessing. **Reopened after live rollout** for the second-order defect: both persisted checkpoints were still on the old basis and ~55 minutes ahead, so corrected records would have been suppressed until the stale watermark was overtaken. `6539c7b` adds the one-time versioned migration, its regression test observed suppressing the valid call before the fix. Verified live: both checkpoints migrated to version 1 at the correct offset, zero `timestamp too new` entries after restart. |
| #67 | Grafana v2: tabbed dashboard and GitSync publication | `666a77a`, `d7f7208`, `823443c`, `ca10d89` | Replaced the legacy five-panel classic dashboard with a generated `dashboard.grafana.app/v2` resource at stable UID `polylens2otel`: seven tabs (Overview, Lens fleet, Phone REST, Calls and logs, Self-o11y, Traces, Profiles), 57 panels, eight variables, a repeated per-device row. **The stronger normalization test exposed five incorrect self-o11y backend names in the catalog** — every offline gate had agreed with itself. **Reopened** because the completing commit auto-closed the tracker before the visual acceptance loop finished: snapshots showed empty collector trace-search, TraceQL-metrics and CDR activity panels while the equivalent direct Tempo and Loki queries returned data. Two corrective commits fixed TraceQL serialization and the Loki CDR rate window; a third added an explicit healthy-uptime threshold because Grafana's implicit 80-unit threshold painted healthy uptime seconds red. Published to `rknightion/gc-gitsync-m7kni`; live readback confirmed v2, UID `polylens2otel`, folder `polylens2otel-dashboards`, generation 4. |

## Not imported

**#20, the Renovate Dependency Dashboard**, is bot-owned, regenerated on every Renovate run, and was
deliberately left on GitHub — it is not project work and it is not a task.
