"""CKS-001 retry-safe installation. Only called on an ownership-checked new guest."""
import base64
import hashlib
import json
import os
from pathlib import Path
import pwd
import shutil
import time
import yaml

from common import ROOT, STATE, apply, default_rules_digest, deployment, get, k, pod, ready, run, wait_static, write, NGINX


def wait(kind, name, ns, seconds=180):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if ready(get(kind, name, ns, optional=True)):
            return
        time.sleep(2)
    raise RuntimeError(f"Fixture not ready: {ns}/{name}")


def cert(folder, name, sans):
    folder = Path(folder)
    folder.mkdir(parents=True, exist_ok=True)
    if not (folder / "tls.crt").exists():
        run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "3650",
             "-subj", f"/CN={name}", "-addext", f"subjectAltName={sans}",
             "-addext", "basicConstraints=critical,CA:TRUE",
             "-keyout", str(folder / "tls.key"), "-out", str(folder / "tls.crt")])
    return {"apiVersion": "v1", "kind": "Secret", "metadata": {"name": name},
            "type": "kubernetes.io/tls", "data": {
                f: base64.b64encode((folder / f).read_bytes()).decode() for f in ["tls.crt", "tls.key"]}}


def service(name, ns, port, target=None):
    apply({"apiVersion": "v1", "kind": "Service", "metadata": {"name": name, "namespace": ns},
           "spec": {"selector": {"app": name}, "ports": [{"port": port, "targetPort": target or port}]}})


def save_manifest(name, obj):
    p = Path(f"/etc/kubernetes/manifests/{name}.yaml")
    if yaml.safe_load(p.read_text()) == obj:
        return
    temp = p.with_name("." + p.name + ".tmp")
    temp.write_text(yaml.safe_dump(obj))
    temp.replace(p)


def main():
    STATE.mkdir(parents=True, exist_ok=True)
    assert len(get("nodes")["items"]) == 1
    user = os.environ.get("SUDO_USER", "milkman")
    home = pwd.getpwnam(user).pw_dir
    write(STATE / "candidate.json", json.dumps({"user": user, "home": home}))
    if not (STATE / "files").exists():
        write("/etc/apparmor.d/site-guard", """#include <tunables/global>
profile site-guard flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  file,
  network,
  capability,
  signal,
  deny /usr/share/nginx/html/staff/ r,
  deny /usr/share/nginx/html/staff/** r,
  deny /usr/share/nginx/html/**/staff/** r,
}
""")
        write("/root/CKS/web-guard.json", json.dumps({
            "defaultAction": "SCMP_ACT_ALLOW", "syscalls": [
                {"names": ["ptrace", "reboot", "kexec_load"], "action": "SCMP_ACT_ERRNO"}]}, indent=2))
        write(STATE / "seccomp.sha256", hashlib.sha256(Path("/root/CKS/web-guard.json").read_bytes()).hexdigest())
        review = {
            "Dockerfile.release": "FROM alpine:3.21.3\nARG RELEASE_TOKEN=release-demo-49\nRUN wget --header=\"Authorization: Bearer $RELEASE_TOKEN\" https://build.example/artifact\n",
            "Dockerfile.backup": "FROM alpine:3.21.3\nCOPY backup.key /tmp/backup.key\nRUN backup-client --key-file /tmp/backup.key\nRUN rm /tmp/backup.key\n",
            "Dockerfile.safe": "# syntax=docker/dockerfile:1\nFROM alpine:3.21.3\nRUN --mount=type=secret,id=repo-token,target=/run/secrets/repo-token fetch-client --token-file /run/secrets/repo-token\n",
            "job-debug.yaml": "apiVersion: batch/v1\nkind: Job\nmetadata: {name: diagnostics}\nspec:\n  template:\n    spec:\n      restartPolicy: Never\n      containers:\n      - name: inspect\n        image: busybox:1.37.0\n        command: [sh, -c, 'cat /credentials/password; sleep 5']\n        volumeMounts: [{name: auth, mountPath: /credentials, readOnly: true}]\n      volumes: [{name: auth, secret: {secretName: app-auth}}]\n",
            "pod-client.yaml": "apiVersion: v1\nkind: Pod\nmetadata: {name: sync-client}\nspec:\n  containers:\n  - name: sync\n    image: alpine:3.21.3\n    args: ['--password=sync-demo-61']\n",
            "pod-safe.yaml": "apiVersion: v1\nkind: Pod\nmetadata: {name: safe-client}\nspec:\n  containers:\n  - name: client\n    image: alpine:3.21.3\n    args: [--password-file=/credentials/password]\n    volumeMounts: [{name: auth, mountPath: /credentials, readOnly: true}]\n  volumes: [{name: auth, secret: {secretName: app-auth}}]\n"}
        for name, text in review.items():
            write("/opt/security-review/" + name, text)
        write("/etc/kubernetes/security-policy.yaml", "apiVersion: audit.k8s.io/v1\nkind: Policy\nrules:\n- level: None\n")
        write("/root/backup/kube-apiserver.yaml", Path("/etc/kubernetes/manifests/kube-apiserver.yaml").read_text())
        write("/etc/falco/falco.yaml", """engine:
  kind: modern_ebpf
plugins:
  - name: container
    library_path: /usr/share/falco/plugins/libcontainer.so
    init_config:
      label_max_len: 100
      with_size: false
load_plugins: [container]
rules_files:
  - /etc/falco/falco_rules.yaml
  - /etc/falco/falco-incubating_rules.yaml
  - /etc/falco/falco-sandbox_rules.yaml
  - /etc/falco/falco_rules.local.yaml
json_output: false
stdout_output:
  enabled: true
syslog_output:
  enabled: false
file_output:
  enabled: false
  filename: /var/log/falco.log
log_stderr: true
log_syslog: false
priority: debug
""")
        write("/etc/falco/falco_rules.local.yaml", "[]\n")
        write(STATE / "falco-default.sha256", default_rules_digest())
        write("/etc/systemd/system/falco.service", """[Unit]
Description=CKS-001 Falco syscall monitor
After=containerd.service
[Service]
ExecStart=/usr/bin/falco -c /etc/falco/falco.yaml
Restart=on-failure
RestartSec=2
[Install]
WantedBy=multi-user.target
""")
        (STATE / "files").touch()
    if not (STATE / "cluster").exists():
        nodes = get("nodes")["items"]
        for node in nodes:
            if any(t["key"] == "node-role.kubernetes.io/control-plane" for t in node["spec"].get("taints", [])):
                k("taint", "nodes", node["metadata"]["name"], "node-role.kubernetes.io/control-plane-")
        k("apply", "-f", str(ROOT / "setup.yaml"))
        write(STATE / "harbor.uid", get("pod", "harbor-web", "harbor")["metadata"]["uid"])
        # Keep admission enforcement present before the deliberately rejected Pod can be created.
        k("label", "namespace", "locked-down", "pod-security.kubernetes.io/enforce=restricted",
          "pod-security.kubernetes.io/enforce-version=latest", "--overwrite")
        k("-n", "locked-down", "delete", "pods", "--all", "--wait=true")
        # Preserve original fields for Q8's narrowly scoped patch requirement.
        write(STATE / "janitor.json", json.dumps(get("deployment", "janitor", "batch-ops")["spec"]))
        write(STATE / "labels.json", json.dumps({
            ns: get("namespace", ns)["metadata"]["labels"] for ns in ["locked-down", "catalog", "storage", "billing"]}))
        (STATE / "cluster").touch()
    if not (STATE / "services").exists():
        apply({"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": "traefik"}})
        apply({"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": "traefik", "namespace": "traefik"}})
        apply({"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole",
               "metadata": {"name": "cks001-traefik"}, "rules": [
                   {"apiGroups": [""], "resources": ["services", "secrets", "nodes"], "verbs": ["get", "list", "watch"]},
                   {"apiGroups": ["discovery.k8s.io"], "resources": ["endpointslices"], "verbs": ["get", "list", "watch"]},
                   {"apiGroups": ["networking.k8s.io"], "resources": ["ingresses", "ingressclasses"], "verbs": ["get", "list", "watch"]},
                   {"apiGroups": ["networking.k8s.io"], "resources": ["ingresses/status"], "verbs": ["update"]}]})
        apply({"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding",
               "metadata": {"name": "cks001-traefik"}, "roleRef": {
                   "apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": "cks001-traefik"},
               "subjects": [{"kind": "ServiceAccount", "name": "traefik", "namespace": "traefik"}]})
        apply({"apiVersion": "networking.k8s.io/v1", "kind": "IngressClass",
               "metadata": {"name": "nginx"}, "spec": {"controller": "traefik.io/ingress-controller"}})
        traefik = deployment("traefik", "traefik", "traefik:v3.6.0")
        spec = traefik["spec"]["template"]["spec"]
        spec["serviceAccountName"] = "traefik"
        spec["containers"][0]["args"] = ["--providers.kubernetesingress", "--providers.kubernetesingress.ingressclass=nginx",
                                       "--entrypoints.websecure.address=:8443", "--entrypoints.websecure.http.tls=true"]
        apply(traefik)
        service("traefik", "traefik", 443, 8443)
        secret = cert(STATE / "ingress-tls", "shuttle-tls", "DNS:shuttle.lab")
        secret["metadata"]["namespace"] = "launch"
        apply(secret)
        shutil.copyfile(STATE / "ingress-tls/tls.crt", "/root/CKS/ingress-ca.crt")
        service("image-gate", "cks001-system", 8443)
        gate_ip = get("service", "image-gate", "cks001-system")["spec"]["clusterIP"]
        secret = cert(STATE / "gate-tls", "image-gate-tls",
                      f"IP:{gate_ip},DNS:image-gate.cks001-system.svc")
        secret["metadata"]["namespace"] = "cks001-system"
        apply(secret)
        webhook = deployment("image-gate", "cks001-system", "python:3.12.12-alpine3.22",
                             ["python3", "/code/webhook.py"])
        spec = webhook["spec"]["template"]["spec"]
        spec["containers"][0]["volumeMounts"] = [{"name": "code", "mountPath": "/code", "readOnly": True},
                                               {"name": "tls", "mountPath": "/tls", "readOnly": True}]
        spec["volumes"] = [{"name": "code", "hostPath": {"path": str(ROOT)}},
                           {"name": "tls", "secret": {"secretName": "image-gate-tls"}}]
        apply(webhook)
        kubeconfig = {"apiVersion": "v1", "kind": "Config", "clusters": [{"name": "gate", "cluster": {
            "server": f"https://{gate_ip}:8443/image-review", "certificate-authority-data":
                base64.b64encode((STATE / "gate-tls/tls.crt").read_bytes()).decode()}}],
            "users": [{"name": "apiserver", "user": {}}],
            "contexts": [{"name": "gate", "context": {"cluster": "gate", "user": "apiserver"}}], "current-context": "gate"}
        write("/root/CKS/ImageGate/admission-kubeconfig.yaml", yaml.safe_dump(kubeconfig))
        # This mount is an explicit prerequisite, not part of the candidate's task.
        p = Path("/etc/kubernetes/manifests/kube-apiserver.yaml")
        obj = yaml.safe_load(p.read_text())
        # This disposable singleton has no in-flight production clients to drain.
        # Fast reloads avoid missed node leases while audit/admission flags change.
        obj["spec"]["terminationGracePeriodSeconds"] = 5
        obj["spec"]["volumes"] = [v for v in obj["spec"]["volumes"] if v["name"] != "image-gate"]
        obj["spec"]["containers"][0]["volumeMounts"] = [
            v for v in obj["spec"]["containers"][0]["volumeMounts"] if v["name"] != "image-gate"]
        obj["spec"]["volumes"].append({"name": "image-gate", "hostPath": {"path": "/root/CKS/ImageGate", "type": "Directory"}})
        obj["spec"]["containers"][0]["volumeMounts"].append({
            "name": "image-gate", "mountPath": "/etc/admission-controllers", "readOnly": True})
        save_manifest("kube-apiserver", obj)
        # The backup includes the admission mount, but no candidate audit/admission answers.
        write("/root/backup/kube-apiserver.yaml", yaml.safe_dump(obj))
        wait_static(["kube-apiserver"])
        for name, ns in [("traefik", "traefik"), ("image-gate", "cks001-system"),
                         ("toolkit", "inventory"), ("shuttle", "launch"), ("catalog-web", "catalog")]:
            wait("deployment", name, ns)
        # Archive real image content, not an invented package list.
        for image in ["busybox:1.37.0", "alpine/curl:8.12.1", "alpine:3.21.3"]:
            name = image.replace("/", "_").replace(":", "_")
            path = f"/root/ImageTarballs/{name}.tar"
            Path(path).parent.mkdir(parents=True, exist_ok=True)
            if not Path(path).exists():
                run(["skopeo", "copy", "--override-os=linux", "--override-arch=amd64",
                     f"docker://{image}", f"docker-archive:{path}.partial:{image}"], timeout=180)
                Path(path + ".partial").replace(path)
        # Save a trusted expected SBOM identity for semantic grading (candidate output may differ in formatting).
        run(["bom", "generate", "--image-archive", "/root/ImageTarballs/alpine_curl_8.12.1.tar",
             "--format", "json", "--output", str(STATE / "expected.spdx.json")], timeout=180)
        # Periodic real events are independent of read-only grading.
        audit_canary = deployment("cks001-audit-canary", "default", NGINX)
        apply(audit_canary)
        write("/etc/systemd/system/cks001-witness.service", """[Unit]
Description=CKS-001 independent audit and admission evidence
After=kubelet.service
[Service]
Environment=KUBECONFIG=/etc/kubernetes/admin.conf
ExecStart=/usr/bin/python3 -B /opt/painkiller/cks001/witness.py
Restart=always
RestartSec=2
[Install]
WantedBy=multi-user.target
""")
        run(["systemctl", "daemon-reload"])
        run(["systemctl", "enable", "--now", "falco", "cks001-witness"])
        # Real binary writes in a disposable container; no hostPath or privileged access.
        suspicious = pod("binary-writer", "runtime-watch", "httpd:2.4-alpine",
                         ["sh", "-c", "while true; do cp /bin/sleep /usr/bin/cks001-sleep; sleep 3; done"])
        apply(suspicious)
        wait("pod", "binary-writer", "runtime-watch")
        (STATE / "services").touch()
    if not (STATE / "faults").exists():
        # Inject once, after supporting infrastructure has converged.
        for path in ["/usr/lib/systemd/system/kubelet.service", "/var/lib/kubelet/config.yaml"]:
            os.chmod(path, 0o666)
        os.chown("/var/lib/etcd", 0, 0)
        for name in ["kube-controller-manager", "kube-scheduler"]:
            obj = yaml.safe_load(Path(f"/etc/kubernetes/manifests/{name}.yaml").read_text())
            cmd = obj["spec"]["containers"][0]["command"]
            # A singleton control plane needs no leader lease across API reloads.
            cmd[:] = [a for a in cmd if not a.startswith(("--profiling=", "--leader-elect="))]
            cmd += ["--profiling=true", "--leader-elect=false"]
            if name == "kube-controller-manager":
                cmd[:] = [a for a in cmd if not a.startswith("--node-monitor-grace-period=")]
                cmd.append("--node-monitor-grace-period=120s")
            save_manifest(name, obj)
        (STATE / "faults").touch()
    # Fault injection restarts two components. Wait here, not in the fast readiness check.
    wait_static(["kube-controller-manager", "kube-scheduler"])
    for name, ns in [("frontend", "helix"), ("connector", "relay"), ("observer", "cks001-system")]:
        wait("pod", name, ns)
    for name, ns in [("janitor", "batch-ops"), ("delta", "observatory")]:
        wait("deployment", name, ns)


if __name__ == "__main__":
    main()
