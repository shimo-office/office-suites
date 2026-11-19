"""A report must not declare completion while global acceptance is pending."""
import tempfile
import unittest
import copy
import json
from pathlib import Path

from build_report import Report, prepare


class ReportCompletionTest(unittest.TestCase):
    def data(self, globals_=None):
        counts = (3, 2, 2, 2, 2, 2, 3, 3, 2, 1, 2, 1, 2, 1)
        return {
            "scenes": [{"number": number, "capabilities": [
                {"id": f"{number}.{index}", "status": "passed"}
                for index in range(1, count + 1)
            ]} for number, count in zip(range(29, 43), counts)],
            "globalNegativeCases": globals_ if globals_ is not None else [
                {"id": f"G{index}", "status": "passed"} for index in range(1, 6)
            ],
        }

    def report(self, globals_):
        return self.from_data(self.data(globals_))

    def from_data(self, data):
        return Report(data, Path(tempfile.gettempdir()) / "unused-phase3-report.pdf", "test")

    def test_missing_function_cannot_declare_completion(self):
        data = self.data()
        data["scenes"][-1]["capabilities"] = []
        self.assertFalse(self.from_data(data).complete)

    def test_duplicate_function_cannot_substitute_for_missing_function(self):
        data = self.data()
        data["scenes"][-1]["capabilities"] = copy.deepcopy(data["scenes"][-2]["capabilities"][:1])
        self.assertFalse(self.from_data(data).complete)

    def test_single_global_pass_cannot_substitute_for_all_global_cases(self):
        self.assertFalse(self.report([{"id": "G2", "status": "passed"}]).complete)

    def test_duplicate_global_cannot_substitute_for_missing_global(self):
        data = self.data()
        data["globalNegativeCases"][-1] = copy.deepcopy(data["globalNegativeCases"][0])
        self.assertFalse(self.from_data(data).complete)

    def test_function_cannot_be_assigned_to_the_wrong_scene(self):
        data = self.data()
        data["scenes"][0]["capabilities"], data["scenes"][1]["capabilities"] = (
            data["scenes"][1]["capabilities"], data["scenes"][0]["capabilities"])
        self.assertFalse(self.from_data(data).complete)

    def test_unknown_extra_global_cannot_pass_completion(self):
        data = self.data()
        data["globalNegativeCases"].append({"id": "G6", "status": "passed"})
        self.assertFalse(self.from_data(data).complete)

    def test_unknown_extra_function_cannot_pass_completion(self):
        data = self.data()
        data["scenes"][-1]["capabilities"].append({"id": "42.2", "status": "passed"})
        self.assertFalse(self.from_data(data).complete)

    def test_all_functions_pass_but_second_account_remains_pending(self):
        data = {
            "id": "G2", "status": "pending", "reason": "No second account",
        }
        fixture = self.data()
        fixture["globalNegativeCases"][1] = data
        report = self.from_data(fixture)
        self.assertEqual(report.counts["passed"], 28)
        self.assertFalse(report.complete)

    def test_missing_global_results_cannot_declare_completion(self):
        self.assertFalse(self.report([]).complete)

    def test_functions_and_global_results_all_pass(self):
        self.assertTrue(self.from_data(self.data()).complete)


class GlobalEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "proof.json").write_text('{"observed":true}')
        self.data = ReportCompletionTest().data()
        for scene in self.data["scenes"]:
            for case in scene["capabilities"]:
                case.update(self.claim(exit_code=0))
                case.update(expected=["真实持久化结果"], evidenceRequired=["proof"])
        for case in self.data["globalNegativeCases"]:
            case.update(self.claim(exit_code=1))
            case["expected"] = "真实失败且不返回成功"

    def claim(self, exit_code):
        return dict(actualCommand=["shimo-net", "phase3", "test-fixture"],
                    observedResult="专用测试物证", executedAt="2026-11-19T03:00:00+08:00",
                    targetGuid="test-fixture", cliVersion="test-build", exitCode=exit_code,
                    evidence=[{"name": "proof", "path": "proof.json"}],
                    assertions=[{"description": "核对预期结果", "passed": True}])

    def prepared(self, updates=None):
        source = self.root / "cases.json"
        source.write_text(json.dumps(self.data))
        manifest = self.root / "manifest.json"
        manifest.write_text(json.dumps({"cases": updates or {}}))
        return prepare(source, manifest)

    def test_global_status_alone_does_not_prove_completion(self):
        self.data["globalNegativeCases"][0] = dict(id="G1", status="passed", expected="拒绝写入")
        with self.assertRaises(ValueError):
            self.prepared()

    def test_each_global_execution_field_is_required(self):
        original = copy.deepcopy(self.data)
        for key in ("actualCommand", "observedResult", "executedAt", "targetGuid", "cliVersion"):
            with self.subTest(key=key):
                self.data = copy.deepcopy(original)
                del self.data["globalNegativeCases"][0][key]
                with self.assertRaises(ValueError):
                    self.prepared()

    def test_global_requires_real_nonzero_integer_exit(self):
        for invalid in (0, False, True, "1", "failure", None, 1.5):
            with self.subTest(exit_code=invalid):
                self.data["globalNegativeCases"][0]["exitCode"] = invalid
                with self.assertRaises(ValueError):
                    self.prepared()

    def test_global_requires_nonempty_existing_evidence(self):
        for evidence in ([], [{"name": "missing", "path": "missing.json"}]):
            self.data["globalNegativeCases"][0]["evidence"] = evidence
            with self.assertRaises(ValueError):
                self.prepared()

    def test_global_requires_one_described_true_assertion(self):
        for assertions in ([], [{"passed": True}], [{"description": "拒绝", "passed": 1}],
                           [{"description": "拒绝", "passed": False}]):
            self.data["globalNegativeCases"][0]["assertions"] = assertions
            with self.assertRaises(ValueError):
                self.prepared()

    def test_pending_global_does_not_need_fake_evidence(self):
        self.data["globalNegativeCases"][1] = dict(id="G2", status="pending", expected="冲突时拒绝")
        prepared = self.prepared()
        self.assertFalse(Report(prepared, self.root / "unused.pdf", "test").complete)

    def test_manifest_can_supply_global_evidence_but_not_rewrite_requirement(self):
        self.data["globalNegativeCases"][1] = dict(id="G2", status="pending", expected="冲突时拒绝")
        update = dict(self.claim(exit_code=1), status="passed")
        prepared = self.prepared({"G2": update})
        self.assertTrue(Report(prepared, self.root / "unused.pdf", "test").complete)
        with self.assertRaises(ValueError):
            self.prepared({"G2": dict(update, expected="不用冲突检测")})

    def test_deferral_is_not_a_pass_or_a_global_bypass(self):
        update = dict(status="deferred", reason="用户将关联字段留待开放", deferral={
            "requestedBy": "user", "requestedAt": "2026-11-19",
            "quote": "先做保留项，后续开放后再支持，不算阻塞",
            "scope": "37.2 关联字段子功能", "releaseBlocking": False,
        })
        data = self.prepared({"37.2": update})
        report = Report(data, self.root / "unused.pdf", "test")
        self.assertEqual(report.counts["passed"], 27)
        self.assertEqual(report.counts["deferred"], 1)
        self.assertFalse(report.complete)  # Original requirements remain incomplete.
        self.assertTrue(report.release_scope_complete)
        with self.assertRaises(ValueError):
            self.prepared({"G3": update})

    def test_deferral_requires_explicit_scope_and_user_decision(self):
        with self.assertRaises(ValueError):
            self.prepared({"37.2": {"status": "deferred", "reason": "environment"}})
        update = {"status": "deferred", "reason": "environment", "deferral": {
            "requestedBy": "agent", "requestedAt": "2026-11-19", "quote": "skip",
            "scope": "association", "releaseBlocking": False,
        }}
        with self.assertRaises(ValueError):
            self.prepared({"37.2": update})


if __name__ == "__main__":
    unittest.main()
