import copy
import json
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
sys.path.insert(0, str(ROOT / "assets"))
import ingress_capture
import mesh_capture
from ingress_fixture import NAME, NS
from solutions import peer_authentication, tls_ingress
from test_workload_capture import FakeKube, fixture, pod_ready


def deployment(name, namespace, uid, revision):
    dep = fixture("Deployment", namespace, name)
    dep["metadata"].update({"uid": uid, "generation": 1,
                            "annotations": {"deployment.kubernetes.io/revision": revision}})
    dep["status"] = {"observedGeneration": 1, "updatedReplicas": 1, "readyReplicas": 1, "availableReplicas": 1}
    return dep


class IngressCaptureTests(unittest.TestCase):
    def setUp(self):
        self.kube = FakeKube()
        self.kube.items["ingress", "secure-portal", "portal-tls"] = copy.deepcopy(tls_ingress)
        self.kube.items["service", "secure-portal", "portal"] = {"spec": {"ports": [{"name": "web", "port": 80}]}}
        self.kube.items["deployment", NS, NAME] = {
            "metadata": {"generation": 1}, "spec": {"replicas": 1},
            "status": {"observedGeneration": 1, "updatedReplicas": 1, "readyReplicas": 1, "availableReplicas": 1}}
        self.kube.items["service", NS, NAME] = {"spec": {"type": "NodePort", "ports": [
            {"name": "http", "nodePort": 30080}, {"name": "https", "nodePort": 30443}]}}
        self.kube.items["nodes", None, ""] = {"items": [{
            "status": {"addresses": [{"type": "InternalIP", "address": "192.0.2.1"}]}}]}

    def test_reference_and_named_service_port(self):
        calls = []

        def traffic(*args):
            calls.append(args)
            return True, True

        self.assertEqual(ingress_capture.capture(self.kube, Path("/"), traffic)["failures"], [])
        self.assertEqual(len(calls), 2)
        self.assertEqual(calls[1][-1], "portal.training.example")
        obj = self.kube.items["ingress", "secure-portal", "portal-tls"]
        obj["spec"]["rules"][0]["http"]["paths"][0]["backend"]["service"]["port"] = {"name": "web"}
        self.assertTrue(ingress_capture.capture(self.kube, Path("/"), traffic)["route"])

    def test_missing_and_bad_answer_never_use_control_as_candidate(self):
        self.kube.items.clear()
        result = ingress_capture.capture(self.kube, traffic=lambda *_: self.fail("unexpected probe"))
        self.assertFalse(any(value is True for key, value in result.items() if key != "failures"))
        self.setUp()
        obj = self.kube.items["ingress", "secure-portal", "portal-tls"]
        obj["metadata"]["annotations"]["nginx.ingress.kubernetes.io/ssl-redirect"] = "false"
        obj["spec"]["rules"][0]["host"] = "wrong.training.example"
        result = ingress_capture.capture(self.kube, traffic=lambda *_: self.fail("unexpected probe"))
        self.assertFalse(result["annotations"])
        self.assertFalse(result["route"])
        self.assertFalse(result["https"])

    def test_broken_canary_is_error_not_denial(self):
        with self.assertRaisesRegex(RuntimeError, "independent ingress canary"):
            ingress_capture.capture(self.kube, traffic=lambda *_: (False, False))
        calls = iter([(True, True), (False, False)])
        result = ingress_capture.capture(self.kube, traffic=lambda *_: next(calls))
        self.assertFalse(result["redirect"])
        self.assertFalse(result["https"])


class MeshCaptureTests(unittest.TestCase):
    def setUp(self):
        self.kube = FakeKube()
        self.kube.items["peerauthentication", "private-mesh", "default"] = copy.deepcopy(peer_authentication)
        sets, pods = [], []
        for name in ("north", "south"):
            uid, rsuid = "deployment-" + name, "rs-" + name
            self.kube.items["deployment", "private-mesh", name] = deployment(name, "private-mesh", uid, "2")
            sets.append({"metadata": {"uid": rsuid, "annotations": {"deployment.kubernetes.io/revision": "2"},
                                      "ownerReferences": [{"uid": uid, "controller": True}]}})
            obj = pod_ready({"metadata": {"name": name + "-pod", "ownerReferences": [
                {"uid": rsuid, "controller": True}]}, "spec": {"containers": [
                    {"name": name}, {"name": "istio-proxy"}]}})
            obj["status"].update({"podIP": "10.244.0.15", "containerStatuses": [
                {"name": "istio-proxy", "ready": True, "state": {"running": {}}}]})
            pods.append(obj)
        self.kube.items["replicasets", "private-mesh", ""] = {"items": sets}
        self.kube.items["pods", "private-mesh", ""] = {"items": pods}
        self.kube.items["deployment", "istio-system", "istiod"] = {
            "metadata": {"generation": 1}, "spec": {"replicas": 1},
            "status": {"observedGeneration": 1, "updatedReplicas": 1, "readyReplicas": 1, "availableReplicas": 1}}
        self.kube.items["pod", "cks001b-system", "net-control"] = pod_ready(
            fixture("Pod", "cks001b-system", "net-control"))
        self.kube.items["service", "secure-portal", "portal"] = {"spec": {"clusterIP": "10.96.0.17"}}
        self.kube.exec_outputs["cks001b-system", "net-control", "python3"] = SimpleNamespace(
            returncode=0, stdout=json.dumps({"control": True, "plaintext": False}))

    def test_strict_and_traffic_success(self):
        self.assertEqual(mesh_capture.capture(self.kube)["failures"], [])
        self.assertTrue(any(call[0] == "exec" and call[1] == "private-mesh" and
                            call[2] == "north-pod" for call in self.kube.calls))

    def test_namespace_scope_and_override(self):
        obj = self.kube.items["peerauthentication", "private-mesh", "default"]
        obj["spec"]["selector"] = {"matchLabels": {"app": "north"}}
        self.assertFalse(mesh_capture.capture(self.kube)["strict"])
        del obj["spec"]["selector"]
        obj["spec"]["portLevelMtls"] = {"80": {"mode": "DISABLE"}}
        self.assertFalse(mesh_capture.capture(self.kube)["strict"])

    def test_plaintext_failure_and_broken_control_are_distinct(self):
        self.kube.exec_outputs["cks001b-system", "net-control", "python3"].stdout = json.dumps(
            {"control": True, "plaintext": True})
        self.assertFalse(mesh_capture.capture(self.kube)["plaintext_blocked"])
        self.kube.exec_outputs["cks001b-system", "net-control", "python3"].stdout = json.dumps(
            {"control": False, "plaintext": False})
        with self.assertRaisesRegex(RuntimeError, "independent plaintext control"):
            mesh_capture.capture(self.kube)

    def test_proxy_must_be_ready_and_current(self):
        self.kube.items["pods", "private-mesh", ""]["items"][0]["status"]["containerStatuses"][0]["ready"] = False
        result = mesh_capture.capture(self.kube)
        self.assertFalse(result["north_proxy"])
        self.assertFalse(result["communication"])
        self.kube.items["pods", "private-mesh", ""]["items"][0]["status"]["containerStatuses"][0]["ready"] = True
        self.kube.items["replicasets", "private-mesh", ""]["items"][0]["metadata"]["annotations"][
            "deployment.kubernetes.io/revision"] = "1"
        self.assertFalse(mesh_capture.capture(self.kube)["north_proxy"])

    def test_native_sidecar_supported(self):
        pod = self.kube.items["pods", "private-mesh", ""]["items"][0]
        proxy = pod["spec"]["containers"].pop()
        proxy["restartPolicy"] = "Always"
        pod["spec"]["initContainers"] = [proxy]
        pod["status"]["initContainerStatuses"] = pod["status"].pop("containerStatuses")
        self.assertTrue(mesh_capture.proxy_ready(pod))


if __name__ == "__main__":
    unittest.main()
