"""Render dependency-free client launchers pinned to verified release assets."""
import re

ASSETS = frozenset(
    f'shimo-net_{system}_{arch}' + ('.exe' if system == 'windows' else '')
    for system in ['darwin', 'linux', 'windows'] for arch in ['amd64', 'arm64']
)


def render_launchers(version, digests) -> dict[str, bytes]:
    """Accept only a stable release and the complete official SHA256 manifest."""
    if not isinstance(version, str) or not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', version):
        raise ValueError('Runtime version must be stable SemVer without a v prefix')
    if not isinstance(digests, dict) or set(digests) != ASSETS:
        raise ValueError('Runtime SHA256 manifest must contain exactly the six official assets')
    if any(not isinstance(value, str) or not re.fullmatch(r'[0-9a-f]{64}', value)
           for value in digests.values()):
        raise ValueError('Runtime SHA256 values must be 64 lowercase hexadecimal characters')
    posix_hashes = '\n'.join(f"  {name}) expected='{digests[name]}' ;;"
                             for name in sorted(ASSETS) if not name.endswith('.exe'))
    windows_hashes = '\n'.join(f"    '{name}' = '{digests[name]}'"
                               for name in sorted(ASSETS) if name.endswith('.exe'))
    return {
        'scripts/shimo-net.sh': POSIX.replace('@VERSION@', version).replace('@HASHES@', posix_hashes).encode('utf-8'),
        'scripts/shimo-net.ps1': POWERSHELL.replace('@VERSION@', version).replace('@HASHES@', windows_hashes).encode('utf-8'),
    }


POSIX = r'''#!/bin/sh
# Generated from the release manifest. Never execute an unverified runtime.
set -eu
fail() { printf '%s\n' "shimo-net runtime: $*" >&2; exit 1; }
case "$(uname -s)" in
  Darwin) system=darwin ;;
  Linux) system=linux ;;
  *) fail 'Unsupported OS; use macOS, Linux, or the Windows PowerShell launcher.' ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fail 'Unsupported CPU architecture; amd64 and arm64 are supported.' ;;
esac
asset="shimo-net_${system}_${arch}"
case "$asset" in
@HASHES@
  *) fail 'Unsupported runtime asset.' ;;
esac
if command -v sha256sum >/dev/null 2>&1; then
  checksum=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  checksum=shasum
else
  fail 'SHA256 verification requires sha256sum or shasum.'
fi
verify() {
  [ ! -L "$1" ] && [ -f "$1" ] || fail 'Runtime cache must be a regular file, not a symlink.'
  if [ "$checksum" = sha256sum ]; then
    actual=$(sha256sum < "$1") || fail 'SHA256 verification failed.'
  else
    actual=$(shasum -a 256 < "$1") || fail 'SHA256 verification failed.'
  fi
  actual=${actual%% *}
  [ "$actual" = "$expected" ] || fail 'SHA256 mismatch; runtime was not executed. Remove the invalid cached file and retry.'
}
if [ -n "${SHIMO_NET_RUNTIME_DIR:-}" ]; then
  root=$SHIMO_NET_RUNTIME_DIR
elif [ -n "${XDG_DATA_HOME:-}" ]; then
  root="$XDG_DATA_HOME/shimo-net-cli/runtime"
else
  [ -n "${HOME:-}" ] || fail 'Set HOME or SHIMO_NET_RUNTIME_DIR for the runtime cache.'
  root="$HOME/.local/share/shimo-net-cli/runtime"
fi
cache_dir="$root/@VERSION@"
binary="$cache_dir/$asset"
if [ ! -e "$binary" ] && [ ! -L "$binary" ]; then
  command -v curl >/dev/null 2>&1 || fail 'First use requires curl to download the pinned runtime.'
  (umask 077; mkdir -p "$cache_dir") || fail 'Cannot create runtime cache directory.'
  temporary=$(mktemp "$cache_dir/.download.XXXXXX") || fail 'Cannot create temporary runtime file.'
  trap 'rm -f "$temporary"' 0
  trap 'exit 1' 1 2 3 15
  curl --fail --location --silent --show-error --proto '=https' --proto-redir '=https' \
    --connect-timeout 20 --max-time 180 --retry 2 \
    --output "$temporary" "https://github.com/shimo-office/office-suites/releases/download/v@VERSION@/$asset" \
    || fail 'Could not download pinned runtime; retry when the release is accessible.'
  verify "$temporary"
  chmod u+x "$temporary" || fail 'Cannot make verified runtime executable.'
  mv -f "$temporary" "$binary" || fail 'Cannot install verified runtime in cache.'
  trap - 0 1 2 3 15
fi
verify "$binary"
[ -x "$binary" ] || fail 'Verified runtime is not executable; restore its executable permission.'
exec "$binary" "$@"
'''


POWERSHELL = r'''# Generated from the release manifest. Compatible with Windows PowerShell 5.1.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$forwardArguments = @($args)
$temporary = $null
try {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
        throw 'Use the POSIX launcher on macOS or Linux.'
    }
    $architecture = $env:PROCESSOR_ARCHITEW6432
    if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
    switch ($architecture.ToLowerInvariant()) {
        'amd64' { $arch = 'amd64' }
        'arm64' { $arch = 'arm64' }
        default { throw 'Unsupported CPU architecture; amd64 and arm64 are supported.' }
    }
    $asset = "shimo-net_windows_$arch.exe"
    $hashes = @{
@HASHES@
    }
    $expected = $hashes[$asset]
    if (-not (Get-Command Get-FileHash -ErrorAction SilentlyContinue)) {
        throw 'SHA256 verification requires Get-FileHash (PowerShell 5.1 or newer).'
    }
    function Confirm-Runtime([string] $Path) {
        $file = Get-Item -LiteralPath $Path -Force
        if ($file.PSIsContainer -or ($file.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw 'Runtime cache must be a regular file, not a link.'
        }
        $actual = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -ne $expected) {
            throw 'SHA256 mismatch; runtime was not executed. Remove the invalid cached file and retry.'
        }
    }
    if ($env:SHIMO_NET_RUNTIME_DIR) {
        $root = $env:SHIMO_NET_RUNTIME_DIR
    } elseif ($env:LOCALAPPDATA) {
        $root = Join-Path $env:LOCALAPPDATA 'shimo-net-cli\runtime'
    } else {
        throw 'Set LOCALAPPDATA or SHIMO_NET_RUNTIME_DIR for the runtime cache.'
    }
    $directory = Join-Path $root '@VERSION@'
    $binary = Join-Path $directory $asset
    if (-not (Test-Path -LiteralPath $binary)) {
        if (-not (Get-Command Invoke-WebRequest -ErrorAction SilentlyContinue)) {
            throw 'First use requires Invoke-WebRequest (PowerShell 5.1 or newer).'
        }
        [IO.Directory]::CreateDirectory($directory) | Out-Null
        $temporary = Join-Path $directory ('.download.' + [Guid]::NewGuid().ToString('N'))
        $tls = [Net.ServicePointManager]::SecurityProtocol
        try {
            [Net.ServicePointManager]::SecurityProtocol = $tls -bor [Net.SecurityProtocolType]::Tls12
            Invoke-WebRequest -UseBasicParsing -TimeoutSec 180 -Uri "https://github.com/shimo-office/office-suites/releases/download/v@VERSION@/$asset" -OutFile $temporary
        } finally {
            [Net.ServicePointManager]::SecurityProtocol = $tls
        }
        Confirm-Runtime $temporary
        try {
            [IO.File]::Move($temporary, $binary)
            $temporary = $null
        } catch {
            # A concurrent first run may have installed the same runtime; verify it below.
            if (-not (Test-Path -LiteralPath $binary)) { throw }
        }
    }
    Confirm-Runtime $binary
    # Windows PowerShell's native invocation loses empty arguments. Quote for CommandLineToArgvW/CRT.
    $quotedArguments = foreach ($argument in $forwardArguments) {
        $escaped = [regex]::Replace([string]$argument, '(\\*)"', '$1$1\"')
        $escaped = [regex]::Replace($escaped, '(\\+)$', '$1$1')
        '"' + $escaped + '"'
    }
    $start = New-Object System.Diagnostics.ProcessStartInfo
    $start.FileName = $binary
    $start.Arguments = $quotedArguments -join ' '
    $start.UseShellExecute = $false
    $start.WorkingDirectory = (Get-Location).ProviderPath
    $process = [Diagnostics.Process]::Start($start)
    $process.WaitForExit()
    $runtimeExitCode = $process.ExitCode
    $process.Dispose()
} catch {
    [Console]::Error.WriteLine('shimo-net runtime: ' + $_.Exception.Message)
    exit 1
} finally {
    if ($temporary -and (Test-Path -LiteralPath $temporary)) {
        Remove-Item -LiteralPath $temporary -Force
    }
}
exit $runtimeExitCode
'''
