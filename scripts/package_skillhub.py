#!/usr/bin/env python3
"""Build usable SkillHub and WorkBuddy packages without embedding CLI binaries."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import posixpath
import re
from urllib.parse import quote, unquote, urlsplit
import zipfile

ROOT = Path(__file__).resolve().parents[1]
REFERENCES = ('docs/install.md', 'docs/scenarios.md', 'docs/cli-reference.md',
              'SECURITY.md', 'docs/agent/runtime.md')
REFERENCE_PATHS = {name: 'references/' + Path(name).name for name in REFERENCES}


def local_links(text, source_path, destination, version):
    """Keep bundled references local; pin other repository links to the release."""
    def replace(match):
        label, target = match.groups()
        url = urlsplit(target)
        github_prefix = 'https://github.com/shimo-office/office-suites/blob/main/'
        if target.startswith(github_prefix):
            repository_path = unquote(url.path.split('/blob/main/', 1)[1])
        elif url.scheme or target.startswith(('#', '//')):
            return match.group(0)
        else:
            repository_path = posixpath.normpath(posixpath.join(posixpath.dirname(source_path), unquote(url.path)))
        suffix = ('?' + url.query if url.query else '') + ('#' + url.fragment if url.fragment else '')
        if repository_path in REFERENCE_PATHS:
            path = posixpath.relpath(REFERENCE_PATHS[repository_path], posixpath.dirname(destination) or '.')
            return f'{label}({path}{suffix})'
        return f'{label}(https://github.com/shimo-office/office-suites/blob/v{version}/{quote(repository_path, safe="/")}{suffix})'
    return re.sub(r'(!?\[[^\]\n]*\])\(([^)\s]+)\)', replace, text)


def asset_digests(directory):
    result = {}
    for system in ('darwin', 'linux', 'windows'):
        for arch in ('amd64', 'arm64'):
            name = f'shimo-net_{system}_{arch}' + ('.exe' if system == 'windows' else '')
            path = directory / name
            if path.is_symlink() or not path.is_file() or not path.stat().st_size:
                raise ValueError(f'missing regular release binary: {name}')
            result[name] = hashlib.sha256(path.read_bytes()).hexdigest()
    return result


def scalar(header, key):
    # The maintained source uses plain single-line metadata. Fail closed on
    # duplicates or unsupported YAML syntax rather than guessing its meaning.
    matches = re.findall(r'^' + re.escape(key) + r': ([^\r\n]+)$', header, re.M)
    if len(matches) != 1 or matches[0].startswith(('"', "'", '|', '>')):
        raise ValueError(f'{key} must occur once as a plain single-line value')
    return matches[0]


PLAIN_SCALAR_START = ('"', "'", '[', '{', '|', '>', '&', '*', '!', '%', '@', '`')


def validate_header(header):
    """Fail before upload on YAML that readers reject with a mapping error.

    A plain scalar containing ": " is parsed as a nested mapping, which the
    marketplace reports as "mapping values are not allowed in this context".
    Line numbers match the platform report, where the first metadata line is 1.
    """
    for index, line in enumerate(header.split('\n'), start=1):
        if not line.strip() or line.lstrip().startswith('#'):
            continue
        stripped = line.lstrip()
        if len(stripped) != len(line):
            if not stripped.startswith('- '):
                raise ValueError(f'frontmatter line {index}: unsupported indentation')
            key, value = 'list item', stripped[2:]
        else:
            match = re.fullmatch(r'([A-Za-z_][\w-]*):(?: (.*))?', line)
            if not match:
                raise ValueError(f'frontmatter line {index}: expected "key: value"')
            key, value = match.group(1), match.group(2) or ''
        value = value.strip()
        if not value or value.startswith(PLAIN_SCALAR_START):
            continue
        if ': ' in value or value.endswith(':'):
            raise ValueError(f'frontmatter line {index}: {key} needs quotes; '
                             'an unquoted colon ends the scalar')


def build(root, out_dir, tag=None, target='skillhub', digests=None):
    source = (root / 'docs/agent/SKILL.md').read_text(encoding='utf-8')
    if not source.startswith('---\n') or '\n---\n' not in source[4:]:
        raise ValueError('SKILL.md requires YAML frontmatter')
    header = source[4:].split('\n---\n', 1)[0]
    name, version = scalar(header, 'name'), scalar(header, 'version')
    if not re.fullmatch(r'[a-z0-9]+(?:-[a-z0-9]+)*', name) or not 2 <= len(name) <= 128:
        raise ValueError('invalid skill slug')
    # Skill and CLI versions stay in lockstep: the marketplace compares the skill
    # version, and the launcher pins the same release tag for its downloads.
    if not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', version):
        raise ValueError('invalid stable release version')
    if tag is not None and tag != 'v' + version:
        raise ValueError('release tag does not match SKILL.md version')
    if target not in ('skillhub', 'workbuddy'):
        raise ValueError('unknown marketplace')
    extra = {'license': 'Proprietary', 'homepage': 'https://github.com/shimo-office/office-suites'}
    if target == 'skillhub':
        extra.update({'slug': name, 'displayName': scalar(header, 'display_name'),
                      'summary': scalar(header, 'description')})
    for key in extra:
        if re.search(r'^' + re.escape(key) + r'\s*:', header, re.M):
            raise ValueError(f'{key} already exists; reconcile packaging metadata')
    skill = '---\n' + ''.join(f'{key}: {json.dumps(value, ensure_ascii=False)}\n'
                             for key, value in extra.items()) + source[4:]
    skill_header, body = skill.split('\n---\n', 1)
    # Validate what the marketplace will parse, after metadata injection.
    validate_header(skill_header.split('\n', 1)[1])
    skill = skill_header + '\n---\n' + body
    skill = local_links(skill, 'docs/agent/SKILL.md', 'SKILL.md', version)
    from skill_runtime import render_launchers
    if digests is None:
        digests = asset_digests(root / 'dist')
    # Explicit allowlist: never zip the checkout, credentials, or build outputs.
    files = {'SKILL.md': skill.encode('utf-8'),
             'LICENSE.md': (root / 'LICENSE').read_bytes()}
    files.update(render_launchers(version, digests))
    for path, destination in REFERENCE_PATHS.items():
        files[destination] = local_links((root / path).read_text(encoding='utf-8'),
                                         path, destination, version).encode('utf-8')
    limit = 3_000_000 if target == 'workbuddy' else 10_000_000
    if len(files) > 200 or sum(map(len, files.values())) > limit:
        raise ValueError('marketplace package file count or expanded size exceeded')
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, 'w', compression=zipfile.ZIP_DEFLATED) as z:
        for filename, data in files.items():
            # Marketplaces require a single top-level {skill-name}/ directory.
            entry = zipfile.ZipInfo(f'{name}/{filename}', date_time=(1980, 1, 1, 0, 0, 0))
            entry.compress_type = zipfile.ZIP_DEFLATED
            entry.create_system = 3
            entry.external_attr = (0o100755 if filename.endswith('.sh') else 0o100644) << 16
            z.writestr(entry, data)
    if len(buffer.getvalue()) > limit:
        raise ValueError('marketplace ZIP size exceeded')
    out_dir.mkdir(parents=True, exist_ok=True)
    archive = out_dir / f'{name}-v{version}-{target}.zip'
    archive.write_bytes(buffer.getvalue())
    return archive


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out-dir', type=Path, default=ROOT / 'dist')
    parser.add_argument('--tag', help='Require this release tag to match the skill version')
    parser.add_argument('--target', choices=['skillhub', 'workbuddy', 'both'], default='both')
    parser.add_argument('--assets-dir', type=Path, default=ROOT / 'dist', help='Six verified release binaries to hash')
    args = parser.parse_args()
    try:
        digests = asset_digests(args.assets_dir)
        for target in (('skillhub', 'workbuddy') if args.target == 'both' else (args.target,)):
            print(build(ROOT, args.out_dir, args.tag, target, digests))
    except (ValueError, OSError) as error:
        parser.exit(1, f'{error}\n')


if __name__ == '__main__':
    main()
