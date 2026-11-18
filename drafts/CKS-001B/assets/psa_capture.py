"""Read-only PSA labels, witness evidence and compliant workload capture."""
import json
from pathlib import Path
import sys
import time

from capture_files import json_file
from host_fixtures import path
from workload_capture import Kubernetes, deployment_pods, web_probe


def witness_valid(evidence, namespace, deployment, now):
    if not isinstance(evidence, dict):
        return False
    timestamp = evidence.get("time")
    if isinstance(timestamp, bool) or not isinstance(timestamp, (float, int)) or not 0 <= now - timestamp <= 30:
        return False
    ns_meta, dep_meta = namespace.get("metadata", {}), deployment.get("metadata", {})
    tests = evidence.get("tests")
    if not isinstance(tests, dict):
        return False
    return (bool(ns_meta.get("uid")) and bool(dep_meta.get("uid")) and
            evidence.get("namespace_uid") == ns_meta.get("uid") and
            evidence.get("namespace_version") == ns_meta.get("resourceVersion") and
            evidence.get("deployment_uid") == dep_meta.get("uid") and
            evidence.get("deployment_generation") == dep_meta.get("generation") and
            tests.get("privileged") in ("accepted", "psa_denied") and
            tests.get("template") in ("accepted", "psa_denied"))


def capture(kube=None, root=Path("/"), now=None):
    kube = kube or Kubernetes()
    now = time.time() if now is None else now
    namespace = kube.get("namespace", "finance-ledger", optional=True)
    labels = namespace.get("metadata", {}).get("labels", {})
    restricted = labels.get("pod-security.kubernetes.io/enforce") == "restricted"
    version = labels.get("pod-security.kubernetes.io/enforce-version") == "latest"
    warning = labels.get("pod-security.kubernetes.io/warn") == "baseline"
    out = {"failures": []}

    def check(key, condition, message):
        out[key] = bool(condition)
        if not condition:
            out["failures"].append(message)

    check("enforce", restricted, "Enforce the restricted Pod Security level.")
    check("version", restricted and version, "Use the latest enforce version.")
    check("warning", warning, "Set Pod Security warnings to baseline.")
    rejected = compliant = False
    if restricted:
        deployment = kube.get("deployment", "ledger-web", "finance-ledger", optional=True)
        if deployment:
            evidence = json_file(path(root, "/var/lib/painkiller/cks001b/psa-evidence.json"))
            if not witness_valid(evidence, namespace, deployment, now):
                raise RuntimeError("independent PSA witness evidence is unavailable, stale or out of date")
            rejected = evidence["tests"]["privileged"] == "psa_denied"
            if evidence["tests"]["template"] == "accepted":
                pods = deployment_pods(kube, deployment, "finance-ledger")
                containers = deployment.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
                compliant = bool(pods and containers) and web_probe(
                    kube, "finance-ledger", pods[0]["metadata"]["name"], containers[0]["name"], 8080)
    check("rejected", restricted and rejected, "The API server must reject a privileged Pod under restricted PSA.")
    check("compliant", restricted and compliant, "Keep the compliant Deployment admitted and serving HTTP.")
    return out


if __name__ == "__main__":
    print(json.dumps(capture()))
