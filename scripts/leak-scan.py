#!/usr/bin/env python3
"""Leak scan gate (job 0.3). Check-only: fails on personal
identifiers or secret patterns, never rewrites anything.

Usage: python3 scripts/leak-scan.py [root]
Exit 0: no leaks. Exit 1: leaks found (listed with file:line).

Scanned: .go, .md, .mod, .sum, .yml, .yaml, .sh, .py files under
the repo root, except:
- scripts/leak-scan.py itself (it lists the patterns),
- docs/archive/joblist/**/*.log (raw test evidence; it contains
  sandbox paths from stack traces by design and is not shipped
  code — see docs/archive/joblist/cluster-a/README.md),
- .git/ (history is scanned separately, job 1.7, Mac-side).

Patterns come from repo-scrub-20261125.py (the scrub that produced
this tree) plus generic secret shapes. Test-only fixture keys in
*_test.go (e.g. the part3 HMAC test key) are not real secrets and
are not flagged; a real secret must never be added to a test.
"""
import re
import sys
from pathlib import Path

ROOT = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).resolve().parent.parent

EXTS = {".go", ".md", ".mod", ".sum", ".yml", ".yaml", ".sh", ".py"}
EXCLUDE_FILES = {"leak-scan.py", "leak-scan.sh"}  # both list the patterns
EXCLUDE_DIRS = {".git", "node_modules"}

# (pattern, label). Case-insensitive where noted by (?i).
PATTERNS = [
    (r"(?i)\bniroj\b", "personal name"),
    (r"(?i)\bnirajdon02\b", "personal email handle"),
    (r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}", "email address"),
    (r"/Users/", "macOS home path"),
    (r"/home/[A-Za-z0-9_.-]+/", "Linux home path"),
    (r"~/hami", "home path (hami)"),
    (r"~/workspace", "home path (workspace)"),
    (r"~/Downloads", "home path (Downloads)"),
    (r"/private/tmp", "private tmp path"),
    (r"Mac17,2", "exact machine model ID"),
    (r"(?i)darwin 25\.", "exact OS version"),
    (r"(?i)\bMuse\b", "internal agent name"),
    (r"(?i)ChatGPT", "external assistant name"),
    (r"M5-H", "chip-named test mode"),
    (r"M5H", "chip-named identifier (no hyphen)"),
    (r"(?i)hami loop", "internal runner name"),
    (r"BEGIN (PRIVATE|RSA|EC|OPENSSH|PGP) KEY", "private key block"),
    (r"AKIA[0-9A-Z]{16}", "AWS access key"),
    (r"ghp_[A-Za-z0-9]{20,}", "GitHub token"),
    (r"gho_[A-Za-z0-9]{20,}", "GitHub OAuth token"),
    (r"sk-[A-Za-z0-9]{20,}", "API secret key"),
    (r"(?i)(api[_-]?key|secret|password|token)\s*[:=]\s*['\"][^\r\n'\"]{8,}['\"]", "assigned secret value"),
]

# Values containing one of these markers are test fixtures (e.g.
# const token = "test token for pipe round trip", Password:
# "turn-test-password", Password: "definitely-the-wrong-password"
# in *_test.go), not real secrets. Anything else matching the
# assigned-secret pattern fails the scan.
TEST_FIXTURE_MARKERS = (
    "test",
    "fake",
    "dummy",
    "example",
    "placeholder",
    "changeme",
    "definitely-the-wrong",
)


def main() -> int:
    bad = 0
    scanned = 0
    for path in sorted(ROOT.rglob("*")):
        if not path.is_file():
            continue
        if any(part in EXCLUDE_DIRS for part in path.parts):
            continue
        if path.suffix not in EXTS:
            continue
        if path.name in EXCLUDE_FILES:
            continue
        if "docs/joblist" in path.relative_to(ROOT).parts and path.suffix == ".log":
            continue
        try:
            text = path.read_text(errors="strict")
        except (UnicodeDecodeError, OSError):
            continue
        scanned += 1
        for pattern, label in PATTERNS:
            for match in re.finditer(pattern, text):
                if label == "assigned secret value" and any(
                    marker in match.group(0).lower() for marker in TEST_FIXTURE_MARKERS
                ):
                    continue
                if label == "email address":
                    domain = match.group(0).rsplit("@", 1)[-1].lower()
                    if domain in ("example.com", "example.org", "example.net") or domain.endswith(".invalid"):
                        continue
                if label == "Linux home path" and "/home/alice/" in match.group(0):
                    continue
                line = text[: match.start()].count("\n") + 1
                print(f"LEAK {path.relative_to(ROOT)}:{line}: {label}: {pattern!r}")
                bad += 1
    fixture = ROOT / "main_test.go"
    if fixture.is_file():
        fixture_text = fixture.read_text(errors="strict")
        for value in ("256.44", "767.56"):
            if value in fixture_text:
                print(f"LEAK main_test.go: live swap fixture value {value}")
                bad += 1
    print(f"leak scan: {scanned} files, {bad} leak(s)")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
