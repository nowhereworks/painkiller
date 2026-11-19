#!/usr/bin/env bash
# One ownership-checked disposable controller; retries preserve completed phases.
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
if [[ $phase == infrastructure ]]; then
  # cloud-init's first-boot upgrades can restart networking and change DHCP IP.
  # Observe completion through the ownership-checked VM's guest agent rather
  # than holding an SSH connection to an address that may disappear.
  deadline=$((SECONDS + 300))
  while (( SECONDS < deadline )); do
    boot=$(ssh -o BatchMode=yes -o ConnectTimeout=10 "${LAB_PROXMOX_HOST:-root@prox-k12}" \
      "qm guest exec ${LAB_VM_START:-3000} -- cloud-init status --format json")
    if jq -e '.exited == 1 and (.exitcode == 0 or .exitcode == 2) and
      (."out-data" | fromjson | .status == "done" or .status == "disabled")' <<<"$boot" >/dev/null; then
      break
    fi
    sleep 3
  done
  (( SECONDS < deadline )) || { echo 'cloud-init did not finish within 300 seconds' >&2; exit 1; }
  state=$("$root/scripts/create-lab.sh" status)
  cp=$(jq -er '.controller_ips | split(",") | select(length == 1) | .[0]' <<<"$state")
  # Pin a changed DHCP address using the guest agent, never ssh-keyscan.
  key_result=$(ssh -o BatchMode=yes -o ConnectTimeout=10 "${LAB_PROXMOX_HOST:-root@prox-k12}" \
    "qm guest exec ${LAB_VM_START:-3000} -- cat /etc/ssh/ssh_host_ed25519_key.pub")
  key=$(jq -er 'select(.exited == 1 and .exitcode == 0) | ."out-data"' <<<"$key_result")
  [[ $key =~ ^ssh-ed25519\ [a-zA-Z0-9+/=]+(\ .*)?$ ]] || { echo 'Invalid guest host key' >&2; exit 1; }
  known_hosts=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}
  [[ ! -L $known_hosts && $(head -n 1 "$known_hosts") == '# Painkiller managed host keys' ]] ||
    { echo 'Refusing unmanaged guest host-key file' >&2; exit 1; }
  grep -Fxq "$cp $key" "$known_hosts" || printf '%s\n' "$cp $key" >> "$known_hosts"
fi
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
node 'sudo -n tee /opt/painkiller/cks001b/tasks.json >/dev/null' < "$exam/tasks.json"
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
node 'kubectl wait pods --all --all-namespaces --for=condition=Ready --timeout=180s'
# shellcheck disable=SC2016 # Expand item in the guest, not on the management host.
node 'for item in immutable-workloads/report-web finance-ledger/ledger-web produce/orchard secure-portal/portal private-mesh/north private-mesh/south editor/editor-web; do
  kubectl -n "${item%/*}" rollout status "deployment/${item#*/}" --timeout=120s
done'
node 'sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf python3 -B /opt/painkiller/cks001b/rbac_capture.py baseline'
node 'sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf python3 -B /opt/painkiller/cks001b/etcd_fault.py'
bash "$exam/check-ready.sh"
node 'sudo -n touch /var/lib/painkiller/cks001b/complete'
