#!/usr/bin/env bash
# Run only on an explicitly marked disposable CKAD-001 lab as the student.
set -Eeuo pipefail
kubectl --request-timeout=5s get namespace ckad001-system -o json |
  jq -e '.metadata.labels["painkiller-shell/exam"] == "CKAD-001"' >/dev/null ||
  { echo 'Refusing to solve an unmarked cluster' >&2; exit 1; }
for q in {1..21}; do
  echo "Solving CKAD-001-Q$q"
  jq -er ".[$((q-1))].solution" /opt/painkiller/ckad001/tasks.json | bash -e -o pipefail
  ok=false
  for _ in {1..60}; do
    if bash /opt/painkiller/ckad001/capture.sh "q$q" | jq -e '.ok == true' >/dev/null; then
      ok=true
      break
    fi
    sleep 2
  done
  [[ $ok == true ]] || { echo "Reference answer Q$q did not converge" >&2; exit 1; }
done
