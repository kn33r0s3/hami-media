import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parent / "load-test.py"
VALIDATOR = SCRIPT.parent / "validate-result.py"


class LoadTestRunnerTests(unittest.TestCase):
    def test_missing_video_fails_with_saved_diagnostics_without_building(self):
        with tempfile.TemporaryDirectory() as temporary:
            out = Path(temporary) / "evidence"
            result = subprocess.run(
                [sys.executable, str(SCRIPT), "--video", str(Path(temporary) / "missing.h264"),
                 "--out", str(out)],
                capture_output=True, text=True,
            )
            self.assertEqual(result.returncode, 2)
            self.assertIn("video file does not exist", result.stderr)
            manifest = json.loads((out / "run.json").read_text())
            self.assertEqual(manifest["status"], "setup-failed")
            self.assertEqual(manifest["parameters"]["total_simulated_viewers"], 1)
            self.assertTrue((out / "stderr.log").is_file())
            self.assertTrue((out / "build.log").is_file())
            validated = subprocess.run(
                [sys.executable, str(VALIDATOR), str(out / "run.json")],
                capture_output=True, text=True,
            )
            self.assertEqual(validated.returncode, 0, validated.stdout + validated.stderr)

    def test_high_load_requires_explicit_confirmation(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            video = root / "input.h264"
            video.write_bytes(b"\x00\x00\x01\x65")
            out = root / "evidence"
            result = subprocess.run(
                [sys.executable, str(SCRIPT), "--video", str(video), "--publishers", "10",
                 "--viewers-per-publisher", "500", "--binary", str(video),
                 "--out", str(out)],
                capture_output=True, text=True,
            )
            self.assertEqual(result.returncode, 2)
            self.assertIn("high-load run", result.stderr)
            self.assertEqual(json.loads((out / "run.json").read_text())["status"], "setup-failed")

    def test_successful_run_records_exact_invocation_and_logs(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            video = root / "input.h264"
            video.write_bytes(b"\x00\x00\x01\x65")
            fake_binary = root / "fake-hami"
            fake_binary.write_text(
                """#!/bin/sh
printf '%s\\n' '{"status":"PASS"}'
""",
                encoding="utf-8",
            )
            fake_binary.chmod(0o755)
            out = root / "evidence"
            result = subprocess.run(
                [sys.executable, str(SCRIPT), "--video", str(video), "--binary", str(fake_binary),
                 "--duration", "1s", "--out", str(out)],
                capture_output=True, text=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            manifest = json.loads((out / "run.json").read_text())
            self.assertEqual(manifest["status"], "pass")
            self.assertIn("-frame-rate", manifest["command"])
            self.assertEqual(manifest["parameters"]["frame_rate_fps"], 30)
            self.assertEqual(len(manifest["code_sha256_main_go"]), 64)
            self.assertEqual(len(manifest["runner_sha256"]), 64)
            self.assertEqual(len(manifest["binary_sha256"]), 64)
            self.assertIn('"status":"PASS"', (out / "result.json").read_text())
            self.assertEqual(manifest["raw_logs"], ["result.json", "stderr.log", "build.log"])
            validated = subprocess.run(
                [sys.executable, str(VALIDATOR), str(out / "run.json")],
                capture_output=True, text=True,
            )
            self.assertEqual(validated.returncode, 0, validated.stdout + validated.stderr)

    def test_hard_caps_are_enforced(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            video = root / "input.h264"
            video.write_bytes(b"fixture")
            out = root / "evidence"
            result = subprocess.run(
                [sys.executable, str(SCRIPT), "--video", str(video), "--publishers", "10",
                 "--viewers-per-publisher", "501", "--out", str(out)],
                capture_output=True, text=True,
            )
            self.assertEqual(result.returncode, 2)
            self.assertIn("viewers-per-publisher", result.stderr)
            self.assertEqual(json.loads((out / "run.json").read_text())["status"], "setup-failed")


if __name__ == "__main__":
    unittest.main()
