"""Read-only image/package evidence for CKS-001B Q11."""
import json
from pathlib import Path
import sys

from capture_files import json_file, read_file
from host_fixtures import path
from workload_capture import Kubernetes, deployment_pods


def valid_sbom(doc, expected):
    """Check image digest, package identity and graph relationship, not generator history."""
    if not isinstance(doc, dict) or not isinstance(expected, dict):
        return False
    if (doc.get("spdxVersion") not in ("SPDX-2.2", "SPDX-2.3") or
            doc.get("SPDXID") != "SPDXRef-DOCUMENT" or not doc.get("documentNamespace") or
            not isinstance(doc.get("creationInfo"), dict)):
        return False

    def packages(value):
        value = value.get("packages", [])
        return value if isinstance(value, list) and all(isinstance(p, dict) for p in value) else []

    def image_package(package):
        refs = package.get("externalRefs", [])
        return (package.get("primaryPackagePurpose") == "CONTAINER" or
                isinstance(refs, list) and any(isinstance(ref, dict) and
                    ref.get("referenceType") == "purl" and
                    str(ref.get("referenceLocator", "")).startswith("pkg:oci/") for ref in refs))

    def digests(package):
        checks = package.get("checksums", [])
        if not isinstance(checks, list):
            return set()
        return {c.get("checksumValue") for c in checks if isinstance(c, dict) and
                c.get("algorithm") == "SHA256" and isinstance(c.get("checksumValue"), str)}

    def is_curl(package):
        name = package.get("name")
        return isinstance(name, str) and name.lower() == "curl"

    reference = packages(expected)
    reference_images = set().union(*(digests(p) for p in reference if image_package(p)))
    reference_curl = {p.get("versionInfo") for p in reference if is_curl(p)
                      and isinstance(p.get("versionInfo"), str) and p["versionInfo"]}
    if not reference_images or not reference_curl:
        raise RuntimeError("expected SBOM lacks image identity or curl inventory")
    actual = packages(doc)
    image_ids = {p.get("SPDXID") for p in actual if image_package(p) and digests(p) & reference_images
                 and isinstance(p.get("SPDXID"), str)}
    curl_ids = {p.get("SPDXID") for p in actual if is_curl(p) and
                p.get("versionInfo") in reference_curl and isinstance(p.get("SPDXID"), str)}
    if not image_ids or not curl_ids:
        return False
    relationships = doc.get("relationships", [])
    if not isinstance(relationships, list) or len(relationships) > 20000:
        return False
    graph = {}
    for item in relationships:
        if not isinstance(item, dict) or item.get("relationshipType") not in (
                "CONTAINS", "CONTAINED_BY", "DEPENDS_ON", "DEPENDENCY_OF", "GENERATED_FROM"):
            continue
        source, target = item.get("spdxElementId"), item.get("relatedSpdxElement")
        if isinstance(source, str) and isinstance(target, str):
            graph.setdefault(source, set()).add(target)
            graph.setdefault(target, set()).add(source)
    seen, pending = set(), list(image_ids)
    while pending:
        node = pending.pop()
        if node in curl_ids:
            return True
        if node not in seen:
            seen.add(node)
            pending.extend(graph.get(node, set()) - seen)
    return False


def capture(kube=None, root=Path("/")):
    kube = kube or Kubernetes()
    state = path(root, "/var/lib/painkiller/cks001b")
    try:
        candidate = json.loads(read_file(state / "candidate.json"))
        home = Path(candidate["home"])
        if not home.is_absolute():
            raise ValueError("candidate home must be absolute")
        expected = json_file(state / "expected-curl.spdx.json")
        if not isinstance(expected, dict):
            raise ValueError("missing expected SBOM")
    except (KeyError, ValueError, TypeError):
        raise RuntimeError("supporting SBOM fixture metadata is missing or invalid") from None
    # root is normally /. The alternate root is for isolated local testing.
    directory = path(root, str(home))
    selected = read_file(directory / "selected-produce.txt").strip()
    sbom = json_file(directory / "selected-produce.spdx.json")
    deployment = kube.get("deployment", "orchard", "produce", optional=True)
    pods = deployment_pods(kube, deployment, "produce")
    container = False
    if selected == "plum" and pods:
        containers = deployment.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
        target = next((c for c in containers if c.get("name") == selected), {})
        if target.get("image") == "alpine/curl:8.12.1":
            result = kube.exec("produce", pods[0]["metadata"]["name"], ["apk", "info", "-e", "curl"], selected)
            container = result.returncode == 0
    inventory = valid_sbom(sbom, expected)
    return {"container": container, "sbom": inventory,
            "failures": ([message for ok, message in (
                (container, "Identify the running curl-containing container in selected-produce.txt."),
                (inventory, "Save an SPDX JSON SBOM for the selected image including its curl package.")) if not ok])}


if __name__ == "__main__":
    print(json.dumps(capture()))
