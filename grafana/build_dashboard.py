#!/usr/bin/env python3
"""Generate the fleet dashboard and enforce complete metric coverage."""

from __future__ import annotations

import argparse
import json

from common import ROOT, catalog, dump_json, write_or_check

OUT = ROOT / "dashboards" / "polylens2otel.json"
WAIVERS = ROOT / "grafana" / "waivers.json"


def panel(panel_id: int, title: str, metrics: list[dict]) -> dict:
    targets = [
        {"refId": chr(65 + i), "expr": metric["prometheus"], "legendFormat": metric["name"]}
        for i, metric in enumerate(metrics)
    ]
    return {
        "id": panel_id,
        "title": title,
        "type": "timeseries",
        "datasource": {"type": "prometheus", "uid": "grafanacloud-prom"},
        "targets": targets,
        "gridPos": {"h": 9, "w": 12, "x": ((panel_id - 1) % 2) * 12, "y": ((panel_id - 1) // 2) * 9},
    }


def render() -> dict:
    cat = catalog()
    waivers = json.loads(WAIVERS.read_text(encoding="utf-8"))["metrics"]
    known = {metric["name"] for metric in cat["metrics"]}
    stale = set(waivers) - known
    if stale:
        raise SystemExit(f"stale metric waivers: {sorted(stale)}")
    if any(not str(reason).strip() for reason in waivers.values()):
        raise SystemExit("every metric waiver requires a reason")
    groups = {"Lens fleet": [], "Phone fleet": [], "Exporter health": []}
    covered = set()
    for metric in cat["metrics"]:
        if metric["name"] in waivers:
            continue
        if metric["name"].startswith("polylens2otel."):
            groups["Exporter health"].append(metric)
        elif metric["name"].startswith("polyphone."):
            groups["Phone fleet"].append(metric)
        else:
            groups["Lens fleet"].append(metric)
        covered.add(metric["name"])
    missing = known - covered - set(waivers)
    if missing:
        raise SystemExit(f"metrics neither panelled nor waived: {sorted(missing)}")
    panels = [panel(i + 1, title, metrics) for i, (title, metrics) in enumerate(groups.items())]
    panels.append({
        "id": 4,
        "title": "Lens CDR events",
        "type": "logs",
        "datasource": {"type": "loki", "uid": "grafanacloud-logs"},
        "targets": [{"refId": "A", "expr": '{service_name="polylens2otel"} | event_name="polylens.cdr"'}],
        "gridPos": {"h": 9, "w": 24, "x": 0, "y": 18},
    })
    panels.append({
        "id": 5,
        "title": "Phone call records",
        "type": "logs",
        "datasource": {"type": "loki", "uid": "grafanacloud-logs"},
        "targets": [{"refId": "A", "expr": '{service_name="polylens2otel"} | event_name="polyphone.call_record"'}],
        "gridPos": {"h": 9, "w": 24, "x": 0, "y": 27},
    })
    return {
        "title": "polylens2otel",
        "uid": "polylens2otel",
        "schemaVersion": 41,
        "tags": ["polylens2otel", "generated"],
        "timezone": "browser",
        "refresh": "1m",
        "time": {"from": "now-6h", "to": "now"},
        "panels": panels,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    write_or_check(OUT, dump_json(render()), args.check)
    print(f"dashboard coverage: {len(catalog()['metrics'])} metrics accounted for")


if __name__ == "__main__":
    main()
