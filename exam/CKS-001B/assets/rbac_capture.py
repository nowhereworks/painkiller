"""Read-only effective RBAC grading for CKS-001B Q5."""
from concurrent.futures import ThreadPoolExecutor
from itertools import product
import json
from pathlib import Path
import subprocess
import sys

from capture_files import json_file
from host_fixtures import create_once, path
from workload_capture import Kubernetes

USER = "system:serviceaccount:delivery:pipeline-agent"
GROUPS = ("system:serviceaccounts", "system:serviceaccounts:delivery", "system:authenticated")
BAD_VERBS = ("create", "update", "patch")
BAD_GROUP = "rbac.authorization.k8s.io"


def bound(subject, namespace):
    kind, name = subject.get("kind"), subject.get("name")
    if kind == "ServiceAccount":
        return name == "pipeline-agent" and subject.get("namespace", namespace) == "delivery"
    return kind == "User" and name == USER or kind == "Group" and name in GROUPS


def atoms(kube):
    """Resolve bound Role/ClusterRole rules, including group-bound permissions."""
    cluster_roles = {o["metadata"]["name"]: o for o in kube.get("clusterroles").get("items", [])}
    roles = {(o["metadata"]["namespace"], o["metadata"]["name"]): o
             for o in kube.list_all("roles").get("items", [])}
    bindings = [(o, "*") for o in kube.get("clusterrolebindings").get("items", [])]
    bindings += [(o, o["metadata"]["namespace"]) for o in kube.list_all("rolebindings").get("items", [])]
    if len(bindings) > 5000 or len(cluster_roles) + len(roles) > 5000:
        raise RuntimeError("RBAC object count exceeds bounded capture budget")
    found = set()
    for binding, scope in bindings:
        if not any(bound(s, None if scope == "*" else scope) for s in binding.get("subjects", [])):
            continue
        ref = binding.get("roleRef", {})
        if ref.get("apiGroup") != "rbac.authorization.k8s.io":
            raise RuntimeError("unsupported RBAC roleRef API group")
        if ref.get("kind") == "ClusterRole":
            role = cluster_roles.get(ref.get("name"))
        elif ref.get("kind") == "Role" and scope != "*":
            role = roles.get((scope, ref.get("name")))
        else:
            raise RuntimeError("unsupported RBAC roleRef kind")
        if role is None:
            # A dangling binding grants no permission.
            continue
        for rule in role.get("rules", []):
            verbs = rule.get("verbs", [])
            urls = rule.get("nonResourceURLs") or []
            groups = rule.get("apiGroups", [""])
            resources = rule.get("resources") or []
            names = rule.get("resourceNames") or [""]
            if len(verbs) * (len(urls) + len(groups) * len(resources) * len(names)) > 20000:
                raise RuntimeError("RBAC rule expansion exceeds bounded capture budget")
            # Non-resource requests are cluster-scoped; RoleBindings cannot
            # authorize them even when they reference a ClusterRole.
            if urls and scope == "*":
                for verb, url in product(verbs, rule["nonResourceURLs"]):
                    found.add((scope, "url", "", url, verb, ""))
            if resources:
                for group, resource, verb, name in product(groups, resources, verbs, names):
                    found.add((scope, "resource", group, resource, verb, name))
            if len(found) > 20000:
                raise RuntimeError("RBAC rule count exceeds bounded capture budget")
    return found


def pattern_covers(base, requested, kind):
    if base == "*" or base == requested:
        return True
    if kind == "resource" and base.startswith("*/") and "/" in requested:
        return base[2:] == requested.split("/", 1)[1]
    if kind == "url" and base.endswith("*"):
        return requested.startswith(base[:-1])
    return False


def covers(base, requested):
    """Whether one original rule atom covers the entire requested atom."""
    bscope, btype, bgroup, bresource, bverb, bname = base
    scope, kind, group, resource, verb, name = requested
    if btype != kind or not (bscope == "*" or bscope == scope):
        return False
    if not pattern_covers(bverb, verb, "verb"):
        return False
    if kind == "url":
        return pattern_covers(bresource, resource, "url")
    return (pattern_covers(bgroup, group, "group") and
            pattern_covers(bresource, resource, "resource") and
            (bname == "" or bname == name))


def forbidden(atom, resource):
    # ClusterRoleBindings are cluster-scoped; a namespaced RoleBinding alone
    # cannot authorize operations on that resource.
    if resource == "clusterrolebindings" and atom[0] != "*":
        return False
    return any(covers(atom, (atom[0], "resource", BAD_GROUP, resource, verb, ""))
               for verb in BAD_VERBS) or any(
        covers(atom, (atom[0], "resource", BAD_GROUP, resource, verb, atom[5]))
        for verb in BAD_VERBS)


def permitted_baseline(grants):
    result = set()
    for grant in grants:
        if any(forbidden(grant, name) for name in ("rolebindings", "clusterrolebindings")):
            # Do not silently discard unrelated privileges bundled in a broad
            # wildcard. The known initial fixture grants the six verbs exactly.
            if grant[2] != BAD_GROUP or grant[3] not in ("rolebindings", "clusterrolebindings") or grant[4] not in BAD_VERBS:
                raise RuntimeError("initial RBAC contains an unexpected broad escalation grant")
            continue
        result.add(grant)
    return result


def subset(a, b):
    if len(a) * len(b) > 1000000:
        raise RuntimeError("RBAC comparison exceeds bounded capture budget")
    return all(any(covers(old, new) for old in b) for new in a)


def required_original_permissions(grants):
    return all(any(covers(atom, ("*", "resource", "", resource, verb, "")) for atom in grants)
               for resource in ("pods", "services", "configmaps")
               for verb in ("get", "list", "watch", "create", "update", "patch", "delete"))


def save_baseline(kube, root):
    state = path(root, "/var/lib/painkiller/cks001b")
    if not (state / "authorized").is_file():
        raise RuntimeError("Refusing RBAC baseline on unauthorized guest")
    namespace = kube.get("namespace", "cks001b-system")
    if namespace.get("metadata", {}).get("labels", {}).get("painkiller-shell/exam") != "CKS-001B":
        raise RuntimeError("Refusing RBAC baseline on unmarked cluster")
    filename = state / "rbac-baseline.json"
    if filename.exists():
        return
    initial = atoms(kube)
    if not required_original_permissions(initial) or not all(
            any(forbidden(atom, resource) for atom in initial)
            for resource in ("rolebindings", "clusterrolebindings")):
        raise RuntimeError("initial RBAC is missing original permissions or intentional faults")
    baseline = sorted(permitted_baseline(initial))
    create_once(filename, json.dumps({"version": 1, "grants": baseline}) + "\n", 0o600)


def auth_check(resource, verb):
    args = ["kubectl", "--request-timeout=5s", "auth", "can-i", verb, resource,
            "--all-namespaces", "--as=" + USER, *["--as-group=" + group for group in GROUPS]]
    result = subprocess.run(args, text=True, capture_output=True, timeout=8)
    value = result.stdout.strip()
    if (result.returncode, value) not in ((0, "yes"), (1, "no")):
        raise RuntimeError("supporting authorization review failed: " +
                           result.stderr.replace("NotFound", "missing supporting object"))
    return value == "yes"


def capture(kube=None, root=Path("/"), review=auth_check):
    kube = kube or Kubernetes()
    baseline = json_file(path(root, "/var/lib/painkiller/cks001b/rbac-baseline.json"))
    if not isinstance(baseline, dict) or baseline.get("version") != 1 or not isinstance(baseline.get("grants"), list):
        raise RuntimeError("supporting RBAC baseline is missing or invalid")
    try:
        expected = {tuple(atom) for atom in baseline["grants"]}
        if any(len(atom) != 6 or not all(isinstance(x, str) for x in atom) for atom in expected):
            raise ValueError("invalid rule atom")
    except (TypeError, ValueError):
        raise RuntimeError("supporting RBAC baseline is invalid") from None
    if not required_original_permissions(expected):
        raise RuntimeError("supporting RBAC baseline lacks original permissions")
    current = atoms(kube)
    account = kube.get("serviceaccount", "pipeline-agent", "delivery", optional=True)
    jobs = [(resource, verb) for resource in ("rolebindings", "clusterrolebindings") for verb in BAD_VERBS]
    with ThreadPoolExecutor(max_workers=3) as pool:
        decisions = list(pool.map(lambda job: review(*job), jobs))
    actual = dict(zip(jobs, decisions))
    if not all(isinstance(value, bool) for value in decisions):
        raise RuntimeError("supporting authorization review returned invalid data")
    denied = {resource: bool(account) and not any(actual[resource, verb] for verb in BAD_VERBS) and
              not any(forbidden(atom, resource) for atom in current)
              for resource in ("rolebindings", "clusterrolebindings")}
    secured = all(denied.values())
    retained = secured and subset(expected, current)
    no_new = secured and subset(current, expected)
    results = {"rolebindings": denied["rolebindings"], "clusterrolebindings": denied["clusterrolebindings"],
               "retained": retained, "no_new": no_new}
    messages = {"rolebindings": "Remove rolebinding create, update and patch access in every namespace.",
                "clusterrolebindings": "Remove clusterrolebinding create, update and patch access.",
                "retained": "Retain all other original permissions.",
                "no_new": "Do not grant any additional permissions."}
    results["failures"] = [message for key, message in messages.items() if not results[key]]
    return results


if __name__ == "__main__":
    if len(sys.argv) == 2 and sys.argv[1] == "baseline":
        save_baseline(Kubernetes(), Path("/"))
    elif len(sys.argv) == 2 and sys.argv[1] == "capture":
        print(json.dumps(capture()))
    else:
        raise SystemExit("usage: rbac_capture.py {baseline|capture}")
