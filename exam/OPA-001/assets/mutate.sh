#!/usr/bin/env bash
# Trusted score-loss fixtures, only on a marked disposable OPA-001 controller.
set -Eeuo pipefail
[[ $# == 2 && $1 =~ ^(remove|restore)$ && $2 =~ ^q([1-9]|1[0-6])$ ]] ||
  { echo 'Usage: mutate.sh {remove|restore} q1..q16' >&2; exit 2; }
kubectl --request-timeout=5s get namespace opa001-system -o json |
  jq -e '.metadata.labels["painkiller-shell/exam"] == "OPA-001"' >/dev/null
action=$1 q=$2
if [[ $action == restore ]]; then
  python3 -B - "$q" <<'PY'
import json
import subprocess
import sys
import time

task = json.load(open("/opt/painkiller/opa001/tasks.json"))[int(sys.argv[1][1:]) - 1]
subprocess.run(["bash", "-e", "-o", "pipefail", "-c", task["solution"]], check=True, timeout=120)
deadline = time.monotonic() + 45
while True:
    result = subprocess.run(["bash", "-o", "pipefail", "-c", task["resource"]], capture_output=True, text=True, timeout=30)
    if result.returncode:
        raise SystemExit(result.stderr)
    if json.loads(result.stdout)["ok"]:
        break
    if time.monotonic() > deadline:
        raise SystemExit(result.stdout)
    time.sleep(1)
PY
  exit 0
fi
case "$q" in
q1|q2|q3|q5|q8|q16)
  printf 'package %s\n\nallow := true\n' "$q" > "/opt/opa-workshop/$q/policy.rego" ;;
q4) printf 'package q4\n\nviolations := []\n' > /opt/opa-workshop/q4/policy.rego ;;
q6) printf 'package tenant_test\n\ntest_placeholder if { true }\n' > /opt/opa-workshop/q6/tests.rego ;;
q7) printf 'import json\nprint(json.dumps({"allow":True,"reason":"allow"}))\n' > /opt/opa-workshop/q7/adapter.py ;;
q9)
  printf '%s\n' '{"revision":"wrong","roots":["release"],"rego_version":1}' > /opt/opa-workshop/q9/src/.manifest
  opa build --bundle /opt/opa-workshop/q9/src --output /opt/opa-workshop/q9/bundle.tar.gz ;;
q10)
  sed -i 's/bundles\/stable.tar.gz/bundles\/broken.tar.gz/' /opt/opa-workshop/q10/config.yaml
  sudo -n systemctl restart opa001-q10 ;;
q11)
  printf 'package system.log\n\nmask := []\n' > /opt/opa-workshop/q11/mask.rego
  sudo -n systemctl restart opa001-q11 ;;
q12) kubectl delete opalabels require-team-owner ;;
q13) kubectl patch opaauditlabels audit-env --type=merge -p '{"spec":{"match":{"excludedNamespaces":["opa-audit"]}}}' ;;
q14) kubectl patch config.config.gatekeeper.sh config -n gatekeeper-system --type=merge -p '{"spec":{"sync":{"syncOnly":[]}}}' ;;
q15) printf 'package main\n\ndeny := []\n' > /opt/opa-workshop/q15/policy/main.rego ;;
esac
