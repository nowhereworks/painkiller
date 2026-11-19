"""Candidate-visible reference solutions; build.py embeds these in tasks.json."""
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent / "assets"))
from fixture_objects import pod


def manifest(obj):
    return "kubectl apply -f - <<'YAML'\n" + json.dumps(obj, indent=2) + "\nYAML"


def replace_pod(obj):
    name, ns = obj["metadata"]["name"], obj["metadata"]["namespace"]
    return (f"kubectl -n {ns} delete pod {name} --ignore-not-found --wait=true\n" +
            manifest(obj) + f"\nkubectl -n {ns} wait pod/{name} --for=condition=Ready --timeout=120s")


def network_policy(name, ns, spec):
    return {"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
            "metadata": {"name": name, "namespace": ns}, "spec": spec}


egress_exclusions = network_policy("exclude-ranges", "outbound-guard", {
    "podSelector": {}, "policyTypes": ["Egress"], "egress": [
        {"to": [{"ipBlock": {"cidr": "0.0.0.0/0",
                             "except": ["198.18.20.0/24", "198.18.30.0/24"]}}]}]})

ingress = network_policy("web-access", "storefront", {
    "podSelector": {"matchLabels": {"app": "shop-web"}}, "policyTypes": ["Ingress"],
    "ingress": [{"from": [
        {"podSelector": {"matchLabels": {"app": "shop-api"}}},
        {"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "telemetry"}}}],
        "ports": [{"protocol": "TCP", "port": 80}]}]})

restricted_egress = network_policy("allow-service-egress", "api-clients", {
    "podSelector": {}, "policyTypes": ["Egress"], "egress": [
        {"ports": [{"protocol": "UDP", "port": 53}, {"protocol": "TCP", "port": 53},
                   {"protocol": "TCP", "port": 443}]},
        {"to": [{"ipBlock": {"cidr": "198.18.40.0/24"}}], "ports": [{"protocol": "TCP", "port": 9443}]}]})

web = pod("web-gate", "workload-identity", "nginxinc/nginx-unprivileged:alpine")
web["spec"]["securityContext"] = {"runAsNonRoot": True, "runAsUser": 101, "runAsGroup": 101}
web["spec"]["containers"][0]["securityContext"] = {"allowPrivilegeEscalation": False}
web["spec"]["containers"][0]["ports"] = [{"containerPort": 8080, "protocol": "TCP"}]

runtime = pod("runtime-web", "runtime-shield", "nginx:1.28-alpine")
runtime["spec"]["securityContext"] = {"seccompProfile": {
    "type": "Localhost", "localhostProfile": "profiles/no-inspect.json"}}

tls_ingress = {
    "apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
    "metadata": {"name": "portal-tls", "namespace": "secure-portal", "annotations": {
        "nginx.ingress.kubernetes.io/ssl-redirect": "true",
        "nginx.ingress.kubernetes.io/ssl-passthrough": "false"}},
    "spec": {"ingressClassName": "nginx",
             "tls": [{"hosts": ["portal.training.example"], "secretName": "portal-cert"}],
             "rules": [{"host": "portal.training.example", "http": {"paths": [{
                 "path": "/", "pathType": "Prefix",
                 "backend": {"service": {"name": "portal", "port": {"number": 80}}}}]}}]},
}

peer_authentication = {"apiVersion": "security.istio.io/v1", "kind": "PeerAuthentication",
                       "metadata": {"name": "default", "namespace": "private-mesh"},
                       "spec": {"mtls": {"mode": "STRICT"}}}

SOLUTIONS = {
    1: """sudo tee /etc/falco/falco_rules.local.yaml >/dev/null <<'YAML'
- rule: Detect mining processes
  desc: Detect execution of known cryptocurrency miners
  condition: spawned_process and proc.name in (xmrig, minerd, cpuminer)
  output: "MINER_NOTICE: %evt.time,%container.name,%proc.name"
  priority: CRITICAL
  tags: [container, crypto_mining, mitre_execution]
YAML
sudo systemctl restart falco
sudo systemctl is-active --quiet falco""",
    2: manifest(egress_exclusions),
    3: replace_pod(web) + "\nkubectl -n workload-identity exec web-gate -- wget -T 3 -qO- http://127.0.0.1:8080/",
    4: manifest(ingress),
    5: """kubectl get clusterrole pipeline-agent-role -o json | python3 -c '
import json, subprocess, sys
role = json.load(sys.stdin)
for rule in role["rules"]:
    if "rbac.authorization.k8s.io" in rule.get("apiGroups", []) and set(rule.get("resources", [])) == {"rolebindings", "clusterrolebindings"}:
        rule["verbs"] = [v for v in rule["verbs"] if v not in {"create", "update", "patch"}]
subprocess.run(["kubectl", "patch", "clusterrole", "pipeline-agent-role", "--type=merge", "-p", json.dumps({"rules": role["rules"]})], check=True)
'
answer=$(kubectl auth can-i --as=system:serviceaccount:delivery:pipeline-agent create clusterrolebindings || true)
test "$answer" = no""",
    6: """kubectl -n immutable-workloads patch deployment report-web --type=strategic -p '{"spec":{"template":{"spec":{"containers":[{"name":"report-web","securityContext":{"readOnlyRootFilesystem":true}}]}}}}'
kubectl -n immutable-workloads rollout status deployment/report-web --timeout=120s
kubectl -n immutable-workloads exec deployment/report-web -- wget -T 3 -qO- http://127.0.0.1/""",
    7: """sudo tee /opt/course/ledger/ledger.Dockerfile >/dev/null <<'DOCKERFILE'
FROM gcr.io/distroless/base
COPY ledger /usr/local/bin/ledger
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ledger"]
DOCKERFILE""",
    8: """sudo python3 - <<'PY'
import json
from pathlib import Path
p = Path('/root/CKS/invoice-web.yaml')
pod = json.loads(p.read_text())
security = pod['spec']['containers'][0]['securityContext']
security['capabilities'] = {'drop': ['ALL'], 'add': ['NET_BIND_SERVICE']}
p.write_text(json.dumps(pod, indent=2) + '\\n')
PY
kubectl -n default delete pod invoice-web --ignore-not-found --wait=true
sudo cat /root/CKS/invoice-web.yaml | kubectl apply -f -
kubectl -n default wait pod/invoice-web --for=condition=Ready --timeout=120s
sudo sh -c 'kubesec scan /root/CKS/invoice-web.yaml > /root/CKS/invoice-kubesec.json'
kubectl -n default exec invoice-web -- wget -T 3 -qO- http://127.0.0.1:8080/""",
    9: """kubectl label namespace finance-ledger pod-security.kubernetes.io/enforce=restricted pod-security.kubernetes.io/enforce-version=latest pod-security.kubernetes.io/warn=baseline --overwrite
kubectl -n finance-ledger rollout status deployment/ledger-web --timeout=120s
if output=$(kubectl -n finance-ledger run psa-denial-probe --image=busybox:1.37.0 --dry-run=server -o yaml --overrides='{"spec":{"containers":[{"name":"psa-denial-probe","image":"busybox:1.37.0","securityContext":{"privileged":true}}]}}' 2>&1); then
  echo 'Privileged Pod was unexpectedly admitted' >&2
  exit 1
fi
printf '%s\\n' "$output" | grep -q 'violates PodSecurity'""",
    10: """sudo mkdir -p /var/lib/kubelet/seccomp/profiles
sudo tee /var/lib/kubelet/seccomp/profiles/no-inspect.json >/dev/null <<'JSON'
{"defaultAction":"SCMP_ACT_ALLOW","syscalls":[{"names":["ptrace","process_vm_readv"],"action":"SCMP_ACT_ERRNO"}]}
JSON
""" + replace_pod(runtime) + "\nkubectl -n runtime-shield exec runtime-web -- wget -T 3 -qO- http://127.0.0.1/",
    11: """kubectl -n produce exec deployment/orchard -c plum -- curl --version
printf '%s\\n' plum > "$HOME/selected-produce.txt"
sudo bom generate --image-archive /root/ImageTarballs/alpine_curl_8.12.1.tar --format json --output "$HOME/selected-produce.spdx.json"
sudo chown "$(id -u):$(id -g)" "$HOME/selected-produce.spdx.json\"""",
    12: manifest(tls_ingress) + """
node=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
http=$(kubectl -n cks001b-ingress get service cks001b-ingress -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}')
https=$(kubectl -n cks001b-ingress get service cks001b-ingress -o jsonpath='{.spec.ports[?(@.name=="https")].nodePort}')
for attempt in $(seq 1 30); do
  if sudo curl -fsS --noproxy '*' --cacert /root/CKS/portal-ca.crt --resolve "portal.training.example:$https:$node" "https://portal.training.example:$https/" >/dev/null; then break; fi
  sleep 1
done
sudo curl -fsS --noproxy '*' --cacert /root/CKS/portal-ca.crt --resolve "portal.training.example:$https:$node" "https://portal.training.example:$https/"
redirect=$(curl -sS --noproxy '*' --resolve "portal.training.example:$http:$node" -H 'Host: portal.training.example' -o /dev/null -w '%{redirect_url}' "http://portal.training.example:$http/")
case "$redirect" in https://portal.training.example/*|https://portal.training.example:*) ;; *) echo 'HTTPS redirect missing' >&2; exit 1;; esac""",
    13: manifest(restricted_egress),
    14: manifest(peer_authentication) + """
kubectl label namespace private-mesh istio-injection=enabled --overwrite
kubectl -n private-mesh rollout restart deployment/north deployment/south
kubectl -n private-mesh rollout status deployment/north --timeout=180s
kubectl -n private-mesh rollout status deployment/south --timeout=180s
kubectl -n private-mesh exec deployment/north -c north -- wget -T 3 -qO- http://south/""",
    15: """sudo kubectl --kubeconfig=/etc/kubernetes/admin.conf -n editor create secret tls editor-cert --cert=/root/portal-client.crt --key=/root/portal-client.key --dry-run=client -o yaml | kubectl apply -f -
kubectl -n editor patch deployment editor-web --type=strategic -p '{"spec":{"template":{"spec":{"volumes":[{"name":"editor-tls","secret":{"secretName":"editor-cert"}}],"containers":[{"name":"editor-web","volumeMounts":[{"name":"editor-tls","mountPath":"/etc/editor/tls","readOnly":true}]}]}}}}'
kubectl -n editor rollout status deployment/editor-web --timeout=120s""",
    16: """sudo kube-bench --config-dir /opt/kube-bench/cfg --benchmark cis-1.10 --check 1.1.11,1.1.12 run --targets master
sudo python3 - <<'PY'
from pathlib import Path
import yaml
spec = yaml.safe_load(Path('/etc/kubernetes/manifests/etcd.yaml').read_text())['spec']
container = spec['containers'][0]
data_dir = next(x.split('=', 1)[1] for x in container['command'] if x.startswith('--data-dir='))
mount = next(m for m in container['volumeMounts'] if data_dir == m['mountPath'])
host = next(v['hostPath']['path'] for v in spec['volumes'] if v['name'] == mount['name'])
if host != '/var/lib/etcd' or data_dir != '/var/lib/etcd':
    raise SystemExit('Unexpected etcd data directory; inspect the manifest before changing permissions')
print('Verified etcd data directory:', host)
PY
sudo chown -R etcd:etcd /var/lib/etcd
sudo find /var/lib/etcd -xdev -type d -exec chmod 0700 {} +
sudo find /var/lib/etcd -xdev -type f -exec chmod 0600 {} +
sudo kube-bench --config-dir /opt/kube-bench/cfg --benchmark cis-1.10 --check 1.1.11,1.1.12 --exit-code 1 run --targets master""",
}


if __name__ == "__main__":
    print(json.dumps(SOLUTIONS, indent=2))
