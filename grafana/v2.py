"""Small Grafana dashboard v2 builder used by the generated fleet dashboard.

The dashboard API v2 separates panels (``spec.elements``) from their placement
(``spec.layout``).  Keeping that split here makes it possible to assert that no
panel is generated but left invisible, and keeps the signal-oriented generator
free of verbose schema plumbing.
"""

from __future__ import annotations

API_VERSION = "dashboard.grafana.app/v2"
VIZ_VERSION = "12.1.0"


def datasource_variable(name: str, label: str, plugin_id: str, default: str) -> dict:
    return {
        "kind": "DatasourceVariable",
        "spec": {
            "name": name,
            "label": label,
            "pluginId": plugin_id,
            "current": {"text": default, "value": default},
            "options": [],
            "multi": False,
            "includeAll": False,
            "allowCustomValue": True,
            "hide": "dontHide",
            "refresh": "onDashboardLoad",
            "regex": "",
            "skipUrlSync": False,
        },
    }


def query_variable(name: str, label: str, query: str) -> dict:
    return {
        "kind": "QueryVariable",
        "spec": {
            "name": name,
            "label": label,
            "hide": "dontHide",
            "query": {
                "kind": "DataQuery",
                "version": "v0",
                "group": "",
                "datasource": {"name": "${ds_prometheus}"},
                "spec": {"query": query, "refId": name},
            },
            "current": {"text": "All", "value": "$__all"},
            "options": [],
            "multi": True,
            "includeAll": True,
            "allowCustomValue": True,
            "refresh": "onTimeRangeChanged",
            "regex": "",
            "skipUrlSync": False,
            "sort": "alphabeticalAsc",
            "allValue": ".*",
        },
    }


def _target(datasource: str, spec: dict, ref_id: str = "A") -> dict:
    return {
        "kind": "PanelQuery",
        "spec": {
            "refId": ref_id,
            "hidden": False,
            "query": {
                "kind": "DataQuery",
                "version": "v0",
                "group": "",
                "datasource": {"name": datasource},
                "spec": spec,
            },
        },
    }


def prometheus(expr: str, legend: str = "", *, instant: bool = False,
               ref_id: str = "A", fmt: str = "time_series") -> dict:
    return _target("${ds_prometheus}", {
        "expr": expr,
        "instant": instant,
        "range": not instant,
        "legendFormat": legend,
        "format": fmt,
    }, ref_id)


def loki(expr: str, *, ref_id: str = "A", max_lines: int = 100) -> dict:
    return _target("${ds_loki}", {
        "expr": expr,
        "queryType": "range",
        "maxLines": max_lines,
        "legendFormat": "",
    }, ref_id)


def tempo(query: str, *, ref_id: str = "A", table_type: str = "traces") -> dict:
    return _target("${ds_tempo}", {
        "query": query,
        "queryType": "traceql",
        "tableType": table_type,
    }, ref_id)


def profile(profile_type: str, *, ref_id: str = "A") -> dict:
    return _target("${ds_profiles}", {
        "labelSelector": '{service_name="polylens2otel"}',
        "profileTypeId": profile_type,
        "queryType": "profile",
        "groupBy": [],
    }, ref_id)


def thresholds(*steps: tuple[float | None, str]) -> dict:
    return {
        "mode": "absolute",
        "steps": [{"value": value, "color": color} for value, color in steps],
    }


def stat_options() -> dict:
    return {
        "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
        "colorMode": "background",
        "graphMode": "area",
        "textMode": "auto",
        "justifyMode": "auto",
    }


def timeseries_options() -> dict:
    return {
        "legend": {
            "displayMode": "table",
            "placement": "bottom",
            "showLegend": True,
            "calcs": ["lastNotNull", "max"],
        },
        "tooltip": {"mode": "multi", "sort": "desc"},
    }


def logs_options() -> dict:
    return {
        "showTime": True,
        "showLabels": False,
        "showCommonLabels": False,
        "wrapLogMessage": True,
        "prettifyLogMessage": False,
        "enableLogDetails": True,
        "sortOrder": "Descending",
    }


class Builder:
    def __init__(self) -> None:
        self.elements: dict[str, dict] = {}
        self._panel_id = 0

    def panel(self, title: str, panel_type: str, targets: list[dict], *,
              width: int = 12, height: int = 7, description: str = "",
              unit: str | None = None, options: dict | None = None,
              custom: dict | None = None, mappings: list | None = None,
              threshold_config: dict | None = None,
              transformations: list | None = None,
              no_value: str = "No data in the selected time range.") -> dict:
        self._panel_id += 1
        name = f"panel-{self._panel_id}"
        for index, target in enumerate(targets):
            target["spec"]["refId"] = chr(65 + index)

        defaults: dict = {"noValue": no_value}
        if unit is not None:
            defaults["unit"] = unit
        if custom:
            defaults["custom"] = custom
        if mappings:
            defaults["mappings"] = mappings
        if threshold_config:
            defaults["thresholds"] = threshold_config

        self.elements[name] = {
            "kind": "Panel",
            "spec": {
                "id": self._panel_id,
                "title": title,
                "description": description,
                "links": [],
                "data": {
                    "kind": "QueryGroup",
                    "spec": {
                        "queries": targets,
                        "queryOptions": {},
                        "transformations": transformations or [],
                    },
                },
                "vizConfig": {
                    "kind": "VizConfig",
                    "group": panel_type,
                    "version": VIZ_VERSION,
                    "spec": {
                        "options": options or {},
                        "fieldConfig": {"defaults": defaults, "overrides": []},
                    },
                },
            },
        }
        return {"name": name, "width": width, "height": height}


def grid(items: list[dict]) -> dict:
    placed, x, y, row_height = [], 0, 0, 0
    for item in items:
        width, height = item["width"], item["height"]
        if x + width > 24:
            x, y, row_height = 0, y + row_height, 0
        placed.append({
            "kind": "GridLayoutItem",
            "spec": {
                "x": x,
                "y": y,
                "width": width,
                "height": height,
                "element": {"kind": "ElementReference", "name": item["name"]},
            },
        })
        x += width
        row_height = max(row_height, height)
    return {"kind": "GridLayout", "spec": {"items": placed}}


def row(title: str, items: list[dict], *, repeat: str | None = None,
        collapse: bool = False) -> dict:
    spec: dict = {"title": title, "collapse": collapse, "layout": grid(items)}
    if not title:
        spec["hideHeader"] = True
    if repeat:
        spec["repeat"] = {"mode": "variable", "value": repeat}
    return {"kind": "RowsLayoutRow", "spec": spec}


def tab(title: str, rows: list[dict]) -> dict:
    return {
        "kind": "TabsLayoutTab",
        "spec": {
            "title": title,
            "layout": {"kind": "RowsLayout", "spec": {"rows": rows}},
        },
    }


def manifest(builder: Builder, tabs: list[dict], variables: list[dict]) -> dict:
    return {
        "apiVersion": API_VERSION,
        "kind": "Dashboard",
        "metadata": {
            "name": "polylens2otel",
            "annotations": {"grafana.app/folder": "polylens2otel-dashboards"},
        },
        "spec": {
            "title": "polylens2otel",
            "description": "Poly Lens fleet, phone REST, call records, exporter self-observability, traces, and profiles.",
            "tags": ["polylens2otel", "generated", "dynamic-dashboard"],
            "cursorSync": "Crosshair",
            "editable": True,
            "liveNow": False,
            "preload": False,
            "timeSettings": {
                "from": "now-24h",
                "to": "now",
                "autoRefresh": "1m",
                "autoRefreshIntervals": ["30s", "1m", "5m", "15m", "30m"],
                "timezone": "browser",
                "fiscalYearStartMonth": 0,
                "hideTimepicker": False,
            },
            "links": [],
            "annotations": [],
            "variables": variables,
            "elements": builder.elements,
            "layout": {"kind": "TabsLayout", "spec": {"tabs": tabs}},
        },
    }
