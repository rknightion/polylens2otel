# polylens2otel development register

The durable implementation tracker is GitHub issue #1. This file contains current engineering truth only.

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
