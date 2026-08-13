# polylens2otel

<!-- x-release-please-start-version -->
![Version](https://img.shields.io/static/v1?label=Version&message=0.3.0&color=informational&style=flat-square)
![AppVersion](https://img.shields.io/static/v1?label=AppVersion&message=0.3.0&color=informational&style=flat-square)
<!-- x-release-please-end -->
![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square)

Export Poly Lens and supported Poly phone telemetry as OpenTelemetry metrics and logs over OTLP.

**Homepage:** <https://github.com/rknightion/polylens2otel>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| rknightion |  |  |

## Source Code

* <https://github.com/rknightion/polylens2otel>

## Install

Create a Kubernetes Secret with the required `PL2O_*` credentials, then refer
to it without placing secrets in Helm values:

```sh
kubectl create secret generic polylens2otel-credentials \
  --from-literal=PL2O_LENS__CLIENT_ID='<client-id>' \
  --from-literal=PL2O_LENS__CLIENT_SECRET='<client-secret>' \
  --from-literal=PL2O_PHONE__AUTH__PASSWORD='<phone-password>' \
  --from-literal=PL2O_OTLP__GRAFANA_CLOUD__INSTANCE_ID='<instance-id>' \
  --from-literal=PL2O_OTLP__GRAFANA_CLOUD__TOKEN='<otlp-token>'

helm install polylens oci://ghcr.io/rknightion/charts/polylens2otel \
  --set existingSecret=polylens2otel-credentials
```

The chart mounts non-secret configuration at
`/etc/polylens2otel/config.yaml` and persists file-backed state under
`/var/lib/polylens2otel`. The process is OTLP-push-only and exposes no HTTP
health endpoint, so the Deployment intentionally has no fabricated probes.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| config.cardinality.max_devices | int | `500` |  |
| config.collectors.lens_active_calls | string | `"1m"` |  |
| config.collectors.lens_cdr | string | `"1h"` |  |
| config.collectors.lens_devices | string | `"1m"` |  |
| config.collectors.lens_firmware | string | `"24h"` |  |
| config.collectors.phone_config | string | `"5m"` |  |
| config.collectors.phone_lines | string | `"1m"` |  |
| config.collectors.phone_status | string | `"1m"` |  |
| config.collectors.selfobs_internal | string | `"1m"` |  |
| config.lens.graphql_url | string | `"https://api.silica-prod01.io.lens.poly.com/graphql"` |  |
| config.lens.page_size | int | `10` |  |
| config.lens.request_timeout | string | `"30s"` |  |
| config.lens.retry.max_attempts | int | `4` |  |
| config.lens.retry.max_backoff | string | `"30s"` |  |
| config.lens.retry.min_backoff | string | `"1s"` |  |
| config.lens.stream.ack_timeout | string | `"10s"` |  |
| config.lens.stream.enabled | bool | `true` |  |
| config.lens.stream.max_backoff | string | `"1m"` |  |
| config.lens.stream.min_backoff | string | `"1s"` |  |
| config.lens.tenants | list | `[]` |  |
| config.lens.token_url | string | `"https://login.lens.poly.com/oauth/token"` |  |
| config.lens.websocket_url | string | `"wss://api.silica-prod01.io.lens.poly.com/graphql"` |  |
| config.log.format | string | `"json"` |  |
| config.log.level | string | `"info"` |  |
| config.otlp.endpoint | string | `""` |  |
| config.otlp.export_interval | string | `"15s"` |  |
| config.otlp.grafana_cloud.instance_id | string | `""` |  |
| config.otlp.insecure | bool | `false` |  |
| config.otlp.protocol | string | `"http"` |  |
| config.phone.auth.from_lens_policy | bool | `false` |  |
| config.phone.auth.username | string | `"Polycom"` |  |
| config.phone.config_params[0] | string | `"reg.1.address"` |  |
| config.phone.config_params[1] | string | `"reg.2.address"` |  |
| config.phone.config_params[2] | string | `"reg.1.label"` |  |
| config.phone.config_params[3] | string | `"device.syslog.serverName"` |  |
| config.phone.config_params[4] | string | `"tcpIpApp.sntp.address"` |  |
| config.phone.config_params[5] | string | `"softkey.1.enable"` |  |
| config.phone.enabled | bool | `true` |  |
| config.phone.request_timeout | string | `"15s"` |  |
| config.phone.targets | object | `{}` |  |
| config.phone.tls.ca_file | string | `""` |  |
| config.phone.tls.verify_chain | bool | `false` |  |
| config.profiling.pyroscope.application | string | `"polylens2otel"` |  |
| config.profiling.pyroscope.basic_auth_user | string | `""` |  |
| config.profiling.pyroscope.endpoint | string | `""` |  |
| config.state.dir | string | `"/var/lib/polylens2otel"` |  |
| existingSecret | string | `""` |  |
| extraEnv | list | `[]` |  |
| extraVolumeMounts | list | `[]` |  |
| extraVolumes | list | `[]` |  |
| fullnameOverride | string | `""` |  |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.repository | string | `"ghcr.io/rknightion/polylens2otel"` |  |
| image.tag | string | `""` |  |
| imagePullSecrets | list | `[]` |  |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` |  |
| persistence.accessMode | string | `"ReadWriteOnce"` |  |
| persistence.enabled | bool | `true` |  |
| persistence.existingClaim | string | `""` |  |
| persistence.size | string | `"1Gi"` |  |
| persistence.storageClass | string | `""` |  |
| podAnnotations | object | `{}` |  |
| podLabels | object | `{}` |  |
| podSecurityContext.fsGroup | int | `65532` |  |
| podSecurityContext.fsGroupChangePolicy | string | `"OnRootMismatch"` |  |
| podSecurityContext.runAsGroup | int | `65532` |  |
| podSecurityContext.runAsNonRoot | bool | `true` |  |
| podSecurityContext.runAsUser | int | `65532` |  |
| podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| replicaCount | int | `1` |  |
| resources.limits.cpu | string | `"500m"` |  |
| resources.limits.memory | string | `"256Mi"` |  |
| resources.requests.cpu | string | `"50m"` |  |
| resources.requests.memory | string | `"64Mi"` |  |
| securityContext.allowPrivilegeEscalation | bool | `false` |  |
| securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| securityContext.readOnlyRootFilesystem | bool | `true` |  |
| securityContext.runAsGroup | int | `65532` |  |
| securityContext.runAsUser | int | `65532` |  |
| serviceAccount.automountServiceAccountToken | bool | `false` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` |  |
| tolerations | list | `[]` |  |
