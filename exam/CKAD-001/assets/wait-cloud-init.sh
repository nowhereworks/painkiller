#!/usr/bin/env bash
# Read-only prerequisite: SSH can become available before cloud-init's apt work.
set -Eeuo pipefail
if command -v cloud-init >/dev/null; then
  rc=0
  timeout 600 sudo -n cloud-init status --wait || rc=$?
  [[ $rc == 0 || $rc == 2 ]] || exit "$rc"
  status=$(sudo -n cloud-init status --format json) || [[ $? == 2 ]]
  printf '%s' "$status" | python3 -c 'import json,sys
s=json.load(sys.stdin)
sys.exit(0 if s["status"] == "done" and not s.get("errors") else 1)'
fi
