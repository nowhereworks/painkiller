import copy
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
from sbom_capture import capture, valid_sbom
from test_workload_capture import FakeKube, deployment_ready, fixture


def spdx():
    return {"spdxVersion": "SPDX-2.3", "SPDXID": "SPDXRef-DOCUMENT",
            "documentNamespace": "https://example.test/spdx/id",
            "creationInfo": {"creators": ["Tool: bom"]}, "packages": [
                {"SPDXID": "SPDXRef-image", "name": "docker.io/alpine/curl:8.12.1",
                 "primaryPackagePurpose": "CONTAINER", "checksums": [
                     {"algorithm": "SHA256", "checksumValue": "image-digest"}]},
                {"SPDXID": "SPDXRef-curl", "name": "curl", "versionInfo": "8.12.1-r1"}],
            "relationships": [{"spdxElementId": "SPDXRef-image", "relationshipType": "CONTAINS",
                               "relatedSpdxElement": "SPDXRef-curl"}]}


class SbomCaptureTests(unittest.TestCase):
    def test_identity_package_and_relationship_are_required(self):
        expected = spdx()
        self.assertTrue(valid_sbom(spdx(), expected))
        for mutate in ("digest", "package", "relation", "malformed"):
            doc = spdx()
            if mutate == "digest":
                doc["packages"][0]["checksums"][0]["checksumValue"] = "different"
            if mutate == "package":
                doc["packages"][1]["versionInfo"] = "wrong"
            if mutate == "relation":
                doc["relationships"] = []
            if mutate == "malformed":
                doc["packages"][1]["name"] = []
            with self.subTest(mutate=mutate):
                self.assertFalse(valid_sbom(doc, expected))

    def test_actual_bom_report_when_supplied(self):
        filename = os.environ.get("CKS001B_BOM_REPORT")
        if not filename:
            self.skipTest("set CKS001B_BOM_REPORT to a real bom-generated curl image SBOM")
        doc = json.loads(Path(filename).read_text())
        self.assertTrue(valid_sbom(doc, copy.deepcopy(doc)))
        doc["relationships"] = []
        self.assertFalse(valid_sbom(doc, json.loads(Path(filename).read_text())))

    def test_capture_selected_container_and_missing_answers(self):
        with tempfile.TemporaryDirectory(dir="/tmp/opencode") as tmp:
            root = Path(tmp)
            state = root / "var/lib/painkiller/cks001b"
            home = root / "home/student"
            state.mkdir(parents=True)
            home.mkdir(parents=True)
            (state / "candidate.json").write_text(json.dumps({"user": "student", "home": "/home/student"}))
            (state / "expected-curl.spdx.json").write_text(json.dumps(spdx()))
            kube = FakeKube()
            deployment_ready(fixture("Deployment", "produce", "orchard"), kube, "produce")
            result = capture(kube, root)
            self.assertFalse(result["container"])
            self.assertFalse(result["sbom"])
            (home / "selected-produce.txt").write_text("plum\n")
            (home / "selected-produce.spdx.json").write_text(json.dumps(spdx()))
            self.assertEqual(capture(kube, root)["failures"], [])
            self.assertTrue(any(call[0] == "exec" and call[3] == ["apk", "info", "-e", "curl"]
                                for call in kube.calls))
            (home / "selected-produce.txt").write_text("pear\n")
            self.assertFalse(capture(kube, root)["container"])


if __name__ == "__main__":
    unittest.main()
