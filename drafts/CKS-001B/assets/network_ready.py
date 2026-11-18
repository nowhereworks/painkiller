"""Read-only initial-state network probes against lab-local fixture endpoints."""
import json
from pathlib import Path
import subprocess
import sys

from host_fixtures import path
from network_server import ADDRESSES
from services import command, k

# This program runs inside an existing non-hostNetwork Pod, not on the node.
# It emits a bounded JSON result even if endpoints are denied or unavailable.
POD_PROBE = r"""
import json, socket, ssl, struct, sys
addresses=json.loads(sys.argv[1]); dns=sys.argv[2]
out={}
for addr in addresses:
  for port in (8081,443,9443):
    key=f'{addr}:{port}'
    try:
      with socket.create_connection((addr,port),timeout=1) as raw:
        raw.settimeout(1)
        if port in (443,9443):
          with ssl._create_unverified_context().wrap_socket(raw,server_hostname='probe.training.example') as sock:
            sock.sendall(b'GET / HTTP/1.1\r\nHost: probe\r\nConnection: close\r\n\r\n')
            out[key]=b'HTTP/1.1 200' in sock.recv(1024)
        else:
          raw.sendall(b'GET / HTTP/1.1\r\nHost: probe\r\nConnection: close\r\n\r\n')
          out[key]=b'HTTP/1.1 200' in raw.recv(1024)
    except (OSError,ssl.SSLError): out[key]=False
name=b''.join(bytes([len(x)])+x for x in b'kubernetes.default.svc.cluster.local'.split(b'.'))+b'\0'
query=struct.pack('!HHHHHH',0x1234,0x0100,1,0,0,0)+name+struct.pack('!HH',1,1)
for mode in ('udp','tcp'):
  try:
    with socket.socket(socket.AF_INET,socket.SOCK_DGRAM if mode=='udp' else socket.SOCK_STREAM) as s:
      s.settimeout(1)
      if mode=='udp':
        s.sendto(query,(dns,53)); response,_=s.recvfrom(4096)
      else:
        s.connect((dns,53)); s.sendall(struct.pack('!H',len(query))+query)
        size=s.recv(2,socket.MSG_WAITALL)
        if len(size)!=2: raise OSError('short DNS length')
        response=s.recv(struct.unpack('!H',size)[0],socket.MSG_WAITALL)
      out['dns_'+mode]=len(response)>=12 and response[:2]==b'\x12\x34' and bool(response[2]&0x80) and not (response[3]&15)
  except OSError: out['dns_'+mode]=False
print(json.dumps(out))
"""


def pod_probe(namespace, dns):
    result = command(["kubectl", "--request-timeout=25s", "-n", namespace, "exec", "net-observer",
                      "--", "python3", "-c", POD_PROBE, json.dumps(ADDRESSES), dns], timeout=28)
    return json.loads(result.stdout)


def inspect(root):
    failures = []
    state = path(root, "/var/lib/painkiller/cks001b")
    if not (state / "network-installed").is_file():
        failures.append("network fixture installation marker missing")
    try:
        command(["systemctl", "is-active", "--quiet", "cks001b-network.service"], timeout=5)
    except subprocess.CalledProcessError:
        failures.append("network fixture service inactive")
    interfaces = json.loads(command(["ip", "-j", "-4", "address", "show"], timeout=5).stdout)
    loopback = next((interface for interface in interfaces if interface.get("ifname") == "lo"), {})
    assigned = {entry.get("local") for entry in loopback.get("addr_info", [])}
    if not set(ADDRESSES) <= assigned:
        failures.append("network fixture addresses missing from guest loopback")
    dns = json.loads(k("-n", "kube-system", "get", "service", "kube-dns", "-o", "json").stdout)
    dns_ip = dns["spec"]["clusterIP"]
    for namespace in ("outbound-guard", "api-clients"):
        result = pod_probe(namespace, dns_ip)
        for address in ADDRESSES:
            for port in (8081, 443, 9443):
                if result.get(f"{address}:{port}") is not True:
                    failures.append(f"{namespace} cannot reach fixture {address}:{port}")
        for protocol in ("udp", "tcp"):
            if result.get("dns_" + protocol) is not True:
                failures.append(f"{namespace} {protocol} DNS unavailable")
    for namespace, name in (("storefront", "shop-api"), ("storefront", "untrusted"), ("telemetry", "observer")):
        result = command(["kubectl", "--request-timeout=5s", "-n", namespace, "exec", name, "--",
                          "wget", "-T", "2", "-qO-", "http://shop-web.storefront.svc.cluster.local/"], timeout=8)
        if "Welcome to nginx" not in result.stdout:
            failures.append(f"{namespace}/{name} cannot reach initial frontend")
    return failures


if __name__ == "__main__":
    problems = inspect(Path("/"))
    if problems:
        print("\n".join(problems), file=sys.stderr)
        sys.exit(1)
    print("CKS-001B network subset ready (not full exam readiness)")
