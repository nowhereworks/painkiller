#!/usr/bin/env bash
# Exam-owned preparation: one API proxy, cached dependencies, fixtures, faults.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
host=${LAB_PROXMOX_HOST:-root@prox-k12}
user=${LAB_SSH_USER:-milkman}
known=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}
started=$SECONDS
node() {
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes \
    -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" "$user@$1" "${@:2}"
}
case "${1:-}" in
preflight)
  # Only one LoadBalancer Service is needed. No API VIP is allocated.
  if ! "$root/scripts/create-lab.sh" status >/dev/null 2>&1; then
    scan=$(ssh -o BatchMode=yes -o ConnectTimeout=10 "$host" \
      'nmap -sn -PR 192.168.254.81 -n')
    if grep -q 'Host is up' <<<"$scan"; then
      echo 'LoadBalancer address 192.168.254.81 is in use' >&2; exit 1
    fi
  fi
  ;;
infrastructure)
  bash "$root/exam/CKA-002/configure-api-proxy.sh"
  ;;
fixtures)
  state=$("$root/scripts/create-lab.sh" status)
  cp=$(jq -r '.controller_ips | split(",")[0]' <<<"$state")
  cp2=$(jq -r '.controller_ips | split(",")[1]' <<<"$state")
  cp3=$(jq -r '.controller_ips | split(",")[2]' <<<"$state")
  w1=$(jq -r '.worker_ips | split(",")[0]' <<<"$state")
  w2=$(jq -r '.worker_ips | split(",")[1]' <<<"$state")
  marker=/var/lib/painkiller/cka002
  kube() { node "$cp" "kubectl $*"; }
  marked() { node "$cp" "sudo -n test -e $marker/$1"; }
  mark() { node "$cp" "sudo -n mkdir -p $marker && sudo -n touch $marker/$1"; }
  apply() { node "$cp" 'kubectl apply --server-side -f -' < "$root/exam/CKA-002/$1"; }
  if marked complete; then
    echo 'CKA-002 already prepared; preserving candidate work'
    exit 0
  fi
  if ! marked fixtures; then
    nodes=$(kube get nodes -o json)
    w1_name=$(jq -r --arg ip "$w1" '.items[] | select(any(.status.addresses[]; .address == $ip)) | .metadata.name' <<<"$nodes")
    w2_name=$(jq -r --arg ip "$w2" '.items[] | select(any(.status.addresses[]; .address == $ip)) | .metadata.name' <<<"$nodes")
    kube label node "$w1_name" topology.kubernetes.io/zone=west --overwrite
    kube label node "$w2_name" topology.kubernetes.io/zone=east --overwrite
    kube taint node "$w2_name" exam=reserved:NoSchedule --overwrite

    echo 'CKA-002: installing dependencies'
    # Apply independent controllers before waiting for any one of them.
    for file in metrics-server local-path-provisioner metallb-native gateway-api traefik; do
      apply "assets/$file.yaml"
    done
    # Keep supporting controllers away from the deliberately broken worker.
    for target in 'kube-system metrics-server' 'kube-system coredns' \
      'local-path-storage local-path-provisioner' 'metallb-system controller' 'traefik traefik'; do
      read -r ns dep <<<"$target"
      kube patch deployment "$dep" -n "$ns" --type=merge \
        "-p '{\"spec\":{\"template\":{\"spec\":{\"nodeSelector\":{\"topology.kubernetes.io/zone\":\"west\"}}}}}'"
    done
    kube rollout status deployment/controller -n metallb-system --timeout=120s
    # Readiness may precede the webhook listener; retry its configuration briefly.
    configured=0
    for _ in $(seq 1 20); do
      if apply assets/metallb-config.yaml; then configured=1; break; fi
      sleep 2
    done
    [[ $configured == 1 ]] || { echo 'MetalLB webhook unavailable' >&2; exit 1; }
    for target in 'kube-system metrics-server' 'local-path-storage local-path-provisioner' 'traefik traefik'; do
      read -r ns dep <<<"$target"
      kube rollout status deployment/"$dep" -n "$ns" --timeout=120s
    done
    node "$cp" 'command -v helm >/dev/null
      mkdir -p /opt/painkiller/cka002'
    scp -r -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes \
      -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" \
      "$root/exam/CKA-002/assets/"{chart,kustomize,operator} \
      "$user@$cp:/opt/painkiller/cka002/"
    apply setup.yaml
    node "$w1" 'sudo -n mkdir -p /opt/painkiller/local-data
      echo painkiller-data | sudo -n tee /opt/painkiller/local-data/data.txt >/dev/null'
    node "$cp" 'helm upgrade --install demo /opt/painkiller/cka002/chart -n exam-helm --set image.tag=9.9.9'
    # Establish a real good revision before introducing the failed rollout.
    kube rollout status deployment/app -n exam-workloads --timeout=120s
    kube set image deployment/app -n exam-workloads app=nginx:9.9.9
    mark fixtures
  fi

  echo 'CKA-002: injecting faults once'
  if ! marked infrastructure; then
    node "$cp3" 'echo net.ipv4.ip_forward=0 | sudo -n tee /etc/sysctl.d/99-cka002.conf >/dev/null
      sudo -n sysctl -p /etc/sysctl.d/99-cka002.conf'
    mark infrastructure
  fi
  if ! marked kubelet; then
    node "$w2" 'if ! grep -q painkiller-fault /var/lib/kubelet/config.yaml; then
        echo "  painkiller-fault: [unclosed" | sudo -n tee -a /var/lib/kubelet/config.yaml >/dev/null
      fi
      sudo -n systemctl restart kubelet || true'
    mark kubelet
  fi
  if ! marked scheduler; then
    node "$cp2" 'sudo -n sed -i "s#kube-scheduler:v[0-9.]*#kube-scheduler:v9.9.9#" /etc/kubernetes/manifests/kube-scheduler.yaml'
    mark scheduler
  fi
  if ! marked dns; then
    kube get cm coredns -n kube-system -o json \
      | jq '.data.Corefile |= (split("forward . /etc/resolv.conf") | join("forward . 1.2.3.4"))' \
      | node "$cp" 'kubectl apply -f -'
    kube rollout restart deployment/coredns -n kube-system
    mark dns
  fi
  mark complete
  ;;
*) echo "Unknown phase: ${1:-}" >&2; exit 2 ;;
esac
echo "CKA-002 ${1}: $((SECONDS-started))s"
