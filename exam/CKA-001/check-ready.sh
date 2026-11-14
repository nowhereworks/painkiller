#!/usr/bin/env bash
# Read-only starting-state check, executed with the attempt's lab environment.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",")[0] | select(length > 0)' <<<"$state")
count=$(jq -er '[.controller_ips, .worker_ips | split(",")[] | select(length > 0)] | length' <<<"$state")
ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=yes \
  -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}" \
  "${LAB_SSH_USER:-milkman}@$cp" bash -s -- "$count" <<'CHECK'
set -eu
check() {
  nodes=$(kubectl get nodes -o json) || return 1
  echo "$nodes" | jq -e --argjson count "$1" \
    '.items | length == $count and all(.[]; any(.status.conditions[]; .type == "Ready" and .status == "True"))' >/dev/null || return 1
  kubectl get namespace exam-repair exam-workloads >/dev/null || return 1
  [ "$(kubectl -n exam-repair get deployment broken-web -o jsonpath='{.spec.template.spec.containers[0].image}')" = nginx:does-not-exist ] || return 1
  [ "$(kubectl -n exam-repair get service broken-endpoints -o jsonpath='{.spec.selector.app}')" = endpoint-target-stale ] || return 1
  kubectl -n exam-repair get deployment crashing endpoint-target >/dev/null || return 1
  kubectl -n exam-workloads get deployment service-target rollout >/dev/null || return 1
}
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if check "$1"; then echo "CKA-001 starting state verified"; exit 0; fi
  echo "CKA-001 starting state not yet ready (attempt $attempt)" >&2
  sleep 3
done
echo "CKA-001 readiness failed: check nodes, fixtures, and intentional faults" >&2
exit 1
CHECK
