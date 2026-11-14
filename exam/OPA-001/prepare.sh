#!/usr/bin/env bash
# Exam-owned provisioning; never modifies a template or another exam.
# shellcheck disable=SC2016
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
exam=$root/exam/OPA-001
known=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}
user=${LAB_SSH_USER:-milkman}
node() {
  local command
  printf -v command '%q' "$*"
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes \
    -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" "$user@$cp" \
    "bash -e -o pipefail -c $command"
}
case "${1:-}" in
preflight)
  [[ ${LAB_TEMPLATE_ID:-9199} == 9199 ]] || { echo 'OPA-001 requires base template 9199' >&2; exit 1; }
  (cd "$exam/assets" && sha256sum --check --quiet SHA256SUMS)
  exit 0 ;;
infrastructure|fixtures) ;;
*) echo "Usage: $0 {preflight|infrastructure|fixtures}" >&2; exit 2 ;;
esac
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",") | select(length == 1) | .[0]' <<<"$state")
jq -e '.worker_ips == ""' <<<"$state" >/dev/null
marker=/var/lib/painkiller/opa001
if [[ $1 == infrastructure ]]; then
  if ! node "sudo -n test -e $marker/packages"; then
    node 'bash -s' < "$exam/assets/install.sh"
    node "sudo -n mkdir -p $marker; sudo -n touch $marker/packages"
  fi
  exit 0
fi
if node "sudo -n test -e $marker/complete"; then
  echo 'OPA-001 already prepared; preserving candidate answers'
  exit 0
fi
node 'sudo -n mkdir -p /opt/painkiller/opa001'
tar -C "$exam/assets" -cf - . | node 'sudo -n tar -C /opt/painkiller/opa001 -xf -'
node 'sudo -n tee /opt/painkiller/opa001/tasks.json >/dev/null' < "$exam/tasks.json"
if ! node "sudo -n test -e $marker/files"; then
  node 'sudo -n python3 -B /opt/painkiller/opa001/workshop.py /opt/opa-workshop
    sudo -n chown -R "$(id -un):$(id -gn)" /opt/opa-workshop
    curl --retry 3 --connect-timeout 10 --max-time 120 -fsSL \
      https://raw.githubusercontent.com/open-policy-agent/gatekeeper/v3.23.1/deploy/gatekeeper.yaml |
      sudo -n tee /opt/painkiller/opa001/gatekeeper-upstream.yaml >/dev/null
    echo "3c178abe6de4dcdcd2810c2945468a0d7a15ccf0036e324b73308643e94d6f72  /opt/painkiller/opa001/gatekeeper-upstream.yaml" | sha256sum -c -
    sudo -n python3 -B /opt/painkiller/opa001/configure.py "$(id -un)"'
  node "sudo -n touch $marker/files"
fi
if ! node "sudo -n test -e $marker/cluster"; then
  node 'if kubectl get nodes -o json | jq -e '"'"'any(.items[].spec.taints[]?; .key == "node-role.kubernetes.io/control-plane")'"'"' >/dev/null; then
      kubectl taint nodes --all node-role.kubernetes.io/control-plane-
    fi
    kubectl apply -f /opt/painkiller/opa001/gatekeeper-single.yaml
    kubectl -n gatekeeper-system rollout status deployment/gatekeeper-controller-manager --timeout=300s
    kubectl -n gatekeeper-system rollout status deployment/gatekeeper-audit --timeout=300s'
  node "sudo -n touch $marker/cluster"
fi
if ! node "sudo -n test -e $marker/fixtures"; then
  node 'kubectl apply -f -' < "$exam/setup.yaml"
  node 'kubectl apply -f /opt/painkiller/opa001/gatekeeper-fixtures.yaml
    kubectl wait --for=create crd/opaauditlabels.constraints.gatekeeper.sh --timeout=90s
    kubectl wait --for=create crd/opanamespaceteam.constraints.gatekeeper.sh --timeout=90s
    kubectl wait --for=condition=Established crd/opaauditlabels.constraints.gatekeeper.sh --timeout=90s
    kubectl wait --for=condition=Established crd/opanamespaceteam.constraints.gatekeeper.sh --timeout=90s
    kubectl apply -f /opt/painkiller/opa001/gatekeeper-constraints.yaml'
  node "sudo -n touch $marker/fixtures"
fi
bash "$exam/check-ready.sh"
node "sudo -n touch $marker/complete"
