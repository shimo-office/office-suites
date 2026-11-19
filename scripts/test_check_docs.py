import tempfile
import subprocess
import shutil
import os
import sys
import unittest
from pathlib import Path

from check_docs import link_errors, tracking_errors, ROOT


class LinkChecks(unittest.TestCase):
    def test_docs_check_under_non_utf8_process_locale(self):
        env = {**os.environ, 'LC_ALL': 'C', 'PYTHONUTF8': '0', 'PYTHONCOERCECLOCALE': '0'}
        result = subprocess.run([sys.executable, str(ROOT / 'scripts/check_docs.py')],
                                cwd=ROOT, env=env, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr.decode('utf-8', errors='replace'))

    def test_git_visibility_distinguishes_private_root_from_docs(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            subprocess.run(['git', 'init', '-q', str(root)], check=True)
            (root / '.gitignore').write_text('internal/\n')
            paths = ['docs/internal/capabilities.json']
            self.assertEqual(len(tracking_errors(root, paths)), 1)
            (root / '.gitignore').write_text('/internal/\n')
            self.assertEqual(tracking_errors(root, paths), [])
            self.assertEqual(len(tracking_errors(root, ['internal/private.json'])), 1)

    @unittest.skipUnless(os.name == 'posix' and shutil.which('bash'), 'POSIX installer block')
    def test_dsh_install_copies_canonical_skill_to_discovery_path(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / '.dsh').mkdir()
            script = (ROOT / 'install.command').read_text(encoding='utf-8')
            block = script.split('if [ -d "$HOME/.dsh" ]; then', 1)[1].split('\nfi', 1)[0]
            block = block.replace('$HOME', directory).replace('$DIR', str(ROOT))
            result = subprocess.run(['bash', '-ec', block], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((root / '.dsh/skills/shimo-net-cli/SKILL.md').read_bytes(),
                             (ROOT / 'docs/agent/SKILL.md').read_bytes())

    def test_reports_missing_relative_target(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            page = root / 'README.md'
            page.write_text('[Guide](docs/missing.md)')
            self.assertEqual(len(link_errors(page)), 1)

    def test_encoded_filename_and_anchor(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'My Guide.md').write_text('# Intro')
            page = root / 'README.md'
            page.write_text('[Guide](My%20Guide.md#intro)')
            self.assertEqual(link_errors(page), [])

    def test_ignores_external_links_and_code_examples(self):
        with tempfile.TemporaryDirectory() as directory:
            page = Path(directory) / 'README.md'
            page.write_text('[Web](https://example.org/a) [Mail](mailto:a@example.org)\n```md\n[Example](not-real.md)\n```\n')
            self.assertEqual(link_errors(page), [])

    def test_checks_reference_links_and_images(self):
        with tempfile.TemporaryDirectory() as directory:
            page = Path(directory) / 'README.md'
            page.write_text('![proof](missing.png)\n[guide][ref]\n[ref]: missing.md\n')
            self.assertEqual(len(link_errors(page)), 2)


if __name__ == '__main__':
    unittest.main()
