#!/usr/bin/env python3
"""Local safety regressions. All hypervisor/network executables are stubbed."""
import fcntl
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "register_dns", ROOT / "playbooks/workstation/files/register-dns.py",
)
DNS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(DNS)
BOOT_SPEC = importlib.util.spec_from_file_location(
    "check_cloud_init", ROOT / "playbooks/workstation/files/check-cloud-init.py",
)
BOOT = importlib.util.module_from_spec(BOOT_SPEC)
BOOT_SPEC.loader.exec_module(BOOT)


class ImageSafetyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "scripts/workstation").mkdir(parents=True)
        shutil.copy2(ROOT / "scripts/build-workstation-template.sh", self.root / "scripts")
        shutil.copy2(ROOT / "scripts/test-workstation-image.sh", self.root / "scripts")
        shutil.copy2(ROOT / "scripts/validate-workstation-image.sh", self.root / "scripts")
        shutil.copytree(ROOT / "playbooks/workstation", self.root / "playbooks/workstation")
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.log = self.root / "calls"
        self.key = self.root / "management.pub"
        self.key.write_text("test public key\n")
        self.env = {
            **os.environ, "PATH": str(self.bin) + ":" + os.environ["PATH"],
            "WORKSTATION_SSH_PUBLIC_KEY": str(self.key),
            "WORKSTATION_IMAGE_KEYRING": str(self.key),
            "PAINKILLER_LAB_LOCK_DIR": str(self.root / "locks"),
            "LAB_WORKSTATION_TEMPLATE_ID": "9150", "LAB_SSH_USER": "student",
            "CALLS": str(self.log),
        }
        # Never fall through to real SSH/downloads, even if a regression changes order.
        for name in ("ssh", "scp", "curl", "gpgv", "ansible-playbook", "ssh-keygen"):
            self.stub(name, f'echo "{name} $*" >> "$CALLS"\nexit 0\n')
        self.stub("df", "echo 'test 999999999 0 999999999 0% /'\n")
        self.stub("curl", """
echo "curl $*" >> "$CALLS"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then
    shift
    case "$1" in
      */SHA256SUMS) printf '%064d *resolute-server-cloudimg-amd64.img\\n' 0 > "$1" ;;
      *) echo fake > "$1" ;;
    esac
    exit 0
  fi
  shift
done
exit 1
""")

    def stub(self, name, body):
        path = self.bin / name
        path.write_text("#!/bin/bash\nset -eu\n" + body)
        path.chmod(0o700)

    def run_builder(self, **settings):
        return subprocess.run(
            ["bash", str(self.root / "scripts/build-workstation-template.sh")],
            env={**self.env, **settings}, capture_output=True, text=True, timeout=10,
        )

    def calls(self):
        return self.log.read_text() if self.log.exists() else ""

    def test_invalid_id_and_root_rejected_before_ssh(self):
        for settings in ({"LAB_WORKSTATION_TEMPLATE_ID": "9000"},
                         {"LAB_WORKSTATION_TEMPLATE_ID": "9199"},
                         {"LAB_WORKSTATION_TEMPLATE_ID": "oops"},
                         {"LAB_SSH_USER": "root"}):
            with self.subTest(settings=settings):
                self.assertNotEqual(self.run_builder(**settings).returncode, 0)
                self.assertNotIn("ssh ", self.calls())

    def test_failed_preflight_never_downloads_or_imports(self):
        self.stub("ssh", 'echo ssh >> "$CALLS"\nexit 1\n')
        self.assertNotEqual(self.run_builder().returncode, 0)
        self.assertNotIn("curl", self.calls())
        self.assertNotIn("scp", self.calls())

    def test_bad_signature_never_downloads_image_or_imports(self):
        self.stub("gpgv", 'echo gpgv >> "$CALLS"\nexit 1\n')
        self.assertNotEqual(self.run_builder().returncode, 0)
        self.assertNotIn("scp", self.calls())
        self.assertNotIn("resolute-server-cloudimg-amd64.img -o", self.calls())

    def test_bad_checksum_never_imports(self):
        self.assertNotEqual(self.run_builder().returncode, 0)
        self.assertIn("gpgv", self.calls())
        self.assertNotIn("scp", self.calls())

    def test_shared_lab_lock_blocks_remote_operations(self):
        locks = Path(self.env["PAINKILLER_LAB_LOCK_DIR"])
        locks.mkdir()
        with (locks / "lab-operation.lock").open("w") as handle:
            fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.run_builder()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("ssh ", self.calls())

    def test_import_refuses_occupied_destination(self):
        self.stub("qm", 'echo "qm $*" >> "$CALLS"\necho "9150 existing stopped"\n')
        result = subprocess.run(
            ["bash", str(ROOT / "scripts/workstation/create-image.sh"),
             "/tmp/opencode/painkiller-workstation.test", "9150", "local-lvm",
             "vmbr0", "student", "4096", "32", "painkiller-workstation-image-v1"],
            env=self.env, capture_output=True, text=True, timeout=10,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls().strip(), "qm list")

    def test_import_checks_checksum_before_any_mutation(self):
        work = Path("/tmp/opencode") / ("painkiller-workstation." + self.root.name.replace("_", ""))
        work.mkdir()
        self.addCleanup(shutil.rmtree, work)
        (work / "resolute-server-cloudimg-amd64.img").write_bytes(b"corrupt")
        (work / "image.sha256").write_text(
            hashlib.sha256(b"expected").hexdigest() + "  resolute-server-cloudimg-amd64.img\n",
        )
        self.stub("qm", 'echo "qm $*" >> "$CALLS"\n')
        result = subprocess.run(
            ["bash", str(ROOT / "scripts/workstation/create-image.sh"), str(work),
             "9150", "local-lvm", "vmbr0", "student", "4096", "32",
             "painkiller-workstation-image-v1"],
            env=self.env, capture_output=True, text=True, timeout=10,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls().strip(), "qm list")


    def test_clone_validation_requires_explicit_safe_range(self):
        for start in ("", "2999", "3999", "oops"):
            with self.subTest(start=start):
                result = subprocess.run(
                    ["bash", str(self.root / "scripts/test-workstation-image.sh")],
                    env={**self.env, "WORKSTATION_TEST_START": start},
                    capture_output=True, text=True, timeout=10,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("ssh ", self.calls())

    def test_import_only_creates_its_new_vm_and_uses_actual_volume(self):
        work = Path("/tmp/opencode") / ("painkiller-workstation." + self.root.name.replace("_", ""))
        work.mkdir()
        self.addCleanup(shutil.rmtree, work)
        (work / "resolute-server-cloudimg-amd64.img").write_bytes(b"verified")
        (work / "image.sha256").write_text(
            hashlib.sha256(b"verified").hexdigest() + "  resolute-server-cloudimg-amd64.img\n",
        )
        self.stub("virt-customize", 'echo "virt-customize $*" >> "$CALLS"\n')
        self.stub("qm", """
echo "qm $*" >> "$CALLS"
if [ "$1" = config ]; then
  echo 'description: painkiller-workstation-image-v1'
  echo 'unused0: local-lvm:vm-9150-disk-7'
fi
""")
        result = subprocess.run(
            ["bash", str(ROOT / "scripts/workstation/create-image.sh"), str(work),
             "9150", "local-lvm", "vmbr0", "student", "4096", "32",
             "painkiller-workstation-image-v1"],
            env=self.env, capture_output=True, text=True, timeout=10,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        self.assertIn("qm create 9150", calls)
        self.assertIn("--scsi0 local-lvm:vm-9150-disk-7", calls)
        self.assertIn("--ipconfig0 ip=dhcp", calls)
        self.assertIn("qm start 9150", calls)
        self.assertNotIn("qm clone", calls)
        self.assertNotIn("9199", calls)
        self.assertNotIn("cipassword", calls)
        self.assertNotIn("qm destroy", calls)

    def test_clone_preflight_failure_cannot_create_or_destroy(self):
        self.stub("ssh", 'echo "ssh $*" >> "$CALLS"\nexit 1\n')
        result = subprocess.run(
            ["bash", str(self.root / "scripts/test-workstation-image.sh")],
            env={**self.env, "WORKSTATION_TEST_START": "3100"},
            capture_output=True, text=True, timeout=10,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("qm clone", self.calls())
        self.assertNotIn("qm destroy", self.calls())

    def test_dns_readiness_requires_an_actual_successful_oneshot(self):
        self.stub("sudo", 'test "$1" = -n\nshift\nexec "$@"\n')
        self.stub("systemctl", 'printf "%s\\n" "$DNS_RESULT"\n')
        for result, expected in (
            ("Result=success\nExecMainStatus=0\nExecMainStartTimestampMonotonic=123", 0),
            ("Result=success\nExecMainStatus=0\nExecMainStartTimestampMonotonic=0", 1),
            ("Result=exit-code\nExecMainStatus=1\nExecMainStartTimestampMonotonic=123", 1),
            ("Result=success\nExecMainStatus=1\nExecMainStartTimestampMonotonic=123", 1),
            ("Result=success\nExecMainStatus=0", 1),
        ):
            with self.subTest(result=result):
                process = subprocess.run(
                    ["bash", str(self.root / "scripts/validate-workstation-image.sh"), "--dns-only"],
                    env={**self.env, "DNS_RESULT": "ActiveState=inactive\n" + result},
                    capture_output=True, text=True, timeout=10,
                )
                self.assertEqual(process.returncode, expected, process.stderr)
        process = subprocess.run(
            ["bash", str(self.root / "scripts/validate-workstation-image.sh"), "--dns-only"],
            env={**self.env, "DNS_RESULT": "ActiveState=activating\nResult=success\nExecMainStatus=0\nExecMainStartTimestampMonotonic=123"},
            capture_output=True, text=True, timeout=10,
        )
        self.assertEqual(process.returncode, 1, "An in-progress DNS invocation must not count as success")


class CloudInitTests(unittest.TestCase):
    def test_done_and_known_proxmox_deprecation(self):
        BOOT.check({"status": "done", "errors": []})
        BOOT.check({
            "status": "done", "errors": [], "recoverable_errors": {
                "DEPRECATED": ["'user' of type string is deprecated in 22.2 and scheduled to be removed"],
            },
        })

    def test_other_boot_errors_are_not_silenced(self):
        for data in (
            {"status": "running"},
            {"status": "done", "errors": ["failure"]},
            {"status": "done", "recoverable_errors": {"WARNING": ["network rename failure"]}},
            {"status": "done", "recoverable_errors": {"DEPRECATED": ["unexpected schema"]}},
        ):
            with self.subTest(data=data), self.assertRaises(ValueError):
                BOOT.check(data)


class DNSRegistrationTests(unittest.TestCase):
    def run_registration(self, records):
        response = io.BytesIO(json.dumps(records).encode())
        route = json.dumps([{"prefsrc": "192.0.2.10"}])
        with patch.object(DNS.socket, "gethostname", return_value="k8s-3100"), \
             patch.object(DNS.subprocess, "check_output", return_value=route), \
             patch.object(DNS.urllib.request, "urlopen", return_value=response) as request:
            DNS.register({"api": "http://dns.test:8888", "zone": "lab.test", "route": "192.0.2.1"})
            return request.call_count

    def test_matching_record_does_not_write(self):
        self.assertEqual(self.run_registration([{"record_type": "A", "value": "192.0.2.10"}]), 1)

    def test_conflicting_records_are_preserved(self):
        for records in ({}, [{"record_type": "AAAA"}], [{}, {}]):
            with self.subTest(records=records), self.assertRaises(ValueError):
                self.run_registration(records)

    def test_new_record_has_only_the_discovered_dhcp_address(self):
        response = io.BytesIO(b"[]")
        with patch.object(DNS.socket, "gethostname", return_value="k8s-3100"), \
             patch.object(DNS.subprocess, "check_output", return_value='[{"prefsrc":"192.0.2.10"}]'), \
             patch.object(DNS.urllib.request, "urlopen", side_effect=[response, io.BytesIO(b"{}")]) as request:
            DNS.register({"api": "http://dns.test:8888", "zone": "lab.test", "route": "192.0.2.1"})
            posted = request.call_args.args[0]
            self.assertEqual(posted.method, "POST")
            self.assertEqual(json.loads(posted.data), {
                "hostname": "k8s-3100", "zone": "lab.test", "record_type": ["a"], "value": "192.0.2.10",
            })


if __name__ == "__main__":
    unittest.main()
