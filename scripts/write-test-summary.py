#!/usr/bin/env python3
"""Write a result JSON for scripts/test-all.sh."""

import hashlib
import json
import platform
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: write-test-summary.py <run-directory>", file=sys.stderr)
        return 2
    run_dir = Path(sys.argv[1]).resolve()
    root = Path(__file__).resolve().parent.parent
    try:
        steps = {}
        trial_id = ""
        for line in (run_dir / "steps.tsv").read_text().splitlines():
            name, value = line.split("\t", 1)
            if name == "trial_id":
                trial_id = value
            else:
                steps[name] = value
        if not trial_id:
            raise ValueError("trial ID is missing")
        raw_logs = sorted(path.name for path in run_dir.glob("*.txt"))
        summary = {
            "schema": "hami-media-test-all/v1",
            "trial_ids": [trial_id],
            "code_sha256_main_go": sha256(root / "main.go"),
            "source_manifest_sha256": sha256(root / "docs/source-manifest.json")
            if (root / "docs/source-manifest.json").is_file()
            else None,
            "image_hash": "n/a-native",
            "started_at_utc": datetime.strptime(trial_id[:16], "%Y%m%dT%H%M%SZ").replace(tzinfo=timezone.utc).isoformat(),
            "environment": {
                "platform": platform.platform(),
                "go_version": subprocess.run(["go", "version"], check=True, capture_output=True, text=True).stdout.strip(),
            },
            "parameters": {
                "runner": "scripts/test-all.sh",
                "fuzz_time_per_target": "10s",
            },
            "steps": steps,
            "status": steps.get("overall", "UNKNOWN"),
            "raw_logs": raw_logs,
            "raw_log_sha256": {name: sha256(run_dir / name) for name in raw_logs},
        }
        (run_dir / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"test summary: {error}", file=sys.stderr)
        return 1
    print(f"test summary written: {run_dir / 'summary.json'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
