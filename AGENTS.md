# polylens2otel development register

The durable tracker is **Backlog.md**, in `backlog/`. This file contains current engineering truth only.

## Task tracking

Open work is a query, not a file: `backlog task list --plain`. Durable reference is `backlog doc list --plain`.

- Read the **Agent fan-out protocol (canonical)** doc before designing a wave, and the **Wave operating model** doc for this project's own rules, recurring defects and lane conventions. The protocol wins on any generic specific; the operating model wins on anything about this repo.
- Work before 2026-08-14 was tracked on GitHub Issues. Those issues were **deleted** and there is no JSON archive, so the **Closed GitHub issues (pre-Backlog history record)** doc is the record, not an index into one. Cite closed work as `#NNN`, new work as `plo-NNNN`.
- **Never use `--notes` or `--plan` bare** — they silently replace the whole section and destroy another session's writes at exit 0. Use `--append-notes` and `--append-plan`. A global `PreToolUse` hook in the agent config denies the bare forms.
- **Never hand-edit task, draft, doc, decision or milestone markdown.** Section boundaries are HTML-comment markers; breaking one silently drops the section at exit 0 and there is no repair command. `backlog/config.yml` is the one exception and is edited by hand, because list-valued keys cannot be set through `backlog config set`.
- **Finalize in one call** — `backlog task edit <id> --check-ac 1 --check-ac 2 -s Done` — so an interrupted run cannot leave finished work looking unfinished.
- `backlog/` is committed, so no credential, token, tenant/collection/policy ID, MAC address, private or external IP, or internal hostname goes in a task or doc. Device names are the deliberate exception: they already appear in tracked fixtures.

## Commands

- make build — compile the binary
- make test — race-enabled tests
- make check — repository green bar
- make regen — regenerate configuration documentation
- make dashboard / make rules — regenerate Grafana artifacts
- make grafana-check — check generated dashboards and alerts
- make docker — build the container

## Method

Write tests first for parsing, retry/backoff, state machines, dedupe, checkpointing and branching. Validate declarative files with their parsers and renderers. Never use a missing fixture as a test skip.

## Architecture seams

- internal/config owns the complete koanf configuration surface.
- internal/semconv owns every signal and attribute name.
- internal/telemetry is the only package that touches OTLP.
- internal/collector owns registration and scheduling.
- Each collector domain exposes Register(collector.Deps); adding a collector changes its file plus its domain register file.
- cmd/polylens2otel/collectors_import.go is the frozen domain call list.

## Configuration and secrets

Configuration precedence is defaults, YAML, then PL2O_ environment variables with double underscores for nesting. Secrets are environment-only. Never commit tokens, tenant IDs, internal hostnames or private addresses.

## Telemetry model

Lens signals use polylens.*, phone REST signals use polyphone.*, and self-observability uses polylens2otel.*. Every signal receives tenant.id at the Emitter boundary. CDRs are logs with structured metadata; only service_name is a Loki stream label.

## Non-negotiable traps

- Lens token requests are JSON, bearer headers must work with HTTP/2, 4xx bodies are evidence, and follow-up pageSize must not change.
- Lens mutations are rejected before network I/O.
- deviceStream is a named DevStream graphql-transport-ws subscription and remains an edge-triggered supplement to polling.
- Phone auth is Digest as Polycom. config/get is the only POST and is a read.
- A phone certificate CN must match the Lens MAC before credentials are sent.
- Static per-device targets override Lens internalIp; discovery never scans.
- 404 before auth means api_disabled; 401 means auth_failed.
- No call-quality, utilization, room, webhook or syslog subsystem exists here.

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
