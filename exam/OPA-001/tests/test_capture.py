#!/usr/bin/env python3
"""Local tests use real pinned CLIs, temporary files, and no configured cluster."""
import contextlib
import importlib.util
import io
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import tarfile
import threading
import time
import http.server
import unittest
from unittest import mock

EXAM = pathlib.Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("capture", EXAM / "assets/capture.py")
capture = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(capture)
TASKS = json.loads((EXAM / "tasks.json").read_text())
LOCAL = (1, 2, 3, 4, 5, 6, 7, 8, 15, 16)


class Metadata(unittest.TestCase):
    def test_contract(self):
        exam = json.loads((EXAM / "test.json").read_text())
        self.assertEqual((exam["lab"]["controllers"], exam["lab"]["workers"]), (1, 0))
        self.assertEqual(exam["lab"]["template_id"], "9199")
        self.assertEqual(len(TASKS), 16)
        self.assertEqual(sum(t["points"] for t in TASKS), 100)
        mutations = json.loads((EXAM / "e2e.json").read_text())["mutations"]
        self.assertEqual({m["task"] for m in mutations}, {t["id"] for t in TASKS})
        for i, task in enumerate(TASKS, 1):
            self.assertEqual(task["question_id"], f"OPA-001-Q{i}")
            self.assertIn(task["category"], exam["categories"])
            self.assertIn("http", task["explanation"])
            self.assertIn(f"capture.py q{i}", task["resource"])
            self.assertNotIn("/exam/CK", task["solution"])

    def test_supporting_api_failure_is_not_a_missing_answer(self):
        with mock.patch.object(capture, "run", return_value=subprocess.CompletedProcess(
            [], 1, "", 'Error from server (NotFound): configmaps "answer" not found')):
            self.assertIsNone(capture.kget("configmap", "answer"))
        with mock.patch.object(capture, "run", return_value=subprocess.CompletedProcess(
            [], 1, "", "transport exec failed (NotFound)")):
            with self.assertRaises(capture.InfrastructureError):
                capture.kget("configmap", "answer")
        with mock.patch.object(capture, "run", return_value=subprocess.CompletedProcess(
            [], 1, "", "Error from server (Forbidden): denied")):
            with self.assertRaises(capture.InfrastructureError):
                capture.kget("configmap", "answer")

    def test_admission_transport_and_unrelated_denials_are_errors(self):
        for stderr in ("connection refused", "(NotFound) namespace missing",
                       'admission webhook "other.example" denied the request'):
            with self.subTest(stderr=stderr), mock.patch.object(
                capture, "run", return_value=subprocess.CompletedProcess([], 1, "", stderr)
            ):
                with self.assertRaises(capture.InfrastructureError):
                    capture.admission("opa-labels", {}, False, "require-team-owner")

    def test_malformed_bundle_yaml_is_an_answer_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            (root / "q10").mkdir()
            (root / "q10/config.yaml").write_text("bundles: [\n")
            with mock.patch.object(capture, "ROOT", root), mock.patch.object(capture, "http", return_value={}):
                self.assertEqual(capture.grade("q10"), ["OPA config.yaml must be valid YAML"])

    def test_inventory_template_accepts_only_api_defaulting(self):
        import yaml
        spec = list(yaml.safe_load_all((EXAM / "assets/gatekeeper-fixtures.yaml").read_text()))[-1]["spec"]
        spec["crd"]["spec"]["validation"]["legacySchema"] = False
        def get(kind, name, namespace=None):
            if kind == "config.config.gatekeeper.sh":
                return {"spec": {"sync": {"syncOnly": [{"group": "", "version": "v1", "kind": "Namespace"}]}}}
            if kind == "constrainttemplate":
                return {"spec": spec}
            return {"metadata": {"labels": {"team": name.rsplit("-", 1)[-1]}}}
        with mock.patch.object(capture, "ASSETS", EXAM / "assets"), mock.patch.object(
            capture, "kget", side_effect=get
        ), mock.patch.object(capture, "constraint", return_value=({}, [])), mock.patch.object(
            capture, "admission", return_value=[]
        ):
            self.assertEqual(capture.grade("q14"), [])
            spec["targets"][0]["code"][0]["source"]["rego"] = "package spoofed\n"
            self.assertEqual(capture.grade("q14"), ["preserve the supplied inventory-based template"])

    def test_missing_runtime_answers_have_diagnostics(self):
        with tempfile.TemporaryDirectory() as directory, mock.patch.object(
            capture, "ROOT", pathlib.Path(directory)
        ), mock.patch.object(capture, "kget", return_value=None), mock.patch.object(capture, "http", return_value=None):
            for number in range(9, 15):
                with self.subTest(number=number):
                    failures = capture.grade(f"q{number}")
                    self.assertTrue(failures)
                    self.assertTrue(all(isinstance(f, str) and f for f in failures))


@unittest.skipUnless(all(shutil.which(t) for t in ("opa", "regal", "conftest")), "requires pinned OPA, Regal, Conftest")
class RealPolicyCapture(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="opa001-")
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        capture.ROOT = self.root
        capture.ASSETS = EXAM / "assets"
        subprocess.run(["python3", "-B", EXAM / "assets/workshop.py", self.root], check=True)

    def solve(self, number):
        command = TASKS[number - 1]["solution"].replace("/opt/opa-workshop", str(self.root)).replace(
            "/opt/painkiller/opa001", str(EXAM / "assets"))
        subprocess.run(["bash", "-e", "-o", "pipefail", "-c", command], check=True,
                       capture_output=True, timeout=30)

    def test_initial_states_and_missing_answers_have_diagnostics(self):
        for number in LOCAL:
            with self.subTest(number=number):
                failures = capture.grade(f"q{number}")
                self.assertTrue(failures)
                self.assertTrue(all(isinstance(f, str) and f for f in failures))
                for path in (self.root / f"q{number}").glob("*"):
                    if path.is_file():
                        path.unlink()
                self.assertTrue(capture.grade(f"q{number}"))

    def test_reference_solutions_and_behavioral_mutations(self):
        for number in LOCAL:
            with self.subTest(number=number):
                self.solve(number)
                self.assertEqual(capture.grade(f"q{number}"), [])
                directory = self.root / f"q{number}"
                if number in (1, 2, 3, 5, 8, 16):
                    (directory / "policy.rego").write_text(f"package q{number}\n\nallow := true\n")
                elif number == 4:
                    (directory / "policy.rego").write_text("package q4\n\nviolations := []\n")
                elif number == 6:
                    (directory / "tests.rego").write_text("package tenant_test\n\ntest_placeholder if { true }\n")
                elif number == 7:
                    (directory / "adapter.py").write_text('print(\'{"allow":true,"reason":"allow"}\')\n')
                elif number == 15:
                    (directory / "policy/main.rego").write_text("package main\n\ndeny := []\n")
                self.assertTrue(capture.grade(f"q{number}"))

    def test_parse_errors_are_answer_failures(self):
        for number in (1, 2, 3, 4, 5, 8, 16):
            (self.root / f"q{number}/policy.rego").write_text("package broken\nallow if {\n")
            with self.subTest(number=number):
                self.assertTrue(capture.grade(f"q{number}"))

    def test_reference_bundle_manifest_and_decisions(self):
        self.solve(9)
        bundle = self.root / "q9/bundle.tar.gz"
        with tarfile.open(bundle) as archive:
            manifest = next(m for m in archive.getmembers() if m.name.lstrip("/") == ".manifest")
            self.assertEqual(json.load(archive.extractfile(manifest)), {
                "revision": "release-1", "roots": ["release"], "rego_version": 1,
            })
        for user, action, expected in [
            ("alice", "read", True), ("bob", "read", True), ("eve", "read", False), ("alice", "write", False),
        ]:
            result = capture.run(["opa", "eval", "--format=json", "--bundle", bundle, "--stdin-input",
                                  "data.release.allow"], {"user": user, "action": action})
            self.assertEqual(result.returncode, 0)
            self.assertIs(capture.decision(json.loads(result.stdout)), expected)

    def test_management_status_posts_use_partition_name(self):
        import yaml
        spec = importlib.util.spec_from_file_location("receiver", EXAM / "assets/service.py")
        receiver = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(receiver)
        receiver.ROOT = self.root
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), receiver.Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        self.solve(9)
        config = yaml.safe_load((self.root / "q9/config.yaml").read_text())
        config["services"]["workshop"]["url"] = f"http://127.0.0.1:{server.server_port}"
        (self.root / "q9/config.yaml").write_text(yaml.safe_dump(config))
        with subprocess.Popen(
            ["opa", "run", "--server", "--addr=127.0.0.1:0", "--config-file", self.root / "q9/config.yaml"],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        ) as process:
            try:
                deadline = time.monotonic() + 8
                while time.monotonic() < deadline:
                    with receiver.LOCK:
                        reports = list(receiver.EVENTS["status/q9"])
                    if reports and reports[-1].get("bundles", {}).get("release", {}).get("active_revision") == "release-1":
                        break
                    time.sleep(0.1)
                else:
                    self.fail("real OPA never posted release activation status to status/q9")
            finally:
                process.terminate()
                process.wait(timeout=5)

    def test_main_returns_diagnostics_for_missing_candidate_files(self):
        for number in LOCAL:
            old = self.root / f"q{number}"
            shutil.rmtree(old)
            with self.subTest(number=number), mock.patch("sys.argv", ["capture.py", f"q{number}"]):
                output = io.StringIO()
                with contextlib.redirect_stdout(output):
                    capture.main()
                result = json.loads(output.getvalue())
                self.assertFalse(result["ok"])
                self.assertTrue(result["failures"])


if __name__ == "__main__":
    unittest.main()
