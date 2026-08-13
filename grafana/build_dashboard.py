#!/usr/bin/env python3
"""Generate the comprehensive Grafana v2 dynamic dashboard."""

from __future__ import annotations

import argparse
import json

from common import ROOT, catalog, dump_json, write_or_check
import v2

OUT = ROOT / "dashboards" / "polylens2otel.json"
FOLDER_OUT = ROOT / "dashboards" / "_folder.json"
WAIVERS = ROOT / "grafana" / "waivers.json"

PROM = {"prometheus": "${ds_prometheus}"}
TENANT = 'tenant_id=~"$tenant"'
FLEET = TENANT + ',site_name=~"$site",device_name=~"$device"'
COLLECTOR = TENANT + ',collector_id=~"$collector"'

TS_CUSTOM = {
    "drawStyle": "line",
    "lineInterpolation": "linear",
    "lineWidth": 2,
    "fillOpacity": 15,
    "gradientMode": "none",
    "showPoints": "never",
    "spanNulls": True,
}

BOOL_MAPPING = [{
    "type": "value",
    "options": {
        "0": {"text": "No", "color": "red"},
        "1": {"text": "Yes", "color": "green"},
    },
}]


def _gauge(expr: str) -> str:
    """Collapse the brief stale-series overlap during versioned deployments."""
    return f"max without (service_version) ({expr})"


def _counter_rate(expr: str) -> str:
    """Combine counter rates across a process replacement without a reset spike."""
    return f"sum without (service_version) (rate({expr}[$__rate_interval]))"


def _current_total(expr: str) -> str:
    return f"sum({_gauge(expr)})"


def _ts(builder: v2.Builder, title: str, queries: list[dict], *,
        unit: str = "short", width: int = 12, height: int = 7,
        description: str = "") -> dict:
    return builder.panel(
        title, "timeseries", queries, width=width, height=height,
        description=description, unit=unit, custom=TS_CUSTOM,
        options=v2.timeseries_options(),
    )


def _stat(builder: v2.Builder, title: str, query: dict, *, unit: str = "short",
          width: int = 4, description: str = "", mappings: list | None = None,
          threshold_config: dict | None = None) -> dict:
    return builder.panel(
        title, "stat", [query], width=width, height=5, description=description,
        unit=unit, options=v2.stat_options(), mappings=mappings,
        threshold_config=threshold_config,
    )


def _table(builder: v2.Builder, title: str, queries: list[dict], *,
           width: int = 12, height: int = 8, description: str = "") -> dict:
    return builder.panel(
        title, "table", queries, width=width, height=height,
        description=description,
        options={"showHeader": True, "cellHeight": "sm"},
    )


def _logs(builder: v2.Builder, title: str, expr: str, *, description: str) -> dict:
    return builder.panel(
        title, "logs", [v2.loki(expr, max_lines=200)], width=24, height=10,
        description=description, options=v2.logs_options(),
        no_value="No matching call records in the selected time range.",
    )


def _variables() -> list[dict]:
    return [
        v2.datasource_variable("ds_prometheus", "Prometheus", "prometheus", "grafanacloud-prom"),
        v2.datasource_variable("ds_loki", "Loki", "loki", "grafanacloud-logs"),
        v2.datasource_variable("ds_tempo", "Tempo", "tempo", "grafanacloud-traces"),
        v2.datasource_variable("ds_profiles", "Pyroscope", "grafana-pyroscope-datasource", "grafanacloud-profiles"),
        v2.query_variable("tenant", "Tenant", "label_values(polylens_device_connected, tenant_id)"),
        v2.query_variable("site", "Site", 'label_values(polylens_device_connected{tenant_id=~"$tenant"}, site_name)'),
        v2.query_variable("device", "Device", 'label_values(polylens_device_connected{tenant_id=~"$tenant",site_name=~"$site"}, device_name)'),
        v2.query_variable("collector", "Collector", 'label_values(polylens2otel_collector_availability{tenant_id=~"$tenant"}, collector_id)'),
    ]


def _overview(builder: v2.Builder) -> dict:
    health = [
        _stat(builder, "Lens-connected phones", v2.prometheus(_current_total(f"polylens_device_connected{{{FLEET}}}"), instant=True),
              description="Phones currently connected to Poly Lens."),
        _stat(builder, "Phone APIs healthy", v2.prometheus(_current_total(f'polyphone_api_state{{{FLEET},state="ok"}}'), instant=True),
              description="Phones whose direct REST API is reachable, identified, and authenticated."),
        _stat(builder, "Registered lines", v2.prometheus(_current_total(f"polyphone_line_registered{{{FLEET}}}"), instant=True),
              description="Currently registered logical phone lines."),
        _stat(builder, "Active calls", v2.prometheus(f"{_current_total(f'polylens_device_active_calls{{{FLEET}}}')} or vector(0)", instant=True),
              description="Calls Poly Lens currently reports active."),
        _stat(builder, "Collectors healthy", v2.prometheus(_current_total(f"polylens2otel_collector_availability{{{COLLECTOR}}}"), instant=True),
              description="Collector runs whose most recent execution succeeded."),
        _stat(builder, "HTTP 5xx (1h)", v2.prometheus(f"sum(increase(polylens2otel_http_5xx_total{{{TENANT}}}[1h])) or vector(0)", instant=True),
              description="Unexpected upstream server failures. Digest 401 challenges are intentionally excluded.",
              threshold_config=v2.thresholds((None, "green"), (1, "red"))),
    ]
    fleet = [
        _ts(builder, "Fleet connectivity", [
            v2.prometheus(_gauge(f"polylens_device_connected{{{FLEET}}}"), "{{device_name}} Lens"),
            v2.prometheus(_gauge(f'polyphone_api_state{{{FLEET},state="ok"}}'), "{{device_name}} phone API"),
            v2.prometheus(_gauge(f"polyphone_line_registered{{{FLEET}}}"), "{{device_name}} line {{line}}"),
        ], unit="bool", description="Cloud, direct REST, and SIP registration health together."),
        _ts(builder, "Collector duration vs expected interval", [
            v2.prometheus(_gauge(f"polylens2otel_collector_duration{{{COLLECTOR}}}"), "{{collector_id}} duration"),
            v2.prometheus(_gauge(f"polylens2otel_collector_expected_interval{{{COLLECTOR}}}"), "{{collector_id}} interval"),
        ], unit="s", description="A collector approaching its interval cannot keep its schedule."),
    ]
    per_device = [
        _stat(builder, "Lens connected", v2.prometheus(_gauge(f"polylens_device_connected{{{FLEET}}}"), instant=True), unit="bool", mappings=BOOL_MAPPING),
        _stat(builder, "Phone API healthy", v2.prometheus(_gauge(f'polyphone_api_state{{{FLEET},state="ok"}}'), instant=True), unit="bool", mappings=BOOL_MAPPING),
        _stat(builder, "Lines registered", v2.prometheus(_current_total(f"polyphone_line_registered{{{FLEET}}}"), instant=True)),
        _stat(builder, "Uptime", v2.prometheus(_gauge(f"polyphone_uptime_seconds{{{FLEET}}}"), instant=True), unit="s"),
        _stat(builder, "Firmware current", v2.prometheus(_gauge(f"polylens_device_firmware_current{{{FLEET}}}"), instant=True), unit="bool", mappings=BOOL_MAPPING),
        _stat(builder, "Active calls", v2.prometheus(f"{_current_total(f'polylens_device_active_calls{{{FLEET}}}')} or vector(0)", instant=True)),
    ]
    return v2.tab("Overview", [
        v2.row("Fleet health", health),
        v2.row("Health over time", fleet),
        v2.row("Per-device health: $device", per_device, repeat="device"),
    ])


def _lens(builder: v2.Builder) -> dict:
    return v2.tab("Lens fleet", [
        v2.row("Connection and recency", [
            _ts(builder, "Lens connection state", [v2.prometheus(_gauge(f"polylens_device_connected{{{FLEET}}}"), "{{device_name}}")], unit="bool"),
            _ts(builder, "Time since Lens detected phone", [v2.prometheus(_gauge(f"polylens_device_last_detected_seconds{{{FLEET}}}"), "{{device_name}}")], unit="s"),
            _ts(builder, "Time since configuration request", [v2.prometheus(_gauge(f"polylens_device_last_config_request_seconds{{{FLEET}}}"), "{{device_name}}")], unit="s"),
            _ts(builder, "Device stream", [v2.prometheus(_gauge(f"polylens_stream_connected{{{TENANT}}}"), "stream")], unit="bool"),
        ]),
        v2.row("Firmware and calls", [
            _table(builder, "Firmware inventory", [
                v2.prometheus(_gauge(f"polylens_device_firmware_info{{{FLEET}}}"), instant=True, fmt="table"),
                v2.prometheus(_gauge(f"polylens_device_firmware_current{{{FLEET}}}"), instant=True, fmt="table"),
            ], description="Installed version/build and whether Lens reports it current."),
            _ts(builder, "Active calls by state", [v2.prometheus(_gauge(f"polylens_device_active_calls{{{FLEET}}}"), "{{device_name}} {{state}}")], description="Active-call counts supplied by Lens."),
        ]),
    ])


def _phone(builder: v2.Builder) -> dict:
    return v2.tab("Phone REST", [
        v2.row("Reachability and registration", [
            _table(builder, "Phone API state", [v2.prometheus(_gauge(f"polyphone_api_state{{{FLEET}}}"), instant=True, fmt="table")],
                   description="Exactly one of ok, api_disabled, auth_failed, or unreachable is 1 per phone."),
            _ts(builder, "Line registration", [v2.prometheus(_gauge(f"polyphone_line_registered{{{FLEET}}}"), "{{device_name}} line {{line}}")], unit="bool"),
            _ts(builder, "Logical lines", [v2.prometheus(_gauge(f"polyphone_lines_total{{{FLEET}}}"), "{{device_name}}")]),
            _ts(builder, "Phone uptime", [v2.prometheus(_gauge(f"polyphone_uptime_seconds{{{FLEET}}}"), "{{device_name}}")], unit="s"),
        ]),
        v2.row("Network and configuration", [
            _ts(builder, "Packet rate", [v2.prometheus(_counter_rate(f"polyphone_network_packets_total{{{FLEET}}}"), "{{device_name}} {{direction}}")], unit="pps"),
            _table(builder, "Network inventory", [v2.prometheus(_gauge(f"polyphone_network_info{{{FLEET}}}"), instant=True, fmt="table")],
                   description="DHCP, gateway, subnet, boot-server, and DHCP-server metadata."),
            _table(builder, "Configuration source", [v2.prometheus(_gauge(f"polyphone_config_param_source{{{FLEET}}}"), instant=True, fmt="table")],
                   description="Whether each allowlisted phone parameter came from config or its default."),
        ]),
    ])


def _calls(builder: v2.Builder) -> dict:
    safe_phone = ('{service_name="polylens2otel"} | event_name="polyphone.call_record" '
                  '| tenant_id=~"$tenant" | site_name=~"$site" | device_name=~"$device" '
                  '| line_format "{{.device_name}} line={{.line}} direction={{.direction}} '
                  'result={{.disposition}} duration={{.duration_seconds}}s"')
    safe_cdr = ('{service_name="polylens2otel"} | event_name="polylens.cdr" '
                '| tenant_id=~"$tenant" '
                '| line_format "device={{.device_id}} direction={{.direction}} '
                'duration={{.duration_seconds}}s model={{.device_model}}"')
    return v2.tab("Calls & logs", [
        v2.row("Call activity", [
            _ts(builder, "Phone call-log records", [v2.prometheus(_counter_rate(f"polyphone_calls_total{{{FLEET}}}"), "{{device_name}} {{direction}}")], unit="ops"),
            _ts(builder, "Lens CDR records", [v2.loki('sum(rate({service_name="polylens2otel"} | event_name="polylens.cdr" | tenant_id=~"$tenant" [$__rate_interval]))')], unit="ops",
                description="Rate of Lens CDR rows. Empty means no calls in the selected range."),
        ]),
        v2.row("Sanitized call records", [
            _logs(builder, "Phone call records", safe_phone,
                  description="Direct-phone call history with remote party deliberately removed from the rendered line."),
            _logs(builder, "Lens CDR events", safe_cdr,
                  description="Lens CDR history with raw bodies and call parties deliberately removed from the rendered line."),
        ]),
    ])


def _self_o11y(builder: v2.Builder) -> dict:
    return v2.tab("Self-o11y", [
        v2.row("Build and scheduling", [
            _table(builder, "Running build", [v2.prometheus(f"topk by (tenant_id) (1, timestamp(polylens2otel_build_info{{{TENANT}}}))", instant=True, fmt="table")]),
            _ts(builder, "Collector availability", [v2.prometheus(_gauge(f"polylens2otel_collector_availability{{{COLLECTOR}}}"), "{{collector_id}}")], unit="bool"),
            _ts(builder, "Collector duration", [v2.prometheus(_gauge(f"polylens2otel_collector_duration{{{COLLECTOR}}}"), "{{collector_id}}")], unit="s"),
            _ts(builder, "Expected interval", [v2.prometheus(_gauge(f"polylens2otel_collector_expected_interval{{{COLLECTOR}}}"), "{{collector_id}}")], unit="s"),
        ]),
        v2.row("Upstream APIs and authentication", [
            _ts(builder, "HTTP request duration", [v2.prometheus(_gauge(f"polylens2otel_http_client_request_duration{{{TENANT}}}"), "{{source}} {{device_name}}")], unit="s"),
            _ts(builder, "HTTP response classes", [
                v2.prometheus(_counter_rate(f"polylens2otel_http_4xx_total{{{TENANT}}}"), "4xx {{source}}"),
                v2.prometheus(_counter_rate(f"polylens2otel_http_5xx_total{{{TENANT}}}"), "5xx {{source}}"),
            ], unit="ops", description="Includes expected Digest 401 challenges and API-disabled 404 classifications; use the 5xx headline for failures."),
            _ts(builder, "Unexpected API shapes", [v2.prometheus(_gauge(f"polylens2otel_api_unexpected{{{TENANT}}}"), "{{source}} {{device_name}}")]),
            _ts(builder, "Lens token refreshes", [v2.prometheus(_counter_rate(f"polylens2otel_auth_token_refresh_total{{{TENANT}}}"), "refreshes/s")], unit="ops"),
            _ts(builder, "Lens token expiry", [v2.prometheus(_gauge(f"polylens2otel_auth_token_expires_seconds{{{TENANT}}}"), "expires in")], unit="s"),
        ]),
        v2.row("Streaming and ingestion", [
            _ts(builder, "Stream reconnects", [v2.prometheus(_counter_rate(f"polylens2otel_stream_reconnects_total{{{TENANT}}}"), "reconnects/s")], unit="ops"),
            _ts(builder, "Stream messages", [v2.prometheus(_counter_rate(f"polylens2otel_stream_messages_total{{{TENANT}}}"), "messages/s")], unit="ops"),
            _ts(builder, "Time since stream message", [v2.prometheus(_gauge(f"polylens2otel_stream_last_message_seconds{{{TENANT}}}"), "age")], unit="s"),
            _ts(builder, "Ingest accounting", [
                v2.prometheus(_counter_rate(f"polylens2otel_ingest_emitted_points_total{{{COLLECTOR}}}"), "{{collector_id}} emitted"),
                v2.prometheus(_counter_rate(f"polylens2otel_ingest_source_records_total{{{COLLECTOR}}}"), "{{collector_id}} source"),
            ], unit="ops"),
            _ts(builder, "Checkpoint persistence errors", [v2.prometheus(_counter_rate(f"polylens2otel_checkpoint_persist_errors_total{{{COLLECTOR}}}"), "{{collector_id}}")], unit="ops"),
        ]),
    ])


def _traces(builder: v2.Builder) -> dict:
    tenant_trace = 'span.tenant.id =~ "$tenant"'
    collector_trace = f'{tenant_trace} && span.collector.id =~ "$collector"'
    return v2.tab("Traces", [
        v2.row("Trace search", [
            _table(builder, "Collector-run traces", [v2.tempo(f'{{ resource.service.name = "polylens2otel" && name =~ "collector\\..+" && {collector_trace} }}')],
                   width=24, height=10, description="One trace per scheduled collector run, with child HTTP calls."),
            _table(builder, "Outbound HTTP spans", [v2.tempo(f'{{ resource.service.name = "polylens2otel" && name = "http.client.request" && {tenant_trace} }}')],
                   width=24, height=10, description="Instrumented Lens and phone HTTP calls."),
        ]),
        v2.row("Trace-derived health", [
            _ts(builder, "Collector p95 duration", [v2.tempo(f'{{ resource.service.name = "polylens2otel" && name =~ "collector\\..+" && {collector_trace} }} | quantile_over_time(duration, 0.95) by (span.collector.id)')], unit="s"),
            _ts(builder, "HTTP p95 by source", [v2.tempo(f'{{ resource.service.name = "polylens2otel" && name = "http.client.request" && {tenant_trace} }} | quantile_over_time(duration, 0.95) by (span.source)')], unit="s"),
            _ts(builder, "Collector run rate", [v2.tempo(f'{{ resource.service.name = "polylens2otel" && name =~ "collector\\..+" && {collector_trace} }} | rate() by (span.collector.id)')], unit="ops"),
            _ts(builder, "HTTP request rate", [v2.tempo(f'{{ resource.service.name = "polylens2otel" && name = "http.client.request" && {tenant_trace} }} | rate() by (span.source)')], unit="ops"),
        ]),
    ])


def _profiles(builder: v2.Builder, cat: dict) -> dict:
    titles = {
        "process_cpu:cpu:nanoseconds:cpu:nanoseconds": "CPU time flame graph",
        "process_cpu:samples:count:cpu:nanoseconds": "CPU samples flame graph",
        "memory:alloc_objects:count:space:bytes": "Allocated objects flame graph",
        "memory:alloc_space:bytes:space:bytes": "Allocated bytes flame graph",
        "memory:inuse_objects:count:space:bytes": "In-use objects flame graph",
        "memory:inuse_space:bytes:space:bytes": "In-use bytes flame graph",
    }
    panels = []
    for item in cat["profiles"]:
        profile_type = item["profile_type"]
        panels.append(builder.panel(
            titles[profile_type], "flamegraph", [v2.profile(profile_type)],
            width=12, height=10,
            description=f"Pyroscope profile type `{profile_type}` for polylens2otel.",
            no_value="This profile type has no samples in the selected time range.",
        ))
    return v2.tab("Profiles", [v2.row("Continuous profiles", panels)])


def render() -> dict:
    cat = catalog()
    waivers = json.loads(WAIVERS.read_text(encoding="utf-8"))["metrics"]
    known = {metric["name"] for metric in cat["metrics"]}
    stale = set(waivers) - known
    if stale:
        raise SystemExit(f"stale metric waivers: {sorted(stale)}")
    if any(not str(reason).strip() for reason in waivers.values()):
        raise SystemExit("every metric waiver requires a reason")

    builder = v2.Builder()
    tabs = [
        _overview(builder),
        _lens(builder),
        _phone(builder),
        _calls(builder),
        _self_o11y(builder),
        _traces(builder),
        _profiles(builder, cat),
    ]
    return v2.manifest(builder, tabs, _variables())


def render_folder() -> dict:
    return {
        "apiVersion": "folder.grafana.app/v1",
        "kind": "Folder",
        "metadata": {"name": "polylens2otel-dashboards"},
        "spec": {"title": "polylens2otel Dashboards"},
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    write_or_check(FOLDER_OUT, dump_json(render_folder()), args.check)
    write_or_check(OUT, dump_json(render()), args.check)
    cat = catalog()
    print(
        "dashboard coverage: "
        f"{len(cat['metrics'])} metrics, {len(cat['logs'])} logs, "
        f"{len(cat['traces'])} trace families, and {len(cat['profiles'])} profile types accounted for"
    )


if __name__ == "__main__":
    main()
