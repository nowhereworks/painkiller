#!/usr/bin/env bash
# Read-only, bounded captures. Missing answers are {}, API/permission errors fail.
# shellcheck disable=SC2016
set -Eeuo pipefail
get() {
  local result
  result=$(kubectl --request-timeout=8s get "$@" --ignore-not-found -o json) || return 1
  printf '%s\n' "${result:-{\}}"
}
file() {
  if sudo -n test -f "$1"; then sudo -n cat "$1"; fi
}
evaluate() {
  jq -n "$@" 'def ready($n): .spec.replicas == $n and .status.replicas == $n and .status.observedGeneration >= .metadata.generation and .status.updatedReplicas == $n and .status.readyReplicas == $n and .status.availableReplicas == $n;
    def running: .status.phase == "Running" and any(.status.conditions[]?; .type == "Ready" and .status == "True");
    def bound($role; $sa): .roleRef.kind == "ClusterRole" and .roleRef.name == $role and .roleRef.apiGroup == "rbac.authorization.k8s.io" and .subjects == [{"kind":"ServiceAccount","name":$sa,"namespace":"default"}];
    {ok: (try (('"$expression"') // false) catch false)}'
}
probe() {
  if curl --connect-timeout 2 --max-time 4 -fsS "$@" 2>/dev/null | grep -q 'Welcome to nginx'; then echo true; else echo false; fi
}
access() {
  local result
  # kubectl warns that namespaces are cluster-scoped; captures must emit JSON only.
  # Suppressing diagnostics does not turn errors into denials: only literal "no" does.
  result=$(kubectl --request-timeout=5s auth can-i "$1" "$2" --as="system:serviceaccount:default:$3" 2>/dev/null) ||
    [[ $result == no ]] || return 1
  case "$result" in yes) echo true ;; no) echo false ;; *) return 1 ;; esac
}
case "${1:-}" in
q1)
  expression='$h.spec.scaleTargetRef == {"apiVersion":"apps/v1","kind":"Deployment","name":"quay-frontend"} and $h.spec.minReplicas == 3 and $h.spec.maxReplicas == 9 and any($h.spec.metrics[]?; .type == "Resource" and .resource.name == "cpu" and .resource.target.type == "Utilization" and .resource.target.averageUtilization == 75)'
  evaluate --argjson h "$(get hpa quay-frontend -n default)"
  ;;
q2)
  expression='$r.rules == [{"apiGroups":[""],"resources":["namespaces"],"verbs":["get"]}] and ($b | bound("namespace-observer-role";"namespace-observer")) and $sa.metadata.name == "namespace-observer" and $get and ($list or $delete or $pods | not)'
  evaluate --argjson r "$(get clusterrole namespace-observer-role)" \
    --argjson b "$(get clusterrolebinding namespace-observer-binding)" \
    --argjson sa "$(get serviceaccount namespace-observer -n default)" \
    --argjson get "$(access get namespaces namespace-observer)" \
    --argjson list "$(access list namespaces namespace-observer)" \
    --argjson delete "$(access delete namespaces namespace-observer)" \
    --argjson pods "$(access get pods namespace-observer)"
  ;;
q3)
  expression='$d | ready(3)'
  evaluate --argjson d "$(get deployment catalog-api -n catalog-repair)"
  ;;
q4)
  expression='$p.spec.resources.requests.storage == "96Mi" and $p.status.phase == "Bound" and $p.status.capacity.storage == "96Mi" and $p.spec.volumeName == "cedar-pv" and $v.spec.capacity.storage == "96Mi" and $v.spec.csi.driver == "nfs.csi.k8s.io" and $s.allowVolumeExpansion == true and ($c | running)'
  evaluate --argjson p "$(get pvc cedar-pvc -n default)" \
    --argjson v "$(get pv cedar-pv)" --argjson s "$(get storageclass cedar-sc)" \
    --argjson c "$(get pod cedar-consumer -n default)"
  ;;
q5)
  release=$(helm list -n portal-lab -o json --filter '^harbor-portal$')
  chart=$(python3 - <<'PY'
import json
import yaml
result = {}
for name in ("Chart", "values"):
    try:
        with open("/opt/portal-chart/" + name + ".yaml") as stream:
            result[name] = yaml.safe_load(stream)
    except (OSError, yaml.YAMLError):
        result[name] = {}
print(json.dumps(result))
PY
)
  expression='any($r[]?; .name == "harbor-portal" and .namespace == "portal-lab" and .status == "deployed" and .app_version == "1.21.6") and ($d | ready(4)) and $d.spec.template.spec.containers[0].image == "nginx:1.21.6" and $s.spec.type == "NodePort" and $s.spec.selector == $d.spec.selector.matchLabels and $chart.Chart.appVersion == "1.21.6" and $chart.values.replicaCount == 4 and $chart.values.service.type == "NodePort"'
  evaluate --argjson r "$release" --argjson chart "$chart" \
    --argjson d "$(get deployment harbor-portal -n portal-lab)" \
    --argjson s "$(get service harbor-portal -n portal-lab)"
  ;;
q6)
  expression='$h.spec.hostnames == ["practice-controller"] and any($h.spec.parentRefs[]?; .name == "portal-gateway" and (.namespace // "gateway-lab") == "gateway-lab") and any($h.spec.rules[]?.backendRefs[]?; .name == "portal-service" and .port == 80 and (.namespace // "gateway-lab") == "gateway-lab") and any($h.status.parents[]?; .parentRef.name == "portal-gateway" and any(.conditions[]?; .type == "Accepted" and .status == "True") and any(.conditions[]?; .type == "ResolvedRefs" and .status == "True")) and $http'
  evaluate --argjson h "$(get httproute portal-route -n gateway-lab)" \
    --argjson http "$(probe -H 'Host: practice-controller' http://practice-controller:30080/)"
  ;;
q7)
  expression='$sa.metadata.name == "release-reader" and $r.rules == [{"apiGroups":["apps"],"resources":["deployments"],"verbs":["get"]}] and ($b | bound("release-reader-role";"release-reader")) and $get and ($list or $delete | not)'
  evaluate --argjson sa "$(get serviceaccount release-reader -n default)" \
    --argjson r "$(get clusterrole release-reader-role)" \
    --argjson b "$(get clusterrolebinding release-reader-binding)" \
    --argjson get "$(access get deployments.apps release-reader)" \
    --argjson list "$(access list deployments.apps release-reader)" \
    --argjson delete "$(access delete deployments.apps release-reader)"
  ;;
q8)
  pod=$(get pod dns-web -n default)
  svc=$(get service dns-web-service -n default)
  expression='($p | running) and any($p.spec.containers[]?; .image == "nginx" or (.image | startswith("nginx:"))) and $s.spec.type == "ClusterIP" and ($s.spec.selector | length) > 0 and all(($s.spec.selector // {}) | to_entries[]; $p.metadata.labels[.key] == .value) and $s.spec.ports[0].port == 80 and ($sf | contains("dns-web-service")) and ($sf | contains($s.spec.clusterIP // "missing-service-ip")) and ($pf | contains(($p.status.podIP // "missing-pod-ip" | gsub("\\."; "-")) + ".default.pod")) and ($pf | contains($p.status.podIP // "missing-pod-ip"))'
  evaluate --argjson p "$pod" --argjson s "$svc" \
    --arg sf "$(file /root/CKA/service-dns.txt)" --arg pf "$(file /root/CKA/pod-dns.txt)"
  ;;
q9)
  expression='($p | running) and $p.spec.nodeName == $node'
  evaluate --argjson p "$(get pod queued-web -n default)" \
    --arg node "$(kubectl --request-timeout=8s get nodes -l node-role.kubernetes.io/control-plane -o 'jsonpath={.items[0].metadata.name}')"
  ;;
q10)
  expression='($d | ready(6)) and $d.spec.template.spec.containers[0].image == "nginx:1.27" and $record == "nginx:1.27" and any($rs.items[]?; .spec.template.spec.containers[0].image == "nginx:cka003-invalid")'
  evaluate --argjson d "$(get deployment inventory-api -n release-lab)" \
    --argjson rs "$(get replicasets -n release-lab -l app=inventory-api)" \
    --arg record "$(file /root/restored-image.txt)"
  ;;
q11)
  expression='($d | ready(1)) and ($d.spec.template.spec.containers[0].env // [] | sort_by(.name)) == ([
    {name:"DB_ROOT_PASSWORD",valueFrom:{secretKeyRef:{name:"ledger-credentials",key:"admin-password"}}},
    {name:"DB_DATABASE",valueFrom:{secretKeyRef:{name:"ledger-credentials",key:"db-name"}}},
    {name:"DB_USER",valueFrom:{secretKeyRef:{name:"ledger-credentials",key:"db-user"}}},
    {name:"DB_PASSWORD",valueFrom:{secretKeyRef:{name:"ledger-credentials",key:"db-password"}}}
    ] | sort_by(.name)) and $s.data == {"admin-password":("practice-root"|@base64),"db-name":("practice"|@base64),"db-user":("practice-user"|@base64),"db-password":("practice-password"|@base64)}'
  evaluate --argjson d "$(get deployment ledger-db -n default)" \
    --argjson s "$(get secret ledger-credentials -n default)"
  ;;
q12)
  expression='($d | ready(4)) and $d.spec.template.spec.containers[0].image == "kodekloud/webapp-color:v1" and $d.spec.strategy.type == "RollingUpdate" and $d.spec.strategy.rollingUpdate.maxSurge == "60%" and $d.spec.strategy.rollingUpdate.maxUnavailable == "30%" and $record == "2" and (($d.metadata.annotations["deployment.kubernetes.io/revision"] // "0" | tonumber) >= 3) and any($rs.items[]?; any(.metadata.ownerReferences[]?; .uid == $d.metadata.uid and .kind == "Deployment") and .spec.template.spec.containers[0].image == "kodekloud/webapp-color:v2" and .metadata.annotations["deployment.kubernetes.io/revision"] == "2")'
  evaluate --argjson d "$(get deployment color-stream -n default)" \
    --argjson rs "$(get replicasets -n default)" \
    --arg record "$(file /opt/rollout-revision.txt)"
  ;;
q13)
  expression='$i.spec.ingressClassName == "harbor-http" and $i.metadata.annotations["nginx.ingress.kubernetes.io/ssl-redirect"] == "false" and any($i.spec.rules[]?; (.host // "") == "" and any(.http.paths[]?; .path == "/" and .pathType == "Prefix" and .backend.service.name == "site-service" and .backend.service.port.number == 80)) and $http'
  evaluate --argjson i "$(get ingress portal-ingress -n default)" \
    --argjson http "$(probe -H 'Host: ingress.cka003.test' http://practice-controller:30080/)"
  ;;
q14)
  expression='($d | ready(1)) and ($d.spec.paused // false) == false and $d.spec.template.metadata.annotations["cka003/update"] == "queued"'
  evaluate --argjson d "$(get deployment reports-api -n default)"
  ;;
q15)
  http=false
  if kubectl --request-timeout=8s exec status-monitor -n default -- \
    curl --connect-timeout 2 --max-time 4 -fsS http://status-service:8080 2>/dev/null | grep -q 'Welcome to nginx'; then http=true; fi
  expression='($p | running) and $s.spec.type == "ClusterIP" and $s.spec.ports[0].port == 8080 and $s.spec.ports[0].targetPort == 80 and $s.spec.selector.app == "status" and $http'
  evaluate --argjson p "$(get pod status-web -n default)" \
    --argjson s "$(get service status-service -n default)" --argjson http "$http"
  ;;
q16)
  expression='$s.provisioner == "kubernetes.io/no-provisioner" and $s.volumeBindingMode == "WaitForFirstConsumer" and $s.allowVolumeExpansion == true'
  evaluate --argjson s "$(get storageclass archive-local)"
  ;;
*) echo "Unknown CKA-003 task: ${1:-}" >&2; exit 2 ;;
esac
