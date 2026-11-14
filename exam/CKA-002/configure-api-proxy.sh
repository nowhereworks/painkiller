#!/usr/bin/env bash
# One TCP proxy, addressed by a lab-local hostname on every owned guest.
# No VIP, VRRP, external DNS record, or changes to clone NIC/DHCP settings.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
user=${LAB_SSH_USER:-milkman}
known=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}
[[ ${LAB_CONTROL_PLANE_ENDPOINT:-} == cka002-api:8443 ]] || {
  echo 'CKA-002 requires endpoint cka002-api:8443' >&2; exit 1;
}
state=$("$root/scripts/create-lab.sh" status)
controllers=$(jq -r .controller_ips <<<"$state")
workers=$(jq -r .worker_ips <<<"$state")
proxy=${workers%%,*}
node() {
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes \
    -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" "$user@$1" "${@:2}"
}
IFS=, read -r -a ips <<<"$controllers,$workers"
for ip in "${ips[@]}"; do
  node "$ip" "sudo -n sed -i '/# cka002-api$/d' /etc/hosts
    echo '$proxy cka002-api # cka002-api' | sudo -n tee -a /etc/hosts >/dev/null"
done
{
  cat <<'CONFIG'
global
  user haproxy
  group haproxy
  daemon
defaults
  mode tcp
  timeout connect 3s
  timeout client 1h
  timeout server 1h
frontend kube_api
  bind *:8443
  default_backend controllers
backend controllers
  balance roundrobin
  option tcp-check
CONFIG
  IFS=, read -r -a ips <<<"$controllers"
  for i in "${!ips[@]}"; do
    echo "  server cp$i ${ips[$i]}:6443 check inter 2s fall 2 rise 2"
  done
} | node "$proxy" 'set -e
  sudo -n tee /etc/haproxy/haproxy.cfg >/dev/null
  sudo -n haproxy -c -f /etc/haproxy/haproxy.cfg
  sudo -n systemctl enable haproxy
  sudo -n systemctl restart haproxy'
