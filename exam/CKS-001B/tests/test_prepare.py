import os
import subprocess
import tempfile
import unittest
from pathlib import Path


class PrepareTests(unittest.TestCase):
    def test_incomplete_copy_refuses_all_phases_without_lab_access(self):
        script = Path(__file__).resolve().parents[1] / "prepare.sh"
        env = dict(os.environ, LAB_TEMPLATE_ID="9199")
        with tempfile.TemporaryDirectory(dir="/tmp/opencode") as tmp:
            isolated = Path(tmp) / "prepare.sh"
            isolated.write_text(script.read_text())
            for phase in ("preflight", "infrastructure", "fixtures"):
                result = subprocess.run(["bash", str(isolated), phase], env=env,
                                        capture_output=True, text=True, timeout=5)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("incomplete", result.stderr)
                self.assertIn("refusing preparation", result.stderr)

    def test_complete_preflight_is_read_only(self):
        script = Path(__file__).resolve().parents[1] / "prepare.sh"
        result = subprocess.run(["bash", str(script), "preflight"],
                                env=dict(os.environ, LAB_TEMPLATE_ID="9199"),
                                capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()
