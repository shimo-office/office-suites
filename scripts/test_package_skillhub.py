"""Verify the upload archive excludes repository files and is safe to publish."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
import zipfile

SCRIPT = Path(__file__).with_name('package_skillhub.py')
spec = importlib.util.spec_from_file_location('package_skillhub', SCRIPT)
packager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packager)

# A packaged skill must never ship code that downloads and runs a remote binary:
# it trips the marketplace scan and cannot work in an offline sandbox.
FORBIDDEN_SUFFIXES = {'.sh', '.ps1', '.bat', '.cmd', '.exe', '.dll', '.so', '.dylib'}


class SkillHubPackageTests(unittest.TestCase):
    def fixture(self, directory):
        root = Path(directory)
        source = root / 'docs/agent/SKILL.md'
        source.parent.mkdir(parents=True)
        source.write_text('---\nname: shimo-net-cli\nversion: 1.2.3\n'
                          'display_name: 石墨办公 CLI\ndescription: Read files.\n---\n'
                          '# Instructions\nKeep the actual instructions.\n'
                          '[Install](https://github.com/shimo-office/office-suites/blob/main/docs/install.md)\n',
                          encoding='utf-8')
        (root / 'docs/agent/selfcheck.py').write_text(
            'import json\nprint(json.dumps({"ready": False}))\n', encoding='utf-8')
        (root / 'docs/agent/plan.py').write_text(
            'import json\nprint(json.dumps({"ok": True, "written": False}))\n',
            encoding='utf-8')
        (root / 'LICENSE').write_text('All rights reserved.\n', encoding='utf-8')
        (root / 'credentials.json').write_text('must never be packaged', encoding='utf-8')
        for name in ['docs/install.md', 'docs/scenarios.md', 'docs/cli-reference.md',
                     'SECURITY.md']:
            p = root / name
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text('# Reference\n[Commands](cli-reference.md#commands)\n', encoding='utf-8')
        return root, source

    def test_both_packages_are_small_reproducible_and_script_free(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source = self.fixture(directory)
            for target in ['skillhub', 'workbuddy']:
                archive = packager.build(root, root / 'dist', 'v1.2.3', target)
                first = archive.read_bytes()
                self.assertLess(len(first), 3_000_000)
                with zipfile.ZipFile(archive) as z:
                    expected = {'shimo-net-cli/SKILL.md', 'shimo-net-cli/LICENSE.md',
                                'shimo-net-cli/scripts/selfcheck.py',
                                'shimo-net-cli/scripts/plan.py',
                                'shimo-net-cli/references/install.md',
                                'shimo-net-cli/references/scenarios.md',
                                'shimo-net-cli/references/cli-reference.md',
                                'shimo-net-cli/references/SECURITY.md'}
                    self.assertEqual(set(z.namelist()), expected)
                    self.assertIsNone(z.testzip())
                    skill = z.read('shimo-net-cli/SKILL.md').decode('utf-8')
                    self.assertIn('Keep the actual instructions.', skill)
                    self.assertIn('(references/install.md)', skill)
                    self.assertNotIn('此上传包', skill)
                    if target == 'skillhub':
                        self.assertIn('slug: "shimo-net-cli"\n', skill)
                    else:
                        self.assertIn('display_name: 石墨办公 CLI\n', skill)
                    self.assertEqual(z.read('shimo-net-cli/LICENSE.md'),
                                     (root / 'LICENSE').read_bytes())
                    self.assertIn('(cli-reference.md#commands)',
                                  z.read('shimo-net-cli/references/install.md').decode())
                    check = z.read('shimo-net-cli/scripts/selfcheck.py').decode('utf-8')
                    compile(check, 'selfcheck.py', 'exec')
                    self.assertNotIn('http', check)
                self.assertEqual(packager.build(root, root / 'dist', 'v1.2.3', target).read_bytes(),
                                 first)

    def test_no_packaged_file_is_run_by_the_system(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source = self.fixture(directory)
            archive = packager.build(root, root / 'dist', 'v1.2.3', 'workbuddy')
            with zipfile.ZipFile(archive) as z:
                for name in z.namelist():
                    self.assertNotIn(Path(name).suffix, FORBIDDEN_SUFFIXES, name)
                body = z.read('shimo-net-cli/SKILL.md').decode('utf-8')
            for pattern in ('curl |', 'curl -fsSL', 'Invoke-WebRequest', 'chmod +x'):
                self.assertNotIn(pattern, body)

    def test_oversized_package_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source = self.fixture(directory)
            (root / 'docs/scenarios.md').write_text('x' * 3_000_001, encoding='utf-8')
            with self.assertRaises(ValueError):
                packager.build(root, root / 'dist', 'v1.2.3', 'workbuddy')
            self.assertFalse((root / 'dist').exists())

    def test_unquoted_colon_in_metadata_is_rejected(self):
        for bad in ['Read files: fast.', 'Read files:']:
            with self.subTest(description=bad), tempfile.TemporaryDirectory() as directory:
                root, source = self.fixture(directory)
                source.write_text(source.read_text(encoding='utf-8').replace(
                    'description: Read files.', f'description: {bad}'), encoding='utf-8')
                with self.assertRaises(ValueError) as raised:
                    packager.build(root, root / 'dist', 'v1.2.3', 'workbuddy')
                self.assertIn('needs quotes', str(raised.exception))
                self.assertFalse((root / 'dist').exists())

    def test_wrong_tag_and_invalid_metadata_fail_before_output(self):
        for replacement, tag in [('version: 1.2.3', 'v1.2.4'),
                                 ('version: ../bad', None),
                                 ('version: 01.2.3', None),
                                 ('version: 1.2.3\nversion: 1.2.4', None)]:
            with self.subTest(replacement=replacement, tag=tag), tempfile.TemporaryDirectory() as directory:
                root, source = self.fixture(directory)
                source.write_text(source.read_text(encoding='utf-8').replace('version: 1.2.3', replacement), encoding='utf-8')
                with self.assertRaises(ValueError):
                    packager.build(root, root / 'dist', tag)
                self.assertFalse((root / 'dist').exists())


if __name__ == '__main__':
    unittest.main()
