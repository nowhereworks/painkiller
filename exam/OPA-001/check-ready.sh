#!/usr/bin/env bash
# Read-only starting-state validation, including intentional bundle/matching faults.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",")[0]' <<<"$state")
ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=yes \
  -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}" \
  "${LAB_SSH_USER:-milkman}@$cp" 'python3 -B /opt/painkiller/opa001/ready.py'
