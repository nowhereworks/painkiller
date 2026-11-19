"""Task-level loss mutations, guarded by both guest and cluster ownership."""
import json
from pathlib import Path
import subprocess
import sys

from fixture_objects import objects
from host_fixtures import DOCKERFILE

ROOT = Path("/opt/painkiller/cks001b")


def run(*args, data=None):
    return subprocess.run(list(args), input=data, text=True, check=True, timeout=180)


def k(*args, data=None):
    return run("kubectl", "--request-timeout=10s", *args, data=data)


def original(kind, name):
    return next(o for o in objects() if o["kind"] == kind and o["metadata"]["name"] == name)


def replace_pod(name):
    obj = original("Pod", name)
    ns = obj["metadata"]["namespace"]
    k("-n", ns, "delete", "pod", name, "--ignore-not-found", "--wait=true")
    k("apply", "-f", "-", data=json.dumps(obj))
    k("-n", ns, "wait", "pod/" + name, "--for=condition=Ready", "--timeout=120s")


def remove(n):
    if not Path("/var/lib/painkiller/cks001b/authorized").is_file():
        raise RuntimeError("Refusing mutation outside authorized guest")
    result = subprocess.run(["kubectl", "get", "ns", "cks001b-system", "-o", "json"],
                            check=True, capture_output=True, text=True, timeout=10)
    if json.loads(result.stdout)["metadata"]["labels"].get("painkiller-shell/exam") != "CKS-001B":
        raise RuntimeError("Refusing mutation on unmarked cluster")
    if n == 1:
        Path("/etc/falco/falco_rules.local.yaml").write_text("[]\n")
        run("systemctl", "restart", "falco")
    elif n in (2, 4, 13):
        ns, name = {2: ("outbound-guard", "exclude-ranges"), 4: ("storefront", "web-access"),
                    13: ("api-clients", "allow-service-egress")}[n]
        k("-n", ns, "delete", "networkpolicy", name, "--ignore-not-found")
    elif n in (3, 10):
        replace_pod("web-gate" if n == 3 else "runtime-web")
        if n == 10:
            Path("/var/lib/kubelet/seccomp/profiles/no-inspect.json").unlink(missing_ok=True)
    elif n == 5:
        k("patch", "clusterrole", "pipeline-agent-role", "--type=merge", "-p",
          json.dumps({"rules": original("ClusterRole", "pipeline-agent-role")["rules"]}))
    elif n == 6:
        k("-n", "immutable-workloads", "patch", "deployment", "report-web", "--type=strategic", "-p",
          json.dumps({"spec": {"template": {"spec": {"containers": [
              {"name": "report-web", "securityContext": {"readOnlyRootFilesystem": False}}]}}}}))
        k("-n", "immutable-workloads", "rollout", "status", "deployment/report-web", "--timeout=120s")
    elif n == 7:
        Path("/opt/course/ledger/ledger.Dockerfile").write_text(DOCKERFILE)
    elif n == 8:
        replace_pod("invoice-web")
        Path("/root/CKS/invoice-web.yaml").write_text(json.dumps(original("Pod", "invoice-web")))
    elif n == 9:
        k("label", "namespace", "finance-ledger", "pod-security.kubernetes.io/enforce-",
          "pod-security.kubernetes.io/enforce-version-", "pod-security.kubernetes.io/warn-")
    elif n == 11:
        candidate = json.loads(Path("/var/lib/painkiller/cks001b/candidate.json").read_text())
        for name in ("selected-produce.txt", "selected-produce.spdx.json"):
            (Path(candidate["home"]) / name).unlink(missing_ok=True)
    elif n == 12:
        k("-n", "secure-portal", "delete", "ingress", "portal-tls", "--ignore-not-found")
    elif n == 14:
        k("-n", "private-mesh", "delete", "peerauthentication", "default", "--ignore-not-found")
        k("label", "namespace", "private-mesh", "istio-injection-")
        k("-n", "private-mesh", "rollout", "restart", "deployment/north", "deployment/south")
        for name in ("north", "south"):
            k("-n", "private-mesh", "rollout", "status", "deployment/" + name, "--timeout=120s")
    elif n == 15:
        k("-n", "editor", "patch", "deployment", "editor-web", "--type=merge", "-p",
          json.dumps({"spec": {"template": {"spec": original("Deployment", "editor-web")["spec"]["template"]["spec"] |
                     {"volumes": None}}}}))
        k("-n", "editor", "rollout", "status", "deployment/editor-web", "--timeout=120s")
        k("-n", "editor", "delete", "secret", "editor-cert", "--ignore-not-found")
    elif n == 16:
        # Reuse the same path/runtime safety checks as initial fault injection.
        from etcd_fault import inject
        Path("/var/lib/painkiller/cks001b/etcd-fault").unlink()
        from workload_capture import Kubernetes
        inject(Kubernetes(), Path("/"))
    else:
        raise ValueError("invalid mutation number")


if __name__ == "__main__":
    remove(int(sys.argv[1]))
