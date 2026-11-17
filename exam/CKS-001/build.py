#!/usr/bin/env python3
"""Authoring source for deterministic tasks/setup/e2e JSON. No lab access."""
import json
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).parent / "assets"))
from common import BUSY, NGINX, deployment, pod

HERE = Path(__file__).parent
tasks = []
objects = []


def manifest(obj):
    return "kubectl apply -f - <<'YAML'\n" + json.dumps(obj, indent=2) + "\nYAML"


def recreate(obj):
    return (f"kubectl -n {obj['metadata']['namespace']} delete pod {obj['metadata']['name']} "
            "--ignore-not-found --wait=true\n" + manifest(obj) +
            f"\nkubectl -n {obj['metadata']['namespace']} wait pod/{obj['metadata']['name']} "
            "--for=condition=Ready --timeout=90s")


def edit_python(code):
    return "sudo python3 - <<'PY'\nimport yaml\nfrom pathlib import Path\n" + code + "\nPY"


def static_wait(names):
    return edit_python("""import json, subprocess, time
from datetime import datetime
names=%r
expected={name:yaml.safe_load(Path('/etc/kubernetes/manifests/'+name+'.yaml').read_text())['spec']['containers'][0]['command'] for name in names}
written={name:int(Path('/etc/kubernetes/manifests/'+name+'.yaml').stat().st_mtime) for name in names}
deadline=time.monotonic()+100
while time.monotonic()<deadline:
    result=subprocess.run(['kubectl','--kubeconfig=/etc/kubernetes/admin.conf','--request-timeout=2s','-n','kube-system','get','pods','-o','json'],capture_output=True,text=True,timeout=5)
    if result.returncode==0:
        pods=json.loads(result.stdout)['items']
        if all(any(p['metadata']['name'].startswith(name+'-') and
                   p['spec']['containers'][0]['command']==expected[name] and
                   any(c['type']=='Ready' and c['status']=='True' for c in p.get('status',{}).get('conditions',[])) and
                   bool(p.get('status',{}).get('containerStatuses')) and
                   all(datetime.fromisoformat(c.get('state',{}).get('running',{}).get('startedAt','1970-01-01T00:00:00Z')).timestamp()>=written[name]
                       for c in p['status']['containerStatuses'])
                   for p in pods) for name in names):
            break
    time.sleep(1)
else:
    raise RuntimeError('Updated static Pods did not become Ready')
""" % names)


def task(n, ident, title, category, instruction, solution, checks, steps, explanation):
    prefix = ("All commands and files belong on the initial controller. Use the supplied kubeconfig "
              "and `sudo` for root-owned paths. This exam has one node, no workers, and no context switching.\n\n"
              if n == 1 else "")
    tasks.append({"id": ident, "question_id": f"CKS-001-Q{n}", "title": title, "category": category,
                  "points": sum(p for _, p, _ in checks), "instructions": prefix + instruction,
                  "steps": steps, "solution": solution,
                  "resource": f"sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf timeout 25s python3 -B /opt/painkiller/cks001/capture.py q{n}",
                  "checks": [{"description": label, "points": points, "expression": f".{key} == true",
                              "diagnostics": ".failures // []"} for key, points, label in checks],
                  "explanation": explanation})


for ns in ["cks001-system", "helix", "relay", "runtime-watch", "inventory", "batch-ops",
           "locked-down", "catalog", "storage", "billing", "observatory", "launch",
           "team-a", "team-b", "team-c", "harbor", "cks001-canary"]:
    objects.append({"apiVersion": "v1", "kind": "Namespace", "metadata": {
        "name": ns, "labels": {"painkiller-shell/exam": "CKS-001"}}})
next(o for o in objects if o["metadata"]["name"] == "locked-down")["metadata"]["labels"].update({
    "pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "latest"})
for name in ["open-web", "blocked-web"]:
    objects.append(pod(name, "cks001-canary", NGINX))
objects.append({"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
                "metadata": {"name": "canary-deny", "namespace": "cks001-canary"},
                "spec": {"podSelector": {"matchLabels": {"app": "blocked-web"}},
                         "policyTypes": ["Ingress"], "ingress": []}})

# Q1: identities differ by effective RBAC, not their suggestive names.
for name, verbs, resources in [("site-reader", ["get"], ["configmaps"]),
                               ("site-editor", ["*"], ["configmaps"]),
                               ("site-admin", ["*"], ["*"])]:
    objects.extend([
        {"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": name, "namespace": "helix"}},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "metadata": {"name": name, "namespace": "helix"},
         "rules": [{"apiGroups": [""], "resources": resources, "verbs": verbs}]},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
         "metadata": {"name": name, "namespace": "helix"}, "roleRef": {
             "apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": name},
         "subjects": [{"kind": "ServiceAccount", "name": name, "namespace": "helix"}]}])
front = pod("frontend", "helix", NGINX)
front["spec"]["serviceAccountName"] = "site-admin"
front["spec"]["containers"][0]["volumeMounts"] = [{"name": "html", "mountPath": "/usr/share/nginx/html", "readOnly": True}]
front["spec"]["volumes"] = [{"name": "html", "configMap": {"name": "site-content", "items": [
    {"key": "public", "path": "index.html"}, {"key": "private", "path": "staff/index.html"}]}}]
objects += [{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "site-content", "namespace": "helix"},
             "data": {"public": "helix-public\n", "private": "helix-private\n"}}, front]
secured_front = json.loads(json.dumps(front))
secured_front["spec"]["serviceAccountName"] = "site-reader"
secured_front["spec"]["containers"][0]["securityContext"] = {"appArmorProfile": {"type": "Localhost", "localhostProfile": "site-guard"}}
task(1, "apparmor-identity", "Constrain a frontend and its identity", "System Hardening",
     "Pod `frontend` in `helix` exposes `/usr/share/nginx/html/staff/`. Load the supplied AppArmor profile "
     "`/etc/apparmor.d/site-guard` and recreate the Pod using it so the private directory cannot be read. "
     "Keep the public site serving HTTP. Inspect the existing service accounts and select the non-default account "
     "with the least privileges. Delete the other unused non-default service accounts; do not create accounts "
     "or use `default`. Preserve the existing image and content volume. "
     "You can test locally with `kubectl -n helix exec frontend -- wget -qO- http://127.0.0.1/staff/`.",
     "sudo apparmor_parser -r /etc/apparmor.d/site-guard\n" + recreate(secured_front) +
     "\nkubectl -n helix delete serviceaccount site-editor site-admin --ignore-not-found",
     [("identity", 1, "Least-privilege existing account and no unused accounts"),
      ("confinement", 2, "Running frontend enforces AppArmor while public HTTP remains available")],
     ["Compare RoleBindings and Role rules for the existing accounts.", "Load the profile on the only node.",
      "Recreate the Pod with the Localhost profile and selected account; remove unused accounts.",
      "Verify public HTTP succeeds and private-file reads fail."],
     "AppArmor restricts host-kernel file access; Kubernetes RBAC independently limits the workload identity.")

secret_pod = pod("connector", "relay", BUSY, ["sh", "-c", "sleep 2147483647"])
secret_pod["spec"]["containers"][0]["env"] = [{"name": "BROKER_PASSWORD", "valueFrom": {
    "secretKeyRef": {"name": "broker-auth", "key": "BROKER_PASSWORD"}}}]
objects += [{"apiVersion": "v1", "kind": "Secret", "metadata": {"name": "broker-auth", "namespace": "relay"},
             "stringData": {"BROKER_PASSWORD": "relay-test-key-73"}}, secret_pod]
mounted = json.loads(json.dumps(secret_pod))
del mounted["spec"]["containers"][0]["env"]
mounted["spec"]["containers"][0]["volumeMounts"] = [{"name": "auth", "mountPath": "/mnt/broker/password", "readOnly": True}]
mounted["spec"]["volumes"] = [{"name": "auth", "secret": {"secretName": "broker-auth"}}]
task(2, "secret-volume", "Move credentials out of the environment", "Minimize Microservice Vulnerabilities",
     "Pod `connector` in `relay` receives `BROKER_PASSWORD` from Secret `broker-auth`. Write the decoded existing "
     "value, without extra whitespace, to `/root/CKS/secrets/BROKER_PASSWORD`. Recreate the Pod so it no longer "
     "receives any Secret through environment variables or envFrom. Mount that existing Secret as a read-only "
     "volume at directory `/mnt/broker/password` (the key becomes a file in that directory). Keep its image and running command.",
     "sudo mkdir -p /root/CKS/secrets\nkubectl -n relay get secret broker-auth -o jsonpath='{.data.BROKER_PASSWORD}' | "
     "base64 -d | sudo tee /root/CKS/secrets/BROKER_PASSWORD >/dev/null\n" + recreate(mounted),
     [("extracted", 1, "Existing secret decoded to the requested file"),
      ("mounted", 2, "Running Pod uses a read-only Secret volume instead of environment exposure")],
     ["Decode the Secret key into the root-owned answer file.", "Replace the env reference with a Secret volume and readOnly volumeMount.",
      "Recreate the Pod and compare its mounted bytes with the original."],
     "A Secret volume avoids exposing credentials through process environment inspection; it does not replace access control.")

task(3, "credential-review", "Review build and workload credential exposure", "Supply Chain Security",
     "Review the Dockerfiles and workload manifests under `/opt/security-review/`. Identify files exposing credentials "
     "through embedded values, image layers, command arguments, or logs. Write only their basenames, one per line, "
     "to `/opt/security-review/findings.txt`. Running as root is outside this review. Assume referenced files, Secrets "
     "and mounts exist; ignore syntax and application logic errors. Do not modify the supplied review files.",
     "printf '%s\\n' Dockerfile.backup Dockerfile.release job-debug.yaml pod-client.yaml | sudo tee /opt/security-review/findings.txt >/dev/null",
     [("review", 8, "Report identifies exactly the credential-exposing files")],
     ["Inspect how each file obtains credentials and where they end up.", "Distinguish build-secret mounts and mounted runtime Secrets from persistent layers or output.",
      "Write the four affected basenames, with no safe files."],
     "Deleting a copied credential in a later layer does not remove it from earlier image layers; build-secret mounts avoid that persistence.")

sec = pod("guard-nginx", "default", "nginx:alpine")
sec["spec"]["securityContext"] = {"seccompProfile": {"type": "Localhost", "localhostProfile": "profiles/web-guard.json"}}
task(4, "seccomp", "Attach a local syscall profile", "System Hardening",
     "Create Pod `guard-nginx` in `default`, using `nginx:alpine`. Move the supplied `/root/CKS/web-guard.json` "
     "into `profiles/` beneath kubelet's default seccomp directory. Apply it through the Pod security context. "
     "Leave the Pod running with nginx serving HTTP.",
     "sudo mkdir -p /var/lib/kubelet/seccomp/profiles\n"
     "sudo mv /root/CKS/web-guard.json /var/lib/kubelet/seccomp/profiles/web-guard.json\n" + manifest(sec) +
     "\nkubectl wait pod/guard-nginx --for=condition=Ready --timeout=90s",
     [("profile", 2, "Supplied profile moved and referenced in Pod security context"),
      ("running", 2, "Correct nginx image runs with seccomp filtering and serves HTTP")],
     ["Move the JSON file beneath /var/lib/kubelet/seccomp/profiles.", "Set Pod securityContext.seccompProfile to Localhost with the relative path.",
      "Create the Pod and verify nginx responds."],
     "Localhost seccomp paths are relative to kubelet's seccomp directory, not an arbitrary container mount.")

hardening = edit_python("""import os, pwd, grp, subprocess
for p in ['/usr/lib/systemd/system/kubelet.service','/var/lib/kubelet/config.yaml']:
    os.chmod(p, 0o600)
subprocess.run(['groupadd','--system','etcd'], check=False)
subprocess.run(['useradd','--system','--gid','etcd','--no-create-home','etcd'], check=False)
os.chown('/var/lib/etcd', pwd.getpwnam('etcd').pw_uid, grp.getgrnam('etcd').gr_gid)
for name in ['kube-controller-manager','kube-scheduler']:
    p=Path('/etc/kubernetes/manifests')/(name+'.yaml')
    obj=yaml.safe_load(p.read_text())
    cmd=obj['spec']['containers'][0]['command']
    cmd[:]=[a for a in cmd if not a.startswith('--profiling=')]+['--profiling=false']
    tmp=p.with_name('.'+p.name+'.tmp')
    text=yaml.safe_dump(obj)
    if text!=p.read_text():
        tmp.write_text(text); tmp.replace(p)""")
task(5, "cis-hardening", "Repair selected CIS failures", "Cluster Setup",
     "Use installed `kube-bench` with `--config-dir /opt/kube-bench/cfg --benchmark cis-1.10`. Repair only "
     "these seeded FAILs: kubelet service unit permissions (`/usr/lib/systemd/system/kubelet.service`, 4.1.1), "
     "kubelet configuration permissions (`/var/lib/kubelet/config.yaml`, 4.1.9), etcd data-directory ownership "
     "(`/var/lib/etcd`, 1.1.12), and profiling for kube-controller-manager (1.3.2) and kube-scheduler (1.4.1). "
     "Use the benchmark's recommended values; create system user/group `etcd` if needed. Other benchmark findings, "
     "including API-server and policy findings, are out of scope. Keep the API server and control-plane components healthy.",
     hardening + "\n" + static_wait(["kube-controller-manager", "kube-scheduler"]),
     [("unit", 1, "Kubelet service unit permissions are 0600 or stricter"),
      ("config", 1, "Kubelet config permissions are 0600 or stricter"),
      ("etcd", 1, "Etcd data directory belongs to etcd:etcd"),
      ("profiling", 2, "Both running control-plane components disable profiling and API is healthy")],
     ["Run kube-bench with the specified benchmark and inspect only the five requested checks.",
      "Apply restrictive file modes and the etcd ownership.", "Set --profiling=false in the two static Pod manifests.",
      "Wait for the components to restart and rerun the selected checks."],
     "The explicit scope avoids turning unrelated or manually assessed benchmark findings into hidden exam requirements.")

falco_output = "File below a known binary directory opened for writing (user_id=%user.uid file_updated=%fd.name command=%proc.cmdline)"
falco_solution = edit_python("""p=Path('/etc/falco/falco_rules.local.yaml')
p.write_text(yaml.safe_dump([{'rule':'Write below binary dir','priority':'CRITICAL',
    'output':%r, 'override':{'priority':'replace','output':'replace'}}]))
p=Path('/etc/falco/falco.yaml')
obj=yaml.safe_load(p.read_text())
obj['file_output']={'enabled':True,'keep_alive':False,'filename':'/opt/runtime-incidents/events.log'}
p.write_text(yaml.safe_dump(obj))
Path('/opt/runtime-incidents').mkdir(exist_ok=True)""" % falco_output)
task(6, "falco-alerts", "Customize runtime threat alerts", "Monitoring, Logging and Runtime Security",
     "A Pod using `httpd:2.4-alpine` in `runtime-watch` repeatedly writes below a binary directory. "
     "Falco reports alerts beginning `File below a known binary directory opened for writing`. Identify the "
     "responsible rule and override it using `/etc/falco/falco_rules.local.yaml`, not the default rules file. "
     "Set priority `CRITICAL` and output exactly:\n\n`" + falco_output + "`\n\n"
     "Configure Falco to write text alerts to `/opt/runtime-incidents/events.log`, then restart service `falco`. "
     "Keep monitoring and the suspicious workload running. New alerts may take up to 30 seconds.",
     falco_solution + "\nsudo systemctl restart falco\n"
     "for i in $(seq 1 30); do sudo grep -q 'Critical File below a known binary directory' /opt/runtime-incidents/events.log 2>/dev/null && break; sleep 1; done",
     [("override", 6, "Local rule override has exact priority/output and default rules are unchanged"),
      ("logging", 8, "Active Falco configuration writes matching real alerts to the requested text log")],
     ["Search the default rules for the alert message and identify Write below binary dir.",
      "Use the override replace syntax for output and priority in the local rules file.",
      "Enable file output, restart Falco, and inspect newly emitted alerts."],
     "Local overrides preserve packaged rules while changing selected rule fields; actual syscall events must reach the configured output.")

toolkit = deployment("toolkit", "inventory")
toolkit["spec"]["template"]["spec"]["containers"] = [
    {"name": "cedar", "image": BUSY, "command": ["sleep", "2147483647"]},
    {"name": "maple", "image": "alpine/curl:8.12.1", "command": ["sleep", "2147483647"]},
    {"name": "birch", "image": "alpine:3.21.3", "command": ["sleep", "2147483647"]}]
objects.append(toolkit)
task(7, "image-sbom", "Identify a package and inventory its image", "Supply Chain Security",
     "Deployment `toolkit` in `inventory` has containers `cedar`, `maple`, and `birch`. Inspect the running containers "
     "to find the one containing the curl package. Write its container name to `~/selected-container.txt`. "
     "Using installed `bom` and that image's archive under `/root/ImageTarballs/` (filenames replace `/` and `:` "
     "in image names with `_`), generate an SPDX JSON SBOM at `~/selected-image.spdx.json`. "
     "The SBOM must describe the selected image and its curl package, not merely the outer tar file.",
     "kubectl -n inventory exec deployment/toolkit -c maple -- curl --version\n"
     "printf 'maple\\n' > ~/selected-container.txt\n"
     "sudo bom generate --image-archive /root/ImageTarballs/alpine_curl_8.12.1.tar --format json "
     "--output \"$(printf '%s' \"$HOME\")/selected-image.spdx.json\"\nsudo chown \"$(id -u):$(id -g)\" ~/selected-image.spdx.json",
     [("container", 2, "Correct running container identified"),
      ("sbom", 10, "SPDX JSON identifies the selected image, its digest, and the curl package")],
     ["Exec into each named container and inspect installed curl.", "Find the archive matching that container's image.",
      "Use bom generate --image-archive with JSON format and inspect the package inventory."],
     "An image SBOM links an image identity with its package inventory; checksums alone on the archive are not an image inventory.")

janitor = deployment("janitor", "batch-ops", BUSY, ["sleep", "2147483647"])
objects.append(janitor)
sa = {"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": "job-agent", "namespace": "batch-ops"},
      "automountServiceAccountToken": False}
token_patch = {"spec": {"template": {"spec": {"serviceAccountName": "job-agent",
    "automountServiceAccountToken": False, "containers": [{"name": "janitor", "volumeMounts": [
        {"name": "identity", "mountPath": "/var/run/job-identity", "readOnly": True}]}],
    "volumes": [{"name": "identity", "projected": {"sources": [
        {"serviceAccountToken": {"path": "token", "expirationSeconds": 3600}}]}}]}}}}
task(8, "projected-identity", "Opt in to a projected workload token", "Cluster Hardening",
     "Create ServiceAccount `job-agent` in `batch-ops` with automatic token mounting disabled. "
     "Set existing Deployment `janitor` to use it and explicitly mount a projected service-account token "
     "as a read-only file beneath `/var/run/job-identity/`. Disable the Pod's automatic token mount too. "
     "Change only the service-account/token-related fields; preserve all other Deployment fields and leave it healthy.",
     manifest(sa) + "\nkubectl -n batch-ops patch deployment janitor --type=strategic -p '" + json.dumps(token_patch) +
     "'\nkubectl -n batch-ops rollout status deployment/janitor --timeout=90s",
     [("account", 2, "New account disables automount"),
      ("projection", 3, "Healthy deployment uses only the explicit projected token and preserves unrelated fields")],
     ["Create the ServiceAccount with automountServiceAccountToken false.", "Patch the Pod template to select it and disable Pod automount.",
      "Add a serviceAccountToken projected volume and read-only mount; verify its token file."],
     "Disabling automatic mounting does not prevent explicitly projecting a short-lived, rotating token.")

portal = deployment("portal", "locked-down", "nginxinc/nginx-unprivileged:1.28-alpine")
objects.append(portal)
security = {"runAsUser": 1002, "runAsNonRoot": True, "allowPrivilegeEscalation": False,
            "capabilities": {"drop": ["ALL"]}, "seccompProfile": {"type": "RuntimeDefault"}}
portal_patch = {"spec": {"template": {"spec": {"containers": [{"name": "portal", "securityContext": security}]}}}}
task(9, "restricted-repair", "Repair a workload under restricted admission", "Minimize Microservice Vulnerabilities",
     "Deployment `portal` in `locked-down` cannot obtain a running Pod. Diagnose and fix it while preserving "
     "its existing image and every namespace label. The namespace must continue to enforce the restricted Pod "
     "Security Standard. Leave the existing web application serving HTTP on port 8080; do not replace it with a sleep process.",
     "kubectl -n locked-down patch deployment portal --type=strategic -p '" + json.dumps(portal_patch) +
     "'\nkubectl -n locked-down rollout status deployment/portal --timeout=90s",
     [("restricted", 5, "Existing application serves HTTP under unchanged restricted namespace enforcement")],
     ["Read Deployment/ReplicaSet events for admission failures.", "Supply the missing restricted-policy container settings.",
      "Wait for rollout and verify the original nginx application responds."],
     "Pod Security Admission rejects the Pod request; relaxing namespace policy would bypass rather than repair the workload.")

web = deployment("catalog-web", "catalog", NGINX)
objects.append(web)
for ns, name in [("storage", "catalog-db"), ("storage", "other-client"), ("billing", "billing-client"),
                 ("default", "outsider"), ("cks001-system", "observer")]:
    objects.append(pod(name, ns, BUSY, ["sleep", "2147483647"]))
policy = {"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
          "metadata": {"name": "catalog-ingress", "namespace": "catalog"},
          "spec": {"podSelector": {"matchLabels": {"app": "catalog-web"}}, "policyTypes": ["Ingress"],
                   "ingress": [{"from": [{"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "storage"}},
                                         "podSelector": {"matchLabels": {"app": "catalog-db"}}},
                                        {"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "billing"}}}]}]}}
task(10, "network-isolation", "Restrict incoming application traffic", "Cluster Setup",
     "Deployment `catalog-web` runs in `catalog`. Create NetworkPolicy `catalog-ingress`, selecting that workload, "
     "to allow inbound traffic from Pod `catalog-db` in `storage` and from every Pod in `billing`. Use existing labels. "
     "Other sources, including `storage/other-client` and `default/outsider`, must not reach the application. "
     "Do not change workload/namespace labels, delete fixtures, or add unrelated policy exceptions.",
     manifest(policy),
     [("policy", 2, "Named policy selects the existing workload without altering labels"),
      ("allowed", 3, "Database and billing traffic reaches the application"),
      ("denied", 3, "Other storage and default clients cannot reach the healthy application")],
     ["Combine the storage namespaceSelector and database podSelector in a single from entry.",
      "Use a separate namespaceSelector entry for billing.", "Probe both allowed and disallowed sources."],
     "Selectors in one peer are ANDed; separate peer entries are ORed. Namespace-only peers include all Pods in that namespace.")

delta = deployment("delta", "observatory", BUSY, ["sleep", "2147483647"])
delta["spec"]["template"]["spec"]["containers"].append({"name": "helper", "image": BUSY, "command": ["sleep", "2147483647"]})
objects.append(delta)
delta_patch = {"spec": {"template": {"spec": {"containers": [
    {"name": name, "securityContext": {"runAsUser": 1002, "allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True}}
    for name in ["delta", "helper"]]}}}}
task(11, "container-hardening", "Harden both containers", "Minimize Microservice Vulnerabilities",
     "Update both containers of Deployment `delta` in `observatory`: run as UID `1002`, disable privilege escalation, "
     "and make their root filesystems read-only. Keep both existing containers, images, and long-running commands healthy.",
     "kubectl -n observatory patch deployment delta --type=strategic -p '" + json.dumps(delta_patch) +
     "'\nkubectl -n observatory rollout status deployment/delta --timeout=90s",
     [("uid", 1, "Both running containers use UID 1002"),
      ("escalation", 1, "Both containers disallow privilege escalation"),
      ("readonly", 2, "Both container root filesystems are mounted read-only")],
     ["Set container security contexts on both containers.", "Wait for the rollout.",
      "Inspect effective UIDs, NoNewPrivs, and root mount options."],
     "runAsUser controls identity, allowPrivilegeEscalation sets no_new_privs, and readOnlyRootFilesystem changes root mount access.")

objects += [deployment("shuttle", "launch", NGINX),
            {"apiVersion": "v1", "kind": "Service", "metadata": {"name": "shuttle", "namespace": "launch"},
             "spec": {"selector": {"app": "shuttle"}, "ports": [{"port": 80, "targetPort": 80}]}}]
ingress = {"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
           "metadata": {"name": "shuttle-entry", "namespace": "launch"},
           "spec": {"ingressClassName": "nginx", "tls": [{"hosts": ["shuttle.lab"], "secretName": "shuttle-tls"}],
                    "rules": [{"host": "shuttle.lab", "http": {"paths": [{"path": "/", "pathType": "Prefix",
                               "backend": {"service": {"name": "shuttle", "port": {"number": 80}}}}]}}]}}
task(12, "tls-ingress", "Expose a web service over TLS", "Cluster Setup",
     "Deployment `shuttle` is exposed by Service `shuttle` on port 80 in `launch`. Create Ingress `shuttle-entry` "
     "routing host `shuttle.lab`, path `/`, to that Service. Use existing TLS Secret `shuttle-tls` and IngressClass "
     "`nginx` (served by the installed Traefik controller). Add a controller Service ClusterIP mapping for "
     "`shuttle.lab` to the controller's `/etc/hosts`; discover it using `kubectl -n traefik get svc`. "
     "Verify HTTPS using supplied CA `/root/CKS/ingress-ca.crt`, without skipping certificate verification.",
     manifest(ingress) + "\nip=$(kubectl -n traefik get svc traefik -o jsonpath='{.spec.clusterIP}')\n"
     "printf '%s shuttle.lab\\n' \"$ip\" | sudo tee -a /etc/hosts >/dev/null\n"
     "for i in $(seq 1 20); do sudo curl --noproxy '*' --cacert /root/CKS/ingress-ca.crt -fsS https://shuttle.lab/ && break; sleep 1; done",
     [("ingress", 3, "Ingress has the required class, host, TLS Secret, path and backend"),
      ("https", 2, "Hosts mapping and certificate-verified HTTPS reach the application")],
     ["Inspect Service and IngressClass resources.", "Create the TLS Ingress with the existing Secret.",
      "Map the controller ClusterIP in /etc/hosts and curl using the supplied CA."],
     "The ingress class name is an administrator-chosen name, not necessarily the controller's product name.")

for ns in ["team-a", "team-b", "team-c"]:
    objects.extend([
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
         "metadata": {"name": "alex-pods", "namespace": ns},
         "rules": [{"apiGroups": [""], "resources": ["*"], "verbs": ["*"]}]},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
         "metadata": {"name": "alex-access", "namespace": ns},
         "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "alex-pods"},
         "subjects": [{"kind": "User", "name": "alex", "apiGroup": "rbac.authorization.k8s.io"}]}])
rbac_solution = "\n".join(manifest({"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
    "metadata": {"name": "alex-pods", "namespace": ns},
    "rules": [{"apiGroups": [""], "resources": ["pods"], "verbs": ["get", "list"] if ns == "team-c" else ["*"]}]})
    for ns in ["team-a", "team-b", "team-c"])
task(13, "rbac-scope", "Reduce a developer's effective permissions", "Cluster Hardening",
     "User `alex` needs all operations on core `pods` in `team-a` and `team-b`, but only get/list on core `pods` "
     "in `team-c`. Remove the current excess permissions, including access to other resource types. "
     "Pod subresources such as exec/log are not required. Retain the existing Role names `alex-pods` and "
     "RoleBinding names `alex-access` in all three namespaces; you may recreate objects with those same names. "
     "Do not grant new cluster-wide access.",
     rbac_solution,
     [("write", 4, "Full Pod-only access in team-a/team-b with original object names"),
      ("read", 2, "Only get/list Pods in team-c, no cluster-wide access")],
     ["Inspect alex's effective permissions and the existing RoleBindings.",
      "Replace wildcard resources with pods in all three Roles; restrict team-c verbs to get/list.",
      "Use kubectl auth can-i --as=alex to test allowed and denied actions."],
     "Namespaced RoleBindings contain access within namespaces; wildcard resource grants exceed a Pod-only requirement.")

audit_policy = {"apiVersion": "audit.k8s.io/v1", "kind": "Policy", "rules": [
    {"level": "Metadata", "verbs": ["delete", "deletecollection"], "namespaces": ["kube-system"],
     "resources": [{"group": "", "resources": ["secrets"]}]},
    {"level": "Request", "verbs": ["create", "update", "patch", "delete", "deletecollection"],
     "namespaces": ["default"], "resources": [{"group": "apps", "resources": ["deployments"]}]},
    {"level": "Metadata"}]}
audit_solution = edit_python("""policy=%r
Path('/etc/kubernetes/security-policy.yaml').write_text(yaml.safe_dump(policy))
p=Path('/etc/kubernetes/manifests/kube-apiserver.yaml')
obj=yaml.safe_load(p.read_text())
c=obj['spec']['containers'][0]
values={'audit-policy-file':'/etc/kubernetes/security-policy.yaml','audit-log-path':'/var/log/security-audit.log',
        'audit-log-maxage':'7','audit-log-maxsize':'12','audit-log-maxbackup':'4'}
c['command']=[a for a in c['command'] if not any(a.startswith('--'+key+'=') for key in values)]
c['command'] += ['--'+key+'='+value for key,value in values.items()]
for name,host,mount,ro,typ in [('audit-policy','/etc/kubernetes/security-policy.yaml','/etc/kubernetes/security-policy.yaml',True,'File'),
                              ('audit-log','/var/log','/var/log',False,'Directory')]:
    obj['spec']['volumes']=[v for v in obj['spec']['volumes'] if v['name']!=name]
    obj['spec']['volumes'].append({'name':name,'hostPath':{'path':host,'type':typ}})
    c['volumeMounts']=[v for v in c['volumeMounts'] if v['name']!=name]
    c['volumeMounts'].append({'name':name,'mountPath':mount,'readOnly':ro})
tmp=p.with_name('.'+p.name+'.tmp'); text=yaml.safe_dump(obj)
if text!=p.read_text():
    tmp.write_text(text); tmp.replace(p)""" % audit_policy)
api_wait = "\n" + static_wait(["kube-apiserver", "kube-controller-manager", "kube-scheduler"]) + "\nkubectl --request-timeout=5s get --raw=/readyz"
task(14, "audit-policy", "Enable and scope API audit logging", "Monitoring, Logging and Runtime Security",
     "Enable API-server auditing using `/etc/kubernetes/security-policy.yaml`. Write logs to "
     "`/var/log/security-audit.log`, with retention 7 days, maximum size 12 MB, and maximum 4 backup files. "
     "Update the supplied policy to record secret deletions (including collection deletion) in `kube-system` "
     "at Metadata level; Deployment changes (create/update/patch/delete/deletecollection) in `default` at Request "
     "level; and every other request at Metadata level. Ensure the running API server uses the policy and "
     "keep it healthy. A recovery manifest is at `/root/backup/kube-apiserver.yaml`: restore it to "
     "`/etc/kubernetes/manifests/kube-apiserver.yaml` if necessary. Q14 and Q15 both edit this manifest; preserve "
     "each other's settings.",
     audit_solution + api_wait,
     [("settings", 3, "Running API server uses the requested audit paths and rotation settings"),
      ("policy", 4, "Policy applies the three requested levels and scopes"),
      ("events", 3, "Real audit events demonstrate the required rules are active")],
     ["Edit the policy with specific rules before the catch-all.", "Add API-server flags and hostPath mounts for policy and logs.",
      "Wait for API recovery; inspect real events for each scope."],
     "Audit rules use first-match semantics. A policy edit must be loaded by the API server, not merely left on the host.")

admission_solution = edit_python("""folder=Path('/root/CKS/ImageGate')
config={'apiVersion':'apiserver.config.k8s.io/v1','kind':'AdmissionConfiguration','plugins':[
 {'name':'ImagePolicyWebhook','configuration':{'imagePolicy':{
 'kubeConfigFile':'/etc/admission-controllers/admission-kubeconfig.yaml',
 'allowTTL':-1,'denyTTL':-1,'retryBackoff':100,'defaultAllow':False}}}]}
(folder/'admission-configuration.yaml').write_text(yaml.safe_dump(config))
p=Path('/etc/kubernetes/manifests/kube-apiserver.yaml'); obj=yaml.safe_load(p.read_text())
cmd=obj['spec']['containers'][0]['command']
enabled=next((a.split('=',1)[1].split(',') for a in cmd if a.startswith('--enable-admission-plugins=')),[])
if 'ImagePolicyWebhook' not in enabled: enabled.append('ImagePolicyWebhook')
cmd[:]=[a for a in cmd if not a.startswith('--enable-admission-plugins=') and not a.startswith('--admission-control-config-file=')]
cmd += ['--enable-admission-plugins='+','.join(enabled),
        '--admission-control-config-file=/etc/admission-controllers/admission-configuration.yaml']
tmp=p.with_name('.'+p.name+'.tmp'); text=yaml.safe_dump(obj)
if text!=p.read_text():
    tmp.write_text(text); tmp.replace(p)""")
harbor = pod("harbor-web", "harbor", "nginx:latest")
objects.append(harbor)
fixed_harbor = pod("harbor-web", "harbor", "nginx:1.28")
task(15, "image-admission", "Integrate fail-closed image admission", "Supply Chain Security",
     "A TLS ImageReview server is already available through Service `image-gate` in `cks001-system`. "
     "Integrate it using the API server's **ImagePolicyWebhook** admission plugin (not a ValidatingWebhookConfiguration). "
     "Create `/root/CKS/ImageGate/admission-configuration.yaml`. That directory is already mounted in the API "
     "server at `/etc/admission-controllers`; reference the supplied kubeconfig there as "
     "`/etc/admission-controllers/admission-kubeconfig.yaml`. Reject `latest` images from any repository and "
     "fail closed if the webhook is unavailable; do not permit cached approvals to bypass this requirement. "
     "Enable the admission controller, preserving existing admission and audit settings. Delete `harbor/harbor-web` "
     "and recreate it with the same nginx repository and tag `1.28`, leaving it running. "
     "Recovery backup: `/root/backup/kube-apiserver.yaml`; restore it if the API becomes unavailable.",
     admission_solution + api_wait + "\n" + recreate(fixed_harbor),
     [("admission", 4, "Running API server enables ImagePolicyWebhook with fail-closed uncached configuration"),
      ("rejected", 2, "Existing admission evidence shows latest images rejected while tagged images pass"),
      ("pod", 2, "Original violating Pod was replaced by running nginx:1.28")],
     ["Inspect the supplied webhook kubeconfig and directory mount.", "Write AdmissionConfiguration with defaultAllow false and allowTTL -1 (0 selects the default cache TTL).",
      "Enable ImagePolicyWebhook without removing other plugins, and wait for the API.",
      "Try a server-side dry run with latest to observe rejection, then recreate harbor-web with tag 1.28."],
     "ImagePolicyWebhook speaks ImageReview over TLS. Fail-closed behavior and disabled approval caching keep backend failures from silently admitting workloads.")


def outputs():
    return {"tasks.json": json.dumps(tasks, indent=2) + "\n",
            # JSON is valid YAML; avoiding a PyYAML authoring dependency keeps builds reproducible.
            "setup.yaml": json.dumps({"apiVersion": "v1", "kind": "List", "items": objects}, indent=2) + "\n",
            "e2e.json": json.dumps({"baseline": 0, "mutations": [
                {"task": t["id"], "remove": f"sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf python3 -B /opt/painkiller/cks001/mutate.py remove q{i}",
                 "restore": f"sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf python3 -B /opt/painkiller/cks001/mutate.py restore q{i}"}
                for i, t in enumerate(tasks, 1)]}, indent=2) + "\n"}


if __name__ == "__main__":
    for name, content in outputs().items():
        if "--check" in sys.argv:
            assert (HERE / name).read_text() == content, f"Regenerate {name}: python3 exam/CKS-001/build.py"
        else:
            (HERE / name).write_text(content)
