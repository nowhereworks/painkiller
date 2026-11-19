"""Validate the authoring manifest; this does not validate a working exam."""
import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class SpecTests(unittest.TestCase):
    def test_metadata_and_source_weights(self):
        metadata = json.loads((ROOT / "test.json").read_text())
        questions = json.loads((ROOT / "questions.json").read_text())
        self.assertEqual(metadata["id"], "CKS-001B")
        self.assertEqual(metadata["minutes"], 120)
        self.assertEqual((metadata["lab"]["controllers"], metadata["lab"]["workers"]), (1, 0))
        self.assertEqual([q["points"] for q in questions],
                         [5, 5, 10, 7, 6, 8, 5, 9, 10, 4, 7, 4, 7, 7, 2, 4])
        self.assertEqual(sum(q["points"] for q in questions), 100)
        self.assertEqual(len({q["id"] for q in questions}), 16)
        self.assertEqual([q["question_id"] for q in questions],
                         [f"CKS-001B-Q{i}" for i in range(1, 17)])
        for q in questions:
            self.assertIn(q["category"], metadata["categories"])
            self.assertTrue(q["instructions"])


if __name__ == "__main__":
    unittest.main()
