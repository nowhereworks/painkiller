#!/usr/bin/env bash
# Run on a lab guest through pinned SSH; this never installs or pulls anything.
# Validates the prebaked three-controller Calico HA image (haproxy
# preinstalled, calico manifest/images cached). Mirrors validate-image.sh.
set -Eeuo pipefail
version=$1
for p in kubelet kubeadm kubectl; do
  installed=$(dpkg-query -W -f='${Version}' "$p")
  [[ $installed == "$version"-* ]]
done
packages=(kubelet kubeadm kubectl containerd cri-tools jq etcd-client haproxy)
# Older HA images also recorded Keepalived; retain validation compatibility.
if grep -q '^keepalived=' /opt/painkiller/packages; then packages+=(keepalived); fi
actual=$(dpkg-query -W -f='${Package}=${Version}\n' "${packages[@]}")
[[ $actual == "$(cat /opt/painkiller/packages)" ]]
sudo -n sha256sum -c /opt/painkiller/calico.sha256
command -v jq >/dev/null
command -v etcdctl >/dev/null
command -v haproxy >/dev/null
images=$(kubeadm config images list --kubernetes-version="$version")
for image in $images $(grep -E '^[[:space:]]*image:' /opt/painkiller/calico.yml | awk '{print $2}' | sort -u) docker.io/library/nginx:1.27 docker.io/library/busybox:1.36; do
  sudo -n crictl inspecti "$image" >/dev/null
done
sandbox_image=$(cat /opt/painkiller/sandbox-image)
[[ $sandbox_image == registry.k8s.io/pause:* && $sandbox_image != *$'\n'* ]]
sudo -n crictl inspecti "$sandbox_image" >/dev/null
