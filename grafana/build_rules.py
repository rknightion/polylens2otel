#!/usr/bin/env python3
"""Generate alert rules using catalog-resolved Prometheus names."""

from __future__ import annotations

import argparse

from common import ROOT, catalog, write_or_check

OUT = ROOT / "alerts" / "polylens2otel.yaml"


def render() -> str:
    names = {metric["name"]: metric["prometheus"] for metric in catalog()["metrics"]}
    rules = [
        ("polylens2otel-phone-api-disabled", f'{names["polyphone.api_state"]}{{state="api_disabled"}} == 1', "warning", "Phone management API is disabled"),
        ("polylens2otel-line-unregistered", f'{names["polyphone.line.registered"]} == 0', "critical", "A configured phone line is unregistered"),
        ("polylens2otel-collector-unavailable", f'{names["polylens2otel.collector.availability"]} == 0', "warning", "A collector run failed"),
    ]
    lines = ["apiVersion: 1", "groups:", "  - orgId: 1", "    name: polylens2otel", "    folder: polylens2otel", "    interval: 1m", "    rules:"]
    for uid, expr, severity, summary in rules:
        lines.extend([
            f"      - uid: {uid}",
            f"        title: {summary}",
            "        condition: A",
            "        for: 2m",
            "        noDataState: NoData",
            "        execErrState: Error",
            "        labels:",
            "          pipeline: polylens2otel",
            f"          severity: {severity}",
            "          source: polylens2otel",
            "        annotations:",
            f"          summary: {summary}",
            "        data:",
            "          - refId: A",
            "            relativeTimeRange: {from: 300, to: 0}",
            "            datasourceUid: grafanacloud-prom",
            "            model:",
            "              datasource: {type: prometheus, uid: grafanacloud-prom}",
            "              editorMode: code",
            f"              expr: '{expr}'",
            "              instant: true",
            "              refId: A",
        ])
    return "\n".join(lines) + "\n"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    write_or_check(OUT, render(), args.check)
    print("alert rules: 3 catalog-resolved expressions")


if __name__ == "__main__":
    main()
