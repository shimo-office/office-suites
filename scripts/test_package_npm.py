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
                command = ['cmd.exe', '/d', '/s', '/c', f'""{shim}" --version"']
            else:
                command = [str(shim), '--version']
            launched = subprocess.run(command, capture_output=True, timeout=15)
            self.assertEqual(launched.returncode, 0, launched.stderr.decode(errors='replace'))
            self.assertEqual(launched.stdout, subprocess.check_output([NODE, '--version']))


if __name__ == '__main__':
    unittest.main()
