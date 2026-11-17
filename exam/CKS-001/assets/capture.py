"""Read-only bounded captures; missing answers are false, supporting failures are errors."""
import base64
import copy
import hashlib
import json
import os
from pathlib import Path
import pwd
import re
import stat
import sys
import time
import yaml
from functools import lru_cache

from common import (STATE, candidate_home, containers, default_rules_digest, exec_probe, flagmap, flags,
                    get, k, ready, run, yaml_file)


def file_text(path):
    try:
        return Path(path).read_text()
    except FileNotFoundError:
        return ""


def tail(path, size=524288):
    try:
        with open(path, "rb") as stream:
            stream.seek(max(0, os.fstat(stream.fileno()).st_size - size))
            data = stream.read()
        return data.decode(errors="replace")
    except FileNotFoundError:
        return ""


def audit_events(path="/var/log/security-audit.log"):
    current = Path(path)
    snapshots = [tail(current)]
    rotated = []
    for candidate in current.parent.glob(current.stem + "-*" + current.suffix):
        try:
            rotated.append((candidate.stat().st_mtime, candidate))
        except FileNotFoundError:
            continue
    if rotated:
        snapshots.append(tail(max(rotated)[1]))
    events = []
    for snapshot in snapshots:
        for line in snapshot.splitlines():
            try:
                events.append(json.loads(line))
            except ValueError:
                continue
    return events


def pod_for(name, ns):
    items = get("pods", ns=ns)["items"]
    return next((p for p in items if p["metadata"].get("labels", {}).get("app") == name and ready(p)), {})


def http(ns, target, port=80):
    return exec_probe(ns, target, ["wget", "-T", "2", "-qO-", f"http://127.0.0.1:{port}/"])


@lru_cache(maxsize=3)
def live_flags(name):
    """Inspect the actual running process, not potentially stale mirror-Pod readiness."""
    objects = json.loads(run(["crictl", "ps", "--name", name, "-o", "json"]).stdout)
    matches = [c for c in objects.get("containers", []) if c.get("metadata", {}).get("name") == name]
    if not matches:
        return {}
    actual = json.loads(run(["crictl", "inspect", matches[0]["id"]]).stdout)
    return flagmap(actual["info"]["runtimeSpec"]["process"]["args"])


def running_manifest(name):
    return bool(live_flags(name)) and live_flags(name) == flagmap(flags(name))


def selector_matches(selector, labels):
    if any(labels.get(key) != value for key, value in selector.get("matchLabels", {}).items()):
        return False
    for rule in selector.get("matchExpressions", []):
        key, op, values = rule["key"], rule["operator"], rule.get("values", [])
        if op == "In" and labels.get(key) not in values:
            return False
        if op == "NotIn" and key in labels and labels[key] in values:
            return False
        if op == "Exists" and key not in labels:
            return False
        if op == "DoesNotExist" and key in labels:
            return False
    return True


def normalized_janitor(spec):
    obj = copy.deepcopy(spec)
    template = obj["template"]
    template["metadata"].pop("annotations", None)  # kubectl rollout restart is immaterial.
    p = template["spec"]
    p.pop("serviceAccount", None)
    p.pop("serviceAccountName", None)
    p.pop("automountServiceAccountToken", None)
    p["volumes"] = [v for v in p.get("volumes", []) if "projected" not in v]
    if not p["volumes"]:
        p.pop("volumes")
    for c in p["containers"]:
        c["volumeMounts"] = [m for m in c.get("volumeMounts", []) if not m["mountPath"].startswith("/var/run/job-identity")]
        if not c["volumeMounts"]:
            c.pop("volumeMounts")
    return obj


def audit_level(policy, namespace, resource, verb, group=""):
    for rule in policy.get("rules", []):
        if rule.get("users") or rule.get("userGroups") or rule.get("nonResourceURLs"):
            continue
        if rule.get("namespaces") and namespace not in rule["namespaces"]:
            continue
        if rule.get("verbs") and verb not in rule["verbs"]:
            continue
        if rule.get("resources") and not any(r.get("group", "") in [group, "*"] and
                any(x in ["*", resource] for x in r.get("resources", ["*"])) for r in rule["resources"]):
            continue
        return rule.get("level")
    return "None"


def policy_correct(policy):
    if not isinstance(policy, dict):
        return False
    for namespace in ["kube-system", "default", "other"]:
        for resource, group in [("secrets", ""), ("deployments", "apps"), ("pods", ""), ("services", "")]:
            for verb in ["get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"]:
                want = "Request" if namespace == "default" and resource == "deployments" and verb in [
                    "create", "update", "patch", "delete", "deletecollection"] else "Metadata"
                if audit_level(policy, namespace, resource, verb, group) != want:
                    return False
    return True


def sbom_correct(doc, expected):
    """Compare semantic identity, not generated UUIDs, timestamps or package ordering."""
    if doc.get("spdxVersion") not in ["SPDX-2.2", "SPDX-2.3"] or not doc.get("documentNamespace"):
        return False
    if doc.get("SPDXID") != "SPDXRef-DOCUMENT" or not doc.get("creationInfo"):
        return False
    packages = doc.get("packages", [])
    if not any(p.get("name", "").lower() == "curl" and p.get("versionInfo") for p in packages):
        return False
    expected_ids = {ref.get("referenceLocator") for p in expected.get("packages", [])
                    for ref in p.get("externalRefs", []) if ref.get("referenceType") == "purl"
                    and ref.get("referenceLocator", "").startswith("pkg:oci/")}
    got_ids = {ref.get("referenceLocator") for p in packages for ref in p.get("externalRefs", [])
               if ref.get("referenceType") == "purl"}
    expected_hashes = {c["checksumValue"] for p in expected.get("packages", []) for c in p.get("checksums", [])
                       if c.get("algorithm") == "SHA256" and "curl" in p.get("name", "").lower()}
    got_hashes = {c["checksumValue"] for p in packages for c in p.get("checksums", []) if c.get("algorithm") == "SHA256"}
    # A real archive-generated image document contains its image digest/purl and package relationships.
    identity = bool(expected_ids & got_ids) or bool(expected_hashes & got_hashes)
    return identity and bool(doc.get("relationships"))


def capture(question):
    result = {"failures": []}

    def check(key, condition, message):
        result[key] = bool(condition)
        if not condition:
            result["failures"].append(message)

    if question == "q1":
        p = get("pod", "frontend", "helix", optional=True)
        accounts = {a["metadata"]["name"] for a in get("serviceaccounts", ns="helix")["items"]}
        check("identity", p.get("spec", {}).get("serviceAccountName") == "site-reader" and
              accounts == {"default", "site-reader"}, "Select site-reader and remove only unused non-default accounts.")
        c = containers(p)
        profile = c and c[0].get("securityContext", {}).get("appArmorProfile", p.get("spec", {}).get("securityContext", {}).get("appArmorProfile", {}))
        attached = profile == {"type": "Localhost", "localhostProfile": "site-guard"}
        confined = False
        if attached and ready(p):
            public = http("helix", "frontend")
            private = exec_probe("helix", "frontend", ["cat", "/usr/share/nginx/html/staff/index.html"])
            effective = exec_probe("helix", "frontend", ["cat", "/proc/1/attr/current"])
            confined = (public.returncode == 0 and "helix-public" in public.stdout and private.returncode != 0
                        and "site-guard (enforce)" in effective.stdout)
        check("confinement", confined, "Enforce site-guard on a healthy frontend; public HTTP must work and private reads must fail.")
    elif question == "q2":
        p = get("pod", "connector", "relay", optional=True)
        secret = base64.b64decode(get("secret", "broker-auth", "relay")["data"]["BROKER_PASSWORD"])
        path = Path("/root/CKS/secrets/BROKER_PASSWORD")
        check("extracted", path.is_file() and path.read_bytes() == secret, "Decoded secret file is absent or differs from the existing Secret.")
        spec = p.get("spec", {})
        vols = {v["name"] for v in spec.get("volumes", []) if v.get("secret", {}).get("secretName") == "broker-auth"
                or any(s.get("secret", {}).get("name") == "broker-auth" for s in v.get("projected", {}).get("sources", []))}
        cs = containers(p)
        secure = bool(cs) and all(not any("secretRef" in e for e in c.get("envFrom", [])) and not any("secretKeyRef" in e.get("valueFrom", {})
                       or e.get("name") == "BROKER_PASSWORD" for e in c.get("env", [])) for c in cs)
        mount = any(m.get("name") in vols and m.get("mountPath") == "/mnt/broker/password" and m.get("readOnly") is True
                    for c in cs for m in c.get("volumeMounts", []))
        good = secure and mount and ready(p)
        if good:
            got = exec_probe("relay", "connector", ["cat", "/mnt/broker/password/BROKER_PASSWORD"])
            good = got.returncode == 0 and got.stdout.encode() == secret
        check("mounted", good, "Keep connector running with the existing Secret mounted read-only and no environment exposure.")
    elif question == "q3":
        lines = file_text("/opt/security-review/findings.txt").splitlines()
        check("review", len(lines) == 4 and set(lines) == {
            "Dockerfile.release", "Dockerfile.backup", "job-debug.yaml", "pod-client.yaml"},
            "Report the basenames of all and only the credential-exposing review fixtures.")
    elif question == "q4":
        p = get("pod", "guard-nginx", "default", optional=True)
        dest = Path("/var/lib/kubelet/seccomp/profiles/web-guard.json")
        expected = {"type": "Localhost", "localhostProfile": "profiles/web-guard.json"}
        profile = p.get("spec", {}).get("securityContext", {}).get("seccompProfile") == expected
        check("profile", profile and dest.is_file() and hashlib.sha256(dest.read_bytes()).hexdigest() ==
              (STATE / "seccomp.sha256").read_text() and not Path("/root/CKS/web-guard.json").exists(),
              "Move the supplied unchanged profile to kubelet's profiles directory and use it at Pod scope.")
        good = profile and ready(p) and containers(p)[0]["image"] == "nginx:alpine"
        if good:
            proc = exec_probe("default", "guard-nginx", ["cat", "/proc/1/status"])
            good = bool(re.search(r"Seccomp:\s+2", proc.stdout)) and http("default", "guard-nginx").returncode == 0
        check("running", good, "Use nginx:alpine and leave nginx serving HTTP under the profile.")
    elif question == "q5":
        for key, path in [("unit", "/usr/lib/systemd/system/kubelet.service"), ("config", "/var/lib/kubelet/config.yaml")]:
            mode = stat.S_IMODE(Path(path).stat().st_mode)
            check(key, mode & 0o177 == 0, f"{path} permits access beyond 0600.")
        owner = Path("/var/lib/etcd").stat()
        try:
            import grp
            correct = owner.st_uid == pwd.getpwnam("etcd").pw_uid and owner.st_gid == grp.getgrnam("etcd").gr_gid
        except KeyError:
            correct = False
        check("etcd", correct, "Etcd data directory must belong to etcd:etcd.")
        check("profiling", all(live_flags(n).get("profiling") == "false" and running_manifest(n)
                               for n in ["kube-controller-manager", "kube-scheduler"]),
              "Both running static Pods must have --profiling=false.")
    elif question == "q6":
        expected = "File below a known binary directory opened for writing (user_id=%user.uid file_updated=%fd.name command=%proc.cmdline)"
        local = yaml_file("/etc/falco/falco_rules.local.yaml") or []
        changed = next((r for r in local if r.get("rule") == "Write below binary dir"), {})
        unchanged = default_rules_digest() == (STATE / "falco-default.sha256").read_text()
        override = changed.get("priority") == "CRITICAL" and changed.get("output") == expected and unchanged
        check("override", override, "Override Write below binary dir locally, preserving the packaged rules.")
        config = yaml_file("/etc/falco/falco.yaml")
        output = config.get("file_output", {})
        path = Path("/opt/runtime-incidents/events.log")
        logs = tail(path)
        check("logging", override and output.get("enabled") is True and output.get("filename") == str(path)
              and not config.get("json_output", False) and path.is_file() and time.time() - path.stat().st_mtime < 40
              and run(["systemctl", "is-active", "--quiet", "falco"], check=False).returncode == 0
              and bool(re.search(r"Critical File below a known binary directory opened for writing \(user_id=\d+ file_updated=/usr/bin/cks001-sleep command=.+\)", logs)),
              "No recent matching real Critical text alert was found in the configured file.")
    elif question == "q7":
        home = candidate_home()
        name = file_text(home / "selected-container.txt").strip()
        check("container", name == "maple" and ready(get("deployment", "toolkit", "inventory")),
              "Identify the running container with curl.")
        path = home / "selected-image.spdx.json"
        try:
            doc = json.loads(path.read_text())
            valid = sbom_correct(doc, json.loads((STATE / "expected.spdx.json").read_text()))
        except (FileNotFoundError, ValueError, TypeError, KeyError):
            valid = False
        check("sbom", valid, "Provide SPDX JSON for the selected image, including its identity, relationships, and curl version.")
    elif question == "q8":
        sa = get("serviceaccount", "job-agent", "batch-ops", optional=True)
        check("account", bool(sa) and sa.get("automountServiceAccountToken") is False, "Create job-agent with automount disabled.")
        d = get("deployment", "janitor", "batch-ops")
        spec = d["spec"]["template"]["spec"]
        volnames = {v["name"] for v in spec.get("volumes", []) if any("serviceAccountToken" in s
                    for s in v.get("projected", {}).get("sources", []))}
        mount = next((m for c in containers(d) for m in c.get("volumeMounts", []) if m["name"] in volnames
                      and (m["mountPath"] == "/var/run/job-identity" or m["mountPath"].startswith("/var/run/job-identity/"))
                      and m.get("readOnly") is True), {})
        unchanged = normalized_janitor(d["spec"]) == normalized_janitor(json.loads((STATE / "janitor.json").read_text()))
        good = (bool(sa) and sa.get("automountServiceAccountToken") is False and ready(d) and
                spec.get("serviceAccountName") == "job-agent" and spec.get("automountServiceAccountToken") is False and
                bool(mount) and unchanged)
        if good:
            p = pod_for("janitor", "batch-ops")
            volume = next(v for v in spec["volumes"] if v["name"] == mount["name"])
            tokenpath = next(s["serviceAccountToken"]["path"] for s in volume["projected"]["sources"] if "serviceAccountToken" in s)
            token = exec_probe("batch-ops", p["metadata"]["name"], ["cat", mount["mountPath"].rstrip("/") + "/" + tokenpath])
            try:
                payload = json.loads(base64.urlsafe_b64decode(token.stdout.split(".")[1] + "==="))
                good = payload["sub"] == "system:serviceaccount:batch-ops:job-agent" and payload["exp"] > time.time()
            except (ValueError, KeyError, IndexError):
                good = False
        check("projection", good, "Use only the explicit projected job-agent token, keep unrelated fields, and leave the rollout healthy.")
    elif question == "q9":
        d = get("deployment", "portal", "locked-down")
        labels = get("namespace", "locked-down")["metadata"]["labels"]
        unchanged = labels == json.loads((STATE / "labels.json").read_text())["locked-down"]
        p = pod_for("portal", "locked-down")
        good = unchanged and ready(d) and containers(d)[0]["image"] == "nginxinc/nginx-unprivileged:1.28-alpine"
        if good:
            good = http("locked-down", p["metadata"]["name"], 8080).returncode == 0
        check("restricted", good, "Repair the existing nginx application without changing namespace labels or its image.")
    elif question == "q10":
        policy = get("networkpolicy", "catalog-ingress", "catalog", optional=True)
        labels = json.loads((STATE / "labels.json").read_text())
        intact = all(get("namespace", ns)["metadata"]["labels"] == labels[ns] for ns in ["catalog", "storage", "billing"])
        intact = intact and all(get("pod", name, ns)["metadata"]["labels"].get("app") == name
                               for ns, name in [("storage", "catalog-db"), ("storage", "other-client"),
                                                ("billing", "billing-client"), ("default", "outsider")])
        target_labels = get("deployment", "catalog-web", "catalog")["spec"]["template"]["metadata"]["labels"]
        intact = intact and target_labels == {"app": "catalog-web"}
        selected = selector_matches(policy.get("spec", {}).get("podSelector", {}), target_labels)
        exists = bool(policy) and selected and intact
        check("policy", exists, "Create catalog-ingress selecting catalog-web and preserve existing labels.")
        allowed = denied = False
        if exists:
            p = pod_for("catalog-web", "catalog")
            if not p:
                raise RuntimeError("supporting catalog-web application is unhealthy")
            ip = p["status"]["podIP"]
            if http("catalog", p["metadata"]["name"]).returncode:
                raise RuntimeError("supporting catalog-web HTTP server is unhealthy")
            probes = {}
            for ns, name in [("storage", "catalog-db"), ("billing", "billing-client"),
                             ("storage", "other-client"), ("default", "outsider")]:
                probes[name] = exec_probe(ns, name, ["wget", "-T", "2", "-qO-", f"http://{ip}/"]).returncode == 0
            allowed = probes["catalog-db"] and probes["billing-client"]
            denied = not probes["other-client"] and not probes["outsider"]
        check("allowed", allowed, "Both allowed clients must reach catalog-web.")
        check("denied", denied, "Other storage and default clients must be blocked from the healthy server.")
    elif question == "q11":
        d = get("deployment", "delta", "observatory")
        cs = containers(d)
        healthy = (ready(d) and len(cs) == 2 and {c["name"] for c in cs} == {"delta", "helper"} and
                   all(c["image"] == "busybox:1.37.0" and c.get("command") == ["sleep", "2147483647"] for c in cs))
        uid = healthy and all(c.get("securityContext", {}).get("runAsUser",
                  d["spec"]["template"]["spec"].get("securityContext", {}).get("runAsUser")) == 1002 for c in cs)
        escalation = healthy and all(c.get("securityContext", {}).get("allowPrivilegeEscalation") is False for c in cs)
        ro = healthy and all(c.get("securityContext", {}).get("readOnlyRootFilesystem") is True for c in cs)
        if healthy:
            p = pod_for("delta", "observatory")
            for c in cs:
                proc = exec_probe("observatory", p["metadata"]["name"], ["cat", "/proc/1/status"], container=c["name"]).stdout
                mounts = exec_probe("observatory", p["metadata"]["name"], ["cat", "/proc/1/mounts"], container=c["name"]).stdout
                uid = uid and bool(re.search(r"Uid:\s+1002\s+1002\s+1002\s+1002", proc))
                escalation = escalation and bool(re.search(r"NoNewPrivs:\s+1", proc))
                ro = ro and any(line.split()[1] == "/" and "ro" in line.split()[3].split(",") for line in mounts.splitlines())
        check("uid", uid, "Both running containers must use UID 1002.")
        check("escalation", escalation, "Both containers must set no_new_privs.")
        check("readonly", ro, "Both container root mounts must be read-only.")
    elif question == "q12":
        ingress = get("ingress", "shuttle-entry", "launch", optional=True)
        spec = ingress.get("spec", {})
        rule = next((r for r in spec.get("rules", []) if r.get("host") == "shuttle.lab"), {})
        paths = rule.get("http", {}).get("paths", [])
        ingress_class = spec.get("ingressClassName", ingress.get("metadata", {}).get("annotations", {}).get("kubernetes.io/ingress.class"))
        good = ingress_class == "nginx" and any(t.get("secretName") == "shuttle-tls" and
                   "shuttle.lab" in t.get("hosts", []) for t in spec.get("tls", [])) and any(
                   p.get("path") == "/" and p.get("backend", {}).get("service") ==
                   {"name": "shuttle", "port": {"number": 80}} for p in paths)
        check("ingress", good, "Ingress must use nginx class, shuttle.lab, shuttle-tls, and shuttle:80 at /.")
        ip = get("service", "traefik", "traefik")["spec"]["clusterIP"]
        host = any(line.split("#")[0].split()[:1] == [ip] and "shuttle.lab" in line.split("#")[0].split()[1:]
                   for line in Path("/etc/hosts").read_text().splitlines())
        probe = run(["curl", "--noproxy", "*", "--connect-timeout", "2", "--max-time", "3",
                     "--cacert", "/root/CKS/ingress-ca.crt", "-fsS", "https://shuttle.lab/"], check=False) if good and host else None
        check("https", probe is not None and probe.returncode == 0 and "Welcome to nginx" in probe.stdout,
              "Map shuttle.lab to Traefik and serve certificate-verified HTTPS.")
    elif question == "q13":
        def permissions(ns):
            # The authorization rules API exposes effective rules, not just the expected Role.
            review = {"apiVersion": "authorization.k8s.io/v1", "kind": "SelfSubjectRulesReview",
                      "spec": {"namespace": ns}}
            response = k("--as=alex", "create", "--raw=/apis/authorization.k8s.io/v1/selfsubjectrulesreviews",
                         "-f", "-", data=json.dumps(review))
            status = json.loads(response.stdout)["status"]
            if status.get("incomplete"):
                raise RuntimeError("effective RBAC rule evaluation incomplete")
            return status["resourceRules"]

        ok = {}
        for ns in ["team-a", "team-b", "team-c"]:
            role = get("role", "alex-pods", ns, optional=True)
            binding = get("rolebinding", "alex-access", ns, optional=True)
            rules = permissions(ns)
            # Ignore Kubernetes' authenticated-user self-review permissions (not namespaced workload access).
            def self_review(rule):
                groups = rule.get("apiGroups", [])
                resources = set(rule.get("resources", []))
                return (groups == ["authorization.k8s.io"] and resources <= {
                    "selfsubjectaccessreviews", "selfsubjectrulesreviews"} or
                    groups == ["authentication.k8s.io"] and resources <= {"selfsubjectreviews"})
            workload = [r for r in rules if not self_review(r)]
            names = bool(role and binding) and binding["roleRef"]["name"] == "alex-pods"
            scoped = all(r.get("apiGroups") == [""] and r.get("resources") == ["pods"] and not r.get("resourceNames") for r in workload)
            verbs = {v for r in workload for v in r["verbs"]}
            want = verbs == {"get", "list"} if ns == "team-c" else "*" in verbs or {
                "get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"} <= verbs
            ok[ns] = names and scoped and want
        check("write", ok["team-a"] and ok["team-b"], "Retain named RBAC objects and grant full Pod-only access in team-a/team-b.")
        outside = k("auth", "can-i", "get", "pods", "-n", "cks001-system", "--as=alex", check=False)
        if outside.returncode not in [0, 1] or outside.stdout.strip() not in ["yes", "no"]:
            raise RuntimeError("supporting RBAC negative-control request failed")
        check("read", ok["team-c"] and outside.stdout.strip() == "no",
              "Grant only get/list Pods in team-c, with no inherited workload access.")
    elif question == "q14":
        current = flagmap(flags("kube-apiserver"))
        expected = {"audit-policy-file": "/etc/kubernetes/security-policy.yaml", "audit-log-path": "/var/log/security-audit.log",
                    "audit-log-maxage": "7", "audit-log-maxsize": "12", "audit-log-maxbackup": "4"}
        settings = all(current.get(key) == value for key, value in expected.items()) and running_manifest("kube-apiserver")
        check("settings", settings, "The running API server must use the requested audit paths and 7/12/4 rotation settings.")
        policy = yaml_file("/etc/kubernetes/security-policy.yaml")
        correct = policy_correct(policy)
        check("policy", correct, "Policy first-match behavior does not match the requested scopes and levels.")
        events = audit_events()
        def event(ns, resource, verb, level, body=False):
            for e in reversed(events):
                ref = e.get("objectRef", {})
                if ref.get("namespace") == ns and ref.get("resource") == resource and e.get("verb") == verb and e.get("level") == level:
                    if not body or bool(e.get("requestObject")):
                        return True
            return False
        evidence = event("kube-system", "secrets", "delete", "Metadata") and event(
            "default", "deployments", "patch", "Request", True) and event("cks001-system", "services", "get", "Metadata")
        check("events", settings and correct and evidence, "Real audit events must show Metadata secret deletes, Request Deployment patches, and Metadata other requests.")
    elif question == "q15":
        current = flagmap(flags("kube-apiserver"))
        path = Path("/root/CKS/ImageGate/admission-configuration.yaml")
        config = yaml.safe_load(file_text(path)) or {}
        plugin = next((p for p in config.get("plugins", []) if p.get("name") == "ImagePolicyWebhook"), {})
        embedded = plugin.get("configuration", {})
        if plugin.get("path"):
            pluginpath = plugin["path"].replace("/etc/admission-controllers", "/root/CKS/ImageGate")
            embedded = yaml.safe_load(file_text(pluginpath)) or {}
        settings = embedded.get("imagePolicy", {})
        active = ("ImagePolicyWebhook" in current.get("enable-admission-plugins", "").split(",") and
                  current.get("admission-control-config-file") == "/etc/admission-controllers/admission-configuration.yaml" and
                  settings.get("kubeConfigFile") == "/etc/admission-controllers/admission-kubeconfig.yaml" and
                  settings.get("defaultAllow") is False and settings.get("allowTTL") == -1 and running_manifest("kube-apiserver"))
        check("admission", active, "Enable ImagePolicyWebhook with the supplied kubeconfig, defaultAllow false and allowTTL -1.")
        evidence = json.loads(file_text(STATE / "admission-evidence.json") or "{}")
        tests = evidence.get("tests", {})
        denied = all(tests.get(key, {}).get("code") == 1 and "image-gate: latest" in tests[key].get("error", "")
                     for key in ["latest", "other", "implicit"])
        check("rejected", active and time.time() - evidence.get("time", 0) < 40 and denied and
              tests.get("tagged", {}).get("code") == 0, "Independent recent server-side dry runs must reject latest from multiple repositories and allow tagged images.")
        p = get("pod", "harbor-web", "harbor", optional=True)
        check("pod", ready(p) and containers(p)[0]["image"] == "nginx:1.28" and
              p.get("metadata", {}).get("uid") != file_text(STATE / "harbor.uid"),
              "Delete and recreate harbor-web with nginx:1.28 and leave it Ready.")
    else:
        raise ValueError("Unknown question")
    return result


if __name__ == "__main__":
    try:
        print(json.dumps(capture(sys.argv[1])))
    except Exception as error:
        print("CKS-001 capture error: " + str(error).replace("NotFound", "missing supporting object"), file=sys.stderr)
        sys.exit(2)
