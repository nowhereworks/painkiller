import copy
import json
import subprocess
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
sys.path.insert(0, str(ROOT / "assets"))
from network_capture import capture, checked_probe, egress_semantics, partition_samples
from network_server import ADDRESSES
from solutions import egress_exclusions, ingress, restricted_egress
from test_workload_capture import FakeKube, fixture, pod_ready


def observed(mode):
    values = {}
    for address in ADDRESSES:
        for port in (8081, 443, 9443):
            if mode == "control":
                allowed = True
            elif mode == "q2":
                allowed = address not in ADDRESSES[:2]
            else:
                allowed = port == 443 or port == 9443 and address == "198.18.40.10"
            values[f"{address}:{port}"] = allowed
    values.update(dns_tcp=True, dns_udp=True)
    return values


def fake_probe(mode):
    return lambda namespace, dns, name: observed("control" if name == "net-control" else mode)


def source(namespace):
    pod = pod_ready(fixture("Pod", namespace, "net-observer"))
    return {"namespace": namespace, "pod_labels": pod["metadata"]["labels"]}


class NetworkCaptureTests(unittest.TestCase):
    def setUp(self):
        self.kube = FakeKube()
        self.kube.items["service", "kube-system", "kube-dns"] = {"spec": {"clusterIP": "10.96.0.10"}}

    def egress_fixture(self, question, policy):
        namespace, name = ("outbound-guard", "exclude-ranges") if question == "q2" else (
            "api-clients", "allow-service-egress")
        self.kube.items["networkpolicy", namespace, name] = copy.deepcopy(policy)
        self.kube.items["networkpolicies", namespace, ""] = {"items": [copy.deepcopy(policy)]}
        self.kube.items["pods", namespace, ""] = {"items": [pod_ready(fixture("Pod", namespace, "net-observer"))]}
        return namespace

    def test_reference_egress_and_initial_no_points(self):
        for q, policy in (("q2", egress_exclusions), ("q13", restricted_egress)):
            with self.subTest(q=q):
                namespace = self.egress_fixture(q, policy)
                self.assertEqual(capture(q, self.kube, fake_probe(q))["failures"], [])
                name = policy["metadata"]["name"]
                del self.kube.items["networkpolicy", namespace, name]
                result = capture(q, self.kube, fake_probe(q))
                self.assertFalse(any(value is True for key, value in result.items() if key != "failures"))

    def test_additive_bypass_even_if_sample_probes_lie(self):
        for q, policy in (("q2", egress_exclusions), ("q13", restricted_egress)):
            with self.subTest(q=q):
                ns = self.egress_fixture(q, policy)
                extra = {"metadata": {"name": "bypass", "namespace": ns}, "spec": {
                    "podSelector": {}, "policyTypes": ["Egress"],
                    "egress": [{"to": [{"ipBlock": {"cidr": "198.18.20.77/32"}}],
                                "ports": [{"port": 22, "protocol": "TCP"}]}]}}
                self.kube.items["networkpolicies", ns, ""]["items"].append(extra)
                result = capture(q, self.kube, fake_probe(q))
                self.assertFalse(result["blocked" if q == "q2" else "denied"])

    def test_positive_control_failure_is_not_a_denied_answer(self):
        self.egress_fixture("q2", egress_exclusions)
        broken = lambda ns, dns, name: {} if name == "net-control" else observed("q2")
        with self.assertRaisesRegex(RuntimeError, "independent network control"):
            capture("q2", self.kube, broken)
        failure = subprocess.CalledProcessError(1, ["kubectl"], stderr="Error from server (NotFound): observer")
        with patch("network_capture.pod_probe", side_effect=failure):
            with self.assertRaisesRegex(RuntimeError, "missing supporting object") as error:
                checked_probe("outbound-guard", "10.96.0.10", "net-observer")
            self.assertNotIn("NotFound", str(error.exception))

    def test_dns_and_policy_scope(self):
        namespace = self.egress_fixture("q13", restricted_egress)
        probes = observed("q13")
        probes["dns_tcp"] = False
        result = capture("q13", self.kube,
                         lambda ns, dns, name: observed("control") if name == "net-control" else probes)
        self.assertFalse(result["dns"])
        self.kube.items["networkpolicy", namespace, "allow-service-egress"]["spec"]["policyTypes"].append("Ingress")
        self.assertFalse(capture("q13", self.kube, fake_probe("q13"))["policy"])

    def test_other_ingress_policy_is_detected(self):
        namespace = self.egress_fixture("q2", egress_exclusions)
        self.kube.items["networkpolicies", namespace, ""]["items"].append({
            "metadata": {"namespace": namespace}, "spec": {"podSelector": {}, "policyTypes": ["Ingress"]}})
        self.assertFalse(capture("q2", self.kube, fake_probe("q2"))["scope"])

    def test_exact_partitions_and_complexity_limit(self):
        policies = [copy.deepcopy(egress_exclusions)]
        policy = {"metadata": {"namespace": "outbound-guard"}, "spec": {"podSelector": {},
                  "policyTypes": ["Egress"], "egress": [{"ports": [{"port": "named"}]}]}}
        with self.assertRaisesRegex(RuntimeError, "named policy ports"):
            partition_samples(policies + [policy])
        with self.assertRaisesRegex(RuntimeError, "too many policies"):
            partition_samples(policies * 41)
        self.assertEqual(egress_semantics(policies, "outbound-guard", [source("outbound-guard")], "q2"),
                         {"blocked": True, "allowed": True})

    def test_ingress_allowed_blocked_and_additive_bypass(self):
        self.kube.items["networkpolicy", "storefront", "web-access"] = copy.deepcopy(ingress)
        self.kube.items["networkpolicies", "storefront", ""] = {"items": [copy.deepcopy(ingress)]}
        for ns, name in (("storefront", "shop-web"), ("storefront", "shop-api"),
                         ("storefront", "untrusted"), ("telemetry", "observer"), ("outbound-guard", "client")):
            obj = pod_ready(fixture("Pod", ns, name))
            obj["status"]["podIP"] = "10.244.0.10"
            self.kube.items["pod", ns, name] = obj
            self.kube.items["namespace", None, ns] = {
                "metadata": {"labels": {"kubernetes.io/metadata.name": ns}}}
        for ns, name in (("storefront", "untrusted"), ("outbound-guard", "client")):
            self.kube.exec_outputs[ns, name, "wget"] = SimpleNamespace(returncode=1, stdout="")
        self.assertEqual(capture("q4", self.kube)["failures"], [])
        self.kube.items["networkpolicies", "storefront", ""]["items"].append({
            "metadata": {"namespace": "storefront"}, "spec": {
                "podSelector": {}, "policyTypes": ["Ingress"], "ingress": [{}]}})
        self.assertFalse(capture("q4", self.kube)["denied"])
        self.kube.items["networkpolicies", "storefront", ""]["items"].append({
            "metadata": {"namespace": "storefront"}, "spec": {
                "podSelector": {}, "policyTypes": ["Egress"], "egress": []}})
        self.assertFalse(capture("q4", self.kube)["scope"])


if __name__ == "__main__":
    unittest.main()
