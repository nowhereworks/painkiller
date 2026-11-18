import os
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
from host_fixtures import initialize, path
from host_ready import cert_matches, check


@unittest.skipUnless(shutil.which("openssl") and shutil.which("go"), "openssl and go required")
class HostFixtureTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir="/tmp/opencode")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.state = path(self.root, "/var/lib/painkiller/cks001b")
        self.state.mkdir(parents=True)
        falco = path(self.root, "/etc/falco")
        falco.mkdir(parents=True)
        for name in ("falco_rules.yaml", "falco-incubating_rules.yaml", "falco-sandbox_rules.yaml"):
            (falco / name).write_text(name)

    def test_authorization_guard(self):
        with self.assertRaisesRegex(RuntimeError, "unauthorized"):
            initialize(self.root)
        self.assertFalse(path(self.root, "/opt/course/ledger/ledger.Dockerfile").exists())

    def test_real_generation_readiness_and_retry(self):
        (self.state / "authorized").touch()
        initialize(self.root)
        (self.state / "packages").touch()
        for name in ("bom", "kube-bench", "kubesec", "istioctl"):
            binary = path(self.root, "/usr/local/bin/" + name)
            binary.parent.mkdir(parents=True, exist_ok=True)
            binary.write_text("#!/bin/sh\n")
            binary.chmod(0o755)
        falco = path(self.root, "/usr/bin/falco")
        falco.parent.mkdir(parents=True, exist_ok=True)
        falco.write_text("#!/bin/sh\n")
        falco.chmod(0o755)
        self.assertEqual(check(self.root, lambda name: name == "falco"), [])
        key = path(self.root, "/root/portal-client.key")
        self.assertEqual(key.stat().st_mode & 0o777, 0o600)
        self.assertFalse(cert_matches(path(self.root, "/root/portal-client.crt"), key, "wrong.example"))
        self.assertTrue(os.access(path(self.root, "/opt/course/ledger/ledger"), os.X_OK))
        dockerfile = path(self.root, "/opt/course/ledger/ledger.Dockerfile")
        dockerfile.write_text("candidate change\n")
        initialize(self.root)
        self.assertEqual(dockerfile.read_text(), "candidate change\n")
        self.assertTrue(any("Dockerfile" in item for item in check(self.root, lambda _: True)))
        self.assertTrue(any("Falco service inactive" in item for item in check(self.root, lambda _: False)))

    def test_incomplete_certificate_pair_fails_closed(self):
        (self.state / "authorized").touch()
        cert = path(self.root, "/root/portal-client.crt")
        cert.parent.mkdir(parents=True)
        cert.write_text("existing certificate")
        with self.assertRaisesRegex(RuntimeError, "Incomplete certificate pair"):
            initialize(self.root)
        self.assertEqual(cert.read_text(), "existing certificate")
        self.assertFalse((self.state / "host-files").exists())


if __name__ == "__main__":
    unittest.main()
