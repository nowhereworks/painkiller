import copy
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
from core_ready import check
from fixture_objects import objects


def ready_items():
    items = copy.deepcopy(objects())
    for obj in items:
        if obj["kind"] == "Pod":
            obj["status"] = {"conditions": [{"type": "Ready", "status": "True"}]}
        elif obj["kind"] == "Deployment":
            obj["metadata"]["generation"] = 1
            obj["status"] = {"observedGeneration": 1, "updatedReplicas": 1, "readyReplicas": 1}
    return items


class CoreReadinessTests(unittest.TestCase):
    def test_ready_subset(self):
        self.assertEqual(check(ready_items()), [])

    def test_missing_or_stale_fixture_rejected(self):
        items = ready_items()
        deployment = next(o for o in items if o["kind"] == "Deployment" and o["metadata"]["name"] == "north")
        deployment["metadata"]["generation"] = 2
        self.assertTrue(any("private-mesh/north" in f for f in check(items)))
        items.remove(next(o for o in items if o["kind"] == "Pod" and o["metadata"]["name"] == "invoice-web"))
        self.assertTrue(any("invoice-web" in f for f in check(items)))

    def test_candidate_answer_and_fault_removal_rejected(self):
        items = ready_items()
        items.append({"kind": "NetworkPolicy", "metadata": {"namespace": "storefront", "name": "web-access"}})
        role = next(o for o in items if o["kind"] == "ClusterRole")
        role["rules"] = role["rules"][:1]
        failures = check(items)
        self.assertTrue(any("storefront/web-access" in f for f in failures))
        self.assertTrue(any("RBAC fault" in f for f in failures))

    def test_cli_uses_only_one_bounded_kubectl_get(self):
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            (directory / "snapshot.json").write_text(json.dumps({"items": ready_items()}))
            stub = directory / "kubectl"
            stub.write_text(
                "#!/usr/bin/env python3\n"
                "import pathlib, sys\n"
                "assert sys.argv[1:3] == ['--request-timeout=5s', 'get']\n"
                "assert sys.argv[-3:] == ['--all-namespaces', '-o', 'json']\n"
                "print((pathlib.Path(__file__).parent / 'snapshot.json').read_text())\n"
            )
            stub.chmod(0o755)
            env = dict(os.environ, PATH=f"{tmp}:{os.environ['PATH']}")
            result = subprocess.run(
                [sys.executable, "-B", str(Path(__file__).resolve().parents[1] / "assets/core_ready.py")],
                env=env, text=True, capture_output=True, timeout=10, check=True)
            self.assertIn("not full exam readiness", result.stdout)


if __name__ == "__main__":
    unittest.main()
