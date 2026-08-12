# polylens2otel

`polylens2otel` reads Poly Lens and Poly Edge phone state and sends OpenTelemetry metrics, logs and traces to an OTLP endpoint. It is a single Go process with no inbound listener and no Prometheus scrape endpoint.

The exporter is read-only. Lens GraphQL mutations are rejected before network I/O. Phone access is limited to documented GET endpoints plus the POST-only `config/get` reader.

## What it reports

- Lens connectivity, detection age, firmware and active-call counts
- Phone API state, uptime, packet counters, line registration and selected configuration sources
- CDR rows as OTLP logs
- Collector, HTTP client, authentication, stream and checkpoint health

Call-quality data is not available from the supported APIs. There are no MOS, jitter, loss, latency or codec metrics here.

## Build and check

```sh
make build
make check
```

`make build` embeds the resolved version, source commit, and UTC build date in
the binary. `make docker` passes the same three values into the image build;
without explicit overrides they are derived from the local Git checkout and the
current UTC time. Published release images carry the release version, the
release commit, and their UTC build date. The mutable `:main` image is an edge
build rather than a release image; use a release tag when deployment needs a
fixed version.

Run with a non-secret YAML file and credentials supplied through `PL2O_` environment variables:

```sh
bin/polylens2otel -config config.yaml
```

See [Getting started](docs/getting-started.md) for installation and [Environment variables](docs/env-vars.md) for the complete generated configuration reference.

## Deployment

The repository includes a production container, Helm chart and Compose reference. All three run as UID/GID 65532 and persist CDR checkpoints under `/var/lib/polylens2otel`.

```sh
make docker
helm lint charts/polylens2otel
docker compose -f deploy/docker-compose.yaml config
```

## Licence

AGPL-3.0. See [LICENSE](LICENSE).
