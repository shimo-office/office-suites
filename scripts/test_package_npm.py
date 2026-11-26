"""Exercise the self-contained npm archive and its native-process boundary."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tarfile
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('package_npm.py')
spec = importlib.util.spec_from_file_location('package_npm', SCRIPT)
packager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packager)
NODE = shutil.which('node')
NPM = shutil.which('npm')


class NpmArchiveTests(unittest.TestCase):
    def fixture(self, folder):
        root = Path(folder)
        (root / 'cli').mkdir()
        (root / 'cli/main.go').write_text('const version = "1.2.3"\n')
        (root / 'docs/agent').mkdir(parents=True)
        (root / 'docs/agent/SKILL.md').write_text('---\nversion: 1.2.3\n---\n')
        (root / 'LICENSE').write_text('All rights reserved.\n')
        (root / 'credentials.json').write_text('do not include')
        assets = root / 'assets'
        assets.mkdir()
        for name in packager.ASSETS:
            (assets / name).write_bytes(('binary:' + name).encode())
        return root, assets

    def test_archive_is_complete_reproducible_and_has_no_install_downloads(self):
        with tempfile.TemporaryDirectory() as folder:
            root, assets = self.fixture(folder)
            archive = packager.build(root, assets, root / 'out', 'v1.2.3')
            original = archive.read_bytes()
            with tarfile.open(archive) as tar:
                expected = {'package/package.json', 'package/README.md', 'package/LICENSE',
                            'package/bin/shimo-net.cjs', 'package/binaries/sha256.json'}
                expected.update('package/binaries/' + name for name in packager.ASSETS)
                self.assertEqual(set(tar.getnames()), expected)
                self.assertTrue(all(member.isfile() for member in tar.getmembers()))
                metadata = json.load(tar.extractfile('package/package.json'))
                self.assertEqual(metadata['name'], 'shimo-net-cli')
                self.assertEqual(metadata['version'], '1.2.3')
                self.assertEqual(metadata['bin'], {'shimo-net': 'bin/shimo-net.cjs'})
                self.assertNotIn('scripts', metadata)
                self.assertNotIn('dependencies', metadata)
                self.assertEqual(tar.extractfile('package/LICENSE').read(), (root / 'LICENSE').read_bytes())
                hashes = json.load(tar.extractfile('package/binaries/sha256.json'))
                for name in packager.ASSETS:
                    member = tar.getmember('package/binaries/' + name)
                    self.assertEqual(member.mode, 0o755)
                    payload = tar.extractfile(member).read()
                    self.assertEqual(hashes[name], hashlib.sha256(payload).hexdigest())
            self.assertEqual(packager.build(root, assets, root / 'out', 'v1.2.3').read_bytes(), original)

    def test_version_mismatch_is_rejected_before_output(self):
        for cli_version, skill_version, tag in [('1.2.3', '1.2.4', 'v1.2.3'),
                                                ('1.2.3', '1.2.3', 'v1.2.4'),
                                                ('../bad', '../bad', None)]:
            with self.subTest(tag=tag), tempfile.TemporaryDirectory() as folder:
                root, assets = self.fixture(folder)
                (root / 'cli/main.go').write_text(f'const version = "{cli_version}"\n')
                (root / 'docs/agent/SKILL.md').write_text(f'---\nversion: {skill_version}\n---\n')
                with self.assertRaises(ValueError):
                    packager.build(root, assets, root / 'out', tag)
                self.assertFalse((root / 'out').exists())

    def test_missing_and_empty_binaries_are_rejected(self):
        for kind in ['missing', 'empty']:
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as folder:
                root, assets = self.fixture(folder)
                binary = assets / sorted(packager.ASSETS)[0]
                binary.unlink()
                if kind == 'empty':
                    binary.touch()
                with self.assertRaises(ValueError):
                    packager.build(root, assets, root / 'out')

    def test_linked_binary_is_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            root, assets = self.fixture(folder)
            binary = assets / sorted(packager.ASSETS)[0]
            binary.unlink()
            try:
                binary.symlink_to(root / 'credentials.json')
            except OSError as error:
                self.skipTest(f'Cannot create test symlink on this platform: {error}')
            with self.assertRaises(ValueError):
                packager.build(root, assets, root / 'out')


@unittest.skipUnless(NODE, 'Node is required for npm launcher execution tests')
class NpmLauncherTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory()
        cls.root = Path(cls.temp.name)
        (cls.root / 'bin').mkdir()
        (cls.root / 'binaries').mkdir()
        platform_arch = json.loads(subprocess.check_output(
            [NODE, '-p', 'JSON.stringify([process.platform,process.arch])'], text=True))
        platform_name, arch = platform_arch
        system = {'darwin': 'darwin', 'linux': 'linux', 'win32': 'windows'}[platform_name]
        goarch = {'x64': 'amd64', 'arm64': 'arm64'}[arch]
        cls.asset = f'shimo-net_{system}_{goarch}' + ('.exe' if system == 'windows' else '')
        cls.binary = cls.root / 'binaries' / cls.asset
        shutil.copyfile(NODE, cls.binary)
        cls.binary.chmod(0o755)
        cls.digest = hashlib.sha256(cls.binary.read_bytes()).hexdigest()
        cls.launcher = cls.root / 'bin/shimo-net.cjs'
        cls.launcher.write_text(packager.LAUNCHER, encoding='utf-8')
        cls.manifest = cls.root / 'binaries/sha256.json'

    @classmethod
    def tearDownClass(cls):
        cls.temp.cleanup()

    def setUp(self):
        self.manifest.write_text(json.dumps({self.asset: self.digest}))

    def run_launcher(self, args, **kwargs):
        return subprocess.run([NODE, str(self.launcher), *args], capture_output=True, timeout=15, **kwargs)

    def test_arguments_stdio_and_exit_status_are_forwarded_without_shell(self):
        args = ['', 'two words', '"quoted"', 'trailing\\', '$HOME; touch nope', '中文', '--json']
        program = ('process.stdout.write(JSON.stringify(process.argv.slice(1)));'
                   'process.stderr.write("fixture stderr"); process.exitCode=23;')
        result = self.run_launcher(['-e', program, '--', *args])
        self.assertEqual(result.returncode, 23)
        self.assertEqual(json.loads(result.stdout), args)
        self.assertEqual(result.stderr, b'fixture stderr')
        result = self.run_launcher(['-e', 'process.stdin.pipe(process.stdout)'], input=b'stdin\x00content\n')
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, b'stdin\x00content\n')

    def test_corrupt_runtime_is_rejected_before_execution(self):
        self.manifest.write_text(json.dumps({self.asset: '0' * 64}))
        result = self.run_launcher(['-e', 'console.log("MUST NOT RUN")'])
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(b'MUST NOT RUN', result.stdout)
        self.assertIn(b'SHA-256', result.stderr)

    def test_missing_runtime_is_actionable(self):
        self.binary.rename(self.binary.with_suffix('.saved'))
        try:
            result = self.run_launcher(['--version'])
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b'Reinstall shimo-net-cli', result.stderr)
        finally:
            self.binary.with_suffix('.saved').rename(self.binary)

    def test_unsupported_platform_is_rejected(self):
        program = ('Object.defineProperty(process,"platform",{value:"freebsd"});'
                   'require(process.argv[1]);')
        result = subprocess.run([NODE, '-e', program, str(self.launcher)], capture_output=True, timeout=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b'Unsupported platform', result.stderr)

    @unittest.skipIf(os.name == 'nt', 'POSIX signal semantics')
    def test_termination_reaches_child_and_preserves_signal_status(self):
        import select
        program = ('process.on("SIGTERM",()=>{process.stderr.write("terminated\\n");process.exit(0)});'
                   'process.stdout.write("ready\\n");'
                   'setInterval(()=>{},1000);')
        process = subprocess.Popen([NODE, str(self.launcher), '-e', program],
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        try:
            self.assertTrue(select.select([process.stdout], [], [], 10)[0], 'child readiness timed out')
            self.assertEqual(process.stdout.readline(), b'ready\n')
            process.send_signal(signal.SIGTERM)
            _, stderr = process.communicate(timeout=10)
            self.assertEqual(stderr, b'terminated\n')
            self.assertEqual(process.returncode, 0)
        finally:
            if process.poll() is None:
                process.kill()
            process.communicate()

    @unittest.skipIf(os.name == 'nt', 'POSIX signal semantics')
    def test_signal_exit_is_preserved(self):
        result = self.run_launcher(['-e', 'process.kill(process.pid,"SIGTERM")'])
        self.assertEqual(result.returncode, -signal.SIGTERM)

    @unittest.skipUnless(NPM, 'npm is required for offline installation test')
    def test_npm_offline_install_creates_working_command_shim(self):
        npm_cli = Path(NPM).resolve()
        if os.name == 'nt':
            npm_cli = Path(NPM).parent / 'node_modules/npm/bin/npm-cli.js'
        if not npm_cli.is_file():
            self.skipTest('Cannot locate installed npm CLI JavaScript entry')
        with tempfile.TemporaryDirectory() as folder:
            root, assets = NpmArchiveTests().fixture(folder)
            # A real executable fixture on this host; the other five targets
            # are inert package fixtures, not claimed as validated CLI builds.
            shutil.copyfile(self.binary, assets / self.asset)
            archive = packager.build(root, assets, root / 'out', 'v1.2.3')
            prefix = root / 'install with spaces'
            userconfig, globalconfig = root / 'user.npmrc', root / 'global.npmrc'
            userconfig.touch()
            globalconfig.touch()
            result = subprocess.run([NODE, str(npm_cli), 'install', '--global',
                                     '--prefix', str(prefix), '--cache', str(root / 'cache'),
                                     '--userconfig', str(userconfig), '--globalconfig', str(globalconfig),
                                     '--offline', '--ignore-scripts', '--no-audit', '--no-fund',
                                     str(archive)], capture_output=True, timeout=90)
            self.assertEqual(result.returncode, 0, result.stderr.decode(errors='replace'))
            shim = prefix / ('shimo-net.cmd' if os.name == 'nt' else 'bin/shimo-net')
            self.assertTrue(shim.is_file())
            if os.name == 'nt':
                # Pass cwd separately: embedding a quoted absolute path inside
                # /c gets quoted again by subprocess.list2cmdline on Windows.
                # The real npm shim still has to handle its spaced directory.
                command = ['cmd.exe', '/d', '/c', 'shimo-net.cmd', '--version']
            else:
                command = [str(shim), '--version']
            launched = subprocess.run(command, cwd=prefix, capture_output=True, timeout=15)
            self.assertEqual(launched.returncode, 0, launched.stderr.decode(errors='replace'))
            self.assertEqual(launched.stdout, subprocess.check_output([NODE, '--version']))



@unittest.skipUnless(NODE, 'Node is required for update execution tests')
class NpmUpdateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.prefix = self.root / 'prefix with spaces'
        modules = self.prefix / ('node_modules' if os.name == 'nt' else 'lib/node_modules')
        self.package = modules / 'shimo-net-cli'
        (self.package / 'bin').mkdir(parents=True)
        (self.package / 'binaries').mkdir()
        system, arch = json.loads(subprocess.check_output([NODE, '-p',
            'JSON.stringify([process.platform,process.arch])'], text=True))
        system = {'win32': 'windows'}.get(system, system)
        arch = {'x64': 'amd64', 'arm64': 'arm64'}[arch]
        self.asset = f'shimo-net_{system}_{arch}' + ('.exe' if os.name == 'nt' else '')
        binary = self.package / 'binaries' / self.asset
        shutil.copyfile(NODE, binary)
        binary.chmod(0o755)
        (self.package / 'binaries/sha256.json').write_text(json.dumps({
            self.asset: hashlib.sha256(binary.read_bytes()).hexdigest()}))
        (self.package / 'package.json').write_text(json.dumps({'name': 'shimo-net-cli', 'version': '1.2.3'}))
        self.launcher = self.package / 'bin/shimo-net.cjs'
        self.launcher.write_text(packager.LAUNCHER)
        self.fake_bin = self.root / 'npm tools'
        cli = self.fake_bin / 'node_modules/npm/bin/npm-cli.js'
        cli.parent.mkdir(parents=True)
        cli.write_text(r"""
const fs = require('fs');
const a = process.argv.slice(2);
fs.appendFileSync(process.env.UPDATE_LOG, JSON.stringify(a) + '\n');
if (a[0] === 'root') { console.log(process.env.UPDATE_GLOBAL_ROOT); }
else if (a[0] === 'view') { console.log(JSON.stringify(process.env.UPDATE_LATEST || '1.3.0')); }
else if (a[0] === 'install') {
  if (process.env.UPDATE_FAIL) { console.error('fixture install failed'); process.exit(9); }
  if (!process.env.UPDATE_UNCHANGED) {
    const file = process.env.UPDATE_PACKAGE + '/package.json';
    const metadata = JSON.parse(fs.readFileSync(file));
    metadata.version = process.env.UPDATE_LATEST || '1.3.0';
    fs.writeFileSync(file, JSON.stringify(metadata));
  }
  if (process.env.UPDATE_CORRUPT) fs.writeFileSync(process.env.UPDATE_PACKAGE + '/binaries/sha256.json', '{}');
  console.log('npm output must not pollute JSON');
} else process.exit(8);
""")
        if os.name == 'nt':
            (self.fake_bin / 'npm.cmd').write_text('@echo off\n')
        else:
            (self.fake_bin / 'npm').symlink_to(cli)
        self.log = self.root / 'npm-log.jsonl'
        preload = self.root / 'runtime-fixture.cjs'
        preload.write_text(r"""
if (require('path').basename(process.argv[1] || '') === 'version') {
  const metadata = JSON.parse(require('fs').readFileSync(process.env.UPDATE_PACKAGE + '/package.json'));
  console.log(JSON.stringify({version: process.env.UPDATE_RUNTIME_VERSION || metadata.version}));
  process.exit(0);
}
""")
        self.env = dict(os.environ, PATH=str(self.fake_bin), UPDATE_LOG=str(self.log),
                        UPDATE_GLOBAL_ROOT=str(modules), UPDATE_PACKAGE=str(self.package),
                        NODE_OPTIONS='--require=' + json.dumps(str(preload)))

    def run_update(self, *args):
        return subprocess.run([NODE, str(self.launcher), *args], env=self.env,
                              capture_output=True, text=True, timeout=20)

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_check_is_read_only_and_json_clean(self):
        result = self.run_update('update', '--check', '--json')
        self.assertEqual(result.returncode, 0, result.stderr)
        data = json.loads(result.stdout)
        self.assertEqual(data['currentVersion'], '1.2.3')
        self.assertEqual(data['latestVersion'], '1.3.0')
        self.assertTrue(data['updateAvailable'])
        self.assertFalse(data['updated'])
        self.assertFalse(any(call[0] == 'install' for call in self.calls()))

    def test_global_update_targets_original_prefix_and_verifies_version(self):
        result = self.run_update('--json', 'update')
        self.assertEqual(result.returncode, 0, result.stderr)
        data = json.loads(result.stdout)
        self.assertTrue(data['updated'])
        install = next(call for call in self.calls() if call[0] == 'install')
        self.assertIn('--global', install)
        self.assertEqual(install[install.index('--prefix') + 1], str(self.prefix.resolve()))
        self.assertIn('shimo-net-cli@1.3.0', install)
        self.assertIn('--registry=https://registry.npmjs.org', install)
        self.assertIn('--ignore-scripts', install)

    def test_project_install_never_becomes_global(self):
        project = self.root / 'project'
        destination = project / 'node_modules/shimo-net-cli'
        destination.parent.mkdir(parents=True)
        shutil.move(str(self.package), destination)
        (project / 'package.json').write_text(json.dumps({'dependencies': {'shimo-net-cli': '^1.2.3'}}))
        self.package = destination
        self.launcher = destination / 'bin/shimo-net.cjs'
        self.env['UPDATE_PACKAGE'] = str(destination)
        result = self.run_update('update', '--json')
        self.assertEqual(result.returncode, 0, result.stderr)
        install = next(call for call in self.calls() if call[0] == 'install')
        self.assertNotIn('--global', install)
        self.assertIn('--global=false', install)
        self.assertIn('--workspaces=false', install)
        self.assertEqual(install[install.index('--prefix') + 1], str(project.resolve()))

    def test_newer_current_version_never_downgrades(self):
        self.env['UPDATE_LATEST'] = '1.2.2'
        result = self.run_update('update', '--json')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(json.loads(result.stdout)['updateAvailable'])
        self.assertFalse(any(call[0] == 'install' for call in self.calls()))

    def test_bad_arguments_fail_before_npm(self):
        for args in [('update', '--force'), ('update', 'other'), ('update', '--check=true')]:
            result = self.run_update(*args, '--json')
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(json.loads(result.stderr)['ok'])
        self.assertEqual(self.calls(), [])

    def test_equal_version_is_noop(self):
        self.env['UPDATE_LATEST'] = '1.2.3'
        result = self.run_update('update', '--json')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(json.loads(result.stdout)['updated'])
        self.assertFalse(any(call[0] == 'install' for call in self.calls()))

    def test_versions_are_compared_numerically(self):
        self.env['UPDATE_LATEST'] = '1.10.0'
        result = self.run_update('update', '--check', '--json')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(json.loads(result.stdout)['updateAvailable'])

    def test_npx_and_ambiguous_installations_are_not_modified(self):
        for location in ['_npx/temp/node_modules/shimo-net-cli', 'unknown/node_modules/shimo-net-cli']:
            with self.subTest(location=location):
                destination = self.root / location
                shutil.copytree(self.package, destination)
                launcher = destination / 'bin/shimo-net.cjs'
                result = subprocess.run([NODE, str(launcher), 'update', '--json'], env=self.env,
                                        capture_output=True, text=True, timeout=20)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(json.loads(result.stderr)['ok'])
                self.assertFalse(any(call[0] == 'install' for call in self.calls()))

    def test_other_package_manager_project_is_not_modified(self):
        project = self.root / 'yarn-project'
        destination = project / 'node_modules/shimo-net-cli'
        shutil.copytree(self.package, destination)
        (project / 'package.json').write_text(json.dumps({'dependencies': {'shimo-net-cli': '^1.2.3'}}))
        (project / 'yarn.lock').touch()
        result = subprocess.run([NODE, str(destination / 'bin/shimo-net.cjs'), 'update', '--json'],
                                env=self.env, capture_output=True, text=True, timeout=20)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('package manager', json.loads(result.stderr)['error'])
        self.assertEqual(self.calls(), [])

    def test_explicit_non_npm_package_manager_is_not_modified_without_lockfile(self):
        project = self.root / 'declared-project'
        destination = project / 'node_modules/shimo-net-cli'
        shutil.copytree(self.package, destination)
        for manager in ['yarn@4.0.0', 'pnpm@9.0.0', 'bun@1.2.0']:
            with self.subTest(manager=manager):
                (project / 'package.json').write_text(json.dumps({
                    'dependencies': {'shimo-net-cli': '^1.2.3'}, 'packageManager': manager}))
                result = subprocess.run([NODE, str(destination / 'bin/shimo-net.cjs'), 'update', '--json'],
                                        env=self.env, capture_output=True, text=True, timeout=20)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('package manager', json.loads(result.stderr)['error'])
                self.assertEqual(self.calls(), [])

    def test_invalid_registry_version_and_failed_install_are_errors(self):
        for flag, value in [('UPDATE_LATEST', '1.3.0-beta'), ('UPDATE_FAIL', '1'),
                            ('UPDATE_UNCHANGED', '1'), ('UPDATE_RUNTIME_VERSION', '0.0.1'), ('UPDATE_CORRUPT', '1')]:
            with self.subTest(flag=flag):
                self.env[flag] = value
                (self.package / 'package.json').write_text(json.dumps({'name': 'shimo-net-cli', 'version': '1.2.3'}))
                result = self.run_update('update', '--json')
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(json.loads(result.stderr)['ok'])
                self.env.pop(flag)


if __name__ == '__main__':
    unittest.main()
