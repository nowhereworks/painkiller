#!/usr/bin/env bash
# Runs only inside the disposable OPA-001 guest.
set -Eeuo pipefail
[[ $(uname -m) == x86_64 ]]
sudo -n cloud-init status --wait >/dev/null || [[ $? == 2 ]]
sudo -n apt-get -o DPkg::Lock::Timeout=300 update -qq
sudo -n env DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 install -y -qq curl jq python3-yaml
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
fetch() {
  curl --retry 3 --connect-timeout 10 --max-time 180 -fsSL "$1" -o "$2"
  echo "$3  $2" | sha256sum -c -
}
fetch https://github.com/open-policy-agent/opa/releases/download/v1.22.2/opa_linux_amd64_static opa \
  24e344d1fc394cd93dc75666f584cfb97c6981f7255af25f8d2979834317317b
fetch https://github.com/open-policy-agent/conftest/releases/download/v0.71.1/conftest_0.71.1_Linux_x86_64.tar.gz conftest.tar.gz \
  c3f6b2a753bd56e377a1e51a95ae3e057f926f2e9139f7d5b5dfbbaf810f7985
fetch https://github.com/open-policy-agent/regal/releases/download/v0.43.0/regal_Linux_x86_64 regal \
  5e7d030064e307d54e6ffa3d85b8d9052d3d40b7afa10262c86c886f8a76559d
tar -xzf conftest.tar.gz conftest
sudo -n install -m 0755 opa conftest regal /usr/local/bin/
opa version | grep -Fx 'Version: 1.22.2'
conftest --version | grep -Fx 'Conftest: 0.71.1'
regal version | grep '0.43.0'
