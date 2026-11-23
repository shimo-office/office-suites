#!/usr/bin/env python3
"""Offline behaviour of the packaged write-plan helper."""
import importlib.util
import json
import os
from pathlib import Path
import stat
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location('plan', ROOT / 'docs/agent/plan.py')
plan = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(plan)


class PlanTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self.tmp.name)
        self.csv = self.dir / '课表.csv'
        self.csv.write_text('科目,时间,教室\n数学,周一 09:00,A101\n英语,周二 10:00,B202\n,\n',
                            encoding='utf-8')

    def tearDown(self):
        self.tmp.cleanup()

    def build(self, **kwargs):
        options = {'sheet': '工作表1', 'raw_range': 'A1', 'out_dir': self.dir / 'output'}
        options.update(kwargs)
        return plan.build(self.csv, options['sheet'], options['raw_range'], options['out_dir'])

    def test_anchor_range_derives_the_written_area(self):
        result = self.build()
        self.assertTrue(result['ok'])
        self.assertFalse(result['written'])
        self.assertEqual(result['range'], '工作表1!A1:C3')
        self.assertEqual((result['rows'], result['columns']), (3, 3))
        payload = json.loads(Path(result['files']['payload']).read_text(encoding='utf-8'))
        self.assertEqual(payload['range'], '工作表1!A1:C3')
        self.assertEqual(payload['resource']['values'][1], ['数学', '周一 09:00', 'A101'])

    def test_explicit_range_must_match_the_data(self):
        result = self.build(raw_range='工作表1!A1:C3')
        self.assertEqual(result['range'], '工作表1!A1:C3')

    def test_range_and_data_mismatch_is_rejected(self):
        with self.assertRaises(plan.PlanError) as caught:
            self.build(raw_range='A1:B2')
        self.assertIn('3 行 × 3 列', str(caught.exception))

    def test_conflicting_sheet_names_are_rejected(self):
        with self.assertRaises(plan.PlanError):
            self.build(raw_range='工作表2!A1:C3')

    def test_missing_sheet_is_rejected(self):
        with self.assertRaises(plan.PlanError):
            self.build(sheet='')

    def test_bad_range_is_rejected(self):
        for raw in ('A0', '1A', 'A1:', 'A1:C0', 'C3:A1'):
            with self.subTest(raw=raw), self.assertRaises(plan.PlanError):
                self.build(raw_range=raw)

    def test_trailing_blank_rows_and_columns_are_trimmed(self):
        result = self.build()
        self.assertTrue(any('空' in warning for warning in result['warnings']))
        self.assertEqual(result['rows'], 3)
        self.assertEqual(result['columns'], 3)

    def test_ragged_rows_are_padded_to_one_width(self):
        self.csv.write_text('a,b,c\n1,2\n', encoding='utf-8')
        result = self.build()
        payload = json.loads(Path(result['files']['payload']).read_text(encoding='utf-8'))
        self.assertEqual(payload['resource']['values'], [['a', 'b', 'c'], ['1', '2', '']])
        self.assertEqual(result['columns'], 3)

    def test_generated_script_is_executable_and_never_hardcodes_a_link(self):
        result = self.build()
        script = Path(result['files']['script'])
        # Windows has no executable bit on files; chmod only toggles read-only.
        if os.name != 'nt':
            self.assertTrue(script.stat().st_mode & stat.S_IXUSR)
        text = script.read_text(encoding='utf-8')
        self.assertIn('--action sheet-update', text)
        self.assertIn('payload.json', text)
        self.assertNotIn('shimo.net/', text)

    def test_empty_file_is_rejected(self):
        self.csv.write_text('\n,\n', encoding='utf-8')
        with self.assertRaises(plan.PlanError):
            self.build()


if __name__ == '__main__':
    unittest.main()
