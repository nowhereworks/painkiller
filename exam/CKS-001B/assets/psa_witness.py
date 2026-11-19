"""Independent dry-run evidence for Q9; grading itself only reads the result."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time

from host_fixtures import path
from workload_capture import Kubernetes

NAMESPACE = "finance-ledger"
STATE = Path("/var/lib/painkiller/cks001b")
UNIT = """[Unit]
Description=CKS-001B Pod Security Admission witness
After=kubelet.service
[Service]
Environment=KUBECONFIG=/etc/kubernetes/admin.conf
ExecStart=/usr/bin/python3 -B /opt/painkiller/cks001b/psa_witness.py
Restart=always
RestartSec=2
[Install]
WantedBy=multi-user.target
"""


def dry_run(obj):
    result = subprocess.run(
        ["kubectl", "--request-timeout=5s", "-n", NAMESPACE, "create", "--dry-run=server", "-f", "-", "-o", "json"],
        input=json.dumps(obj), text=True, capture_output=True, timeout=10,
    )
    if result.returncode == 0:
        response = json.loads(result.stdout)
        if response.get("kind") != "Pod":
            raise RuntimeError("admission dry run returned a non-Pod object")
        return "accepted"
    if "violates PodSecurity" in result.stderr and "Forbidden" in result.stderr:
        return "psa_denied"
    raise RuntimeError("admission witness failed: " + result.stderr.replace("NotFound", "missing supporting object"))


def sample(kube, test, state, now):
    marker = kube.get("namespace", "cks001b-system")
    if marker.get("metadata", {}).get("labels", {}).get("painkiller-shell/exam") != "CKS-001B":
        raise RuntimeError("Refusing PSA witness on unmarked cluster")
    before = kube.get("namespace", NAMESPACE)
    deployment = kube.get("deployment", "ledger-web", NAMESPACE)
    namespace_meta = before["metadata"]
    deployment_meta = deployment["metadata"]
    template = deployment["spec"]["template"]
    compliant = {"apiVersion": "v1", "kind": "Pod",
                 "metadata": {"generateName": "cks001b-compliant-", "namespace": NAMESPACE,
                              "labels": template.get("metadata", {}).get("labels", {}),
                              "annotations": template.get("metadata", {}).get("annotations", {})},
                 "spec": template["spec"]}
    privileged = {"apiVersion": "v1", "kind": "Pod",
                  "metadata": {"generateName": "cks001b-privileged-", "namespace": NAMESPACE},
                  "spec": {"containers": [{"name": "probe", "image": "busybox:1.37.0",
                                           "securityContext": {"privileged": True}}]}}
    tests = {"privileged": test(privileged), "template": test(compliant)}
    after = kube.get("namespace", NAMESPACE)
    current = kube.get("deployment", "ledger-web", NAMESPACE)
    if (after["metadata"]["uid"] != namespace_meta["uid"] or
            after["metadata"]["resourceVersion"] != namespace_meta["resourceVersion"] or
            current["metadata"]["uid"] != deployment_meta["uid"] or
            current["metadata"]["generation"] != deployment_meta["generation"]):
        raise RuntimeError("namespace or workload changed during admission witness")
    evidence = {"time": now(), "namespace_uid": namespace_meta["uid"],
                "namespace_version": namespace_meta["resourceVersion"],
                "deployment_uid": deployment_meta["uid"], "deployment_generation": deployment_meta["generation"],
                "tests": tests}
    state.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode="w", dir=state, prefix=".psa-evidence.", delete=False) as stream:
        temporary = Path(stream.name)
        json.dump(evidence, stream)
    try:
        temporary.chmod(0o600)
        os.replace(temporary, state / "psa-evidence.json")
    finally:
        temporary.unlink(missing_ok=True)


if __name__ == "__main__":
    state = path(Path("/"), str(STATE))
    if not (state / "authorized").is_file():
        raise SystemExit("Refusing PSA witness on unauthorized guest")
    kube = Kubernetes()
    while True:
        try:
            sample(kube, dry_run, state, time.time)
        except Exception as error:
            print(str(error).replace("NotFound", "missing supporting object"), flush=True)
        time.sleep(5)
