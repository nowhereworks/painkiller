"""Provision guest-local loopback test endpoints without changing its NIC or DHCP."""
import json

from host_fixtures import create_once, path
from network_server import ADDRESSES

UNIT = """[Unit]
Description=CKS-001B local network probe endpoints
After=network.target
[Service]
ExecStartPre=/usr/bin/python3 -B /opt/painkiller/cks001b/network_bind.py
ExecStart=/usr/bin/python3 -B /opt/painkiller/cks001b/network_server.py
Restart=on-failure
RestartSec=2
[Install]
WantedBy=multi-user.target
"""


def ensure_addresses(root, command):
    state = path(root, "/var/lib/painkiller/cks001b")
    if not (state / "authorized").is_file() or not (state / "host-files").is_file():
        raise RuntimeError("Refusing network setup on an unauthorized or unprepared guest")
    interfaces = json.loads(command(["ip", "-j", "-4", "address", "show"]).stdout)
    assigned = {}
    for interface in interfaces:
        for entry in interface.get("addr_info", []):
            if "local" in entry:
                assigned.setdefault(entry["local"], []).append(interface["ifname"])
    for address in ADDRESSES:
        if address in assigned and (assigned[address] != ["lo"] or not (state / "network-started").exists()):
            raise RuntimeError(f"Refusing to reuse existing address {address} on {assigned[address]}")
    # Marker first so interrupted attempts may recognize their own loopback IPs.
    create_once(state / "network-started", "complete\n", 0o600)
    for address in ADDRESSES:
        if address not in assigned:
            command(["ip", "address", "add", address + "/32", "dev", "lo"])


def setup(root, command):
    state = path(root, "/var/lib/painkiller/cks001b")
    if not (state / "authorized").is_file() or not (state / "host-files").is_file():
        raise RuntimeError("Refusing network setup on an unauthorized or unprepared guest")
    if (state / "network-installed").exists():
        return
    ensure_addresses(root, command)
    create_once(path(root, "/etc/systemd/system/cks001b-network.service"), UNIT)
    command(["systemctl", "daemon-reload"])
    command(["systemctl", "enable", "--now", "cks001b-network.service"])
    create_once(state / "network-installed", "complete\n", 0o600)
