#!/usr/bin/env python3
"""Check that a release carries exactly the files this build produced.

Packaging cannot detect an upload that went wrong: the ZIP, the npm tarball and
the six binaries are all built from whatever was on disk at the time. This
verifier closes that gap after upload. It reads the SHA-256 GitHub reports for
each asset and compares it with the local file of the same name, and it fails
when a required asset is missing from either side.

Usage: verify_release.py dist --tag v0.9.22
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import sys
import urllib.error
import urllib.request

DEFAULT_REPO = 'shimo-office/office-suites'
BINARIES = tuple(f'shimo-net_{system}_{arch}' + ('.exe' if system == 'windows' else '')
                 for system in ('darwin', 'linux', 'windows')
                 for arch in ('amd64', 'arm64'))


def required_assets(version):
    """Names this release must ship, derived from the version alone."""
    return BINARIES + (
        'bootstrap.sh',
        'SKILL.md',
        f'shimo-net-cli-{version}.tgz',
        f'shimo-net-cli-v{version}-skillhub.zip',
        f'shimo-net-cli-v{version}-workbuddy.zip',
    )


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
    """GitHub reports sha256 per asset; a missing digest cannot be verified."""
    digests, missing = {}, []
    for asset in release.get('assets', []):
        digest = asset.get('digest') or ''
        if digest.startswith('sha256:'):
            digests[asset['name']] = digest.split(':', 1)[1]
        else:
            missing.append(asset['name'])
    return digests, missing


def verify(directory, tag, repo=DEFAULT_REPO, token=None):
    """Return (problems, report). An empty problem list means the release is sound."""
    version = tag[1:] if tag.startswith('v') else tag
    release = fetch_release(repo, tag, token)
    problems = []
    if release.get('draft') or release.get('prerelease'):
        problems.append(f'release {tag} is draft or prerelease')
    digests, undigested = published_digests(release)
    for name in undigested:
        problems.append(f'{name}: release asset has no sha256 digest to compare')

    checked = 0
    for name in required_assets(version):
        local = Path(directory) / name
        if not local.is_file():
            problems.append(f'{name}: missing from the build output')
            continue
        published = digests.get(name)
        if published is None:
            problems.append(f'{name}: not published in release {tag}')
            continue
        local_digest = hashlib.sha256(local.read_bytes()).hexdigest()
        if local_digest != published:
            problems.append(f'{name}: local build {local_digest[:16]}…, '
                            f'release has {published[:16]}…')
            continue
        checked += 1

    report = {'repo': repo, 'tag': tag, 'version': version,
              'required': len(required_assets(version)), 'checked': checked,
              'releaseAssets': len(digests), 'problems': problems}
    return problems, report


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('directory', type=Path,
                        help='Build output holding the files to be released')
    parser.add_argument('--tag', required=True, help='Release tag, e.g. v0.9.22')
    parser.add_argument('--repo', default=DEFAULT_REPO)
    parser.add_argument('--token', default=os.environ.get('GITHUB_TOKEN'),
                        help='Optional token; public releases need none')
    parser.add_argument('--json', action='store_true')
    args = parser.parse_args()
    try:
        problems, report = verify(args.directory, args.tag, args.repo, args.token)
    except (ValueError, OSError) as error:
        parser.exit(1, f'{error}\n')
    if args.json:
        print(json.dumps(report, ensure_ascii=False, indent=2))
    elif problems:
        print(f'{report["tag"]}: {len(problems)} problem(s)')
        for problem in problems:
            print(f'  - {problem}')
    else:
        print(f'{report["tag"]}: all {report["checked"]} required assets match '
              f'the published release')
    parser.exit(1 if problems else 0)


if __name__ == '__main__':
    main()
