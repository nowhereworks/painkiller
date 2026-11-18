"""Offline fixture invariants; these do not prove a cluster is ready."""
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
from fixture_objects import NAMESPACES, objects


class FixtureTests(unittest.TestCase):
    def setUp(self):
        self.items = objects()
        self.by_key = {
            (o["kind"], o["metadata"].get("namespace"), o["metadata"]["name"]): o
            for o in self.items
        }

    def test_unique_objects_and_exam_marker(self):
        self.assertEqual(len(self.items), len(self.by_key))
        for ns in NAMESPACES:
            obj = self.by_key["Namespace", None, ns]
            self.assertEqual(obj["metadata"]["labels"]["painkiller-shell/exam"], "CKS-001B")

    def test_answers_are_not_precreated(self):
        self.assertFalse(any(o["kind"] in ("NetworkPolicy", "Ingress", "PeerAuthentication")
                             for o in self.items))
        self.assertNotIn(("Secret", "editor", "editor-cert"), self.by_key)
        finance = self.by_key["Namespace", None, "finance-ledger"]
        self.assertNotIn("pod-security.kubernetes.io/enforce", finance["metadata"]["labels"])
        gate = self.by_key["Pod", "workload-identity", "web-gate"]
        self.assertNotIn("securityContext", gate["spec"]["containers"][0])

    def test_runtime_fixtures_and_safety(self):
        report = self.by_key["Deployment", "immutable-workloads", "report-web"]
        spec = report["spec"]["template"]["spec"]
        self.assertEqual({v["mountPath"] for v in spec["containers"][0]["volumeMounts"]},
                         {"/tmp", "/var/log", "/var/cache/nginx", "/var/run"})
        self.assertTrue(all("emptyDir" in v for v in spec["volumes"]))
        invoice = self.by_key["Pod", "default", "invoice-web"]
        self.assertEqual(invoice["spec"]["containers"][0]["securityContext"]["capabilities"]["add"], ["NET_RAW"])
        orchard = self.by_key["Deployment", "produce", "orchard"]
        containers = orchard["spec"]["template"]["spec"]["containers"]
        self.assertEqual([c["name"] for c in containers], ["pear", "plum", "cherry"])
        self.assertEqual(sum("alpine/curl:" in c["image"] for c in containers), 1)

    def test_q5_preserves_explicit_rules_and_rbac_fault(self):
        role = self.by_key["ClusterRole", None, "pipeline-agent-role"]
        self.assertEqual(role["rules"][0]["resources"], ["pods", "services", "configmaps"])
        self.assertEqual(role["rules"][1]["resources"], ["rolebindings", "clusterrolebindings"])
        self.assertTrue({"create", "update", "patch"} <= set(role["rules"][1]["verbs"]))

    def test_restricted_workload_and_service_targets(self):
        finance = self.by_key["Deployment", "finance-ledger", "ledger-web"]
        spec = finance["spec"]["template"]["spec"]
        self.assertTrue(spec["securityContext"]["runAsNonRoot"])
        self.assertEqual(spec["securityContext"]["seccompProfile"]["type"], "RuntimeDefault")
        self.assertEqual(spec["containers"][0]["securityContext"]["capabilities"]["drop"], ["ALL"])
        for name, namespace in [("portal", "secure-portal"), ("north", "private-mesh"),
                                ("south", "private-mesh"), ("shop-web", "storefront")]:
            self.assertEqual(self.by_key["Service", namespace, name]["spec"]["selector"], {"app": name})


if __name__ == "__main__":
    unittest.main()
