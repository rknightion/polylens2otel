# polylens2otel

Collects Poly Lens cloud (GraphQL) and Poly desk-phone REST telemetry and exports it as OTLP metrics
and logs.

## Task interface

`just check` is the gate and must pass before you commit. CI runs it verbatim, then builds the
container image and the release artifacts in separate jobs; there is no `ci` recipe here. No recipe
is marked `[confirm]`. Run `just` with stdin from `/dev/null`.

`just install-hooks` points git at the tracked `.githooks/` pre-commit gate.

## Ownership seams

Cross a seam and two packages start disagreeing about the same name.

- `internal/config` owns the complete koanf configuration surface.
- `internal/semconv` owns every signal and attribute name. Nothing else declares one.
- `internal/telemetry` is the only package that touches OTLP.
- `internal/collector` owns registration and scheduling.
- Each collector domain exposes `Register(collector.Deps)`. Adding a collector changes its own file
  plus its domain register file, nothing else.
- `cmd/polylens2otel/collectors_import.go` is the frozen domain call list.

## Configuration and telemetry model

- Precedence is defaults, then YAML, then `PL2O_` environment variables with **double underscores**
  for nesting (`PL2O_OTLP__ENDPOINT`). Secrets are environment-only and never enter a YAML file.
- Lens signals are `polylens.*`, phone REST signals are `polyphone.*`, self-observability is
  `polylens2otel.*`.
- `tenant.id` is stamped at the Emitter boundary and nowhere else.
- CDRs are OTLP logs whose attributes arrive as Loki **structured metadata**, not stream labels. Only
  `service_name` is a stream label, so `{event_name="polylens.cdr"}` matches zero rows silently -
  always `{service_name="polylens2otel"} | event_name=...`.

## Traps

- The static runtime uses distroless Debian 13 for the patched CA bundle. Debian 12's nonroot tag still carried `ca-certificates 20250419~deb12u1` on 2026-10-21; require the patched Debian 13 package `20250419+deb13u1` or newer when refreshing this base.

- The Lens **token** request body is JSON, and the bearer header is set lowercase (`authorization`);
  HTTP/2 field names are lowercase and `TestGraphQLUsesLowercaseBearerAuthorization` pins it. Do not
  switch to the canonical-cased form.
- A Lens **mutation is rejected before any network I/O** - the client regex-matches the keyword. This
  exporter is read-only against Lens by construction; do not add a bypass.
- 4xx bodies from Lens are evidence, not noise: read them before retrying.
- Paginate with `nextToken` and keep `pageSize` **identical** on every follow-up page.
- `deviceStream` is a named `DevStream` `graphql-transport-ws` subscription and is an edge-triggered
  *supplement* to polling, never a replacement. It is silent when nothing changes.
- Phone auth is HTTP Digest, presenting as Polycom. `config/get` is the only POST and it is a read.
- A phone's certificate CN must match the Lens MAC before credentials are sent
  (`internal/phoneclient/client.go`).
- A static per-device target overrides the Lens `internalIp`. Discovery never scans a network.
- Phone probe states: 404 before auth means `api_disabled`, 401 means `auth_failed`. They are not
  interchangeable.
- **No call-quality, utilization, room, webhook or syslog subsystem exists here** - deliberate, not a
  gap to fill. Handset SIP voice quality is a separate exporter.

## Tracking work

Work is tracked in this repository's GitHub issues; they are the only queue. Do not add a checklist
file beside them.

- A pull request body says how it relates to an issue, on a line of its own: `Closes #N` with the
  `Acceptance evidence` table from `.github/pull_request_template.md` (one row for each acceptance
  criterion of that issue), `Refs #N` when it only relates to one, or `Task: none, <reason>`.
- A pull request that changes `.github/workflows/` or `.github/actions/` carries no closing keyword:
  write `Task: none, <reason>`, with `Refs #N` beside it if you like.
- Contributors never need the project board or the tracker tooling; a maintainer handles both.
- Issues and pull requests are public: no credential, token, tenant, collection or policy ID, MAC
  address, private or external IP or internal hostname. Device names are the deliberate exception;
  they already appear in tracked fixtures.
- Older commits, docs and tests cite legacy board ids such as `PLO-0001`. `just issue <ID>` prints
  the issue each one became; `.github/tracker/id-map.json` is the map. The retired board's documents
  are in `.github/tracker/history/`. A bare `#N` below #157 in them is a pre-board GitHub issue that
  was deleted: the "Closed GitHub issues (pre-Backlog history record)" document there is the record
  itself, not an index into anything.
