#!/usr/bin/env python3
"""Report whether this machine can run shimo-net yet.

Pure standard library, no network, no credentials read: it only inspects the
environment and prints one JSON object. That makes it safe to run in any
sandbox, and it gives the caller a definite answer instead of a stack trace
when the CLI is missing.

Exit status is 0 whenever the check itself ran. ``ready`` in the output says
whether commands can be executed, and ``next`` names the one thing to do next.
"""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

PROFILE_DIRS = {
    'darwin': ('~/.shimo-net-cli',),
    'linux': ('~/.shimo-net-cli', '~/.config/shimo-net-cli'),
    'win32': ('%APPDATA%/shimo-net-cli',),
}
INSTALL = 'npm install -g shimo-net-cli'
# Commands in this skill assume the options that shipped with this release.
MINIMUM_CLI = '0.9.20'


def parse_version(value):
    parts = [int(part) for part in re.findall(r'\d+', value or '')[:3]]
    return tuple(parts + [0] * (3 - len(parts)))


def skill_version():
    """Read this package's own version so the report can flag a mismatch."""
    base = Path(__file__).resolve().parent
    # Packaged layout keeps SKILL.md one level up from scripts/; the source tree
    # keeps it next to this file.
    for candidate in (base.parent / 'SKILL.md', base / 'SKILL.md'):
        try:
            match = re.search(r'(?m)^version:\s*(\S+)\s*$', candidate.read_text(encoding='utf-8'))
        except OSError:
            continue
        if match:
            return match.group(1)
    return None


def cli_version(path):
    try:
        out = subprocess.run([path, 'version', '--json'], capture_output=True,
                             text=True, timeout=15).stdout
        return json.loads(out).get('version')
    except (OSError, ValueError, subprocess.SubprocessError):
        return None


def profile_state():
    candidates = PROFILE_DIRS.get(sys.platform, ('~/.shimo-net-cli',))
    for template in candidates:
        directory = Path(os.path.expandvars(template)).expanduser()
        if not directory.is_dir():
            continue
        origin = None
        for name in ('config.json', 'settings.json'):
            file = directory / name
            if file.is_file():
                try:
                    origin = json.loads(file.read_text(encoding='utf-8')).get('origin')
                except (OSError, ValueError):
                    origin = None
                if origin:
                    break
        return {'dir': str(directory),
                'initialized': origin is not None,
                'origin': origin,
                'credentials': (directory / 'credentials.json').is_file()}
    return {'dir': str(Path(os.path.expandvars(candidates[0])).expanduser()),
            'initialized': False, 'origin': None, 'credentials': False}


def next_step(cli, profile):
    if not cli['found']:
        return {'action': 'install', 'command': INSTALL,
                'reason': 'shimo-net is not on PATH; the skill cannot run any command yet'}
    if not profile['initialized']:
        return {'action': 'init', 'command': 'shimo-net init --origin https://shimo.net',
                'reason': 'no service origin is configured'}
    if not profile['credentials']:
        return {'action': 'login', 'command': 'shimo-net login',
                'reason': 'no stored credentials; the user logs in through the browser'}
    return {'action': 'none', 'command': None, 'reason': 'ready'}


def check():
    path = shutil.which('shimo-net')
    version = cli_version(path) if path else None
    cli = {'found': path is not None, 'path': path, 'version': version,
           'minimum': MINIMUM_CLI,
           'outdated': bool(version) and parse_version(version) < parse_version(MINIMUM_CLI)}
    profile = profile_state()
    return {'ready': cli['found'] and profile['credentials'],
            'skillVersion': skill_version(),
            'cli': cli,
            'origin': os.environ.get('SHIMO_NET_ORIGIN'),
            'profile': profile,
            'python': sys.version.split()[0],
            'next': next_step(cli, profile)}


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--pretty', action='store_true', help='indent the JSON output')
    args = parser.parse_args()
    report = check()
    print(json.dumps(report, ensure_ascii=False, indent=2 if args.pretty else None))
    return 0


if __name__ == '__main__':
    sys.exit(main())
