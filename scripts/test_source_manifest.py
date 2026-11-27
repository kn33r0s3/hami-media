#!/usr/bin/env python3

import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("source-manifest.py")


class SourceManifestTest(unittest.TestCase):
    def test_check_detects_source_changes(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            source = root / "example.go"
            source.write_text("package example\n")
            subprocess.run(["git", "-C", str(root), "add", "example.go"], check=True)
            generate = subprocess.run(
                ["python3", str(SCRIPT), "--root", str(root)],
                capture_output=True,
                text=True,
            )
            self.assertEqual(generate.returncode, 0, generate.stderr)
            check = subprocess.run(
                ["python3", str(SCRIPT), "--root", str(root), "--check"],
                capture_output=True,
                text=True,
            )
            self.assertEqual(check.returncode, 0, check.stderr)
            source.write_text("package example\n// changed\n")
            stale = subprocess.run(
                ["python3", str(SCRIPT), "--root", str(root), "--check"],
                capture_output=True,
                text=True,
            )
            self.assertEqual(stale.returncode, 1)


if __name__ == "__main__":
    unittest.main()
