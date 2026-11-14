#!/usr/bin/env bash
# Fast, read-only initial-state validation using the saved attempt environment.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",")[0]' <<<"$state")
ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=yes \
  -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}" \
  "${LAB_SSH_USER:-milkman}@$cp" bash -s <<'CHECK'
set -Eeuo pipefail
k() { kubectl --request-timeout=5s "$@"; }
check() {
  k get nodes -o json | jq -e '.items | length == 3 and all(.[]; any(.status.conditions[]; .type == "Ready" and .status == "True"))' >/dev/null || return 1
  k get namespace cka003-system -o json | jq -e '.metadata.labels["painkiller-shell/exam"] == "CKA-003"' >/dev/null || return 1
  for target in 'kube-system coredns' 'kube-system metrics-server' 'kube-system csi-nfs-controller' \
    'cka003-system routing' 'nginx-gateway web-backend' \
    'default frontend-deployment' 'default nginx-deployment-cka04-svcn'; do
    read -r ns name <<<"$target"
    k get deployment "$name" -n "$ns" -o json | jq -e '.status.availableReplicas >= 1' >/dev/null || return 1
  done
  k get --raw /apis/metrics.k8s.io/v1beta1/nodes | jq -e '.items | length == 3' >/dev/null || return 1
  k get gateway web-gateway -n nginx-gateway -o json |
    jq -e 'any(.status.conditions[]?; .type == "Programmed" and .status == "True")' >/dev/null || return 1
  k get pvc papaya-pvc-cka09-str -n default -o json |
    jq -e '.status.phase == "Bound" and .spec.resources.requests.storage == "50Mi"' >/dev/null || return 1
  k exec papaya-consumer -n default -- cat /data/ready | grep -qx cka003-storage || return 1
  k get deployment pink-depl-cka14-trb -n pink-troubleshooting -o json |
    jq -e '.spec.replicas == 2 and .status.readyReplicas == 1' >/dev/null || return 1
  k get resourcequota pink-quota -n pink-troubleshooting -o json |
    jq -e '.spec.hard.pods == "1"' >/dev/null || return 1
  k get pod grey-cka21-trb -n default -o json |
    jq -e '.status.phase == "Pending" and .spec.nodeSelector["cka003/controlplane"] == "misspelled"' >/dev/null || return 1
  k get deployment rollback-app -n dev-wl07 -o json |
    jq -e '.spec.template.spec.containers[0].image == "nginx:cka003-invalid"' >/dev/null || return 1
  k get replicasets -n dev-wl07 -o json |
    jq -e 'any(.items[]; .spec.template.spec.containers[0].image == "nginx:1.27")' >/dev/null || return 1
  k get deployment db-deployment-cka05-trb -n default -o json |
    jq -e '(.status.readyReplicas // 0) == 0 and .spec.template.spec.containers[0].env[0].valueFrom.secretKeyRef.key == "wrong-root"' >/dev/null || return 1
  k get deployment black-cka25-trb -n default -o json |
    jq -e '.spec.paused == true and (.status.updatedReplicas // 0) == 0' >/dev/null || return 1
  k get service purple-svc-cka27-trb -n default -o json |
    jq -e '.spec.ports[0].targetPort == 81' >/dev/null || return 1
  k get pod purple-app-cka27-trb purple-curl-cka27-trb -n default -o json |
    jq -e 'all(.items[]; .status.phase == "Running")' >/dev/null || return 1
  k get clusterrole green-role-cka22-arch -o json |
    jq -e '.rules[0].verbs | index("delete") != null' >/dev/null || return 1
  k get serviceaccount green-sa-cka22-arch -n default >/dev/null || return 1
  k get clusterrolebinding green-role-binding-cka22-arch -o json |
    jq -e '.roleRef.name == "green-role-cka22-arch" and .subjects[0].name == "green-sa-cka22-arch" and .subjects[0].namespace == "default"' >/dev/null || return 1
  k get secret db-credentials-cka05-trb -n default -o json |
    jq -e '.data | keys == ["database","password","root-password","username"]' >/dev/null || return 1
  k get daemonset csi-nfs-node -n kube-system -o json |
    jq -e '.status.numberReady == 3' >/dev/null || return 1
  command -v helm >/dev/null || return 1
  grep -q 'app: incorrect-selector' /opt/webapp-color-apd/templates/deployment.yaml || return 1
  grep -q 'app: incorrect-selector' /opt/webapp-color-apd/templates/service.yaml || return 1
}
for _ in {1..5}; do
  if check; then echo 'CKA-003 starting state verified'; exit 0; fi
  sleep 2
done
echo 'CKA-003 readiness failed: inspect dependencies, fixtures, and intentional faults' >&2
exit 1
CHECK
