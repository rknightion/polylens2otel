---
id: doc-0002
title: Wave operating model
type: guide
created_date: '2026-08-14 16:56'
updated_date: '2026-08-14 16:57'
---
This document carries **only what is true of polylens2otel**. The campaign model itself — run
contract and run modes, the routing contract, authority and the thread pool, child lane briefs,
external-contract freezing, the blocker contract, the goal-file template, the run-end protocol and
the pre-flight checklist — is the *Agent fan-out protocol (canonical)* doc, and that doc wins on any
specific. Nothing here restates it. If a section below could be pasted into another repo unchanged,
it is in the wrong document.

That protocol is harness-neutral and names lanes by **role**; its Appendix A (Codex) or Appendix B
(Claude Code) resolves a role into a concrete route. Waves 1-3 on this repo were planned by Claude
and executed by Codex, so **name the harness in the run contract and resolve every lane's route from
that harness's profile** — a lane brief carrying a role name alone is not routed.

Every rule here exists because something failed. The failure is kept with the rule; a rule without
its reason gets argued away by the next session.

---

## 1. Rules this project added

### Read-only against Lens and the handsets is a product gate, not a style preference

`internal/lensclient` rejects any operation whose document contains `mutation` **before network
I/O**, and a test proves the rejection. The phone client exposes GETs plus `config/get`, which is a
POST that is a read.

The reason is two live incidents, not caution: Lens's `setDeviceConfiguration` has returned success
while silently doing nothing, and a policy write took **both** handsets off the network for hours on
2026-08-11. An exporter has no business in that blast radius. A lane that finds itself wanting a
write has misread its brief.

### `testdata/` is append-only. Never overwrite, never regenerate

The seven degraded-fleet fixtures record a broken state that was repaired externally mid-wave-1 and
**cannot be recreated**. They are the only coverage for the `api_disabled` and unregistered-line
paths. Healthy fixtures were added *beside* them, never over them.

The enforcement that has actually worked: SHA-256 every fixture present at run start, and check the
same hashes at run end. Wave 2 did this for seven files, wave 3 for twenty-one, and both came back
byte-identical. Do it again.

### Sanitize before writing a fixture, not after

No credential, token, tenant ID, collection ID, policy ID, MAC address, external IP or internal
hostname reaches the repository, the tracker or a fixture. Device names (`deskie`, `extra`) **are**
present in tracked fixtures and tests deliberately — they are what makes per-instance assertions
readable — so the line this repo draws is *identifiers and addresses out, device names in*. Do not
widen it in either direction without saying so.

Wave 1 shipped five live private-IP literals into `internal/phonetarget` tests and had to replace
them with RFC 5737 documentation addresses in a follow-up commit. `./scripts/check-secret-hygiene.sh`
is the gate; an early version of it was itself wrong, falsely classifying Go field names and its own
YAML canary as credentials, so a *passing* hygiene script is not by itself proof the script is right.

### Work that touches a live system stays on the root agent

Lanes do local edits, tests and inventory sweeps. **SSH to the deployment host, deploys, GitHub
actions, Grafana Cloud queries and pushes, and every Lens or handset probe stay with the root
agent.** This is not only a blast-radius rule: a dispatched lane inherits the parent's permission
mode and cannot clear a soft block, because clearing one requires a message from the user and a
lane's transcript contains none. A blocked lane must be run by the root agent, never re-dispatched.

### A lane that hits a decision its brief does not cover stops and returns the question

It does not invent an answer. Wave 2's P2-L8 did exactly this correctly: it found the runtime
constructed `phonetarget.Resolver` with a nil policy source, recorded that the fix needed a
`cmd/**` edit outside its ownership, and handed it to the root integration owner instead of taking
the file. One round trip is cheaper than the rewrite. A boundary with no escape hatch is a stop
condition wearing a safety label.

### Commits go straight to `main`, staged by explicit pathspec

No branches, no PRs for this repo's own work; release-please owns the only PR. `git add -A` and
`git commit -a` are forbidden — the working tree has carried an untracked `.DS_Store` through three
waves and it must stay untracked. Conventional Commits (`type(scope): subject`) is parsed by
release-please and Renovate, not a style preference.

### `1.0.0` is forbidden until BOTH conditions are met

A second Lens tenant, **and** a non-empty two-day CDR window observed end to end. As of the wave-3
L0 re-check the CDR condition is **met** (nine rows in the two-day first page) and the tenant
condition is **not** (`tenant_count` is 1). Both are required, so `1.0.0` remains forbidden however
complete the feature set looks. Re-check both at the start of a wave; do not infer either from the
other having passed.

### The no-cardinality-limiter licence has an observable expiry, not a predicted one

There is deliberately no central cardinality limiter — the fleet is two devices and roughly thirty
series, so one would be dead code. The licence ends when `polylens_device_connected` reports **50 or
more series**. Check that metric at the start of each wave. Do not restate the trigger as "when we
onboard a bigger tenant", which is a prediction wearing a trigger's clothes.

---

## 2. Recurring defects in this codebase

Each of these has shipped at least once. They are things to check for, not things to hope about.

### Fixture-green is not live-schema-green

Wave 1 passed every golden-fixture test in `internal/lensclient` and then failed at startup against
the real gateway on **three** GraphQL variable-type mismatches the fixtures could not express:
`$deviceID` and `$tenantID` were declared `ID!` where the gateway wants `String!`, and firmware
`$pid` was declared `String!` where it wants `ID!`. A fixture records a *response*; it says nothing
about whether the request would have been accepted.

The mitigation exists and must stay honest: the Lens schema canary workflow asserts the three live
types and is proven to **fail** for all three historical wrong types as negative controls. A canary
that only passes proves nothing.

### A state-enum gauge that never zeroes its superseded members

`polyphone_api_state` emitted only the current state and never wrote the others, so every device
read as both `ok` **and** `unreachable` at once for the whole staleness window, and an alert on
`state="unreachable"` would have fired permanently after one bad cycle and never cleared. The
contract is: emit the **full enum every cycle**, `1` for the current member and `0` for every other.
Audit any new labelled-state gauge for the same shape.

### A mutable attribute keying a metric series

`net.host.ip` was a resource attribute on the device metrics. When one handset's resolved address
moved from the wrong address Lens reports to its real one, the old series did not end — it froze at
`1`, so `count(polylens_device_connected)` returned **3 for a two-device fleet** and the exporter
was asserting a device was connected at an address where nothing existed. `device.id` is the
identity. An attribute that can change while the device stays the same must not key a series;
`device.id`, `device.mac`, `device.model`, `site.name` and `tenant.id` are stable, an address is
not, and a firmware version is not — which is exactly why `firmware_info` is its own gauge.

### A validation-only fix that reads as a runtime fix

The "deployment with no Lens credentials must start" requirement was closed once by relaxing
`Config.Validate` while `cmd/` startup still unconditionally constructed and queried Lens. It passed
its tests and was **wrong**; the issue was reopened and re-closed by a commit that made the static
phone-only path real. When acceptance says *behaviour*, a config-layer change is not evidence — run
the path.

### A time-basis fix that does not migrate its persisted checkpoints

Phone `callLogs` rows carry offset-free local wall-clock times. Parsing them as UTC shifted every
instant an hour forward during BST and Grafana rejected them as `timestamp too new`. Correcting the
parse to the phone's own `tcpIpApp.sntp.olsonTimezoneID` was necessary and **not sufficient**: the
two live checkpoints were still written on the old basis and about 55 minutes ahead, so corrected
records would have been silently suppressed until the stale watermark was overtaken. Any change to
how a timestamp is derived needs a one-time, tested checkpoint migration in the same wave.

### Local gate green is a different question from CI green

Two distinct instances. Workflow contracts differed between the private and public repository
states, so jobs that passed local `actionlint` and `zizmor` failed live on missing `contents:read` /
`actions:read` and unavailable SARIF upload. And adding two collector interval keys passed local
build while failing the exact-head generated-doc job, because `config.example.yaml` and
`docs/env-vars.md` are generated and drift-checked — a config-surface change without `make regen` in
the same commit is a red CI.

### A green coverage gate over an empty dashboard panel

The Grafana coverage gate counts catalogued metric names; it cannot tell you whether those names
exist in the backend. Five self-observability names in `spec/signal-catalog.json` were wrong against
the normalized names actually present, and every offline gate agreed with itself. Only a rendered
snapshot readback caught it. Remember the OTLP→Prometheus normalization is real — gauges gain
`_ratio`/`_seconds`/`_percent`, counters gain `_total` — and dashboards and alerts use the
**normalized** names.

Same class, one layer up: a dashboard that validates and publishes can still render "No data"
because of query-model or variable-interpolation drift. The dashboard is not done until every tab
has been snapshotted and looked at.

### A false pass by absence

The GHCR cleanup dry run reported `0 untagged manifests older than 30 days would be deleted` and
succeeded. That is **not** proof the candidate filter works — it is proof there were no candidates.
The lane was correctly left partial rather than counted as a pass. When a check can only be
satisfied by a positive result, an empty result is a `Parked`, not a `Done`.

### The commit-footer trap, in both directions

One commit carried literal `\n` characters before an otherwise correct `Closes #50` footer, so
GitHub never auto-closed it. Another auto-closed its tracker *before* the visual acceptance loop had
finished, and had to be reopened. Under Backlog.md neither mechanism applies — a task's status is
set by an explicit `backlog task edit`, never inferred from a commit — which removes both failure
modes and makes finalization the deliberate act described in §4.

---

## 3. Lane conventions

### One new signal touches six files, and four of them are shared

Measured by tracing every file `polyphone.uptime_seconds` reaches:

```text
internal/semconv/semconv.go                  ← every signal name, one file
internal/collectors/<domain>/collectors.go   ← all collectors of that domain, one file
internal/collectors/<domain>/register.go     ← the domain registration call list
spec/signal-catalog.json                     ← hand-maintained, read by grafana/common.py
docs/signals.md                              ← one document
dashboards/*.json                            ← generated by `make dashboard`
```

Two concurrent signal lanes collide on four of these. Either an L0 pass splits the shared files
before fan-out — creating the empty collector seam and freezing the semconv names and registry call
list up front, which is what let two wave-3 signal lanes run at once — or the signal lanes are
serialized. There is no third option, and discovering the collision mid-wave costs a rewrite.

### Single-owner files — never two lanes, never concurrently

- `internal/semconv/**` — every signal and attribute name. Frozen in L0, then read-only for lanes.
- `cmd/polylens2otel/collectors_import.go` — the frozen domain call list. Wiring pass only.
- `internal/config/**` and `config.example.yaml` — `docs/env-vars.md` is generated from them, so two
  lanes editing either produce a conflicting regeneration.
- `spec/signal-catalog.json` — a late single-owner integration pass, after the signal lanes land.
- `grafana/build_dashboard.py`, `grafana/build_rules.py` — the generators. Serialize.

### Generated artifacts are never hand-edited

`docs/env-vars.md` and `config.example.yaml` come from `make regen`; `dashboards/**` and `alerts/**`
from `make dashboard` / `make rules`. Each is drift-gated in CI. A lane that changes an input
regenerates in the **same** commit.

### Exclusive resources — one at a time, root agent only

- **The live Lens tenant and both handsets.** Read-only, and a certificate CN must match the Lens
  MAC before any credential is sent — that check has no off switch. Discovery never scans: it
  contacts only an address Lens named for a device or one an operator configured statically. Lens's
  reported `internalIp` is wrong for one of the two devices in the only fleet that exists, which is
  why the static override map is not optional.
- **The deployment host.** Root agent only, named only in ignored local config. It follows the
  `:main` tag rather than a release digest, so **a broken `main` reaches the live service
  automatically** — a red gate here is a live-service problem, not only a CI problem.
- **The m7kni Grafana stack.** Alert rules go through `gcx`; **dashboards go through the GitSync
  repository, not the API.** Grafana writes UI saves back into GitSync, so an API push is an
  out-of-band edit leaving both sides disagreeing with no way to tell which is right. Retire a
  dashboard by deleting its file.

---

## 4. Run-end against this tracker

The tracker *is* the report. There is no run-end file.

- Landed work: `backlog task edit <id> --check-ac N -s Done` **in one call**, with the commit SHA in
  the final summary. Splitting the criteria check from the status change lets an interrupted run
  leave finished work looking unfinished.
- Attempted and blocked: `-s Parked` with a concrete resume boundary — what was tried, what the next
  action is, what would unblock it. This repo has a live example worth copying: the GHCR cleanup lane
  parked with an exact eligibility timestamp and the precise sequence to run at it, which is why it
  survives the loss of its GitHub issue.
- Untouched work needs no action; it is still `To Do` and self-evidently so.
- Discovered work: a new task labelled `needs-triage`. Never a note in a summary nobody queries.
- Notes and plans are appended (`--append-notes`, `--append-plan`), never set. The bare flags
  silently replace the whole section and destroy another lane's writes; `.claude/hooks/backlog-guard.py`
  denies them.

The run's closing terminal message carries only what no single task can: what this run learned as a
whole. Nothing durable may live only there.

Before any task goes to `Done`, the `definition_of_done` gate in `backlog/config.yml` must have been
run and its output **seen**. For anything touching live telemetry that means one thing more: an
offline gate cannot prove a signal reaches Grafana Cloud. Query the backend.
