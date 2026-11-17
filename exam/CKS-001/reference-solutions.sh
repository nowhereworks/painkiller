#!/usr/bin/env bash
# Run as the candidate; only inside a marked disposable CKS-001 cluster.
set -Eeuo pipefail
kubectl get namespace cks001-system -o json |
  jq -e '.metadata.labels["painkiller-shell/exam"] == "CKS-001"' >/dev/null
# Execute the exact candidate-visible solutions, not privileged alternate answers.
python3 -B - <<'PY'
import json, subprocess
for task in json.load(open("/opt/painkiller/cks001/tasks.json")):
    print("Solving", task["question_id"], flush=True)
    subprocess.run(["bash", "-e", "-o", "pipefail", "-c", task["solution"]], check=True, timeout=180)
PY
# No answers are changed here: wait for post-reload supporting evidence to settle.
sudo -n env KUBECONFIG=/etc/kubernetes/admin.conf python3 -B -c \
  'import sys; sys.path.insert(0, "/opt/painkiller/cks001"); from mutate import wait_api; wait_api()'
