#!/usr/bin/env bash
# Exact candidate-visible answers, as the candidate on the disposable controller.
set -Eeuo pipefail
kubectl get namespace cks001b-system -o json |
  jq -e '.metadata.labels["painkiller-shell/exam"] == "CKS-001B"' >/dev/null
for question in $(seq 1 16); do
  python3 -B /opt/painkiller/cks001b/answer.py "$question"
done
