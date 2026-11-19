#!/usr/bin/env bash
# Read-only, bounded verification through saved lab ownership and candidate SSH.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",") | select(length == 1) | .[0] | select(length > 0)' <<<"$state")
jq -e '.worker_ips == ""' <<<"$state" >/dev/null
exec ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes \
  -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}" \
  "${LAB_SSH_USER:-milkman}@$cp" \
  'sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf timeout 50s python3 -B /opt/painkiller/cks001b/ready.py'
