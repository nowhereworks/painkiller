#!/usr/bin/env bash
# Only the selected attempt's ownership-checked guests are accessed.
# shellcheck disable=SC2016
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
assets=$root/exam/CKA-003/assets
known=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}
user=${LAB_SSH_USER:-milkman}
node() {
  local ip=$1 command
  shift
  printf -v command '%q' "$*"
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes \
    -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" "$user@$ip" \
    "bash -e -o pipefail -c $command"
}
case "${1:-}" in
preflight)
  (cd "$assets" && sha256sum --check --quiet SHA256SUMS)
  exit 0
  ;;
infrastructure|fixtures) ;;
*) echo "Usage: $0 {preflight|infrastructure|fixtures}" >&2; exit 2 ;;
esac
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",")[0]' <<<"$state")
w1=$(jq -er '.worker_ips | split(",")[0]' <<<"$state")
mapfile -t ips < <(jq -r '[.controller_ips,.worker_ips] | .[] | split(",")[] | select(length>0)' <<<"$state")
marker=/var/lib/painkiller/cka003
if [[ $1 == infrastructure ]]; then
  for ip in "${ips[@]}"; do
    node "$ip" "if ! sudo -n test -e $marker/packages || ! python3 -c 'import yaml' 2>/dev/null; then
      sudo -n apt-get update -qq
      sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nfs-common python3-yaml
      sudo -n mkdir -p $marker
      sudo -n touch $marker/packages
    fi"
  done
  node "$w1" 'sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nfs-kernel-server
    sudo -n mkdir -p /srv/cka003/papaya /etc/exports.d
    sudo -n touch /etc/exports
    sudo -n chmod 0777 /srv/cka003/papaya'
  {
    printf '/srv/cka003'
    for ip in "${ips[@]}"; do printf ' %s(rw,sync,no_subtree_check,no_root_squash)' "$ip"; done
    printf '\n'
  } | node "$w1" 'sudo -n tee /etc/exports.d/cka003.exports >/dev/null
    sudo -n exportfs -ra
    sudo -n systemctl enable --now nfs-server'
  node "$cp" 'if ! command -v helm >/dev/null; then
    d=$(mktemp -d)
    cd "$d"
    curl -fsSLO https://get.helm.sh/helm-v3.19.0-linux-amd64.tar.gz
    curl -fsSLO https://get.helm.sh/helm-v3.19.0-linux-amd64.tar.gz.sha256sum
    sha256sum -c helm-v3.19.0-linux-amd64.tar.gz.sha256sum
    tar -xzf helm-v3.19.0-linux-amd64.tar.gz
    sudo -n install linux-amd64/helm /usr/local/bin/helm
  fi'
  exit 0
fi
if node "$cp" "sudo -n test -e $marker/complete"; then
  echo 'CKA-003 already prepared; preserving candidate work'
  exit 0
fi
kube() { node "$cp" "kubectl $*"; }
apply() { node "$cp" 'kubectl apply --server-side --force-conflicts -f -' < "$1"; }
# Copy only trusted capture logic and the deliberately broken candidate chart.
node "$cp" 'sudo -n mkdir -p /opt/painkiller/cka003 /opt/webapp-color-apd
  sudo -n chown "$(id -un):$(id -gn)" /opt/painkiller/cka003 /opt/webapp-color-apd'
node "$cp" 'cat > /opt/painkiller/cka003/capture.sh' < "$assets/capture.sh"
tar -C "$assets/chart" -cf - . | node "$cp" 'tar -C /opt/webapp-color-apd -xf -'
apply "$assets/gateway-api.yaml"
apply "$assets/metrics-server.yaml"
kube patch deployment metrics-server -n kube-system --type=json \
  "-p='[{\"op\":\"add\",\"path\":\"/spec/template/spec/containers/0/args/-\",\"value\":\"--kubelet-insecure-tls\"}]'"
for file in rbac-csi-nfs csi-nfs-driverinfo csi-nfs-controller csi-nfs-node; do apply "$assets/$file.yaml"; done
# No snapshot task: do not run a snapshot controller or require snapshot CRDs.
kube patch deployment csi-nfs-controller -n kube-system --type=strategic \
  "-p='{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"csi-snapshotter\",\"\$patch\":\"delete\"}]}}}}'"
# Establish fixtures once. Each subsequent fault/history phase has its own marker.
if ! node "$cp" "sudo -n test -e $marker/fixtures"; then
  apply "$root/exam/CKA-003/setup.yaml"
  node "$cp" "sudo -n touch $marker/fixtures"
fi
apply "$assets/routing.yaml"
control=$(kube get nodes -l node-role.kubernetes.io/control-plane -o 'jsonpath={.items[0].metadata.name}')
kube label node "$control" cka003/controlplane=true --overwrite
node "$cp" "if ! grep -q ' cka003-controlplane$' /etc/hosts; then
  echo '$cp cka003-controlplane' | sudo -n tee -a /etc/hosts >/dev/null
fi"
if ! node "$cp" "sudo -n test -e $marker/storage"; then
  node "$cp" "kubectl apply -f -" <<YAML
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata: {name: papaya-stc-cka09-str}
provisioner: nfs.csi.k8s.io
allowVolumeExpansion: true
volumeBindingMode: Immediate
parameters: {server: "$w1", share: /srv/cka003}
---
apiVersion: v1
kind: PersistentVolume
metadata: {name: papaya-pv-cka09-str}
spec:
  capacity: {storage: 50Mi}
  accessModes: [ReadWriteMany]
  persistentVolumeReclaimPolicy: Retain
  storageClassName: papaya-stc-cka09-str
  mountOptions: [nfsvers=4.1]
  csi:
    driver: nfs.csi.k8s.io
    volumeHandle: "$w1#/srv/cka003#papaya"
    volumeAttributes: {server: "$w1", share: /srv/cka003/papaya}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: papaya-pvc-cka09-str, namespace: default}
spec:
  accessModes: [ReadWriteMany]
  storageClassName: papaya-stc-cka09-str
  volumeName: papaya-pv-cka09-str
  resources: {requests: {storage: 50Mi}}
YAML
  node "$cp" 'kubectl apply -f -' <<'YAML'
apiVersion: v1
kind: Pod
metadata: {name: papaya-consumer, namespace: default}
spec:
  containers:
    - name: storage
      image: busybox:1.37
      command: [sh, -c, "echo cka003-storage > /data/ready; exec sleep 2147483647"]
      volumeMounts: [{name: data, mountPath: /data}]
  volumes:
    - name: data
      persistentVolumeClaim: {claimName: papaya-pvc-cka09-str}
YAML
  node "$cp" "sudo -n touch $marker/storage"
fi
for target in 'kube-system metrics-server' 'kube-system csi-nfs-controller' \
  'cka003-system routing' 'default frontend-deployment' 'default nginx-deployment-cka04-svcn' \
  'nginx-gateway web-backend'; do
  read -r ns dep <<<"$target"
  kube rollout status deployment/"$dep" -n "$ns" --timeout=240s
done
kube rollout status daemonset/csi-nfs-node -n kube-system --timeout=240s
kube wait pod/papaya-consumer -n default --for=condition=Ready --timeout=240s
if ! node "$cp" "sudo -n test -e $marker/rollback"; then
  kube rollout status deployment/rollback-app -n dev-wl07 --timeout=180s
  kube set image deployment/rollback-app -n dev-wl07 web=nginx:cka003-invalid
  node "$cp" "sudo -n touch $marker/rollback"
fi
if ! node "$cp" "sudo -n test -e $marker/black"; then
  kube rollout status deployment/black-cka25-trb --timeout=180s
  kube rollout pause deployment/black-cka25-trb
  kube patch deployment black-cka25-trb --type=merge \
    "-p='{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"cka003/revision\":\"pending\"}}}}}'"
  node "$cp" "sudo -n touch $marker/black"
fi
# Protect trusted grading logic from accidental candidate edits (sudo remains available).
node "$cp" 'sudo -n chown -R root:root /opt/painkiller/cka003
  sudo -n chmod -R go-w /opt/painkiller/cka003'
node "$cp" "sudo -n touch $marker/complete"
echo 'CKA-003 fixtures and isolated faults prepared'
