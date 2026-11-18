import base64
import copy
import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
from fixture_objects import objects
from workload_capture import Kubernetes, capture


class FakeKube:
    def __init__(self):
        self.items = {}
        self.exec_outputs = {}
        self.calls = []

    def get(self, kind, name="", namespace=None, optional=False):
        self.calls.append(("get", kind, name, namespace, optional))
        return copy.deepcopy(self.items.get((kind, namespace, name), {}))

    def exec(self, namespace, name, args, container=None):
        self.calls.append(("exec", namespace, name, args, container))
        response = self.exec_outputs.get((namespace, name, args[0]), SimpleNamespace(returncode=0, stdout=""))
        if isinstance(response, Exception):
            raise response
        return response


def fixture(kind, ns, name):
    return copy.deepcopy(next(o for o in objects() if o["kind"] == kind and
                              o["metadata"].get("namespace") == ns and o["metadata"]["name"] == name))


def pod_ready(pod):
    pod["status"] = {"conditions": [{"type": "Ready", "status": "True"}]}
    return pod


def deployment_ready(deploy, kube, namespace):
    name = deploy["metadata"]["name"]
    deploy["metadata"].update({"uid": "deploy-uid", "generation": 2,
                               "annotations": {"deployment.kubernetes.io/revision": "3"}})
    deploy["status"] = {"observedGeneration": 2, "updatedReplicas": 1, "readyReplicas": 1, "availableReplicas": 1}
    rs = {"metadata": {"uid": "rs-uid", "annotations": {"deployment.kubernetes.io/revision": "3"},
                       "ownerReferences": [{"uid": "deploy-uid", "controller": True}]}}
    pod = pod_ready({"metadata": {"name": name + "-abc", "ownerReferences": [
        {"uid": "rs-uid", "controller": True}]}})
    kube.items["deployment", namespace, name] = deploy
    kube.items["replicasets", namespace, ""] = {"items": [rs]}
    kube.items["pods", namespace, ""] = {"items": [pod]}
    return pod


class WorkloadCapturesTests(unittest.TestCase):
    def setUp(self):
        self.kube = FakeKube()
        self.tmp = tempfile.TemporaryDirectory(dir="/tmp/opencode")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)

    def write(self, filename, value):
        filename = self.root / filename
        filename.parent.mkdir(parents=True, exist_ok=True)
        filename.write_text(value)

    def test_q3_effective_settings_and_runtime(self):
        pod = pod_ready(fixture("Pod", "workload-identity", "web-gate"))
        pod["spec"]["securityContext"] = {"runAsNonRoot": True, "runAsUser": 101, "runAsGroup": 101}
        c = pod["spec"]["containers"][0]
        c.update({"image": "nginxinc/nginx-unprivileged:alpine", "ports": [{"containerPort": 8080}],
                  "securityContext": {"allowPrivilegeEscalation": False}})
        self.kube.items["pod", "workload-identity", "web-gate"] = pod
        self.kube.exec_outputs["workload-identity", "web-gate", "cat"] = SimpleNamespace(
            returncode=0, stdout="Uid:\t101\t101\t101\t101\nGid:\t101\t101\t101\t101\nNoNewPrivs:\t1\n")
        self.assertEqual(capture("q3", self.kube, self.root)["failures"], [])
        c["securityContext"]["runAsUser"] = 0
        self.assertFalse(capture("q3", self.kube, self.root)["uid"])
        self.kube.exec_outputs["workload-identity", "web-gate", "wget"] = SimpleNamespace(returncode=1, stdout="")
        self.assertFalse(capture("q3", self.kube, self.root)["serving"])

    def test_q6_mount_modes_and_current_revision(self):
        dep = fixture("Deployment", "immutable-workloads", "report-web")
        dep["spec"]["template"]["spec"]["containers"][0]["securityContext"] = {"readOnlyRootFilesystem": True}
        deployment_ready(dep, self.kube, "immutable-workloads")
        lines = ["1 0 0:1 / / ro,relatime - overlay overlay ro"]
        lines += [f"{i} 1 0:{i} / {mount} rw,nosuid - tmpfs tmpfs rw" for i, mount in
                  enumerate(("/tmp", "/var/log", "/var/cache/nginx", "/var/run"), 2)]
        self.kube.exec_outputs["immutable-workloads", "report-web-abc", "cat"] = SimpleNamespace(
            returncode=0, stdout="\n".join(lines))
        self.assertEqual(capture("q6", self.kube, self.root)["failures"], [])
        self.kube.exec_outputs["immutable-workloads", "report-web-abc", "cat"].stdout = "\n".join(lines[:-1])
        self.assertFalse(capture("q6", self.kube, self.root)["mounts"])
        self.kube.items["replicasets", "immutable-workloads", ""]["items"][0]["metadata"]["annotations"][
            "deployment.kubernetes.io/revision"] = "2"
        self.assertFalse(capture("q6", self.kube, self.root)["serving"])

    def test_q8_checks_both_pod_manifest_and_report(self):
        obj = pod_ready(fixture("Pod", "default", "invoice-web"))
        obj["spec"]["containers"][0]["securityContext"]["capabilities"] = {
            "drop": ["ALL"], "add": ["NET_BIND_SERVICE"]}
        self.kube.items["pod", "default", "invoice-web"] = obj
        self.write("root/CKS/invoice-web.yaml", json.dumps(obj))
        self.write("root/CKS/invoice-kubesec.json", json.dumps([{
            "object": "Pod/invoice-web.default", "valid": True, "score": 4, "message": "Passed with score 4"}]))
        self.assertEqual(capture("q8", self.kube, self.root)["failures"], [])
        self.write("root/CKS/invoice-web.yaml", "spec:\n  containers: [bad]\n")
        result = capture("q8", self.kube, self.root)
        self.assertFalse(result["drop"])
        self.assertFalse(result["add"])
        self.assertFalse(result["report"])
        self.write("root/CKS/invoice-web.yaml", '{"spec":{"containers":[{"securityContext":{"capabilities":{"drop":1}}}]}}')
        self.assertFalse(capture("q8", self.kube, self.root)["drop"])

    def test_q10_profile_and_runtime(self):
        self.write("var/lib/kubelet/seccomp/profiles/no-inspect.json", json.dumps({
            "defaultAction": "SCMP_ACT_ALLOW", "syscalls": [
                {"names": ["ptrace", "process_vm_readv"], "action": "SCMP_ACT_ERRNO"}]}))
        pod = pod_ready(fixture("Pod", "runtime-shield", "runtime-web"))
        pod["spec"]["securityContext"] = {"seccompProfile": {
            "type": "Localhost", "localhostProfile": "profiles/no-inspect.json"}}
        self.kube.items["pod", "runtime-shield", "runtime-web"] = pod
        self.kube.exec_outputs["runtime-shield", "runtime-web", "cat"] = SimpleNamespace(returncode=0, stdout="Seccomp:\t2\n")
        self.assertEqual(capture("q10", self.kube, self.root)["failures"], [])
        pod["spec"]["securityContext"]["seccompProfile"]["localhostProfile"] = "profiles/wrong.json"
        self.assertFalse(capture("q10", self.kube, self.root)["profile"])

    def test_q15_secret_and_actual_mounted_contents_without_disclosure(self):
        cert, key = "CERTIFICATE-CONTENT", "PRIVATE-KEY-CONTENT"
        self.write("root/portal-client.crt", cert)
        self.write("root/portal-client.key", key)
        self.kube.items["secret", "editor", "editor-cert"] = {"type": "kubernetes.io/tls", "data": {
            "tls.crt": base64.b64encode(cert.encode()).decode(), "tls.key": base64.b64encode(key.encode()).decode()}}
        dep = fixture("Deployment", "editor", "editor-web")
        spec = dep["spec"]["template"]["spec"]
        spec["volumes"] = [{"name": "editor-tls", "secret": {"secretName": "editor-cert"}}]
        spec["containers"][0]["volumeMounts"] = [{"name": "editor-tls", "mountPath": "/etc/editor/tls"}]
        deployment_ready(dep, self.kube, "editor")
        digest = lambda s: hashlib.sha256(s.encode()).hexdigest()
        self.kube.exec_outputs["editor", "editor-web-abc", "sha256sum"] = SimpleNamespace(
            returncode=0, stdout=f"{digest(cert)}  /etc/editor/tls/tls.crt\n{digest(key)}  /etc/editor/tls/tls.key\n")
        result = capture("q15", self.kube, self.root)
        self.assertEqual(result["failures"], [])
        self.assertNotIn(key, json.dumps(result))
        self.kube.exec_outputs["editor", "editor-web-abc", "sha256sum"].stdout = "bad /etc/editor/tls/tls.crt\n"
        self.assertFalse(capture("q15", self.kube, self.root)["mounted"])

    def test_missing_pod_and_exec_transport_error(self):
        self.assertFalse(any(value is True for key, value in capture("q3", self.kube, self.root).items()
                             if key != "failures"))
        result = SimpleNamespace(returncode=1, stderr="Error from server (NotFound): node missing", stdout="")
        with patch("workload_capture.command", return_value=result):
            with self.assertRaisesRegex(RuntimeError, "missing supporting object") as error:
                Kubernetes().exec("default", "fixture", ["cat", "/proc/1/status"])
            self.assertNotIn("NotFound", str(error.exception))
        with patch("workload_capture.command", side_effect=subprocess.TimeoutExpired(["kubectl"], 10)):
            with self.assertRaises(subprocess.TimeoutExpired):
                Kubernetes().get("pods", namespace="default")

    def test_initial_healthy_workloads_do_not_score(self):
        dep = fixture("Deployment", "immutable-workloads", "report-web")
        deployment_ready(dep, self.kube, "immutable-workloads")
        self.kube.exec_outputs["immutable-workloads", "report-web-abc", "cat"] = SimpleNamespace(
            returncode=0, stdout="1 0 0:1 / / rw,relatime - overlay overlay rw\n")
        invoice = pod_ready(fixture("Pod", "default", "invoice-web"))
        self.kube.items["pod", "default", "invoice-web"] = invoice
        self.write("root/CKS/invoice-web.yaml", json.dumps(invoice))
        runtime = pod_ready(fixture("Pod", "runtime-shield", "runtime-web"))
        self.kube.items["pod", "runtime-shield", "runtime-web"] = runtime
        self.kube.exec_outputs["runtime-shield", "runtime-web", "cat"] = SimpleNamespace(
            returncode=0, stdout="Seccomp:\t2\n")
        for question in ("q6", "q8", "q10"):
            with self.subTest(question=question):
                result = capture(question, self.kube, self.root)
                self.assertFalse(any(value is True for key, value in result.items() if key != "failures"))


if __name__ == "__main__":
    unittest.main()
