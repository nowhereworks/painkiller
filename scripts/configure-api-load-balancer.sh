#!/usr/bin/env bash
# Only discover and configure guests already checked for lab ownership.
set -Eeuo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
# shellcheck source=scripts/playbooks.sh
source "$root/scripts/playbooks.sh"
check_playbooks
[[ ${LAB_CONTROLLERS:-1} == 3 && ${LAB_INITIAL_CONTROLLERS:-3} == 2 &&
   ${LAB_PROVISION_MODE:-prebaked} == legacy &&
   ${LAB_CONTROL_PLANE_ENDPOINT:-} == 192.168.254.80:8443 ]] || {
  echo 'API balancing requires the explicit three-controller legacy HA lab configuration' >&2
  exit 1
}
state=$("$root/scripts/create-lab.sh" status)
vars=$(jq -ce --arg user "${LAB_SSH_USER:-milkman}" '
  select((.controller_ips | split(",") | length) == 3 and (.worker_ips | split(",") | length) == 2) |
  {controller_ips,worker_ips,ssh_user:$user,api_vip:"192.168.254.80"}
' <<<"$state") || { echo 'Expected exactly three controllers and two workers' >&2; exit 1; }
known_hosts=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}
[[ $known_hosts != *[[:space:]]* && $known_hosts != *"'"* ]] || { echo 'Unsafe known-hosts path' >&2; exit 1; }
ANSIBLE_FORCE_COLOR=1 ansible-playbook -i 'localhost,' -c local "$playbooks/api-load-balancer.yml" \
  -e "$vars" --ssh-common-args "-o UserKnownHostsFile=$known_hosts -o GlobalKnownHostsFile=/dev/null -o StrictHostKeyChecking=yes"
