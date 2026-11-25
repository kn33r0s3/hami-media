#!/bin/sh
# Leak scan (R2) — the CI/pre-commit gate.
# Single source of truth is scripts/leak-scan.py (the pattern set
# from repo-scrub-20261125.py, plus secret shapes and the live-swap
# fixture check). This wrapper exists so CI and docs can call the
# stable `scripts/leak-scan.sh` entry point; it adds nothing.
# "Apple M5" is the APPROVED anonymised hardware-class label, not a
# leak; the exact model ID "Mac17,2" and OS build "darwin 25" are.
# Fails (exit 1) on any hit. Usage: scripts/leak-scan.sh [tree]
set -eu
TREE="${1:-.}"
exec python3 "$(dirname "$0")/leak-scan.py" "$TREE"
