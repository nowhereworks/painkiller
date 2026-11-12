#!/usr/bin/env bash
# Manage only VMs explicitly marked as belonging to this lab.
set -Eeuo pipefail
command=${1:-help}
host=${LAB_PROXMOX_HOST:-root@prox-k12}
template=${LAB_TEMPLATE_ID:-9010}
start=${LAB_VM_START:-3000}
end=${LAB_VM_END:-3004}
controllers=${LAB_CONTROLLERS:-3}
marker=${LAB_MARKER:-painkiller-mvp}
prefix=${LAB_IP_PREFIX:-192.168.254.}
timeout=${LAB_READY_TIMEOUT:-600}
ssh_user=${LAB_SSH_USER:-milkman}
playbooks=${LAB_PLAYBOOKS_DIR:-$HOME/repo/_nowhereworks/lab-ansible-playbooks}
revision=${LAB_PLAYBOOKS_REV:-8ec676e93017ce3e110c27485e25d1f32d541056}
memory=${LAB_VM_MEMORY:-3072}

[[ $start =~ ^[0-9]+$ && $end =~ ^[0-9]+$ && $template =~ ^[0-9]+$ && $controllers =~ ^[0-9]+$ && $timeout =~ ^[0-9]+$ && $memory =~ ^[0-9]+$ ]] || { echo 'Invalid numeric configuration' >&2; exit 1; }
[[ $start -le $end && $controllers -gt 0 && $controllers -le $((end-start+1)) && $timeout -gt 0 ]] || { echo 'Invalid VM range or controller count' >&2; exit 1; }
[[ $marker =~ ^[a-zA-Z0-9_-]+$ && $prefix =~ ^[0-9.]+$ && $ssh_user =~ ^[a-zA-Z0-9_-]+$ ]] || { echo 'Invalid marker, IP prefix, or SSH user' >&2; exit 1; }
remote() { ssh -o BatchMode=yes -o ConnectTimeout=10 "$host" "$@"; }
list_vms() { vm_list=$(remote 'qm list') || { echo 'Could not list VMs; refusing operation' >&2; exit 1; }; }
exists() { awk -v id="$1" '$1 == id { found=1 } END { exit !found }' <<<"$vm_list"; }
owned() { remote "qm config $1" | grep -Fxq "description: $marker"; }
require_owned() { owned "$1" || { echo "VM $1 is not marked $marker; refusing operation" >&2; exit 1; }; }
check_playbooks() {
  [[ $(git -C "$playbooks" rev-parse HEAD) == "$revision" ]] || { echo "Playbooks revision differs from pinned $revision" >&2; return 1; }
  git -C "$playbooks" diff --quiet HEAD -- kubeadm-cluster || { echo 'Playbook has local modifications' >&2; return 1; }
}
ip_for() {
  local json ips
  json=$(remote "qm agent $1 network-get-interfaces") || return 1
  ips=$(jq -r --arg prefix "$prefix" '[.[] | ."ip-addresses"[]? | select(."ip-address-type" == "ipv4") | ."ip-address" | select(startswith($prefix))] | unique | .[]' <<<"$json") || return 1
  [[ $(grep -c . <<<"$ips") == 1 ]] || return 1
  printf '%s\n' "$ips"
}
wait_ip() {
  local id=$1 deadline=$((SECONDS+timeout)) ip
  while (( SECONDS < deadline )); do
    if ip=$(ip_for "$id" 2>/dev/null) && ssh -o BatchMode=yes -o ConnectTimeout=5 "$ssh_user@$ip" true </dev/null >/dev/null 2>&1; then printf '%s\n' "$ip"; return; fi
    sleep 5
  done
  echo "Timed out waiting for guest agent and SSH on VM $id" >&2; return 1
}
case "$command" in
  help) echo "Usage: $0 {create|provision|status|start|stop|destroy}" ;;
  create)
    check_playbooks
    # Preflight every ID before cloning anything; an SSH failure must not mean 'free'.
    list_vms
    for ((id=start; id<=end; id++)); do
      if exists "$id"; then echo "VM $id already exists; refusing to create" >&2; exit 1; fi
    done
    available=$(remote "awk '/MemAvailable:/ {print int(\$2/1024)}' /proc/meminfo")
    [[ $available =~ ^[0-9]+$ ]] && (( available >= (end-start+1)*memory + 4096 )) || { echo 'Insufficient available host memory (including 4 GiB reserve)' >&2; exit 1; }
    for ((id=start; id<=end; id++)); do
      echo "Creating VM $id" >&2
      remote "qm clone $template $id --name k8s-$id --full 0"
      # Mark immediately; if this fails, manual cleanup is required.
      remote "qm set $id --description $marker"
      remote "qm set $id --memory $memory"
      remote "qm start $id"
    done
    echo 'Waiting for guest agents and SSH...' >&2
    for ((id=start; id<=end; id++)); do wait_ip "$id" >/dev/null; done
    "$0" status
    ;;
  status)
    ips=()
    for ((id=start; id<=end; id++)); do require_owned "$id"; ips+=("$(ip_for "$id")"); done
    [[ $(printf '%s\n' "${ips[@]}" | sort -u | wc -l) -eq ${#ips[@]} ]] || { echo 'Duplicate guest IPs detected; check template machine IDs and DHCP' >&2; exit 1; }
    control_ips=$(IFS=,; echo "${ips[*]:0:controllers}")
    worker_ips=$(IFS=,; echo "${ips[*]:controllers}")
    jq -n --arg control "$control_ips" --arg workers "$worker_ips" '{controller_ips:$control,worker_ips:$workers}'
    ;;
  provision)
    check_playbooks
    state=$("$0" status)
    control_ips=$(jq -r .controller_ips <<<"$state")
    worker_ips=$(jq -r .worker_ips <<<"$state")
    args=( -i localhost, -c local "$playbooks/kubeadm-cluster/kubeadm-cluster.yml" -e "controller_ips=$control_ips worker_ips=$worker_ips ansible_ssh_user=$ssh_user kubernetes_version=${LAB_KUBERNETES_VERSION:-1.36.1} cni_plugin=${LAB_CNI_PLUGIN:-flannel}" )
    if [[ -n ${LAB_CONTROL_PLANE_ENDPOINT:-} ]]; then args+=( -e "control_plane_endpoint=$LAB_CONTROL_PLANE_ENDPOINT" ); fi
    ansible-playbook "${args[@]}"
    first=${control_ips%%,*}
    ssh -o BatchMode=yes -o ConnectTimeout=10 "$ssh_user@$first" 'set -eu; mkdir -p "$HOME/.kube"; chmod 700 "$HOME/.kube"; sudo -n install -m 600 -o "$(id -u)" -g "$(id -g)" /etc/kubernetes/admin.conf "$HOME/.kube/config"; if ! command -v jq >/dev/null; then sudo -n apt-get update && sudo -n apt-get install -y jq; fi'
    node_count=$(ssh -o BatchMode=yes -o ConnectTimeout=10 "$ssh_user@$first" 'kubectl get nodes --no-headers' | wc -l)
    [[ $node_count -eq $((end-start+1)) ]] || { echo "Expected $((end-start+1)) nodes, got $node_count" >&2; exit 1; }
    ssh -o BatchMode=yes -o ConnectTimeout=10 "$ssh_user@$first" "kubectl wait --for=condition=Ready nodes --all --timeout=${timeout}s"
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
