#!/usr/bin/env python3

import json
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("write-test-summary.py")


class WriteTestSummaryTest(unittest.TestCase):
    def test_summary_is_validator_compatible(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            run_dir = Path(temporary_directory)
            (run_dir / "steps.tsv").write_text("trial_id\t20261127T000000Z-test\nunit\tPASS\noverall\tPASS\n")
            (run_dir / "unit.txt").write_text("unit output\n")
            generated = subprocess.run(
                ["python3", str(SCRIPT), str(run_dir)],
                capture_output=True,
                text=True,
            )
            self.assertEqual(generated.returncode, 0, generated.stderr)
            summary = run_dir / "summary.json"
            json.loads(summary.read_text())
            validated = subprocess.run(
                ["python3", str(Path(__file__).with_name("validate-result.py")), str(summary)],
                capture_output=True,
                text=True,
            )
            self.assertEqual(validated.returncode, 0, validated.stdout + validated.stderr)


if __name__ == "__main__":
    unittest.main()
