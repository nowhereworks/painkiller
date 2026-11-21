#!/usr/bin/env bash
# Management-host entry point. This image is independent of Kubernetes base 9199.
set -Eeuo pipefail
umask 077
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
host=${LAB_PROXMOX_HOST:-root@prox-k12}
id=${LAB_WORKSTATION_TEMPLATE_ID:-9150}
storage=${WORKSTATION_STORAGE:-local-lvm}
bridge=${WORKSTATION_BRIDGE:-vmbr0}
user=${LAB_SSH_USER:-milkman}
key=${WORKSTATION_SSH_PUBLIC_KEY:-$HOME/.ssh/id_rsa.pub}
prefix=${LAB_IP_PREFIX:-192.168.254.}
memory=${WORKSTATION_MEMORY_MB:-4096}
disk=${WORKSTATION_DISK_GB:-32}
base_url=${WORKSTATION_IMAGE_BASE_URL:-https://cloud-images.ubuntu.com/resolute/current}
keyring=${WORKSTATION_IMAGE_KEYRING:-/usr/share/keyrings/ubuntu-cloudimage-keyring.gpg}
dns_api=${WORKSTATION_DNS_API:-http://192.168.254.4:8888}
dns_zone=${WORKSTATION_DNS_ZONE:-nowhereworks.local}
dns_route=${WORKSTATION_DNS_ROUTE:-192.168.254.4}
lockdir=${PAINKILLER_LAB_LOCK_DIR:-$root/data}
marker=painkiller-workstation-image-v1
image=resolute-server-cloudimg-amd64.img
[[ $# == 0 ]] || { echo 'Usage: scripts/build-workstation-template.sh (see docs/workstation-image.md)' >&2; exit 1; }
[[ $id =~ ^[0-9]+$ && $id -ge 9100 && $id -le 9200 && $id != 9199 &&
   $memory =~ ^[0-9]+$ && $memory -ge 4096 && $disk =~ ^[0-9]+$ && $disk -ge 32 &&
   $storage =~ ^[a-zA-Z0-9_-]+$ && $bridge =~ ^[a-zA-Z0-9_-]+$ &&
   $user =~ ^[a-z_][a-z0-9_-]*$ && $user != root && $prefix =~ ^[0-9.]+$ &&
   $base_url == https://cloud-images.ubuntu.com/resolute/* &&
   $base_url != *[[:space:]]* && $host != -* ]] || { echo 'Invalid workstation image configuration' >&2; exit 1; }
for tool in ssh scp curl gpgv sha256sum jq flock ansible-playbook; do
  command -v "$tool" >/dev/null || { echo "Missing $tool" >&2; exit 1; }
done
[[ -s $key && -s $keyring ]] || { echo 'A management SSH public key and trusted Ubuntu cloud-image keyring are required' >&2; exit 1; }
ssh-keygen -lf "$key" >/dev/null
(cd "$root/playbooks/workstation" && sha256sum --check --status SHA256SUMS)
mkdir -p "$lockdir" "$root/data"
(( $(df -Pk "$root/data" | awk 'END {print $4}') >= 2097152 )) || { echo 'Need 2 GiB local artifact space' >&2; exit 1; }
exec 9>"$lockdir/lab-operation.lock"
flock -n 9 || { echo 'Another lab operation is running' >&2; exit 1; }
remote() { ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes "$host" "$@"; }
# Refuse occupied IDs, unknown capacity and missing prerequisites before downloads.
remote bash -s -- "$id" "$storage" "$bridge" "$memory" "$disk" <<'PREFLIGHT'
set -Eeuo pipefail
id=$1 storage=$2 bridge=$3 memory=$4 disk=$5
list=$(qm list)
if awk -v id="$id" '$1 == id {found=1} END {exit !found}' <<<"$list"; then echo "VM $id already exists; refusing to overwrite it" >&2; exit 1; fi
for tool in qm pvesm virt-customize qemu-img sha256sum python3; do command -v "$tool" >/dev/null; done
ip link show "$bridge" >/dev/null
free_kib=$(pvesm status | awk -v storage="$storage" '$1 == storage && $3 == "active" {print $6}')
[[ $free_kib =~ ^[0-9]+$ && $free_kib -ge $(((disk+4)*1048576)) ]] || { echo 'Insufficient or unknown VM storage capacity' >&2; exit 1; }
available=$(awk '/MemAvailable:/ {print int($2/1024)}' /proc/meminfo)
(( available >= memory+4096 )) || { echo 'Insufficient memory including 4 GiB reserve' >&2; exit 1; }
# The verified download and libguestfs scratch use tmpfs, not Proxmox root storage.
(( $(df -Pk /tmp | awk 'END {print $4}') >= 4194304 )) || { echo 'Need 4 GiB temporary storage' >&2; exit 1; }
PREFLIGHT
work=$(mktemp -d "$root/data/workstation-image-$id.XXXXXXXX")
echo "Build artifacts: $work"
echo 'Downloading and verifying the Ubuntu LTS cloud image...'
curl -fSL --retry 3 --connect-timeout 15 --max-time 120 "${base_url%/}/SHA256SUMS" -o "$work/SHA256SUMS"
curl -fSL --retry 3 --connect-timeout 15 --max-time 120 "${base_url%/}/SHA256SUMS.gpg" -o "$work/SHA256SUMS.gpg"
gpgv --keyring "$keyring" "$work/SHA256SUMS.gpg" "$work/SHA256SUMS"
expected=$(awk -v image="$image" '$2 == "*" image || $2 == image {print $1}' "$work/SHA256SUMS")
[[ $expected =~ ^[a-f0-9]{64}$ ]] || { echo 'Missing or ambiguous signed image checksum' >&2; exit 1; }
curl -fSL --retry 3 --connect-timeout 15 --max-time 1800 "${base_url%/}/$image" -o "$work/$image"
printf '%s  %s\n' "$expected" "$image" > "$work/image.sha256"
(cd "$work" && sha256sum --check image.sha256)
jq -n --arg url "${base_url%/}/$image" --arg sha256 "$expected" --arg template_id "$id" \
  --arg built_at "$(date -u +%FT%TZ)" --arg proxmox_host "$host" --arg user "$user" \
  --arg storage "$storage" --arg bridge "$bridge" --argjson memory_mb "$memory" --argjson disk_gb "$disk" \
  '{source_url:$url,source_sha256:$sha256,template_id:$template_id,built_at:$built_at,proxmox_host:$proxmox_host,user:$user,storage:$storage,bridge:$bridge,memory_mb:$memory_mb,disk_gb:$disk_gb}' > "$work/source.json"
cp "$key" "$work/management.pub"
remote_work=$(remote 'mkdir -p /tmp/opencode && mktemp -d /tmp/opencode/painkiller-workstation.XXXXXXXX')
[[ $remote_work =~ ^/tmp/opencode/painkiller-workstation\.[a-zA-Z0-9]+$ ]] || { echo 'Unexpected remote scratch directory' >&2; exit 1; }
# Preserve artifacts/VM on failure; never infer that an interrupted import is safe to delete.
trap 'echo "Build interrupted. Inspect VM $id, $work and $host:$remote_work before retry or cleanup." >&2' ERR
scp -q -o BatchMode=yes -o StrictHostKeyChecking=yes "$work/$image" "$work/image.sha256" "$work/management.pub" "$host:$remote_work/"
remote bash -s -- "$remote_work" "$id" "$storage" "$bridge" "$user" "$memory" "$disk" "$marker" < "$root/scripts/workstation/create-image.sh"
known=$work/known_hosts
node() { ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" "$user@$ip" "$@"; }
require_owned() { remote "qm config $id" | grep -Fxq "description: $marker"; }
wait_guest() {
  local deadline=$((SECONDS+900)) interfaces keyjson hostkey
  while (( SECONDS < deadline )); do
    require_owned
    interfaces=$(remote "qm agent $id network-get-interfaces" 2>/dev/null || true)
    ip=$(jq -r --arg prefix "$prefix" '[.[] | ."ip-addresses"[]? | select(."ip-address-type" == "ipv4") | ."ip-address" | select(startswith($prefix))] | unique | if length == 1 then .[0] else empty end' <<<"${interfaces:-[]}" 2>/dev/null || true)
    if [[ -n $ip ]]; then
      keyjson=$(remote "qm guest exec $id -- cat /etc/ssh/ssh_host_ed25519_key.pub" 2>/dev/null || true)
      hostkey=$(jq -er 'select(.exited == 1 and .exitcode == 0) | ."out-data"' <<<"${keyjson:-null}" 2>/dev/null || true)
      if [[ $hostkey =~ ^ssh-ed25519\ [a-zA-Z0-9+/=]+(\ .*)?$ ]]; then
        printf '%s %s\n' "$ip" "$hostkey" > "$known"
        if node true </dev/null 2>/dev/null; then return; fi
      fi
    fi
    sleep 3
  done
  echo 'Builder did not become reachable; inspect its guest-agent and cloud-init state' >&2
  return 1
}
wait_guest
# shellcheck disable=SC2016 # Inspect the guest command's exit status.
node 'sudo -n cloud-init status --wait >/dev/null; code=$?; test "$code" = 0 || test "$code" = 2'
node python3 - < "$root/playbooks/workstation/files/check-cloud-init.py"
# The playbook targets only this freshly created, ownership-checked builder.
require_owned
jq -n --arg ip "$ip" --arg user "$user" \
  '{all:{hosts:{workstation:{ansible_host:$ip,ansible_user:$user,ansible_become:true}}}}' > "$work/inventory.json"
jq -n --arg user "$user" --arg api "$dns_api" --arg zone "$dns_zone" --arg route "$dns_route" \
  '{workstation_user:$user,workstation_dns_api:$api,workstation_dns_zone:$zone,workstation_dns_route:$route}' > "$work/vars.json"
ANSIBLE_FORCE_COLOR=1 ansible-playbook -i "$work/inventory.json" "$root/playbooks/workstation/workstation.yml" \
  -e "@$work/vars.json" --ssh-common-args "-o StrictHostKeyChecking=yes -o GlobalKnownHostsFile=/dev/null -o UserKnownHostsFile=$known" | tee "$work/ansible.log"
require_owned
wait_guest
node 'sudo -n install -d -m 755 /opt/painkiller/workstation'
node 'sudo -n tee /opt/painkiller/workstation/source.json >/dev/null' < "$work/source.json"
node 'sudo -n tee /opt/painkiller/workstation/recipe.sha256 >/dev/null' < "$root/playbooks/workstation/SHA256SUMS"
node bash -s -- --skip-dns < "$root/scripts/validate-workstation-image.sh" | tee "$work/validation.log"
node python3 - < "$root/scripts/workstation/check-desktop.py" | tee "$work/desktop.log"
# This is image preparation, never a disposable-clone workaround.
# Future clones must run cloud-init with their own Proxmox metadata and host keys.
require_owned
# shellcheck disable=SC2016 # Expand HOME on the builder, not on management.
node 'set -eu
sudo -n systemctl stop painkiller-desktop.service
sudo -n rm -rf "$HOME/.cache" "$HOME/.mozilla" "$HOME/snap"
sudo -n rm -f /etc/ssh/ssh_host_* /var/lib/systemd/random-seed
sudo -n cloud-init clean --logs --machine-id --seed
sudo -n sync'
remote "qm shutdown $id --timeout 120"
remote "qm status $id" | grep -Fxq 'status: stopped'
require_owned
remote "qm template $id"
# Delete only known scratch inputs after success; retain signed provenance locally.
remote "rm -f '$remote_work/$image' '$remote_work/image.sha256' '$remote_work/management.pub' && rmdir '$remote_work'"
trap - ERR
echo "Built workstation template $id. Validate disposable linked clones before use. Artifacts: $work"
