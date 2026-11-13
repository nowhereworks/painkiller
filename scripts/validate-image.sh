#!/usr/bin/env bash
# Run on a lab guest through pinned SSH; this never installs or pulls anything.
set -Eeuo pipefail
version=$1 revision=$2
[[ $(cat /opt/painkiller/image-version) == "painkiller-image-v1 $version $revision flannel-0.28.4" ]]
for p in kubelet kubeadm kubectl; do
  installed=$(dpkg-query -W -f='${Version}' "$p")
  [[ $installed == "$version"-* ]]
done
actual=$(dpkg-query -W -f='${Package}=${Version}\n' kubelet kubeadm kubectl containerd cri-tools jq etcd-client)
[[ $actual == "$(cat /opt/painkiller/packages)" ]]
sudo -n sha256sum -c /opt/painkiller/flannel.sha256
command -v jq >/dev/null
command -v etcdctl >/dev/null
images=$(kubeadm config images list --kubernetes-version="$version")
for image in $images $(grep -E '^[[:space:]]*image:' /opt/painkiller/flannel.yml | awk '{print $2}' | sort -u) docker.io/library/nginx:1.27 docker.io/library/nginx:1.26 docker.io/library/busybox:1.36; do
  sudo -n crictl inspecti "$image" >/dev/null
done
sandbox_image=$(cat /opt/painkiller/sandbox-image)
[[ $sandbox_image == registry.k8s.io/pause:* && $sandbox_image != *$'\n'* ]]
sudo -n crictl inspecti "$sandbox_image" >/dev/null
