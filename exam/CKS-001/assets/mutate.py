"""Trusted loss/restore fixtures; never used by grading or readiness."""
import json
import os
from pathlib import Path
import sys
import time
import yaml

from common import ROOT, STATE, apply, candidate_home, flagmap, flags, get, k, marked, run, wait_static, write


def wait_api():
    wait_static(["kube-apiserver", "kube-controller-manager", "kube-scheduler"])
    k("get", "--raw=/readyz")
    # Let independent audit/admission evidence observe this API generation.
    started = time.time()
    enabled = "ImagePolicyWebhook" in flagmap(flags("kube-apiserver")).get("enable-admission-plugins", "").split(",")
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        try:
            evidence = json.loads((STATE / "admission-evidence.json").read_text())
            tests = evidence.get("tests", {})
            negatives = all(
                tests.get(key, {}).get("code") == (1 if enabled else 0) and
                (not enabled or "image-gate: latest" in tests[key].get("error", ""))
                for key in ["latest", "other", "implicit"])
            if evidence.get("time", 0) >= started and tests.get("tagged", {}).get("code") == 0 and negatives:
                return
        except (FileNotFoundError, ValueError):
            pass
        time.sleep(1)
    raise RuntimeError("Independent witness did not recover after API restart")


def static(name, transform):
    path = Path(f"/etc/kubernetes/manifests/{name}.yaml")
    obj = yaml.safe_load(path.read_text())
    transform(obj["spec"]["containers"][0]["command"])
    tmp = path.with_name("." + path.name + ".tmp")
    text = yaml.safe_dump(obj)
    if text != path.read_text():
        tmp.write_text(text)
        tmp.replace(path)


def original(kind, name, ns):
    items = json.loads((ROOT / "setup.yaml").read_text())["items"]
    return next(o for o in items if o["kind"] == kind and o["metadata"]["name"] == name and o["metadata"].get("namespace") == ns)


def replace_pod(name, ns):
    k("-n", ns, "delete", "pod", name, "--ignore-not-found", "--wait=true", "--timeout=90s", timeout=100)
    apply(original("Pod", name, ns))
    k("-n", ns, "wait", "pod/" + name, "--for=condition=Ready", "--timeout=90s", timeout=100)


def remove(q):
    if q == "q1":
        items = json.loads((ROOT / "setup.yaml").read_text())["items"]
        for obj in items:
            if obj["kind"] == "ServiceAccount" and obj["metadata"].get("namespace") == "helix":
                apply(obj)
        replace_pod("frontend", "helix")
    elif q == "q2":
        Path("/root/CKS/secrets/BROKER_PASSWORD").unlink(missing_ok=True)
        replace_pod("connector", "relay")
    elif q == "q3":
        Path("/opt/security-review/findings.txt").unlink(missing_ok=True)
    elif q == "q4":
        k("delete", "pod", "guard-nginx", "--ignore-not-found", "--wait=true")
        Path("/var/lib/kubelet/seccomp/profiles/web-guard.json").replace("/root/CKS/web-guard.json")
    elif q == "q5":
        for path in ["/usr/lib/systemd/system/kubelet.service", "/var/lib/kubelet/config.yaml"]:
            os.chmod(path, 0o666)
        os.chown("/var/lib/etcd", 0, 0)
        for name in ["kube-controller-manager", "kube-scheduler"]:
            def profiling(cmd):
                cmd[:] = [a for a in cmd if not a.startswith("--profiling=")] + ["--profiling=true"]
            static(name, profiling)
        wait_static(["kube-controller-manager", "kube-scheduler"])
    elif q == "q6":
        write("/etc/falco/falco_rules.local.yaml", "[]\n")
        path = Path("/etc/falco/falco.yaml")
        obj = yaml.safe_load(path.read_text())
        obj["file_output"] = {"enabled": False, "filename": "/var/log/falco.log"}
        path.write_text(yaml.safe_dump(obj))
        run(["systemctl", "restart", "falco"])
    elif q == "q7":
        for name in ["selected-container.txt", "selected-image.spdx.json"]:
            (candidate_home() / name).unlink(missing_ok=True)
    elif q == "q8":
        k("-n", "batch-ops", "delete", "serviceaccount", "job-agent", "--ignore-not-found")
        k("-n", "batch-ops", "patch", "deployment", "janitor", "--type=json", "-p",
          json.dumps([{"op": "replace", "path": "/spec", "value": json.loads((STATE / "janitor.json").read_text())}]))
        k("-n", "batch-ops", "rollout", "status", "deployment/janitor", "--timeout=90s", timeout=100)
    elif q in ["q9", "q11"]:
        name, ns = ("portal", "locked-down") if q == "q9" else ("delta", "observatory")
        d = get("deployment", name, ns)
        cs = d["spec"]["template"]["spec"]["containers"]
        patch = [{"op": "remove", "path": f"/spec/template/spec/containers/{i}/securityContext"}
                 for i, c in enumerate(cs) if "securityContext" in c]
        k("-n", ns, "patch", "deployment", name, "--type=json", "-p", json.dumps(patch))
        if q == "q9":
            k("-n", ns, "scale", "deployment/portal", "--replicas=0")
            for _ in range(60):
                if not get("pods", ns=ns)["items"]:
                    break
                time.sleep(1)
            else:
                raise RuntimeError("Scaled-down portal Pods did not terminate")
        else:
            k("-n", ns, "rollout", "status", "deployment/delta", "--timeout=90s", timeout=100)
    elif q == "q10":
        k("-n", "catalog", "delete", "networkpolicy", "catalog-ingress")
    elif q == "q12":
        k("-n", "launch", "delete", "ingress", "shuttle-entry")
    elif q == "q13":
        for ns in ["team-a", "team-b", "team-c"]:
            apply(original("Role", "alex-pods", ns))
    elif q == "q14":
        def audit(cmd):
            cmd[:] = [a for a in cmd if not a.startswith("--audit-")]
        static("kube-apiserver", audit)
        write("/etc/kubernetes/security-policy.yaml", "apiVersion: audit.k8s.io/v1\nkind: Policy\nrules:\n- level: None\n")
        wait_api()
    elif q == "q15":
        def admission(cmd):
            for i, arg in enumerate(cmd):
                if arg.startswith("--enable-admission-plugins="):
                    cmd[i] = "--enable-admission-plugins=" + ",".join(
                        p for p in arg.split("=", 1)[1].split(",") if p != "ImagePolicyWebhook")
            cmd[:] = [a for a in cmd if not a.startswith("--admission-control-config-file=")]
        static("kube-apiserver", admission)
        wait_api()
        replace_pod("harbor-web", "harbor")
    else:
        raise ValueError(q)


if __name__ == "__main__":
    marked()
    action, question = sys.argv[1:]
    if action == "remove":
        remove(question)
    elif action == "restore":
        if question == "q9":
            k("-n", "locked-down", "scale", "deployment/portal", "--replicas=1")
        task = json.loads((ROOT / "tasks.json").read_text())[int(question[1:]) - 1]
        candidate = json.loads((STATE / "candidate.json").read_text())
        run(["runuser", "-u", candidate["user"], "--", "env", "HOME=" + candidate["home"],
             "KUBECONFIG=" + candidate["home"] + "/.kube/config",
             "bash", "-e", "-o", "pipefail", "-c", task["solution"]], timeout=180)
        if question in ["q14", "q15"]:
            wait_api()
    else:
        raise ValueError(action)
