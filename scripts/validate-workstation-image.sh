#!/usr/bin/env bash
# Fast, read-only check. Run as the lab user inside a builder/disposable clone.
set -Eeuo pipefail
check_dns_registration() {
  local status started
  # A successful oneshot is inactive, not "active": verify its actual last run.
  status=$(sudo -n systemctl show painkiller-register-dns.service \
    --property=Result --property=ExecMainStatus --property=ExecMainStartTimestampMonotonic --property=ActiveState)
  started=$(sed -n 's/^ExecMainStartTimestampMonotonic=//p' <<<"$status")
  if ! grep -Fxq 'Result=success' <<<"$status" ||
     ! grep -Fxq 'ExecMainStatus=0' <<<"$status" ||
     ! grep -Fxq 'ActiveState=inactive' <<<"$status" ||
     ! [[ $started =~ ^[0-9]+$ && $started -gt 0 ]]; then
    printf 'DNS registration has not completed successfully:\n%s\n' "$status" >&2
    return 1
  fi
  echo 'Live DNS registration: OK'
}
if [[ $# == 1 && $1 == --dns-only ]]; then check_dns_registration; exit; fi
skip_dns=false
if [[ $# == 1 && $1 == --skip-dns ]]; then skip_dns=true; else [[ $# == 0 ]] || { echo 'Usage: validate-workstation-image.sh [--skip-dns|--dns-only]' >&2; exit 1; }; fi
# shellcheck disable=SC1091
source /etc/os-release
[[ $ID == ubuntu && $VERSION_ID == 26.04 && $VERSION_CODENAME == resolute ]]
painkiller-check-cloud-init
for tool in Xtigervnc tigervncconfig startxfce4 xfce4-terminal onboard mousepad xdpyinfo xclip; do command -v "$tool" >/dev/null; done
snap list firefox
sudo -n systemctl is-enabled painkiller-desktop.service painkiller-register-dns.service
sudo -n systemctl is-active painkiller-desktop.service qemu-guest-agent.service
if $skip_dns; then
  echo 'SKIP: live DNS registration (image baking or explicitly requested external-service diagnostic)'
else
  check_dns_registration
fi
DISPLAY=:1 xdpyinfo >/dev/null
sudo -n test -s /opt/painkiller/workstation/source.json
sudo -n test -s /opt/painkiller/workstation/packages
sudo -n test ! -e /etc/kubernetes/admin.conf
sudo -n test ! -d /etc/kubernetes/pki
sudo -n test ! -d /var/lib/etcd
test ! -e "$HOME/.kube/config"
test ! -e "$HOME/.ssh/painkiller_lab"
python3 - <<'PY'
import json
import socket
import subprocess

listeners = subprocess.check_output(["ss", "-H", "-ltn", "sport = :5901"], text=True).splitlines()
assert listeners, "VNC is not listening"
assert all(line.split()[3] in ("127.0.0.1:5901", "[::1]:5901") for line in listeners), "VNC exposed beyond loopback"
with socket.create_connection(("127.0.0.1", 5901), timeout=5) as client:
    assert client.recv(12) == b"RFB 003.008\n", "Unexpected VNC protocol"
    client.sendall(b"RFB 003.008\n")
    count = client.recv(1)[0]
    assert client.recv(count) == b"\x01", "Expected SSH-protected, loopback-only VNC"
with open("/opt/painkiller/workstation/source.json", encoding="utf-8") as handle:
    source = json.load(handle)
assert len(source["source_sha256"]) == 64
print("Ubuntu LTS / XFCE / Firefox / loopback VNC / clean exam state: OK")
PY
