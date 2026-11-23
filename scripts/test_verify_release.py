#!/usr/bin/env python3
"""Tests for the post-release consistency check."""
import hashlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest import mock
import zipfile

import verify_release

PLATFORMS = {'shimo-net_darwin_amd64': 'a', 'shimo-net_darwin_arm64': 'b',
             'shimo-net_linux_amd64': 'c', 'shimo-net_linux_arm64': 'd',
             'shimo-net_windows_amd64.exe': 'e', 'shimo-net_windows_arm64.exe': 'f'}


def digest(seed):
    return hashlib.sha256(seed.encode()).hexdigest()


def launcher(digests, tag='v0.9.21'):
    posix = ['URL="https://github.com/shimo-office/office-suites/releases/download/%s/$ASSET"' % tag,
             'case "$ASSET" in']
    powershell = ['$Expected = @{']
    for name, value in digests.items():
        if name.endswith('.exe'):
            powershell.append("  '%s' = '%s'" % (name, value))
        else:
            posix.append("  %s) expected='%s' ;;" % (name, value))
    posix.append('esac')
    powershell.append('}')
    return '\n'.join(posix) + '\n', '\n'.join(powershell) + '\n'


def write_archive(path, tag='v0.9.21'):
    digests = {name: digest(seed) for name, seed in PLATFORMS.items()}
    posix, powershell = launcher(digests, tag)
    with zipfile.ZipFile(path, 'w') as z:
        z.writestr('shimo-net-cli/scripts/shimo-net.sh', posix)
        z.writestr('shimo-net-cli/scripts/shimo-net.ps1', powershell)
        z.writestr('shimo-net-cli/SKILL.md', 'version: 0.9.21\n')
    return path


def release(archive, digests, tag='v0.9.21', extra_assets=()):
    assets = [{'name': name, 'digest': 'sha256:' + value} for name, value in digests.items()]
    assets.append({'name': Path(archive).name,
                   'digest': 'sha256:' + hashlib.sha256(Path(archive).read_bytes()).hexdigest()})
    assets.extend(extra_assets)
    return {'tag_name': tag, 'draft': False, 'prerelease': False, 'assets': assets}


class VerifyReleaseTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.root = Path(self.directory.name)
        self.archive = write_archive(self.root / 'shimo-net-cli-v0.9.21-workbuddy.zip')
        self.digests = {name: digest(seed) for name, seed in PLATFORMS.items()}

    def tearDown(self):
        self.directory.cleanup()

    def run_verify(self, published, **kwargs):
        with mock.patch.object(verify_release, 'fetch_release', return_value=published):
            return verify_release.verify(self.archive, **kwargs)

    def test_pinned_hashes_cover_every_platform(self):
        pinned, tag = verify_release.pinned_hashes(self.archive)
        self.assertEqual(pinned, self.digests)
        self.assertEqual(tag, 'v0.9.21')

    def test_matching_release_passes(self):
        problems, report = self.run_verify(release(self.archive, self.digests))
        self.assertEqual(problems, [])
        self.assertEqual(report['platforms'], 6)

    def test_binary_digest_drift_is_reported(self):
        published = release(self.archive, self.digests)
        published['assets'][0]['digest'] = 'sha256:' + digest('rebuilt-binary')
        problems, _ = self.run_verify(published)
        self.assertTrue(any('package pins' in p for p in problems), problems)

    def test_missing_platform_asset_is_reported(self):
        published = release(self.archive, self.digests)
        published['assets'] = published['assets'][1:]
        problems, _ = self.run_verify(published)
        self.assertTrue(any('not published in release' in p for p in problems), problems)

    def test_local_zip_must_match_published_asset(self):
        published = release(self.archive, self.digests)
        published['assets'][-1]['digest'] = 'sha256:' + digest('other-zip')
        problems, _ = self.run_verify(published)
        self.assertTrue(any('local file differs' in p for p in problems), problems)

    def test_tag_mismatch_is_reported(self):
        published = release(self.archive, self.digests, tag='v0.9.22')
        problems, _ = self.run_verify(published, tag='v0.9.22')
        self.assertTrue(any('launchers pin v0.9.21' in p for p in problems), problems)

    def test_asset_without_digest_is_not_silently_accepted(self):
        published = release(self.archive, self.digests)
        published['assets'][0].pop('digest')
        problems, _ = self.run_verify(published)
        self.assertTrue(problems)


if __name__ == '__main__':
    unittest.main()
