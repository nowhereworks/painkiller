"""Read-only readiness checks for the Q5 baseline and Q9 independent witness."""
from pathlib import Path
import subprocess
import sys
import time

from capture_files import json_file
from host_fixtures import path
from psa_capture import witness_valid
from rbac_capture import atoms, forbidden, permitted_baseline, required_original_permissions, subset
from workload_capture import Kubernetes


def service_active():
    return subprocess.run(["systemctl", "is-active", "--quiet", "cks001b-psa-witness.service"],
                          timeout=5, check=False).returncode == 0


def inspect(kube=None, root=Path("/"), now=None, active=service_active):
    kube = kube or Kubernetes()
    now = time.time() if now is None else now
    state = path(root, "/var/lib/painkiller/cks001b")
    failures = []
    if not kube.get("serviceaccount", "pipeline-agent", "delivery", optional=True):
        failures.append("pipeline service account missing")
    baseline = json_file(state / "rbac-baseline.json")
    if not isinstance(baseline, dict) or baseline.get("version") != 1 or not isinstance(baseline.get("grants"), list):
        failures.append("RBAC baseline missing or invalid")
    else:
        try:
            expected = {tuple(item) for item in baseline["grants"]}
            if any(len(atom) != 6 or not all(isinstance(x, str) for x in atom) for atom in expected):
                raise ValueError("invalid atom")
            current = atoms(kube)
            desired = permitted_baseline(current)
            if (not required_original_permissions(expected) or not subset(expected, desired) or
                    not subset(desired, expected) or not all(any(forbidden(atom, name) for atom in current)
                    for name in ("rolebindings", "clusterrolebindings"))):
                failures.append("RBAC starting permissions or intentional fault not intact")
        except (TypeError, ValueError):
            failures.append("RBAC baseline invalid")
    if not (state / "psa-witness-installed").is_file() or not active():
        failures.append("PSA witness service unavailable")
    namespace = kube.get("namespace", "finance-ledger")
    deployment = kube.get("deployment", "ledger-web", "finance-ledger")
    labels = namespace.get("metadata", {}).get("labels", {})
    if any(labels.get("pod-security.kubernetes.io/" + suffix) for suffix in ("enforce", "enforce-version", "warn")):
        failures.append("finance-ledger already has PSA answer labels")
    evidence = json_file(state / "psa-evidence.json")
    if not witness_valid(evidence, namespace, deployment, now):
        failures.append("PSA witness evidence missing, stale or out of date")
    elif evidence["tests"] != {"privileged": "accepted", "template": "accepted"}:
        failures.append("PSA initial admission state is not as expected")
    return failures


if __name__ == "__main__":
    problems = inspect()
    if problems:
        print("\n".join(problems), file=sys.stderr)
        sys.exit(1)
    print("CKS-001B admission subset ready (not full exam readiness)")
