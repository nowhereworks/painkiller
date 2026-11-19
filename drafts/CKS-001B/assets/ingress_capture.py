"""Read-only Q12 capture with an independent certificate-verified canary."""
import json
from pathlib import Path
import subprocess
import sys

from host_fixtures import path
from ingress_fixture import NS, NAME
from ingress_ready import probe
from workload_capture import Kubernetes, ready_deployment

HOST = "portal.training.example"


def capture(kube=None, root=Path("/"), traffic=probe):
    kube = kube or Kubernetes()
    ingress = kube.get("ingress", "portal-tls", "secure-portal", optional=True)
    spec = ingress.get("spec", {})
    annotations = ingress.get("metadata", {}).get("annotations", {})
    out = {"failures": []}

    def check(key, condition, message):
        out[key] = bool(condition)
        if not condition:
            out["failures"].append(message)

    ingress_class = spec.get("ingressClassName") or annotations.get("kubernetes.io/ingress.class")
    class_ok = bool(ingress) and ingress_class == "nginx"
    check("class", class_ok, "Use the nginx IngressClass.")
    tls = bool(ingress) and any(entry.get("secretName") == "portal-cert" and
                                (not entry.get("hosts") or HOST in entry["hosts"])
                                for entry in spec.get("tls", []))
    check("tls", tls, "Attach the supplied portal-cert TLS Secret to the requested host.")
    annotation_ok = (bool(ingress) and annotations.get("nginx.ingress.kubernetes.io/ssl-redirect") == "true" and
                     annotations.get("nginx.ingress.kubernetes.io/ssl-passthrough") == "false")
    check("annotations", annotation_ok, "Set the requested SSL redirect and passthrough annotations.")
    route = False
    if ingress:
        service = kube.get("service", "portal", "secure-portal")
        ports = service.get("spec", {}).get("ports", [])
        valid_names = {port.get("name") for port in ports if port.get("port") == 80 and port.get("name")}
        for rule in spec.get("rules", []):
            if rule.get("host") != HOST:
                continue
            for item in rule.get("http", {}).get("paths", []):
                backend = item.get("backend", {}).get("service", {})
                port = backend.get("port", {})
                if (item.get("path") == "/" and backend.get("name") == "portal" and
                        (port.get("number") == 80 or port.get("name") in valid_names)):
                    route = True
    check("route", route, "Route the requested host and / path to the portal Service.")

    redirect = https = False
    if ingress and route and tls and class_ok:
        controller = kube.get("deployment", NAME, NS)
        service = kube.get("service", NAME, NS)
        ports = {p.get("name"): p.get("nodePort") for p in service.get("spec", {}).get("ports", [])}
        nodes = kube.get("nodes").get("items", [])
        if not ready_deployment(controller) or service.get("spec", {}).get("type") != "NodePort" or len(nodes) != 1:
            raise RuntimeError("supporting ingress controller, NodePort service or singleton node is unavailable")
        if not all(isinstance(ports.get(name), int) and 30000 <= ports[name] <= 32767 for name in ("http", "https")):
            raise RuntimeError("supporting ingress NodePorts are unavailable")
        addresses = [a.get("address") for a in nodes[0].get("status", {}).get("addresses", [])
                     if a.get("type") == "InternalIP" and ":" not in a.get("address", "")]
        if not addresses:
            raise RuntimeError("supporting ingress node has no IPv4 InternalIP")
        try:
            control = traffic(addresses[0], ports["http"], ports["https"],
                              path(root, "/var/lib/painkiller/cks001b/canary-tls.crt"))
            if control != (True, True):
                raise RuntimeError("independent ingress canary is unavailable")
            redirect, https = traffic(addresses[0], ports["http"], ports["https"],
                                      path(root, "/root/CKS/portal-ca.crt"), HOST)
        except (OSError, subprocess.SubprocessError) as error:
            raise RuntimeError("supporting ingress traffic probe failed") from error
    check("redirect", class_ok and route and tls and redirect, "HTTP must redirect to HTTPS.")
    check("https", class_ok and route and tls and https, "Serve the portal with certificate-verified HTTPS.")
    return out


if __name__ == "__main__":
    print(json.dumps(capture()))
