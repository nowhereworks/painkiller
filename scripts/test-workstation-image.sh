#!/usr/bin/env bash
# Two disposable linked clones; does not install Kubernetes or touch app state.
set -Eeuo pipefail
umask 077
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
host=${LAB_PROXMOX_HOST:-root@prox-k12}
template=${LAB_WORKSTATION_TEMPLATE_ID:-9150}
start=${WORKSTATION_TEST_START:?Set WORKSTATION_TEST_START to a verified-free pair of VM IDs}
user=${LAB_SSH_USER:-milkman}
prefix=${LAB_IP_PREFIX:-192.168.254.}
lockdir=${PAINKILLER_LAB_LOCK_DIR:-$root/data}
skip_dns=${WORKSTATION_TEST_SKIP_DNS:-0}
[[ $start =~ ^[0-9]+$ && $start -ge 3000 && $start -lt 3999 &&
   $template =~ ^[0-9]+$ && $template -ge 9100 && $template -le 9200 && $template != 9199 &&
   $user =~ ^[a-z_][a-z0-9_-]*$ && $prefix =~ ^[0-9.]+$ && $host != -* &&
   ( $skip_dns == 0 || $skip_dns == 1 ) ]] || { echo 'Invalid clone-test configuration' >&2; exit 1; }
end=$((start+1))
mkdir -p "$lockdir" "$root/data"
exec 9>"$lockdir/lab-operation.lock"
flock -n 9 || { echo 'Another lab operation is running' >&2; exit 1; }
remote() { ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes "$host" "$@"; }
remote bash -s -- "$start" "$end" "$template" <<'PREFLIGHT'
set -Eeuo pipefail
start=$1 end=$2 template=$3
list=$(qm list)
for id in "$start" "$end"; do
  if awk -v id="$id" '$1 == id {found=1} END {exit !found}' <<<"$list"; then echo "VM $id occupied; refusing test" >&2; exit 1; fi
done
config=$(qm config "$template")
grep -Fxq 'template: 1' <<<"$config"
grep -Fxq 'description: painkiller-workstation-image-v1' <<<"$config"
available=$(awk '/MemAvailable:/ {print int($2/1024)}' /proc/meminfo)
(( available >= 12288 )) || { echo 'Need 12 GiB available memory including reserve' >&2; exit 1; }
storage=$(sed -n 's/^scsi0: \([^:]*\):.*/\1/p' <<<"$config")
free_kib=$(pvesm status | awk -v storage="$storage" '$1 == storage && $3 == "active" {print $6}')
[[ $free_kib =~ ^[0-9]+$ && $free_kib -ge 10485760 ]] || { echo 'Need 10 GiB free clone storage' >&2; exit 1; }
PREFLIGHT
work=$(mktemp -d "$root/data/workstation-test.XXXXXXXX")
marker=painkiller-workstation-test-${work##*.}
known=$work/known_hosts
echo "Workstation test IDs: $start–$end; marker: $marker; artifacts: $work"
trap 'echo "Test failed. Preserve and inspect IDs $start–$end, marker $marker, artifacts $work before cleanup." >&2' ERR
owned() { remote "qm config $1" | grep -Fxq "description: $marker"; }
node() { ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" "$user@$ip" "$@"; }
for ((id=start; id<=end; id++)); do
  remote "qm clone $template $id --name k8s-$id --full 0 --description $marker"
  owned "$id"
  remote "qm set $id --memory 4096"
  remote "qm start $id"
done
for ((id=start; id<=end; id++)); do
  deadline=$((SECONDS+900))
  ready=false
  while (( SECONDS < deadline )); do
    owned "$id"
    boot=$(remote "qm guest exec $id -- /usr/local/bin/painkiller-check-cloud-init" 2>/dev/null || true)
    if jq -e '.exited == 1 and .exitcode == 0' <<<"${boot:-null}" >/dev/null 2>&1; then ready=true; break; fi
    sleep 3
  done
  $ready || { echo "VM $id cloud-init did not complete" >&2; exit 1; }
  interfaces=$(remote "qm agent $id network-get-interfaces")
  ip=$(jq -er --arg prefix "$prefix" '[.[] | ."ip-addresses"[]? | select(."ip-address-type" == "ipv4") | ."ip-address" | select(startswith($prefix))] | unique | select(length == 1) | .[0]' <<<"$interfaces")
  result=$(remote "qm guest exec $id -- cat /etc/ssh/ssh_host_ed25519_key.pub")
  key=$(jq -er 'select(.exited == 1 and .exitcode == 0) | ."out-data"' <<<"$result")
  [[ $key =~ ^ssh-ed25519\ [a-zA-Z0-9+/=]+(\ .*)?$ ]]
  printf '%s %s\n' "$ip" "$key" >> "$known"
  name=$(node hostname)
  printf '%s\n' "$ip" > "$work/$id.ip"
  printf '%s\n' "$key" > "$work/$id.key"
  printf '%s\n' "$name" > "$work/$id.hostname"
  [[ $name == "k8s-$id" ]]
  # Bounded readiness, never repair/reinstall the image during validation.
  # shellcheck disable=SC2016 # Arguments expand inside the bounded child shell.
  timeout 90s bash -c '
    while ! ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=yes -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$1" "$2" "systemctl is-active --quiet painkiller-desktop.service"; do sleep 2; done
  ' bash "$known" "$user@$ip"
  if [[ $skip_dns == 1 ]]; then
    node bash -s -- --skip-dns < "$root/scripts/validate-workstation-image.sh" | tee "$work/$id.validation.log"
  else
    node bash -s < "$root/scripts/validate-workstation-image.sh" | tee "$work/$id.validation.log"
  fi
  node python3 - < "$root/scripts/workstation/check-desktop.py" | tee "$work/$id.desktop.log"
done
for kind in ip key hostname; do
  if cmp -s "$work/$start.$kind" "$work/$end.$kind"; then echo "Duplicate clone $kind; refusing success" >&2; exit 1; fi
done
# Precheck every clone before any destruction; no marker-only broad cleanup.
for ((id=start; id<=end; id++)); do owned "$id"; done
for ((id=start; id<=end; id++)); do
  remote "qm stop $id"
  remote "qm destroy $id"
done
trap - ERR
echo "PASS: two distinct linked clones, desktop smoke checks and cleanup. Artifacts: $work"
if [[ $skip_dns == 1 ]]; then echo 'SKIP: live DNS registration; this is not a complete DNS-readiness pass.'; fi
