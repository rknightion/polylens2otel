import json
import unittest

import build_dashboard
import build_rules


class GeneratedArtefactTests(unittest.TestCase):
    def test_every_catalog_metric_is_queried_or_waived(self):
        cat = build_dashboard.catalog()
        rendered = json.dumps(build_dashboard.render())
        waivers = json.loads(build_dashboard.WAIVERS.read_text())["metrics"]
        for metric in cat["metrics"]:
            self.assertTrue(metric["prometheus"] in rendered or metric["name"] in waivers, metric["name"])

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
