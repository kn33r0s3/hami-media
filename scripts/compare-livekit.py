#!/usr/bin/env python3
"""Job 16.6 — Comparison generated from raw files.

Reads one hami-media result JSON and one LiveKit result JSON (both in
the R4 schema validated by scripts/validate-result.py) and prints a
comparison table against the preregistered win bar
(docs/benchmark-preregistration.json). Failures appear as failures:
a missing side, a failed trial, or a missing field is printed as
MISSING/FAIL, never silently dropped and never scored as a pass.

Usage: compare-livekit.py <hami-result.json> <livekit-result.json>
  Either path may be the literal word MISSING (side not run yet).
Exit 0 = table produced; exit 1 = a side is missing or invalid
(the table is still printed, so the gap is visible, not hidden).
"""
import json, sys

def load(path, side):
    if path == "MISSING":
        return None, f"{side}: MISSING (not run yet)"
    try:
        return json.load(open(path)), None
    except Exception as e:
        return None, f"{side}: UNREADABLE ({e})"

def main():
    if len(sys.argv) != 3:
        print("usage: compare-livekit.py <hami-result.json|MISSING> <livekit-result.json|MISSING>", file=sys.stderr)
        return 2
    hami, h_err = load(sys.argv[1], "hami-media")
    lk, l_err = load(sys.argv[2], "livekit")
    print("| Metric | hami-media | LiveKit | Win bar (16.2) |")
    print("|---|---|---|---|")
    def cell(doc, *keys):
        if doc is None:
            return "MISSING"
        v = doc
        for k in keys:
            v = v.get(k) if isinstance(v, dict) else None
            if v is None:
                return "MISSING"
        return str(v)
    rows = [
        ("trial_ids", ("trial_ids",), ("trial_ids",)),
        ("viewers at p99<=150ms", ("max_viewers_at_p99_150ms",), ("max_viewers_at_p99_150ms",)),
        ("CPU per subscriber per Mbps", ("cpu_per_sub_per_mbps",), ("cpu_per_sub_per_mbps",)),
        ("RSS per subscriber per Mbps", ("rss_per_sub_per_mbps",), ("rss_per_sub_per_mbps",)),
        ("p99 median (ms)", ("p99_med_ms",), ("p99_med_ms",)),
        ("failed trials", ("failed_trials",), ("failed_trials",)),
    ]
    bad = False
    for label, hk, lk_k in rows:
        h, l = cell(hami, *hk), cell(lk, *lk_k)
        if h in ("MISSING",) or l in ("MISSING",):
            bad = True
        print(f"| {label} | {h} | {l} | 2x (either leg) |")
    for e in (h_err, l_err):
        if e:
            print(f"NOTE: {e}")
            bad = True
    # Verdict: only computable when both sides present and no failures hidden.
    if hami is None or lk is None:
        print("VERDICT: UNDECIDED — a side is missing; no win/loss claim is made.")
        return 1
    hf = hami.get("failed_trials") or []
    lf = lk.get("failed_trials") or []
    if hf or lf:
        print(f"VERDICT: FAILURES PRESENT — hami failed trials: {hf}; livekit failed trials: {lf}. "
              "Failures are failures; the win bar is not met on a failed leg.")
        return 1
    print("VERDICT: see docs/decision-gate-a-16.7.md — the gate is decided from this table, not from prose.")
    return 0

if __name__ == "__main__":
    sys.exit(main())
