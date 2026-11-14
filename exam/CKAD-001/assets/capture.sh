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
    def requirement($message; condition):
      if (try ([condition] | any(.[]; . == true)) catch false) then empty else $message end;
    {ok: (try (['"$expression"'] | any(.[]; . == true)) catch false),
     failures: ('"$diagnostics"')}'
}
probe() {
  local output
  # The remote shell succeeds even when HTTP fails. A failed exec itself is
  # a capture error, not evidence that the candidate Service is incorrect.
  output=$(timeout 8 kubectl --request-timeout=6s exec -n ckad001-system observer -- \
    sh -c 'body=$(wget -T 3 -qO- "$1" 2>/dev/null) || exit 0; printf "%s" "$body"' sh "$1" 2>/dev/null) || {
    echo "Could not run HTTP probe from observer" >&2; return 1;
  }
  if [[ -n $output && ( -z ${2:-} || $output == *"$2"* ) ]]; then echo true; else echo false; fi
}
case "${1:-}" in
q1)
  jq -n --argjson p "$(get pod trio-runner -n container-lab)" '
    def container($name): first($p.spec.containers[]? | select(.name == $name)) // {};
    def requirement($message; $condition):
      {message: $message, passed: (try ($condition // false) catch false)};
    [
      requirement("Pod container-lab/trio-runner must exist"; $p.metadata.name == "trio-runner"),
      requirement("Pod must be Running and Ready";
        $p.status.phase == "Running" and any($p.status.conditions[]?; .type == "Ready" and .status == "True")),
      requirement("Pod must contain exactly three containers"; ($p.spec.containers | length) == 3),
      requirement("Container alpha must use busybox:1.28"; container("alpha").image == "busybox:1.28"),
      requirement("Container alpha must set ORDER=LEAD"; any(container("alpha").env[]?; .name == "ORDER" and .value == "LEAD")),
      requirement("Container web must use nginx:1.27"; container("web").image == "nginx:1.27"),
      requirement("Container web must declare containerPort 8081"; any(container("web").ports[]?; .containerPort == 8081)),
      requirement("Container omega must use busybox:1.31.1"; container("omega").image == "busybox:1.31.1"),
      requirement("Container omega must set ORDER=TAIL"; any(container("omega").env[]?; .name == "ORDER" and .value == "TAIL")),
      requirement("All three containers must be running and ready";
        ($p.status.containerStatuses | length) == 3 and all($p.status.containerStatuses[]?; .state.running != null and .ready))
    ] | {ok: all(.[]; .passed), failures: [.[] | select(.passed | not) | .message]}'
  ;;
q2)
  diagnostics='[requirement("StorageClass must use kubernetes.io/no-provisioner"; $s.provisioner == "kubernetes.io/no-provisioner"),
    requirement("StorageClass must use WaitForFirstConsumer binding"; $s.volumeBindingMode == "WaitForFirstConsumer"),
    requirement("StorageClass must allow volume expansion"; $s.allowVolumeExpansion == true)]'
  expression='$s.provisioner == "kubernetes.io/no-provisioner" and $s.volumeBindingMode == "WaitForFirstConsumer" and $s.allowVolumeExpansion == true'
  evaluate --argjson s "$(get storageclass cedar-static)"
  ;;
q3)
  diagnostics='[requirement("CronJob must run every 20 minutes and not be suspended"; $j.spec.schedule == "*/20 * * * *" and $j.spec.suspend != true),
    requirement("Job restartPolicy must be OnFailure or Never"; ($j.spec.jobTemplate.spec.template.spec.restartPolicy | . == "OnFailure" or . == "Never")),
    requirement("Container must use node:22-bookworm and run ps -eaf through a shell"; any($j.spec.jobTemplate.spec.template.spec.containers[]?; .image == "node:22-bookworm" and (command | test("(^|/)(ba)?sh -(e?c|ce) ps -eaf$"))))]'
  expression='$j.spec.schedule == "*/20 * * * *" and ($j.spec.suspend != true) and
    ($j.spec.jobTemplate.spec.template.spec.restartPolicy | . == "OnFailure" or . == "Never") and
    any($j.spec.jobTemplate.spec.template.spec.containers[]?; .image == "node:22-bookworm" and
      (command | test("(^|/)(ba)?sh -(e?c|ce) ps -eaf$")))'
  evaluate --argjson j "$(get cronjob process-report -n batch-lab)"
  ;;
q4)
  diagnostics='[requirement("Pod must be Running and Ready; retry after startup if still settling"; $p | running),
    requirement("ubuntu-agent must use ubuntu:24.04 and run sleep 7200"; $p | container("ubuntu-agent") | .image == "ubuntu:24.04" and (command | test("(^|[ /])sleep 7200$")))]'
  expression='($p | running) and ($p | container("ubuntu-agent") | .image == "ubuntu:24.04" and (command | test("(^|[ /])sleep 7200$")))'
  evaluate --argjson p "$(get pod ubuntu-rest -n pod-workshop)"
  ;;
q5)
  diagnostics='[requirement("azure-web must have six fully updated, ready, available replicas"; $a | ready(6)),
    requirement("jade-web must have four fully updated, ready, available replicas"; $b | ready(4)),
    requirement("Deployment and template track labels must match azure/jade"; $a.metadata.labels["track-one"] == "azure" and $a.spec.template.metadata.labels["track-one"] == "azure" and $b.metadata.labels["track-two"] == "jade" and $b.spec.template.metadata.labels["track-two"] == "jade"),
    requirement("Both Pod templates must have release=stable"; $a.spec.template.metadata.labels.release == "stable" and $b.spec.template.metadata.labels.release == "stable"),
    requirement("azure-web and jade-web must use webapp-color:v1 and :v2 respectively"; $a.spec.template.spec.containers[0].image == "kodekloud/webapp-color:v1" and $b.spec.template.spec.containers[0].image == "kodekloud/webapp-color:v2"),
    requirement("NodePort Service must select release=stable and map 8081 to 8080"; ($s | service("NodePort";8081;8080)) and $s.spec.selector == {"release":"stable"}),
    requirement("Service must have exactly ten ready endpoints"; ([$e.items[]?.endpoints[]? | select(.conditions.ready == true)] | length) == 10),
    requirement("NodePort must return a nonempty HTTP response"; $http)]'
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
  diagnostics='[requirement("Deployment must have two fully updated, ready, available replicas"; $d | ready(2)),
    requirement("Container must use nginx:1.27 with an HTTP readiness probe on port 80"; $d.spec.template.spec.containers[0].image == "nginx:1.27" and $d.spec.template.spec.containers[0].readinessProbe.httpGet.port == 80),
    requirement("Service must return the nginx welcome page"; $http)]'
  expression='($d | ready(2)) and $d.spec.template.spec.containers[0].image == "nginx:1.27" and
    $d.spec.template.spec.containers[0].readinessProbe.httpGet.port == 80 and $http'
  evaluate --argjson d "$(get deployment bulletin-web -n dev-workshop)" \
    --argjson http "$(probe http://bulletin-entry.dev-workshop 'Welcome to nginx')"
  ;;
q7)
  diagnostics='[requirement("lantern-web Helm release must be deployed in frontend-workshop with appVersion 1.21.0"; any($r[]?; .name == "lantern-web" and .namespace == "frontend-workshop" and .status == "deployed" and .app_version == "1.21.0")),
    requirement("Chart appVersion must be 1.21.0"; $chart.appVersion == "1.21.0"),
    requirement("Chart values must specify four replicas, NodePort, and webapp-color:v1"; $values.replicaCount == 4 and $values.service.type == "NodePort" and $values.image == {"repository":"kodekloud/webapp-color","tag":"v1"}),
    requirement("Deployment must have four fully updated, ready, available replicas using webapp-color:v1"; ($d | ready(4)) and $d.spec.template.spec.containers[0].image == "kodekloud/webapp-color:v1"),
    requirement("NodePort Service must select app=lantern-web and map 8080 to 8080"; ($s | service("NodePort";8080;8080)) and $s.spec.selector.app == "lantern-web"),
    requirement("Service must return a nonempty HTTP response"; $http)]'
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
  diagnostics='[requirement("ClusterIP Service must map TCP port 6445 to 80"; $s | service("ClusterIP";6445;80)),
    requirement("Service must have a nonempty selector matching the Pod labels"; ($s.spec.selector | length) > 0 and all($s.spec.selector | to_entries[]; $p.metadata.labels[.key] == .value)),
    requirement("Service must return the nginx welcome page"; $http)]'
  expression='($s | service("ClusterIP";6445;80)) and ($s.spec.selector | length > 0) and
    all($s.spec.selector | to_entries[]; $p.metadata.labels[.key] == .value) and $http'
  evaluate --argjson s "$(get service parcel-entry -n default)" --argjson p "$(get pod parcel-web -n default)" \
    --argjson http "$(probe http://parcel-entry.default:6445 'Welcome to nginx')"
  ;;
q9)
  diagnostics='[requirement("Namespace sealed-workshop and policy isolate-everything must exist"; $n.metadata.name == "sealed-workshop" and $p.metadata.name == "isolate-everything"),
    requirement("Policy must select every Pod and deny both ingress and egress"; $p.spec.podSelector == {} and ($p.spec.policyTypes | sort) == ["Egress","Ingress"] and ($p.spec.ingress // []) == [] and ($p.spec.egress // []) == []),
    requirement("Namespace must have exactly one NetworkPolicy"; ($policies.items | length) == 1)]'
  expression='$n.metadata.name == "sealed-workshop" and $p.metadata.name == "isolate-everything" and
    $p.spec.podSelector == {} and ($p.spec.policyTypes | sort) == ["Egress","Ingress"] and
    ($p.spec.ingress // []) == [] and ($p.spec.egress // []) == [] and
    ($policies.items | length == 1)'
  evaluate --argjson n "$(get namespace sealed-workshop)" --argjson p "$(get networkpolicy isolate-everything -n sealed-workshop)" \
    --argjson policies "$(get networkpolicies -n sealed-workshop)"
  ;;
q10)
  diagnostics='[requirement("Selectorless ClusterIP Service must map TCP port 8082 to 9998"; ($s.spec.selector // {}) == {} and ($s | service("ClusterIP";8082;9998))),
    requirement("IPv4 EndpointSlice must belong to host-web-entry and point only at the controller IP with a ready endpoint"; $e.addressType == "IPv4" and $e.metadata.labels["kubernetes.io/service-name"] == "host-web-entry" and $e.endpoints == [{"addresses":[$ip],"conditions":{"ready":true}}]),
    requirement("EndpointSlice must declare TCP http port 9998"; any($e.ports[]?; .name == "http" and .protocol == "TCP" and .port == 9998)),
    requirement("Service must return the host web fixture response"; $http)]'
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
  diagnostics='[requirement("Pod must be Running and Ready; retry after startup if still settling"; $p | running),
    requirement("Pod must use webapp-color:v1 with tier=messages"; $p.spec.containers[0].image == "kodekloud/webapp-color:v1" and $p.metadata.labels.tier == "messages"),
    requirement("ClusterIP Service must select tier=messages and map TCP port 6381 to 8080"; ($s | service("ClusterIP";6381;8080)) and $s.spec.selector == {"tier":"messages"}),
    requirement("Service must return a nonempty HTTP response"; $http)]'
  expression='($p | running) and $p.spec.containers[0].image == "kodekloud/webapp-color:v1" and
    $p.metadata.labels.tier == "messages" and ($s | service("ClusterIP";6381;8080)) and
    $s.spec.selector == {"tier":"messages"} and $http'
  evaluate --argjson p "$(get pod message-web -n default)" --argjson s "$(get service message-entry -n default)" \
    --argjson http "$(probe http://message-entry.default:6381)"
  ;;
q12)
  diagnostics='[requirement("Service must be ExternalName pointing at dns.google"; $s.spec.type == "ExternalName" and $s.spec.externalName == "dns.google"),
    requirement("Service must declare UDP port 53"; any($s.spec.ports[]?; .port == 53 and .protocol == "UDP"))]'
  expression='$s.spec.type == "ExternalName" and $s.spec.externalName == "dns.google" and any($s.spec.ports[]?; .port == 53 and .protocol == "UDP")'
  evaluate --argjson s "$(get service resolver-alias -n default)"
  ;;
q13)
  diagnostics='[requirement("Pod must be Running and Ready with Burstable QoS"; ($p | running) and $p.status.qosClass == "Burstable"),
    requirement("elastic-container must use nginx:1.27"; $p | container("elastic-container") | .image == "nginx:1.27"),
    requirement("QoS report must have NAME QOS header and exactly match every namespace Pod, in any order"; ($report | split("\n") | map([scan("\\S+")] | select(length > 0))) as $rows | $rows[0] == ["NAME","QOS"] and ($rows[1:] | sort) == ([$pods.items[] | [.metadata.name,.status.qosClass]] | sort))]'
  expression='($p | running) and $p.status.qosClass == "Burstable" and
    ($p | container("elastic-container") | .image == "nginx:1.27") and
    ($report | split("\n") | map([scan("\\S+")] | select(length > 0))) as $rows |
    $rows[0] == ["NAME","QOS"] and
    ($rows[1:] | sort) == ([$pods.items[] | [.metadata.name,.status.qosClass]] | sort)'
  evaluate --argjson p "$(get pod elastic-web -n qos-workshop)" --argjson pods "$(get pods -n qos-workshop)" \
    --arg report "$(file /root/qos-report.txt)"
  ;;
q14)
  diagnostics='[requirement("Resource must use entertainment.ckad001.test/v1 and kind Anime"; $a.apiVersion == "entertainment.ckad001.test/v1" and $a.kind == "Anime"),
    requirement("Anime spec must contain only title Cowboy Bebop and episodes 26"; $a.spec == {"title":"Cowboy Bebop","episodes":26})]'
  expression='$a.apiVersion == "entertainment.ckad001.test/v1" and $a.kind == "Anime" and $a.spec == {"title":"Cowboy Bebop","episodes":26}'
  evaluate --argjson a "$(get animes.entertainment.ckad001.test evening-series -n default)"
  ;;
q15)
  diagnostics='[requirement("ConfigMap must contain exactly APP_MODE=practice, APP_REGION=harbor, CACHE_HOST=cache02, CACHE_TTL=180"; $c.data == {"APP_MODE":"practice","APP_REGION":"harbor","CACHE_HOST":"cache02","CACHE_TTL":"180"})]'
  expression='$c.data == {"APP_MODE":"practice","APP_REGION":"harbor","CACHE_HOST":"cache02","CACHE_TTL":"180"}'
  evaluate --argjson c "$(get configmap workshop-settings -n default)"
  ;;
q16)
  diagnostics='[requirement("Pod must be Running and Ready; retry after startup if still settling"; $p | running),
    requirement("Secret must contain exactly the requested DB_Host, DB_User, and DB_Password values"; $s.data == {"DB_Host":("sql02"|@base64),"DB_User":("ledger"|@base64),"DB_Password":("practice456"|@base64)}),
    requirement("client must retain its supplied image and command and import ledger-settings without a prefix or overriding DB variables"; $p | container("client") | .image == "busybox:1.36" and .command == ["sh","-c","test \"$DB_Host\" = sql02 && test \"$DB_User\" = ledger && test \"$DB_Password\" = practice456 && exec sleep 86400"] and any(.envFrom[]?; .secretRef.name == "ledger-settings" and (.prefix // "") == "") and all(.env[]?; .name != "DB_Host" and .name != "DB_User" and .name != "DB_Password")),
    requirement("Live container environment must match all three Secret values"; $envs == "sql02\nledger\npractice456")]'
  p=$(get pod ledger-client -n database-workshop)
  envs=''
  # Phase Running can still mean CrashLoopBackOff. An unready answer already
  # fails grading; do not mistake inability to exec there for a capture outage.
  if jq -e '.status.phase == "Running" and any(.status.conditions[]?; .type == "Ready" and .status == "True")' >/dev/null <<<"$p"; then
    envs=$(timeout 8 kubectl --request-timeout=6s exec -n database-workshop ledger-client -- \
      sh -c 'printenv DB_Host DB_User DB_Password || true' 2>/dev/null) || {
      echo "Could not inspect the live client environment" >&2; exit 1;
    }
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
  diagnostics='[requirement("ResourceQuota quota-object-cap must exist"; $q.metadata.name == "quota-object-cap"),
    requirement("ResourceQuota must limit resourcequotas or count/resourcequotas to one"; $q.spec.hard.resourcequotas == "1" or $q.spec.hard["count/resourcequotas"] == "1")]'
  expression='$q.metadata.name == "quota-object-cap" and
    ($q.spec.hard.resourcequotas == "1" or $q.spec.hard["count/resourcequotas"] == "1")'
  evaluate --argjson q "$(get resourcequota quota-object-cap -n quota-workshop)"
  ;;
q18)
  diagnostics='[requirement("Pod must be Running and Ready; retry after startup if still settling"; $p | running),
    requirement("credential-volume must reference vault-settings"; any($p.spec.volumes[]?; .name == "credential-volume" and .secret.secretName == "vault-settings")),
    requirement("reader must retain its supplied image/command and mount credential-volume read-only at /etc/credentials"; $p | container("reader") | .image == "busybox:1.36" and .command == ["sh","-c","sleep 86400"] and any(.volumeMounts[]?; .name == "credential-volume" and .mountPath == "/etc/credentials" and .readOnly == true)),
    requirement("Mounted .token must be readable with the expected content"; $token == "workshop-token-42")]'
  p=$(get pod vault-reader -n secret-workshop)
  token=''
  if jq -e '.status.phase == "Running" and any(.status.conditions[]?; .type == "Ready" and .status == "True")' >/dev/null <<<"$p"; then
    token=$(timeout 8 kubectl --request-timeout=6s exec -n secret-workshop vault-reader -- \
      sh -c 'cat /etc/credentials/.token 2>/dev/null || true' 2>/dev/null) || {
      echo "Could not inspect the mounted credential" >&2; exit 1;
    }
  fi
  expression='($p | running) and
    any($p.spec.volumes[]?; .name == "credential-volume" and .secret.secretName == "vault-settings") and
    ($p | container("reader") | .image == "busybox:1.36" and .command == ["sh","-c","sleep 86400"] and
      any(.volumeMounts[]?; .name == "credential-volume" and .mountPath == "/etc/credentials" and .readOnly == true)) and
    $token == "workshop-token-42"'
  evaluate --argjson p "$p" --arg token "$token"
  ;;
q19)
  diagnostics='[requirement("Pod must be Running and Ready; retry after startup if still settling"; $p | running),
    requirement("web must use the delayed-start image and HTTP readiness probe /ready:8080 with initialDelaySeconds 20"; $p | container("web") | .image == "kodekloud/webapp-delayed-start" and .readinessProbe.httpGet.path == "/ready" and .readinessProbe.httpGet.port == 8080 and .readinessProbe.initialDelaySeconds == 20)]'
  expression='($p | running) and ($p | container("web") | .image == "kodekloud/webapp-delayed-start" and
    .readinessProbe.httpGet.path == "/ready" and .readinessProbe.httpGet.port == 8080 and .readinessProbe.initialDelaySeconds == 20)'
  evaluate --argjson p "$(get pod warmup-web -n default)"
  ;;
q20)
  diagnostics='[requirement("Source file must define v1 Pod toolbox-rest"; $f.apiVersion == "v1" and $f.kind == "Pod" and $f.metadata.name == "toolbox-rest"),
    requirement("Source toolbox container must use busybox:1.36 and command sh -c sleep 86400"; $f.spec.containers[0] | .name == "toolbox" and .image == "busybox:1.36" and .command == ["sh","-c","sleep 86400"]),
    requirement("Live Pod must be Running and Ready with matching toolbox image and command"; ($p | running) and ($p | container("toolbox") | .image == "busybox:1.36" and .command == ["sh","-c","sleep 86400"]))]'
  expression='$f.apiVersion == "v1" and $f.kind == "Pod" and $f.metadata.name == "toolbox-rest" and
    ($f.spec.containers[0] | .name == "toolbox" and .image == "busybox:1.36" and .command == ["sh","-c","sleep 86400"]) and
    ($p | running) and ($p | container("toolbox") | .image == "busybox:1.36" and .command == ["sh","-c","sleep 86400"])'
  evaluate --argjson f "$(yaml_json /root/workshop-busybox.yaml)" --argjson p "$(get pod toolbox-rest -n default)"
  ;;
q21)
  diagnostics='[requirement("Pod must be Running and Ready; retry after startup if still settling"; $p | running),
    requirement("Redis container must use redis:7.4 with exec ls liveness probe and initialDelaySeconds 8"; any($p.spec.containers[]?; .image == "redis:7.4" and .livenessProbe.exec.command == ["ls"] and .livenessProbe.initialDelaySeconds == 8))]'
  expression='($p | running) and any($p.spec.containers[]?; .image == "redis:7.4" and
    .livenessProbe.exec.command == ["ls"] and .livenessProbe.initialDelaySeconds == 8)'
  evaluate --argjson p "$(get pod redis-watch -n default)"
  ;;
*) echo 'Usage: capture.sh q1..q21' >&2; exit 2 ;;
esac
