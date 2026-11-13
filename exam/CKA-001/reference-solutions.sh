#!/usr/bin/env bash
# Apply the reference answers to a disposable, freshly initialized exam cluster.
set -euo pipefail
kubectl get namespace exam-repair -o json | jq -e '.metadata.labels["painkiller-shell/exam"] == "true"' >/dev/null || { echo 'Refusing to modify a cluster without the Painkiller exam marker' >&2; exit 1; }

control=$(kubectl get nodes -o json | jq -r '[.items[] | select(.metadata.labels["node-role.kubernetes.io/control-plane"] != null)][0].metadata.name')
worker=$(kubectl get nodes -o json | jq -r '[.items[] | select(.metadata.labels["node-role.kubernetes.io/control-plane"] == null)][0].metadata.name')

# Cluster architecture: namespace, label, static Pod and etcd backup.
kubectl create namespace exam-sandbox --dry-run=client -o yaml | kubectl apply -f -
kubectl label node "$control" exam=ready --overwrite
worker_ip=$(kubectl get node "$worker" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}')
ssh "$worker_ip" 'sudo tee /etc/kubernetes/manifests/static-web.yaml >/dev/null' <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: static-web
spec:
  containers:
    - name: web
      image: nginx:1.27
YAML
sudo -n mkdir -p /opt/backup
sudo -n env ETCDCTL_API=3 etcdctl --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  snapshot save /opt/backup/etcd.db

# Workloads and scheduling.
kubectl -n exam-workloads run web --image=nginx:1.27
kubectl -n exam-workloads create deployment api --image=nginx:1.27 --replicas=2
kubectl -n exam-workloads set image deployment/rollout web=nginx:1.26
kubectl -n exam-workloads rollout status deployment/rollout --timeout=180s
kubectl -n exam-workloads rollout undo deployment/rollout
kubectl -n exam-workloads rollout status deployment/rollout --timeout=180s
kubectl apply -f - <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: pipeline
  namespace: exam-workloads
spec:
  initContainers:
    - name: prepare
      image: busybox:1.36
      command: ["sh", "-c", "sleep 2"]
  containers:
    - name: app
      image: nginx:1.27
    - name: log-sidecar
      image: busybox:1.36
      command: ["sh", "-c", "tail -f /dev/null"]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: resource-demo
  namespace: exam-workloads
spec:
  replicas: 1
  selector:
    matchLabels:
      app: resource-demo
  template:
    metadata:
      labels:
        app: resource-demo
    spec:
      containers:
        - name: web
          image: nginx:1.27
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 256Mi
YAML
kubectl -n exam-workloads create quota limit-pods --hard=pods=20
kubectl apply -f - <<'YAML'
apiVersion: v1
kind: LimitRange
metadata:
  name: default-limits
  namespace: exam-workloads
spec:
  limits:
    - type: Container
      default:
        cpu: 200m
        memory: 256Mi
      defaultRequest:
        cpu: 100m
        memory: 128Mi
YAML
kubectl create priorityclass exam-critical --value=100000
kubectl -n exam-workloads create job hello --image=busybox:1.36 -- echo hello
kubectl -n exam-workloads create cronjob periodic --image=busybox:1.36 --schedule='*/5 * * * *' -- echo hello
kubectl -n exam-workloads create configmap settings --from-literal=mode=production
kubectl -n exam-workloads create secret generic credentials --from-literal=username=operator
kubectl -n exam-workloads create serviceaccount observer

# Storage.
kubectl apply -f - <<'YAML'
apiVersion: v1
kind: PersistentVolume
metadata:
  name: exam-pv
spec:
  capacity:
    storage: 1Gi
  accessModes: [ReadWriteOnce]
  storageClassName: exam-local
  hostPath:
    path: /tmp/exam-data
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: exam-claim
  namespace: exam-workloads
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: exam-local
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: Pod
metadata:
  name: config-mount
  namespace: exam-workloads
spec:
  containers:
    - name: web
      image: nginx:1.27
      volumeMounts:
        - name: config
          mountPath: /etc/exam
  volumes:
    - name: config
      configMap:
        name: exam-material
YAML

# Services and networking.
kubectl -n exam-workloads expose deployment service-target --name=api --port=80 --target-port=80
kubectl -n exam-workloads expose deployment service-target --name=web-nodeport --type=NodePort --port=80 --target-port=80
kubectl apply -f - <<'YAML'
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: exam-workloads
spec:
  rules:
    - host: exam.local
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: exam-backend
                port:
                  number: 80
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-ingress
  namespace: exam-workloads
spec:
  podSelector: {}
  policyTypes: [Ingress]
YAML
kubectl -n kube-system scale deployment/coredns --replicas=3
kubectl -n kube-system rollout status deployment/coredns --timeout=180s

# Troubleshooting.
kubectl -n exam-repair set image deployment/broken-web web=nginx:1.27
kubectl -n exam-repair rollout status deployment/broken-web --timeout=180s
kubectl -n exam-repair patch service broken-endpoints --type=merge -p '{"spec":{"selector":{"app":"endpoint-target"}}}'
kubectl -n exam-repair patch deployment crashing --type=json -p '[{"op":"replace","path":"/spec/template/spec/containers/0/command","value":["sh","-c","sleep 3600"]}]'
kubectl -n exam-repair rollout status deployment/crashing --timeout=180s

# RBAC.
kubectl -n exam-workloads create role pod-reader --verb=get,list,watch --resource=pods
kubectl -n exam-workloads create rolebinding read-pods --role=pod-reader --serviceaccount=exam-workloads:binding-observer
kubectl create clusterrole node-reader --verb=get,list,watch --resource=nodes
kubectl create clusterrolebinding node-reader --clusterrole=node-reader --serviceaccount=exam-workloads:cluster-observer

# Scheduling constraints are applied last so earlier Pods are unaffected.
kubectl taint node "$worker" exam=reserved:NoSchedule --overwrite
kubectl cordon "$worker"
