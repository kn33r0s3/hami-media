#!/usr/bin/env python3
"""Generate or check SHA-256 hashes for tracked source files."""

import argparse
import hashlib
import json
import subprocess
import sys
from pathlib import Path

MANIFEST_RELATIVE = Path("docs/source-manifest.json")


def tracked_source_paths(root: Path) -> list[Path]:
    result = subprocess.run(
        ["git", "-C", str(root), "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
        check=True,
        stdout=subprocess.PIPE,
    )
    paths = []
    for raw_path in result.stdout.split(b"\0"):
        if not raw_path:
            continue
        path = Path(raw_path.decode("utf-8"))
        if path == MANIFEST_RELATIVE or path.parts[0] == "results":
            continue
        if (root / path).is_file():
            paths.append(path)
    return paths


def build_manifest(root: Path) -> bytes:
    files = {}
    for relative_path in tracked_source_paths(root):
        files[relative_path.as_posix()] = hashlib.sha256(
            (root / relative_path).read_bytes()
        ).hexdigest()
    document = {"schema": "hami-media-source-manifest/v1", "files": files}
    return (json.dumps(document, indent=2, sort_keys=True) + "\n").encode("utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    root = args.root.resolve()
    manifest_path = root / MANIFEST_RELATIVE
    try:
        expected = build_manifest(root)
    except (OSError, subprocess.CalledProcessError) as error:
        print(f"source manifest: {error}", file=sys.stderr)
        return 2
    if args.check:
        try:
            actual = manifest_path.read_bytes()
        except OSError as error:
            print(f"source manifest: cannot read {MANIFEST_RELATIVE}: {error}", file=sys.stderr)
            return 1
        if actual != expected:
            print("source manifest is stale", file=sys.stderr)
            return 1
        print("source manifest: PASS")
        return 0
    manifest_path.parent.mkdir(parents=True, exist_ok=True)
    manifest_path.write_bytes(expected)
    print(f"source manifest written: {MANIFEST_RELATIVE}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
