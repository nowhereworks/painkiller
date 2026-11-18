import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
sys.path.insert(0, str(ROOT / "assets"))
from fixture_objects import objects
from network_policy import permits
from semantic_checks import dockerfile_checks, kubesec_report_valid, seccomp_checks
from solutions import SOLUTIONS, egress_exclusions, ingress, restricted_egress


class SolutionTests(unittest.TestCase):
    def test_every_question_has_bash_syntax_checked_solution(self):
        questions = json.loads((ROOT / "questions.json").read_text())
        self.assertEqual(set(SOLUTIONS), set(range(1, 17)))
        self.assertEqual(len(questions), len(SOLUTIONS))
        for number, solution in SOLUTIONS.items():
            with self.subTest(question=number):
                self.assertTrue(solution.strip())
                result = subprocess.run(["bash", "-n"], input=solution, text=True, capture_output=True, timeout=5)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_network_policy_reference_decisions(self):
        client = {"namespace": "outbound-guard", "pod_labels": {"app": "client"}}
        external = lambda ip: {"ip": ip, "namespace": None}
        for addr in ("198.18.20.10", "198.18.30.10"):
            self.assertFalse(permits([egress_exclusions], "Egress", client, external(addr), "TCP", 8081))
        self.assertTrue(permits([egress_exclusions], "Egress", client, external("198.18.50.10"), "TCP", 8081))
        self.assertTrue(permits([egress_exclusions], "Egress", client, external("10.96.0.10"), "UDP", 53))
        self.assertTrue(permits([egress_exclusions], "Egress", client, external("10.96.0.10"), "TCP", 53))
        self.assertTrue(permits([egress_exclusions], "Ingress", client, external("198.18.20.10"), "TCP", 80))
        client["namespace"] = "api-clients"
        self.assertTrue(permits([restricted_egress], "Egress", client, external("198.18.40.10"), "TCP", 9443))
        self.assertFalse(permits([restricted_egress], "Egress", client, external("198.18.30.10"), "TCP", 9443))
        self.assertTrue(permits([restricted_egress], "Egress", client, external("198.18.30.10"), "TCP", 443))
        self.assertFalse(permits([restricted_egress], "Egress", client, external("198.18.50.10"), "TCP", 8081))
        front = {"namespace": "storefront", "pod_labels": {"app": "shop-web"}}
        backend = {"namespace": "storefront", "pod_labels": {"app": "shop-api"}, "ip": "10.244.0.5"}
        self.assertTrue(permits([ingress], "Ingress", front, backend, "TCP", 80))
        backend["pod_labels"]["app"] = "untrusted"
        self.assertFalse(permits([ingress], "Ingress", front, backend, "TCP", 80))

    def test_rbac_solution_executes_against_fake_kubectl(self):
        role = next(obj for obj in objects() if obj["kind"] == "ClusterRole")
        with tempfile.TemporaryDirectory(dir="/tmp/opencode") as tmp:
            directory = Path(tmp)
            (directory / "role.json").write_text(json.dumps(role))
            program = directory / "kubectl"
            program.write_text("""#!/usr/bin/env python3
import json, pathlib, sys
root = pathlib.Path(__file__).parent
if sys.argv[1:4] == ['get', 'clusterrole', 'pipeline-agent-role']:
    print((root / 'role.json').read_text())
elif sys.argv[1:4] == ['patch', 'clusterrole', 'pipeline-agent-role']:
    (root / 'patch.json').write_text(sys.argv[sys.argv.index('-p') + 1])
elif sys.argv[1:3] == ['auth', 'can-i']:
    print('no')
    sys.exit(1)
else:
    sys.exit(2)
""")
            program.chmod(0o755)
            env = dict(os.environ, PATH=f"{tmp}:{os.environ['PATH']}")
            subprocess.run(["bash", "-e", "-o", "pipefail", "-c", SOLUTIONS[5]],
                           env=env, check=True, timeout=10, capture_output=True)
            rules = json.loads((directory / "patch.json").read_text())["rules"]
            self.assertEqual(rules[0], role["rules"][0])
            self.assertEqual(rules[1]["verbs"], ["get", "list", "watch"])

    def test_dockerfile_grading_semantics_and_negatives(self):
        good = ('FROM gcr.io/distroless/base\nCOPY ledger /usr/local/bin/ledger\n'
                'USER nonroot\nEXPOSE 8080\nENTRYPOINT ["/usr/local/bin/ledger"]\n')
        self.assertTrue(all(dockerfile_checks(good).values()))
        for original, replacement, key in [
            ("FROM gcr.io/distroless/base", "FROM ubuntu:20.04", "base"),
            ("COPY ledger /usr/local/bin/ledger", "COPY . /usr/local/bin/ledger", "binary"),
            ("USER nonroot", "USER root", "nonroot"),
            ('ENTRYPOINT ["/usr/local/bin/ledger"]', "ENTRYPOINT /usr/local/bin/ledger", "exec"),
            ("EXPOSE 8080", "RUN apt-get update", "minimal"),
        ]:
            with self.subTest(key=key):
                self.assertFalse(dockerfile_checks(good.replace(original, replacement))[key])
        self.assertFalse(dockerfile_checks(good + "# extra\n# extra\n")["minimal"])
        self.assertFalse(dockerfile_checks(good.replace("COPY ledger /usr/local/bin/ledger", 'COPY ["ledger",1]'))["binary"])
        self.assertFalse(dockerfile_checks(good.replace("EXPOSE 8080", 'CMD [1]'))["minimal"])

    def test_seccomp_grading_semantics_and_negative_cases(self):
        profile = {"defaultAction": "SCMP_ACT_ALLOW", "syscalls": [
            {"names": ["ptrace", "process_vm_readv"], "action": "SCMP_ACT_ERRNO"}]}
        self.assertTrue(all(seccomp_checks(profile).values()))
        bad = json.loads(json.dumps(profile))
        bad["syscalls"][0]["args"] = [{"index": 0, "op": "SCMP_CMP_EQ", "value": 1}]
        self.assertFalse(seccomp_checks(bad)["debug_blocked"])
        bad = json.loads(json.dumps(profile))
        bad["syscalls"][0]["errnoRet"] = 0
        self.assertFalse(seccomp_checks(bad)["debug_blocked"])
        bad = json.loads(json.dumps(profile))
        bad["syscalls"][0]["names"].append("open")
        self.assertFalse(seccomp_checks(bad)["other_allowed"])
        bad = json.loads(json.dumps(profile))
        bad["syscalls"].append({"names": ["ptrace"], "action": "SCMP_ACT_ALLOW"})
        self.assertFalse(seccomp_checks(bad)["debug_blocked"])

    def test_kubesec_report_grading(self):
        report = [{"object": "Pod/invoice-web.default", "valid": True,
                   "score": 4, "message": "Passed with a score of 4 points"}]
        self.assertTrue(kubesec_report_valid(report))
        for bad in ([], [{}], [{"object": "Pod/invoice-web.default", "valid": True,
                                "score": True, "message": "Passed"}]):
            self.assertFalse(kubesec_report_valid(bad))

    def test_real_kubesec_when_available(self):
        binary = os.environ.get("CKS001B_KUBESEC") or shutil.which("kubesec")
        if not binary:
            self.skipTest("pinned kubesec not on PATH; set CKS001B_KUBESEC")
        obj = next(o for o in objects() if o["kind"] == "Pod" and o["metadata"]["name"] == "invoice-web")
        obj["spec"]["containers"][0]["securityContext"]["capabilities"] = {
            "drop": ["ALL"], "add": ["NET_BIND_SERVICE"]}
        with tempfile.TemporaryDirectory(dir="/tmp/opencode") as tmp:
            filename = Path(tmp) / "invoice.yaml"
            filename.write_text(json.dumps(obj))
            result = subprocess.run([binary, "scan", str(filename)], capture_output=True, text=True,
                                    check=True, timeout=15)
            self.assertTrue(kubesec_report_valid(json.loads(result.stdout)))


if __name__ == "__main__":
    unittest.main()
