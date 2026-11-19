"""Exercise the release gate with weighted Go coverprofile fixtures."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("check_coverage.py")


class CoverageGateTests(unittest.TestCase):
    def run_gate(self, profile):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "coverage.out"
            if profile is not None:
                path.write_text(profile, encoding="utf-8")
            return subprocess.run([sys.executable, str(SCRIPT), str(path)],
                                  text=True, capture_output=True)

    def test_statement_weighting_rejects_most_blocks_but_few_statements(self):
        result = self.run_gate("mode: set\na.go:1.1,2.1 1 1\n"
                               "a.go:3.1,4.1 1 1\na.go:5.1,6.1 98 0\n")
        self.assertEqual(result.returncode, 1, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["coveredStatements"], 2)
        self.assertEqual(report["totalStatements"], 100)
        self.assertFalse(report["passed"])

    def test_exactly_eighty_percent_passes(self):
        result = self.run_gate("mode: set\na.go:1.1,2.1 8 1\nb.go:1.1,2.1 2 0\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertTrue(report["passed"])
        self.assertEqual(report["minimumPercent"], 80)

    def test_rounding_cannot_promote_below_threshold(self):
        result = self.run_gate("mode: count\na.go:1.1,2.1 79999 4\nb.go:1.1,2.1 20001 0\n")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertFalse(json.loads(result.stdout)["passed"])

    def test_count_and_atomic_modes_count_covered_statements_once(self):
        for mode in ("count", "atomic"):
            with self.subTest(mode=mode):
                result = self.run_gate(f"mode: {mode}\na.go:1.1,2.1 8 200\nb.go:1.1,2.1 2 0\n")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(json.loads(result.stdout)["coveredStatements"], 8)

    def test_windows_path_and_zero_statement_blocks_are_valid(self):
        result = self.run_gate("mode: set\nC:/source with spaces/a.go:1.1,2.1 8 1\n"
                               "C:/source with spaces/a.go:3.1,4.1 0 0\n"
                               "C:/source with spaces/b.go:1.1,2.1 2 0\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["totalStatements"], 10)

    def test_missing_empty_and_zero_statement_profiles_are_errors(self):
        for profile in (None, "", "mode: set\n", "mode: set\na.go:1.1,2.1 0 1\n"):
            with self.subTest(profile=profile):
                result = self.run_gate(profile)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertFalse(json.loads(result.stderr)["passed"])
                self.assertEqual(result.stdout, "")

    def test_invalid_header_or_truncated_and_malformed_records_are_errors(self):
        profiles = ("mode: unknown\na.go:1.1,2.1 1 1\n", "a.go:1.1,2.1 1 1\n",
                    "mode: set\na.go:1.1,2.1 1\n", "mode: set\na.go:1.1,2.1 -1 1\n",
                    "mode: set\na.go:1.1,2.1 1 -1\n", "mode: set\na.go:1.1,2.1 1 2\n",
                    "mode: set\na.go:2.1,1.1 1 1\n", "mode: set\na.go:0.1,2.1 1 1\n",
                    "mode: set\na.go:1.0,2.1 1 1\n", "mode: set\n\na.go:1.1,2.1 1 1\n")
        for profile in profiles:
            with self.subTest(profile=profile):
                result = self.run_gate(profile)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertFalse(json.loads(result.stderr)["passed"])

    def test_duplicate_blocks_are_rejected_instead_of_inflating_weight(self):
        result = self.run_gate("mode: set\na.go:1.1,2.1 8 1\na.go:1.1,2.1 8 1\nb.go:1.1,2.1 3 0\n")
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("duplicate", json.loads(result.stderr)["error"].lower())


if __name__ == "__main__":
    unittest.main()
