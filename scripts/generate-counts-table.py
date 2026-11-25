#!/usr/bin/env python3
"""Job 15.7 — Report generated from raw files.

Regenerates the REPORT.md section 4.5 counts table (20 x 8,000) from
the raw aggregate JSONs and checks it against the table printed in
REPORT.md. Each trial maps to a raw file; a trial with no raw file on
disk is reported RAW-MISSING, never invented. Per-trial figures use
the same recompute as recompute-vm-001.py (median of per-viewer p95 /
p99, frame histogram, delivered-frame cross-check).

Usage: generate-counts-table.py <report.md> [trial=path ...]
  Trial defaults (paths relative to the hami-media-work dir, or
  absolute): FINAL, verify-s005, gomem4g, gomem8g,
  verify-t0-repeat, verify-t0-repeat2, stall-capture.
  T0/T1/T2/T3/T4 have no raw JSON in the work dir as of 25 Nov 2026;
  they are reported RAW-MISSING and excluded from the match check.
Exit 0 = every present trial matches REPORT.md; 1 = mismatch or a
present trial disagrees with the report.
"""
import json, os, re, sys
from collections import Counter

def med(xs):
    xs = sorted(xs)
    n = len(xs)
    return xs[n // 2] if n % 2 else (xs[n // 2 - 1] + xs[n // 2]) / 2

DEFAULT_TRIALS = {
    "FINAL": "cap10x6000-final-20x8000-20261124.json",
    "verify-s005": "peakmem-verify-s005-20261124.json",
    "gomem4g": "peakmem-gomem4g-20261124.json",
    "gomem8g": "peakmem-gomem8g-20261124.json",
    "verify-t0-repeat": "peakmem-verify-t0-repeat-20261124.json",
    "verify-t0-repeat2": "peakmem-verify-t0-repeat2-20261124.json",
    "stall-capture": "peakmem-stall-20x8000-20261124.json",
}
MISSING_TRIALS = ["T0", "T1", "T2", "T3", "T4"]

def trial_stats(path):
    agg = json.load(open(path))
    vs = agg.get("viewers", [])
    p95 = [v["latency"]["p95_ns"] for v in vs if v.get("latency") and "p95_ns" in v["latency"]]
    p99 = [v["latency"]["p99_ns"] for v in vs if v.get("latency") and "p99_ns" in v["latency"]]
    frames = [v["latency"]["frames"] for v in vs if v.get("latency")]
    hist = dict(sorted(Counter(frames).items()))
    if len(hist) == 1:
        frames_summary = str({str(k): v for k, v in hist.items()})
    elif frames:
        frames_summary = ("range %d-%d, median %d, n=%d" %
                          (min(frames), max(frames), med(frames), len(frames)))
    else:
        frames_summary = "{}"
    delivered_ok = sum(frames) == agg.get("delivered_frames")
    n_pass = sum(1 for v in vs if v.get("latency") and v["latency"].get("pass"))
    verdict = "PASS" if (n_pass == len(vs) and len(vs) > 0) else "FAIL"
    return {"trial": os.path.basename(path), "viewers": len(vs),
            "p95_med_ms": round(med(p95) / 1e6, 2) if p95 else None,
            "p99_med_ms": round(med(p99) / 1e6, 2) if p99 else None,
            "frame_histogram": frames_summary,
            "delivered_frames": agg.get("delivered_frames"),
            "sum_per_viewer_frames": sum(frames),
            "delivered_crosscheck": delivered_ok,
            "pass_count": n_pass, "verdict": verdict}

def parse_report_table(text):
    rows = {}
    for m in re.finditer(r"\| (T\d|FINAL|verify-s005|gomem4g|gomem8g|verify-t0-repeat2?|stall-capture) \|[^|]*\| \*\*(\w+)", text):
        rows[m.group(1)] = m.group(2)
    return rows

def main():
    if len(sys.argv) < 2:
        print("usage: generate-counts-table.py <report.md> [trial=path ...]", file=sys.stderr)
        return 2
    report_path = sys.argv[1]
    trials = dict(DEFAULT_TRIALS)
    for arg in sys.argv[2:]:
        name, _, path = arg.partition("=")
        trials[name] = path
    workdir = os.path.dirname(os.path.abspath(report_path))
    print("| Trial | Verdict (from raw) | p95 med | p99 med | Frames | Delivered cross-check |")
    print("|---|---|---|---|---|---|")
    mismatches = []
    report_rows = parse_report_table(open(report_path).read())
    for name, rel in sorted(trials.items()):
        path = rel if os.path.isabs(rel) else os.path.join(workdir, rel)
        if not os.path.isfile(path):
            print(f"| {name} | RAW-MISSING | - | - | - | - |")
            continue
        s = trial_stats(path)
        print(f"| {name} | {s['verdict']} ({s['pass_count']}/{s['viewers']}) | "
              f"{s['p95_med_ms']} ms | {s['p99_med_ms']} ms | {s['frame_histogram']} | {s['delivered_crosscheck']} |")
        want = report_rows.get(name)
        if want and want.upper() != s["verdict"]:
            mismatches.append(f"{name}: report says {want}, raw files say {s['verdict']}")
        if not s["delivered_crosscheck"]:
            mismatches.append(f"{name}: delivered_frames cross-check failed")
    for name in MISSING_TRIALS:
        print(f"| {name} | RAW-MISSING (no raw JSON in work dir) | - | - | - | - |")
    if mismatches:
        print("MISMATCH:")
        for m in mismatches:
            print(f"  - {m}")
        return 1
    print("MATCH: every trial with a raw file agrees with REPORT.md; "
          "T0/T1/T2/T3/T4 remain RAW-MISSING (their JSONs are not in the work dir).")
    return 0

if __name__ == "__main__":
    sys.exit(main())
