import copy
import json
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
from fixture_objects import objects
from admission_ready import inspect
from rbac_capture import atoms, auth_check, capture, covers, save_baseline


class FakeRBAC:
    def __init__(self):
        self.cluster_roles = [copy.deepcopy(o) for o in objects() if o["kind"] == "ClusterRole"]
        self.cluster_bindings = [copy.deepcopy(o) for o in objects() if o["kind"] == "ClusterRoleBinding"]
        self.roles = []
        self.bindings = []
        self.account = {"metadata": {"name": "pipeline-agent", "namespace": "delivery"}}

    def get(self, kind, name="", namespace=None, optional=False):
        if kind == "namespace":
            return {"metadata": {"labels": {"painkiller-shell/exam": "CKS-001B"}}}
        if kind == "clusterroles":
            return {"items": copy.deepcopy(self.cluster_roles)}
        if kind == "clusterrolebindings":
            return {"items": copy.deepcopy(self.cluster_bindings)}
        if kind == "serviceaccount":
            return copy.deepcopy(self.account)
        raise AssertionError(kind)

    def list_all(self, kind):
        if kind == "roles":
            return {"items": copy.deepcopy(self.roles)}
        if kind == "rolebindings":
            return {"items": copy.deepcopy(self.bindings)}
        raise AssertionError(kind)

    def secure(self):
        rule = self.cluster_roles[0]["rules"][1]
        rule["verbs"] = [v for v in rule["verbs"] if v not in ("create", "update", "patch")]


class RBACCaptureTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir="/tmp/opencode")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.state = self.root / "var/lib/painkiller/cks001b"
        self.state.mkdir(parents=True)
        (self.state / "authorized").touch()
        self.kube = FakeRBAC()
        save_baseline(self.kube, self.root)

    def test_baseline_is_unscored_solved_and_retry_safe(self):
        first = (self.state / "rbac-baseline.json").read_bytes()
        initial = capture(self.kube, self.root, review=lambda *_: False)
        self.assertFalse(any(value is True for key, value in initial.items() if key != "failures"))
        self.kube.secure()
        save_baseline(self.kube, self.root)
        self.assertEqual((self.state / "rbac-baseline.json").read_bytes(), first)
        self.assertEqual(capture(self.kube, self.root, review=lambda *_: False)["failures"], [])
        self.kube.account = {}
        self.assertFalse(capture(self.kube, self.root, review=lambda *_: False)["retained"])

    def test_additional_group_bound_permissions_and_namespace_escalation(self):
        self.kube.secure()
        self.kube.roles.append({"metadata": {"name": "extra", "namespace": "other"},
                                "rules": [{"apiGroups": [""], "resources": ["secrets"], "verbs": ["get"]}]})
        self.kube.bindings.append({"metadata": {"name": "extra", "namespace": "other"},
                                   "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "extra"},
                                   "subjects": [{"kind": "Group", "name": "system:serviceaccounts:delivery"}]})
        result = capture(self.kube, self.root, review=lambda *_: False)
        self.assertTrue(result["retained"])
        self.assertFalse(result["no_new"])
        self.kube.roles[0]["rules"] = [{"apiGroups": ["rbac.authorization.k8s.io"],
                                        "resources": ["rolebindings"], "verbs": ["patch"],
                                        "resourceNames": ["some-rolebinding"]}]
        result = capture(self.kube, self.root, review=lambda *_: False)
        self.assertFalse(result["rolebindings"])
        self.assertTrue(result["clusterrolebindings"])

    def test_removing_original_permission_is_not_retained(self):
        self.kube.secure()
        self.kube.cluster_roles[0]["rules"][0]["resources"].remove("pods")
        result = capture(self.kube, self.root, review=lambda *_: False)
        self.assertFalse(result["retained"])
        self.assertTrue(result["no_new"])

    def test_subject_groups_and_wildcard_coverage(self):
        self.assertTrue(covers(("*", "resource", "", "*", "get", ""),
                               ("other", "resource", "", "pods", "get", "name")))
        self.assertFalse(covers(("other", "resource", "", "pods", "get", "name"),
                                ("other", "resource", "", "pods", "get", "")))
        self.assertTrue(covers(("*", "resource", "", "*/logs", "get", ""),
                               ("other", "resource", "", "pods/logs", "get", "")))
        self.assertTrue(covers(("*", "url", "", "/api/*", "get", ""),
                               ("*", "url", "", "/api/v1", "get", "")))
        self.assertTrue(atoms(self.kube))

    def test_namespaced_binding_does_not_grant_nonresource_urls(self):
        self.kube.cluster_roles.append({"metadata": {"name": "url-role"},
                                        "rules": [{"nonResourceURLs": ["/sensitive"], "verbs": ["get"]}]})
        self.kube.bindings.append({"metadata": {"name": "url", "namespace": "delivery"},
                                   "roleRef": {"apiGroup": "rbac.authorization.k8s.io",
                                               "kind": "ClusterRole", "name": "url-role"},
                                   "subjects": [{"kind": "ServiceAccount", "name": "pipeline-agent",
                                                 "namespace": "delivery"}]})
        self.assertFalse(any(atom[1] == "url" and atom[3] == "/sensitive" for atom in atoms(self.kube)))

    def test_auth_review_denial_and_transport_classification(self):
        with patch("rbac_capture.subprocess.run", return_value=SimpleNamespace(returncode=1, stdout="no\n", stderr="")) as run:
            self.assertFalse(auth_check("rolebindings", "create"))
            args = run.call_args.args[0]
            self.assertIn("--all-namespaces", args)
            self.assertIn("--as-group=system:serviceaccounts:delivery", args)
        with patch("rbac_capture.subprocess.run", return_value=SimpleNamespace(
                returncode=1, stdout="", stderr="Error (NotFound)")):
            with self.assertRaisesRegex(RuntimeError, "missing supporting object") as error:
                auth_check("rolebindings", "create")
            self.assertNotIn("NotFound", str(error.exception))

    def test_unexpected_broad_initial_grant_and_corrupt_baseline_rejected(self):
        path = self.state / "rbac-baseline.json"
        path.unlink()
        self.kube.cluster_roles[0]["rules"][1]["verbs"] = ["*"]
        with self.assertRaisesRegex(RuntimeError, "unexpected broad"):
            save_baseline(self.kube, self.root)
        path.write_text(json.dumps({"version": 1, "grants": []}))
        with self.assertRaisesRegex(RuntimeError, "lacks original"):
            capture(self.kube, self.root, review=lambda *_: False)

    def test_readiness_checks_fault_and_fresh_witness(self):
        now = 100
        namespace = {"metadata": {"uid": "ns", "resourceVersion": "8", "labels": {}}}
        deployment = {"metadata": {"uid": "dep", "generation": 2}}
        evidence = {"time": now, "namespace_uid": "ns", "namespace_version": "8",
                    "deployment_uid": "dep", "deployment_generation": 2,
                    "tests": {"privileged": "accepted", "template": "accepted"}}
        (self.state / "psa-witness-installed").touch()
        (self.state / "psa-evidence.json").write_text(json.dumps(evidence))
        original = self.kube.get

        def get(kind, name="", namespace=None, optional=False):
            if kind == "namespace" and name == "finance-ledger":
                return namespace_data
            if kind == "deployment" and name == "ledger-web":
                return deployment
            return original(kind, name, namespace, optional)

        namespace_data = namespace
        with patch.object(self.kube, "get", side_effect=get):
            self.assertEqual(inspect(self.kube, self.root, now=101, active=lambda: True), [])
            self.kube.secure()
            self.assertTrue(any("intentional fault" in msg
                                for msg in inspect(self.kube, self.root, now=101, active=lambda: True)))
            self.assertTrue(any("stale" in msg
                                for msg in inspect(self.kube, self.root, now=131, active=lambda: True)))


if __name__ == "__main__":
    unittest.main()
