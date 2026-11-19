"""Regression tests for source validation and non-mutating drift checks."""
import contextlib
import copy
import importlib.util
import io
import json
import os
import subprocess
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).with_name("generate_capabilities.py")
spec = importlib.util.spec_from_file_location("generate_capabilities", SCRIPT)
generator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(generator)


class CapabilityGeneratorTests(unittest.TestCase):
    def setUp(self):
        self.raw = generator.SOURCE.read_bytes()
        self.source = json.loads(self.raw)

    def test_catalog_checkout_keeps_digest_with_autocrlf_enabled(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'docs/internal/capabilities.json'
            source.parent.mkdir(parents=True)
            source.write_bytes(self.raw)
            (root / '.gitattributes').write_bytes((generator.ROOT / '.gitattributes').read_bytes())
            def git(*args):
                subprocess.run(['git', *args], cwd=root, check=True, capture_output=True)
            git('init', '-q')
            git('config', 'core.autocrlf', 'true')
            git('add', '.gitattributes', 'docs/internal/capabilities.json')
            source.unlink()
            git('checkout', '--', 'docs/internal/capabilities.json')
            self.assertEqual(source.read_bytes(), self.raw)

    def test_generation_under_non_utf8_process_locale(self):
        env = {**os.environ, 'LC_ALL': 'C', 'PYTHONUTF8': '0', 'PYTHONCOERCECLOCALE': '0'}
        result = subprocess.run([sys.executable, str(SCRIPT), '--check'], env=env,
                                capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr.decode('utf-8', errors='replace'))

    def test_reject_duplicate_capability(self):
        first = self.source["scenarios"][0]["capabilities"]
        first.append(copy.deepcopy(first[0]))
        with self.assertRaisesRegex(ValueError, "duplicate capability"):
            generator.validate(self.source)

    def test_reject_missing_original_scene(self):
        self.source["scenarios"].pop()
        with self.assertRaisesRegex(ValueError, "original IDs"):
            generator.validate(self.source)

    def test_reject_cross_scene_id(self):
        self.source["scenarios"][1]["capabilities"][0]["id"] = "01.9"
        with self.assertRaisesRegex(ValueError, "capability ID"):
            generator.validate(self.source)

    def test_reject_unclassified_execution(self):
        self.source["scenarios"][0]["capabilities"][0]["mode"] = "maybe works"
        with self.assertRaisesRegex(ValueError, "execution mode"):
            generator.validate(self.source)

    def test_reject_missing_limits(self):
        self.source["scenarios"][0]["capabilities"][0]["limits"] = ""
        with self.assertRaisesRegex(ValueError, "limits"):
            generator.validate(self.source)

    def test_check_detects_each_stale_output_without_repairing_it(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "docs/internal/capabilities.json"
            source.parent.mkdir(parents=True)
            source.write_bytes(self.raw)
            with patch.object(generator, "ROOT", root), patch.object(generator, "SOURCE", source):
                with patch.object(sys, "argv", [str(SCRIPT)]), contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(generator.main(), 0)
                outputs = generator.render(self.raw)
                for path, expected in outputs.items():
                    with self.subTest(path=path.name):
                        path.write_text("stale content\n", encoding="utf-8")
                        with patch.object(sys, "argv", [str(SCRIPT), "--check"]), contextlib.redirect_stderr(io.StringIO()):
                            self.assertEqual(generator.main(), 1)
                        self.assertEqual(path.read_text(), "stale content\n")
                        path.write_text(expected, encoding="utf-8")
                with patch.object(sys, "argv", [str(SCRIPT), "--check"]), contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(generator.main(), 0)


if __name__ == "__main__":
    unittest.main()
