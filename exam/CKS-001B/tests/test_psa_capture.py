import copy
import json
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
from psa_capture import capture, witness_valid
from psa_witness import dry_run, sample
from test_workload_capture import FakeKube, deployment_ready, fixture


class PSAWitnessTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir="/tmp/opencode")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.state = self.root / "var/lib/painkiller/cks001b"
        self.state.mkdir(parents=True)
        self.kube = FakeKube()
        self.kube.items["namespace", None, "cks001b-system"] = {
            "metadata": {"labels": {"painkiller-shell/exam": "CKS-001B"}}}
        self.namespace = fixture("Namespace", None, "finance-ledger")
        self.namespace["metadata"].update({"uid": "ns-uid", "resourceVersion": "7"})
        self.kube.items["namespace", None, "finance-ledger"] = self.namespace
        self.deployment = fixture("Deployment", "finance-ledger", "ledger-web")
        deployment_ready(self.deployment, self.kube, "finance-ledger")

    def test_witness_is_nonpersistent_dry_run_and_classifies_errors(self):
        pod = {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": "x"}}
        denied = SimpleNamespace(returncode=1, stderr='Error from server (Forbidden): violates PodSecurity "restricted"',
                                 stdout="")
        with patch("psa_witness.subprocess.run", return_value=denied) as call:
            self.assertEqual(dry_run(pod), "psa_denied")
            args = call.call_args.args[0]
            self.assertIn("--dry-run=server", args)
            self.assertEqual(args[args.index("-f") + 1], "-")
        with patch("psa_witness.subprocess.run", return_value=SimpleNamespace(
                returncode=1, stderr="Error from server (NotFound): unrelated", stdout="")):
            with self.assertRaisesRegex(RuntimeError, "missing supporting object") as error:
                dry_run(pod)
            self.assertNotIn("NotFound", str(error.exception))
        with patch("psa_witness.subprocess.run", return_value=SimpleNamespace(
                returncode=0, stderr="", stdout='{"kind":"Pod"}')):
            self.assertEqual(dry_run(pod), "accepted")

    def test_baseline_is_unscored_and_solved_state_uses_fresh_evidence(self):
        self.assertFalse(any(value is True for key, value in capture(self.kube, self.root, now=100).items()
                             if key != "failures"))
        labels = self.namespace["metadata"]["labels"]
        labels.update({"pod-security.kubernetes.io/enforce": "restricted",
                       "pod-security.kubernetes.io/enforce-version": "latest",
                       "pod-security.kubernetes.io/warn": "baseline"})
        calls = []

        def test(pod):
            calls.append(pod)
            return "psa_denied" if pod["metadata"]["generateName"].endswith("privileged-") else "accepted"

        sample(self.kube, test, self.state, lambda: 100)
        self.assertEqual(len(calls), 2)
        self.assertTrue(all(p["metadata"]["generateName"].startswith("cks001b-") for p in calls))
        evidence = json.loads((self.state / "psa-evidence.json").read_text())
        self.assertTrue(witness_valid(evidence, self.namespace, self.deployment, 101))
        self.assertEqual(capture(self.kube, self.root, now=101)["failures"], [])
        with self.assertRaisesRegex(RuntimeError, "stale"):
            capture(self.kube, self.root, now=131)
        self.namespace["metadata"]["resourceVersion"] = "8"
        with self.assertRaisesRegex(RuntimeError, "out of date"):
            capture(self.kube, self.root, now=101)

    def test_evidence_refresh_during_capture_does_not_look_future_dated(self):
        self.namespace["metadata"]["labels"].update({
            "pod-security.kubernetes.io/enforce": "restricted",
            "pod-security.kubernetes.io/enforce-version": "latest",
            "pod-security.kubernetes.io/warn": "baseline"})
        sample(self.kube, lambda obj: "psa_denied" if
               obj["metadata"]["generateName"].endswith("privileged-") else "accepted",
               self.state, lambda: 100)
        evidence = json.loads((self.state / "psa-evidence.json").read_text())
        clock = [99]

        def updated_evidence(_):
            clock[0] = 101
            return evidence

        with patch("psa_capture.json_file", side_effect=updated_evidence), \
                patch("psa_capture.time.time", side_effect=lambda: clock[0]):
            self.assertEqual(capture(self.kube, self.root)["failures"], [])

    def test_unstable_snapshot_never_overwrites_evidence(self):
        previous = self.state / "psa-evidence.json"
        previous.write_text('{"old":true}')
        first = copy.deepcopy(self.namespace)
        second = copy.deepcopy(self.namespace)
        second["metadata"]["resourceVersion"] = "8"
        original_get = self.kube.get
        versions = iter([first, second])

        def get(kind, name="", namespace=None, optional=False):
            if kind == "namespace" and name == "finance-ledger":
                return next(versions)
            return original_get(kind, name, namespace, optional)

        with patch.object(self.kube, "get", side_effect=get):
            with self.assertRaisesRegex(RuntimeError, "changed during"):
                sample(self.kube, lambda _: "accepted", self.state, lambda: 100)
        self.assertEqual(previous.read_text(), '{"old":true}')

    def test_dry_run_denial_of_the_workload_is_not_compliance(self):
        self.namespace["metadata"]["labels"]["pod-security.kubernetes.io/enforce"] = "restricted"
        sample(self.kube, lambda _: "psa_denied", self.state, lambda: 100)
        result = capture(self.kube, self.root, now=101)
        self.assertTrue(result["rejected"])
        self.assertFalse(result["compliant"])


if __name__ == "__main__":
    unittest.main()
