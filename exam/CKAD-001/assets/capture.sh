#!/usr/bin/env bash
# Bounded, read-only captures: missing/malformed answers fail, API errors propagate.
# shellcheck disable=SC2016
set -Eeuo pipefail
get() {
  local result
  result=$(kubectl --request-timeout=6s get "$@" --ignore-not-found -o json) || return 1
  printf '%s\n' "${result:-{\}}"
}
file() { if sudo -n test -f "$1"; then sudo -n cat "$1"; fi; }
yaml_json() {
  file "$1" | python3 -c 'import json,sys,yaml
try:
    value=yaml.safe_load(sys.stdin)
except yaml.YAMLError:
    value={}
print(json.dumps(value or {}))'
}
evaluate() {
  jq -n "$@" '
    def running: .status.phase == "Running" and any(.status.conditions[]?; .type == "Ready" and .status == "True");
    def ready($n): .spec.replicas == $n and .status.observedGeneration >= .metadata.generation and .status.updatedReplicas == $n and .status.readyReplicas == $n and .status.availableReplicas == $n and .status.replicas == $n;
    def container($name): first(.spec.containers[] | select(.name == $name));
    def command: ((.command // []) + (.args // [])) | join(" ");
    def service($type; $port; $target): .spec.type == $type and any(.spec.ports[]?; .port == $port and .targetPort == $target and .protocol == "TCP");
    {ok: (try (('"$expression"') // false) catch false)}'
}
probe() {
  local output
  output=$(timeout 8 kubectl --request-timeout=6s exec -n ckad001-system observer -- \
    wget -T 3 -qO- "$1" 2>/dev/null) || { echo false; return; }
  if [[ -n $output && ( -z ${2:-} || $output == *"$2"* ) ]]; then echo true; else echo false; fi
}
case "${1:-}" in
q1)
  expression='($p | running) and ($p.spec.containers | length == 3) and
    ($p | container("alpha") | .image == "busybox:1.28" and any(.env[]?; .name == "ORDER" and .value == "LEAD")) and
    ($p | container("web") | .image == "nginx:1.27" and any(.ports[]?; .containerPort == 8081)) and
    ($p | container("omega") | .image == "busybox:1.31.1" and any(.env[]?; .name == "ORDER" and .value == "TAIL")) and
    ($p.status.containerStatuses | length == 3 and all(.[]; .state.running != null and .ready))'
  evaluate --argjson p "$(get pod trio-runner -n container-lab)"
  ;;
q2)
  expression='$s.provisioner == "kubernetes.io/no-provisioner" and $s.volumeBindingMode == "WaitForFirstConsumer" and $s.allowVolumeExpansion == true'
  evaluate --argjson s "$(get storageclass cedar-static)"
  ;;
q3)
  expression='$j.spec.schedule == "*/20 * * * *" and ($j.spec.suspend != true) and
    ($j.spec.jobTemplate.spec.template.spec.restartPolicy | . == "OnFailure" or . == "Never") and
    any($j.spec.jobTemplate.spec.template.spec.containers[]?; .image == "node:22-bookworm" and
      (command | test("(^|/)(ba)?sh -(e?c|ce) ps -eaf$")))'
  evaluate --argjson j "$(get cronjob process-report -n batch-lab)"
  ;;
q4)
  expression='($p | running) and ($p | container("ubuntu-agent") | .image == "ubuntu:24.04" and (command | test("(^|[ /])sleep 7200$")))'
  evaluate --argjson p "$(get pod ubuntu-rest -n pod-workshop)"
  ;;
q5)
  service=$(get service release-entry -n default)
  port=$(jq -r '.spec.ports[0].nodePort // 0' <<<"$service")
  ip=$(kubectl --request-timeout=6s get nodes -l node-role.kubernetes.io/control-plane -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
  expression='($a | ready(6)) and ($b | ready(4)) and
    $a.metadata.labels["track-one"] == "azure" and $a.spec.template.metadata.labels["track-one"] == "azure" and
    $b.metadata.labels["track-two"] == "jade" and $b.spec.template.metadata.labels["track-two"] == "jade" and
    $a.spec.template.metadata.labels.release == "stable" and $b.spec.template.metadata.labels.release == "stable" and
    $a.spec.template.spec.containers[0].image == "kodekloud/webapp-color:v1" and
    $b.spec.template.spec.containers[0].image == "kodekloud/webapp-color:v2" and
    ($s | service("NodePort";8081;8080)) and $s.spec.selector == {"release":"stable"} and
    ([$e.items[]?.endpoints[]? | select(.conditions.ready == true)] | length == 10) and $http'
  evaluate --argjson a "$(get deployment azure-web -n default)" --argjson b "$(get deployment jade-web -n default)" \
    --argjson s "$service" --argjson e "$(get endpointslices -n default -l kubernetes.io/service-name=release-entry)" \
    --argjson http "$(probe "http://$ip:$port")"
  ;;
q6)
  expression='($d | ready(2)) and $d.spec.template.spec.containers[0].image == "nginx:1.27" and
    $d.spec.template.spec.containers[0].readinessProbe.httpGet.port == 80 and $http'
  evaluate --argjson d "$(get deployment bulletin-web -n dev-workshop)" \
    --argjson http "$(probe http://bulletin-entry.dev-workshop 'Welcome to nginx')"
  ;;
q7)
  release=$(helm list -n frontend-workshop -o json --filter '^lantern-web$')
  expression='any($r[]?; .name == "lantern-web" and .namespace == "frontend-workshop" and .status == "deployed" and .app_version == "1.21.0") and
    $chart.appVersion == "1.21.0" and $values.replicaCount == 4 and $values.service.type == "NodePort" and
    $values.image == {"repository":"kodekloud/webapp-color","tag":"v1"} and
    ($d | ready(4)) and $d.spec.template.spec.containers[0].image == "kodekloud/webapp-color:v1" and
    ($s | service("NodePort";8080;8080)) and $s.spec.selector.app == "lantern-web" and $http'
  evaluate --argjson r "$release" --argjson chart "$(yaml_json /opt/lantern-chart/Chart.yaml)" \
    --argjson values "$(yaml_json /opt/lantern-chart/values.yaml)" \
    --argjson d "$(get deployment lantern-web -n frontend-workshop)" --argjson s "$(get service lantern-web -n frontend-workshop)" \
    --argjson http "$(probe http://lantern-web.frontend-workshop:8080)"
  ;;
q8)
  expression='($s | service("ClusterIP";6445;80)) and ($s.spec.selector | length > 0) and
    all($s.spec.selector | to_entries[]; $p.metadata.labels[.key] == .value) and $http'
  evaluate --argjson s "$(get service parcel-entry -n default)" --argjson p "$(get pod parcel-web -n default)" \
    --argjson http "$(probe http://parcel-entry.default:6445 'Welcome to nginx')"
  ;;
q9)
  expression='$n.metadata.name == "sealed-workshop" and $p.metadata.name == "isolate-everything" and
    $p.spec.podSelector == {} and ($p.spec.policyTypes | sort) == ["Egress","Ingress"] and
    ($p.spec.ingress // []) == [] and ($p.spec.egress // []) == [] and
    ($policies.items | length == 1)'
  evaluate --argjson n "$(get namespace sealed-workshop)" --argjson p "$(get networkpolicy isolate-everything -n sealed-workshop)" \
    --argjson policies "$(get networkpolicies -n sealed-workshop)"
  ;;
q10)
  ip=$(kubectl --request-timeout=6s get nodes -l node-role.kubernetes.io/control-plane -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
  expression='($s.spec.selector // {}) == {} and ($s | service("ClusterIP";8082;9998)) and
    $e.addressType == "IPv4" and $e.metadata.labels["kubernetes.io/service-name"] == "host-web-entry" and
    $e.endpoints == [{"addresses":[$ip],"conditions":{"ready":true}}] and
    any($e.ports[]?; .name == "http" and .protocol == "TCP" and .port == 9998) and $http'
  evaluate --arg ip "$ip" --argjson s "$(get service host-web-entry -n default)" \
    --argjson e "$(get endpointslice host-web-backend -n default)" \
    --argjson http "$(probe http://host-web-entry.default:8082 ckad001-host-web)"
  ;;
q11)
  expression='($p | running) and $p.spec.containers[0].image == "kodekloud/webapp-color:v1" and
    $p.metadata.labels.tier == "messages" and ($s | service("ClusterIP";6381;8080)) and
    $s.spec.selector == {"tier":"messages"} and $http'
  evaluate --argjson p "$(get pod message-web -n default)" --argjson s "$(get service message-entry -n default)" \
    --argjson http "$(probe http://message-entry.default:6381)"
  ;;
q12)
  expression='$s.spec.type == "ExternalName" and $s.spec.externalName == "dns.google" and any($s.spec.ports[]?; .port == 53 and .protocol == "UDP")'
  evaluate --argjson s "$(get service resolver-alias -n default)"
  ;;
q13)
  expression='($p | running) and $p.status.qosClass == "Burstable" and
    ($p | container("elastic-container") | .image == "nginx:1.27") and
    ($report | split("\n") | map([scan("\\S+")] | select(length > 0))) as $rows |
    $rows[0] == ["NAME","QOS"] and
    ($rows[1:] | sort) == ([$pods.items[] | [.metadata.name,.status.qosClass]] | sort)'
  evaluate --argjson p "$(get pod elastic-web -n qos-workshop)" --argjson pods "$(get pods -n qos-workshop)" \
    --arg report "$(file /root/qos-report.txt)"
  ;;
q14)
  expression='$a.apiVersion == "entertainment.ckad001.test/v1" and $a.kind == "Anime" and $a.spec == {"title":"Cowboy Bebop","episodes":26}'
  evaluate --argjson a "$(get animes.entertainment.ckad001.test evening-series -n default)"
  ;;
q15)
  expression='$c.data == {"APP_MODE":"practice","APP_REGION":"harbor","CACHE_HOST":"cache02","CACHE_TTL":"180"}'
  evaluate --argjson c "$(get configmap workshop-settings -n default)"
  ;;
q16)
  p=$(get pod ledger-client -n database-workshop)
  envs=''
  if jq -e '.status.phase == "Running"' >/dev/null <<<"$p"; then
    envs=$(timeout 8 kubectl --request-timeout=6s exec -n database-workshop ledger-client -- \
      printenv DB_Host DB_User DB_Password 2>/dev/null) || envs=''
  fi
  expression='($p | running) and $s.data == {"DB_Host":("sql02"|@base64),"DB_User":("ledger"|@base64),"DB_Password":("practice456"|@base64)} and
    ($p | container("client") | .image == "busybox:1.36" and
      .command == ["sh","-c","test \"$DB_Host\" = sql02 && test \"$DB_User\" = ledger && test \"$DB_Password\" = practice456 && exec sleep 86400"] and
      any(.envFrom[]?; .secretRef.name == "ledger-settings" and (.prefix // "") == "") and
      all(.env[]?; .name != "DB_Host" and .name != "DB_User" and .name != "DB_Password")) and
    $envs == "sql02\nledger\npractice456"'
  evaluate --argjson p "$p" --argjson s "$(get secret ledger-settings -n database-workshop)" --arg envs "$envs"
  ;;
q17)
  expression='$q.metadata.name == "quota-object-cap" and
    ($q.spec.hard.resourcequotas == "1" or $q.spec.hard["count/resourcequotas"] == "1")'
  evaluate --argjson q "$(get resourcequota quota-object-cap -n quota-workshop)"
  ;;
q18)
  p=$(get pod vault-reader -n secret-workshop)
  token=''
  if jq -e '.status.phase == "Running"' >/dev/null <<<"$p"; then
    token=$(timeout 8 kubectl --request-timeout=6s exec -n secret-workshop vault-reader -- \
      cat /etc/credentials/.token 2>/dev/null) || token=''
  fi
  expression='($p | running) and
    any($p.spec.volumes[]?; .name == "credential-volume" and .secret.secretName == "vault-settings") and
    ($p | container("reader") | .image == "busybox:1.36" and .command == ["sh","-c","sleep 86400"] and
      any(.volumeMounts[]?; .name == "credential-volume" and .mountPath == "/etc/credentials" and .readOnly == true)) and
    $token == "workshop-token-42"'
  evaluate --argjson p "$p" --arg token "$token"
  ;;
q19)
  expression='($p | running) and ($p | container("web") | .image == "kodekloud/webapp-delayed-start" and
    .readinessProbe.httpGet.path == "/ready" and .readinessProbe.httpGet.port == 8080 and .readinessProbe.initialDelaySeconds == 20)'
  evaluate --argjson p "$(get pod warmup-web -n default)"
  ;;
q20)
  expression='$f.apiVersion == "v1" and $f.kind == "Pod" and $f.metadata.name == "toolbox-rest" and
    ($f.spec.containers[0] | .name == "toolbox" and .image == "busybox:1.36" and .command == ["sh","-c","sleep 86400"]) and
    ($p | running) and ($p | container("toolbox") | .image == "busybox:1.36" and .command == ["sh","-c","sleep 86400"])'
  evaluate --argjson f "$(yaml_json /root/workshop-busybox.yaml)" --argjson p "$(get pod toolbox-rest -n default)"
  ;;
q21)
  expression='($p | running) and any($p.spec.containers[]?; .image == "redis:7.4" and
    .livenessProbe.exec.command == ["ls"] and .livenessProbe.initialDelaySeconds == 8)'
  evaluate --argjson p "$(get pod redis-watch -n default)"
  ;;
*) echo 'Usage: capture.sh q1..q21' >&2; exit 2 ;;
esac
