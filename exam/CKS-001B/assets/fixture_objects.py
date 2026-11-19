"""Declarative Kubernetes starting fixtures for CKS-001B.

These objects are not a complete lab: host tools, ingress, Istio, TLS material,
archive exports, network probes and the etcd fault are installed separately.
No candidate answers (policies, labels or Secrets) are included here.
"""

NGINX = "nginx:1.28-alpine"
UNPRIVILEGED = "nginxinc/nginx-unprivileged:1.28-alpine"
BUSYBOX = "busybox:1.37.0"
PYTHON = "python:3.12.12-alpine3.22"
SLEEP = ["sh", "-c", "sleep 2147483647"]
NAMESPACES = (
    "cks001b-system", "compute-watch", "outbound-guard", "workload-identity",
    "storefront", "telemetry", "delivery", "immutable-workloads",
    "finance-ledger", "runtime-shield", "produce", "secure-portal",
    "api-clients", "private-mesh", "editor",
)


def pod(name, namespace, image=BUSYBOX, command=None, labels=None):
    container = {"name": name, "image": image, "imagePullPolicy": "IfNotPresent"}
    if command is not None:
        container["command"] = command
    return {
        "apiVersion": "v1", "kind": "Pod",
        "metadata": {"name": name, "namespace": namespace, "labels": labels or {"app": name}},
        "spec": {"containers": [container]},
    }


def deployment(name, namespace, image=BUSYBOX, command=None):
    template = pod(name, namespace, image, command)
    return {
        "apiVersion": "apps/v1", "kind": "Deployment",
        "metadata": {"name": name, "namespace": namespace},
        "spec": {
            "replicas": 1,
            "selector": {"matchLabels": {"app": name}},
            "template": {
                "metadata": {"labels": {"app": name}},
                "spec": template["spec"],
            },
        },
    }


def service(name, namespace, port=80):
    return {
        "apiVersion": "v1", "kind": "Service",
        "metadata": {"name": name, "namespace": namespace},
        "spec": {"selector": {"app": name}, "ports": [{"port": port, "targetPort": port}]},
    }


def objects():
    result = [
        {"apiVersion": "v1", "kind": "Namespace", "metadata": {
            "name": ns, "labels": {"painkiller-shell/exam": "CKS-001B"}}}
        for ns in NAMESPACES
    ]
    # All candidates start without the answers the question asks them to create.
    result.extend([
        pod("net-control", "cks001b-system", PYTHON, SLEEP),
        pod("isolation-canary", "cks001b-system", NGINX),
        # Benign copied BusyBox executables produce real exec events, not mining.
        pod("suspect", "compute-watch", BUSYBOX, ["sh", "-c",
            "for n in xmrig minerd cpuminer; do cp /bin/busybox /tmp/$n; done; "
            "while true; do for n in xmrig minerd cpuminer; do /tmp/$n true >/dev/null 2>&1 || true; done; sleep 3; done"]),
        pod("client", "outbound-guard", BUSYBOX, SLEEP),
        pod("net-observer", "outbound-guard", PYTHON, SLEEP),
        pod("web-gate", "workload-identity", NGINX),
        pod("shop-web", "storefront", NGINX),
        pod("shop-api", "storefront", BUSYBOX, SLEEP),
        pod("untrusted", "storefront", BUSYBOX, SLEEP),
        pod("observer", "telemetry", BUSYBOX, SLEEP),
        pod("runtime-web", "runtime-shield", NGINX),
        pod("client", "api-clients", BUSYBOX, SLEEP),
        pod("net-observer", "api-clients", PYTHON, SLEEP),
        service("shop-web", "storefront"),
        {"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
         "metadata": {"name": "isolate-canary", "namespace": "cks001b-system"},
         "spec": {"podSelector": {"matchLabels": {"app": "isolation-canary"}},
                  "policyTypes": ["Ingress"], "ingress": []}},
    ])

    # The granted Pod/Service/ConfigMap verbs must be retained by Q5.
    result.extend([
        {"apiVersion": "v1", "kind": "ServiceAccount",
         "metadata": {"name": "pipeline-agent", "namespace": "delivery"}},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole",
         "metadata": {"name": "pipeline-agent-role"},
         "rules": [
             {"apiGroups": [""], "resources": ["pods", "services", "configmaps"],
              "verbs": ["get", "list", "watch", "create", "update", "patch", "delete"]},
             {"apiGroups": ["rbac.authorization.k8s.io"],
              "resources": ["rolebindings", "clusterrolebindings"],
              "verbs": ["get", "list", "watch", "create", "update", "patch"]},
         ]},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding",
         "metadata": {"name": "pipeline-agent-binding"},
         "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole",
                     "name": "pipeline-agent-role"},
         "subjects": [{"kind": "ServiceAccount", "name": "pipeline-agent", "namespace": "delivery"}]},
    ])

    report = deployment("report-web", "immutable-workloads", NGINX)
    mounts = [("tmp", "/tmp"), ("log", "/var/log"), ("cache", "/var/cache/nginx"), ("run", "/var/run")]
    report_spec = report["spec"]["template"]["spec"]
    report_spec["volumes"] = [{"name": name, "emptyDir": {}} for name, _ in mounts]
    report_spec["containers"][0]["volumeMounts"] = [
        {"name": name, "mountPath": path} for name, path in mounts]
    report_spec["containers"][0]["command"] = [
        "sh", "-c", "mkdir -p /var/log/nginx; exec nginx -g 'daemon off;'"]
    report_spec["containers"][0]["readinessProbe"] = {"httpGet": {"path": "/", "port": 80}}
    result.append(report)

    # Restricted-compliant before enforcement is switched on in Q9.
    finance = deployment("ledger-web", "finance-ledger", UNPRIVILEGED)
    finance_spec = finance["spec"]["template"]["spec"]
    finance_spec["securityContext"] = {"runAsNonRoot": True, "runAsUser": 101, "runAsGroup": 101,
                                      "seccompProfile": {"type": "RuntimeDefault"}}
    finance_spec["containers"][0]["securityContext"] = {
        "allowPrivilegeEscalation": False, "capabilities": {"drop": ["ALL"]}}
    result.append(finance)

    orchard = deployment("orchard", "produce")
    orchard["spec"]["template"]["spec"]["containers"] = [
        {"name": name, "image": image, "imagePullPolicy": "IfNotPresent", "command": SLEEP}
        for name, image in (("pear", BUSYBOX), ("plum", "alpine/curl:8.12.1"),
                            ("cherry", "alpine:3.21.3"))
    ]
    result.append(orchard)

    for name, namespace, image, command in (
        ("portal", "secure-portal", NGINX, None),
        ("north", "private-mesh", NGINX, None),
        ("south", "private-mesh", NGINX, None),
        ("editor-web", "editor", BUSYBOX, SLEEP),
    ):
        result.append(deployment(name, namespace, image, command))
        if name in ("portal", "north", "south"):
            result.append(service(name, namespace))

    # Q8 starts with an extra capability; preparation writes this same object
    # as the candidate's manifest, while applying it to the default namespace.
    invoice = pod("invoice-web", "default", UNPRIVILEGED)
    container = invoice["spec"]["containers"][0]
    container["ports"] = [{"containerPort": 8080, "protocol": "TCP"}]
    container["securityContext"] = {
        "allowPrivilegeEscalation": False, "runAsNonRoot": True, "runAsUser": 101,
        "capabilities": {"add": ["NET_RAW"]},
    }
    invoice["spec"]["securityContext"] = {"seccompProfile": {"type": "RuntimeDefault"}}
    result.append(invoice)
    return result


if __name__ == "__main__":
    import json
    print(json.dumps({"apiVersion": "v1", "kind": "List", "items": objects()}, indent=2))
