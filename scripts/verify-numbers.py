#!/usr/bin/env python3
"""Recompute numeric report claims from raw Hami-media result JSON.

Usage: verify-numbers.py <raw-results.json> <claims.json>

Raw schema: hami-media-numeric-results/1. A trial has trial_id, outcome,
'samples' (metric -> {unit, values}), and 'metrics' (metric -> {unit, value}).
Claims schema: hami-media-numeric-claims/1, with percentile, arithmetic,
trial-count, and repeat-statistics claims. Percentiles use nearest rank:
sort samples ascending, select rank ceil(p*n/100), with ranks 1-indexed.
Every reported percentile metric/trial must provide p50, p95, and p99.
"""
from decimal import Decimal, InvalidOperation
import json
import math
import sys

RAW_SCHEMA = "hami-media-numeric-results/1"
CLAIMS_SCHEMA = "hami-media-numeric-claims/1"
PERCENTILES = {"p50": 50, "p95": 95, "p99": 99}
UNITS = {
    "1": ("scalar", Decimal("1")),
    "viewers": ("count", Decimal("1")),
    "requests": ("count", Decimal("1")),
    "frames": ("count", Decimal("1")),
    "bps": ("bit_rate", Decimal("1")),
    "kbps": ("bit_rate", Decimal("1000")),
    "Mbps": ("bit_rate", Decimal("1000000")),
    "Gbps": ("bit_rate", Decimal("1000000000")),
    "B/s": ("byte_rate", Decimal("1")),
    "kB/s": ("byte_rate", Decimal("1000")),
    "MB/s": ("byte_rate", Decimal("1000000")),
    "MiB/s": ("byte_rate", Decimal("1048576")),
    "B": ("bytes", Decimal("1")),
    "kB": ("bytes", Decimal("1000")),
    "MB": ("bytes", Decimal("1000000")),
    "MiB": ("bytes", Decimal("1048576")),
    "ns": ("time", Decimal("1")),
    "us": ("time", Decimal("1000")),
    "ms": ("time", Decimal("1000000")),
    "s": ("time", Decimal("1000000000")),
}

class VerificationError(Exception):
    pass

def require(condition, message):
    if not condition:
        raise VerificationError(message)

def decimal(value, label):
    require(isinstance(value, (int, float, Decimal)) and not isinstance(value, bool),
            f"{label}: expected a number")
    try:
        out = Decimal(str(value))
    except InvalidOperation:
        raise VerificationError(f"{label}: invalid number {value!r}")
    require(out.is_finite(), f"{label}: number must be finite")
    return out

def unit_info(unit, label):
    require(unit in UNITS, f"{label}: unsupported unit {unit!r}")
    return UNITS[unit]

def close_enough(actual, expected):
    return abs(actual - expected) <= max(Decimal("1e-12"), abs(expected) * Decimal("1e-9"))

def load_json(path):
    try:
        with open(path, encoding="utf-8") as stream:
            return json.load(stream, parse_float=Decimal, parse_int=Decimal)
    except (OSError, json.JSONDecodeError) as exc:
        raise VerificationError(f"cannot read {path}: {exc}")

def index_trials(raw):
    require(raw.get("schema") == RAW_SCHEMA,
            f"raw results must use schema {RAW_SCHEMA}; aggregate/legacy results lack raw numeric samples")
    trials = raw.get("trials")
    require(isinstance(trials, list) and trials, "raw results must contain a non-empty trials list")
    indexed = {}
    for i, trial in enumerate(trials):
        require(isinstance(trial, dict), f"trial[{i}] must be an object")
        tid = trial.get("trial_id")
        require(isinstance(tid, str) and tid.strip(), f"trial[{i}] has no valid trial_id")
        require(tid not in indexed, f"duplicate trial_id {tid!r}")
        require(trial.get("outcome") in ("pass", "fail"), f"trial {tid}: outcome must be pass or fail")
        for group in ("samples", "metrics"):
            require(isinstance(trial.get(group), dict), f"trial {tid}: {group} must be an object")
        indexed[tid] = trial
    return indexed

def verify_percentiles(claim, trials):
    tid, metric, percentile = claim.get("trial_id"), claim.get("metric"), claim.get("percentile")
    require(tid in trials, f"percentile claim references unknown trial {tid!r}")
    require(percentile in PERCENTILES, f"percentile claim has unsupported percentile {percentile!r}")
    if claim.get("gate") is not None:
        require(percentile == "p99", f"latency gate must use p99, not {percentile}")
        require(isinstance(claim.get("max"), dict), "p99 gate requires max {value, unit}")
    sample = trials[tid]["samples"].get(metric)
    require(isinstance(sample, dict), f"trial {tid}: missing raw sample metric {metric!r}")
    unit = sample.get("unit")
    dim, factor = unit_info(unit, f"trial {tid} sample {metric}")
    cdim, cfactor = unit_info(claim.get("unit"), "percentile claim")
    require(dim == cdim, f"{tid}/{metric}/{percentile}: unit dimension mismatch ({unit} vs {claim.get('unit')})")
    values = sample.get("values")
    require(isinstance(values, list) and values, f"trial {tid}: {metric} sample values must be non-empty")
    converted = sorted(decimal(v, f"{tid}/{metric} sample") * factor for v in values)
    rank = math.ceil(PERCENTILES[percentile] * len(converted) / 100)
    expected = converted[rank - 1] / cfactor
    actual = decimal(claim.get("value"), "percentile claim value")
    require(close_enough(actual, expected),
            f"{tid}/{metric}/{percentile}: report says {actual} {claim['unit']}, raw samples recompute to {expected} {claim['unit']}")
    if claim.get("gate") is not None:
        gate = claim["gate"]
        require(gate in ("max", "min"), "percentile gate must be 'max' or 'min'")
        max_obj = claim["max"]
        mdim, mfactor = unit_info(max_obj.get("unit"), "p99 gate maximum")
        require(mdim == cdim, "p99 gate maximum unit dimension mismatch")
        limit = decimal(max_obj.get("value"), "p99 gate maximum") * mfactor
        passed = expected * cfactor <= limit if gate == "max" else expected * cfactor >= limit
        require(passed or trials[tid]["outcome"] == "fail",
                f"{tid}: raw {percentile}={expected} {claim['unit']} fails gate {gate} {max_obj['value']} {max_obj['unit']}, but trial outcome is pass")
    return f"{tid} {metric} {percentile} = {expected} {claim['unit']} (nearest-rank)"

def verify_arithmetic(claim):
    op = claim.get("operation")
    inputs = claim.get("inputs")
    require(op in ("add", "subtract", "multiply", "divide"), f"arithmetic claim has unsupported operation {op!r}")
    require(isinstance(inputs, list) and len(inputs) >= (2 if op != "subtract" else 2),
            "arithmetic claim requires at least two inputs")
    quantities = []
    for i, item in enumerate(inputs):
        require(isinstance(item, dict), f"arithmetic input[{i}] must be an object")
        dim, factor = unit_info(item.get("unit"), f"arithmetic input[{i}]")
        quantities.append((decimal(item.get("value"), f"arithmetic input[{i}]") * factor, dim))
    value, dim = quantities[0]
    for other, odim in quantities[1:]:
        if op in ("add", "subtract"):
            require(dim == odim, f"cannot {op} incompatible unit dimensions {dim} and {odim}")
            value = value + other if op == "add" else value - other
        elif op == "multiply":
            require(dim == "count" or odim == "count" or dim == "scalar" or odim == "scalar",
                    f"unsupported compound multiplication {dim} x {odim}")
            dim = odim if dim in ("count", "scalar") else dim
            value *= other
        else:
            require(other != 0, "arithmetic division by zero")
            if odim in ("count", "scalar"):
                pass
            elif dim == odim:
                dim = "scalar"
            else:
                raise VerificationError(f"unsupported compound division {dim} / {odim}")
            value /= other
    expected = claim.get("expected")
    require(isinstance(expected, dict), "arithmetic claim requires expected {value, unit}")
    edim, efactor = unit_info(expected.get("unit"), "arithmetic expected value")
    require(dim == edim, f"arithmetic result has dimension {dim}, expected {edim}")
    reported = decimal(expected.get("value"), "arithmetic expected value") * efactor
    require(close_enough(value, reported),
            f"arithmetic claim disagrees: expression computes {value} base units, expected {reported}")
    return f"arithmetic {op} = {expected['value']} {expected['unit']}"

def verify_trial_counts(claim, trials):
    listed = claim.get("trials")
    require(isinstance(listed, list), "trial-count claim requires a full trials list")
    seen = []
    actual_counts = {"pass": 0, "fail": 0}
    for item in listed:
        require(isinstance(item, dict), "trial-count entry must be {trial_id, outcome}")
        tid, outcome = item.get("trial_id"), item.get("outcome")
        require(tid in trials, f"trial-count claim references unknown trial {tid!r}")
        require(tid not in seen, f"trial-count claim repeats trial {tid!r}")
        seen.append(tid)
        require(outcome == trials[tid]["outcome"],
                f"trial {tid}: report outcome {outcome!r}, raw outcome {trials[tid]['outcome']!r}")
        actual_counts[outcome] += 1
    require(set(seen) == set(trials), "trial-count claim must list every raw trial exactly once")
    expected = claim.get("counts")
    require(isinstance(expected, dict), "trial-count claim requires counts")
    for outcome in ("pass", "fail"):
        require(expected.get(outcome) == actual_counts[outcome],
                f"trial count {outcome}: report says {expected.get(outcome)!r}, raw trials count {actual_counts[outcome]}")
    return f"trials: {actual_counts['pass']} pass, {actual_counts['fail']} fail; all trial IDs listed"

def median(values):
    values = sorted(values)
    n = len(values)
    if n % 2:
        return values[n // 2]
    return (values[n // 2 - 1] + values[n // 2]) / 2

def verify_repeat_stats(claim, trials):
    ids = claim.get("trial_ids")
    require(isinstance(ids, list) and len(ids) >= 2, "repeat-statistics claim requires at least two trial IDs")
    require(len(set(ids)) == len(ids), "repeat-statistics claim contains duplicate trial IDs")
    values = []
    dim = None
    for tid in ids:
        require(tid in trials, f"repeat-statistics claim references unknown trial {tid!r}")
        metric = trials[tid]["metrics"].get(claim.get("metric"))
        require(isinstance(metric, dict), f"trial {tid}: missing metric {claim.get('metric')!r}")
        mdim, factor = unit_info(metric.get("unit"), f"trial {tid} metric")
        require(dim is None or dim == mdim, f"repeat metric units are dimensionally inconsistent at {tid}")
        dim = mdim
        values.append(decimal(metric.get("value"), f"trial {tid} metric") * factor)
    summaries = {"min": min(values), "median": median(values), "max": max(values)}
    for key, computed in summaries.items():
        reported = claim.get(key)
        require(isinstance(reported, dict), f"repeat-statistics claim requires {key} {{value, unit}}")
        rdim, factor = unit_info(reported.get("unit"), f"repeat {key}")
        require(rdim == dim, f"repeat {key} unit dimension mismatch")
        actual = decimal(reported.get("value"), f"repeat {key}") * factor
        require(close_enough(actual, computed),
                f"repeat {key}: report says {reported['value']} {reported['unit']}, raw trials recompute to {computed / factor} {reported['unit']}")
    return f"{claim['metric']} across {len(ids)} trials: median/min/max verified"

def verify(raw, claims):
    require(claims.get("schema") == CLAIMS_SCHEMA, f"claims must use schema {CLAIMS_SCHEMA}")
    require(claims.get("percentile_method") == "nearest-rank",
            "claims must state percentile_method='nearest-rank' (rank ceil(p*n), 1-indexed)")
    require(claims.get("tail_gate") == "p99", "claims must state tail_gate='p99'")
    trials = index_trials(raw)
    items = claims.get("claims")
    require(isinstance(items, list) and items, "claims must contain a non-empty claims list")
    percentile_sets = {}
    output = []
    for i, claim in enumerate(items):
        require(isinstance(claim, dict), f"claim[{i}] must be an object")
        kind = claim.get("type")
        if kind == "percentile":
            key = (claim.get("trial_id"), claim.get("metric"))
            percentile_sets.setdefault(key, set()).add(claim.get("percentile"))
            output.append(verify_percentiles(claim, trials))
        elif kind == "arithmetic":
            output.append(verify_arithmetic(claim))
        elif kind == "trial-count":
            output.append(verify_trial_counts(claim, trials))
        elif kind == "repeat-statistics":
            output.append(verify_repeat_stats(claim, trials))
        else:
            raise VerificationError(f"claim[{i}]: unsupported type {kind!r}")
    for key, present in percentile_sets.items():
        require(present == set(PERCENTILES),
                f"{key[0]}/{key[1]}: report must include p50, p95, and p99 claims; found {sorted(present)}")
    return output

def verify_report(report_path, results):
    try:
        with open(report_path, encoding="utf-8") as stream:
            report = stream.read()
    except OSError as exc:
        raise VerificationError(f"cannot read report {report_path}: {exc}")
    marker = "<!-- numeric-claims:start -->"
    end_marker = "<!-- numeric-claims:end -->"
    require(report.count(marker) == 1 and report.count(end_marker) == 1,
            f"report must contain exactly one {marker} ... {end_marker} block")
    start = report.index(marker) + len(marker)
    end = report.index(end_marker)
    require(end >= start, "numeric-claims report markers are out of order")
    written = [line.strip() for line in report[start:end].splitlines() if line.strip()]
    require(written == results,
            "report numeric-claims block disagrees with recomputed claims; replace its lines with:\\n" +
            "\\n".join(results))
    return "report numeric-claims block matches recomputed values"

def main(argv):
    if len(argv) != 4:
        print("usage: verify-numbers.py <raw-results.json> <claims.json> <report.md>", file=sys.stderr)
        return 2
    try:
        results = verify(load_json(argv[1]), load_json(argv[2]))
        results.append(verify_report(argv[3], results))
        for line in results:
            print("VERIFIED:", line)
    except VerificationError as exc:
        print(f"INVALID: {exc}", file=sys.stderr)
        return 1
    print("PASS: every marked report claim matches the listed raw trials and samples")
    return 0

if __name__ == "__main__":
    sys.exit(main(sys.argv))
