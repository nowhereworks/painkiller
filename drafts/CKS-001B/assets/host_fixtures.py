"""Initialize exam-owned host files once, without overwriting candidate work."""
import hashlib
import json
import os
from pathlib import Path
import pwd
import subprocess
import tempfile

from fixture_objects import objects

FALCO_CONFIG = """engine:
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
"""

FALCO_SERVICE = """[Unit]
Description=CKS-001B Falco syscall monitor
After=containerd.service
[Service]
ExecStart=/usr/bin/falco -c /etc/falco/falco.yaml
Restart=on-failure
RestartSec=2
[Install]
WantedBy=multi-user.target
"""

DOCKERFILE = """FROM ubuntu:20.04
RUN apt-get update && apt-get install -y ca-certificates
COPY ledger /usr/local/bin/ledger
USER root
EXPOSE 8080
ENTRYPOINT /usr/local/bin/ledger
"""

GO_SOURCE = """package main
import ("fmt"; "net/http")
func main() {
  http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ledger ready") })
  if err := http.ListenAndServe(":8080", nil); err != nil { panic(err) }
}
"""


def path(root, absolute):
    return root / absolute.lstrip("/")


def create_once(target, content, mode=0o644):
    """An interrupted setup can be retried without replacing an existing file."""
    target.parent.mkdir(parents=True, exist_ok=True)
    try:
        fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    except FileExistsError:
        return
    with os.fdopen(fd, "w") as stream:
        stream.write(content)


def create_certificate(cert, key, hostname):
    if cert.exists() and key.exists():
        return
    if cert.exists() or key.exists():
        raise RuntimeError(f"Incomplete certificate pair: {cert} and {key}; refusing to replace either")
    cert.parent.mkdir(parents=True, exist_ok=True)
    key.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=cert.parent) as folder:
        tmp_cert, tmp_key = Path(folder) / "cert", Path(folder) / "key"
        subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "3650",
                        "-subj", f"/CN={hostname}", "-addext", f"subjectAltName=DNS:{hostname}",
                        "-keyout", str(tmp_key), "-out", str(tmp_cert)],
                       stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, check=True, timeout=30)
        tmp_key.chmod(0o600)
        tmp_cert.chmod(0o644)
        os.link(tmp_key, key)
        os.link(tmp_cert, cert)


def initialize(root):
    state = path(root, "/var/lib/painkiller/cks001b")
    if not (state / "authorized").is_file():
        raise RuntimeError("Refusing to initialize an unauthorized guest")
    if (state / "host-files").exists():
        return
    candidate_name = os.environ.get("SUDO_USER") or pwd.getpwuid(os.getuid()).pw_name
    candidate = pwd.getpwnam(candidate_name)
    create_once(state / "candidate.json", json.dumps({"user": candidate_name, "home": candidate.pw_dir}) + "\n", 0o600)
    rules = [path(root, "/etc/falco/" + name) for name in
             ("falco_rules.yaml", "falco-incubating_rules.yaml", "falco-sandbox_rules.yaml")]
    if not all(rule.is_file() for rule in rules):
        raise RuntimeError("Pinned Falco rules must be installed before host fixtures")
    digest = hashlib.sha256(b"".join(rule.read_bytes() for rule in rules)).hexdigest()
    create_once(state / "falco-default.sha256", digest + "\n")
    create_once(path(root, "/etc/falco/falco.yaml"), FALCO_CONFIG)
    create_once(path(root, "/etc/falco/falco_rules.local.yaml"), "[]\n")
    create_once(path(root, "/etc/systemd/system/falco.service"), FALCO_SERVICE)
    create_once(path(root, "/opt/course/ledger/ledger.Dockerfile"), DOCKERFILE)
    create_once(state / "ledger.go", GO_SOURCE)
    create_certificate(path(root, "/root/portal-client.crt"), path(root, "/root/portal-client.key"),
                       "editor.training.example")
    create_certificate(state / "portal-tls.crt", state / "portal-tls.key", "portal.training.example")
    create_once(path(root, "/root/CKS/portal-ca.crt"), (state / "portal-tls.crt").read_text())
    create_certificate(state / "canary-tls.crt", state / "canary-tls.key", "ingress-check.training.example")
    create_certificate(state / "network-tls.crt", state / "network-tls.key", "probe.training.example")
    invoice = next(o for o in objects() if o["kind"] == "Pod" and o["metadata"]["name"] == "invoice-web")
    create_once(path(root, "/root/CKS/invoice-web.yaml"), json.dumps(invoice, indent=2) + "\n")
    binary = path(root, "/opt/course/ledger/ledger")
    if not binary.exists():
        # Build to a temporary file and publish only after success.
        with tempfile.TemporaryDirectory(dir=binary.parent) as folder:
            result = Path(folder) / "ledger"
            env = dict(os.environ, GO111MODULE="off", CGO_ENABLED="0", GOPROXY="off", GOTOOLCHAIN="local")
            subprocess.run(["go", "build", "-o", str(result), str(state / "ledger.go")],
                           env=env, check=True, timeout=180)
            os.link(result, binary)
    create_once(state / "host-files", "complete\n", 0o600)


if __name__ == "__main__":
    initialize(Path("/"))
