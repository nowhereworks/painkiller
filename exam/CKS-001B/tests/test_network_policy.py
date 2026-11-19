import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
from network_policy import permits, selector_matches


def pod(namespace, labels, ip="10.244.0.5", namespace_labels=None):
    return {"namespace": namespace, "pod_labels": labels, "ip": ip,
            "namespace_labels": namespace_labels or {}}


def policy(namespace, spec):
    return {"metadata": {"namespace": namespace}, "spec": spec}


class NetworkPolicyTests(unittest.TestCase):
    def setUp(self):
        self.front = pod("market", {"app": "front"})
        self.backend = pod("market", {"app": "backend"})
        self.monitor = pod("observe", {"app": "agent"},
                           namespace_labels={"kubernetes.io/metadata.name": "observe"})
        self.outside = pod("other", {"app": "backend"})
        self.external = {"ip": "198.51.100.25", "namespace": None}

    def test_selectors_and_invalid_operator(self):
        self.assertTrue(selector_matches({"matchExpressions": [
            {"key": "missing", "operator": "NotIn", "values": ["x"]},
            {"key": "app", "operator": "In", "values": ["front"]},
            {"key": "app", "operator": "Exists"},
            {"key": "missing", "operator": "DoesNotExist"}]}, self.front["pod_labels"]))
        with self.assertRaises(ValueError):
            selector_matches({"matchExpressions": [{"key": "x", "operator": "Unknown"}]}, {})

    def test_ingress_namespace_and_pod_selector_and_port(self):
        rules = [policy("market", {
            "podSelector": {"matchLabels": {"app": "front"}}, "policyTypes": ["Ingress"],
            "ingress": [{"from": [
                {"podSelector": {"matchLabels": {"app": "backend"}}},
                {"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "observe"}}}],
                "ports": [{"port": 8080}]}]})]
        self.assertTrue(permits(rules, "Ingress", self.front, self.backend, "TCP", 8080))
        self.assertTrue(permits(rules, "Ingress", self.front, self.monitor, "TCP", 8080))
        self.assertFalse(permits(rules, "Ingress", self.front, self.outside, "TCP", 8080))
        self.assertFalse(permits(rules, "Ingress", self.front, self.monitor, "UDP", 8080))
        self.assertFalse(permits(rules, "Ingress", self.front, self.monitor, "TCP", 8081))
        self.assertTrue(permits(rules, "Egress", self.front, self.external, "TCP", 80))

    def test_combined_selectors_are_and_not_or(self):
        rules = [policy("market", {"podSelector": {}, "ingress": [{"from": [{
            "namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "observe"}},
            "podSelector": {"matchLabels": {"app": "agent"}}}]}]})]
        self.assertTrue(permits(rules, "Ingress", self.front, self.monitor, "TCP", 80))
        self.assertFalse(permits(rules, "Ingress", self.front, self.backend, "TCP", 80))
        wrong = pod("observe", {"app": "wrong"}, namespace_labels=self.monitor["namespace_labels"])
        self.assertFalse(permits(rules, "Ingress", self.front, wrong, "TCP", 80))

    def test_egress_except_dns_and_additive_bypass(self):
        rules = [policy("market", {"podSelector": {}, "policyTypes": ["Egress"], "egress": [
            {"to": [{"ipBlock": {"cidr": "0.0.0.0/0", "except": ["198.51.100.0/24"]}}]},
            {"to": [{"ipBlock": {"cidr": "203.0.113.53/32"}}],
             "ports": [{"protocol": "TCP", "port": 53}, {"protocol": "UDP", "port": 53}]}]})]
        self.assertFalse(permits(rules, "Egress", self.front, self.external, "TCP", 443))
        dns = {"ip": "203.0.113.53", "namespace": None}
        self.assertTrue(permits(rules, "Egress", self.front, dns, "UDP", 53))
        self.assertTrue(permits(rules, "Egress", self.front, dns, "TCP", 53))
        rules.append(policy("market", {"podSelector": {}, "policyTypes": ["Egress"], "egress": [{}]}))
        self.assertTrue(permits(rules, "Egress", self.front, self.external, "TCP", 443))

    def test_port_range_and_named_port_ambiguity(self):
        rules = [policy("market", {"podSelector": {}, "policyTypes": ["Egress"],
                                   "egress": [{"ports": [{"port": 8000, "endPort": 8002}]}]})]
        self.assertTrue(permits(rules, "Egress", self.front, self.external, "TCP", 8001))
        self.assertFalse(permits(rules, "Egress", self.front, self.external, "TCP", 8003))
        rules[0]["spec"]["egress"][0]["ports"][0] = {"port": "web"}
        with self.assertRaises(ValueError):
            permits(rules, "Egress", self.front, self.external, "TCP", 8001)

    def test_egress_policy_does_not_implicitly_isolate_ingress(self):
        # API defaulting: ingress is always included unless policyTypes is explicit.
        rules = [policy("market", {"podSelector": {}, "egress": [{}]})]
        self.assertFalse(permits(rules, "Ingress", self.front, self.backend, "TCP", 80))
        self.assertTrue(permits(rules, "Egress", self.front, self.external, "TCP", 80))
        rules[0]["spec"]["egress"] = []
        self.assertTrue(permits(rules, "Egress", self.front, self.external, "TCP", 80))
        rules[0]["spec"]["policyTypes"] = ["Egress"]
        self.assertFalse(permits(rules, "Egress", self.front, self.external, "TCP", 80))
        self.assertTrue(permits(rules, "Ingress", self.front, self.backend, "TCP", 80))


if __name__ == "__main__":
    unittest.main()
