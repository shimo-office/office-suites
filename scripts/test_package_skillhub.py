"""Verify the upload archive excludes repository files and matches its release."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
import zipfile

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
                          '# Instructions\nKeep the actual instructions.\n', encoding='utf-8')
        (root / 'LICENSE').write_text('All rights reserved.\n', encoding='utf-8')
        (root / 'credentials.json').write_text('must never be packaged', encoding='utf-8')
        return root, source

    def test_minimal_reproducible_zip_preserves_body_and_license(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source = self.fixture(directory)
            archive = packager.build(root, root / 'dist', 'v1.2.3')
            first = archive.read_bytes()
            with zipfile.ZipFile(archive) as z:
                self.assertEqual(z.namelist(), ['SKILL.md', 'LICENSE.md'])
                self.assertTrue(all(Path(name).suffix == '.md' for name in z.namelist()))
                self.assertIsNone(z.testzip())
                skill = z.read('SKILL.md').decode('utf-8')
                self.assertEqual(skill.split('\n---\n', 1)[1],
                                 source.read_text(encoding='utf-8').split('\n---\n', 1)[1])
                self.assertIn('slug: "shimo-net-cli"\n', skill)
                self.assertIn('displayName: "石墨办公 CLI"\n', skill)
                self.assertEqual(z.read('LICENSE.md'), (root / 'LICENSE').read_bytes())
            self.assertEqual(packager.build(root, root / 'dist', 'v1.2.3').read_bytes(), first)

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
