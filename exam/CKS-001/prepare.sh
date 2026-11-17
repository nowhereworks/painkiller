#!/usr/bin/env bash
# All exam state belongs to one ownership-checked disposable controller.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
exam=$root/exam/CKS-001
case "${1:-}" in
preflight)
  [[ ${LAB_TEMPLATE_ID:-9199} == 9199 ]] || { echo 'CKS-001 requires base 9199' >&2; exit 1; }
  (cd "$exam/assets" && sha256sum --check --quiet SHA256SUMS)
  exit 0 ;;
infrastructure|fixtures) ;;
*) echo "Usage: $0 {preflight|infrastructure|fixtures}" >&2; exit 2 ;;
esac
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",") | select(length == 1) | .[0]' <<<"$state")
jq -e '.worker_ips == ""' <<<"$state" >/dev/null
node() {
  local command
  printf -v command '%q' "$*"
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes \
    -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}" \
    "${LAB_SSH_USER:-milkman}@$cp" "bash -e -o pipefail -c $command"
}
if [[ $1 == infrastructure ]]; then
  node 'bash -s' < "$exam/assets/install.sh"
  exit 0
fi
if node 'sudo -n test -e /var/lib/painkiller/cks001/complete'; then
  echo 'CKS-001 already prepared; preserving candidate work'
  exit 0
fi
node 'sudo -n mkdir -p /opt/painkiller/cks001'
tar -C "$exam/assets" -cf - . | node 'sudo -n tar -C /opt/painkiller/cks001 -xf -'
node 'sudo -n tee /opt/painkiller/cks001/setup.yaml >/dev/null' < "$exam/setup.yaml"
node 'sudo -n tee /opt/painkiller/cks001/tasks.json >/dev/null' < "$exam/tasks.json"
node 'sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf python3 -B /opt/painkiller/cks001/fixtures.py'
bash "$exam/check-ready.sh"
node 'sudo -n touch /var/lib/painkiller/cks001/complete'
