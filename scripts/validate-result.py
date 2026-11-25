#!/usr/bin/env python3
"""Job 15.6 — Result schema validator (R4).

Rejects any result JSON missing the code sha, image hash, parameters,
trial IDs or raw logs. Exit 0 = valid, exit 1 = invalid (lists every
missing field). The image hash may be "n/a-native" for native-process
runs (no container image exists); that value must be explicit, never
absent. Raw-log paths must exist relative to the result file's dir
(or be absolute); a missing raw log file is a rejection.

Usage: validate-result.py <result.json> [<result.json> ...]
"""
import json, os, sys

REQUIRED = ["code_sha256_main_go", "image_hash", "parameters",
            "trial_ids", "raw_logs"]

def validate(path):
    errors = []
    try:
        doc = json.load(open(path))
    except Exception as e:
        return [f"{path}: not valid JSON: {e}"]
    base = os.path.dirname(os.path.abspath(path))
    for field in REQUIRED:
        if field not in doc or doc[field] in (None, "", [], {}):
            errors.append(f"{path}: missing or empty required field {field!r}")
    sha = doc.get("code_sha256_main_go", "")
    if sha and (len(sha) != 64 or any(c not in "0123456789abcdef" for c in sha)):
        errors.append(f"{path}: code_sha256_main_go is not a 64-hex sha256: {sha!r}")
    trials = doc.get("trial_ids")
    if isinstance(trials, list):
        for t in trials:
            if not isinstance(t, str) or not t.strip():
                errors.append(f"{path}: trial_ids contains a non-string/empty entry: {t!r}")
    logs = doc.get("raw_logs")
    if isinstance(logs, list):
        for entry in logs:
            p = entry if os.path.isabs(entry) else os.path.join(base, entry)
            if not os.path.isfile(p):
                errors.append(f"{path}: raw log file missing: {entry!r}")
    elif logs is not None and not isinstance(logs, list):
        errors.append(f"{path}: raw_logs must be a list of paths")
    return errors

def main():
    if len(sys.argv) < 2:
        print("usage: validate-result.py <result.json> [...]", file=sys.stderr)
        return 2
    all_errors = []
    for path in sys.argv[1:]:
        all_errors.extend(validate(path))
    if all_errors:
        print("INVALID:")
        for e in all_errors:
            print(f"  - {e}")
        return 1
    print(f"VALID: {len(sys.argv) - 1} result file(s)")
    return 0

if __name__ == "__main__":
    sys.exit(main())
