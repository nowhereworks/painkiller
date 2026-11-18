"""Read-only checks of installed Istio, supplied Secret and SBOM prerequisites."""
import base64
import json
from pathlib import Path
import tarfile

from host_fixtures import path
from services import IMAGES, archive_name, k


def normalized(image):
    if "/" not in image:
        return "docker.io/library/" + image
    if "." not in image.split("/", 1)[0] and ":" not in image.split("/", 1)[0]:
        return "docker.io/" + image
    return image


def archive_matches(archive, image):
    """Check bounded Docker archive metadata; no extraction or filesystem writes."""
    try:
        with tarfile.open(archive, "r:") as stream:
            manifest = stream.getmember("manifest.json")
            if not manifest.isfile() or manifest.size > 1024 * 1024:
                return False
            source = stream.extractfile(manifest)
            if source is None:
                return False
            entries = json.loads(source.read())
            return any(normalized(tag) == normalized(image) for entry in entries
                       for tag in (entry.get("RepoTags") or []))
    except (OSError, tarfile.TarError, KeyError, ValueError, TypeError, AttributeError):
        return False


def ready_deployment(obj):
    spec, status = obj.get("spec", {}), obj.get("status", {})
    count = spec.get("replicas", 1)
    return (count > 0 and status.get("observedGeneration", 0) >= obj.get("metadata", {}).get("generation", 0)
            and status.get("updatedReplicas", 0) >= count and status.get("readyReplicas", 0) >= count)


def inspect(root):
    """Return failures or raise when the API is unavailable (not an answer error)."""
    state = path(root, "/var/lib/painkiller/cks001b")
    failures = []

    def require(condition, message):
        if not condition:
            failures.append(message)

    for marker in ("istio-installed", "portal-cert-installed", "services-complete"):
        require((state / marker).is_file(), f"service setup marker missing: {marker}")
    istiod = json.loads(k("-n", "istio-system", "get", "deployment", "istiod", "-o", "json").stdout)
    require(ready_deployment(istiod), "Istio control plane not ready")
    webhook = json.loads(k("get", "mutatingwebhookconfiguration", "istio-sidecar-injector", "-o", "json").stdout)
    require(bool(webhook.get("webhooks")), "Istio sidecar injection webhook is unavailable")
    endpoints = json.loads(k("-n", "istio-system", "get", "endpointslices",
                             "-l", "kubernetes.io/service-name=istiod", "-o", "json").stdout)
    require(any(endpoint.get("conditions", {}).get("ready") is True
                for item in endpoints.get("items", []) for endpoint in item.get("endpoints", [])),
            "Istio control-plane service has no ready endpoints")
    namespace = json.loads(k("get", "namespace", "private-mesh", "-o", "json").stdout)
    labels = namespace.get("metadata", {}).get("labels", {})
    require(labels.get("istio-injection") != "enabled" and "istio.io/rev" not in labels,
            "private-mesh injection already enabled")
    deployments = json.loads(k("-n", "private-mesh", "get", "deployments", "-o", "json").stdout)
    for name in ("north", "south"):
        deployment = next((d for d in deployments.get("items", []) if d["metadata"]["name"] == name), {})
        template = deployment.get("spec", {}).get("template", {})
        require(bool(deployment) and ready_deployment(deployment) and
                template.get("metadata", {}).get("annotations", {}).get("sidecar.istio.io/inject") != "true" and
                not any(c.get("name") == "istio-proxy" for c in template.get("spec", {}).get("containers", [])),
                f"{name} workload missing, unready or already configured for sidecar injection")
    pods = json.loads(k("-n", "private-mesh", "get", "pods", "-o", "json").stdout)
    for name in ("north", "south"):
        current = [pod for pod in pods.get("items", []) if
                   pod.get("metadata", {}).get("labels", {}).get("app") == name and
                   not pod.get("metadata", {}).get("deletionTimestamp")]
        require(bool(current) and all(not any(c.get("name") == "istio-proxy"
                    for c in pod.get("spec", {}).get("containers", [])) for pod in current),
                f"{name} initial Pod missing or already injected")
    peers = json.loads(k("-n", "private-mesh", "get", "peerauthentications", "-o", "json").stdout)
    require(not any(p["metadata"]["name"] == "default" for p in peers["items"]),
            "default PeerAuthentication already exists")
    secret = json.loads(k("-n", "secure-portal", "get", "secret", "portal-cert", "-o", "json").stdout)
    cert_matches = secret.get("type") == "kubernetes.io/tls"
    for key, filename in (("tls.crt", "portal-tls.crt"), ("tls.key", "portal-tls.key")):
        try:
            cert_matches = cert_matches and base64.b64decode(secret.get("data", {}).get(key, ""), validate=True) == (
                state / filename).read_bytes()
        except (OSError, ValueError):
            cert_matches = False
    require(cert_matches, "portal TLS Secret does not match supplied files")
    for image in IMAGES:
        require(archive_matches(path(root, "/root/ImageTarballs") / archive_name(image), image),
                f"archive missing, corrupt or wrong image: {image}")
    try:
        sbom = json.loads((state / "expected-curl.spdx.json").read_text())
        require(bool(sbom.get("spdxVersion")) and any(
            package.get("name", "").lower() == "curl" for package in sbom.get("packages", [])),
            "expected curl SBOM invalid")
    except (OSError, ValueError, TypeError):
        failures.append("expected curl SBOM missing or invalid")
    return failures


if __name__ == "__main__":
    import sys
    problems = inspect(Path("/"))
    if problems:
        print("\n".join(problems), file=sys.stderr)
        sys.exit(1)
    print("CKS-001B service subset ready (not full exam readiness)")
