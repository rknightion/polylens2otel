# Configuration

Precedence is defaults, YAML, then environment. Environment names start with `PL2O_`; double underscores represent nesting. For example, `otlp.endpoint` becomes `PL2O_OTLP__ENDPOINT`.

Credentials are environment-only. The loader rejects a YAML file containing any of these values:

- Lens client secret
- Phone administrator password
- Grafana Cloud token
- Pyroscope basic-auth password

The generated [environment reference](env-vars.md) describes every key.

## Multiple tenants

Set `lens.tenants` to a list of tenant IDs. Leave it empty to query the tenant list and discover them. Every emitted metric, log and span receives `tenant.id` at the telemetry boundary.

## Phone targets

Lens supplies each device's `internalIp`, but that value can be stale or wrong. `phone.targets` is a map from Lens device ID to a fixed host and takes precedence:

```yaml
phone:
  targets:
    '<device-id>': phone-a.example.net
```

The exporter does not scan. It contacts only the Lens-provided address or the operator's override. Before any password is sent, the phone certificate CN must match the Lens MAC address.

## Phone configuration parameters

`phone.config_params` is the read-only allowlist requested by the phone configuration collector. It accepts at most 50 parameters, which bounds both the phone configuration response and the resulting metric-series count. Keep the list focused on values that are useful to observe.

## Policy passwords

`phone.auth.from_lens_policy` defaults to `true`. The exporter reads `device.auth.localAdminPassword` from the already-selected winning Lens policy and falls back to the configured password on failure. The value is held in a redacting type and never becomes a log or telemetry attribute.

To restore environment-configured password-only authentication, set `phone.auth.from_lens_policy: false` in YAML or `PL2O_PHONE__AUTH__FROM_LENS_POLICY=false` in the environment. In that mode the exporter does not query Lens for a policy password.
