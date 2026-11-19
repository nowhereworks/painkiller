import json
import socket
import ssl
import sys
import tempfile
import threading
import time
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
import ingress_fixture
import ingress_ready
import network_ready
import network_server
import network_setup
from host_fixtures import create_certificate


class NetworkInfrastructureTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir="/tmp/opencode")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.state = self.root / "var/lib/painkiller/cks001b"
        self.state.mkdir(parents=True)
        (self.state / "authorized").touch()
        (self.state / "host-files").touch()

    def test_loopback_only_retry_and_conflict(self):
        calls = []
        interfaces = [{"ifname": "lo", "addr_info": [{"local": "127.0.0.1"}]}]

        def command(args, **kwargs):
            calls.append(args)
            if args[:2] == ["ip", "-j"]:
                return SimpleNamespace(stdout=json.dumps(interfaces))
            return SimpleNamespace(stdout="")

        network_setup.setup(self.root, command)
        self.assertEqual([args[-1] for args in calls if args[:3] == ["ip", "address", "add"]], ["lo"] * 4)
        self.assertTrue((self.state / "network-installed").exists())
        count = len(calls)
        network_setup.setup(self.root, command)
        self.assertEqual(len(calls), count)
        (self.state / "network-installed").unlink()
        interfaces.append({"ifname": "eth0", "addr_info": [{"local": network_server.ADDRESSES[0]}]})
        with self.assertRaisesRegex(RuntimeError, "Refusing to reuse"):
            network_setup.setup(self.root, command)

    def test_network_server_real_http_and_tls(self):
        cert, key = self.state / "test.crt", self.state / "test.key"
        create_certificate(cert, key, "probe.training.example")

        def free_port():
            with socket.socket() as sock:
                sock.bind(("127.0.0.1", 0))
                return sock.getsockname()[1]

        http_port, tls_port = free_port(), free_port()
        while tls_port == http_port:
            tls_port = free_port()
        stopped = threading.Event()
        thread = threading.Thread(target=network_server.serve,
                                  args=(("127.0.0.1",), (http_port, tls_port), str(cert), str(key)),
                                  kwargs={"stop": stopped, "tls_ports": (tls_port,)}, daemon=True)
        thread.start()
        self.addCleanup(thread.join, 3)
        self.addCleanup(stopped.set)
        deadline = time.monotonic() + 3
        while True:
            try:
                raw = socket.create_connection(("127.0.0.1", http_port), timeout=0.5)
                break
            except OSError:
                if time.monotonic() >= deadline:
                    raise
                time.sleep(0.02)
        with raw:
            raw.sendall(b"GET / HTTP/1.1\r\nHost: test\r\n\r\n")
            self.assertIn(network_server.BODY, raw.recv(4096))
        context = ssl.create_default_context(cafile=str(cert))
        with socket.create_connection(("127.0.0.1", tls_port), timeout=2) as raw:
            with context.wrap_socket(raw, server_hostname="probe.training.example") as conn:
                conn.sendall(b"GET / HTTP/1.1\r\nHost: test\r\n\r\n")
                self.assertIn(network_server.BODY, conn.recv(4096))

    def test_ingress_fixture_is_separate_from_candidate(self):
        objs = ingress_fixture.objects()
        deployment = next(o for o in objs if o["kind"] == "Deployment")
        self.assertEqual(deployment["spec"]["replicas"], 1)
        self.assertIn("@sha256:", deployment["spec"]["template"]["spec"]["containers"][0]["image"])
        svc = next(o for o in objs if o["kind"] == "Service")
        self.assertEqual(svc["spec"]["type"], "NodePort")
        ingress = ingress_fixture.canary()
        self.assertNotEqual(ingress["metadata"]["name"], "portal-tls")
        self.assertNotEqual(ingress["spec"]["rules"][0]["host"], "portal.training.example")

    def test_ingress_probe_requires_redirect_and_verified_https(self):
        class Result:
            def __init__(self, stdout, code=0):
                self.stdout, self.returncode = stdout, code
        responses = [Result("HTTP/1.1 308 Permanent Redirect\r\nLocation: https://ingress-check.training.example/\r\n"),
                     Result("Welcome to nginx")]
        with patch.object(ingress_ready.subprocess, "run", side_effect=responses) as run:
            self.assertEqual(ingress_ready.probe("192.0.2.2", 30080, 30443, Path("/tmp/ca")), (True, True))
        self.assertIn("--cacert", run.call_args_list[1].args[0])
        responses = [Result("HTTP/1.1 200 OK\r\n"), Result("", 60)]
        with patch.object(ingress_ready.subprocess, "run", side_effect=responses):
            self.assertEqual(ingress_ready.probe("192.0.2.2", 30080, 30443, Path("/tmp/ca")), (False, False))

    def test_network_readiness_tests_all_endpoints_and_dns(self):
        (self.state / "network-installed").touch()
        result = {f"{address}:{port}": True for address in network_server.ADDRESSES for port in (8081, 443, 9443)}
        result.update(dns_udp=True, dns_tcp=True)

        def command(args, **kwargs):
            if args[0] == "ip":
                data = [{"ifname": "lo", "addr_info": [{"local": x} for x in network_server.ADDRESSES]}]
                return SimpleNamespace(stdout=json.dumps(data))
            if args[0] == "kubectl":
                return SimpleNamespace(stdout="Welcome to nginx")
            return SimpleNamespace(stdout="")

        with patch.object(network_ready, "command", command), \
             patch.object(network_ready, "k", return_value=SimpleNamespace(stdout='{"spec":{"clusterIP":"10.96.0.10"}}')), \
             patch.object(network_ready, "pod_probe", return_value=result):
            self.assertEqual(network_ready.inspect(self.root), [])
            result["198.18.30.10:9443"] = False
            result["dns_tcp"] = False
            failures = network_ready.inspect(self.root)
            self.assertTrue(any("198.18.30.10:9443" in failure for failure in failures))
            self.assertTrue(any("tcp DNS" in failure for failure in failures))


if __name__ == "__main__":
    unittest.main()
