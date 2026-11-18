"""A single ingress-nginx controller for the disposable Q12 lab.

The upstream controller is retired. It is used solely to test the source
question's nginx-specific annotations, never as a production recommendation.
The image digest is from the upstream v1.15.1 bare-metal deployment.
"""

NS = "cks001b-ingress"
NAME = "cks001b-ingress"
IMAGE = ("registry.k8s.io/ingress-nginx/controller:v1.15.1@sha256:"
         "594ceea76b01c592858f803f9ff4d2cb40542cae2060410b2c95f75907d659e1")


def objects():
    labels = {"app": NAME}
    account = {"kind": "ServiceAccount", "name": NAME, "namespace": NS}
    role_ref = {"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": NAME}
    perms = [
        {"apiGroups": [""], "resources": ["configmaps", "endpoints", "nodes", "pods", "secrets",
                                             "namespaces", "services"], "verbs": ["get", "list", "watch"]},
        {"apiGroups": [""], "resources": ["events"], "verbs": ["create", "patch"]},
        {"apiGroups": ["networking.k8s.io"], "resources": ["ingresses", "ingressclasses"],
         "verbs": ["get", "list", "watch"]},
        {"apiGroups": ["networking.k8s.io"], "resources": ["ingresses/status"], "verbs": ["update"]},
        {"apiGroups": ["discovery.k8s.io"], "resources": ["endpointslices"], "verbs": ["get", "list", "watch"]},
        {"apiGroups": ["coordination.k8s.io"], "resources": ["leases"],
         "verbs": ["get", "list", "watch", "create", "update"]},
    ]
    container = {
        "name": "controller", "image": IMAGE, "imagePullPolicy": "IfNotPresent",
        "args": ["/nginx-ingress-controller", "--election-id=cks001b-ingress-leader",
                 "--controller-class=k8s.io/ingress-nginx", "--ingress-class=nginx",
                 f"--configmap={NS}/{NAME}"],
        "env": [{"name": name, "valueFrom": {"fieldRef": {"fieldPath": field}}}
                for name, field in (("POD_NAME", "metadata.name"), ("POD_NAMESPACE", "metadata.namespace"))],
        "ports": [{"name": "http", "containerPort": 80}, {"name": "https", "containerPort": 443}],
        "readinessProbe": {"httpGet": {"path": "/healthz", "port": 10254}, "initialDelaySeconds": 10},
        "livenessProbe": {"httpGet": {"path": "/healthz", "port": 10254}, "initialDelaySeconds": 10},
        "resources": {"requests": {"cpu": "100m", "memory": "90Mi"}},
        "securityContext": {"runAsUser": 101, "runAsGroup": 82, "runAsNonRoot": True,
                            "allowPrivilegeEscalation": False,
                            "capabilities": {"drop": ["ALL"], "add": ["NET_BIND_SERVICE"]},
                            "seccompProfile": {"type": "RuntimeDefault"}},
    }
    return [
        {"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": NS}},
        {"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": NAME, "namespace": NS}},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole",
         "metadata": {"name": NAME}, "rules": perms},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding",
         "metadata": {"name": NAME}, "roleRef": role_ref, "subjects": [account]},
        {"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": NAME, "namespace": NS}},
        {"apiVersion": "networking.k8s.io/v1", "kind": "IngressClass",
         "metadata": {"name": "nginx"}, "spec": {"controller": "k8s.io/ingress-nginx"}},
        {"apiVersion": "v1", "kind": "Service", "metadata": {"name": NAME, "namespace": NS},
         "spec": {"type": "NodePort", "selector": labels, "ports": [
             {"name": "http", "port": 80, "targetPort": "http"},
             {"name": "https", "port": 443, "targetPort": "https"}]}},
        {"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": NAME, "namespace": NS},
         "spec": {"replicas": 1, "selector": {"matchLabels": labels}, "template": {
             "metadata": {"labels": labels}, "spec": {"serviceAccountName": NAME, "containers": [container]}}}},
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
