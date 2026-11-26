"""Tests for the post-release asset check."""
import hashlib
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import verify_release


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def release_for(directory, tag='v1.2.3', drop=(), digest=True):
    assets = []
    for name in verify_release.required_assets(tag[1:]):
        if name in drop:
            continue
        path = Path(directory) / name
        assets.append({'name': name,
                       'digest': f'sha256:{sha(path)}' if digest else None})
    return {'tag_name': tag, 'draft': False, 'prerelease': False, 'assets': assets}


class ReleaseCheckTests(unittest.TestCase):
    def build(self, directory):
        for name in verify_release.required_assets('1.2.3'):
            (Path(directory) / name).write_bytes(f'content of {name}'.encode())
        return Path(directory)

    def test_matching_assets_pass(self):
        with tempfile.TemporaryDirectory() as directory:
            root = self.build(directory)
            with mock.patch.object(verify_release, 'fetch_release',
                                   return_value=release_for(root)):
                problems, report = verify_release.verify(root, 'v1.2.3')
            self.assertEqual(problems, [])
            self.assertEqual(report['checked'], report['required'])
            self.assertEqual(report['required'], 11)

    def test_local_file_changed_after_upload_is_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            root = self.build(directory)
            published = release_for(root)
            (root / 'shimo-net_linux_arm64').write_bytes(b'rebuilt differently')
            with mock.patch.object(verify_release, 'fetch_release', return_value=published):
                problems, _ = verify_release.verify(root, 'v1.2.3')
            self.assertTrue(any('shimo-net_linux_arm64' in p and 'local build' in p
                                for p in problems))

    def test_missing_local_asset_is_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            root = self.build(directory)
            published = release_for(root)
            (root / 'shimo-net-cli-1.2.3.tgz').unlink()
            with mock.patch.object(verify_release, 'fetch_release', return_value=published):
                problems, _ = verify_release.verify(root, 'v1.2.3')
            self.assertTrue(any('missing from the build output' in p for p in problems))

    def test_asset_missing_from_release_is_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            root = self.build(directory)
            published = release_for(root, drop={'shimo-net_windows_arm64.exe'})
            with mock.patch.object(verify_release, 'fetch_release', return_value=published):
                problems, _ = verify_release.verify(root, 'v1.2.3')
            self.assertTrue(any('not published' in p for p in problems))

    def test_draft_release_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            root = self.build(directory)
            published = release_for(root)
            published['draft'] = True
            with mock.patch.object(verify_release, 'fetch_release', return_value=published):
                problems, _ = verify_release.verify(root, 'v1.2.3')
            self.assertIn('release v1.2.3 is draft or prerelease', problems)

    def test_asset_without_digest_cannot_be_verified(self):
        with tempfile.TemporaryDirectory() as directory:
            root = self.build(directory)
            published = release_for(root, digest=False)
            with mock.patch.object(verify_release, 'fetch_release', return_value=published):
                problems, _ = verify_release.verify(root, 'v1.2.3')
            self.assertTrue(any('no sha256 digest' in p for p in problems))

    def test_required_assets_follow_the_version(self):
        names = verify_release.required_assets('9.9.9')
        self.assertIn('shimo-net-cli-ent-v9.9.9-workbuddy.zip', names)
        self.assertIn('shimo-net-cli-v9.9.9-skillhub.zip', names)
        self.assertIn('shimo-net-cli-9.9.9.tgz', names)
        self.assertIn('bootstrap.sh', names)
        self.assertEqual(sum(1 for n in names if n.startswith('shimo-net_')), 6)


if __name__ == '__main__':
    unittest.main()
