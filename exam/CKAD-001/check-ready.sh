#!/usr/bin/env bash
# Read-only starting-state checks, including real ingress/egress enforcement.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",")[0]' <<<"$state")
ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=yes \
  -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}" \
  "${LAB_SSH_USER:-milkman}@$cp" bash -s <<'CHECK'
set -Eeuo pipefail
k() { kubectl --request-timeout=5s "$@"; }
check() {
  k get nodes -o json | jq -e '.items | length == 3 and all(.[]; any(.status.conditions[]; .type == "Ready" and .status == "True"))' >/dev/null || return 1
  k get namespace ckad001-system -o json | jq -e '.metadata.labels["painkiller-shell/exam"] == "CKAD-001"' >/dev/null || return 1
  k -n kube-system get daemonset calico-node -o json | jq -e '.status.numberReady == 3' >/dev/null || return 1
  k get crd animes.entertainment.ckad001.test -o json | jq -e 'any(.status.conditions[]; .type == "Established" and .status == "True")' >/dev/null || return 1
  for target in 'ckad001-system observer' 'ckad001-policycheck open-web' 'ckad001-policycheck blocked-web' \
    'default parcel-web' 'default warmup-web' 'qos-workshop baseline-best-effort' 'qos-workshop baseline-guaranteed'; do
    read -r ns name <<<"$target"
    k -n "$ns" get pod "$name" -o json | jq -e 'any(.status.conditions[]?; .type == "Ready" and .status == "True")' >/dev/null || return 1
  done
  k -n database-workshop get deployment sql02 -o json | jq -e '.status.readyReplicas == 1' >/dev/null || return 1
  k -n dev-workshop get deployment bulletin-web -o json |
    jq -e '(.status.readyReplicas // 0) == 0 and .spec.template.spec.containers[0].command[2] == "/etc/nginx/missing-workshop.conf"' >/dev/null || return 1
  k -n database-workshop get pod ledger-client -o json |
    jq -e '(.spec.containers[0].envFrom // []) == [] and any(.status.containerStatuses[]?; (.restartCount > 0) or .state.terminated.exitCode == 1)' >/dev/null || return 1
  k get endpointslice host-web-backend -o json |
    jq -e '.endpoints[0].addresses == ["192.0.2.123"] and .ports[0].port == 9997' >/dev/null || return 1
  curl --connect-timeout 2 --max-time 3 -fsS http://127.0.0.1:9998 | grep -qx ckad001-host-web || return 1
  command -v helm >/dev/null || return 1
  grep -q 'app: broken-selector' /opt/lantern-chart/templates/deployment.yaml || return 1
  grep -q 'app: broken-selector' /opt/lantern-chart/templates/service.yaml || return 1
  sudo -n test -s /root/workshop-env/app.env || return 1
  sudo -n test -s /root/workshop-env/cache.env || return 1
  sudo -n test -s /root/workshop-secret-pod.yaml || return 1
  sudo -n grep -q 'apiVersion: apps/v1' /root/workshop-busybox.yaml || return 1
  k -n secret-workshop get secret vault-settings -o json | jq -e '.data[".token"] == ("workshop-token-42"|@base64)' >/dev/null || return 1
  k get namespaces -o json |
    jq -e 'all(.items[]; .metadata.name != "sealed-workshop" and .metadata.name != "quota-workshop")' >/dev/null || return 1
  # Positive controls make a stopped server or broken general networking a failure.
  local open blocked
  open=$(k -n ckad001-policycheck get pod open-web -o jsonpath='{.status.podIP}') || return 1
  blocked=$(k -n ckad001-policycheck get pod blocked-web -o jsonpath='{.status.podIP}') || return 1
  k -n ckad001-system exec observer -- wget -T 2 -qO- "http://$open:8080" | grep -q ckad001-policy || return 1
  k -n ckad001-policycheck exec blocked-web -- wget -T 2 -qO- http://127.0.0.1:8080 | grep -q ckad001-policy || return 1
  if k -n ckad001-system exec observer -- wget -T 2 -qO- "http://$blocked:8080" 2>/dev/null; then return 1; fi
  if k -n ckad001-policycheck exec blocked-web -- wget -T 2 -qO- "http://$open:8080" 2>/dev/null; then return 1; fi
}
for _ in {1..3}; do
  if check; then echo 'CKAD-001 starting state and NetworkPolicy enforcement verified'; exit 0; fi
  sleep 2
done
echo 'CKAD-001 readiness failed: inspect fixtures, faults, and policy enforcement' >&2
exit 1
CHECK
