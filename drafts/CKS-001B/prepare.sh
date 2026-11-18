#!/usr/bin/env bash
# Draft only: preflight deliberately refuses until all exam-owned components exist.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
exam=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
case "${1:-}" in
  preflight|infrastructure|fixtures) phase=$1 ;;
  *) echo 'Usage: prepare.sh {preflight|infrastructure|fixtures}' >&2; exit 2 ;;
esac
[[ ${LAB_TEMPLATE_ID:-9199} == 9199 ]] || { echo 'CKS-001B requires base 9199' >&2; exit 1; }
for name in tasks.json setup.yaml check-ready.sh assets/SHA256SUMS assets/services.py; do
  [[ -f "$exam/$name" ]] || { echo "CKS-001B incomplete: missing $name; refusing preparation" >&2; exit 1; }
done
(cd "$exam/assets" && sha256sum --check --quiet SHA256SUMS)
[[ $phase != preflight ]] || exit 0

# status validates ownership of every VM before guest IPs are returned.
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",") | select(length == 1) | .[0] | select(length > 0)' <<<"$state")
jq -e '.worker_ips == ""' <<<"$state" >/dev/null
node() {
  local command
  printf -v command '%q' "$*"
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes \
    -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}" \
    "${LAB_SSH_USER:-milkman}@$cp" "bash -e -o pipefail -c $command"
}
if node 'sudo -n test -e /var/lib/painkiller/cks001b/complete'; then
  echo 'CKS-001B already prepared; preserving candidate work'
  exit 0
fi
if [[ $phase == infrastructure ]]; then
  node 'bash -s' < "$exam/assets/install.sh"
  exit 0
fi

node 'sudo -n mkdir -p /opt/painkiller/cks001b /var/lib/painkiller/cks001b
  sudo -n touch /var/lib/painkiller/cks001b/authorized'
tar -C "$exam/assets" -cf - . | node 'sudo -n tar -C /opt/painkiller/cks001b -xf -'
node 'sudo -n tee /opt/painkiller/cks001b/setup.yaml >/dev/null' < "$exam/setup.yaml"
node 'sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf python3 -B /opt/painkiller/cks001b/host_fixtures.py
  sudo -n systemctl daemon-reload
  sudo -n systemctl enable --now falco'
if ! node 'sudo -n test -e /var/lib/painkiller/cks001b/core-applied'; then
  node 'sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf bash -e -o pipefail -s' <<'REMOTE'
[[ $(kubectl get nodes -o json | jq '.items | length') == 1 ]]
node=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
if kubectl get node "$node" -o json | jq -e 'any(.spec.taints[]?; .key == "node-role.kubernetes.io/control-plane")' >/dev/null; then
  kubectl taint node "$node" node-role.kubernetes.io/control-plane-
fi
kubectl apply -f /opt/painkiller/cks001b/setup.yaml
touch /var/lib/painkiller/cks001b/core-applied
REMOTE
fi
node 'sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf python3 -B /opt/painkiller/cks001b/services.py'
node 'sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf python3 -B /opt/painkiller/cks001b/rbac_capture.py baseline'
bash "$exam/check-ready.sh"
node 'sudo -n touch /var/lib/painkiller/cks001b/complete'
