#!/usr/bin/env bash
# Run only as the candidate on a newly created, marked disposable CKA-003 lab.
set -Eeuo pipefail
kubectl get namespace cka003-system -o json |
  jq -e '.metadata.labels["painkiller-shell/exam"] == "CKA-003"' >/dev/null ||
  { echo 'Refusing to modify a cluster without the CKA-003 marker' >&2; exit 1; }

kubectl -n default autoscale deployment frontend-deployment --cpu=70% --min=2 --max=10
kubectl patch clusterrole green-role-cka22-arch --type=merge \
  -p '{"rules":[{"apiGroups":[""],"resources":["namespaces"],"verbs":["get"]}]}'
kubectl -n pink-troubleshooting patch resourcequota pink-quota --type=merge \
  -p '{"spec":{"hard":{"pods":"2"}}}'
kubectl -n pink-troubleshooting wait resourcequota/pink-quota \
  --for=jsonpath='{.status.hard.pods}'=2 --timeout=60s
# A long-standing admission failure may have a backed-off ReplicaSet queue.
kubectl -n pink-troubleshooting annotate replicaset -l app=pink cka003/retry="$(date +%s)" --overwrite
kubectl -n pink-troubleshooting wait deployment/pink-depl-cka14-trb \
  --for=jsonpath='{.status.readyReplicas}'=2 --timeout=180s
kubectl -n default patch pvc papaya-pvc-cka09-str --type=merge \
  -p '{"spec":{"resources":{"requests":{"storage":"80Mi"}}}}'
kubectl -n default wait pvc/papaya-pvc-cka09-str --for=jsonpath='{.status.capacity.storage}'=80Mi --timeout=180s

sed -i 's/replicaCount: 1/replicaCount: 3/; s/type: ClusterIP/type: NodePort/' /opt/webapp-color-apd/values.yaml
sed -i 's/appVersion: "broken"/appVersion: "1.20.0"/' /opt/webapp-color-apd/Chart.yaml
sed -i 's/app: incorrect-selector/app: {{ .Release.Name }}/' /opt/webapp-color-apd/templates/*.yaml
helm upgrade --install webapp-color-apd /opt/webapp-color-apd -n frontend-apd --create-namespace --wait --timeout 180s
kubectl apply -f - <<'YAML'
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: {name: web-route, namespace: nginx-gateway}
spec:
  parentRefs: [{name: web-gateway}]
  hostnames: [cka003-controlplane]
  rules:
    - backendRefs: [{name: web-service, port: 80}]
YAML
kubectl -n default create serviceaccount deploy-cka20-arch
kubectl create clusterrole deploy-role-cka20-arch --verb=get --resource=deployments.apps
kubectl create clusterrolebinding deploy-role-binding-cka20-arch \
  --clusterrole=deploy-role-cka20-arch --serviceaccount=default:deploy-cka20-arch
kubectl -n default run nginx-resolver-cka06-svcn --image=nginx
kubectl -n default expose pod nginx-resolver-cka06-svcn --name=nginx-resolver-service-cka06-svcn --port=80
kubectl -n default wait pod/nginx-resolver-cka06-svcn --for=condition=Ready --timeout=180s
sudo -n mkdir -p /root/CKA
kubectl -n default run lookup-service --image=busybox:1.28 --rm -i --restart=Never -- \
  nslookup nginx-resolver-service-cka06-svcn.default.svc.cluster.local </dev/null |
  sudo -n tee /root/CKA/nginx.svc.cka06.svcn
ip=$(kubectl -n default get pod nginx-resolver-cka06-svcn -o jsonpath='{.status.podIP}')
kubectl -n default run lookup-pod --image=busybox:1.28 --rm -i --restart=Never -- \
  nslookup "${ip//./-}.default.pod.cluster.local" </dev/null |
  sudo -n tee /root/CKA/nginx.pod.cka06.svcn

kubectl -n default get pod grey-cka21-trb -o json |
  jq 'del(.metadata.uid,.metadata.resourceVersion,.metadata.creationTimestamp,.metadata.managedFields,.status,.spec.nodeName) | .spec.nodeSelector={"cka003/controlplane":"true"}' > /tmp/grey-fixed.json
kubectl -n default delete pod grey-cka21-trb --wait=true
kubectl apply -f /tmp/grey-fixed.json
kubectl -n default wait pod/grey-cka21-trb --for=condition=Ready --timeout=180s
kubectl -n dev-wl07 rollout undo deployment/rollback-app
kubectl -n dev-wl07 rollout status deployment/rollback-app --timeout=180s
kubectl -n dev-wl07 get deployment rollback-app -o jsonpath='{.spec.template.spec.containers[0].image}' |
  sudo -n tee /root/rolling-back-record.txt
kubectl -n dev-wl07 scale deployment rollback-app --replicas=5
kubectl -n dev-wl07 rollout status deployment/rollback-app --timeout=180s
kubectl -n default get deployment db-deployment-cka05-trb -o json |
  jq '.spec.template.spec.containers[0].env |= map(.valueFrom.secretKeyRef.key = ({DB_ROOT_PASSWORD:"root-password",DB_DATABASE:"database",DB_USER:"username",DB_PASSWORD:"password"}[.name]))' |
  kubectl apply -f -
kubectl -n default rollout status deployment/db-deployment-cka05-trb --timeout=180s

kubectl -n default create deployment ocean-tv-wl09 --image=kodekloud/webapp-color:v1 --replicas=3
kubectl -n default patch deployment ocean-tv-wl09 --type=merge \
  -p '{"spec":{"strategy":{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":"40%","maxSurge":"55%"}}}}'
kubectl -n default rollout status deployment/ocean-tv-wl09 --timeout=180s
kubectl -n default set image deployment/ocean-tv-wl09 webapp-color=kodekloud/webapp-color:v2
kubectl -n default rollout status deployment/ocean-tv-wl09 --timeout=180s
kubectl -n default get deployment ocean-tv-wl09 -o json |
  jq -r '.metadata.annotations["deployment.kubernetes.io/revision"]' | sudo -n tee /opt/revision-count.txt
kubectl -n default rollout undo deployment/ocean-tv-wl09
kubectl -n default rollout status deployment/ocean-tv-wl09 --timeout=180s
kubectl apply -f - <<'YAML'
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: nginx-ingress-cka04-svcn
  namespace: default
  annotations: {nginx.ingress.kubernetes.io/ssl-redirect: "false"}
spec:
  ingressClassName: nginx-cka04
  rules:
    - http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service: {name: nginx-service-cka04-svcn, port: {number: 80}}
YAML
kubectl -n default rollout resume deployment/black-cka25-trb
kubectl -n default rollout status deployment/black-cka25-trb --timeout=180s
kubectl -n default patch service purple-svc-cka27-trb --type=merge \
  -p '{"spec":{"ports":[{"port":80,"targetPort":80}]}}'
kubectl apply -f - <<'YAML'
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata: {name: banana-sc-cka08-str}
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
