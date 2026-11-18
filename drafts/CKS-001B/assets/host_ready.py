"""Read-only readiness for the implemented host-file subset, not the whole lab."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

from host_fixtures import DOCKERFILE, path


def cert_matches(cert, key, hostname):
    try:
        subprocess.run(["openssl", "x509", "-in", str(cert), "-noout", "-checkend", "0"],
                       check=True, capture_output=True, timeout=5)
        validity = subprocess.run(["openssl", "x509", "-in", str(cert), "-noout", "-checkhost", hostname],
                                  check=True, capture_output=True, timeout=5)
        # OpenSSL prints a mismatch but still exits successfully for -checkhost.
        if b"does match certificate" not in validity.stdout:
            return False
        public = subprocess.run(["openssl", "x509", "-in", str(cert), "-pubkey", "-noout"],
                                check=True, capture_output=True, timeout=5).stdout
        key_public = subprocess.run(["openssl", "pkey", "-in", str(key), "-pubout"],
                                    check=True, capture_output=True, timeout=5).stdout
        return public == key_public
    except (OSError, subprocess.SubprocessError):
        return False


def service_active(name):
    result = subprocess.run(["systemctl", "is-active", "--quiet", name], timeout=5, check=False)
    return result.returncode == 0


def check(root, active=service_active):
    state = path(root, "/var/lib/painkiller/cks001b")
    failures = []

    def require(condition, message):
        if not condition:
            failures.append(message)

    require((state / "authorized").is_file(), "guest authorization marker missing")
    require((state / "packages").is_file(), "dependency installation incomplete")
    require((state / "host-files").is_file(), "host fixture initialization incomplete")
    for binary in ("bom", "kube-bench", "kubesec", "istioctl"):
        require(os.access(path(root, "/usr/local/bin/" + binary), os.X_OK), f"{binary} missing")
    for binary in ("/usr/bin/falco", "/opt/course/ledger/ledger"):
        require(os.access(path(root, binary), os.X_OK), f"{binary} missing")
    dockerfile = path(root, "/opt/course/ledger/ledger.Dockerfile")
    require(dockerfile.is_file() and dockerfile.read_text() == DOCKERFILE, "initial Dockerfile missing or changed")
    invoice = path(root, "/root/CKS/invoice-web.yaml")
    try:
        obj = json.loads(invoice.read_text())
        capabilities = obj["spec"]["containers"][0]["securityContext"]["capabilities"]
        require("NET_RAW" in capabilities.get("add", []) and "ALL" not in capabilities.get("drop", []),
                "invoice manifest capability fault missing")
    except (OSError, ValueError, KeyError, IndexError, TypeError):
        failures.append("invoice manifest missing or invalid")
    for cert, key, host in [
        (path(root, "/root/portal-client.crt"), path(root, "/root/portal-client.key"), "editor.training.example"),
        (state / "portal-tls.crt", state / "portal-tls.key", "portal.training.example"),
    ]:
        require(cert_matches(cert, key, host), f"certificate pair for {host} missing or invalid")
    rules = [path(root, "/etc/falco/" + name) for name in
             ("falco_rules.yaml", "falco-incubating_rules.yaml", "falco-sandbox_rules.yaml")]
    expected = state / "falco-default.sha256"
    if all(p.is_file() for p in rules) and expected.is_file():
        digest = hashlib.sha256(b"".join(p.read_bytes() for p in rules)).hexdigest()
        require(digest == expected.read_text().strip(), "default Falco rules changed")
    else:
        failures.append("default Falco rules or baseline checksum missing")
    local = path(root, "/etc/falco/falco_rules.local.yaml")
    require(local.is_file() and local.read_text().strip() == "[]", "Falco local rules not in initial state")
    require(active("falco"), "Falco service inactive")
    for output in ("/root/CKS/invoice-kubesec.json", "/var/lib/kubelet/seccomp/profiles/no-inspect.json"):
        require(not path(root, output).exists(), f"answer already exists: {output}")
    return failures


if __name__ == "__main__":
    problems = check(Path("/"))
    if problems:
        print("\n".join(problems), file=sys.stderr)
        sys.exit(1)
    print("CKS-001B host-file subset ready (not full exam readiness)")
