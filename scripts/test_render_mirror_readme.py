#!/usr/bin/env python3
"""Tests for the mirror-side README rewrite."""
import tempfile
from pathlib import Path
import unittest

import render_mirror_readme as renderer

GITHUB = 'https://github.com/shimo-office/office-suites'
GITEE = 'https://gitee.com/graphite-office/office-suites'


class RenderTest(unittest.TestCase):
    def test_release_badge_keeps_the_image_but_drops_the_link(self):
        text = ('<a href="%s/releases"><img src="https://img.shields.io/github/v/release/'
                'shimo-office/office-suites?color=0d9488" alt="Release" /></a>\n' % GITHUB)
        out = renderer.render(text)
        self.assertIn('<img src="https://img.shields.io', out)
        self.assertNotIn('<a href=', out)
        self.assertNotIn(GITHUB, out)

    def test_ci_badge_drops_the_link_too(self):
        text = ('<a href="%s/actions/workflows/ci.yml"><img src="https://img.shields.io/'
                'github/actions/workflow/status/x?label=CI" alt="CI" /></a>\n' % GITHUB)
        out = renderer.render(text)
        self.assertIn('<img src="https://img.shields.io', out)
        self.assertNotIn('<a href=', out)

    def test_blob_and_issue_links_move_to_the_mirror(self):
        text = ('%s/blob/main/docs/agent/setup.md\n%s/issues\n' % (GITHUB, GITHUB))
        out = renderer.render(text)
        self.assertEqual(out.count(GITEE), 2)
        self.assertNotIn(GITHUB, out)

    def test_relative_links_and_other_hosts_are_untouched(self):
        text = ('[安装](docs/install.md) ![图](docs/images/a.svg) '
                '[npm](https://www.npmjs.com/package/shimo-net-cli) '
                '[产品](https://shimo.net/products/office-suites)\n')
        self.assertEqual(renderer.render(text), text)

    def test_release_downloads_keep_pointing_at_github(self):
        text = 'curl -fsSL %s/releases/download/v0.9.21/bootstrap.sh\n' % GITHUB
        self.assertEqual(renderer.render(text), text)

    def test_rewrite_is_idempotent(self):
        once = renderer.render('%s/issues\n' % GITHUB)
        self.assertEqual(renderer.render(once), once)

    def test_cli_rewrites_both_readmes_only(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'README.md').write_text('%s/issues\n' % GITHUB, encoding='utf-8')
            (root / 'README.en.md').write_text('%s/blob/main/x.md\n' % GITHUB, encoding='utf-8')
            (root / 'docs.md').write_text('%s/issues\n' % GITHUB, encoding='utf-8')
            self.assertEqual(renderer.main([directory]), 0)
            self.assertIn(GITEE, (root / 'README.md').read_text(encoding='utf-8'))
            self.assertIn(GITEE, (root / 'README.en.md').read_text(encoding='utf-8'))
            self.assertIn(GITHUB, (root / 'docs.md').read_text(encoding='utf-8'))

    def test_second_run_reports_nothing_to_rewrite(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'README.md').write_text('%s/issues\n' % GITHUB, encoding='utf-8')
            renderer.main([directory])
            before = (root / 'README.md').read_text(encoding='utf-8')
            self.assertEqual(renderer.main([directory]), 0)
            self.assertEqual((root / 'README.md').read_text(encoding='utf-8'), before)


class RealReadmeTest(unittest.TestCase):
    def test_repository_readmes_leave_no_github_repository_link(self):
        root = Path(__file__).resolve().parent.parent
        for name in renderer.FILES:
            text = (root / name).read_text(encoding='utf-8')
            mirrored = renderer.render(text)
            self.assertNotIn(GITHUB, mirrored, name)
            self.assertIn(GITEE, mirrored, name)

    def test_github_readmes_are_not_rewritten_in_place(self):
        root = Path(__file__).resolve().parent.parent
        for name in renderer.FILES:
            self.assertIn(GITHUB, (root / name).read_text(encoding='utf-8'), name)


if __name__ == '__main__':
    unittest.main()
