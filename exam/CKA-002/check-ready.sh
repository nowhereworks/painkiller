#!/usr/bin/env bash
# Read-only starting-state check; the east worker is intentionally NotReady.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state=$("$root/scripts/create-lab.sh" status)
IFS=, read -r cp cp2 cp3 <<<"$(jq -er .controller_ips <<<"$state")"
IFS=, read -r west east <<<"$(jq -er .worker_ips <<<"$state")"
[[ -n $cp && -n $cp2 && -n $cp3 && -n $west && -n $east ]] || { echo "CKA-002: missing lab addresses" >&2; exit 1; }
ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=yes \
  -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}" \
  "${LAB_SSH_USER:-milkman}@$cp" bash -s -- "$cp2" "$cp3" "$west" "$east" <<'CHECK'
set -eu
check() {
  nodes=$(kubectl get nodes -o json) || return 1
  echo "$nodes" | jq -e --arg west "$3" --arg east "$4" --arg cp3 "$2" '
    def ip($a): any(.status.addresses[]; .address == $a);
    def ready: any(.status.conditions[]; .type == "Ready" and .status == "True");
    .items | length == 4 and
    (any(.[]; ip($west) and .metadata.labels["topology.kubernetes.io/zone"] == "west" and ready)) and
    (any(.[]; ip($east) and .metadata.labels["topology.kubernetes.io/zone"] == "east" and (ready | not))) and
    (all(.[]; if ip($east) then true else ready end)) and
    (all(.[]; ip($cp3) | not))' >/dev/null || return 1
  kubectl get namespace exam-workloads exam-rbac exam-helm exam-ops exam-netpol exam-gw exam-scheduling exam-ingress exam-lb exam-storage exam-repair >/dev/null || return 1
  kubectl -n exam-workloads get deployment web app >/dev/null || return 1
  kubectl -n exam-workloads get pdb web-pdb >/dev/null || return 1
  [ "$(kubectl -n exam-workloads get deployment app -o jsonpath='{.spec.template.spec.containers[0].image}')" = nginx:9.9.9 ] || return 1
  kubectl -n kube-system get configmap coredns -o json | jq -e '.data.Corefile | contains("forward . 1.2.3.4")' >/dev/null || return 1
  for target in 'kube-system metrics-server' 'local-path-storage local-path-provisioner' 'metallb-system controller' 'traefik traefik'; do
    set -- $target
    kubectl -n "$1" get deployment "$2" -o json | jq -e '(.status.availableReplicas // 0) >= 1' >/dev/null || return 1
  done
}
for attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  if check "$@"; then break; fi
  echo "CKA-002 starting state not yet ready (attempt $attempt)" >&2
  [ "$attempt" -lt 15 ] || { echo "CKA-002 readiness failed: check nodes, fixtures, dependencies and faults" >&2; exit 1; }
  sleep 3
done
# Use the student's node-to-node access and inspect the three guest-level faults.
ssh -o BatchMode=yes -o ConnectTimeout=5 "$4" 'grep -q painkiller-fault /var/lib/kubelet/config.yaml' || { echo "CKA-002: east kubelet fault or student SSH missing" >&2; exit 1; }
ssh -o BatchMode=yes -o ConnectTimeout=5 "$1" 'sudo -n grep -q "kube-scheduler:v9.9.9" /etc/kubernetes/manifests/kube-scheduler.yaml' || { echo "CKA-002: scheduler fault or student SSH missing" >&2; exit 1; }
ssh -o BatchMode=yes -o ConnectTimeout=5 "$2" 'test "$(cat /proc/sys/net/ipv4/ip_forward)" = 0' || { echo "CKA-002: controller network fault or student SSH missing" >&2; exit 1; }
echo "CKA-002 starting state verified"
CHECK
