"""Offline contracts and grading regression tests; never contact the configured cluster."""
import copy
import ast
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
from types import SimpleNamespace
import unittest
from unittest.mock import patch

EXAM = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(EXAM / "assets"))
import capture
import common
import mutate
import webhook


class Definitions(unittest.TestCase):
    def test_generated_contracts(self):
        subprocess.run([sys.executable, "-B", str(EXAM / "build.py"), "--check"], check=True)
        tasks = json.loads((EXAM / "tasks.json").read_text())
        self.assertEqual(len(tasks), 15)
        self.assertEqual(sum(t["points"] for t in tasks), 100)
        self.assertEqual([t["question_id"] for t in tasks], [f"CKS-001-Q{i}" for i in range(1, 16)])
        self.assertEqual([t["points"] for t in tasks], [3, 3, 8, 4, 5, 14, 12, 5, 5, 8, 4, 5, 6, 10, 8])
        for t in tasks:
            self.assertEqual(sum(c["points"] for c in t["checks"]), t["points"])
            self.assertTrue(t["steps"] and t["explanation"])
            subprocess.run(["bash", "-n"], input=t["solution"], text=True, check=True)
            import re
            for code in re.findall(r"sudo python3 - <<'PY'\n(.*?)\nPY", t["solution"], re.S):
                ast.parse(code)
            self.assertNotIn("CKA-", t["resource"] + t["solution"])
        lab = json.loads((EXAM / "test.json").read_text())["lab"]
        self.assertEqual((lab["controllers"], lab["workers"], lab["template_id"]), (1, 0, "9199"))
        self.assertEqual(lab["cni"], "calico")

    def test_initial_fault_and_isolated_canaries(self):
        fixtures = json.loads((EXAM / "setup.yaml").read_text())["items"]
        ns = next(o for o in fixtures if o["kind"] == "Namespace" and o["metadata"]["name"] == "locked-down")
        self.assertEqual(ns["metadata"]["labels"]["pod-security.kubernetes.io/enforce"], "restricted")
        canary = next(o for o in fixtures if o["kind"] == "NetworkPolicy")
        self.assertEqual(canary["metadata"]["namespace"], "cks001-canary")
        self.assertEqual(canary["spec"]["ingress"], [])
        self.assertFalse(any(o["kind"] == "NetworkPolicy" and o["metadata"]["namespace"] == "catalog" for o in fixtures))


class ImagePolicy(unittest.TestCase):
    def test_deny_latest_across_repositories_and_implicit_tags(self):
        for image in ["nginx:latest", "nginx", "repo.example:5000/app:latest", "repo.example:5000/app"]:
            with self.subTest(image=image):
                self.assertFalse(webhook.allowed(image))

    def test_allow_versions_and_digests(self):
        for image in ["nginx:1.28", "repo.example:5000/app:stable", "nginx@sha256:" + "a" * 64]:
            self.assertTrue(webhook.allowed(image))


class Grading(unittest.TestCase):
    def test_audit_ordering_and_scope(self):
        spec = importlib.util.spec_from_file_location("cks001_build", EXAM / "build.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        good = copy.deepcopy(module.audit_policy)
        self.assertTrue(capture.policy_correct(good))
        for mutation in ["catchall-first", "wrong-level", "missing-patch", "wrong-namespace", "extra-request-body"]:
            bad = copy.deepcopy(good)
            if mutation == "catchall-first":
                bad["rules"].insert(0, {"level": "Metadata"})
            elif mutation == "wrong-level":
                bad["rules"][0]["level"] = "Request"
            elif mutation == "missing-patch":
                bad["rules"][1]["verbs"].remove("patch")
            elif mutation == "wrong-namespace":
                bad["rules"][1]["namespaces"] = ["other"]
            else:
                bad["rules"][-1]["level"] = "RequestResponse"
            with self.subTest(mutation=mutation):
                self.assertFalse(capture.policy_correct(bad))

    def test_sbom_requires_image_identity_and_curl_inventory(self):
        expected = {"packages": [{"name": "curlimages/curl:8.12.1", "externalRefs": [
            {"referenceType": "purl", "referenceLocator": "pkg:oci/curl@sha256:123"}]}]}
        doc = {"SPDXID": "SPDXRef-DOCUMENT", "spdxVersion": "SPDX-2.3", "documentNamespace": "https://example.test/sbom",
               "creationInfo": {"creators": ["Tool: bom"]},
               "packages": copy.deepcopy(expected["packages"]) + [{"name": "curl", "versionInfo": "8.12.1"}],
               "relationships": [{"spdxElementId": "SPDXRef-DOCUMENT", "relationshipType": "DESCRIBES", "relatedSpdxElement": "SPDXRef-image"}]}
        self.assertTrue(capture.sbom_correct(doc, expected))
        for key in ["relationships", "documentNamespace", "creationInfo"]:
            bad = copy.deepcopy(doc)
            del bad[key]
            self.assertFalse(capture.sbom_correct(bad, expected))
        bad = copy.deepcopy(doc)
        bad["packages"] = [{"name": "curl", "versionInfo": "8.12.1"}]
        self.assertFalse(capture.sbom_correct(bad, expected))
        bad = copy.deepcopy(doc)
        bad["packages"].pop()
        self.assertFalse(capture.sbom_correct(bad, expected))

    def test_review_exact_set_and_missing_answer_diagnostics(self):
        with patch.object(capture, "file_text", return_value=""):
            missing = capture.capture("q3")
        self.assertFalse(missing["review"])
        self.assertTrue(missing["failures"])
        right = "Dockerfile.release\nDockerfile.backup\njob-debug.yaml\npod-client.yaml\n"
        with patch.object(capture, "file_text", return_value=right):
            self.assertTrue(capture.capture("q3")["review"])
        with patch.object(capture, "file_text", return_value=right + "pod-safe.yaml\n"):
            self.assertFalse(capture.capture("q3")["review"])

    def test_secret_missing_answer_is_failure_not_transport_error(self):
        def fake_get(kind, name, ns, **kwargs):
            if kind == "secret":
                return {"data": {"BROKER_PASSWORD": "c2VjcmV0"}}
            return {}
        with patch.object(capture, "get", side_effect=fake_get), patch.object(Path, "is_file", return_value=False):
            result = capture.capture("q2")
        self.assertFalse(result["extracted"])
        self.assertFalse(result["mounted"])
        self.assertEqual(len(result["failures"]), 2)

    def test_supporting_transport_errors_are_not_missing_answers(self):
        with patch.object(common, "k", return_value=SimpleNamespace(returncode=1, stdout="", stderr="Error from server (NotFound)")):
            with self.assertRaisesRegex(RuntimeError, "supporting exec transport") as caught:
                common.exec_probe("ns", "pod", ["true"])
            self.assertNotIn("NotFound", str(caught.exception))
        with patch.object(common, "k", return_value=SimpleNamespace(returncode=1, stdout="", stderr="command terminated with exit code 1")):
            self.assertEqual(common.exec_probe("ns", "pod", ["false"]).returncode, 1)

    def test_janitor_changes_only_identity_related_fields(self):
        initial = {"replicas": 1, "template": {"metadata": {"labels": {"app": "janitor"}},
                   "spec": {"containers": [{"name": "janitor", "image": "busybox:1.37.0"}]}}}
        secured = copy.deepcopy(initial)
        spec = secured["template"]["spec"]
        spec.update(serviceAccountName="job-agent", automountServiceAccountToken=False,
                    volumes=[{"name": "identity", "projected": {"sources": [{"serviceAccountToken": {"path": "token"}}]}}])
        spec["containers"][0]["volumeMounts"] = [{"name": "identity", "mountPath": "/var/run/job-identity", "readOnly": True}]
        self.assertEqual(capture.normalized_janitor(initial), capture.normalized_janitor(secured))
        secured["replicas"] = 2
        self.assertNotEqual(capture.normalized_janitor(initial), capture.normalized_janitor(secured))

    def test_scaled_to_zero_is_not_a_running_workload(self):
        self.assertFalse(common.ready({"kind": "Deployment", "spec": {"replicas": 0},
                                       "status": {"readyReplicas": 0}}))
        self.assertTrue(common.ready({"kind": "Deployment", "spec": {"replicas": 1},
                                      "status": {"readyReplicas": 1, "updatedReplicas": 1}}))

    def test_old_rollouts_and_terminating_pods_are_not_ready_answers(self):
        self.assertFalse(common.ready({"kind": "Deployment", "metadata": {"generation": 2},
                                       "spec": {"replicas": 1}, "status": {
                                           "observedGeneration": 1, "readyReplicas": 1, "updatedReplicas": 1}}))
        self.assertFalse(common.ready({"kind": "Pod", "metadata": {"deletionTimestamp": "2026-11-17T12:00:00Z"},
                                       "status": {"conditions": [{"type": "Ready", "status": "True"}]}}))

    def test_static_pod_solutions_wait_for_the_new_command(self):
        tasks = json.loads((EXAM / "tasks.json").read_text())
        for index in [4, 13, 14]:
            self.assertIn("expected[name]", tasks[index]["solution"])
            self.assertIn("Updated static Pods did not become Ready", tasks[index]["solution"])
            self.assertIn("with_name('.'+p.name+'.tmp')", tasks[index]["solution"])
        for index in [13, 14]:
            self.assertIn("['kube-apiserver', 'kube-controller-manager', 'kube-scheduler']", tasks[index]["solution"])

    def test_mount_only_static_changes_wait_for_the_new_process(self):
        wanted = {"containers": [{"name": "kube-apiserver", "command": ["kube-apiserver"],
                                 "volumeMounts": [{"name": "gate", "mountPath": "/gate", "readOnly": False}]}],
                  "volumes": [{"name": "gate", "hostPath": {"path": "/root/gate"}}]}
        good = {"kind": "Pod", "metadata": {"name": "kube-apiserver-node"}, "spec": copy.deepcopy(wanted),
                "status": {"conditions": [{"type": "Ready", "status": "True"}],
                           "containerStatuses": [{"state": {"running": {"startedAt": "2026-11-17T12:00:02Z"}}}]}}
        del good["spec"]["containers"][0]["volumeMounts"][0]["readOnly"]
        old_mount = copy.deepcopy(good)
        old_mount["spec"]["containers"][0]["volumeMounts"] = []
        old_process = copy.deepcopy(good)
        old_process["status"]["containerStatuses"][0]["state"]["running"]["startedAt"] = "2026-11-17T11:00:00Z"
        from datetime import datetime
        written = datetime.fromisoformat("2026-11-17T12:00:00Z").timestamp()
        responses = [SimpleNamespace(returncode=0, stdout=json.dumps({"items": [p]}))
                     for p in [old_mount, old_process, good]]
        with patch.object(common, "yaml_file", return_value={"spec": wanted}), \
                patch.object(Path, "stat", return_value=SimpleNamespace(st_mtime=written)), \
                patch.object(common, "k", side_effect=responses) as probe, patch("time.sleep"):
            common.wait_static(["kube-apiserver"])
        self.assertEqual(probe.call_count, 3)

    def test_mutation_deletion_allows_normal_pod_grace_period(self):
        with patch.object(mutate, "k") as command, patch.object(mutate, "apply"), \
                patch.object(mutate, "original", return_value={}):
            mutate.replace_pod("connector", "relay")
        deletion = command.call_args_list[0]
        self.assertIn("--timeout=90s", deletion.args)
        self.assertGreaterEqual(deletion.kwargs["timeout"], 90)

    def test_audit_evidence_survives_rotation(self):
        with tempfile.TemporaryDirectory() as directory:
            current = Path(directory) / "security-audit.log"
            previous = current.with_name("security-audit-2026-11-17T15-10-38.750.log")
            previous.write_text('partial JSON line\n' + json.dumps({"level": "Request", "verb": "patch"}) + "\n")
            current.write_text(json.dumps({"level": "Metadata", "verb": "get"}) + "\n")
            events = capture.audit_events(str(current))
        self.assertEqual({e["verb"] for e in events}, {"get", "patch"})

    def test_api_mutations_wait_for_all_components_and_new_witness_evidence(self):
        good = {"time": 101, "tests": {key: {"code": 1, "error": "image-gate: latest is forbidden"}
                                      for key in ["latest", "other", "implicit"]}}
        good["tests"]["tagged"] = {"code": 0}
        transient = copy.deepcopy(good)
        transient["tests"]["tagged"] = {"code": 1, "error": "backend unavailable"}
        with patch.object(mutate, "wait_static") as wait, patch.object(mutate, "k"), \
                patch.object(mutate, "flags", return_value=["kube-apiserver", "--enable-admission-plugins=ImagePolicyWebhook"]), \
                patch.object(Path, "read_text", side_effect=[json.dumps(transient), json.dumps(good)]) as evidence, \
                patch("time.time", return_value=100), patch("time.sleep"):
            mutate.wait_api()
        wait.assert_called_once_with(["kube-apiserver", "kube-controller-manager", "kube-scheduler"])
        self.assertEqual(evidence.call_count, 2)

    def test_security_flags_come_from_the_running_container(self):
        capture.live_flags.cache_clear()
        responses = [SimpleNamespace(stdout=json.dumps({"containers": [
            {"id": "live-container", "metadata": {"name": "kube-scheduler"}}]})),
            SimpleNamespace(stdout=json.dumps({"info": {"runtimeSpec": {"process": {
                "args": ["kube-scheduler", "--profiling=false", "--leader-elect=false"]}}}}))]
        with patch.object(capture, "run", side_effect=responses) as command:
            self.assertEqual(capture.live_flags("kube-scheduler")["profiling"], "false")
            self.assertEqual(capture.live_flags("kube-scheduler")["leader-elect"], "false")
        self.assertEqual(command.call_count, 2)
        capture.live_flags.cache_clear()

    def test_equivalent_network_selectors(self):
        for selector in [{"matchLabels": {"app": "catalog-web"}}, {"matchExpressions": [
                {"key": "app", "operator": "In", "values": ["catalog-web"]}]}]:
            self.assertTrue(capture.selector_matches(selector, {"app": "catalog-web"}))
            self.assertFalse(capture.selector_matches(selector, {"app": "other"}))

    def test_effective_rbac_ignores_only_default_self_reviews(self):
        def fake_get(kind, name="", ns=None, **kwargs):
            if kind == "role":
                return {"kind": "Role"}
            return {"roleRef": {"name": "alex-pods"}}

        def fake_k(*args, **kwargs):
            if args[:3] == ("auth", "can-i", "get"):
                return SimpleNamespace(returncode=1, stdout="no", stderr="")
            namespace = json.loads(kwargs["data"])["spec"]["namespace"]
            rules = [{"apiGroups": [""], "resources": ["pods"], "verbs":
                      ["get", "list"] if namespace == "team-c" else ["*"]},
                     {"apiGroups": ["authorization.k8s.io"],
                      "resources": ["selfsubjectaccessreviews", "selfsubjectrulesreviews"], "verbs": ["create"]},
                     {"apiGroups": ["authentication.k8s.io"], "resources": ["selfsubjectreviews"], "verbs": ["create"]}]
            return SimpleNamespace(returncode=0, stdout=json.dumps({"status": {"resourceRules": rules}}), stderr="")

        with patch.object(capture, "get", side_effect=fake_get), patch.object(capture, "k", side_effect=fake_k):
            result = capture.capture("q13")
        self.assertTrue(result["write"])
        self.assertTrue(result["read"])

    def test_admission_zero_is_default_cache_not_disabled(self):
        for ttl, expected in [(-1, True), (0, False), (30, False)]:
            config = {"plugins": [{"name": "ImagePolicyWebhook", "configuration": {"imagePolicy": {
                "kubeConfigFile": "/etc/admission-controllers/admission-kubeconfig.yaml",
                "defaultAllow": False, "allowTTL": ttl}}}]}
            evidence = {"time": time.time(), "tests": {
                key: {"code": 1, "error": "image-gate: latest is forbidden"} for key in ["latest", "other", "implicit"]}}
            evidence["tests"]["tagged"] = {"code": 0, "error": ""}

            def text(path):
                if str(path).endswith("admission-configuration.yaml"):
                    return json.dumps(config)
                if str(path).endswith("admission-evidence.json"):
                    return json.dumps(evidence)
                return ""

            command = ["kube-apiserver", "--enable-admission-plugins=NodeRestriction,ImagePolicyWebhook",
                       "--admission-control-config-file=/etc/admission-controllers/admission-configuration.yaml"]
            with patch.object(capture, "file_text", side_effect=text), patch.object(capture, "flags", return_value=command), \
                    patch.object(capture, "running_manifest", return_value=True), patch.object(capture, "get", return_value={}):
                result = capture.capture("q15")
            self.assertEqual(result["admission"], expected)
            self.assertEqual(result["rejected"], expected)


if __name__ == "__main__":
    unittest.main()
