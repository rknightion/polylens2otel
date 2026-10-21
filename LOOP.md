# Loop: polylens2otel
tier: guarded
gate: just check
ci-required: ci-success
release-on-push: yes
deploy-on-push: yes
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
- Read-only against Lens and the handsets is a product gate: `internal/lensclient` rejects any
  `mutation` before network I/O and a test proves it. A lane wanting a write has misread its brief.
- `testdata/` is append-only: never overwrite or regenerate a fixture (the degraded-fleet fixtures
  cannot be recreated). Hash every fixture at run start and compare at run end.
- Sanitize before writing a fixture: identifiers and addresses out, device names in. A passing
  `./scripts/check-secret-hygiene.sh` is not proof the script is right.
- Fixture-green is not live-schema-green: the Lens schema canary asserts the live GraphQL variable
  types and is proven to fail on the historical wrong ones. A canary that only passes proves nothing.
- Emit the full enum every cycle for a labelled-state gauge (`1` current, `0` the rest). A mutable
  attribute (an address, a firmware version) never keys a series; `device.id` is the identity.
- A config-layer fix is not evidence for a behaviour requirement: run the path. A change to how a
  timestamp is derived needs a tested one-time checkpoint migration in the same change.
- A config-surface change needs `just gen` in the same commit: `config.example.yaml` and
  `docs/env-vars.md` are generated and drift-checked.
- Dashboards and alerts use the normalized Prometheus names (`_ratio`, `_seconds`, `_total`). The
  coverage gate counts catalogued names, not backend series, so snapshot every dashboard tab.
- An empty dry-run result is no candidates, not a pass: park it.
- `1.0.0` stays forbidden until a second Lens tenant exists and a non-empty two-day CDR window has
  been observed. Re-check both at the start of a run. There is deliberately no cardinality limiter
  until `polylens_device_connected` reports 50 or more series.
- `internal/semconv` owns every signal name and `internal/telemetry` is the only OTLP package;
  `cmd/polylens2otel/collectors_import.go` is the frozen domain call list.

## Mutexes

- Single owner, never two lanes: `internal/semconv/**` (frozen first, then read-only),
  `cmd/polylens2otel/collectors_import.go`, `internal/config/**` with `config.example.yaml`,
  `spec/signal-catalog.json` (late integration pass), `grafana/build_dashboard.py` and
  `grafana/build_rules.py`. One new signal touches six files, four of them shared, so signal lanes
  either run after a seam-splitting pass or serialize.
- The live Lens tenant and both handsets: root only, read-only; a certificate CN must match the Lens
  MAC before any credential is sent. The deployment host follows the `:main` tag, so a broken `main`
  reaches the live service.
- Alert rules go through `gcx`; dashboards go through the GitSync repository, never the API.
- One `grafana-sync` writes the stack at a time and is never cancelled mid-write.
