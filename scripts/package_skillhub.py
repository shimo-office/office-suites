#!/usr/bin/env python3
"""Build a minimal SkillHub upload ZIP using only Python's standard library."""
import argparse
import json
from pathlib import Path
import re
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def scalar(header, key):
    # The maintained source uses plain single-line metadata. Fail closed on
    # duplicates or unsupported YAML syntax rather than guessing its meaning.
    matches = re.findall(r'^' + re.escape(key) + r': ([^\r\n]+)$', header, re.M)
    if len(matches) != 1 or matches[0].startswith(('"', "'", '|', '>')):
        raise ValueError(f'{key} must occur once as a plain single-line value')
    return matches[0]


def build(root, out_dir, tag=None):
    source = (root / 'docs/agent/SKILL.md').read_text(encoding='utf-8')
    if not source.startswith('---\n') or '\n---\n' not in source[4:]:
        raise ValueError('SKILL.md requires YAML frontmatter')
    header = source[4:].split('\n---\n', 1)[0]
    name, version = scalar(header, 'name'), scalar(header, 'version')
    if not re.fullmatch(r'[a-z0-9]+(?:-[a-z0-9]+)*', name) or not 2 <= len(name) <= 128:
        raise ValueError('invalid skill slug')
    # Release packaging currently supports stable numeric release versions.
    if not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', version):
        raise ValueError('invalid stable release version')
    if tag is not None and tag != 'v' + version:
        raise ValueError('release tag does not match SKILL.md version')
    extra = {'slug': name, 'displayName': scalar(header, 'display_name'),
             'summary': scalar(header, 'description'), 'license': 'Proprietary',
             'homepage': 'https://github.com/shimo-office/office-suites'}
    for key in extra:
        if re.search(r'^' + re.escape(key) + r'\s*:', header, re.M):
            raise ValueError(f'{key} already exists; reconcile packaging metadata')
    skill = '---\n' + ''.join(f'{key}: {json.dumps(value, ensure_ascii=False)}\n'
                             for key, value in extra.items()) + source[4:]
    # Explicit allowlist: never zip the checkout, credentials, or build outputs.
    files = {'SKILL.md': skill.encode('utf-8'),
             'LICENSE.md': (root / 'LICENSE').read_bytes()}
    out_dir.mkdir(parents=True, exist_ok=True)
    archive = out_dir / f'{name}-v{version}-skillhub.zip'
    with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED) as z:
        for name, data in files.items():
            entry = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
            entry.compress_type = zipfile.ZIP_DEFLATED
            entry.create_system = 3
            entry.external_attr = 0o100644 << 16
            z.writestr(entry, data)
    return archive


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out-dir', type=Path, default=ROOT / 'dist')
    parser.add_argument('--tag', help='Require this release tag to match the skill version')
    args = parser.parse_args()
    try:
        print(build(ROOT, args.out_dir, args.tag))
    except (ValueError, OSError) as error:
        parser.exit(1, f'{error}\n')


if __name__ == '__main__':
    main()
