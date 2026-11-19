import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
from capture_files import capture


class FileCapturesTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir="/tmp/opencode")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)

    def write(self, name, content):
        filename = self.root / name
        filename.parent.mkdir(parents=True, exist_ok=True)
        filename.write_text(content)
        return filename

    def test_missing_and_correct_answers(self):
        for question in ("q7", "q8", "q10"):
            result = capture(question, self.root)
            self.assertTrue(result["failures"])
            self.assertFalse(any(value is True for key, value in result.items() if key != "failures"))
        self.write("opt/course/ledger/ledger.Dockerfile",
                   'FROM gcr.io/distroless/base\nCOPY ledger /usr/local/bin/ledger\n'
                   'USER nonroot\nEXPOSE 8080\nENTRYPOINT ["/usr/local/bin/ledger"]\n')
        self.write("root/CKS/invoice-kubesec.json", json.dumps([{
            "object": "Pod/invoice-web.default", "score": 4, "valid": True,
            "message": "Passed with a score of 4 points"}]))
        self.write("var/lib/kubelet/seccomp/profiles/no-inspect.json", json.dumps({
            "defaultAction": "SCMP_ACT_ALLOW", "syscalls": [{
                "names": ["ptrace", "process_vm_readv"], "action": "SCMP_ACT_ERRNO"}]}))
        for question in ("q7", "q8", "q10"):
            self.assertEqual(capture(question, self.root)["failures"], [])

    def test_invalid_oversized_and_transport_errors(self):
        path = self.write("root/CKS/invoice-kubesec.json", "not JSON")
        self.assertFalse(capture("q8", self.root)["report"])
        path.write_text("x" * 1048577)
        self.assertFalse(capture("q8", self.root)["report"])
        path.unlink()
        path.mkdir()
        with self.assertRaises(IsADirectoryError):
            capture("q8", self.root)


if __name__ == "__main__":
    unittest.main()
