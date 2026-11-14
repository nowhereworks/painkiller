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
  expression='$h.spec.scaleTargetRef == {"apiVersion":"apps/v1","kind":"Deployment","name":"frontend-deployment"} and $h.spec.minReplicas == 2 and $h.spec.maxReplicas == 10 and any($h.spec.metrics[]?; .type == "Resource" and .resource.name == "cpu" and .resource.target.type == "Utilization" and .resource.target.averageUtilization == 70)'
  evaluate --argjson h "$(get hpa frontend-deployment -n default)"
  ;;
q2)
  expression='$r.rules == [{"apiGroups":[""],"resources":["namespaces"],"verbs":["get"]}] and ($b | bound("green-role-cka22-arch";"green-sa-cka22-arch")) and $sa.metadata.name == "green-sa-cka22-arch" and $get and ($list or $delete or $pods | not)'
  evaluate --argjson r "$(get clusterrole green-role-cka22-arch)" \
    --argjson b "$(get clusterrolebinding green-role-binding-cka22-arch)" \
    --argjson sa "$(get serviceaccount green-sa-cka22-arch -n default)" \
    --argjson get "$(access get namespaces green-sa-cka22-arch)" \
    --argjson list "$(access list namespaces green-sa-cka22-arch)" \
    --argjson delete "$(access delete namespaces green-sa-cka22-arch)" \
    --argjson pods "$(access get pods green-sa-cka22-arch)"
  ;;
q3)
  expression='$d | ready(2)'
  evaluate --argjson d "$(get deployment pink-depl-cka14-trb -n pink-troubleshooting)"
  ;;
q4)
  expression='$p.spec.resources.requests.storage == "80Mi" and $p.status.phase == "Bound" and $p.status.capacity.storage == "80Mi" and $p.spec.volumeName == "papaya-pv-cka09-str" and $v.spec.capacity.storage == "80Mi" and $v.spec.csi.driver == "nfs.csi.k8s.io" and $s.allowVolumeExpansion == true and ($c | running)'
  evaluate --argjson p "$(get pvc papaya-pvc-cka09-str -n default)" \
    --argjson v "$(get pv papaya-pv-cka09-str)" --argjson s "$(get storageclass papaya-stc-cka09-str)" \
    --argjson c "$(get pod papaya-consumer -n default)"
  ;;
q5)
  release=$(helm list -n frontend-apd -o json --filter '^webapp-color-apd$')
  chart=$(python3 - <<'PY'
import json
import yaml
result = {}
for name in ("Chart", "values"):
    try:
        with open("/opt/webapp-color-apd/" + name + ".yaml") as stream:
            result[name] = yaml.safe_load(stream)
    except (OSError, yaml.YAMLError):
        result[name] = {}
print(json.dumps(result))
PY
)
  expression='any($r[]?; .name == "webapp-color-apd" and .namespace == "frontend-apd" and .status == "deployed" and .app_version == "1.20.0") and ($d | ready(3)) and $d.spec.template.spec.containers[0].image == "nginx:1.20.0" and $s.spec.type == "NodePort" and $s.spec.selector == $d.spec.selector.matchLabels and $chart.Chart.appVersion == "1.20.0" and $chart.values.replicaCount == 3 and $chart.values.service.type == "NodePort"'
  evaluate --argjson r "$release" --argjson chart "$chart" \
    --argjson d "$(get deployment webapp-color-apd -n frontend-apd)" \
    --argjson s "$(get service webapp-color-apd -n frontend-apd)"
  ;;
q6)
  expression='$h.spec.hostnames == ["cka003-controlplane"] and any($h.spec.parentRefs[]?; .name == "web-gateway" and (.namespace // "nginx-gateway") == "nginx-gateway") and any($h.spec.rules[]?.backendRefs[]?; .name == "web-service" and .port == 80 and (.namespace // "nginx-gateway") == "nginx-gateway") and any($h.status.parents[]?; .parentRef.name == "web-gateway" and any(.conditions[]?; .type == "Accepted" and .status == "True") and any(.conditions[]?; .type == "ResolvedRefs" and .status == "True")) and $http'
  evaluate --argjson h "$(get httproute web-route -n nginx-gateway)" \
    --argjson http "$(probe -H 'Host: cka003-controlplane' http://cka003-controlplane:30080/)"
  ;;
q7)
  expression='$sa.metadata.name == "deploy-cka20-arch" and $r.rules == [{"apiGroups":["apps"],"resources":["deployments"],"verbs":["get"]}] and ($b | bound("deploy-role-cka20-arch";"deploy-cka20-arch")) and $get and ($list or $delete | not)'
  evaluate --argjson sa "$(get serviceaccount deploy-cka20-arch -n default)" \
    --argjson r "$(get clusterrole deploy-role-cka20-arch)" \
    --argjson b "$(get clusterrolebinding deploy-role-binding-cka20-arch)" \
    --argjson get "$(access get deployments.apps deploy-cka20-arch)" \
    --argjson list "$(access list deployments.apps deploy-cka20-arch)" \
    --argjson delete "$(access delete deployments.apps deploy-cka20-arch)"
  ;;
q8)
  pod=$(get pod nginx-resolver-cka06-svcn -n default)
  svc=$(get service nginx-resolver-service-cka06-svcn -n default)
  expression='($p | running) and any($p.spec.containers[]?; .image == "nginx" or (.image | startswith("nginx:"))) and $s.spec.type == "ClusterIP" and ($s.spec.selector | length) > 0 and all(($s.spec.selector // {}) | to_entries[]; $p.metadata.labels[.key] == .value) and $s.spec.ports[0].port == 80 and ($sf | contains("nginx-resolver-service-cka06-svcn")) and ($sf | contains($s.spec.clusterIP // "missing-service-ip")) and ($pf | contains(($p.status.podIP // "missing-pod-ip" | gsub("\\."; "-")) + ".default.pod")) and ($pf | contains($p.status.podIP // "missing-pod-ip"))'
  evaluate --argjson p "$pod" --argjson s "$svc" \
    --arg sf "$(file /root/CKA/nginx.svc.cka06.svcn)" --arg pf "$(file /root/CKA/nginx.pod.cka06.svcn)"
  ;;
q9)
  expression='($p | running) and $p.spec.nodeName == $node'
  evaluate --argjson p "$(get pod grey-cka21-trb -n default)" \
    --arg node "$(kubectl --request-timeout=8s get nodes -l node-role.kubernetes.io/control-plane -o 'jsonpath={.items[0].metadata.name}')"
  ;;
q10)
  expression='($d | ready(5)) and $d.spec.template.spec.containers[0].image == "nginx:1.27" and $record == "nginx:1.27" and any($rs.items[]?; .spec.template.spec.containers[0].image == "nginx:cka003-invalid")'
  evaluate --argjson d "$(get deployment rollback-app -n dev-wl07)" \
    --argjson rs "$(get replicasets -n dev-wl07 -l app=rollback-app)" \
    --arg record "$(file /root/rolling-back-record.txt)"
  ;;
q11)
  expression='($d | ready(1)) and ($d.spec.template.spec.containers[0].env // [] | sort_by(.name)) == ([
    {name:"DB_ROOT_PASSWORD",valueFrom:{secretKeyRef:{name:"db-credentials-cka05-trb",key:"root-password"}}},
    {name:"DB_DATABASE",valueFrom:{secretKeyRef:{name:"db-credentials-cka05-trb",key:"database"}}},
    {name:"DB_USER",valueFrom:{secretKeyRef:{name:"db-credentials-cka05-trb",key:"username"}}},
    {name:"DB_PASSWORD",valueFrom:{secretKeyRef:{name:"db-credentials-cka05-trb",key:"password"}}}
    ] | sort_by(.name)) and $s.data == {"root-password":("practice-root"|@base64),"database":("practice"|@base64),"username":("practice-user"|@base64),"password":("practice-password"|@base64)}'
  evaluate --argjson d "$(get deployment db-deployment-cka05-trb -n default)" \
    --argjson s "$(get secret db-credentials-cka05-trb -n default)"
  ;;
q12)
  expression='($d | ready(3)) and $d.spec.template.spec.containers[0].image == "kodekloud/webapp-color:v1" and $d.spec.strategy.type == "RollingUpdate" and $d.spec.strategy.rollingUpdate.maxSurge == "55%" and $d.spec.strategy.rollingUpdate.maxUnavailable == "40%" and $record == "2" and (($d.metadata.annotations["deployment.kubernetes.io/revision"] // "0" | tonumber) >= 3) and any($rs.items[]?; any(.metadata.ownerReferences[]?; .uid == $d.metadata.uid and .kind == "Deployment") and .spec.template.spec.containers[0].image == "kodekloud/webapp-color:v2" and .metadata.annotations["deployment.kubernetes.io/revision"] == "2")'
  evaluate --argjson d "$(get deployment ocean-tv-wl09 -n default)" \
    --argjson rs "$(get replicasets -n default)" \
    --arg record "$(file /opt/revision-count.txt)"
  ;;
q13)
  expression='$i.spec.ingressClassName == "nginx-cka04" and $i.metadata.annotations["nginx.ingress.kubernetes.io/ssl-redirect"] == "false" and any($i.spec.rules[]?; (.host // "") == "" and any(.http.paths[]?; .path == "/" and .pathType == "Prefix" and .backend.service.name == "nginx-service-cka04-svcn" and .backend.service.port.number == 80)) and $http'
  evaluate --argjson i "$(get ingress nginx-ingress-cka04-svcn -n default)" \
    --argjson http "$(probe -H 'Host: ingress.cka003.test' http://cka003-controlplane:30080/)"
  ;;
q14)
  expression='($d | ready(1)) and ($d.spec.paused // false) == false and $d.spec.template.metadata.annotations["cka003/revision"] == "pending"'
  evaluate --argjson d "$(get deployment black-cka25-trb -n default)"
  ;;
q15)
  http=false
  if kubectl --request-timeout=8s exec purple-curl-cka27-trb -n default -- \
    curl --connect-timeout 2 --max-time 4 -fsS http://purple-svc-cka27-trb 2>/dev/null | grep -q 'Welcome to nginx'; then http=true; fi
  expression='($p | running) and $s.spec.type == "ClusterIP" and $s.spec.ports[0].port == 80 and $s.spec.ports[0].targetPort == 80 and $s.spec.selector.app == "purple" and $http'
  evaluate --argjson p "$(get pod purple-app-cka27-trb -n default)" \
    --argjson s "$(get service purple-svc-cka27-trb -n default)" --argjson http "$http"
  ;;
q16)
  expression='$s.provisioner == "kubernetes.io/no-provisioner" and $s.volumeBindingMode == "WaitForFirstConsumer" and $s.allowVolumeExpansion == true'
  evaluate --argjson s "$(get storageclass banana-sc-cka08-str)"
  ;;
*) echo "Unknown CKA-003 task: ${1:-}" >&2; exit 2 ;;
esac
