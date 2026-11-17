#!/usr/bin/env bash
# Dependencies only, installed in the new guest, never in template 9199.
set -Eeuo pipefail
sudo -n cloud-init status --wait >/dev/null || [[ $? == 2 ]]
if sudo -n test -e /var/lib/painkiller/cks001/packages; then exit 0; fi
sudo -n apt-get -o DPkg::Lock::Timeout=300 update -qq
sudo -n env DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 install -y -qq \
  apparmor apparmor-utils curl jq python3-yaml openssl skopeo libelf1
[[ $(uname -m) == x86_64 ]]
sudo -n test -r /sys/kernel/btf/vmlinux
[[ $(cat /sys/module/apparmor/parameters/enabled) == Y ]]
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
fetch() {
  curl --retry 3 --connect-timeout 10 --max-time 180 -fsSL "$1" -o "$2"
  echo "$3  $2" | sha256sum -c -
}
fetch https://github.com/kubernetes-sigs/bom/releases/download/v0.8.0/bom-amd64-linux bom \
  cafd00b75a8df860fec54d19b9e24fb48556c0848d0abf9e40cd2eabb11a8710
fetch https://github.com/aquasecurity/kube-bench/releases/download/v0.16.0/kube-bench_0.16.0_linux_amd64.tar.gz bench.tgz \
  82dbc7e598740dc9344d41f8ad0b8210d57c4c00bdb2c5f1d8a69a2b98baddcf
fetch https://download.falco.org/packages/bin/x86_64/falco-0.45.2-x86_64.tar.gz falco.tgz \
  b2e9961e51601095765c6c644329673e6af2f06c02357d0e77da0f680e4910da
fetch https://raw.githubusercontent.com/falcosecurity/rules/falco-rules-5.2.0/rules/falco_rules.yaml falco_rules.yaml \
  30a1a60263ab97650c851011468da03b17fc0096a7cfa3b6686eba5de73b865b
fetch https://raw.githubusercontent.com/falcosecurity/rules/falco-rules-5.2.0/rules/falco-incubating_rules.yaml falco-incubating_rules.yaml \
  47bf87041e2f39ca336e7acf1ae4e6183771a92e81031f367d01f2b49cabe1ac
fetch https://raw.githubusercontent.com/falcosecurity/rules/falco-rules-5.2.0/rules/falco-sandbox_rules.yaml falco-sandbox_rules.yaml \
  f05ad34f07c43930932ea6dd84c08a37bc8e5ba9f88901ca091ff1b52b3f33d3
mkdir bench falco
tar -C bench -xzf bench.tgz
tar -C falco --strip-components=1 -xzf falco.tgz
sudo -n install -m 0755 bom bench/kube-bench /usr/local/bin/
sudo -n mkdir -p /opt/kube-bench
sudo -n cp -a bench/cfg /opt/kube-bench/
sudo -n cp -a falco/usr/. /usr/
sudo -n cp -a falco/etc/. /etc/
sudo -n install -m 0644 falco_rules.yaml /etc/falco/falco_rules.yaml
sudo -n install -m 0644 falco-incubating_rules.yaml falco-sandbox_rules.yaml /etc/falco/
sudo -n mkdir -p /var/lib/painkiller/cks001
sudo -n touch /var/lib/painkiller/cks001/packages
