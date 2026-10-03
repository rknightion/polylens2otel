# Loop: polylens2otel
tier: guarded
gate: just check
ci-required: ci-success
release-on-push: yes
deploy-on-push: yes
receiver: https://loopwatch.m7kni.com
grafana-stack: robknight

Public repository: no credential, token, tenant, collection or policy ID, MAC, private or external
IP or internal host name in any tracked file, `backlog/` included. Run `just` with stdin from
`/dev/null`. There is no `ci` recipe and no confirm-gated recipe.

## Credentials

- Secrets are environment-only (`PL2O_` variables, double underscore for nesting) and never enter a
  YAML file.

## Traps

- A push to `main` touching dashboards, alerts or `grafana/**` runs `grafana-sync`, which commits
  dashboards into the GitSync repository. Never push dashboards through the API.
- The Lens schema canary runs on every push to `main` and on a schedule: a red canary can be
  upstream drift, not the change.
- The exporter is read-only against Lens: a mutation is rejected before any network I/O, so never
  add a bypass. The bearer header stays lowercase `authorization`.
- 4xx bodies from Lens are evidence: read them before retrying. Keep `pageSize` identical on every
  follow-up page.
- A phone's certificate CN must match the Lens MAC before credentials are sent. Phone probe 404
  before auth is `api_disabled`, 401 is `auth_failed`; they are not interchangeable.
- CDRs reach Loki as structured metadata, not stream labels; query
  `{service_name="polylens2otel"} | event_name=...`.
- `internal/semconv` owns every signal name and `internal/telemetry` is the only OTLP package;
  `cmd/polylens2otel/collectors_import.go` is the frozen domain call list.

## Mutexes

- One `grafana-sync` writes the stack at a time and is never cancelled mid-write.
