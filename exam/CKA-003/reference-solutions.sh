#!/usr/bin/env bash
# Run only as the candidate on a newly created, marked disposable CKA-003 lab.
set -Eeuo pipefail
kubectl get namespace cka003-system -o json |
  jq -e '.metadata.labels["painkiller-shell/exam"] == "CKA-003"' >/dev/null ||
  { echo 'Refusing to modify a cluster without the CKA-003 marker' >&2; exit 1; }

kubectl -n default autoscale deployment quay-frontend --cpu=75% --min=3 --max=9
kubectl patch clusterrole namespace-observer-role --type=merge \
  -p '{"rules":[{"apiGroups":[""],"resources":["namespaces"],"verbs":["get"]}]}'
kubectl -n catalog-repair patch resourcequota catalog-quota --type=merge \
  -p '{"spec":{"hard":{"pods":"3"}}}'
kubectl -n catalog-repair wait resourcequota/catalog-quota \
  --for=jsonpath='{.status.hard.pods}'=3 --timeout=60s
# A long-standing admission failure may have a backed-off ReplicaSet queue.
kubectl -n catalog-repair annotate replicaset -l app=catalog cka003/retry="$(date +%s)" --overwrite
kubectl -n catalog-repair wait deployment/catalog-api \
  --for=jsonpath='{.status.readyReplicas}'=3 --timeout=180s
kubectl -n default patch pvc cedar-pvc --type=merge \
  -p '{"spec":{"resources":{"requests":{"storage":"96Mi"}}}}'
kubectl -n default wait pvc/cedar-pvc --for=jsonpath='{.status.capacity.storage}'=96Mi --timeout=180s

sed -i 's/replicaCount: 1/replicaCount: 4/; s/type: ClusterIP/type: NodePort/' /opt/portal-chart/values.yaml
sed -i 's/appVersion: "broken"/appVersion: "1.21.6"/' /opt/portal-chart/Chart.yaml
sed -i 's/app: incorrect-selector/app: {{ .Release.Name }}/' /opt/portal-chart/templates/*.yaml
helm upgrade --install harbor-portal /opt/portal-chart -n portal-lab --create-namespace --wait --timeout 180s
kubectl apply -f - <<'YAML'
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: {name: portal-route, namespace: gateway-lab}
spec:
  parentRefs: [{name: portal-gateway}]
  hostnames: [practice-controller]
  rules:
    - backendRefs: [{name: portal-service, port: 80}]
YAML
kubectl -n default create serviceaccount release-reader
kubectl create clusterrole release-reader-role --verb=get --resource=deployments.apps
kubectl create clusterrolebinding release-reader-binding \
  --clusterrole=release-reader-role --serviceaccount=default:release-reader
kubectl -n default run dns-web --image=nginx
kubectl -n default expose pod dns-web --name=dns-web-service --port=80
kubectl -n default wait pod/dns-web --for=condition=Ready --timeout=180s
sudo -n mkdir -p /root/CKA
kubectl -n default run lookup-service --image=busybox:1.28 --rm -i --restart=Never -- \
  nslookup dns-web-service.default.svc.cluster.local </dev/null |
  sudo -n tee /root/CKA/service-dns.txt
ip=$(kubectl -n default get pod dns-web -o jsonpath='{.status.podIP}')
kubectl -n default run lookup-pod --image=busybox:1.28 --rm -i --restart=Never -- \
  nslookup "${ip//./-}.default.pod.cluster.local" </dev/null |
  sudo -n tee /root/CKA/pod-dns.txt

kubectl -n default get pod queued-web -o json |
  jq 'del(.metadata.uid,.metadata.resourceVersion,.metadata.creationTimestamp,.metadata.managedFields,.status,.spec.nodeName) | .spec.nodeSelector={"cka003/placement":"controller"}' > /tmp/queued-web-fixed.json
kubectl -n default delete pod queued-web --wait=true
kubectl apply -f /tmp/queued-web-fixed.json
kubectl -n default wait pod/queued-web --for=condition=Ready --timeout=180s
kubectl -n release-lab rollout undo deployment/inventory-api
kubectl -n release-lab rollout status deployment/inventory-api --timeout=180s
kubectl -n release-lab get deployment inventory-api -o jsonpath='{.spec.template.spec.containers[0].image}' |
  sudo -n tee /root/restored-image.txt
kubectl -n release-lab scale deployment inventory-api --replicas=6
kubectl -n release-lab rollout status deployment/inventory-api --timeout=180s
kubectl -n default get deployment ledger-db -o json |
  jq '.spec.template.spec.containers[0].env |= map(.valueFrom.secretKeyRef.key = ({DB_ROOT_PASSWORD:"admin-password",DB_DATABASE:"db-name",DB_USER:"db-user",DB_PASSWORD:"db-password"}[.name]))' |
  kubectl apply -f -
kubectl -n default rollout status deployment/ledger-db --timeout=180s

kubectl -n default create deployment color-stream --image=kodekloud/webapp-color:v1 --replicas=4
kubectl -n default patch deployment color-stream --type=merge \
  -p '{"spec":{"strategy":{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":"30%","maxSurge":"60%"}}}}'
kubectl -n default rollout status deployment/color-stream --timeout=180s
kubectl -n default set image deployment/color-stream webapp-color=kodekloud/webapp-color:v2
kubectl -n default rollout status deployment/color-stream --timeout=180s
kubectl -n default get deployment color-stream -o json |
  jq -r '.metadata.annotations["deployment.kubernetes.io/revision"]' | sudo -n tee /opt/rollout-revision.txt
kubectl -n default rollout undo deployment/color-stream
kubectl -n default rollout status deployment/color-stream --timeout=180s
kubectl apply -f - <<'YAML'
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: portal-ingress
  namespace: default
  annotations: {nginx.ingress.kubernetes.io/ssl-redirect: "false"}
spec:
  ingressClassName: harbor-http
  rules:
    - http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service: {name: site-service, port: {number: 80}}
YAML
kubectl -n default rollout resume deployment/reports-api
kubectl -n default rollout status deployment/reports-api --timeout=180s
kubectl -n default patch service status-service --type=merge \
  -p '{"spec":{"ports":[{"port":8080,"targetPort":80}]}}'
kubectl apply -f - <<'YAML'
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata: {name: archive-local}
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
YAML
# Wait for asynchronous routing/status reconciliation before grading.
for q in {1..16}; do
  passed=false
  for _ in {1..30}; do
    if bash /opt/painkiller/cka003/capture.sh "q$q" | jq -e '.ok' >/dev/null; then passed=true; break; fi
    sleep 2
  done
  [[ $passed == true ]] || { echo "Reference answer q$q did not converge" >&2; exit 1; }
done
