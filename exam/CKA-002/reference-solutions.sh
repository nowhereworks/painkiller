#!/usr/bin/env bash
# Apply the CKA-002 reference answers to a disposable, freshly prepared exam
# cluster. Refuses any cluster without the CKA-002 marker or a multi-controller
# topology. Intended only for validation on a disposable lab.
set -Eeuo pipefail

kubectl get namespace exam-workloads -o json \
  | jq -e '.metadata.labels["painkiller-shell/exam-id"] == "CKA-002"' >/dev/null \
  || { echo 'Refusing: cluster lacks the CKA-002 marker' >&2; exit 1; }

controllers=$(kubectl get nodes -o json | jq '[.items[] | select(.metadata.labels["node-role.kubernetes.io/control-plane"] != null)] | length')
[[ $controllers -ge 2 ]] || { echo 'Refusing: not a multi-controller CKA-002 cluster' >&2; exit 1; }

worker() { kubectl get nodes -o json | jq -r '[.items[] | select(.metadata.labels["node-role.kubernetes.io/control-plane"] == null)][0].metadata.name'; }

# The unjoined third controller is the lab guest IP not present in the node
# list; all five guest IPs are pinned in the candidate SSH known-hosts file.
third_controller_ip() {
  local joined all
  joined=$(kubectl get nodes -o json | jq -r '[.items[].status.addresses[] | select(.type=="InternalIP") | .address] | unique | .[]')
  all=$(awk '!/^#/ {print $1}' "$HOME/.ssh/painkiller_known_hosts" 2>/dev/null)
  comm -23 <(printf '%s\n' "$all" | sort -u) <(printf '%s\n' "$joined" | sort -u) | head -1
}

# 1. Join the third control-plane node (generate a fresh certificate key).
third=$(third_controller_ip)
if [[ -n $third ]]; then
  key=$(sudo -n kubeadm init phase upload-certs --upload-certs 2>/dev/null | tail -1)
  join=$(sudo -n kubeadm token create --print-join-command)
  ssh -o ConnectTimeout=10 "$third" "sudo -n $join --control-plane --certificate-key $key" </dev/null
  kubectl wait --for=condition=Ready nodes --all --timeout=300s || true
fi

# Repair node-level faults first so downstream tasks that require both workers
# (scheduling, drain) run against a healthy cluster.

# 15. Repair the worker kubelet (makes the second worker Ready again).
bad_worker=$(kubectl get nodes -o json | jq -r '[.items[] | select(.metadata.labels["node-role.kubernetes.io/control-plane"] == null and ([.status.conditions[]? | select(.type=="Ready")][0].status != "True"))][0].metadata.name // empty' 2>/dev/null || true)
if [[ -n $bad_worker && $bad_worker != null ]]; then
  bad_ip=$(kubectl get node "$bad_worker" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}')
  ssh -o ConnectTimeout=10 "$bad_ip" 'sudo -n sed -i "/painkiller-fault/d" /var/lib/kubelet/config.yaml && sudo -n systemctl restart kubelet' </dev/null
  kubectl wait --for=condition=Ready node "$bad_worker" --timeout=180s || true
fi

# 16. Repair the broken scheduler static Pod.
bad_sched=$(kubectl -n kube-system get pods -l component=kube-scheduler -o json | jq -r '.items[] | select(any(.spec.containers[]?; (.image | contains("9.9.9")))) | .spec.nodeName' | head -1)
if [[ -n $bad_sched ]]; then
  sched_ip=$(kubectl get node "$bad_sched" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}')
  ver=$(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.kubeletVersion}')
  ssh -o ConnectTimeout=10 "$sched_ip" "sudo -n sed -i \"s#kube-scheduler:.*#kube-scheduler:${ver}#\" /etc/kubernetes/manifests/kube-scheduler.yaml" </dev/null
fi

# 17. Repair CoreDNS and configure the application Pod DNS.
kubectl -n kube-system get cm coredns -o json \
  | jq '.data.Corefile = (.data.Corefile | split("forward . 1.2.3.4") | join("forward . /etc/resolv.conf"))' \
  | kubectl apply -f -
kubectl -n kube-system rollout restart deployment/coredns
kubectl apply -f - <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: dns-app
  namespace: exam-repair
spec:
  containers:
    - name: app
      image: busybox:1.36
      command: ["sh", "-c", "sleep 3600"]
  dnsConfig:
    nameservers:
      - 1.1.1.1
    searches:
      - example.test
YAML

# 2. Safe drain (with evidence) and an etcd snapshot.
w=$(worker)
kubectl cordon "$w"
kubectl drain "$w" --ignore-daemonsets --delete-emptydir-data --timeout=180s 2>&1 | tee /tmp/drain-evidence.txt || true
kubectl uncordon "$w"
sudo -n mkdir -p /opt/backup
sudo -n env ETCDCTL_API=3 etcdctl --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  snapshot save /opt/backup/etcd.db >/dev/null

# 3. Least-privilege RBAC.
kubectl -n exam-rbac apply -f - <<'YAML'
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pod-log-reader
  namespace: exam-rbac
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: read-pod-logs
  namespace: exam-rbac
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: pod-log-reader
subjects:
  - kind: ServiceAccount
    name: auditor
    namespace: exam-rbac
YAML

# 4. Repair the Helm release and deploy the Kustomize overlay.
helm upgrade demo /opt/painkiller/cka002/chart -n exam-helm --set image.tag=1.27
kubectl -n exam-helm rollout status deployment/demo-web --timeout=180s
kubectl apply -k /opt/painkiller/cka002/kustomize/overlay
kubectl -n exam-helm rollout status deployment/kustom-app --timeout=180s

# 5. Install the CRD and operator, then create a Widget.
kubectl apply -f /opt/painkiller/cka002/operator/
kubectl -n exam-ops rollout status deployment/widget-operator --timeout=180s
kubectl -n exam-ops apply -f - <<'YAML'
apiVersion: example.painkiller.io/v1
kind: Widget
metadata:
  name: demo
spec:
  message: hello
YAML

# 6. Recover the rollout with probes and rolling-update constraints.
kubectl -n exam-workloads set image deployment/app app=nginx:1.27
kubectl -n exam-workloads rollout status deployment/app --timeout=180s
kubectl -n exam-workloads patch deployment app --type=strategic -p '{"spec":{"strategy":{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":0,"maxSurge":1}},"template":{"spec":{"containers":[{"name":"app","readinessProbe":{"httpGet":{"path":"/","port":80}},"livenessProbe":{"httpGet":{"path":"/","port":80}}}]}}}}'
kubectl -n exam-workloads rollout status deployment/app --timeout=180s

# 7. Configure the HPA application.
kubectl -n exam-workloads patch deployment hpa-web --type=strategic -p '{"spec":{"template":{"spec":{"containers":[{"name":"web","resources":{"requests":{"cpu":"100m"}},"volumeMounts":[{"name":"config","mountPath":"/etc/config"},{"name":"secret","mountPath":"/etc/secret"}]}],"volumes":[{"name":"config","configMap":{"name":"app-config"}},{"name":"secret","secret":{"secretName":"app-secret"}}]}}}}'
kubectl -n exam-workloads rollout status deployment/hpa-web --timeout=180s
kubectl -n exam-workloads autoscale deployment hpa-web --min=1 --max=5 --cpu=50% --dry-run=client -o yaml | kubectl apply -f -

# 8. Pinned scheduling with toleration, affinity, spread, and a quota.
kubectl -n exam-workloads create quota exam-quota --hard=pods=10 --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f - <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pinned
  namespace: exam-workloads
spec:
  replicas: 3
  selector:
    matchLabels:
      app: pinned
  template:
    metadata:
      labels:
        app: pinned
    spec:
      tolerations:
        - key: exam
          value: reserved
          effect: NoSchedule
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: topology.kubernetes.io/zone
                    operator: In
                    values: [west, east]
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: DoNotSchedule
          labelSelector:
            matchLabels:
              app: pinned
      containers:
        - name: web
          image: nginx:1.27
YAML
kubectl -n exam-workloads rollout status deployment/pinned --timeout=180s

# 9. NetworkPolicy isolation.
kubectl apply -f - <<'YAML'
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-web
  namespace: exam-netpol
spec:
  podSelector:
    matchLabels:
      app: db
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: web
      ports:
        - port: 80
YAML

# 10. Gateway API with a cross-namespace backend and ReferenceGrant.
kubectl apply -f - <<'YAML'
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: eg
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: exam
  namespace: exam-gw
spec:
  gatewayClassName: eg
  listeners:
    - name: http
      protocol: HTTP
      port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: ReferenceGrant
metadata:
  name: allow-exam-gw
  namespace: exam-gw-backend
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: exam-gw
  to:
    - group: ""
      kind: Service
      name: backend
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: exam
  namespace: exam-gw
spec:
  parentRefs:
    - name: exam
      namespace: exam-gw
  hostnames:
    - gw.example.com
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: backend
          namespace: exam-gw-backend
          port: 80
YAML

# 11. Ingress and NodePort exposure.
kubectl apply -f - <<'YAML'
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: exam-ingress
spec:
  ingressClassName: traefik
  rules:
    - host: ing.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 80
---
apiVersion: v1
kind: Service
metadata:
  name: web-np
  namespace: exam-ingress
spec:
  type: NodePort
  selector:
    app: web
  ports:
    - port: 80
      targetPort: 80
YAML

# 12. Repair the LoadBalancer allocation.
kubectl -n exam-lb annotate service lb-app metallb.universe.tf/loadBalancerIPs-

# 13. Dynamic storage with delayed binding.
kubectl apply -f - <<'YAML'
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: exam-sc
provisioner: rancher.io/local-path
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Retain
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: exam-pvc
  namespace: exam-storage
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: exam-sc
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: Pod
metadata:
  name: data-app
  namespace: exam-storage
spec:
  containers:
    - name: web
      image: nginx:1.27
      volumeMounts:
        - name: data
          mountPath: /data
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: exam-pvc
YAML

# 14. Recover the static local volume binding.
# The PVC's storageClassName and the PV's nodeAffinity are immutable, so recreate
# both. The data lives on the node's host path and is retained by the PV's
# ReclaimPolicy=Retain, so deleting the PV object does not lose it.
local_worker=$(worker)
kubectl -n exam-storage delete pvc exam-local-pvc --ignore-not-found
kubectl delete pv exam-local-pv --ignore-not-found
kubectl apply -f - <<YAML
apiVersion: v1
kind: PersistentVolume
metadata:
  name: exam-local-pv
spec:
  capacity:
    storage: 1Gi
  accessModes: [ReadWriteOnce]
  persistentVolumeReclaimPolicy: Retain
  storageClassName: exam-local
  local:
    path: /opt/painkiller/local-data
  nodeAffinity:
    required:
      nodeSelectorTerms:
        - matchExpressions:
            - key: kubernetes.io/hostname
              operator: In
              values: [$local_worker]
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: exam-local-pvc
  namespace: exam-storage
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: exam-local
  resources:
    requests:
      storage: 1Gi
YAML
kubectl apply -f - <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: local-app
  namespace: exam-storage
spec:
  containers:
    - name: web
      image: busybox:1.36
      command: ["sh", "-c", "sleep 3600"]
      volumeMounts:
        - name: data
          mountPath: /mnt
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: exam-local-pvc
YAML

# 18. Repair the Service selector and targetPort.
kubectl -n exam-repair patch service backend-svc --type=json -p '[{"op":"replace","path":"/spec/selector","value":{"app":"backend"}},{"op":"replace","path":"/spec/ports/0/targetPort","value":8080}]'

# 19. Fix the OOM-killed workload and save evidence.
kubectl -n exam-repair describe pod -l app=oom-app > /tmp/oom-evidence.txt || true
kubectl -n exam-repair patch deployment oom-app --type=json -p '[{"op":"replace","path":"/spec/template/spec/containers/0/resources/limits/memory","value":"256Mi"}]'
kubectl -n exam-repair rollout status deployment/oom-app --timeout=180s

# 20. Recover the multi-container crash and save previous logs.
kubectl -n exam-repair logs -l app=crash-app --all-containers --previous > /tmp/crash-logs.txt 2>/dev/null || true
kubectl -n exam-repair patch deployment crash-app --type=json -p '[{"op":"replace","path":"/spec/template/spec/containers/0/command","value":["sh","-c","sleep 3600"]},{"op":"replace","path":"/spec/template/spec/containers/1/command","value":["sh","-c","sleep 3600"]}]'
kubectl -n exam-repair rollout status deployment/crash-app --timeout=180s

echo 'CKA-002 reference solutions applied.'
