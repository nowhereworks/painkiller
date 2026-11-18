"""Read-only captures for CKS-001B Q3, Q6, Q8, Q10 and Q15.

Only candidate state is returned. Private key bytes and subprocess output never
appear in capture JSON. Every Kubernetes request and exec is bounded.
"""
import base64
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

import yaml

from capture_files import capture as capture_files, read_file
from host_fixtures import path


def command(args, *, timeout=10, allow_exit=False):
    result = subprocess.run(args, text=True, capture_output=True, timeout=timeout)
    if result.returncode and not allow_exit:
        # Generic grading treats NotFound as an absent answer. An API/transport
        # failure must not be mistaken for one, including exec transport errors.
        raise RuntimeError("supporting command failed: " + result.stderr.replace("NotFound", "missing supporting object"))
    return result


class Kubernetes:
    def get(self, kind, name="", namespace=None, optional=False):
        args = ["kubectl", "--request-timeout=5s"]
        if namespace:
            args += ["-n", namespace]
        args += ["get", kind]
        if name:
            args.append(name)
        args += ["-o", "json"]
        if optional:
            args.append("--ignore-not-found")
        result = command(args)
        return json.loads(result.stdout) if result.stdout.strip() else {}

    def exec(self, namespace, name, args, container=None):
        cmd = ["kubectl", "--request-timeout=5s", "-n", namespace, "exec", name]
        if container:
            cmd += ["-c", container]
        result = command(cmd + ["--", *args], allow_exit=True)
        if result.returncode and "command terminated with exit code" not in result.stderr:
            raise RuntimeError("supporting exec transport failed: " +
                               result.stderr.replace("NotFound", "missing supporting object"))
        return result


def ready_pod(pod):
    return bool(pod) and not pod.get("metadata", {}).get("deletionTimestamp") and any(
        c.get("type") == "Ready" and c.get("status") == "True"
        for c in pod.get("status", {}).get("conditions", []))


def ready_deployment(deploy):
    if not deploy or deploy.get("metadata", {}).get("deletionTimestamp"):
        return False
    spec, status = deploy.get("spec", {}), deploy.get("status", {})
    desired = spec.get("replicas", 1)
    return (isinstance(desired, int) and desired > 0 and
            status.get("observedGeneration", 0) >= deploy.get("metadata", {}).get("generation", 0) and
            status.get("updatedReplicas", 0) >= desired and status.get("readyReplicas", 0) >= desired and
            status.get("availableReplicas", 0) >= desired)


def deployment_pods(kube, deploy, namespace):
    """Only Ready, current-revision pods belonging to the candidate Deployment."""
    if not ready_deployment(deploy):
        return []
    uid = deploy.get("metadata", {}).get("uid")
    revision = deploy.get("metadata", {}).get("annotations", {}).get("deployment.kubernetes.io/revision")
    if not uid or not revision:
        return []
    sets = kube.get("replicasets", namespace=namespace).get("items", [])
    current = {item["metadata"]["uid"] for item in sets if
               item.get("metadata", {}).get("annotations", {}).get("deployment.kubernetes.io/revision") == revision and
               any(owner.get("uid") == uid and owner.get("controller") is True
                   for owner in item.get("metadata", {}).get("ownerReferences", []))}
    pods = kube.get("pods", namespace=namespace).get("items", [])
    selected = [pod for pod in pods if ready_pod(pod) and any(
        owner.get("uid") in current and owner.get("controller") is True
        for owner in pod.get("metadata", {}).get("ownerReferences", []))]
    if len(selected) < deploy.get("spec", {}).get("replicas", 1):
        return []
    # Bound the number of commands irrespective of candidate-controlled replicas.
    return selected[:1]


def effective(spec, container, field):
    return container.get("securityContext", {}).get(field,
           spec.get("securityContext", {}).get(field))


def proc_fields(text):
    fields = {}
    for line in text.splitlines():
        if ":" in line:
            key, value = line.split(":", 1)
            fields[key] = value.strip()
    return fields


def mount_flags(text):
    result = {}
    for line in text.splitlines():
        left = line.split(" - ", 1)[0].split()
        if len(left) >= 6:
            result[left[4]] = set(left[5].split(","))
    return result


def web_probe(kube, namespace, pod, container, port):
    result = kube.exec(namespace, pod, ["wget", "-T", "3", "-q", "-O", "/dev/null",
                                      f"http://127.0.0.1:{port}/"], container)
    return result.returncode == 0


def capture(question, kube=None, root=Path("/")):
    kube = kube or Kubernetes()
    out = {"failures": []}

    def check(key, condition, message):
        out[key] = bool(condition)
        if not condition:
            out["failures"].append(message)

    if question == "q3":
        obj = kube.get("pod", "web-gate", "workload-identity", optional=True)
        spec = obj.get("spec", {})
        containers = spec.get("containers", [])
        container = next((c for c in containers if c.get("name") == "web-gate"), {})
        security = container.get("securityContext", {})
        fields = {}
        healthy = ready_pod(obj)
        if healthy and container:
            status = kube.exec("workload-identity", "web-gate", ["cat", "/proc/1/status"], container["name"])
            if status.returncode == 0:
                fields = proc_fields(status.stdout)
        check("image", container.get("image") == "nginxinc/nginx-unprivileged:alpine",
              "Use the specified nginx-unprivileged image.")
        check("escalation", security.get("allowPrivilegeEscalation") is False and
              fields.get("NoNewPrivs") == "1", "Disable privilege escalation in the running container.")
        check("nonroot", effective(spec, container, "runAsNonRoot") is True,
              "Set runAsNonRoot to true.")
        check("uid", effective(spec, container, "runAsUser") == 101 and
              bool(re.fullmatch(r"101\s+101\s+101\s+101", fields.get("Uid", ""))), "Run as UID 101.")
        check("gid", effective(spec, container, "runAsGroup") == 101 and
              bool(re.fullmatch(r"101\s+101\s+101\s+101", fields.get("Gid", ""))), "Run as GID 101.")
        check("port", any(p.get("containerPort") == 8080 and p.get("protocol", "TCP") == "TCP"
                          for p in container.get("ports", [])), "Declare TCP container port 8080.")
        check("serving", healthy and bool(container) and web_probe(kube, "workload-identity",
              "web-gate", container["name"], 8080), "Leave the Pod Ready and serving HTTP on port 8080.")

    elif question == "q6":
        deploy = kube.get("deployment", "report-web", "immutable-workloads", optional=True)
        spec = deploy.get("spec", {}).get("template", {}).get("spec", {})
        containers = spec.get("containers", [])
        pods = deployment_pods(kube, deploy, "immutable-workloads")
        requested = {"/tmp", "/var/log", "/var/cache/nginx", "/var/run"}
        declared = bool(containers) and all(c.get("securityContext", {}).get("readOnlyRootFilesystem") is True
                                           for c in containers)
        mounts = bool(pods) and bool(containers)
        serving = bool(pods) and bool(containers)
        if pods and containers:
            name = pods[0]["metadata"]["name"]
            for container in containers:
                result = kube.exec("immutable-workloads", name, ["cat", "/proc/1/mountinfo"], container["name"])
                actual = mount_flags(result.stdout) if result.returncode == 0 else {}
                mounts = mounts and "ro" in actual.get("/", set()) and all(
                    "rw" in actual.get(mount, set()) for mount in requested)
            serving = web_probe(kube, "immutable-workloads", name, containers[0]["name"], 80)
        check("readonly", declared, "Set readOnlyRootFilesystem on every container.")
        check("mounts", mounts, "The running root must be read-only and each requested volume writable.")
        check("serving", declared and mounts and serving,
              "Leave the hardened Deployment current, Ready and serving HTTP.")

    elif question == "q8":
        obj = kube.get("pod", "invoice-web", "default", optional=True)
        candidate_file = read_file(path(root, "/root/CKS/invoice-web.yaml"))
        try:
            doc = yaml.safe_load(candidate_file) or {}
        except yaml.YAMLError:
            doc = {}
        pod_containers = obj.get("spec", {}).get("containers", [])
        file_spec = doc.get("spec", {}) if isinstance(doc, dict) else {}
        file_containers = file_spec.get("containers", []) if isinstance(file_spec, dict) else []
        if not isinstance(file_containers, list) or not all(isinstance(c, dict) for c in file_containers):
            file_containers = []
        def capabilities(container):
            security = container.get("securityContext")
            value = security.get("capabilities") if isinstance(security, dict) else None
            return value if isinstance(value, dict) else {}

        caps = [capabilities(c) for c in pod_containers + file_containers]
        have_both = len(pod_containers) == 1 and len(file_containers) == 1
        drop = have_both and all(isinstance(c.get("drop"), list) and "ALL" in c["drop"] for c in caps)
        add = have_both and all(c.get("add", []) == ["NET_BIND_SERVICE"] for c in caps)
        check("drop", drop,
              "Drop ALL capabilities in the manifest and running Pod.")
        check("add", add,
              "Add back only NET_BIND_SERVICE in the manifest and running Pod.")
        check("report", drop and add and capture_files("q8", root)["report"],
              "Save a passing Kubesec report for the corrected invoice-web.")
        healthy = ready_pod(obj) and bool(pod_containers)
        check("serving", drop and add and healthy and web_probe(
            kube, "default", "invoice-web", pod_containers[0]["name"], 8080),
            "Leave the corrected invoice-web Ready and serving HTTP on port 8080.")

    elif question == "q10":
        files = capture_files("q10", root)
        for key, message in (("default_allow", "Set the profile default to allow."),
                             ("debug_blocked", "Block both debugging syscalls with errno."),
                             ("other_allowed", "Do not deny additional syscalls.")):
            check(key, files[key], message)
        obj = kube.get("pod", "runtime-web", "runtime-shield", optional=True)
        spec = obj.get("spec", {})
        containers = spec.get("containers", [])
        container = next((c for c in containers if c.get("name") == "runtime-web"), {})
        profile = effective(spec, container, "seccompProfile") or {}
        selected = profile.get("type") == "Localhost" and profile.get("localhostProfile") == "profiles/no-inspect.json"
        fields = {}
        if ready_pod(obj) and container:
            result = kube.exec("runtime-shield", "runtime-web", ["cat", "/proc/1/status"], container["name"])
            if result.returncode == 0:
                fields = proc_fields(result.stdout)
        check("profile", selected and fields.get("Seccomp") == "2",
              "Attach the Localhost profile and leave the running process in seccomp filter mode.")
        check("serving", selected and fields.get("Seccomp") == "2" and
            all(files[key] for key in ("default_allow", "debug_blocked", "other_allowed")) and
            ready_pod(obj) and bool(container) and web_probe(
            kube, "runtime-shield", "runtime-web", container["name"], 80),
            "Leave runtime-web Ready and serving HTTP.")

    elif question == "q15":
        secret = kube.get("secret", "editor-cert", "editor", optional=True)
        deploy = kube.get("deployment", "editor-web", "editor", optional=True)
        expected = {}
        for key, filename in (("tls.crt", "/root/portal-client.crt"), ("tls.key", "/root/portal-client.key")):
            try:
                expected[key] = path(root, filename).read_bytes()
            except FileNotFoundError:
                raise RuntimeError("supporting certificate fixture is missing") from None
        matches = secret.get("type") == "kubernetes.io/tls"
        for key, data in expected.items():
            try:
                matches = matches and base64.b64decode(secret.get("data", {}).get(key, ""), validate=True) == data
            except (ValueError, TypeError):
                matches = False
        check("secret", matches, "Create editor-cert from the supplied certificate and key.")
        spec = deploy.get("spec", {}).get("template", {}).get("spec", {})
        volumes = spec.get("volumes", [])
        candidates = [v for v in volumes if v.get("name") == "editor-tls" and
                      v.get("secret", {}).get("secretName") == "editor-cert"]
        containers = spec.get("containers", [])
        selected = [c for c in containers if any(m.get("name") == "editor-tls" and
                    m.get("mountPath") == "/etc/editor/tls" for m in c.get("volumeMounts", []))]
        pods = deployment_pods(kube, deploy, "editor")
        mounted = bool(candidates and selected and pods and matches)
        if mounted:
            hashes = kube.exec("editor", pods[0]["metadata"]["name"],
                               ["sha256sum", "/etc/editor/tls/tls.crt", "/etc/editor/tls/tls.key"],
                               selected[0]["name"])
            wanted = [hashlib.sha256(expected[key]).hexdigest() for key in ("tls.crt", "tls.key")]
            got = [line.split()[0] for line in hashes.stdout.splitlines() if line.split()]
            mounted = hashes.returncode == 0 and got == wanted
        check("mounted", mounted, "Mount editor-tls at /etc/editor/tls in a healthy Deployment.")

    else:
        raise ValueError("unsupported capture: " + question)
    return out


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: workload_capture.py {q3|q6|q8|q10|q15}")
    print(json.dumps(capture(sys.argv[1])))
