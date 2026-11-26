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
const { spawn, execFile } = require('child_process');

function bundledBinary(root) {
  const system = { darwin: 'darwin', linux: 'linux', win32: 'windows' }[process.platform];
  const arch = { x64: 'amd64', arm64: 'arm64' }[process.arch];
  if (!system || !arch) throw new Error(`Unsupported platform: ${process.platform}/${process.arch}`);
  const asset = `shimo-net_${system}_${arch}${system === 'windows' ? '.exe' : ''}`;
  const directory = path.join(root, 'binaries');
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
  return binary;
}

function stableVersion(value) {
  if (typeof value !== 'string' || !/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(value)) {
    throw new Error('Expected a stable npm version; no update performed.');
  }
  return value.split('.').map(BigInt);
}

function newerVersion(latest, current) {
  const a = stableVersion(latest), b = stableVersion(current);
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] > b[i];
  }
  return false;
}

function npmCLI() {
  const dirs = (process.env.PATH || '').split(path.delimiter).filter(Boolean);
  dirs.push(path.dirname(process.execPath));
  for (const dir of dirs) {
    const candidates = [path.join(dir, 'node_modules/npm/bin/npm-cli.js')];
    if (process.platform !== 'win32') {
      try { candidates.unshift(fs.realpathSync(path.join(dir, 'npm'))); } catch (_) { /* Try next location. */ }
    }
    for (const candidate of candidates) {
      if (path.basename(candidate) === 'npm-cli.js' && fs.existsSync(candidate)) return candidate;
    }
  }
  throw new Error('Cannot locate npm-cli.js. Install npm, then retry shimo-net update.');
}

function runNpm(cli, args, cwd) {
  return new Promise((resolve, reject) => {
    execFile(process.execPath, [cli, ...args, '--registry=https://registry.npmjs.org'], {
      cwd, shell: false, timeout: 120000, maxBuffer: 1024 * 1024, windowsHide: true,
      killSignal: 'SIGKILL', encoding: 'utf8'
    }, (error, stdout) => {
      if (error) reject(new Error(`npm ${args[0]} failed (${error.killed ? 'timeout/output limit' : error.code || 'unknown error'}). Retry using npm install shimo-net-cli@latest with the original installation scope.`));
      else resolve(stdout.trim());
    });
  });
}

async function installationScope(root, cli) {
  const modules = path.dirname(root), prefix = path.dirname(modules);
  if (path.basename(modules) !== 'node_modules' || root.split(path.sep).some(p => p === '_npx' || p === '.pnpm') ||
      root.split(path.sep).filter(p => p === 'node_modules').length !== 1) {
    throw new Error('Cannot safely update this installation. Use its original package manager or reinstall shimo-net-cli.');
  }
  const projectFile = path.join(prefix, 'package.json');
  if (fs.existsSync(projectFile)) {
    const project = JSON.parse(fs.readFileSync(projectFile, 'utf8'));
    if (!['dependencies', 'devDependencies', 'optionalDependencies'].some(key => project[key] && project[key]['shimo-net-cli'])) {
      throw new Error('This is not a direct project dependency. Update it with the original package manager.');
    }
    if ((project.packageManager && !/^npm@/.test(project.packageManager)) ||
        ['pnpm-lock.yaml', 'yarn.lock', 'bun.lock', 'bun.lockb'].some(file => fs.existsSync(path.join(prefix, file)))) {
      throw new Error('This project uses another package manager. Update shimo-net-cli with that package manager.');
    }
    return { prefix, args: ['--global=false', '--prefix', prefix, '--workspaces=false'], scope: 'project' };
  }
  if (process.platform !== 'win32' && path.basename(prefix) === 'lib') {
    return { prefix: path.dirname(prefix), args: ['--global', '--prefix', path.dirname(prefix)], scope: 'global' };
  }
  const globalRoot = await runNpm(cli, ['root', '--global'], prefix);
  if (fs.realpathSync(globalRoot) === modules) {
    return { prefix, args: ['--global', '--prefix', prefix], scope: 'global' };
  }
  throw new Error('Cannot identify this npm installation scope. Update it with the original npm install command.');
}

async function updatePackage(args, root) {
  if (args.some(arg => !['update', '--check', '--json'].includes(arg)) || args.filter(arg => arg === 'update').length !== 1) {
    throw new Error('Usage: shimo-net update [--check] [--json]');
  }
  const metadata = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'));
  if (metadata.name !== 'shimo-net-cli') throw new Error('Unexpected npm package name; nothing updated.');
  stableVersion(metadata.version);
  const cli = npmCLI();
  const scope = await installationScope(root, cli);
  const latest = JSON.parse(await runNpm(cli, ['view', 'shimo-net-cli@latest', 'version', '--json'], scope.prefix));
  const available = newerVersion(latest, metadata.version);
  const result = { ok: true, source: 'npm', currentVersion: metadata.version,
    latestVersion: latest, updateAvailable: available, updated: false, checkOnly: args.includes('--check'), path: root, scope: scope.scope };
  if (available && !args.includes('--check')) {
    await runNpm(cli, ['install', ...scope.args, `shimo-net-cli@${latest}`, '--ignore-scripts', '--no-audit', '--no-fund'], scope.prefix);
    const installed = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'));
    if (installed.name !== 'shimo-net-cli' || installed.version !== latest) {
      throw new Error('npm finished but the current installation has not reached the requested version. Check the installation path.');
    }
    const binary = bundledBinary(root);
    await new Promise((resolve, reject) => {
      execFile(binary, ['version', '--json'], { timeout: 10000, maxBuffer: 65536, shell: false, windowsHide: true }, (error, stdout) => {
        let reported;
        try { reported = JSON.parse(stdout).version; } catch (_) { /* Invalid version output fails below. */ }
        if (error || reported !== latest) reject(new Error('Updated runtime did not report the expected version. Reinstall shimo-net-cli using npm.'));
        else resolve();
      });
    });
    result.updated = true;
  }
  const message = result.updated ? `Updated shimo-net ${metadata.version} -> ${latest}` :
    available ? `Update available: ${metadata.version} -> ${latest}; run shimo-net update` : `shimo-net ${metadata.version} is up to date`;
  process.stdout.write(args.includes('--json') ? JSON.stringify(result) + '\n' : message + '\n');
}

async function main() {
  const args = process.argv.slice(2);
  const root = fs.realpathSync(path.join(__dirname, '..'));
  const binary = bundledBinary(root);
  const command = args.find(arg => arg !== '--json' && arg !== '--relogin');
  if (command === 'update' && !args.includes('--help') && !args.includes('-h')) {
    await updatePackage(args, root);
    return;
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
main().catch(error => {
  const args = process.argv.slice(2);
  process.stderr.write(args.includes('update') && args.includes('--json') ?
    JSON.stringify({ ok: false, error: error.message }) + '\n' : `shimo-net: ${error.message}\n`);
  process.exitCode = 1;
});
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
shimo-net update --check
shimo-net update
```

需要 Node.js 18+（建议使用受支持的 Node.js LTS）。本包包含 macOS/Linux/Windows 的 amd64、arm64 完整 Go 二进制，无第三方 npm 依赖、无安装脚本、无首次执行下载，不访问 GitHub 获取运行程序。每次启动会校验包内二进制 SHA-256，参数、标准输入输出及退出码传递给 `shimo-net`。

`shimo-net update` 通过官方 npm registry 更新当前安装范围中的整个包；`--check` 只检查。支持可识别的 npm 全局安装和项目直接依赖；其他包管理器或无法识别的安装位置应使用原安装方式更新。更新不会安装或修改助手技能。

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
