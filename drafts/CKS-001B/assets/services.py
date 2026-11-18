"""CKS-001B service subset: minimal Istio, ingress, TLS and image archives.

Run only in an ownership-checked disposable guest. Network probe endpoints and
fault injection are not yet implemented.
"""
import base64
import json
import os
from pathlib import Path
import subprocess
import tempfile

from host_fixtures import create_once, path
from ingress_fixture import NS as INGRESS_NS, NAME as INGRESS_NAME, canary, objects as ingress_objects
from network_setup import setup as setup_network
from psa_witness import UNIT as PSA_UNIT

IMAGES = ("busybox:1.37.0", "alpine/curl:8.12.1", "alpine:3.21.3")


def archive_name(image):
    return image.replace("/", "_").replace(":", "_") + ".tar"


def command(args, *, data=None, timeout=30):
    return subprocess.run(args, input=data, text=True, capture_output=True, check=True, timeout=timeout)


def k(*args, data=None, timeout=30):
    return command(["kubectl", "--request-timeout=10s", *args], data=data, timeout=timeout)


def setup(root):
    state = path(root, "/var/lib/painkiller/cks001b")
    if not (state / "authorized").is_file() or not (state / "host-files").is_file():
        raise RuntimeError("Refusing service setup on an unauthorized or unprepared guest")
    marker = json.loads(k("get", "namespace", "cks001b-system", "-o", "json").stdout)
    if marker.get("metadata", {}).get("labels", {}).get("painkiller-shell/exam") != "CKS-001B":
        raise RuntimeError("Refusing service setup on an unmarked cluster")
    if (state / "services-complete").exists():
        return

    if not (state / "istio-installed").exists():
        # Minimal has the control plane only, no Istio gateway; Q14 starts with
        # sidecar injection disabled and no PeerAuthentication in its namespace.
        command(["istioctl", "install", "-y", "--set", "profile=minimal"], timeout=600)
        command(["kubectl", "-n", "istio-system", "rollout", "status", "deployment/istiod",
                 "--timeout=240s"], timeout=250)
        create_once(state / "istio-installed", "complete\n", 0o600)

    if not (state / "portal-cert-installed").exists():
        secret = {
            "apiVersion": "v1", "kind": "Secret", "type": "kubernetes.io/tls",
            "metadata": {"name": "portal-cert", "namespace": "secure-portal"},
            "data": {key: base64.b64encode((state / name).read_bytes()).decode()
                     for key, name in (("tls.crt", "portal-tls.crt"), ("tls.key", "portal-tls.key"))},
        }
        k("apply", "-f", "-", data=json.dumps(secret))
        create_once(state / "portal-cert-installed", "complete\n", 0o600)

    if not (state / "ingress-installed").exists():
        k("apply", "-f", "-", data=json.dumps({"apiVersion": "v1", "kind": "List", "items": ingress_objects()}))
        secret = {"apiVersion": "v1", "kind": "Secret", "type": "kubernetes.io/tls",
                  "metadata": {"name": "cks001b-canary-cert", "namespace": "secure-portal"},
                  "data": {key: base64.b64encode((state / name).read_bytes()).decode()
                           for key, name in (("tls.crt", "canary-tls.crt"), ("tls.key", "canary-tls.key"))}}
        k("apply", "-f", "-", data=json.dumps(secret))
        k("apply", "-f", "-", data=json.dumps(canary()))
        command(["kubectl", "-n", INGRESS_NS, "rollout", "status", "deployment/" + INGRESS_NAME,
                 "--timeout=240s"], timeout=250)
        create_once(state / "ingress-installed", "complete\n", 0o600)

    setup_network(root, command)
    if not (state / "psa-witness-installed").exists():
        create_once(path(root, "/etc/systemd/system/cks001b-psa-witness.service"), PSA_UNIT)
        command(["systemctl", "daemon-reload"])
        command(["systemctl", "enable", "--now", "cks001b-psa-witness.service"])
        create_once(state / "psa-witness-installed", "complete\n", 0o600)

    archives = path(root, "/root/ImageTarballs")
    archives.mkdir(parents=True, exist_ok=True)
    for image in IMAGES:
        dest = archives / archive_name(image)
        if dest.exists():
            continue
        # The temporary output is not exposed as a usable archive until copy succeeds.
        with tempfile.TemporaryDirectory(dir=archives) as tmp:
            partial = Path(tmp) / "image.tar"
            command(["skopeo", "copy", "--override-os=linux", "--override-arch=amd64",
                     f"docker://{image}", f"docker-archive:{partial}:{image}"], timeout=300)
            os.link(partial, dest)

    expected = state / "expected-curl.spdx.json"
    if not expected.exists():
        with tempfile.TemporaryDirectory(dir=state) as tmp:
            out = Path(tmp) / "expected.spdx.json"
            command(["bom", "generate", "--image-archive", str(archives / archive_name(IMAGES[1])),
                     "--format", "json", "--output", str(out)], timeout=180)
            doc = json.loads(out.read_text())
            if not doc.get("spdxVersion") or not any(
                    p.get("name", "").lower() == "curl" for p in doc.get("packages", [])):
                raise RuntimeError("Expected SBOM does not contain the curl package")
            os.link(out, expected)
    create_once(state / "services-complete", "complete\n", 0o600)


if __name__ == "__main__":
    setup(Path("/"))
