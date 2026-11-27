#!/usr/bin/env python3
"""Convert go test -json output into a compact baseline table."""

import json
import sys
from pathlib import Path


def read_statuses(path: Path) -> dict[str, str]:
    statuses: dict[str, str] = {}
    for line in path.read_text().splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        name = event.get("Test")
        action = event.get("Action")
        if name and action in ("pass", "fail", "skip"):
            statuses[name] = action
    return statuses


def main() -> int:
    if len(sys.argv) != 4:
        print("usage: baseline-json.py <tests.jsonl> <nack.jsonl> <output.md>", file=sys.stderr)
        return 2
    tests = read_statuses(Path(sys.argv[1]))
    nack = read_statuses(Path(sys.argv[2]))
    tests.update(nack)
    counts = {
        status: sum(value == status for value in tests.values())
        for status in ("pass", "fail", "skip")
    }
    lines = [
        "# Linux baseline test table",
        "",
        f"PASS: {counts['pass']}  FAIL: {counts['fail']}  SKIP: {counts['skip']}",
        "",
        "| Test | Status |",
        "|---|---|",
    ]
    lines.extend(f"| `{name}` | {status} |" for name, status in sorted(tests.items()))
    output = Path(sys.argv[3])
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text("\n".join(lines) + "\n")
    print(f"baseline table written: {output}")
    return 1 if counts["fail"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
