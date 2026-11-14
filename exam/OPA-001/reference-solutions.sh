#!/usr/bin/env bash
# Run on the disposable student controller only.
set -Eeuo pipefail
kubectl --request-timeout=5s get namespace opa001-system -o json |
  jq -e '.metadata.labels["painkiller-shell/exam"] == "OPA-001"' >/dev/null ||
  { echo 'Refusing reference solutions on an unmarked cluster' >&2; exit 1; }
python3 -B - <<'PY'
import json
import subprocess
import time

tasks = json.load(open("/opt/painkiller/opa001/tasks.json"))
for task in tasks:
    subprocess.run(["bash", "-e", "-o", "pipefail", "-c", task["solution"]], check=True, timeout=120)
for task in tasks:
    deadline = time.monotonic() + 45
    while True:
        result = subprocess.run(["bash", "-o", "pipefail", "-c", task["resource"]], capture_output=True, text=True, timeout=30)
        if result.returncode:
            raise SystemExit(task["question_id"] + ": " + result.stderr)
        capture = json.loads(result.stdout)
        if capture["ok"]:
            break
        if time.monotonic() >= deadline:
            raise SystemExit(task["question_id"] + ": " + str(capture["failures"]))
        time.sleep(1)
    print(task["question_id"] + " reference verified", flush=True)
PY
