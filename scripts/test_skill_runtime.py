"""Execute generated launchers with isolated caches and synthetic executables."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('skill_runtime.py')
spec = importlib.util.spec_from_file_location('skill_runtime', SCRIPT)
runtime = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runtime)
ASSETS = [f'shimo-net_{system}_{arch}' + ('.exe' if system == 'windows' else '')
          for system in ['darwin', 'linux', 'windows'] for arch in ['amd64', 'arm64']]
VERSION = '1.2.3'
ARGS = ['', 'two words', '"quoted"', 'trailing\\', '$HOME; touch nope', '中文', '--json']


def digests(payload):
    return {name: hashlib.sha256(payload).hexdigest() for name in ASSETS}


class RenderTests(unittest.TestCase):
    def test_exact_outputs_and_pinned_release(self):
        rendered = runtime.render_launchers(VERSION, digests(b'binary'))
        self.assertEqual(set(rendered), {'scripts/shimo-net.sh', 'scripts/shimo-net.ps1'})
        for payload in rendered.values():
            self.assertIsInstance(payload, bytes)
            self.assertIn(b'https://github.com/shimo-office/office-suites/releases/download/v1.2.3/', payload)
            self.assertNotIn(b'/latest/', payload)

    def test_invalid_release_or_hash_manifest_is_rejected(self):
        for version in ['v1.2.3', '1.2', '01.2.3', '1.2.3-rc.1', '1.2.3+foo', '1.2.3;id', None]:
            with self.subTest(version=version), self.assertRaises(ValueError):
                runtime.render_launchers(version, digests(b'binary'))
        for hashes in [{}, {**digests(b'binary'), 'extra': '0' * 64},
                       {**digests(b'binary'), ASSETS[0]: 'x' * 64},
                       {**digests(b'binary'), ASSETS[0]: '0' * 63}]:
            with self.subTest(hashes=hashes), self.assertRaises(ValueError):
                runtime.render_launchers(VERSION, hashes)


@unittest.skipIf(os.name == 'nt', 'POSIX launcher runs on macOS/Linux')
class PosixRuntimeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        system = {'Darwin': 'darwin', 'Linux': 'linux'}[platform.system()]
        arch = {'x86_64': 'amd64', 'amd64': 'amd64', 'arm64': 'arm64', 'aarch64': 'arm64'}[platform.machine()]
        self.asset = f'shimo-net_{system}_{arch}'
        self.payload = b'#!/bin/sh\nprintf "%s\\0" "$@"\nexit 23\n'
        self.launcher = self.root / 'launcher.sh'
        self.launcher.write_bytes(runtime.render_launchers(VERSION, digests(self.payload))['scripts/shimo-net.sh'])
        self.cache = self.root / 'cache with spaces'
        self.binary = self.cache / VERSION / self.asset
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        for name in ['uname', 'mkdir', 'mktemp', 'chmod', 'mv', 'rm', 'sha256sum', 'shasum']:
            command = shutil.which(name)
            if command:
                (self.bin / name).symlink_to(command)
        self.env = {**os.environ, 'PATH': str(self.bin), 'SHIMO_NET_RUNTIME_DIR': str(self.cache)}

    def run_launcher(self, args=ARGS):
        return subprocess.run(['/bin/sh', str(self.launcher), *args], env=self.env, capture_output=True)

    def cached(self, payload):
        self.binary.parent.mkdir(parents=True, exist_ok=True)
        self.binary.write_bytes(payload)
        self.binary.chmod(0o700)

    def fake_curl(self, payload):
        source = self.root / 'download'
        source.write_bytes(payload)
        # Record all arguments, then copy the synthetic response to curl's output path.
        curl = self.bin / 'curl'
        curl.write_text('#!/bin/sh\nprintf "%s\\n" "$@" > "$CURL_LOG"\n'
                        'while [ "$#" -gt 0 ]; do\n'
                        '  if [ "$1" = "--output" ]; then shift; out=$1; fi\n'
                        '  shift\ndone\n/bin/cp "$CURL_SOURCE" "$out"\n')
        curl.chmod(0o700)
        self.env.update(CURL_LOG=str(self.root / 'curl.log'), CURL_SOURCE=str(source))

    def test_verified_cache_preserves_arguments_and_exit_without_curl(self):
        self.cached(self.payload)
        result = self.run_launcher()
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertEqual(result.stdout, b'\0'.join(a.encode() for a in ARGS) + b'\0')

    def test_cached_mismatch_never_executes_or_downloads(self):
        self.cached(b'#!/bin/sh\nprintf BAD_EXECUTED\n')
        self.fake_curl(self.payload)
        result = self.run_launcher()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b'SHA256', result.stderr)
        self.assertNotIn(b'BAD_EXECUTED', result.stdout)
        self.assertFalse((self.root / 'curl.log').exists())

    def test_downloaded_mismatch_never_executes_or_enters_cache(self):
        self.fake_curl(b'#!/bin/sh\nprintf BAD_EXECUTED\n')
        result = self.run_launcher()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b'SHA256', result.stderr)
        self.assertNotIn(b'BAD_EXECUTED', result.stdout)
        self.assertFalse(self.binary.exists())
        self.assertEqual(list(self.binary.parent.iterdir()), [])

    def test_download_is_pinned_verified_and_cached(self):
        self.fake_curl(self.payload)
        result = self.run_launcher()
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertEqual(self.binary.read_bytes(), self.payload)
        curl_args = (self.root / 'curl.log').read_text().splitlines()
        self.assertIn(f'https://github.com/shimo-office/office-suites/releases/download/v{VERSION}/{self.asset}', curl_args)
        self.assertIn('--proto', curl_args)
        self.assertIn('--proto-redir', curl_args)
        self.assertIn('=https', curl_args)
        for option, value in [('--connect-timeout', '20'), ('--max-time', '180'), ('--retry', '2')]:
            self.assertEqual(curl_args[curl_args.index(option) + 1], value)
        self.binary.write_bytes(b'#!/bin/sh\nprintf BAD_EXECUTED\n')
        second = self.run_launcher()
        self.assertNotEqual(second.returncode, 0)
        self.assertIn(b'SHA256', second.stderr)
        self.assertNotIn(b'BAD_EXECUTED', second.stdout)

    def test_missing_prerequisites_are_clear(self):
        result = self.run_launcher()
        self.assertIn(b'curl', result.stderr)
        self.assertNotEqual(result.returncode, 0)
        for name in ['sha256sum', 'shasum']:
            (self.bin / name).unlink(missing_ok=True)
        result = self.run_launcher()
        self.assertIn(b'sha256sum', result.stderr)
        self.assertIn(b'shasum', result.stderr)
        self.assertNotEqual(result.returncode, 0)

    def test_cache_symlink_is_rejected(self):
        self.binary.parent.mkdir(parents=True)
        target = self.root / 'target'
        target.write_bytes(self.payload)
        target.chmod(0o700)
        self.binary.symlink_to(target)
        result = self.run_launcher()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b'regular file', result.stderr)

    def test_supported_platforms_select_the_pinned_asset(self):
        (self.bin / 'uname').unlink()
        (self.bin / 'uname').write_text('#!/bin/sh\ncase "$1" in -s) printf "%s" "$TEST_OS";; '
                                       '-m) printf "%s" "$TEST_ARCH";; esac\n')
        (self.bin / 'uname').chmod(0o700)
        for system, os_name in [('darwin', 'Darwin'), ('linux', 'Linux')]:
            for arch, machine in [('amd64', 'x86_64'), ('arm64', 'aarch64')]:
                with self.subTest(system=system, arch=arch):
                    self.env.update(TEST_OS=os_name, TEST_ARCH=machine)
                    asset = f'shimo-net_{system}_{arch}'
                    self.binary = self.cache / VERSION / asset
                    self.cached(self.payload)
                    result = self.run_launcher()
                    self.assertEqual(result.returncode, 23, result.stderr)
        self.env.update(TEST_OS='FreeBSD', TEST_ARCH='x86_64')
        result = self.run_launcher()
        self.assertIn(b'Unsupported OS', result.stderr)
        self.assertNotEqual(result.returncode, 0)

    def test_failed_download_cleans_partial_file(self):
        self.fake_curl(self.payload)
        curl = self.bin / 'curl'
        curl.write_text(curl.read_text() + 'exit 22\n')
        result = self.run_launcher()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b'Could not download', result.stderr)
        self.assertFalse(self.binary.exists())
        self.assertEqual(list(self.binary.parent.iterdir()), [])


@unittest.skipUnless(os.name == 'nt', 'Actual Windows PowerShell/native process verification requires Windows')
class WindowsRuntimeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.powershell = shutil.which('powershell.exe')
        if not cls.powershell or not shutil.which('go'):
            raise RuntimeError('Windows runtime verification requires PowerShell 5.1 and Go')
        cls.build = tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.build.cleanup)
        source = Path(cls.build.name) / 'main.go'
        source.write_text('package main\nimport("encoding/json";"os")\n'
                          'func main(){json.NewEncoder(os.Stdout).Encode(os.Args[1:]);os.Exit(23)}\n')
        binary = Path(cls.build.name) / 'fixture.exe'
        subprocess.run(['go', 'build', '-o', str(binary), str(source)], check=True, capture_output=True)
        cls.payload = binary.read_bytes()

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.cache = self.root / 'cache with spaces'
        arch = os.environ.get('PROCESSOR_ARCHITEW6432', os.environ.get('PROCESSOR_ARCHITECTURE', '')).lower()
        self.asset = f'shimo-net_windows_{"arm64" if arch == "arm64" else "amd64"}.exe'
        self.binary = self.cache / VERSION / self.asset
        self.launcher = self.root / 'launcher.ps1'
        self.launcher.write_bytes(runtime.render_launchers(VERSION, digests(self.payload))['scripts/shimo-net.ps1'])
        self.env = {**os.environ, 'SHIMO_NET_RUNTIME_DIR': str(self.cache)}

    @staticmethod
    def quote(value):
        return "'" + str(value).replace("'", "''") + "'"

    def run_launcher(self, prelude=''):
        command = prelude + '\n$forward = @(' + ','.join(self.quote(a) for a in ARGS) + ')\n'
        command += '& ' + self.quote(self.launcher) + ' @forward\nexit $LASTEXITCODE'
        return subprocess.run([self.powershell, '-NoProfile', '-NonInteractive', '-ExecutionPolicy',
                               'Bypass', '-Command', command], env=self.env, capture_output=True)

    def cached(self, payload):
        self.binary.parent.mkdir(parents=True, exist_ok=True)
        self.binary.write_bytes(payload)

    def fake_download(self, payload):
        source = self.root / 'download.exe'
        source.write_bytes(payload)
        log = self.root / 'url.txt'
        return ('function global:Invoke-WebRequest { param($Uri, $OutFile, [switch]$UseBasicParsing, $TimeoutSec)\n'
                "if ($TimeoutSec -ne 180) { throw 'Expected bounded 180 second download timeout' }\n"
                f'[IO.File]::WriteAllText({self.quote(log)}, $Uri); '
                f'[IO.File]::Copy({self.quote(source)}, $OutFile)\n' + '}\n')

    def test_cached_native_arguments_and_exit_code(self):
        self.cached(self.payload)
        result = self.run_launcher()
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertEqual(json.loads(result.stdout), ARGS)

    def test_cached_mismatch_never_executes(self):
        self.cached(self.payload)
        self.binary.write_bytes(self.payload + b'changed')
        result = self.run_launcher()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotEqual(result.returncode, 23)
        self.assertIn(b'SHA256 mismatch', result.stderr)
        self.assertEqual(result.stdout, b'')

    def test_downloaded_mismatch_never_executes_or_enters_cache(self):
        result = self.run_launcher(self.fake_download(self.payload + b'changed'))
        self.assertNotEqual(result.returncode, 0)
        self.assertNotEqual(result.returncode, 23)
        self.assertIn(b'SHA256 mismatch', result.stderr)
        self.assertFalse(self.binary.exists())
        self.assertEqual(list(self.binary.parent.iterdir()), [])

    def test_download_pinned_url_and_native_execution(self):
        result = self.run_launcher(self.fake_download(self.payload))
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertEqual(json.loads(result.stdout), ARGS)
        self.assertEqual((self.root / 'url.txt').read_text(),
                         f'https://github.com/shimo-office/office-suites/releases/download/v{VERSION}/{self.asset}')
        self.assertEqual(self.binary.read_bytes(), self.payload)

    def test_cached_execution_with_inherited_non_windows_module_path(self):
        self.env['PSModulePath'] = str(self.root / 'unrelated-pwsh-modules')
        self.cached(self.payload)
        result = self.run_launcher()
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertEqual(json.loads(result.stdout), ARGS)

    def test_download_execution_with_inherited_non_windows_module_path(self):
        self.env['PSModulePath'] = str(self.root / 'unrelated-pwsh-modules')
        result = self.run_launcher(self.fake_download(self.payload))
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertEqual(json.loads(result.stdout), ARGS)
        self.assertEqual(self.binary.read_bytes(), self.payload)

    def test_missing_hash_prerequisite_is_clear(self):
        prelude = ('function global:Get-Command { param($Name, $ErrorAction)\n'
                   "if ($Name -ne 'Get-FileHash') { Microsoft.PowerShell.Core\\Get-Command $Name }\n}\n")
        result = self.run_launcher(prelude)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b'Get-FileHash', result.stderr)
        self.assertIn(b'SHA256', result.stderr)
        self.assertEqual(result.stdout, b'')


if __name__ == '__main__':
    unittest.main()
