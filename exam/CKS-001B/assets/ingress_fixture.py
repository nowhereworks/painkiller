"""One Traefik controller using its actual Kubernetes Ingress NGINX provider."""

NS = "cks001b-ingress"
NAME = "cks001b-ingress"
IMAGE = ("traefik:v3.7.19@sha256:"
         "aa942a00bce033c909bf1f7e861d93761be4b31a035f10d2410d33e38a8f41cf")


def objects():
    labels = {"app": NAME}
    return [
        {"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": NS}},
        {"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": NAME, "namespace": NS}},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole",
         "metadata": {"name": NAME}, "rules": [
             {"apiGroups": [""], "resources": ["services", "endpoints", "secrets", "namespaces", "pods", "nodes", "configmaps"],
              "verbs": ["get", "list", "watch"]},
             {"apiGroups": ["discovery.k8s.io"], "resources": ["endpointslices"], "verbs": ["get", "list", "watch"]},
             {"apiGroups": ["networking.k8s.io"], "resources": ["ingresses", "ingressclasses"],
              "verbs": ["get", "list", "watch"]},
             {"apiGroups": ["networking.k8s.io"], "resources": ["ingresses/status"], "verbs": ["update"]},
         ]},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding",
         "metadata": {"name": NAME}, "roleRef": {
             "apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": NAME},
         "subjects": [{"kind": "ServiceAccount", "name": NAME, "namespace": NS}]},
        # The provider intentionally claims nginx's class, matching the source.
        {"apiVersion": "networking.k8s.io/v1", "kind": "IngressClass",
         "metadata": {"name": "nginx"}, "spec": {"controller": "k8s.io/ingress-nginx"}},
        {"apiVersion": "v1", "kind": "Service", "metadata": {"name": NAME, "namespace": NS},
         "spec": {"type": "NodePort", "selector": labels, "ports": [
             {"name": "http", "port": 80, "targetPort": "http"},
             {"name": "https", "port": 443, "targetPort": "https"}]}},
        {"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": NAME, "namespace": NS},
         "spec": {"replicas": 1, "selector": {"matchLabels": labels}, "template": {
             "metadata": {"labels": labels}, "spec": {"serviceAccountName": NAME, "containers": [{
                 "name": "controller", "image": IMAGE,
                 "args": ["--providers.kubernetesingressnginx=true",
                          "--providers.kubernetesingressnginx.ingressclass=nginx",
                          "--entrypoints.web.address=:8000", "--entrypoints.websecure.address=:8443",
                          "--entrypoints.health.address=:9000", "--ping=true", "--ping.entrypoint=health"],
                 "ports": [{"name": "http", "containerPort": 8000}, {"name": "https", "containerPort": 8443}],
                 "readinessProbe": {"httpGet": {"path": "/ping", "port": 9000}},
                 "resources": {"requests": {"cpu": "100m", "memory": "90Mi"}},
                 "securityContext": {"runAsUser": 65532, "runAsNonRoot": True,
                                     "allowPrivilegeEscalation": False, "capabilities": {"drop": ["ALL"]}},
             }]}}}},
    ]


def canary():
    return {
        "apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
        "metadata": {"name": "cks001b-ingress-canary", "namespace": "secure-portal",
                     "annotations": {"nginx.ingress.kubernetes.io/ssl-redirect": "true",
                                     "nginx.ingress.kubernetes.io/ssl-passthrough": "false"}},
        "spec": {"ingressClassName": "nginx",
                 "tls": [{"hosts": ["ingress-check.training.example"], "secretName": "cks001b-canary-cert"}],
                 "rules": [{"host": "ingress-check.training.example", "http": {"paths": [{
                     "path": "/", "pathType": "Prefix",
                     "backend": {"service": {"name": "portal", "port": {"number": 80}}}}]}}]},
    }
