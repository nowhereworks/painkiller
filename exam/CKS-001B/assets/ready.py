"""Read-only aggregate initial readiness; independent groups run concurrently."""
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import json
import subprocess
import sys

import admission_ready
import core_ready
import etcd_ready
import host_ready
import ingress_ready
import network_ready
import services_ready
from workload_capture import Kubernetes


def core():
    kube = Kubernetes()
    nodes = kube.get("nodes").get("items", [])
    if len(nodes) != 1 or not core_ready.ready_pod(nodes[0]):
        return ["exactly one Ready node required"]
    if any(t.get("effect") in ("NoSchedule", "NoExecute") for t in nodes[0].get("spec", {}).get("taints", [])):
        return ["the single control plane must be schedulable"]
    if not kube.get("daemonset", "calico-node", "kube-system"):
        return ["Calico is missing"]
    if kube.get("secret", "editor-cert", "editor", optional=True):
        return ["Q15 Secret already exists"]
    from capture_files import read_file
    home = Path(json.loads(read_file(Path("/var/lib/painkiller/cks001b/candidate.json")))["home"])
    if any((home / name).exists() for name in ("selected-produce.txt", "selected-produce.spdx.json")):
        return ["Q11 output files already exist"]
    target = kube.get("pod", "isolation-canary", "cks001b-system")
    if not core_ready.ready_pod(target):
        return ["network isolation canary is not Ready"]
    if kube.exec("cks001b-system", "isolation-canary",
                 ["wget", "-T", "2", "-qO-", "http://127.0.0.1/"]).returncode:
        return ["network isolation canary HTTP service is unavailable"]
    probe = kube.exec("cks001b-system", "net-control", ["python3", "-c",
        "import socket,sys\n"
        "try:\n s=socket.create_connection((sys.argv[1],80),timeout=1); s.close()\n"
        "except OSError:\n sys.exit(0)\n"
        "sys.exit(1)", target["status"]["podIP"]])
    if probe.returncode != 0:
        return ["Calico does not enforce the independent ingress deny canary"]
    result = subprocess.run(["kubectl", "--request-timeout=5s", "get",
        "namespaces,pods,deployments,services,networkpolicies,clusterroles,clusterrolebindings,serviceaccounts",
        "--all-namespaces", "-o", "json"], text=True, capture_output=True, check=True, timeout=12)
    return core_ready.check(json.loads(result.stdout)["items"])


def main():
    root = Path("/")
    checks = [core, lambda: host_ready.check(root), lambda: services_ready.inspect(root),
              lambda: network_ready.inspect(root), lambda: ingress_ready.inspect(root),
              admission_ready.inspect, lambda: etcd_ready.inspect(root=root)]
    def run(check):
        try:
            return check()
        except Exception as error:
            return [str(error)]
    with ThreadPoolExecutor(max_workers=len(checks)) as pool:
        failures = [message for group in pool.map(run, checks) for message in group]
    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("CKS-001B ready: one node, all fixtures, dependencies and intentional faults verified")
    return 0


if __name__ == "__main__":
    sys.exit(main())
