#!/usr/bin/env bash
# Trusted loss/restore mutations for the disposable live validation only.
set -Eeuo pipefail
[[ $# == 2 && $1 =~ ^(remove|restore)$ && $2 =~ ^q([1-9]|1[0-9]|2[01])$ ]] || exit 2
kubectl --request-timeout=5s get namespace ckad001-system -o json |
  jq -e '.metadata.labels["painkiller-shell/exam"] == "CKAD-001"' >/dev/null
action=$1 q=$2
saved=${TMPDIR:-/tmp}/painkiller-ckad001-mutation
mkdir -p "$saved"
pod() {
  local ns=$1 name=$2
  if [[ $action == remove ]]; then
    kubectl -n "$ns" get pod "$name" -o json |
      jq 'del(.metadata.uid,.metadata.resourceVersion,.metadata.creationTimestamp,.metadata.managedFields,.status)' > "$saved/$q.json"
    kubectl -n "$ns" delete pod "$name" --wait=true --grace-period=1
  else
    kubectl create -f "$saved/$q.json"
  fi
}
value() { if [[ $action == remove ]]; then echo "$1"; else echo "$2"; fi; }
case "$q" in
q1) pod container-lab trio-runner ;;
q2) kubectl patch storageclass cedar-static --type=merge -p "{\"allowVolumeExpansion\":$(value false true)}" ;;
q3) kubectl -n batch-lab patch cronjob process-report --type=merge -p "{\"spec\":{\"schedule\":\"$(value '*/10 * * * *' '*/20 * * * *')\"}}" ;;
q4) pod pod-workshop ubuntu-rest ;;
q5) kubectl scale deployment azure-web --replicas="$(value 5 6)"; kubectl rollout status deployment/azure-web --timeout=180s ;;
q6) kubectl -n dev-workshop scale deployment bulletin-web --replicas="$(value 0 2)"; kubectl -n dev-workshop rollout status deployment/bulletin-web --timeout=180s ;;
q7) kubectl -n frontend-workshop patch service lantern-web --type=merge -p "{\"spec\":{\"type\":\"$(value ClusterIP NodePort)\"}}" ;;
q8) kubectl patch service parcel-entry --type=merge -p "{\"spec\":{\"ports\":[{\"port\":6445,\"targetPort\":$(value 81 80)}]}}" ;;
q9) kubectl -n sealed-workshop patch networkpolicy isolate-everything --type=merge -p "{\"spec\":{\"policyTypes\":$(value '["Ingress"]' '["Ingress","Egress"]')}}" ;;
q10) kubectl patch endpointslice host-web-backend --type=merge -p "{\"ports\":[{\"name\":\"http\",\"protocol\":\"TCP\",\"port\":$(value 9997 9998)}]}" ;;
q11) kubectl label pod message-web "tier=$(value wrong messages)" --overwrite ;;
q12) kubectl patch service resolver-alias --type=merge -p "{\"spec\":{\"externalName\":\"$(value example.org dns.google)\"}}" ;;
q13)
  if [[ $action == remove ]]; then
    sudo -n cp /root/qos-report.txt "$saved/qos-report.txt"
    sudo -n truncate -s 0 /root/qos-report.txt
  else sudo -n cp "$saved/qos-report.txt" /root/qos-report.txt; fi ;;
q14) kubectl patch animes.entertainment.ckad001.test evening-series --type=merge -p "{\"spec\":{\"episodes\":$(value 25 26)}}" ;;
q15) kubectl patch configmap workshop-settings --type=merge -p "{\"data\":{\"CACHE_TTL\":\"$(value 181 180)\"}}" ;;
q16)
  encoded=$(printf '%s' "$(value wrong ledger)" | base64 -w0)
  kubectl -n database-workshop patch secret ledger-settings --type=merge -p "{\"data\":{\"DB_User\":\"$encoded\"}}" ;;
q17) kubectl -n quota-workshop patch resourcequota quota-object-cap --type=merge -p "{\"spec\":{\"hard\":{\"count/resourcequotas\":\"$(value 2 1)\"}}}" ;;
q18) pod secret-workshop vault-reader ;;
q19) pod default warmup-web ;;
q20)
  if [[ $action == remove ]]; then
    sudo -n sed -i 's/apiVersion: v1/apiVersion: apps\/v1/' /root/workshop-busybox.yaml
  else sudo -n sed -i 's/apiVersion: apps\/v1/apiVersion: v1/' /root/workshop-busybox.yaml; fi ;;
q21) pod default redis-watch ;;
esac
if [[ $action == restore ]]; then
  for _ in {1..90}; do
    if bash /opt/painkiller/ckad001/capture.sh "$q" | jq -e '.ok == true' >/dev/null; then exit 0; fi
    sleep 2
  done
  echo "$q did not recover" >&2
  exit 1
fi
