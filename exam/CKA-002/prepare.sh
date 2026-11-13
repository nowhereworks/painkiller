#!/usr/bin/env bash
# CKA-002 staged preparation. Invoked by the server with a single phase:
#   preflight        read-only allocation checks, before any VM is cloned
#   infrastructure   pre-bootstrap guest services (redundant API endpoint)
#   fixtures         dependencies, fixtures, readiness, then intentional faults
# Each phase must be idempotent so a retry reconciles without resetting a
# healthy cluster or re-injecting faults. Streams progress to the operation log.
set -Eeuo pipefail

phase=${1:-}
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
host=${LAB_PROXMOX_HOST:-root@prox-k12}
user=${LAB_SSH_USER:-milkman}
known=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}

remote() { ssh -o BatchMode=yes -o ConnectTimeout=10 "$host" "$@"; }
node_ssh() {
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes \
    -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" "$@"
}
state_json() { "$root/scripts/create-lab.sh" status; }
first_controller() { state_json | jq -r .controller_ips | cut -d, -f1; }
kube() { node_ssh "$user@$(first_controller)" "kubectl $*"; }
apply_file() { node_ssh "$user@$(first_controller)" "kubectl apply -f -" < "$1"; }

case "$phase" in
preflight)
  echo "CKA-002 preflight: reserved-address conflict check"
  # The API VIP and LoadBalancer pool (.80-.89) must be free before allocation.
  # Once the lab exists it legitimately owns those addresses, so only scan when
  # no owned VMs are present yet (a fresh allocation, not a retry).
  if "$root/scripts/create-lab.sh" status >/dev/null 2>&1; then
    echo "Preflight: lab already exists; skipping reserved-address scan"
  else
    if ! remote 'command -v nmap >/dev/null' 2>/dev/null; then
      echo 'Preflight: nmap is required on the Proxmox host' >&2
      exit 1
    fi
    # shellcheck disable=SC2016  # $5 expands in awk on the Proxmox host, not locally
    conflicts=$(remote 'nmap -sn 192.168.254.80-89 -n 2>/dev/null | awk "/Nmap scan report/{print \$5}"')
    if [[ -n $conflicts ]]; then
      echo "Preflight: reserved addresses already in use: $(echo "$conflicts" | tr '\n' ' ')" >&2
      exit 1
    fi
    echo "Preflight: reserved range 192.168.254.80-89 is free"
  fi
  ;;

infrastructure)
  echo "CKA-002 infrastructure: waiting for guests to finish first boot"
  # cloud-init's first boot can trigger a reboot, so retry the connection and
  # wait for both cloud-init and the package manager to settle.
  state=$(state_json)
  w1_ip=$(jq -r .worker_ips <<<"$state" | cut -d, -f1)
  w2_ip=$(jq -r .worker_ips <<<"$state" | cut -d, -f2)
  for ip in "$w1_ip" "$w2_ip"; do
    deadline=$((SECONDS + 600))
    ready=0
    while (( SECONDS < deadline )); do
      # shellcheck disable=SC2016  # expands on the guest, not locally
      if node_ssh "$user@$ip" 'set -u
        if command -v cloud-init >/dev/null 2>&1; then sudo -n cloud-init status --wait >/dev/null 2>&1 || true; fi
        for _ in $(seq 1 30); do
          pgrep -x apt-get >/dev/null 2>&1 || pgrep -x dpkg >/dev/null 2>&1 || exit 0
          sleep 2
        done
        exit 1' </dev/null >/dev/null 2>&1; then
        ready=1
        break
      fi
      sleep 3
    done
    [[ $ready -eq 1 ]] || { echo "Worker $ip did not finish booting" >&2; exit 1; }
  done
  echo "CKA-002 infrastructure: redundant API endpoint (before bootstrap)"
  # Idempotent: a retry must not reconfigure an already-assigned VIP (the route
  # to .80 changes once keepalived owns it, breaking a second discovery run).
  if node_ssh "$user@$w1_ip" 'systemctl is-active --quiet keepalived' 2>/dev/null; then
    echo "API balancer already configured; skipping"
  else
    "$root/scripts/configure-api-load-balancer.sh"
  fi
  ;;

fixtures)
  echo "CKA-002 fixtures: dependencies"
  apply_file "$root/exam/CKA-002/assets/metrics-server.yaml"
  apply_file "$root/exam/CKA-002/assets/local-path-provisioner.yaml"
  apply_file "$root/exam/CKA-002/assets/metallb-native.yaml"
  # MetalLB's validating webhook must be serving before its pool/CRD config.
  kube wait --for=condition=available deployment/controller -n metallb-system --timeout=300s
  apply_file "$root/exam/CKA-002/assets/metallb-config.yaml"
  # Envoy Gateway bundles CRDs whose schemas exceed kubectl's 256 KiB
  # last-applied annotation limit; server-side apply avoids that annotation.
  node_ssh "$user@$(first_controller)" "kubectl apply --server-side -f -" \
    < "$root/exam/CKA-002/assets/envoy-gateway-install.yaml"
  apply_file "$root/exam/CKA-002/assets/traefik.yaml"

  echo "CKA-002 fixtures: helm tooling and supplied assets"
  kube wait --for=condition=available deployment/metrics-server -n kube-system --timeout=300s
  # shellcheck disable=SC2016  # $(id -u) expands on the controller, not locally
  node_ssh "$user@$(first_controller)" 'set -e
    if ! command -v helm >/dev/null 2>&1; then
      curl -fsSL https://get.helm.sh/helm-v3.19.0-linux-amd64.tar.gz \
        | sudo -n tar -xz -C /usr/local/bin --strip-components=1 linux-amd64/helm
    fi
    sudo -n mkdir -p /opt/painkiller/cka002
    sudo -n chown -R "$(id -u):$(id -g)" /opt/painkiller/cka002'
  tar -C "$root/exam/CKA-002/assets" -cf - chart kustomize operator \
    | node_ssh "$user@$(first_controller)" 'tar -xf - -C /opt/painkiller/cka002'

  echo "CKA-002 fixtures: apply test objects (setup.yaml)"
  node_ssh "$user@$(first_controller)" "kubectl apply -f -" < "$root/exam/CKA-002/setup.yaml"

  echo "CKA-002 fixtures: node labels, taint, and local data"
  state=$(state_json)
  w1_ip=$(jq -r .worker_ips <<<"$state" | cut -d, -f1)
  w2_ip=$(jq -r .worker_ips <<<"$state" | cut -d, -f2)
  nodes=$(node_ssh "$user@$(first_controller)" "kubectl get nodes -o json")
  w1_name=$(jq -r '[.items[] | select(.metadata.labels["node-role.kubernetes.io/control-plane"] == null)][0].metadata.name' <<<"$nodes")
  w2_name=$(jq -r '[.items[] | select(.metadata.labels["node-role.kubernetes.io/control-plane"] == null)][1].metadata.name' <<<"$nodes")
  kube label node "$w1_name" topology.kubernetes.io/zone=west --overwrite >/dev/null
  kube label node "$w2_name" topology.kubernetes.io/zone=east --overwrite >/dev/null
  kube taint node "$w2_name" exam=reserved:NoSchedule --overwrite >/dev/null
  node_ssh "$user@$w1_ip" 'sudo -n mkdir -p /opt/painkiller/local-data && echo painkiller-data | sudo -n tee /opt/painkiller/local-data/data.txt >/dev/null'

  echo "CKA-002 fixtures: install the broken Helm release"
  node_ssh "$user@$(first_controller)" \
    "helm upgrade --install demo /opt/painkiller/cka002/chart -n exam-helm --create-namespace --set image.tag=9.9.9"

  echo "CKA-002 fixtures: dependency readiness"
  kube wait --for=condition=available deployment/controller -n metallb-system --timeout=300s || true
  kube rollout status deployment/traefik -n traefik --timeout=300s
  kube rollout status deployment/local-path-provisioner -n local-path-storage --timeout=300s || true
  kube rollout status deployment/envoy-gateway -n envoy-gateway-system --timeout=300s || true
  kube wait --for=condition=Ready pod -l app.kubernetes.io/name=traefik -n traefik --timeout=300s || true

  echo "CKA-002 fixtures: intentional faults (idempotent)"
  # Worker kubelet fault (task 15).
  node_ssh "$user@$w2_ip" 'if ! grep -q painkiller-fault /var/lib/kubelet/config.yaml 2>/dev/null; then
      echo "  painkiller-fault: [unclosed" | sudo -n tee -a /var/lib/kubelet/config.yaml >/dev/null
      sudo -n systemctl restart kubelet || true
    fi'
  # Scheduler static-Pod fault on a non-primary controller (task 16).
  c2_ip=$(jq -r .controller_ips <<<"$state" | cut -d, -f2)
  node_ssh "$user@$c2_ip" 'if ! grep -q "kube-scheduler:v9.9.9" /etc/kubernetes/manifests/kube-scheduler.yaml 2>/dev/null; then
      sudo -n sed -i "s#kube-scheduler:v[0-9.]*#kube-scheduler:v9.9.9#" /etc/kubernetes/manifests/kube-scheduler.yaml
    fi'
  # CoreDNS forward fault (task 17).
  if ! kube get cm coredns -n kube-system -o json | jq -e '.data.Corefile | contains("1.2.3.4")' >/dev/null 2>&1; then
    kube get cm coredns -n kube-system -o json \
      | jq '.data.Corefile = (.data.Corefile | split("forward . /etc/resolv.conf") | join("forward . 1.2.3.4"))' \
      | node_ssh "$user@$(first_controller)" 'kubectl apply -f -' >/dev/null
    kube rollout restart deployment/coredns -n kube-system >/dev/null || true
  fi

  echo "CKA-002 fixtures: complete"
  ;;

*)
  echo "unknown phase: $phase" >&2
  exit 2
  ;;
esac
