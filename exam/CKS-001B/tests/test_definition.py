import json
from pathlib import Path
import sys
import unittest
from unittest.mock import patch

HERE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE / "assets"))
import build
import capture
import falco_capture


class DefinitionTests(unittest.TestCase):
    def test_generated_contracts_and_independence(self):
        definitions = build.generated()
        self.assertEqual(len(definitions["tasks.json"]), 16)
        self.assertEqual(sum(t["points"] for t in definitions["tasks.json"]), 100)
        self.assertEqual(len(definitions["e2e.json"]["mutations"]), 16)
        for name, content in definitions.items():
            self.assertEqual(json.loads((HERE / name).read_text()), content)
        for number, task in enumerate(definitions["tasks.json"], 1):
            self.assertEqual(task["question_id"], f"CKS-001B-Q{number}")
            self.assertEqual(sum(c["points"] for c in task["checks"]), task["points"])
            self.assertTrue(task["steps"] and task["explanation"])
            self.assertNotIn("/cks001/", task["resource"])
            self.assertNotIn("exam/CKS-001/", task["solution"])

    def test_unchanged_dockerfile_cannot_earn_points(self):
        with patch.object(capture, "files", return_value={
                "base": False, "binary": True, "nonroot": False, "exec": False,
                "minimal": False, "failures": ["Use distroless"]}):
            result = capture.capture("q7")
        self.assertFalse(any(value for key, value in result.items() if key != "failures"))

    def test_falco_requires_all_real_processes_and_correct_fields(self):
        rule = {"rule": "original name", "condition": "some valid Falco expression",
                "priority": "CRITICAL", "output": falco_capture.OUTPUT, "tags": list(falco_capture.TAGS)}
        self.assertTrue(all(falco_capture.rule_fields(rule, falco_capture.MINERS).values()))
        self.assertFalse(any(falco_capture.rule_fields(rule, {"xmrig"}).values()))
        self.assertFalse(falco_capture.rule_fields(rule | {"priority": "WARNING"}, falco_capture.MINERS)["priority"])
        self.assertFalse(falco_capture.rule_fields(rule | {"tags": []}, falco_capture.MINERS)["output"])

    def test_falco_distribution_rules_cannot_shadow_candidate_rule(self):
        import yaml
        from host_fixtures import FALCO_CONFIG
        config = yaml.safe_load(FALCO_CONFIG)
        self.assertEqual(config["rule_matching"], "all")
        self.assertFalse(config["buffered_outputs"])


if __name__ == "__main__":
    unittest.main()
