"""Read-only ingress controller and independent TLS/redirect canary checks."""
import json
from pathlib import Path
import subprocess
import sys

from host_fixtures import path
from ingress_fixture import NS, NAME
from services import k
from services_ready import ready_deployment

HOST = "ingress-check.training.example"


def probe(ip, http_port, https_port, ca, host=HOST):
    """Observe an existing route only; never create or mutate an Ingress."""
    base = ["curl", "--silent", "--show-error", "--noproxy", "*", "--connect-timeout", "2", "--max-time", "5"]
    http = subprocess.run(
        [*base, "--resolve", f"{host}:{http_port}:{ip}", "-H", f"Host: {host}",
         "--dump-header", "-", "--output", "/dev/null", f"http://{host}:{http_port}/"],
        text=True, capture_output=True, timeout=7,
    )
    https = subprocess.run(
        [*base, "--fail", "--cacert", str(ca), "--resolve", f"{host}:{https_port}:{ip}",
         f"https://{host}:{https_port}/"],
        text=True, capture_output=True, timeout=7,
    )
    lines = http.stdout.replace("\r", "").splitlines()
    status = lines[0].split()[1] if lines and len(lines[0].split()) >= 2 else ""
    locations = [line.split(":", 1)[1].strip() for line in lines if line.lower().startswith("location:")]
    redirect = (http.returncode == 0 and status in ("301", "302", "307", "308")
                and any(loc.startswith(f"https://{host}/") or loc == f"https://{host}"
                        or loc.startswith(f"https://{host}:") for loc in locations))
    return redirect, https.returncode == 0 and "Welcome to nginx" in https.stdout


def inspect(root):
    failures = []
    state = path(root, "/var/lib/painkiller/cks001b")
    if not (state / "ingress-installed").is_file():
        failures.append("ingress installation marker missing")
    controller = json.loads(k("-n", NS, "get", "deployment", NAME, "-o", "json").stdout)
    if not ready_deployment(controller):
        failures.append("ingress controller not ready")
    klass = json.loads(k("get", "ingressclass", "nginx", "-o", "json").stdout)
    if klass.get("spec", {}).get("controller") != "k8s.io/ingress-nginx":
        failures.append("nginx IngressClass has wrong controller")
    svc = json.loads(k("-n", NS, "get", "service", NAME, "-o", "json").stdout)
    ports = {p.get("name"): p.get("nodePort") for p in svc.get("spec", {}).get("ports", [])}
    if svc.get("spec", {}).get("type") != "NodePort" or not all(
            isinstance(ports.get(name), int) and 30000 <= ports[name] <= 32767 for name in ("http", "https")):
        failures.append("ingress HTTP/HTTPS NodePorts missing")
        return failures
    endpoints = json.loads(k("-n", NS, "get", "endpointslices", "-l",
                             f"kubernetes.io/service-name={NAME}", "-o", "json").stdout)
    if not any(e.get("conditions", {}).get("ready") is True
               for item in endpoints.get("items", []) for e in item.get("endpoints", [])):
        failures.append("ingress service has no ready endpoint")
    candidate = k("-n", "secure-portal", "get", "ingress", "portal-tls",
                  "--ignore-not-found", "-o", "json").stdout
    if candidate.strip():
        failures.append("candidate ingress already exists")
    nodes = json.loads(k("get", "nodes", "-o", "json").stdout).get("items", [])
    if len(nodes) != 1:
        failures.append("expected one node for ingress probe")
        return failures
    addresses = [a["address"] for a in nodes[0].get("status", {}).get("addresses", [])
                 if a.get("type") == "InternalIP" and ":" not in a.get("address", "")]
    if not addresses:
        failures.append("node InternalIP missing")
        return failures
    try:
        redirect, https = probe(addresses[0], ports["http"], ports["https"], state / "canary-tls.crt")
        if not redirect:
            failures.append("canary HTTP does not redirect to HTTPS")
        if not https:
            failures.append("canary HTTPS certificate verification or backend failed")
    except (OSError, subprocess.SubprocessError):
        failures.append("ingress canary probe failed")
    return failures


if __name__ == "__main__":
    problems = inspect(Path("/"))
    if problems:
        print("\n".join(problems), file=sys.stderr)
        sys.exit(1)
    print("CKS-001B ingress subset ready (not full exam readiness)")
