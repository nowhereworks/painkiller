#!/usr/bin/env python3
"""Deterministic exam definitions; no cluster access."""
import json
from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE / "assets"))
from fixture_objects import objects
from solutions import SOLUTIONS

# Each task's source weight is retained. Runtime-dependent checks are gated so
# an untouched starting fixture never earns points simply for being healthy.
CHECKS = [
    [("processes", 2, "Detect execution of all three mining process names"),
     ("priority", 1, "Use CRITICAL priority"), ("output", 1, "Use the requested output and tags"),
     ("active", 1, "Load the local rule in active Falco without changing defaults")],
    [("policy", 1, "Select all namespace pods for egress only"), ("blocked", 2, "Block both dangerous CIDRs"),
     ("allowed", 1, "Retain other egress and leave ingress unaffected"), ("dns", 1, "Retain UDP and TCP DNS")],
    [("escalation", 2, "Disable privilege escalation"), ("nonroot", 1, "Require non-root execution"),
     ("uid", 1, "Run with UID 101"), ("gid", 1, "Run with GID 101"),
     ("image", 1, "Use the specified image"), ("port", 1, "Declare TCP 8080"),
     ("serving", 3, "Leave the Pod healthy and serving HTTP on 8080")],
    [("policy", 1, "Select only the frontend for ingress"), ("scope", 1, "Preserve frontend egress"),
     ("allowed", 3, "Permit the two specified source groups"), ("denied", 2, "Deny other frontend ingress")],
    [("rolebindings", 2, "Deny rolebinding writes everywhere"), ("clusterrolebindings", 2, "Deny clusterrolebinding writes"),
     ("retained", 1, "Retain other current permissions"), ("no_new", 1, "Add no permissions")],
    [("readonly", 3, "Configure read-only root filesystems"), ("mounts", 3, "Enforce read-only root with writable mounts"),
     ("serving", 2, "Keep hardened nginx serving HTTP")],
    [("base", 1, "Use distroless"), ("minimal", 1, "Remove shell and package installation without adding lines"),
     ("binary", 1, "Copy only the application binary"), ("nonroot", 1, "Select nonroot"), ("exec", 1, "Use exec ENTRYPOINT")],
    [("drop", 3, "Drop ALL in the manifest and Pod"), ("add", 2, "Add back only NET_BIND_SERVICE"),
     ("serving", 2, "Keep the restricted Pod serving HTTP"), ("report", 2, "Save a passing Kubesec report")],
    [("enforce", 3, "Enforce restricted"), ("version", 1, "Use latest"), ("warning", 2, "Warn at baseline"),
     ("rejected", 2, "Reject privileged admission"), ("compliant", 2, "Keep the compliant application healthy")],
    [("default_allow", 1, "Allow other syscalls by default"), ("debug_blocked", 1, "Deny both debugging syscalls with errno"),
     ("profile", 1, "Apply the Localhost profile"), ("serving", 1, "Keep the protected application serving HTTP")],
    [("container", 2, "Identify the curl-containing running container"), ("sbom", 5, "Save an SPDX image SBOM including curl")],
    [("route", 1, "Route the specified host and path to portal"), ("tls", 1, "Attach the supplied TLS Secret"),
     ("configuration", 1, "Use nginx class and requested annotations"), ("traffic", 1, "Verify TLS and HTTP redirect")],
    [("policy", 1, "Select all pods for egress only"), ("dns", 1, "Allow TCP and UDP DNS"),
     ("https", 1, "Allow HTTPS"), ("api", 2, "Allow only the specified API CIDR on 9443"),
     ("denied", 2, "Deny other egress without changing ingress")],
    [("strict", 2, "Enforce namespace-wide STRICT mTLS"), ("north_proxy", 1, "Inject a Ready north sidecar"),
     ("south_proxy", 1, "Inject a Ready south sidecar"), ("communication", 2, "Allow meshed communication"),
     ("plaintext_blocked", 1, "Reject unmeshed plaintext traffic")],
    [("secret", 1, "Create the correct TLS Secret"), ("mounted", 1, "Mount its contents in the healthy Deployment")],
    [("permissions", 1, "Restrict directory and file permissions recursively"), ("ownership", 1, "Own the tree as etcd:etcd"),
     ("benchmark", 1, "Pass CIS checks 1.1.11 and 1.1.12"), ("healthy", 1, "Keep the API healthy")],
]

TEACHING = [
    (["Inspect the local rule-loading configuration.", "Match process execution and all three process names.",
      "Set priority, output and tags, validate the rules, then restart Falco."],
     "Local rules survive distribution-rule updates. An exec event is different from a process merely existing."),
    (["Select every Pod for egress isolation.", "Allow IPv4 destinations with the two CIDR exceptions.",
      "Verify both DNS protocols, excluded destinations and an allowed control."],
     "NetworkPolicies add allowed traffic; a second broad allow can defeat an exclusion."),
    (["Inspect the existing Pod and unprivileged nginx image.", "Set non-root UID/GID and disallow escalation.",
      "Recreate the Pod and verify HTTP on 8080."],
     "runAsNonRoot checks identity, while allowPrivilegeEscalation controls no_new_privs independently."),
    (["Select frontend Pods only.", "Use separate peers for same-namespace backend Pods and the telemetry namespace.",
      "Limit the rule to TCP 80 and test permitted and denied clients."],
     "Separate peer entries are ORed; namespaceSelector and podSelector in one peer are ANDed."),
    (["Inspect the service account's effective role bindings.", "Remove only the dangerous binding-write verbs.",
      "Verify denied binding writes and unchanged ordinary resource access."],
     "Preserving permissions is as important as removing escalation. Removing the entire binding is not sufficient."),
    (["Inspect the writable storage mounts.", "Set readOnlyRootFilesystem on each container.",
      "Wait for the rollout and verify root mount flags, writable volumes and HTTP."],
     "Read-only root filesystems prevent image-layer writes without making separately mounted scratch volumes read-only."),
    (["Inspect the supplied binary and build file.", "Remove package installation and shell execution.",
      "Choose distroless/nonroot and an exec-form entrypoint without increasing line count."],
     "Distroless has no shell or package manager. The final file proves its structure, not the history of edits."),
    (["Update the supplied manifest's capabilities.", "Recreate the Pod from that manifest and test nginx.",
      "Scan the corrected manifest with Kubesec and retain its JSON report."],
     "Dropping ALL removes the runtime's default capability set. NET_BIND_SERVICE is not needed for the nonprivileged 8080 port."),
    (["Set the namespace's enforcement and warning labels.", "Keep the existing compliant Deployment running.",
      "Use server-side dry-run to observe rejection of a privileged Pod."],
     "Pod Security Admission acts on new admission; applying labels does not retroactively evict existing Pods."),
    (["Create a default-allow seccomp profile denying the two named syscalls.", "Reference it relative to kubelet's seccomp root.",
      "Recreate the Pod and verify the service."],
     "Localhost seccomp profiles are node files loaded when a container starts; editing a file does not update an already-running filter."),
    (["Inspect package inventories in each running container.", "Select the matching local image archive.",
      "Generate SPDX JSON with bom and save both outputs in your home directory."],
     "An image SBOM should identify the image and its packages. A curl binary alone does not establish package-manager inventory."),
    (["Inspect the Service, certificate and controller NodePorts.", "Create the host/path route with TLS termination and annotations.",
      "Test certificate-verified HTTPS and the HTTP redirect separately."],
     "TLS termination uses the ingress Secret. HTTP can remain open solely to redirect clients to HTTPS."),
    (["Select every Pod for egress isolation.", "Allow DNS, HTTPS and a separate CIDR-scoped custom API rule.",
      "Test permitted ports and negative controls."],
     "A CIDR restriction and port must be in the same rule to constrain the custom API exception."),
    (["Enable injection and restart both workloads.", "Create namespace-wide STRICT PeerAuthentication.",
      "Verify Ready proxies, meshed connectivity and rejection of plaintext."],
     "Sidecar injection alone does not require mutual TLS; STRICT peer authentication rejects unmeshed plaintext clients."),
    (["Create a TLS Secret from the supplied matching certificate/key.", "Add its volume and container mount.",
      "Wait for the rollout and verify the mounted bytes."],
     "Secret volumes expose keys as files; a Secret's existence alone does not prove the application can read its mounted contents."),
    (["Run the named CIS checks and inspect the actual etcd data-dir mount.",
      "Set recursive etcd ownership, 0700 directories and 0600 files only in that tree.",
      "Rerun the checks and verify API readiness."],
     "Directory traversal and file read/write permissions differ. This disposable lab's root-run etcd can access the etcd-owned tree."),
]


def generated():
    tasks = json.loads((HERE / "questions.json").read_text())
    for n, task in enumerate(tasks, 1):
        if n == 1:
            task["instructions"] = ("All commands/files belong on the initial controller. "
                "This lab has one schedulable control plane, no workers and no context switching. "
                "Use sudo for root-owned files; ~ means your candidate home. "
                "Questions are independent; preserve other tasks and supporting services.\n\n" + task["instructions"])
        checks = CHECKS[n - 1]
        assert sum(p for _, p, _ in checks) == task["points"]
        task.update(steps=TEACHING[n - 1][0], explanation=TEACHING[n - 1][1], solution=SOLUTIONS[n],
                    resource=f"sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf timeout 27s python3 -B /opt/painkiller/cks001b/capture.py q{n}",
                    checks=[{"description": label, "points": points, "expression": f".{key} == true",
                             "diagnostics": ".failures // []"} for key, points, label in checks])
    assert sum(t["points"] for t in tasks) == 100
    return {
        "tasks.json": tasks,
        "setup.yaml": {"apiVersion": "v1", "kind": "List", "items": objects()},
        "e2e.json": {"baseline": 0, "mutations": [
            {"task": task["id"],
             "remove": f"sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf python3 -B /opt/painkiller/cks001b/mutate.py {n}",
             "restore": f"python3 -B /opt/painkiller/cks001b/answer.py {n}"}
            for n, task in enumerate(tasks, 1)]},
    }


if __name__ == "__main__":
    for name, obj in generated().items():
        text = json.dumps(obj, indent=2) + "\n"
        target = HERE / name
        if "--check" in sys.argv:
            if not target.exists() or target.read_text() != text:
                raise SystemExit("Generated file differs: " + str(target))
        else:
            target.write_text(text)
