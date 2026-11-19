"""Read-only Q2/Q4/Q13 captures: policy semantics plus independent live probes."""
from ipaddress import IPv4Address, ip_network
import json
import subprocess
import sys

from network_policy import permits, selector_matches
from network_ready import pod_probe
from network_server import ADDRESSES
from workload_capture import Kubernetes, ready_pod

BLOCKED = tuple(ip_network(cidr) for cidr in ("198.18.20.0/24", "198.18.30.0/24"))
API = ip_network("198.18.40.0/24")


def all_selector(selector):
    return (isinstance(selector, dict) and not selector.get("matchLabels") and
            not selector.get("matchExpressions"))


def valid_egress_policy(policy):
    if not policy:
        return False
    spec = policy.get("spec", {})
    return (all_selector(spec.get("podSelector", {})) and
            spec.get("policyTypes") == ["Egress"])


def partition_samples(policies):
    """Represent every IPv4 CIDR and numeric port interval in these policies."""
    addresses = {0, 2**32}
    ports = {1, 65536, 53, 54, 443, 444, 9443, 9444}

    def add_cidr(text):
        network = ip_network(text)
        if network.version == 4:
            addresses.add(int(network.network_address))
            addresses.add(int(network.broadcast_address) + 1)

    for network in (*BLOCKED, API):
        add_cidr(str(network))
    if len(policies) > 40:
        raise RuntimeError("too many policies for bounded network capture")
    for policy in policies:
        for rule in policy.get("spec", {}).get("egress", []):
            for peer in rule.get("to", []):
                block = peer.get("ipBlock")
                if block:
                    add_cidr(block["cidr"])
                    for exception in block.get("except", []):
                        add_cidr(exception)
            for item in rule.get("ports", []):
                number = item.get("port")
                if number is None:
                    continue
                if not isinstance(number, int) or isinstance(number, bool):
                    raise RuntimeError("named policy ports cannot be resolved for external-address analysis")
                end = item.get("endPort", number)
                ports.add(number)
                ports.add(end + 1)
    if len(addresses) > 64 or len(ports) > 64:
        raise RuntimeError("policy has too many CIDR or port ranges for bounded network capture")
    return ([str(IPv4Address(n)) for n in sorted(addresses) if n < 2**32],
            [n for n in sorted(ports) if 1 <= n <= 65535])


def egress_semantics(policies, namespace, subjects, mode):
    """Check unions over external IPv4 ranges/ports; DNS is also probed live."""
    flags = {"blocked": True, "allowed": True} if mode == "q2" else {
        "dns": True, "https": True, "api": True, "denied": True}
    addresses, ports = partition_samples(policies)
    # Empty labels additionally catch policies that select only existing Pods.
    subjects = subjects + [{"namespace": namespace, "pod_labels": {}}]
    # Different Pods with identical labels have identical policy decisions.
    unique = {json.dumps(subject["pod_labels"], sort_keys=True): subject for subject in subjects}
    if len(unique) > 24:
        raise RuntimeError("too many distinct Pod label sets for bounded network capture")
    if len(addresses) * len(ports) * 3 * len(unique) * max(len(policies), 1) > 250000:
        raise RuntimeError("policy analysis exceeds the bounded capture budget")
    for subject in unique.values():
        for address in addresses:
            ip = IPv4Address(address)
            peer = {"ip": address, "namespace": None}
            for protocol in ("TCP", "UDP", "SCTP"):
                for port in ports:
                    actual = permits(policies, "Egress", subject, peer, protocol, port)
                    if mode == "q2":
                        excluded = any(ip in block for block in BLOCKED)
                        if actual == excluded:
                            flags["blocked" if excluded else "allowed"] = False
                    else:
                        dns = protocol in ("TCP", "UDP") and port == 53
                        https = protocol == "TCP" and port == 443
                        api = protocol == "TCP" and port == 9443 and ip in API
                        expected = dns or https or api
                        if actual != expected:
                            flags["dns" if dns else "https" if https else "api" if api else "denied"] = False
    return flags


def checked_probe(namespace, dns, name):
    try:
        result = pod_probe(namespace, dns, name)
    except subprocess.CalledProcessError as error:
        message = (error.stderr or "").replace("NotFound", "missing supporting object")
        raise RuntimeError("network probe transport failed: " + message) from None
    if not isinstance(result, dict):
        raise RuntimeError("network probe returned invalid data")
    return result


def egress_capture(question, kube, probe=checked_probe):
    ns, name = ("outbound-guard", "exclude-ranges") if question == "q2" else (
        "api-clients", "allow-service-egress")
    policy = kube.get("networkpolicy", name, ns, optional=True)
    valid = valid_egress_policy(policy)
    result = {"failures": []}

    def check(key, ok, message):
        result[key] = bool(ok)
        if not ok:
            result["failures"].append(message)

    check("policy", valid, f"Create {ns}/{name} selecting all pods for egress only.")
    if not valid:
        keys = ("scope", "blocked", "allowed", "dns") if question == "q2" else (
            "scope", "dns", "https", "api", "denied")
        for key in keys:
            check(key, False, "The named egress policy is not configured.")
        return result
    pods = kube.get("pods", namespace=ns).get("items", [])
    observer = next((p for p in pods if p.get("metadata", {}).get("name") == "net-observer"), {})
    if not ready_pod(observer):
        raise RuntimeError(f"supporting network observer in {ns} is unavailable")
    subjects = [{"namespace": ns, "pod_labels": p.get("metadata", {}).get("labels", {})} for p in pods
                if not p.get("metadata", {}).get("deletionTimestamp") and not p.get("spec", {}).get("hostNetwork")]
    policies = kube.get("networkpolicies", namespace=ns).get("items", [])
    def isolates_ingress(item, subject):
        policy_spec = item.get("spec", {})
        types = policy_spec.get("policyTypes")
        if types is None:
            types = ["Ingress"] + (["Egress"] if policy_spec.get("egress") else [])
        return "Ingress" in types and selector_matches(policy_spec.get("podSelector", {}), subject["pod_labels"])

    scope = not any(isolates_ingress(item, subject) for item in policies for subject in subjects)
    check("scope", scope, "Do not isolate ingress for pods in this namespace.")
    semantics = egress_semantics(policies, ns, subjects, question)
    dns = kube.get("service", "kube-dns", "kube-system")["spec"]["clusterIP"]
    # Control Pod lives in an independent namespace, so endpoint/CNI failures
    # cannot masquerade as the candidate's desired denied connections.
    control = probe("cks001b-system", dns, "net-control")
    expected = [f"{ip}:{port}" for ip in ADDRESSES for port in (8081, 443, 9443)] + ["dns_udp", "dns_tcp"]
    if not all(control.get(key) is True for key in expected):
        raise RuntimeError("independent network control cannot reach fixture endpoints or DNS")
    live = probe(ns, dns, "net-observer")
    if not all(isinstance(live.get(key), bool) for key in expected):
        raise RuntimeError("network observer returned incomplete probe data")
    if question == "q2":
        blocked = all(not live[f"{ip}:{port}"] for ip in ADDRESSES[:2] for port in (8081, 443, 9443))
        allowed = all(live[f"{ip}:{port}"] for ip in ADDRESSES[2:] for port in (8081, 443, 9443))
        check("blocked", semantics["blocked"] and blocked, "Block both excluded CIDRs on all egress ports.")
        check("allowed", semantics["allowed"] and allowed, "Allow other IPv4 egress.")
        check("dns", live["dns_tcp"] and live["dns_udp"], "Allow both TCP and UDP DNS.")
    else:
        check("dns", semantics["dns"] and live["dns_tcp"] and live["dns_udp"], "Allow TCP and UDP DNS.")
        check("https", semantics["https"] and all(live[f"{ip}:443"] for ip in ADDRESSES),
              "Allow TCP port 443.")
        check("api", semantics["api"] and live["198.18.40.10:9443"],
              "Allow TCP port 9443 to the specified API CIDR.")
        denied = all(not live[f"{ip}:8081"] for ip in ADDRESSES) and all(
            not live[f"{ip}:9443"] for ip in ADDRESSES if ip != "198.18.40.10")
        check("denied", semantics["denied"] and denied, "Block other egress.")
    return result


def ingress_capture(kube):
    result = {"failures": []}

    def check(key, ok, message):
        result[key] = bool(ok)
        if not ok:
            result["failures"].append(message)

    policy = kube.get("networkpolicy", "web-access", "storefront", optional=True)
    spec = policy.get("spec", {})
    selector = spec.get("podSelector", {})
    selected = bool(policy) and selector_matches(selector, {"app": "shop-web"}) and not any(
        selector_matches(selector, {"app": value}) for value in ("shop-api", "untrusted"))
    valid = selected and spec.get("policyTypes") == ["Ingress"]
    check("policy", valid, "Create web-access selecting only shop-web for ingress.")
    if not valid:
        check("scope", False, "Do not isolate egress from the frontend.")
        check("allowed", False, "Allow the specified clients to reach shop-web on TCP 80.")
        check("denied", False, "Block other ingress to shop-web.")
        return result

    names = (("storefront", "shop-web"), ("storefront", "shop-api"), ("storefront", "untrusted"),
             ("telemetry", "observer"), ("outbound-guard", "client"))
    pods = {(ns, name): kube.get("pod", name, ns) for ns, name in names}
    if not all(ready_pod(p) for p in pods.values()):
        raise RuntimeError("supporting ingress probe workloads are not Ready")
    target = pods["storefront", "shop-web"]
    target_ip = target.get("status", {}).get("podIP")
    if not target_ip:
        raise RuntimeError("supporting ingress target has no Pod IP")
    if kube.exec("storefront", "shop-web", ["wget", "-T", "2", "-qO-", "http://127.0.0.1/"]).returncode:
        raise RuntimeError("supporting frontend HTTP service is unavailable")
    policies = kube.get("networkpolicies", namespace="storefront").get("items", [])
    subject = {"namespace": "storefront", "pod_labels": target["metadata"].get("labels", {})}
    def isolates_egress(item):
        policy_spec = item.get("spec", {})
        types = policy_spec.get("policyTypes")
        if types is None:
            types = ["Ingress"] + (["Egress"] if policy_spec.get("egress") else [])
        return "Egress" in types and selector_matches(policy_spec.get("podSelector", {}), subject["pod_labels"])

    check("scope", not any(isolates_egress(item) for item in policies),
          "Do not isolate egress from the frontend.")
    namespace_labels = {ns: kube.get("namespace", ns).get("metadata", {}).get("labels", {})
                        for ns in ("storefront", "telemetry", "outbound-guard")}

    def model(ns, labels, port=80):
        return permits(policies, "Ingress", subject, {"namespace": ns, "namespace_labels": namespace_labels[ns],
                       "pod_labels": labels, "ip": "10.244.0.3"}, "TCP", port)

    expected_allowed = (model("storefront", {"app": "shop-api"}) and model("telemetry", {}) and
                        model("telemetry", {"app": "anything"}))
    expected_denied = (not model("storefront", {"app": "untrusted"}) and
                       not model("outbound-guard", {"app": "client"}) and
                       not model("telemetry", {}, 81))
    probes = {}
    for ns, name in names[1:]:
        probe = kube.exec(ns, name, ["wget", "-T", "2", "-qO-", f"http://{target_ip}/"])
        probes[ns, name] = probe.returncode == 0
    check("allowed", expected_allowed and probes["storefront", "shop-api"] and probes["telemetry", "observer"],
          "Allow shop-api and telemetry pods to reach the frontend.")
    check("denied", expected_denied and not probes["storefront", "untrusted"] and
          not probes["outbound-guard", "client"], "Block all other ingress to the frontend.")
    return result


def capture(question, kube=None, probe=checked_probe):
    kube = kube or Kubernetes()
    if question in ("q2", "q13"):
        return egress_capture(question, kube, probe)
    if question == "q4":
        return ingress_capture(kube)
    raise ValueError("unsupported capture: " + question)


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: network_capture.py {q2|q4|q13}")
    print(json.dumps(capture(sys.argv[1])))
