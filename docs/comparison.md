# Comparison and omissions

## Call quality

There is no supported call-quality telemetry in this stack. Poly Lens and the phone management API do not provide MOS, jitter, packet loss, latency, codec or per-call duration. `polylens2otel` does not invent placeholder metrics for them.

## Syslog

Phone syslog belongs in an existing Alloy syslog pipeline. This exporter does not open a syslog receiver or duplicate those records.

## Webhooks and event history

The Lens tenant has no generic webhook, event stream or event-history query. The device WebSocket carries connection changes only and supplements polling.

## Scaling

Series count grows with device count, not call volume. Metrics hold per-device aggregates; CDR records are logs. A `cardinality.max_devices` guard defaults to 500 and refuses to collect above that limit while emitting `polylens2otel.api.unexpected`.

The temporary no-limiter licence ends when `polylens_device_connected` reaches 50 series. Check that observed count at the start of the next release wave.
