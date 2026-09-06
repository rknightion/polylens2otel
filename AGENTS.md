# polylens2otel

Collects Poly Lens cloud (GraphQL) and Poly desk-phone REST telemetry and exports it as OTLP metrics
and logs.

## Task interface

`just check` is the gate and must pass before you commit. CI runs it verbatim, then builds the
container image and the release artifacts in separate jobs; there is no `ci` recipe here. No recipe
is marked `[confirm]`. Run `just` with stdin from `/dev/null`.

`just install-hooks` points git at the tracked `.githooks/` pre-commit gate.

## Tracker

Tasks are `plo-NNNN` in `backlog/`. Read the **Agent fan-out protocol (canonical)** doc before
designing a wave, and the **Wave operating model** doc for this repo's own rules, recurring defects
and lane conventions. The operating model wins on anything about this repo.

`#NNN` refers to a pre-migration GitHub issue. Those issues were deleted and there is no JSON
archive: the **Closed GitHub issues (pre-Backlog history record)** doc is the record itself, not an
index into one.

Tracker traps:

- **Never `--notes` or `--plan` bare.** They *silently replace* the whole section and exit 0,
  destroying another session's writes. Use `--append-notes` and `--append-plan`. A `PreToolUse` hook
  denies the bare forms.
- Break an HTML-comment section marker by hand-editing task markdown and the section is silently
  dropped at exit 0. There is no repair command. `backlog/config.yml` is the one exception and is
  edited by hand, because list-valued keys cannot be set through `backlog config set`.
- **Finalize in one call** so an interrupted run cannot leave finished work looking unfinished:
  `backlog task edit <id> --check-ac 1 --check-ac 2 -s Done`.
- `backlog/` is committed and public: no credential, token, tenant/collection/policy ID, MAC address,
  private or external IP, or internal hostname in a task or doc. Device names are the deliberate
  exception - they already appear in tracked fixtures.

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
- Never use a missing fixture as a reason to skip a test.

<!-- BACKLOG.MD GUIDELINES START -->
<!-- backlog.md-instructions-version: 1.50.1 -->
<CRITICAL_INSTRUCTION>

## Backlog.md Workflow

This project uses Backlog.md for task and project management.

**For every user request in this project, run `backlog instructions overview` before answering or taking action.**

Use the overview to decide whether to search, read, create, or update Backlog tasks.

Before task lifecycle actions, read the matching detailed guide:
- `backlog instructions task-creation` before creating or splitting tasks
- `backlog instructions task-execution` before planning, changing status or assignee, adding a plan or implementation notes, or implementing task work
- `backlog instructions task-finalization` before checking acceptance criteria, writing final summaries, or moving tasks to terminal statuses

Use `backlog <command> --help` before running unfamiliar commands. Help shows options, fields, and examples.

Do not edit Backlog task, draft, document, decision, or milestone markdown files directly. Use the `backlog` CLI so metadata, relationships, and history stay consistent.

</CRITICAL_INSTRUCTION>
<!-- BACKLOG.MD GUIDELINES END -->
