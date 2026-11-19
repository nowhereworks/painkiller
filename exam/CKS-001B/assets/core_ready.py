"""Read-only checks for the Kubernetes fixture subset, not full exam readiness."""
import json
import subprocess
import sys

from fixture_objects import NAMESPACES


def ready_pod(obj):
    return (not obj.get("metadata", {}).get("deletionTimestamp") and
            any(c.get("type") == "Ready" and c.get("status") == "True"
                for c in obj.get("status", {}).get("conditions", [])))


def ready_deployment(obj):
    status = obj.get("status", {})
    desired = obj.get("spec", {}).get("replicas", 1)
    return (desired > 0 and status.get("observedGeneration", 0) >= obj["metadata"].get("generation", 0)
            and status.get("updatedReplicas", 0) >= desired and status.get("readyReplicas", 0) >= desired)


def check(items):
    """Return diagnostics; only the subset in fixture_objects is covered."""
    index = {(o["kind"], o["metadata"].get("namespace"), o["metadata"]["name"]): o for o in items}
    failures = []

    def require(condition, message):
        if not condition:
            failures.append(message)

    for ns in NAMESPACES:
        obj = index.get(("Namespace", None, ns), {})
        require(obj.get("metadata", {}).get("labels", {}).get("painkiller-shell/exam") == "CKS-001B",
                f"missing exam-owned namespace {ns}")
    for ns, name in [
        ("cks001b-system", "net-control"), ("compute-watch", "suspect"), ("outbound-guard", "client"), ("outbound-guard", "net-observer"),
        ("workload-identity", "web-gate"), ("storefront", "shop-web"),
        ("storefront", "shop-api"), ("storefront", "untrusted"),
        ("telemetry", "observer"), ("runtime-shield", "runtime-web"),
        ("api-clients", "client"), ("api-clients", "net-observer"), ("default", "invoice-web"),
    ]:
        obj = index.get(("Pod", ns, name), {})
        require(bool(obj) and ready_pod(obj), f"Pod {ns}/{name} not ready")
    for ns, name in [
        ("immutable-workloads", "report-web"), ("finance-ledger", "ledger-web"),
        ("produce", "orchard"), ("secure-portal", "portal"),
        ("private-mesh", "north"), ("private-mesh", "south"), ("editor", "editor-web"),
    ]:
        obj = index.get(("Deployment", ns, name), {})
        require(bool(obj) and ready_deployment(obj), f"Deployment {ns}/{name} not ready")
    for ns, name in [("storefront", "shop-web"), ("secure-portal", "portal"),
                     ("private-mesh", "north"), ("private-mesh", "south")]:
        obj = index.get(("Service", ns, name), {})
        require(obj.get("spec", {}).get("selector", {}).get("app") == name,
                f"Service {ns}/{name} missing or has wrong selector")

    for ns, name in [("outbound-guard", "exclude-ranges"), ("storefront", "web-access"),
                     ("api-clients", "allow-service-egress")]:
        require(("NetworkPolicy", ns, name) not in index, f"answer {ns}/{name} already exists")
    finance = index.get(("Namespace", None, "finance-ledger"), {})
    labels = finance.get("metadata", {}).get("labels", {})
    require("pod-security.kubernetes.io/enforce" not in labels, "finance-ledger already enforces PSA")
    invoice = index.get(("Pod", "default", "invoice-web"), {})
    containers = invoice.get("spec", {}).get("containers", [])
    require(bool(containers) and "NET_RAW" in containers[0].get("securityContext", {})
            .get("capabilities", {}).get("add", []), "invoice capability fault missing")
    role = index.get(("ClusterRole", None, "pipeline-agent-role"), {})
    require(any({"rolebindings", "clusterrolebindings"} <= set(rule.get("resources", []))
                and {"create", "update", "patch"} <= set(rule.get("verbs", []))
                and "rbac.authorization.k8s.io" in rule.get("apiGroups", [])
                for rule in role.get("rules", [])), "pipeline RBAC fault missing")
    account = index.get(("ServiceAccount", "delivery", "pipeline-agent"))
    binding = index.get(("ClusterRoleBinding", None, "pipeline-agent-binding"), {})
    require(bool(account) and binding.get("roleRef", {}).get("name") == "pipeline-agent-role" and
            any(s.get("kind") == "ServiceAccount" and s.get("name") == "pipeline-agent" and
                s.get("namespace") == "delivery" for s in binding.get("subjects", [])),
            "pipeline service account or binding missing")
    report = index.get(("Deployment", "immutable-workloads", "report-web"), {})
    report_containers = report.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
    require(bool(report_containers) and not report_containers[0].get("securityContext", {})
            .get("readOnlyRootFilesystem", False), "report-web root filesystem fault missing")
    web = index.get(("Pod", "workload-identity", "web-gate"), {})
    web_containers = web.get("spec", {}).get("containers", [])
    require(bool(web_containers) and web_containers[0].get("image") != "nginxinc/nginx-unprivileged:alpine",
            "web-gate image fault missing")
    return failures


def main():
    result = subprocess.run(
        ["kubectl", "--request-timeout=5s", "get",
         "namespaces,pods,deployments,services,networkpolicies,clusterroles,clusterrolebindings,serviceaccounts",
         "--all-namespaces", "-o", "json"],
        text=True, capture_output=True, timeout=15, check=True,
    )
    failures = check(json.loads(result.stdout)["items"])
    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("CKS-001B Kubernetes fixture subset ready (not full exam readiness)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
