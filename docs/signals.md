# Signals

## Lens metrics

| OTLP name | Prometheus name | Meaning |
|---|---|---|
| `polylens.device.connected` | `polylens_device_connected` | Lens connection state |
| `polylens.device.last_detected_seconds` | `polylens_device_last_detected_seconds` | Age of the last detection |
| `polylens.device.last_config_request_seconds` | `polylens_device_last_config_request_seconds` | Age of the last configuration request |
| `polylens.device.firmware_info` | `polylens_device_firmware_info` | Current version and build |
| `polylens.device.firmware_current` | `polylens_device_firmware_current` | Installed version matches the latest available version |
| `polylens.device.active_calls` | `polylens_device_active_calls` | Active calls grouped by state |
| `polylens.stream.connected` | `polylens_stream_connected` | WebSocket connection state |

## Phone metrics

| OTLP name | Prometheus name | Meaning |
|---|---|---|
| `polyphone.uptime_seconds` | `polyphone_uptime_seconds` | Phone uptime |
| `polyphone.network.packets` | `polyphone_network_packets_total` | Monotonic RX/TX packet counts |
| `polyphone.line.registered` | `polyphone_line_registered` | Registration state per logical line |
| `polyphone.lines_total` | `polyphone_lines_total` | Logical line count after duplicate line-key records are collapsed |
| `polyphone.config_param_source` | `polyphone_config_param_source` | Source of an allowlisted configuration parameter |
| `polyphone.api_state` | `polyphone_api_state` | One of `ok`, `api_disabled`, `auth_failed` or `unreachable` |

Self-observability uses the `polylens2otel.*` namespace. The generated dashboard accounts for every metric in `spec/signal-catalog.json`.

## CDR logs

Each new CDR row is an OTLP log with `event.name=polylens.cdr`. CDR attributes are Loki structured metadata. Only `service_name` is a stream label, so query the stream first and filter the event afterward:

```logql
{service_name="polylens2otel"} | event_name="polylens.cdr"
```

`{event_name="polylens.cdr"}` returns no rows because `event_name` is not a stream label.

Rows without parseable event timestamps are dropped. Duration is computed from `startTime` and `endTime`; deduplication uses `deviceId + startTime` because Lens returns a null CDR ID.
