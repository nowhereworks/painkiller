#!/usr/bin/env bash
# Manage only VMs explicitly marked as belonging to this lab.
# Templates are pre-configured (SSH keys, DNS, DHCP identity); clones only
# need to be started and waited for. No net0/cloud-init/identity manipulation.
set -Eeuo pipefail
command=${1:-help}
host=${LAB_PROXMOX_HOST:-root@prox-k12}
mode=${LAB_PROVISION_MODE:-prebaked}
if [[ $mode == prebaked ]]; then template=${LAB_TEMPLATE_ID:-9100}; else template=${LAB_TEMPLATE_ID:-9010}; fi
start=${LAB_VM_START:-3000}
end=${LAB_VM_END:-3002}
controllers=${LAB_CONTROLLERS:-1}
initial_controllers=${LAB_INITIAL_CONTROLLERS:-$controllers}
marker=${LAB_MARKER:-painkiller-mvp}
prefix=${LAB_IP_PREFIX:-192.168.254.}
timeout=${LAB_READY_TIMEOUT:-600}
ssh_user=${LAB_SSH_USER:-milkman}
# shellcheck source=scripts/playbooks.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/playbooks.sh"
memory=${LAB_VM_MEMORY:-3072}
known_hosts=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}
version=${LAB_KUBERNETES_VERSION:-1.36.1}
api_vip=${LAB_CONTROL_PLANE_ENDPOINT:-}
api_vip=${api_vip%:*}

[[ $start =~ ^[0-9]+$ && $end =~ ^[0-9]+$ && $template =~ ^[0-9]+$ && $controllers =~ ^[0-9]+$ && $initial_controllers =~ ^[0-9]+$ && $timeout =~ ^[0-9]+$ && $memory =~ ^[0-9]+$ ]] || { echo 'Invalid numeric configuration' >&2; exit 1; }
[[ $start -ge 3000 && $end -le 3999 && $start -le $end && $controllers -gt 0 && $controllers -le $((end-start+1)) && $timeout -gt 0 ]] || { echo 'VM range must be within 3000–3999 and controller count must be valid' >&2; exit 1; }
[[ $marker =~ ^[a-zA-Z0-9_-]+$ && $prefix =~ ^[0-9.]+$ && $ssh_user =~ ^[a-zA-Z0-9_-]+$ ]] || { echo 'Invalid marker, IP prefix, or SSH user' >&2; exit 1; }
[[ $mode == legacy || $mode == prebaked ]] || { echo 'Invalid provisioning mode' >&2; exit 1; }
(( initial_controllers >= 1 && initial_controllers <= controllers )) || { echo 'Invalid initial controller count' >&2; exit 1; }
if [[ $mode == prebaked ]]; then
  [[ $template -ge 9100 && $template -le 9200 && $controllers == 1 && ${LAB_CNI_PLUGIN:-flannel} == flannel && $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ && -z ${LAB_CONTROL_PLANE_ENDPOINT:-} ]] || { echo 'Prebaked mode requires a Painkiller template, one controller, Flannel, and no custom endpoint' >&2; exit 1; }
fi
remote() {
  # Retry only when the SSH connection itself could not be established (e.g. a
  # transient DNS blip), never on a command error, so non-idempotent commands
  # such as `qm clone` are not repeated.
  local out err errfile
  errfile=$(mktemp) || return 1
  for _ in 1 2 3 4 5 6; do
    if out=$(ssh -o BatchMode=yes -o ConnectTimeout=10 "$host" "$@" 2>"$errfile"); then
      if [[ -s $errfile ]]; then cat "$errfile" >&2; fi
      rm -f "$errfile"; printf '%s\n' "$out"; return 0
    fi
    err=$(cat "$errfile" 2>/dev/null || true)
    case "$err" in
      *"Could not resolve hostname"*|*"Temporary failure in name resolution"*|*"Connection refused"*|*"Connection timed out"*|*"Network is unreachable"*|*"No route to host"*) sleep 3 ;;
      *) printf '%s\n' "$err" >&2; rm -f "$errfile"; return 1 ;;
    esac
  done
  printf '%s\n' "$err" >&2; rm -f "$errfile"; return 1
}
node_ssh() { ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known_hosts" "$@"; }
list_vms() { vm_list=$(remote 'qm list') || { echo 'Could not list VMs; refusing operation' >&2; exit 1; }; }
exists() { awk -v id="$1" '$1 == id { found=1 } END { exit !found }' <<<"$vm_list"; }
owned() { remote "qm config $1" | grep -Fxq "description: $marker"; }
require_owned() { owned "$1" || { echo "VM $1 is not marked $marker; refusing operation" >&2; exit 1; }; }
ip_for() {
  local json ips
  json=$(remote "qm agent $1 network-get-interfaces") || return 1
  # A balancing worker also carries the /32 API VIP. That is an endpoint,
  # never the guest's DHCP/SSH identity.
  ips=$(jq -r --arg prefix "$prefix" --arg vip "$api_vip" '[.[] | ."ip-addresses"[]? | select(."ip-address-type" == "ipv4") | select(."ip-address" != $vip or .prefix != 32) | ."ip-address" | select(startswith($prefix))] | unique | .[]' <<<"$json") || return 1
  [[ $(grep -c . <<<"$ips") == 1 ]] || { echo "VM $1 must have exactly one primary lab IPv4 address (excluding the API VIP)" >&2; return 1; }
  printf '%s\n' "$ips"
}
guest_host_key() {
  local id=$1 result key
  result=$(remote "qm guest exec $id -- cat /etc/ssh/ssh_host_ed25519_key.pub") || return 1
  key=$(jq -er 'select(.exited == 1 and .exitcode == 0) | ."out-data"' <<<"$result") || return 1
  [[ $key =~ ^ssh-ed25519\ [a-zA-Z0-9+/=]+(\ .*)?$ ]] || return 1
  printf '%s\n' "$key"
}
trust_guest() {
  local id=$1 ip=$2 key line
  key=$(guest_host_key "$id") || return 1
  line="$ip $key"
  grep -Fxq "$line" "$known_hosts" || printf '%s\n' "$line" >> "$known_hosts"
}
wait_ip() {
  local id=$1 deadline=$((SECONDS+timeout)) ip
  while (( SECONDS < deadline )); do
    if ip=$(ip_for "$id" 2>/dev/null) && trust_guest "$id" "$ip" && node_ssh "$ssh_user@$ip" true </dev/null >/dev/null 2>&1; then
      printf '%s\n' "$ip"; return
    fi
    sleep 3
  done
  echo "Timed out waiting for guest agent and SSH on VM $id" >&2; return 1
}
case "$command" in
  help) echo "Usage: $0 {preflight|create|prerequisites|provision|status|start|stop|destroy}" ;;
  preflight|create)
    started=$SECONDS
    check_playbooks
    # Preflight every ID before cloning anything; an SSH failure must not mean 'free'.
    list_vms
    for ((id=start; id<=end; id++)); do
      if exists "$id"; then echo "VM $id already exists; refusing to create" >&2; exit 1; fi
    done
    [[ $known_hosts != *[[:space:]]* && $known_hosts != *"'"* && ! -L $known_hosts ]] || { echo 'Unsafe known-hosts path' >&2; exit 1; }
    if [[ -e $known_hosts ]] && [[ $(head -n 1 "$known_hosts") != '# Painkiller managed host keys' ]]; then
      echo "Refusing to overwrite unmanaged $known_hosts" >&2; exit 1
    fi
    config=$(remote "qm config $template") || { echo 'Cannot inspect image template' >&2; exit 1; }
    grep -Fxq 'template: 1' <<<"$config" || { echo 'Selected image is not a template' >&2; exit 1; }
    available=$(remote "awk '/MemAvailable:/ {print int(\$2/1024)}' /proc/meminfo")
    if ! [[ $available =~ ^[0-9]+$ ]] || (( available < (end-start+1)*memory + 4096 )); then
      echo 'Insufficient available host memory (including 4 GiB reserve)' >&2; exit 1
    fi
    if [[ $command == preflight ]]; then exit 0; fi
    mkdir -p "$(dirname "$known_hosts")"
    (umask 077; printf '%s\n' '# Painkiller managed host keys' > "$known_hosts")
    for ((id=start; id<=end; id++)); do
      echo "Creating linked clone VM $id" >&2
      remote "qm clone $template $id --name k8s-$id --full 0 --description $marker"
      require_owned "$id"
      remote "qm set $id --memory $memory"
      remote "qm start $id"
    done
    echo "Linked cloning and start: $((SECONDS-started))s" >&2
    boot_started=$SECONDS
    echo 'Waiting for guest agents and SSH...' >&2
    for ((id=start; id<=end; id++)); do
      if ! wait_ip "$id" >/dev/null; then
        echo "Lab VM $id did not become reachable; inspect it and reset" >&2; exit 1
      fi
    done
    echo "Guest readiness: $((SECONDS-boot_started))s; create total: $((SECONDS-started))s" >&2
    "$0" status
    ;;
  status)
    ips=()
    for ((id=start; id<=end; id++)); do require_owned "$id"; ips+=("$(ip_for "$id")"); done
    [[ $(printf '%s\n' "${ips[@]}" | sort -u | wc -l) -eq ${#ips[@]} ]] || { echo 'Duplicate guest IPs detected; check DHCP and reset the lab' >&2; exit 1; }
    control_ips=$(IFS=,; echo "${ips[*]:0:controllers}")
    worker_ips=$(IFS=,; echo "${ips[*]:controllers}")
    jq -n --arg control "$control_ips" --arg workers "$worker_ips" '{controller_ips:$control,worker_ips:$workers}'
    ;;
  prerequisites|provision)
    started=$SECONDS
    check_playbooks
    state=$("$0" status)
    control_ips=$(jq -r .controller_ips <<<"$state")
    worker_ips=$(jq -r .worker_ips <<<"$state")
    [[ -f $known_hosts && ! -L $known_hosts && $(head -n 1 "$known_hosts") == '# Painkiller managed host keys' ]] || { echo 'Missing or unmanaged lab host-key file; reset the lab' >&2; exit 1; }
    # Only marked VMs in the authorized range receive this disposable lab policy.
    for ((id=start; id<=end; id++)); do
      ip=$(ip_for "$id") || { echo "VM $id address unavailable" >&2; exit 1; }
      trust_guest "$id" "$ip" || { echo "Could not verify the host key for VM $id" >&2; exit 1; }
      result=$(remote "qm guest exec $id -- sh -c 'set -e; id $ssh_user >/dev/null; mkdir -p /etc/sudoers.d; printf \"%s ALL=(ALL) NOPASSWD:ALL\\n\" $ssh_user > /etc/sudoers.d/painkiller-shell; chmod 440 /etc/sudoers.d/painkiller-shell; visudo -cf /etc/sudoers.d/painkiller-shell >/dev/null'")
      jq -e '.exited == 1 and .exitcode == 0' <<<"$result" >/dev/null || { echo "Failed to configure lab sudo on VM $id: $result" >&2; exit 1; }
    done
    # Wait for cloud-init and any package installation on first boot to finish;
    # they hold the dpkg lock that the playbook's package step needs.
    for ((id=start; id<=end; id++)); do
      ip=$(ip_for "$id") || { echo "VM $id address unavailable" >&2; exit 1; }
      # shellcheck disable=SC2016
      node_ssh "$ssh_user@$ip" 'set -u
        if command -v cloud-init >/dev/null 2>&1; then sudo -n cloud-init status --wait >/dev/null 2>&1 || true; fi
        for _ in $(seq 1 150); do
          pgrep -x apt-get >/dev/null 2>&1 || pgrep -x dpkg >/dev/null 2>&1 || exit 0
          sleep 2
        done
        echo "Guest package manager still busy" >&2; exit 1' </dev/null || { echo "VM $id did not finish booting" >&2; exit 1; }
    done
    if [[ $mode == prebaked ]]; then
      IFS=, read -r -a node_ips <<<"$control_ips${worker_ips:+,$worker_ips}"
      for ip in "${node_ips[@]}"; do
        node_ssh "$ssh_user@$ip" bash -s -- "$version" < "$(dirname "$0")/validate-image.sh" || { echo "Incompatible image or missing cached images on $ip" >&2; exit 1; }
      done
      first=${control_ips%%,*}
      node_ssh "$ssh_user@$first" bash -s -- "$first" <<'HEALTH' || exit 1
set -eu
if sudo -n test -e /etc/kubernetes/admin.conf; then
  sudo -n grep -Fxq -- "    - --advertise-address=$1" /etc/kubernetes/manifests/kube-apiserver.yaml || { echo 'Control-plane address changed; reset the lab' >&2; exit 1; }
  sudo -n kubectl --kubeconfig=/etc/kubernetes/admin.conf --request-timeout=15s get --raw=/readyz >/dev/null || { echo 'Existing control plane unhealthy; inspect and reset the lab' >&2; exit 1; }
fi
HEALTH
    fi
    mkdir -p "$PWD/data/kubeconfigs"
    kubeconfig_dir=$(mktemp -d "$PWD/data/kubeconfigs/exam.XXXXXXXX")
    kubeconfig_vars=$(jq -nc --arg dir "$kubeconfig_dir" '{local_kubeconfig_dir:$dir}')
    playbook=$playbooks/kubeadm-cluster/kubeadm-cluster.yml
    if [[ $mode == prebaked ]]; then
      # Keep inventory and bootstrap plays from the pinned playbook, but never
      # run package preparation, automatic reset, or network image pulls.
      awk '
        /^- name: Prepare all kubeadm nodes$/ {prep=1; next}
        /^- name: Bootstrap first kubeadm control-plane node$/ {prep=0; bootstrap=1}
        prep {next}
        bootstrap && /^    - name: Check if existing etcd manifest references a different IP$/ {skip=1; next}
        skip && /^    - name: Initialize first control-plane node$/ {skip=0; print "    - name: Preserve existing cluster on retry\n      ansible.builtin.set_fact:\n        kubeadm_reset_needed: false\n"}
        skip {next}
        {print}
      ' "$playbook" > "$kubeconfig_dir/bootstrap.yml"
      playbook=$kubeconfig_dir/bootstrap.yml
      if ! grep -Fxq -- '- name: Bootstrap first kubeadm control-plane node' "$playbook" || ! grep -Fq 'kubeadm_reset_needed: false' "$playbook" || grep -Fxq -- '- name: Prepare all kubeadm nodes' "$playbook" || grep -Fq 'kubeadm config images pull' "$playbook"; then echo 'Unexpected playbook layout' >&2; exit 1; fi
    fi
    args=( -i 'localhost,' -c local "$playbook" -e "controller_ips=$control_ips ansible_ssh_user=$ssh_user kubernetes_version=$version cni_plugin=${LAB_CNI_PLUGIN:-flannel} initial_controllers=$initial_controllers preserve_existing=true" -e "$kubeconfig_vars" )
    if [[ $command == prerequisites ]]; then args+=( --tags 'inventory,prerequisites' ); fi
    if [[ -n $worker_ips ]]; then args+=( -e "worker_ips=$worker_ips" ); fi
    if [[ $mode == prebaked ]]; then args+=( -e 'flannel_manifest_url=/opt/painkiller/flannel.yml' ); fi
    if [[ -n ${LAB_CONTROL_PLANE_ENDPOINT:-} ]]; then args+=( -e "control_plane_endpoint=$LAB_CONTROL_PLANE_ENDPOINT" ); fi
    ANSIBLE_FORCE_COLOR=1 ansible-playbook "${args[@]}" --ssh-common-args "-o UserKnownHostsFile=$known_hosts -o GlobalKnownHostsFile=/dev/null -o StrictHostKeyChecking=yes"
    if [[ $command == prerequisites ]]; then exit 0; fi
    first=${control_ips%%,*}
    # Expand the home directory and user IDs on the guest, not on the management host.
    # shellcheck disable=SC2016
    node_ssh "$ssh_user@$first" 'set -eu; mkdir -p "$HOME/.kube"; chmod 700 "$HOME/.kube"; sudo -n install -m 600 -o "$(id -u)" -g "$(id -g)" /etc/kubernetes/admin.conf "$HOME/.kube/config"'
    if [[ $mode == legacy ]]; then
      node_ssh "$ssh_user@$first" 'if ! command -v jq >/dev/null || ! command -v etcdctl >/dev/null; then sudo -n apt-get update && sudo -n apt-get install -y jq etcd-client; fi'
    fi
    node_count=$(node_ssh "$ssh_user@$first" 'kubectl get nodes --no-headers' | wc -l)
    expected=$((end-start+1-controllers+initial_controllers))
    [[ $node_count -eq $expected ]] || { echo "Expected $expected joined nodes, got $node_count" >&2; exit 1; }
    node_ssh "$ssh_user@$first" "kubectl wait --for=condition=Ready nodes --all --timeout=${timeout}s"
    echo "Provision ($mode): $((SECONDS-started))s" >&2
    ;;
  start|stop|destroy)
    # Check all VMs before acting, so a mixed range is never partially operated on.
    list_vms
    for ((id=start; id<=end; id++)); do if exists "$id"; then require_owned "$id"; fi; done
    for ((id=start; id<=end; id++)); do
      if exists "$id"; then
        case "$command" in
          start) remote "qm start $id" ;;
          stop) remote "qm stop $id" ;;
          destroy) if remote "qm status $id" | grep -q 'status: running'; then remote "qm stop $id"; fi; remote "qm destroy $id" ;;
        esac
      fi
    done
    ;;
  *) echo "Unknown command: $command" >&2; exit 2 ;;
esac
