#!/usr/bin/env bash
# Apply the reference answers to a disposable, freshly initialized exam cluster.
set -euo pipefail
kubectl get namespace exam-repair -o json | jq -e '.metadata.labels["painkiller-shell/exam"] == "true"' >/dev/null || { echo 'Refusing to modify a cluster without the Painkiller exam marker' >&2; exit 1; }
kubectl create namespace exam-sandbox
kubectl -n exam-workloads run web --image=nginx:1.27
kubectl -n exam-workloads create deployment api --image=nginx:1.27 --replicas=2
kubectl -n exam-workloads expose deployment service-target --name=api --port=80 --target-port=80
kubectl -n exam-workloads create configmap settings --from-literal=mode=production
kubectl -n exam-workloads create secret generic credentials --from-literal=username=operator
kubectl -n exam-workloads create serviceaccount observer
kubectl -n exam-workloads create quota limit-pods --hard=pods=10
kubectl -n exam-workloads create job hello --image=busybox:1.36 -- echo hello
node=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
kubectl label node "$node" exam=ready --overwrite
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
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-ingress
  namespace: exam-workloads
spec:
  podSelector: {}
  policyTypes: [Ingress]
YAML
kubectl -n exam-workloads create role pod-reader --verb=get,list,watch --resource=pods
kubectl -n exam-workloads create rolebinding read-pods --role=binding-reader --serviceaccount=exam-workloads:binding-observer
kubectl -n exam-repair set image deployment/broken-web web=nginx:1.27
kubectl -n exam-repair rollout status deployment/broken-web --timeout=180s
kubectl -n exam-workloads create cronjob periodic --image=busybox:1.36 --schedule='*/5 * * * *' -- echo hello
