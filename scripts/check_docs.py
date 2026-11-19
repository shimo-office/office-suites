#!/usr/bin/env python3
"""Check maintained documentation links and migration/distribution contracts."""
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]


def tracking_errors(root, paths):
    result = subprocess.run(['git', 'check-ignore', '--no-index', '--stdin'],
                            input='\n'.join(paths) + '\n', cwd=root,
                            capture_output=True, text=True, encoding='utf-8')
    if result.returncode not in (0, 1):
        return [f'cannot check Git visibility: {result.stderr.strip()}']
    return [f'required source is ignored by Git: {name}' for name in result.stdout.splitlines()]


def link_errors(page):
    text = page.read_text(encoding='utf-8')
    text = re.sub(r'(?ms)^\s*(`{3,}|~{3,}).*?^\s*\1\s*$', '', text)
    targets = re.findall(r'!?\[[^\]\n]*\]\((<[^>]+>|[^)]+)\)', text)
    targets += re.findall(r'(?m)^\s*\[[^\]]+\]:\s*(<[^>]+>|\S+)', text)
    errors = []
    for raw in targets:
        target = raw.strip()
        if target.startswith('<'):
            target = target[1:target.index('>')]
        else:
            target = re.split(r'\s+["\']', target, maxsplit=1)[0]
        if target.startswith('#') or urlsplit(target).scheme:
            continue
        path = unquote(target.split('#', 1)[0].split('?', 1)[0])
        if not path:
            continue
        # Local line-number links used in an audit still point to the same file.
        path = re.sub(r':\d+$', '', path)
        if not (page.parent / path).exists():
            errors.append(f'{page}: missing target {target}')
    return errors


def check(root=ROOT):
    required = ['docs/README.md', 'docs/install.md', 'docs/cli-reference.md',
                'docs/scenarios.md', 'docs/agent/SKILL.md',
                'docs/internal/capabilities.json', 'docs/internal/capability-matrix.md',
                'docs/internal/test-results.md', 'LICENSE']
    errors = [f'missing canonical document {name}' for name in required if not (root / name).is_file()]
    pages = [root / name for name in ('README.md', 'SECURITY.md', 'CHANGELOG.md')]
    pages += sorted((root / 'docs').rglob('*.md')) + sorted((root / 'cli').glob('*.md'))
    errors.extend(tracking_errors(root, sorted(set(required + [p.relative_to(root).as_posix() for p in pages]))))
    for page in pages:
        errors.extend(link_errors(page))
    redirects = ['cli/README.md', 'cli/SKILL.md', 'cli/scenarios.md', 'cli/安装使用说明.md',
                 'docs/internal-capability-matrix.md', 'docs/internal-capability-test-results.md']
    redirects += [f'docs/tutorials/{name}.md' for name in ('README','workbuddy','codex','dsh','doubao','validation','browser')]
    for name in redirects:
        p = root / name
        if not p.is_file() or len(p.read_text(encoding='utf-8').splitlines()) > 15:
            errors.append(f'old path must be a short redirect: {name}')
    public = [root / 'README.md', *(root / 'docs/agent').glob('*.md')]
    public += [root / f'docs/{name}.md' for name in ('README', 'install', 'scenarios', 'cli-reference')]
    legacy_mentions = []
    for page in public:
        if not page.is_file():
            continue
        text = page.read_text(encoding='utf-8')
        if re.search(r'(?im)^#{1,6}\s+.*(?:一期|二期|phase2)', text):
            errors.append(f'internal phase in public heading: {page}')
        legacy_mentions.extend([page.relative_to(root).as_posix()] * text.count('phase2'))
        if page.name != 'scenarios.md' and re.search(r'(?m)^\|\s*(?:23|27|28)\s*\|', text):
            errors.append(f'duplicated scenario table: {page}')
    if legacy_mentions != ['docs/cli-reference.md']:
        errors.append(f'phase2 must have one compatibility note in cli-reference, found {legacy_mentions}')
    for name, expected in [('install.command','docs/agent/SKILL.md'),
                           ('install.ps1','docs\\agent\\SKILL.md'),
                           ('.github/workflows/release.yml','docs/agent/SKILL.md')]:
        if expected not in (root / name).read_text(encoding='utf-8'):
            errors.append(f'skill distribution must use canonical source: {name}')
    if '"$HOME/.dsh/skills/shimo-net-cli/SKILL.md"' not in (root / 'install.command').read_text(encoding='utf-8'):
        errors.append('DSH skill installation must keep the shimo-net-cli/SKILL.md destination')
    if (root / 'docs/images/0.9.6').exists():
        errors.append('internal images still use a version-named directory')
    skill = root / 'docs/agent/SKILL.md'
    if skill.exists():
        declared = re.search(r'(?m)^version:\s*(\S+)',skill.read_text(encoding='utf-8'))
        version = re.search(r'const version = "([^"]+)"',(root / 'cli/main.go').read_text(encoding='utf-8'))
        if not declared or not version or declared.group(1) != version.group(1):
            errors.append('skill and CLI versions differ')
    return pages, errors


def main():
    # A C locale can decode UTF-8 filenames as surrogates before Git sees them.
    # Restart before walking paths; changing subprocess text encoding is too late.
    if sys.getfilesystemencoding().lower().replace('-', '') != 'utf8':
        if sys.flags.utf8_mode:
            print('UTF-8 filesystem encoding is required to check documentation.', file=sys.stderr)
            return 1
        return subprocess.call([sys.executable, '-X', 'utf8', __file__, *sys.argv[1:]])
    pages, errors = check()
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    print(f'Documentation checks passed: {len(pages)} Markdown files, local links, redirects, public naming and skill distribution.')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
