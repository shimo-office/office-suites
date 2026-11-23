#!/usr/bin/env python3
"""Check a packaged skill ZIP against the binaries actually published in its release.

The launchers shipped inside a skill package pin a SHA-256 per platform, so a
package is only usable while those hashes match the release assets. Nothing in
the packaging path can detect a mismatch after upload, because the hashes are
computed from whatever binaries were on disk at build time. This verifier closes
that gap: it reads the pinned hashes out of the ZIP and compares them with the
digests GitHub reports for the release assets.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import urllib.error
import urllib.request
import zipfile

DEFAULT_REPO = 'shimo-office/office-suites'
POSIX_PIN = re.compile(r"^\s*(shimo-net_[a-z0-9_]+?)\) expected='([0-9a-f]{64})'", re.M)
POWERSHELL_PIN = re.compile(r"'(shimo-net_windows_[a-z0-9_]+?\.exe)'\s*=\s*'([0-9a-f]{64})'", re.M)
RELEASE_URL = re.compile(r'releases/download/(v[0-9][0-9A-Za-z.\-]*)/')


def pinned_hashes(archive):
    """Return the platform digests a packaged skill will accept, plus its tag."""
    with zipfile.ZipFile(archive) as z:
        names = z.namelist()
        scripts = [n for n in names if n.endswith('/scripts/shimo-net.sh')]
        if len(scripts) != 1:
            raise ValueError('archive must contain exactly one scripts/shimo-net.sh')
        launcher = z.read(scripts[0]).decode('utf-8')
        powershell = [n for n in names if n.endswith('/scripts/shimo-net.ps1')]
        if len(powershell) != 1:
            raise ValueError('archive must contain exactly one scripts/shimo-net.ps1')
        launcher += z.read(powershell[0]).decode('utf-8')
    digests = dict(POSIX_PIN.findall(launcher))
    digests.update(dict(POWERSHELL_PIN.findall(launcher)))
    tags = set(RELEASE_URL.findall(launcher))
    if not digests:
        raise ValueError('archive pins no platform digests')
    if len(tags) != 1:
        raise ValueError(f'launchers must pin exactly one release tag, found {sorted(tags)}')
    return digests, tags.pop()


def fetch_release(repo, tag, token=None):
    request = urllib.request.Request(
        f'https://api.github.com/repos/{repo}/releases/tags/{tag}',
        headers={'Accept': 'application/vnd.github+json',
                 'User-Agent': 'shimo-net-verify-release'})
    if token:
        request.add_header('Authorization', f'Bearer {token}')
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        detail = error.read().decode('utf-8', 'replace')[:200]
        raise ValueError(f'cannot read release {tag}: HTTP {error.code} {detail}') from error


def published_digests(release):
    """GitHub reports sha256 for every uploaded asset; none means no digest yet."""
    digests, missing = {}, []
    for asset in release.get('assets', []):
        digest = asset.get('digest') or ''
        if digest.startswith('sha256:'):
            digests[asset['name']] = digest.split(':', 1)[1]
        elif asset['name'].startswith('shimo-net_'):
            missing.append(asset['name'])
    return digests, missing


def verify(archive, repo=DEFAULT_REPO, tag=None, token=None, expected_zip_digest=True):
    """Return (problems, report). An empty problem list means the package is usable."""
    archive = Path(archive)
    pinned, launcher_tag = pinned_hashes(archive)
    tag = tag or launcher_tag
    problems = []
    if tag != launcher_tag:
        problems.append(f'launchers pin {launcher_tag}, verifying against {tag}')
    release = fetch_release(repo, tag, token)
    if release.get('draft') or release.get('prerelease'):
        problems.append(f'release {tag} is draft or prerelease')
    digests, missing = published_digests(release)
    for name, want in sorted(pinned.items()):
        got = digests.get(name)
        if got is None:
            problems.append(f'{name}: not published in release {tag}')
        elif got != want:
            problems.append(f'{name}: package pins {want[:16]}…, release has {got[:16]}…')
    for name in missing:
        problems.append(f'{name}: release asset has no sha256 digest to compare')
    if expected_zip_digest:
        local = hashlib.sha256(archive.read_bytes()).hexdigest()
        published = digests.get(archive.name)
        if published is None:
            problems.append(f'{archive.name}: not published in release {tag}')
        elif published != local:
            problems.append(f'{archive.name}: local file differs from the published asset')
    report = {'repo': repo, 'tag': tag, 'archive': str(archive),
              'platforms': len(pinned), 'releaseAssets': len(digests),
              'problems': problems}
    return problems, report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('archive', type=Path, help='Packaged skill ZIP, e.g. dist/…-workbuddy.zip')
    parser.add_argument('--repo', default=DEFAULT_REPO)
    parser.add_argument('--tag', help='Release tag to verify; defaults to the tag the launchers pin')
    parser.add_argument('--token', default=os.environ.get('GITHUB_TOKEN'),
                        help='Optional token; public releases need none')
    parser.add_argument('--skip-zip-digest', action='store_true',
                        help='Only compare platform binaries, not this ZIP asset')
    parser.add_argument('--json', action='store_true')
    args = parser.parse_args()
    try:
        problems, report = verify(args.archive, args.repo, args.tag, args.token,
                                  expected_zip_digest=not args.skip_zip_digest)
    except (ValueError, OSError, zipfile.BadZipFile) as error:
        parser.exit(1, f'{error}\n')
    if args.json:
        print(json.dumps(report, ensure_ascii=False, indent=2))
    elif problems:
        print(f'{report["tag"]}: {len(problems)} problem(s)')
        for problem in problems:
            print(f'  - {problem}')
    else:
        print(f'{report["tag"]}: {report["platforms"]} platform digests and '
              f'{args.archive.name} match the published release')
    parser.exit(1 if problems else 0)


if __name__ == '__main__':
    main()
