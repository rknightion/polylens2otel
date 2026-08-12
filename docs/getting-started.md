# Getting started

## Requirements

- Go 1.26.5 or a container runtime
- Poly Lens OAuth client credentials
- A Poly Edge phone account with read access to the management API
- An OTLP endpoint that accepts metrics, logs and traces

## Build from source

```sh
make build
cp config.example.yaml config.yaml
```

Edit only non-secret settings in `config.yaml`. Supply credentials in the process environment:

```sh
export PL2O_LENS__CLIENT_ID='<client-id>'
export PL2O_LENS__CLIENT_SECRET='<client-secret>'
export PL2O_PHONE__AUTH__PASSWORD='<phone-password>'
export PL2O_OTLP__ENDPOINT='https://otlp.example.com/otlp'
export PL2O_OTLP__GRAFANA_CLOUD__INSTANCE_ID='<instance-id>'
export PL2O_OTLP__GRAFANA_CLOUD__TOKEN='<token>'
bin/polylens2otel -config config.yaml
```

Keep the state directory writable by the process. Startup fails if it cannot persist checkpoints.

## Container

The Compose reference expects a sibling `.env` file and a host configuration at `/opt/polylens2otel/config.yaml`. Keep `.env` mode `0600`; don't put credentials in the YAML file.

```sh
docker compose -f deploy/docker-compose.yaml config
docker compose -f deploy/docker-compose.yaml up -d
```

## Helm

Create a Kubernetes Secret containing the required `PL2O_` keys, then reference its name through `existingSecret`. Do not put credential values in `--set` arguments.

```sh
helm lint charts/polylens2otel
helm template polylens2otel charts/polylens2otel --set existingSecret=polylens2otel-secrets
```
