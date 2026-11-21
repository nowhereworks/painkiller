#!/usr/bin/env python3
"""Register exactly one unambiguous A record; never edit DHCP/NIC configuration."""
import ipaddress
import json
import re
import socket
import subprocess
import urllib.request


def register(config):
    hostname = socket.gethostname().split(".")[0].lower()
    if not re.fullmatch(r"[a-z0-9](?:[a-z0-9-]*[a-z0-9])?", hostname) or hostname in (
        "localhost", "ubuntu", "ubuntu-server-cloud-init",
    ):
        raise ValueError("A unique clone hostname is required")
    route = json.loads(subprocess.check_output(
        ["ip", "-j", "-4", "route", "get", str(ipaddress.IPv4Address(config["route"]))],
        text=True,
    ))
    address = str(ipaddress.IPv4Address(route[0]["prefsrc"]))
    url = f'{config["api"].rstrip("/")}/hosts/{config["zone"]}/{hostname}'
    with urllib.request.urlopen(url, timeout=15) as response:
        records = json.load(response)
    if not isinstance(records, list):
        raise ValueError("Unexpected DNS API response")
    if records:
        if len(records) != 1 or records[0].get("record_type", "").lower() != "a":
            raise ValueError("Conflicting or ambiguous DNS records; refusing to overwrite")
        if records[0].get("value") == address:
            return
    payload = json.dumps({
        "hostname": hostname, "zone": config["zone"],
        "record_type": ["a"], "value": address,
    }).encode()
    request = urllib.request.Request(
        config["api"].rstrip("/") + "/hosts", data=payload,
        headers={"Content-Type": "application/json"}, method="POST",
    )
    with urllib.request.urlopen(request, timeout=15) as response:
        response.read()


if __name__ == "__main__":
    with open("/etc/painkiller-dns.json", encoding="utf-8") as handle:
        register(json.load(handle))
