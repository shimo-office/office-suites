"""Verify the upload archive excludes repository files and matches its release."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
import zipfile
import hashlib

SCRIPT = Path(__file__).with_name('package_skillhub.py')
spec = importlib.util.spec_from_file_location('package_skillhub', SCRIPT)
packager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packager)


class SkillHubPackageTests(unittest.TestCase):
    def fixture(self, directory):
        root = Path(directory)
        source = root / 'docs/agent/SKILL.md'
        source.parent.mkdir(parents=True)
        source.write_text('---\nname: shimo-net-cli\nversion: 1.2.3\n'
                          'display_name: 石墨办公 CLI\ndescription: Read files.\n---\n'
                          '# Instructions\nKeep the actual instructions.\n'
                          '[Install](https://github.com/shimo-office/office-suites/blob/main/docs/install.md)\n', encoding='utf-8')
        (root / 'LICENSE').write_text('All rights reserved.\n', encoding='utf-8')
        (root / 'credentials.json').write_text('must never be packaged', encoding='utf-8')
        for name in ['docs/install.md', 'docs/scenarios.md', 'docs/cli-reference.md',
                     'docs/phase3-acceptance.md', 'SECURITY.md', 'docs/agent/runtime.md']:
            p = root / name
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text('# Reference\n[Commands](cli-reference.md#commands)\n', encoding='utf-8')
        return root, source

    def digests(self):
        return {f'shimo-net_{system}_{arch}' + ('.exe' if system == 'windows' else ''):
                hashlib.sha256(f'{system}/{arch}'.encode()).hexdigest()
                for system in ['darwin', 'linux', 'windows'] for arch in ['amd64', 'arm64']}

    def test_both_packages_are_self_contained_small_and_reproducible(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source = self.fixture(directory)
            for target in ['skillhub', 'workbuddy']:
                archive = packager.build(root, root / 'dist', 'v1.2.3', target, self.digests())
                first = archive.read_bytes()
                self.assertLess(len(first), 3_000_000)
                with zipfile.ZipFile(archive) as z:
                    expected = {'SKILL.md', 'LICENSE.md', 'scripts/shimo-net.sh',
                                'scripts/shimo-net.ps1', 'references/install.md',
                                'references/scenarios.md', 'references/cli-reference.md',
                                'references/phase3-acceptance.md', 'references/SECURITY.md',
                                'references/runtime.md'}
                    self.assertEqual(set(z.namelist()), expected)
                    self.assertIsNone(z.testzip())
                    skill = z.read('SKILL.md').decode('utf-8')
                    self.assertIn('Keep the actual instructions.', skill)
                    self.assertIn('(references/install.md)', skill)
                    self.assertIn('(references/runtime.md)', skill)
                    if target == 'skillhub':
                        self.assertIn('slug: "shimo-net-cli"\n', skill)
                    else:
                        self.assertIn('display_name: 石墨办公 CLI\n', skill)
                    self.assertEqual(z.read('LICENSE.md'), (root / 'LICENSE').read_bytes())
                    self.assertIn('(cli-reference.md#commands)', z.read('references/install.md').decode())
                    self.assertTrue(all(Path(name).suffix in ['.md', '.sh', '.ps1'] for name in z.namelist()))
                self.assertEqual(packager.build(root, root / 'dist', 'v1.2.3', target, self.digests()).read_bytes(), first)

    def test_missing_asset_hashes_and_oversized_package_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source = self.fixture(directory)
            with self.assertRaises(ValueError):
                packager.build(root, root / 'dist', 'v1.2.3', 'workbuddy', {})
            (root / 'docs/scenarios.md').write_text('x' * 3_000_001, encoding='utf-8')
            with self.assertRaises(ValueError):
                packager.build(root, root / 'dist', 'v1.2.3', 'workbuddy', self.digests())
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
