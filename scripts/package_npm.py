#!/usr/bin/env python3
"""Package all six CLI binaries for npm, without install-time downloads."""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import re
import tarfile

from skill_runtime import ASSETS

ROOT = Path(__file__).resolve().parents[1]

LAUNCHER = r'''#!/usr/bin/env node
'use strict';
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const { spawn } = require('child_process');

function main() {
  const system = { darwin: 'darwin', linux: 'linux', win32: 'windows' }[process.platform];
  const arch = { x64: 'amd64', arm64: 'arm64' }[process.arch];
  if (!system || !arch) throw new Error(`Unsupported platform: ${process.platform}/${process.arch}`);
  const asset = `shimo-net_${system}_${arch}${system === 'windows' ? '.exe' : ''}`;
  const directory = path.join(__dirname, '..', 'binaries');
  const binary = path.join(directory, asset);
  let expected;
  let actual;
  try {
    const info = fs.lstatSync(binary);
    if (!info.isFile() || info.isSymbolicLink()) throw new Error('Not a regular binary');
    expected = JSON.parse(fs.readFileSync(path.join(directory, 'sha256.json'), 'utf8'))[asset];
    actual = crypto.createHash('sha256').update(fs.readFileSync(binary)).digest('hex');
  } catch (_) {
    throw new Error('Bundled runtime is missing or unreadable. Reinstall shimo-net-cli from the official npm registry.');
  }
  if (!/^[a-f0-9]{64}$/.test(expected || '') || actual !== expected) {
    throw new Error('Bundled runtime SHA-256 mismatch; nothing executed. Reinstall shimo-net-cli from the official npm registry.');
  }
  const child = spawn(binary, process.argv.slice(2), { stdio: 'inherit', shell: false });
  const signals = ['SIGINT', 'SIGTERM', 'SIGHUP'];
  const forward = signal => {
    if (child.exitCode === null && child.signalCode === null) {
      child.kill(process.platform === 'win32' && signal === 'SIGHUP' ? 'SIGTERM' : signal);
    }
  };
  const handlers = signals.map(signal => () => forward(signal));
  signals.forEach((signal, index) => process.on(signal, handlers[index]));
  const cleanup = () => signals.forEach((signal, index) => process.removeListener(signal, handlers[index]));
  child.on('error', error => {
    cleanup();
    process.stderr.write(`shimo-net: Cannot start bundled runtime (${error.code || 'unknown error'}). Reinstall shimo-net-cli.\n`);
    process.exitCode = 1;
  });
  child.on('exit', (code, signal) => {
    cleanup();
    if (signal) {
      try { process.kill(process.pid, signal); } catch (_) { process.exitCode = 1; }
    } else {
      process.exitCode = code === null ? 1 : code;
    }
  });
}
try { main(); } catch (error) {
  process.stderr.write(`shimo-net: ${error.message}\n`);
  process.exitCode = 1;
}
'''


def release_version(root, tag):
    source = (root / 'cli/main.go').read_text(encoding='utf-8')
    matches = re.findall(r'^const version = "([^"]+)"$', source, re.M)
    if len(matches) != 1 or not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', matches[0]):
        raise ValueError('CLI must declare exactly one stable release version')
    version = matches[0]
    skill = (root / 'docs/agent/SKILL.md').read_text(encoding='utf-8')
    if re.findall(r'^version: (\S+)$', skill, re.M) != [version]:
        raise ValueError('CLI and skill versions must match')
    if tag is not None and tag != 'v' + version:
        raise ValueError('release tag does not match CLI version')
    return version


def build(root, assets_dir, out_dir, tag=None):
    version = release_version(root, tag)
    binaries = {}
    for name in sorted(ASSETS):
        source = assets_dir / name
        if source.is_symlink() or not source.is_file() or not source.stat().st_size:
            raise ValueError(f'missing regular release binary: {name}')
        binaries[name] = source.read_bytes()
    manifest = {name: hashlib.sha256(data).hexdigest() for name, data in binaries.items()}
    metadata = {
        'name': 'shimo-net-cli', 'version': version,
        'description': 'Shimo office CLI with bundled native binaries for macOS, Linux and Windows.',
        'license': 'SEE LICENSE IN LICENSE',
        'author': 'shimo-office',
        'homepage': 'https://github.com/shimo-office/office-suites',
        'repository': {'type': 'git', 'url': 'git+https://github.com/shimo-office/office-suites.git'},
        'bugs': {'url': 'https://github.com/shimo-office/office-suites/issues'},
        'bin': {'shimo-net': 'bin/shimo-net.cjs'},
        'engines': {'node': '>=18'},
        'os': ['darwin', 'linux', 'win32'], 'cpu': ['x64', 'arm64'],
        'publishConfig': {'access': 'public', 'registry': 'https://registry.npmjs.org/'},
    }
    readme = f'''# shimo-net-cli {version}

石墨 shimo.net 及客户私有部署的命令行工具。npm 包名是 `shimo-net-cli`，安装后的命令是 `shimo-net`。

```sh
npm install --global shimo-net-cli@{version} --registry=https://registry.npmjs.org
shimo-net version
shimo-net --help
shimo-net schema --json
```

需要 Node.js 18+（建议使用受支持的 Node.js LTS）。本包包含 macOS/Linux/Windows 的 amd64、arm64 完整 Go 二进制，无第三方 npm 依赖、无安装脚本、无首次执行下载，不访问 GitHub 获取运行程序。每次启动会校验包内二进制 SHA-256，参数、标准输入输出及退出码传递给 `shimo-net`。

安装成功不等于石墨已登录。文件链接优先在宿主内置预览展示；新建、导入或编辑后同样展示实际结果链接。内置工具能操作就优先使用，只能预览时使用可用的外部 Computer Use / Browser Use 并复用会话。预览不代表保存成功，纯页面任务无需先登录 CLI。

确需 CLI/API 时，先用 `shimo-net doctor --no-browser --json` 和 `shimo-net whoami --no-browser --json` 检查；这不会接管其他浏览器。需要配置 CLI 登录时，说明其独立浏览器要求并沿用用户授权，再执行 `shimo-net login`。各通道登录态可能独立；不要公开或手动搬运密码、cookie。完整选择规则见 [浏览器规则](https://github.com/shimo-office/office-suites/blob/v{version}/docs/scenarios.md#浏览器优先级)。

CLI 本身不依赖 Node.js；直接二进制安装方式仍可用。完整说明见 [仓库安装文档](https://github.com/shimo-office/office-suites/blob/v{version}/docs/install.md)、[命令参考](https://github.com/shimo-office/office-suites/blob/v{version}/docs/cli-reference.md) 和 [安全说明](https://github.com/shimo-office/office-suites/blob/v{version}/SECURITY.md)。许可证见本包 LICENSE。
'''
    files = {
        'package.json': (json.dumps(metadata, ensure_ascii=False, indent=2) + '\n').encode(),
        'README.md': readme.encode(), 'LICENSE': (root / 'LICENSE').read_bytes(),
        'bin/shimo-net.cjs': LAUNCHER.encode(),
        'binaries/sha256.json': (json.dumps(manifest, indent=2) + '\n').encode(),
        **{'binaries/' + name: data for name, data in binaries.items()},
    }
    output = io.BytesIO()
    with gzip.GzipFile(filename='', mode='wb', fileobj=output, mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode='w', format=tarfile.USTAR_FORMAT) as archive:
            for name, data in sorted(files.items()):
                entry = tarfile.TarInfo('package/' + name)
                entry.size = len(data)
                entry.mode = 0o755 if name == 'bin/shimo-net.cjs' or name in {'binaries/' + asset for asset in ASSETS} else 0o644
                archive.addfile(entry, io.BytesIO(data))
    out_dir.mkdir(parents=True, exist_ok=True)
    destination = out_dir / f'shimo-net-cli-{version}.tgz'
    destination.write_bytes(output.getvalue())
    return destination


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tag', help='Require CLI version to match this release tag')
    parser.add_argument('--assets-dir', type=Path, default=ROOT / 'dist')
    parser.add_argument('--out-dir', type=Path, default=ROOT / 'dist')
    args = parser.parse_args()
    try:
        print(build(ROOT, args.assets_dir, args.out_dir, args.tag))
    except (ValueError, OSError) as error:
        parser.exit(1, f'{error}\n')


if __name__ == '__main__':
    main()
