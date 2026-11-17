#!/usr/bin/env bash
# Read-only, bounded validation of the initial state, never a repair.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",")[0]' <<<"$state")
ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=yes \
  -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}" \
  "${LAB_SSH_USER:-milkman}@$cp" \
  'sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf timeout 50s python3 -B /opt/painkiller/cks001/ready.py'
