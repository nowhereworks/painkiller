"""Bounded, read-only NetworkPolicy model for CKS-001B's named probe cases.

This is not a replacement for live connectivity tests.  In particular, CNI
handling of ipBlock and source NAT can differ, so the exam must probe the
actual cluster as well.  Inputs are Kubernetes API objects, not YAML text.
"""

from ipaddress import ip_address, ip_network


def selector_matches(selector, labels):
    """Evaluate Kubernetes matchLabels and matchExpressions."""
    if any(labels.get(k) != v for k, v in selector.get("matchLabels", {}).items()):
        return False
    for expression in selector.get("matchExpressions", []):
        key = expression["key"]
        op = expression["operator"]
        values = expression.get("values", [])
        present = key in labels
        if op == "In":
            matches = present and labels[key] in values
        elif op == "NotIn":
            matches = not present or labels[key] not in values
        elif op == "Exists":
            matches = present
        elif op == "DoesNotExist":
            matches = not present
        else:
            raise ValueError("unsupported selector operator: " + op)
        if not matches:
            return False
    return True


def peer_matches(peer, policy_namespace, endpoint):
    """An endpoint has ip, namespace, namespace_labels and pod_labels keys.

    For an external address namespace is None. A podSelector without a
    namespaceSelector means pods in the policy's own namespace.
    """
    if "ipBlock" in peer:
        block = peer["ipBlock"]
        address = ip_address(endpoint["ip"])
        return (address in ip_network(block["cidr"]) and
                not any(address in ip_network(excluded) for excluded in block.get("except", [])))
    namespace = endpoint.get("namespace")
    if namespace is None:
        return False
    if "namespaceSelector" in peer:
        if not selector_matches(peer["namespaceSelector"], endpoint.get("namespace_labels", {})):
            return False
    elif namespace != policy_namespace:
        return False
    if "podSelector" in peer:
        return selector_matches(peer["podSelector"], endpoint.get("pod_labels", {}))
    return True


def port_matches(rule, protocol, port):
    ports = rule.get("ports")
    if not ports:
        return True
    for entry in ports:
        if entry.get("protocol", "TCP") != protocol:
            continue
        candidate = entry.get("port")
        if candidate is None:
            return True
        if not isinstance(candidate, int):
            # Resolving named ports requires the destination Pod's port table.
            # Never silently treat an unresolved name as allowing or denying.
            raise ValueError("named NetworkPolicy port requires resolution")
        if candidate <= port <= entry.get("endPort", candidate):
            return True
    return False


def permits(policies, direction, subject, peer, protocol, port):
    """Model one direction for a Pod and one concrete connection.

    subject contains namespace and pod_labels. Consider all selecting policies:
    isolation starts if any policy selects the Pod for that direction, while
    allowances are the union across rules and policies. The caller must check
    ingress and egress independently for Pod-to-Pod connections.
    """
    if direction not in ("Ingress", "Egress"):
        raise ValueError("invalid direction")
    policy_key = direction.lower()
    peer_key = "from" if direction == "Ingress" else "to"
    selected = []
    for policy in policies:
        if policy["metadata"]["namespace"] != subject["namespace"]:
            continue
        spec = policy["spec"]
        types = spec.get("policyTypes")
        if types is None:
            types = ["Ingress"] + (["Egress"] if spec.get("egress") else [])
        if direction in types and selector_matches(spec.get("podSelector", {}), subject["pod_labels"]):
            selected.append(spec)
    if not selected:
        return True
    for spec in selected:
        for rule in spec.get(policy_key, []):
            if not port_matches(rule, protocol, port):
                continue
            peers = rule.get(peer_key)
            if not peers or any(peer_matches(item, subject["namespace"], peer) for item in peers):
                return True
    return False
