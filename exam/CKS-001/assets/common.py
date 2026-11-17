"""Small CKS-001 helpers. Non-NotFound API/transport failures are capture errors."""
import json
import os
from pathlib import Path
import subprocess

ROOT = Path("/opt/painkiller/cks001")
STATE = Path("/var/lib/painkiller/cks001")
HOME = Path("/home/milkman")
NGINX = "nginx:1.28-alpine"
BUSY = "busybox:1.37.0"


def run(args, *, data=None, timeout=12, check=True):
    result = subprocess.run(args, input=data, text=True, capture_output=True, timeout=timeout)
    if check and result.returncode:
        # Do not let the generic grader mistake supporting failures for missing answers.
        raise RuntimeError("supporting command failed: " + result.stderr.replace("NotFound", "missing supporting object"))
    return result


def k(*args, **kwargs):
    return run(["kubectl", "--request-timeout=5s", *args], **kwargs)


def get(kind, name="", ns=None, optional=False):
    args = (["-n", ns] if ns else []) + ["get", kind]
    if name:
        args.append(name)
    result = k(*args, "-o", "json", *(["--ignore-not-found"] if optional else []))
    return json.loads(result.stdout) if result.stdout.strip() else {}


def apply(obj):
    k("apply", "-f", "-", data=json.dumps(obj))


def write(path, text, mode=0o644):
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(text)
    p.chmod(mode)


def pod(name, ns, image=BUSY, command=None):
    c = {"name": name, "image": image, "imagePullPolicy": "IfNotPresent"}
    if command is not None:
        c["command"] = command
    return {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": name, "namespace": ns,
            "labels": {"app": name}}, "spec": {"containers": [c]}}


def deployment(name, ns, image=BUSY, command=None):
    p = pod(name, ns, image, command)
    return {"apiVersion": "apps/v1", "kind": "Deployment", "metadata": p["metadata"],
            "spec": {"replicas": 1, "selector": {"matchLabels": {"app": name}},
                     "template": {"metadata": {"labels": {"app": name}}, "spec": p["spec"]}}}


def ready(obj):
    if not obj:
        return False
    if obj["kind"] == "Pod":
        return not obj.get("metadata", {}).get("deletionTimestamp") and any(c["type"] == "Ready" and c["status"] == "True"
                   for c in obj.get("status", {}).get("conditions", []))
    desired = obj["spec"].get("replicas", 1)
    status = obj.get("status", {})
    return (desired > 0 and status.get("readyReplicas", 0) >= desired and
            status.get("updatedReplicas", 0) >= desired and
            status.get("observedGeneration", 0) >= obj.get("metadata", {}).get("generation", 0))


def containers(obj):
    spec = obj.get("spec", {})
    return spec.get("template", {}).get("spec", spec).get("containers", [])


def exec_probe(ns, name, args, *, container=None):
    result = k("-n", ns, "exec", name, *(["-c", container] if container else []),
               "--", *args, check=False)
    if result.returncode and "command terminated with exit code" not in result.stderr:
        raise RuntimeError("supporting exec transport failed: " +
                           result.stderr.replace("NotFound", "missing supporting object"))
    return result


def flags(name):
    return yaml_file(f"/etc/kubernetes/manifests/{name}.yaml")["spec"]["containers"][0]["command"]


def yaml_file(path):
    import yaml
    return yaml.safe_load(Path(path).read_text())


def flagmap(args):
    return dict(a[2:].split("=", 1) for a in args if a.startswith("--") and "=" in a)


def marked():
    assert get("namespace", "cks001-system")["metadata"]["labels"]["painkiller-shell/exam"] == "CKS-001"


def candidate_home():
    return Path(json.loads((STATE / "candidate.json").read_text())["home"])


def default_rules_digest():
    import hashlib
    return hashlib.sha256(b"".join(Path("/etc/falco/" + name).read_bytes() for name in [
        "falco_rules.yaml", "falco-incubating_rules.yaml", "falco-sandbox_rules.yaml"])).hexdigest()


def wait_static(names, timeout=100):
    from datetime import datetime
    import time
    expected = {name: yaml_file(f"/etc/kubernetes/manifests/{name}.yaml")["spec"] for name in names}
    written = {name: int(Path(f"/etc/kubernetes/manifests/{name}.yaml").stat().st_mtime) for name in names}

    def matches(actual, wanted):
        if isinstance(wanted, dict):
            # Kubernetes omits false-valued optional booleans (for example readOnly).
            return isinstance(actual, dict) and all(
                (key in actual or value is False) and matches(actual.get(key, False), value)
                for key, value in wanted.items())
        if isinstance(wanted, list):
            return isinstance(actual, list) and len(actual) == len(wanted) and all(matches(a, w) for a, w in zip(actual, wanted))
        return actual == wanted

    def updated(pod, name):
        statuses = pod.get("status", {}).get("containerStatuses", [])
        return (ready(pod) and matches(pod["spec"], expected[name]) and bool(statuses) and
                all(datetime.fromisoformat(c.get("state", {}).get("running", {}).get("startedAt", "1970-01-01T00:00:00Z")).timestamp() >= written[name]
                    for c in statuses))

    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        result = k("-n", "kube-system", "get", "pods", "-o", "json", check=False)
        if result.returncode == 0:
            pods = json.loads(result.stdout)["items"]
            if all(any(p["metadata"]["name"].startswith(name + "-") and updated(p, name) for p in pods) for name in names):
                return
        time.sleep(1)
    raise RuntimeError("Updated static Pods did not become Ready")
