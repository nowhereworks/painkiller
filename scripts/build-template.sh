#!/usr/bin/env bash
# Build an uninitialized Kubernetes image from the immutable base template.
set -Eeuo pipefail
host=${LAB_PROXMOX_HOST:-root@prox-k12}
id=${LAB_IMAGE_TEMPLATE_ID:-9100}
base=${LAB_BASE_TEMPLATE_ID:-9199}
version=${LAB_KUBERNETES_VERSION:-1.36.1}
# shellcheck source=scripts/playbooks.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/playbooks.sh"
user=${LAB_SSH_USER:-milkman}
prefix=${LAB_IP_PREFIX:-192.168.254.}
timeout=${LAB_READY_TIMEOUT:-600}
lockdir=${PAINKILLER_LAB_LOCK_DIR:-data}
marker=painkiller-image-v1
action=${1:-build}
[[ $# -le 1 && ( $action == build || $action == --resume ) ]] || { echo 'Usage: build-template.sh [--resume]' >&2; exit 1; }
[[ $id =~ ^[0-9]+$ && $base == 9199 && $id -ge 9100 && $id -le 9200 && $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ && $user =~ ^[a-zA-Z0-9_-]+$ && $prefix =~ ^[0-9.]+$ && $timeout =~ ^[0-9]+$ && $timeout -gt 0 ]] || { echo 'Invalid image build configuration' >&2; exit 1; }
mkdir -p "$lockdir"
exec 9>"$lockdir/lab-operation.lock"
flock -n 9 || { echo 'Another lab operation is running' >&2; exit 1; }
check_playbooks
remote() { ssh -o BatchMode=yes -o ConnectTimeout=10 "$host" "$@"; }
list=$(remote 'qm list')
if awk -v id="$id" '$1 == id { found=1 } END { exit !found }' <<<"$list"; then
  [[ $action == --resume ]] || { echo "VM $id already exists" >&2; exit 1; }
  config=$(remote "qm config $id")
  if ! grep -Fxq "description: $marker" <<<"$config" || grep -Fxq 'template: 1' <<<"$config"; then echo "VM $id is not an unfinished owned builder" >&2; exit 1; fi
  status=$(remote "qm status $id")
  [[ $status == 'status: stopped' || $status == 'status: running' ]] || { echo 'Unexpected builder VM status' >&2; exit 1; }
else
  [[ $action == build ]] || { echo "Builder VM $id does not exist" >&2; exit 1; }
fi
base_config=$(remote "qm config $base")
grep -Fxq 'template: 1' <<<"$base_config" || { echo 'Base is not a template' >&2; exit 1; }
# Reserve the full virtual disk size plus 4 GiB, even on thin storage.
disk=$(awk '/^scsi0: / {print $2}' <<<"$base_config")
storage=${disk%%:*}
size=$(sed -nE 's/.*size=([0-9]+)([MG])([^a-zA-Z].*)?$/\1 \2/p' <<<"$disk")
read -r size_number size_unit <<<"$size"
[[ $storage =~ ^[a-zA-Z0-9_-]+$ && ${size_number:-} =~ ^[0-9]+$ && ( ${size_unit:-} == M || ${size_unit:-} == G ) ]] || { echo 'Cannot determine base disk storage and size' >&2; exit 1; }
needed_kib=$((size_number * 1024))
if [[ $size_unit == G ]]; then needed_kib=$((needed_kib * 1024)); fi
if [[ $action == --resume ]]; then needed_kib=0; fi
storage_available=$(remote 'pvesm status' | awk -v storage="$storage" '$1 == storage && $3 == "active" {print $6}')
if ! [[ $storage_available =~ ^[0-9]+$ ]] || (( storage_available < needed_kib + 4194304 )); then echo 'Insufficient or unknown image storage space' >&2; exit 1; fi
available=$(remote "awk '/MemAvailable:/ {print int(\$2/1024)}' /proc/meminfo")
builder_memory=$(awk '/^memory: / {print $2}' <<<"$base_config")
[[ $builder_memory =~ ^[0-9]+$ ]] || { echo 'Cannot determine builder memory' >&2; exit 1; }
if [[ $action == --resume && $status == 'status: running' ]]; then builder_memory=0; fi
[[ $available =~ ^[0-9]+$ && $available -ge $((builder_memory+4096)) ]] || { echo 'Insufficient memory for image builder and reserve' >&2; exit 1; }
mkdir -p "${TMPDIR:-/tmp/opencode}"
work=$(mktemp -d "${TMPDIR:-/tmp/opencode}/painkiller-image.XXXXXXXX")
trap 'rm -rf "$work"' EXIT
known=$work/known_hosts
touch "$known"
chmod 600 "$known"
source_playbook=$playbooks/kubeadm-cluster/kubeadm-cluster.yml
awk '/^- name: Bootstrap first kubeadm control-plane node$/ {exit} {print}' "$source_playbook" > "$work/prepare.yml"
grep -Fxq -- '- name: Prepare all kubeadm nodes' "$work/prepare.yml" || { echo 'Unexpected playbook layout' >&2; exit 1; }
if [[ $action == build ]]; then
  remote "qm clone $base $id --name painkiller-image-$id --full 1 --description $marker"
  remote "qm config $id" | grep -Fxq "description: $marker" || { echo 'Image ownership not established; inspect manually' >&2; exit 1; }
  remote "qm start $id"
else
  if [[ $status == 'status: stopped' ]]; then remote "qm start $id"; fi
fi
deadline=$((SECONDS+timeout))
ip=
while (( SECONDS < deadline )); do
  interfaces=$(remote "qm agent $id network-get-interfaces" 2>/dev/null || true)
  ips=$(jq -r --arg prefix "$prefix" '[.[] | ."ip-addresses"[]? | select(."ip-address-type" == "ipv4") | ."ip-address" | select(startswith($prefix))] | unique | .[]' <<<"${interfaces:-[]}" 2>/dev/null || true)
  if [[ $(grep -c . <<<"$ips") == 1 ]]; then
    keyjson=$(remote "qm guest exec $id -- cat /etc/ssh/ssh_host_ed25519_key.pub" 2>/dev/null || true)
    key=$(jq -er 'select(.exited == 1 and .exitcode == 0) | ."out-data"' <<<"${keyjson:-null}" 2>/dev/null || true)
    if [[ $key =~ ^ssh-ed25519\ [a-zA-Z0-9+/=]+(\ .*)?$ ]]; then
      printf '%s %s\n' "$ips" "$key" > "$known"
      if ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" "$user@$ips" true </dev/null 2>/dev/null; then ip=$ips; break; fi
    fi
  fi
  sleep 3
done
[[ -n $ip ]] || { echo "Image VM $id did not become reachable; inspect manually" >&2; exit 1; }
result=$(remote "qm guest exec $id -- sh -c 'set -e; id $user >/dev/null; mkdir -p /etc/sudoers.d; printf \"%s ALL=(ALL) NOPASSWD:ALL\\n\" $user > /etc/sudoers.d/painkiller-shell; chmod 440 /etc/sudoers.d/painkiller-shell; visudo -cf /etc/sudoers.d/painkiller-shell >/dev/null'")
jq -e '.exited == 1 and .exitcode == 0' <<<"$result" >/dev/null || { echo 'Failed to configure builder sudo' >&2; exit 1; }
ssh_args="-o UserKnownHostsFile=$known -o GlobalKnownHostsFile=/dev/null -o StrictHostKeyChecking=yes"
node() { ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" "$user@$ip" "$@"; }
node 'if command -v cloud-init >/dev/null; then sudo -n cloud-init status --wait || true; fi'
ansible-playbook -i 'localhost,' -c local "$work/prepare.yml" -e "controller_ips=$ip ansible_ssh_user=$user kubernetes_version=$version" --ssh-common-args "$ssh_args"
# Install the requested patch explicitly; the upstream preparation play installs
# the repository's current version, which may be newer than the requested image.
node bash -s -- "$version" <<'GUEST'
set -euo pipefail
version=$1
available_versions=$(apt-cache madison kubeadm)
pkg=$(awk -v version="$version" 'index($3, version "-") == 1 {print $3; exit}' <<<"$available_versions")
test -n "$pkg"
sudo -n apt-mark unhold kubeadm kubelet kubectl
sudo -n apt-get install -y --allow-downgrades "kubeadm=$pkg" "kubelet=$pkg" "kubectl=$pkg"
sudo -n apt-mark hold kubeadm kubelet kubectl
for p in kubelet kubeadm kubectl; do test "$(dpkg-query -W -f='${Version}' "$p")" = "$pkg"; done
sudo -n apt-get update
sudo -n apt-get install -y jq etcd-client
sudo -n mkdir -p /opt/painkiller
sudo -n curl -fsSL https://github.com/flannel-io/flannel/releases/download/v0.28.4/kube-flannel.yml -o /opt/painkiller/flannel.yml
sudo -n kubeadm config images pull --kubernetes-version="$version"
# The CRI sandbox default can differ from kubeadm's pause version. Cache both
# so kubeadm preflight cannot trigger an additional sandbox download.
runtime_config=$(sudo -n containerd config dump)
sandbox_image=$(sed -nE "/^[[:space:]]*(sandbox|sandbox_image)[[:space:]]*=/ s/.*=[[:space:]]*['\"]([^'\"]+)['\"].*/\\1/p" <<<"$runtime_config")
[[ $sandbox_image == registry.k8s.io/pause:* && $sandbox_image != *$'\n'* ]]
sudo -n crictl pull "$sandbox_image"
printf '%s\n' "$sandbox_image" | sudo -n tee /opt/painkiller/sandbox-image >/dev/null
for image in $(grep -E '^[[:space:]]*image:' /opt/painkiller/flannel.yml | awk '{print $2}' | sort -u) docker.io/library/nginx:1.27 docker.io/library/nginx:1.26 docker.io/library/busybox:1.36; do sudo -n crictl pull "$image"; done
sudo -n test ! -e /etc/kubernetes/admin.conf
sudo -n test ! -e /etc/kubernetes/kubelet.conf
sudo -n test ! -d /etc/kubernetes/pki
sudo -n test ! -d /var/lib/etcd
dpkg-query -W -f='${Package}=${Version}\n' kubelet kubeadm kubectl containerd cri-tools jq etcd-client | sudo -n tee /opt/painkiller/packages >/dev/null
sudo -n crictl images -o json | sudo -n tee /opt/painkiller/images.json >/dev/null
sudo -n sha256sum /opt/painkiller/flannel.yml | sudo -n tee /opt/painkiller/flannel.sha256 >/dev/null
GUEST
remote "qm shutdown $id --timeout 120"
remote "qm status $id" | grep -Fxq 'status: stopped' || { echo 'Image VM is not stopped; inspect manually' >&2; exit 1; }
remote "qm set $id --description ${marker}"
remote "qm template $id"
echo "Built template $id; validate clones before use."
