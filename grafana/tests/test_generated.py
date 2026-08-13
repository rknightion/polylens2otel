import json
import unittest

import build_dashboard
import build_rules


class GeneratedArtefactTests(unittest.TestCase):
    @staticmethod
    def _panel(dashboard, title):
        return next(
            panel
            for panel in dashboard["spec"]["elements"].values()
            if panel["spec"]["title"] == title
        )

    @staticmethod
    def _queries(panel):
        return [
            query["spec"]["query"]["spec"]
            for query in panel["spec"]["data"]["spec"]["queries"]
        ]

    def test_dashboard_is_v2_with_the_complete_top_level_tab_set(self):
        dashboard = build_dashboard.render()
        self.assertEqual(dashboard["apiVersion"], "dashboard.grafana.app/v2")
        self.assertEqual(dashboard["kind"], "Dashboard")
        self.assertEqual(dashboard["metadata"]["name"], "polylens2otel")
        self.assertEqual(dashboard["spec"]["layout"]["kind"], "TabsLayout")
        self.assertEqual(
            [tab["spec"]["title"] for tab in dashboard["spec"]["layout"]["spec"]["tabs"]],
            ["Overview", "Lens fleet", "Phone REST", "Calls & logs", "Self-o11y", "Traces", "Profiles"],
        )

    def test_dashboard_targets_the_generated_project_folder(self):
        folder = build_dashboard.render_folder()
        dashboard = build_dashboard.render()
        self.assertEqual(folder["apiVersion"], "folder.grafana.app/v1")
        self.assertEqual(folder["metadata"]["name"], "polylens2otel-dashboards")
        self.assertEqual(folder["spec"]["title"], "polylens2otel Dashboards")
        self.assertEqual(
            dashboard["metadata"]["annotations"]["grafana.app/folder"],
            folder["metadata"]["name"],
        )

    def test_every_catalog_metric_is_queried_or_waived(self):
        cat = build_dashboard.catalog()
        rendered = json.dumps(build_dashboard.render())
        waivers = json.loads(build_dashboard.WAIVERS.read_text())["metrics"]
        for metric in cat["metrics"]:
            self.assertTrue(metric["prometheus"] in rendered or metric["name"] in waivers, metric["name"])

    def test_catalog_prometheus_names_follow_the_exported_normalization_contract(self):
        for metric in build_dashboard.catalog()["metrics"]:
            expected = metric["name"].replace(".", "_")
            if metric["kind"] == "counter":
                expected += "_total"
            self.assertEqual(metric["prometheus"], expected, metric["name"])

    def test_every_non_metric_signal_is_visualized(self):
        cat = build_dashboard.catalog()
        rendered = build_dashboard.render()
        strings = []

        def collect(value):
            if isinstance(value, str):
                strings.append(value)
            elif isinstance(value, dict):
                for child in value.values():
                    collect(child)
            elif isinstance(value, list):
                for child in value:
                    collect(child)

        collect(rendered)
        query_text = "\n".join(strings)
        for log in cat["logs"]:
            self.assertIn(log["event_name"], query_text)
        for trace in cat["traces"]:
            self.assertIn(trace["span_name"], query_text)
        for profile in cat["profiles"]:
            self.assertIn(profile["profile_type"], query_text)

    def test_every_built_panel_is_placed_once(self):
        dashboard = build_dashboard.render()
        placed = []
        for tab in dashboard["spec"]["layout"]["spec"]["tabs"]:
            for row in tab["spec"]["layout"]["spec"]["rows"]:
                for item in row["spec"]["layout"]["spec"]["items"]:
                    placed.append(item["spec"]["element"]["name"])
        self.assertEqual(set(placed), set(dashboard["spec"]["elements"]))
        self.assertEqual(len(placed), len(set(placed)))

    def test_current_state_panels_collapse_stale_series_from_the_previous_build(self):
        dashboard = build_dashboard.render()
        for title in [
            "Lens-connected phones",
            "Phone APIs healthy",
            "Registered lines",
            "Collectors healthy",
            "Fleet connectivity",
        ]:
            expressions = [
                query["expr"]
                for query in self._queries(self._panel(dashboard, title))
            ]
            self.assertTrue(
                all("without (service_version)" in expression for expression in expressions),
                (title, expressions),
            )

    def test_cdr_activity_is_a_loki_rate_query_not_an_unwrapped_log_stream(self):
        dashboard = build_dashboard.render()
        query = self._queries(self._panel(dashboard, "Lens CDR records"))[0]["expr"]
        self.assertIn("rate(", query)
        self.assertIn("event_name=\"polylens.cdr\"", query)
        self.assertIn("[5m]", query)
        self.assertNotIn("$__rate_interval", query)
        self.assertNotIn("unwrap", query)

    def test_trace_search_uses_tables_and_collector_metrics_group_by_collector_id(self):
        dashboard = build_dashboard.render()
        for title in ["Collector-run traces", "Outbound HTTP spans"]:
            panel = self._panel(dashboard, title)
            self.assertEqual(panel["spec"]["vizConfig"]["group"], "table")
        for title in ["Collector p95 duration", "Collector run rate"]:
            query = self._queries(self._panel(dashboard, title))[0]["query"]
            self.assertIn("span.collector.id", query)
            self.assertIn("$collector", query)

    def test_collector_trace_queries_use_a_serialization_safe_dot_regex(self):
        dashboard = build_dashboard.render()
        for title in ["Collector-run traces", "Collector p95 duration", "Collector run rate"]:
            query = self._queries(self._panel(dashboard, title))[0]["query"]
            self.assertIn('name =~ "collector[.].+"', query)
            self.assertNotIn(r"collector\..+", query)

    def test_headline_http_failure_stat_does_not_treat_digest_challenges_as_errors(self):
        dashboard = build_dashboard.render()
        panel = self._panel(dashboard, "HTTP 5xx (1h)")
        query = self._queries(panel)[0]["expr"]
        self.assertIn("polylens2otel_http_5xx_total", query)
        self.assertNotIn("polylens2otel_http_4xx_total", query)

    def test_uptime_stat_does_not_use_grafanas_misleading_default_threshold(self):
        dashboard = build_dashboard.render()
        defaults = self._panel(dashboard, "Uptime")["spec"]["vizConfig"]["spec"]["fieldConfig"]["defaults"]
        self.assertEqual(
            defaults["thresholds"],
            {"mode": "absolute", "steps": [{"value": None, "color": "green"}]},
        )

    def test_rules_use_only_catalog_metric_names(self):
        rendered = build_rules.render()
        known = {metric["prometheus"] for metric in build_rules.catalog()["metrics"]}
        for line in rendered.splitlines():
            if "expr: '" not in line:
                continue
            expression = line.split("expr: '", 1)[1].rsplit("'", 1)[0]
            self.assertTrue(any(name in expression for name in known), expression)


if __name__ == "__main__":
    unittest.main()
