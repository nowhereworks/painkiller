import json
import os
from pathlib import Path
import tempfile
import unittest
from types import SimpleNamespace

import yaml
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
from etcd_capture import benchmark, capture, tree_checks, validate_config
from etcd_fault import inject
from etcd_ready import inspect


def manifest():
    return {"spec": {"containers": [{"name": "etcd", "command": ["etcd", "--data-dir=/var/lib/etcd"],
                                    "volumeMounts": [{"name": "data", "mountPath": "/var/lib/etcd"}]}],
                     "volumes": [{"name": "data", "hostPath": {"path": "/var/lib/etcd"}}]}}


def runtime(uid=0):
    return {"info": {"runtimeSpec": {"process": {
        "user": {"uid": uid}, "args": ["etcd", "--data-dir=/var/lib/etcd"]}}}}


class FakeKube:
    def __init__(self, marked=True, nodes=1):
        self.marked, self.nodes = marked, nodes

    def get(self, kind, name=""):
        if kind == "namespace":
            return {"metadata": {"labels": {"painkiller-shell/exam": "CKS-001B" if self.marked else "OTHER"}}}
        if kind == "nodes":
            return {"items": [{}] * self.nodes}
        raise AssertionError(kind)


class EtcdCaptureTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir="/tmp/opencode")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.state = self.root / "var/lib/painkiller/cks001b"
        self.state.mkdir(parents=True)
        (self.state / "authorized").touch()
        (self.state / "services-complete").touch()
        self.data = self.root / "var/lib/etcd"
        (self.data / "member").mkdir(parents=True)
        self.file = self.data / "member/wal"
        self.file.write_text("fixture")
        self.data.chmod(0o700)
        (self.data / "member").chmod(0o700)
        self.file.chmod(0o600)
        manifest_path = self.root / "etc/kubernetes/manifests/etcd.yaml"
        manifest_path.parent.mkdir(parents=True)
        manifest_path.write_text(yaml.safe_dump(manifest()))

    def test_live_manifest_and_hostpath_must_match(self):
        self.assertEqual(validate_config(manifest(), runtime()), 0)
        bad = manifest()
        bad["spec"]["volumes"][0]["hostPath"]["path"] = "/var/lib/other"
        with self.assertRaisesRegex(RuntimeError, "data-directory"):
            validate_config(bad, runtime())
        altered = runtime()
        altered["info"]["runtimeSpec"]["process"]["args"] = ["etcd", "--data-dir=/tmp/other"]
        with self.assertRaisesRegex(RuntimeError, "data-directory"):
            validate_config(manifest(), altered)

    def test_recursive_modes_and_symlink_safety(self):
        self.assertEqual(tree_checks(self.root, os.getuid(), os.getgid()), (True, True))
        self.file.chmod(0o640)
        self.assertEqual(tree_checks(self.root, os.getuid(), os.getgid()), (False, True))
        self.file.chmod(0o600)
        (self.data / "member/link").symlink_to("/etc/passwd")
        self.assertEqual(tree_checks(self.root, os.getuid(), os.getgid()), (False, False))
        with self.assertRaisesRegex(RuntimeError, "bounded"):
            tree_checks(self.root, os.getuid(), os.getgid(), limit=2)

    def test_capture_baseline_and_solved_health(self):
        run = lambda *args, **kwargs: SimpleNamespace(stdout="ok\n")
        ids = lambda: (os.getuid(), os.getgid())
        kwargs = {"run": run, "runtime": runtime, "ids": ids, "bench": lambda: True}
        result = capture(self.root, **kwargs)
        self.assertEqual(result["failures"], [])
        self.data.chmod(0o750)
        baseline = capture(self.root, run=run, runtime=runtime, ids=lambda: (999999, 999999), bench=lambda: False)
        self.assertFalse(any(value is True for key, value in baseline.items() if key != "failures"))
        self.data.chmod(0o700)
        kwargs["run"] = lambda *args, **kwargs: SimpleNamespace(stdout="unhealthy")
        self.assertFalse(capture(self.root, **kwargs)["healthy"])

    def test_benchmark_selected_checks_and_invalid_report(self):
        statuses = [{"tests": [{"results": [
            {"test_number": "1.1.11", "status": "PASS"}, {"test_number": "1.1.12", "status": "PASS"}]}]}]
        seen = []

        def run(args, **kwargs):
            seen.append(args)
            return SimpleNamespace(stdout=json.dumps(statuses))

        self.assertTrue(benchmark(run))
        self.assertEqual(seen[0][seen[0].index("--check") + 1], "1.1.11,1.1.12")
        statuses[0]["tests"][0]["results"][1]["status"] = "FAIL"
        self.assertFalse(benchmark(run))
        statuses[0]["tests"][0]["results"].pop()
        with self.assertRaisesRegex(RuntimeError, "incomplete"):
            benchmark(run)

    def test_fault_only_changes_expected_open_directory_and_is_idempotent(self):
        seen = []

        def owner(fd, uid, gid):
            seen.append(("owner", os.readlink(f"/proc/self/fd/{fd}"), uid, gid))

        def mode(fd, value):
            seen.append(("mode", os.readlink(f"/proc/self/fd/{fd}"), value))

        options = {"runtime": runtime, "ids": lambda: (1, 1), "euid": lambda: 0,
                   "change_owner": owner, "change_mode": mode}
        with self.assertRaisesRegex(RuntimeError, "unmarked cluster"):
            inject(FakeKube(marked=False), self.root, **options)
        with self.assertRaisesRegex(RuntimeError, "single-node"):
            inject(FakeKube(nodes=2), self.root, **options)
        with self.assertRaisesRegex(RuntimeError, "not root"):
            inject(FakeKube(), self.root, **dict(options, runtime=lambda: runtime(123)))
        self.assertEqual(seen, [])
        inject(FakeKube(), self.root, **options)
        self.assertEqual(seen, [("owner", str(self.data), 0, 0), ("mode", str(self.data), 0o750)])
        inject(FakeKube(), self.root, **options)
        self.assertEqual(len(seen), 2)
        self.assertEqual(self.file.read_text(), "fixture")

    def test_readiness_requires_actual_fault_and_healthy_api(self):
        (self.state / "etcd-fault").touch()
        fake_stat = lambda _: SimpleNamespace(st_uid=0, st_gid=0, st_mode=0o40750)
        good = lambda *args, **kwargs: SimpleNamespace(stdout="ok\n")
        opts = {"runtime": runtime, "ids": lambda: (100, 100), "stat_path": fake_stat, "run": good}
        self.assertEqual(inspect(self.root, **opts), [])
        opts["stat_path"] = lambda _: SimpleNamespace(st_uid=0, st_gid=0, st_mode=0o40700)
        self.assertTrue(any("fault missing" in msg for msg in inspect(self.root, **opts)))


if __name__ == "__main__":
    unittest.main()
