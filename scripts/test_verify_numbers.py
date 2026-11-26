import importlib.util
import pathlib
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parent / "verify-numbers.py"
SPEC = importlib.util.spec_from_file_location("verify_numbers", SCRIPT)
verify_numbers = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(verify_numbers)


def fixtures():
    trials = []
    for tid, outcome, rate in (("t1", "pass", 10), ("t2", "fail", 20), ("t3", "pass", 30)):
        trials.append({
            "trial_id": tid,
            "outcome": outcome,
            "samples": {"latency": {"unit": "ms", "values": [1, 2, 3, 4, 100]}},
            "metrics": {"throughput": {"unit": "Mbps", "value": rate}},
        })
    raw = {"schema": verify_numbers.RAW_SCHEMA, "trials": trials}
    claims = {
        "schema": verify_numbers.CLAIMS_SCHEMA,
        "percentile_method": "nearest-rank",
        "tail_gate": "p99",
        "claims": [
            {"type": "percentile", "trial_id": "t1", "metric": "latency", "percentile": p,
             "value": value, "unit": "ms", **({"gate": "max", "max": {"value": 150, "unit": "ms"}} if p == "p99" else {})}
            for p, value in (("p50", 3), ("p95", 100), ("p99", 100))
        ] + [
            {"type": "arithmetic", "operation": "multiply",
             "inputs": [{"value": 10000, "unit": "viewers"}, {"value": 300, "unit": "kbps"}],
             "expected": {"value": 3, "unit": "Gbps"}},
            {"type": "trial-count", "trials": [
                {"trial_id": "t1", "outcome": "pass"},
                {"trial_id": "t2", "outcome": "fail"},
                {"trial_id": "t3", "outcome": "pass"}],
             "counts": {"pass": 2, "fail": 1}},
            {"type": "repeat-statistics", "metric": "throughput", "trial_ids": ["t1", "t2", "t3"],
             "min": {"value": 10, "unit": "Mbps"},
             "median": {"value": 20, "unit": "Mbps"},
             "max": {"value": 30, "unit": "Mbps"}},
        ],
    }
    return raw, claims


class VerifyNumbersTests(unittest.TestCase):
    def test_valid_claims(self):
        raw, claims = fixtures()
        self.assertEqual(len(verify_numbers.verify(raw, claims)), 6)

    def test_percentile_mismatch_rejected(self):
        raw, claims = fixtures()
        claims["claims"][0]["value"] = 2
        with self.assertRaisesRegex(verify_numbers.VerificationError, "raw samples recompute"):
            verify_numbers.verify(raw, claims)

    def test_unit_dimension_mismatch_rejected(self):
        raw, claims = fixtures()
        claims["claims"][0]["unit"] = "MB/s"
        with self.assertRaisesRegex(verify_numbers.VerificationError, "dimension mismatch"):
            verify_numbers.verify(raw, claims)

    def test_wrong_arithmetic_rejected(self):
        raw, claims = fixtures()
        claims["claims"][3]["expected"]["value"] = 3.2
        with self.assertRaisesRegex(verify_numbers.VerificationError, "expression computes"):
            verify_numbers.verify(raw, claims)

    def test_incomplete_trial_list_rejected(self):
        raw, claims = fixtures()
        claims["claims"][4]["trials"].pop()
        with self.assertRaisesRegex(verify_numbers.VerificationError, "every raw trial"):
            verify_numbers.verify(raw, claims)

    def test_wrong_repeat_median_rejected(self):
        raw, claims = fixtures()
        claims["claims"][5]["median"]["value"] = 25
        with self.assertRaisesRegex(verify_numbers.VerificationError, "repeat median"):
            verify_numbers.verify(raw, claims)

    def test_percentile_gate_requires_p99(self):
        raw, claims = fixtures()
        claims["claims"][0].update({"gate": "max", "max": {"value": 150, "unit": "ms"}})
        with self.assertRaisesRegex(verify_numbers.VerificationError, "latency gate must use p99"):
            verify_numbers.verify(raw, claims)

    def test_p99_gate_outcome_must_match_raw_data(self):
        raw, claims = fixtures()
        claims["claims"][2].update({"gate": "max", "max": {"value": 50, "unit": "ms"}})
        with self.assertRaisesRegex(verify_numbers.VerificationError, "trial outcome is pass"):
            verify_numbers.verify(raw, claims)

    def test_report_block_must_match_recomputed_claims(self):
        raw, claims = fixtures()
        output = verify_numbers.verify(raw, claims)
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8") as report:
            report.write("<!-- numeric-claims:start -->\n" + "\n".join(output) + "\n<!-- numeric-claims:end -->\n")
            report.flush()
            self.assertEqual(verify_numbers.verify_report(report.name, output),
                             "report numeric-claims block matches recomputed values")
            report.seek(0)
            report.truncate()
            report.write("<!-- numeric-claims:start -->\nwrong\n<!-- numeric-claims:end -->\n")
            report.flush()
            with self.assertRaisesRegex(verify_numbers.VerificationError, "disagrees with recomputed"):
                verify_numbers.verify_report(report.name, output)

    def test_legacy_aggregate_rejected(self):
        _, claims = fixtures()
        with self.assertRaisesRegex(verify_numbers.VerificationError, "aggregate/legacy"):
            verify_numbers.verify({"schema": "hami-media-benchmark/1", "trials": []}, claims)


if __name__ == "__main__":
    unittest.main()
