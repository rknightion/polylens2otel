# Troubleshooting

## `api_disabled`

TLS succeeded and the certificate matched, but an unauthenticated management request returned 404. Check `apps.restapi.enabled` in the phone configuration. This is not a routing failure.

## `auth_failed`

The phone was reached and identified, but HTTP Digest authentication returned 401. Check the `Polycom` username and the environment-supplied password.

## `unreachable`

TCP/TLS failed, or the certificate CN did not match the Lens MAC. Check the static per-device override before trusting Lens `internalIp`; Lens inventory addresses can be stale.

## A line exists but is unregistered

A single line whose SIP address and label look like the phone model is the factory placeholder. The provisioning policy has not delivered the real registration. `polyphone.config_param_source{param="reg.1.address",source="default"}` distinguishes this case.

## CDR logs don't appear

Use `{service_name="polylens2otel"} | event_name="polylens.cdr"`. CDR metadata is not a Loki stream label. Empty CDR windows are normal, and late records can take a few minutes to become queryable.

## No WebSocket messages

`deviceStream` is edge-triggered. It sends no snapshot and no keepalive data. A quiet stream is normal; polling remains the source of truth.
