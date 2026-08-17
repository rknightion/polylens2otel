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
| `polyphone.api_state` | `polyphone_api_state` | Full current-state enum: `state` is one of `ok`, `api_disabled`, `auth_failed` or `unreachable`; the current state is `1` and every noncurrent state is `0` |
| `polyphone.network.info` | `polyphone_network_info` | Current network configuration; dimensions are DHCP state, DHCP server, gateway, subnet mask, and boot-server option (never the phone address) |
| `polyphone.calls` | `polyphone_calls_total` | New call-log records by `direction` (`placed`, `received`, or `missed`) |

## Metric series identity

The shared device portion of metric-series identity uses only stable attributes:
`tenant.id`, `device.id`, `device.name`, `device.mac`, `device.model`, and
`site.name` when available. Each metric can additionally have its documented
dimensions, such as `state` for `polyphone.api_state`. `net.host.ip` is
deliberately excluded: a device address can change, and it must not create a
new metric series. Consumers should treat this as the public metric identity
contract and must not group, alert, or join device metric series by IP address.

## Self-observability

Self-observability uses the `polylens2otel.*` namespace. The generated dashboard accounts for every metric in `spec/signal-catalog.json`.

## Grafana dashboard

`make dashboard` generates a Grafana dashboard API v2 resource at
`dashboards/polylens2otel.json` and its dedicated folder at
`dashboards/_folder.json`. The single dynamic dashboard uses tabs for Overview,
Lens fleet, Phone REST, Calls and logs, Self-o11y, Traces, and Profiles. Tenant,
site, device, and collector variables scope the fleet; the Overview also repeats
a compact health row for each selected device.

Every catalog metric, log event, trace family, and configured Pyroscope profile
type must appear in at least one panel. `make grafana-check` enforces that
coverage and validates backend-normalized Prometheus names so a plausible but
nonexistent `_seconds` or `_ratio` suffix cannot silently pass the dashboard
gate.

On `main`, `.github/workflows/grafana-sync.yml` publishes both resources to the
`polylens2otel/` directory of `m7kni/gc-gitsync-m7kni`. That repository is
the Grafana GitSync source of truth; do not push this dashboard directly through
the Grafana API because an out-of-band save would diverge from GitSync.

At process start the exporter emits the `polylens2otel.startup` event with the
`version`, `commit`, and `build.date` attributes. `polylens2otel.build_info` is
a metric with the same build metadata.

## Call-record logs

Each new Lens CDR row is an OTLP log with `event.name=polylens.cdr`. Each new phone call-log row is an OTLP log with `event.name=polyphone.call_record`. Call-record attributes are Loki structured metadata. Only `service_name` is a stream label, so query the stream first and filter the event afterward:

```logql
{service_name="polylens2otel"} | event_name="polylens.cdr"
```

For phone call records, use:

```logql
{service_name="polylens2otel"} | event_name="polyphone.call_record"
```

`event_name` and every call-record field, including direction, remote party, line, duration, disposition, and start time, are structured metadata rather than Loki stream labels.

Phone call-log timestamps are local wall-clock values without an offset. The
collector reads each phone's `tcpIpApp.sntp.olsonTimezoneID` setting and applies
that IANA timezone, including its daylight-saving rules, before emitting the
event timestamp. A missing, unsupported, or invalid timezone fails the
collector rather than emitting a guessed timestamp.

Rows without parseable event timestamps are dropped. Duration is computed from `startTime` and `endTime`; deduplication uses `deviceId + startTime` because Lens returns a null CDR ID.
