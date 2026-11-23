#!/usr/bin/env python3
"""Point the READMEs at the Gitee mirror.

The repository is mirrored verbatim, so a README that hard-codes github.com
sends mainland readers over a link that is often slow or unreachable. The
rewrite runs on the mirror side only: the files committed here keep the GitHub
addresses, and only the mirror commit carries the Gitee ones.

Links between repository files are already relative and resolve per platform,
so only the absolute addresses below need help:

* the release and CI badges lose their outbound link but keep the image, since
  the version number and CI status are still true and still worth showing;
* everything else (blob, issues) moves to the Gitee host.

Release assets and Actions only exist on GitHub. Those stay untouched here:
docs/install.md keeps pointing at GitHub Releases, because that is where the
binaries live.
"""
import argparse
from pathlib import Path
import re
import sys

GITHUB = 'https://github.com/shimo-office/office-suites'
GITEE = 'https://gitee.com/graphite-office/office-suites'

# Paths that exist only on GitHub: linking to them from the mirror would land
# the reader on a 404 or an empty page.
MIRROR_MISSING = r'(?:releases|actions/[^"]*)'

BADGE = re.compile(r'<a href="' + re.escape(GITHUB) + '/' + MIRROR_MISSING
                   + r'">(<img [^>]*/>)</a>')

# Release binaries live on GitHub only, so a download address must survive the
# rewrite even if a README ever starts quoting one.
LINK = re.compile(re.escape(GITHUB) + r'(?!/releases/download)')

FILES = ('README.md', 'README.en.md')


def render(text):
    """Return the mirror variant of a README, leaving relative links alone."""
    text = BADGE.sub(r'\1', text)
    return LINK.sub(GITEE, text)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('root', nargs='?', default='.', help='repository root')
    args = parser.parse_args(argv)

    root = Path(args.root)
    changed = 0
    for name in FILES:
        path = root / name
        if not path.exists():
            print(f'{name}: missing, skipped', file=sys.stderr)
            continue
        original = path.read_text(encoding='utf-8')
        updated = render(original)
        if updated == original:
            print(f'{name}: already points at the mirror')
            continue
        path.write_text(updated, encoding='utf-8')
        moved = updated.count(GITEE) - original.count(GITEE)
        unlinked = len(BADGE.findall(original))
        print(f'{name}: {moved} link(s) moved to the mirror, '
              f'{unlinked} badge link(s) dropped')
        changed += 1

    if not changed:
        print('nothing to rewrite')
    return 0


if __name__ == '__main__':
    sys.exit(main())
