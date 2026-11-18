import os
import subprocess
import unittest
from pathlib import Path


class PrepareTests(unittest.TestCase):
    def test_incomplete_draft_refuses_all_phases_without_lab_access(self):
        script = Path(__file__).resolve().parents[1] / "prepare.sh"
        env = dict(os.environ, LAB_TEMPLATE_ID="9199")
        for phase in ("preflight", "infrastructure", "fixtures"):
            result = subprocess.run(["bash", str(script), phase], env=env,
                                    capture_output=True, text=True, timeout=5)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("incomplete", result.stderr)
            self.assertIn("refusing preparation", result.stderr)


if __name__ == "__main__":
    unittest.main()
