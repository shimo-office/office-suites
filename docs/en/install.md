# Installation, sign-in and environment

[中文](../install.md) | **English** · [Documentation](README.md)

This guide covers installing the CLI, choosing a Shimo service, signing in and troubleshooting. To work through an AI assistant, start with the [assistant guide](agent/README.md); your assistant handles configuration, and you complete any required sign-in or authorization.

| What you need | Start here |
|---|---|
| Install the command-line tool | [Choose an installation method](#installation) |
| Connect to your company's deployment | [Configure the service address](#initialization-and-private-deployments) |
| Sign in or switch accounts | [Account and session management](#sign-in-session-reuse-and-account-switching) |
| Resolve an installation or connection issue | [Troubleshooting](#troubleshooting) |

The native program needs neither Python nor Node.js. The npm entry point requires Node.js 18+. All methods provide the same `shimo-net` command; see the [command reference](cli-reference.md) for parameters.

## Requirements

| Component | Requirement |
|---|---|
| Operating system | macOS, Linux or Windows 10/11; x64 or ARM64 |
| Browser | Session reuse supports Chrome, Chromium, Edge, Brave and Vivaldi. Automatic browser discovery for a new sign-in window and ordinary `open` supports Chrome, Chromium and Edge. |
| Build tools | Go 1.26.11+ only when building from source; Release binaries do not require Go |
| Network | Access to shimo.net or your private deployment; npm installation needs the official npm registry, while repository and Release installation need GitHub |

Run `shimo-net doctor --json` to inspect browser discovery, configuration and account status. A nonzero result on a fresh installation may mean you still need to sign in; inspect the reported checks. Brave or Vivaldi can supply an existing session, but a new sign-in window still needs Chrome, Chromium or Edge. Assistants must follow the [browser selection rules](../scenarios.md#浏览器优先级) (Chinese).

## Installation

Choose **one** method. Installing the CLI does not by itself connect an assistant's skill.

| Method | Best for | Result |
|---|---|---|
| [npm](#install-with-npm) | Any supported OS with Node.js 18+ | A `shimo-net` command |
| [curl setup script](#setup-with-curl) | macOS/Linux with guided initialization and sign-in | CLI, service configuration and account check; skill scope is described below |
| [Native binary](#download-a-native-binary) | No Node.js or Go | A standalone executable for your platform |
| [Repository installer](#install-from-the-repository) | An existing checkout or a local build | A source build when Go is available, otherwise a Release binary |

### Install with npm

```bash
npm install -g shimo-net-cli --registry=https://registry.npmjs.org
shimo-net version
```

The package name is `shimo-net-cli`; the command is `shimo-net`. The package bundles the complete program, so running it does not trigger a GitHub download. npm does not install assistant skills. Once the version prints, continue with [sign-in](#sign-in-session-reuse-and-account-switching). See the [npm guide](npm.md) for updates, pinned versions, project installation and removal.

### Setup with curl

On macOS/Linux, download and run the setup script from the official Release. Node.js and Go are not required.

```bash
curl -fL --retry 2 --connect-timeout 20 \
  https://github.com/shimo-office/office-suites/releases/latest/download/bootstrap.sh \
  -o shimo-net-bootstrap.sh &&
bash shimo-net-bootstrap.sh codex
```

Replace `codex` with `workbuddy`, `dsh` or `doubao` as needed. The script downloads the CLI for your platform, initializes the service address and checks the account. If needed, it opens a window for you to sign in.

The script writes skills only to its preset Codex / DSH directories (`~/.codex/skills/shimo-net-cli/` and `~/.dsh/skills/shimo-net-cli/`). It does not install WorkBuddy or Doubao skills. Your assistant must still verify the actual skill directory and loading scope using the [assistant guide](agent/README.md); a successful script does not prove the skill is loaded.

The program is installed at `~/.local/bin/shimo-net`. If that directory is missing from PATH, run:

```bash
export PATH="$HOME/.local/bin:$PATH"
shimo-net version
```

This updates the current shell only; add the directory to your shell's PATH configuration for future sessions. The default `latest` mode downloads the program again, so do not repeatedly run the setup script just to check installation status.

### Download a native binary

Choose the matching file from [GitHub Releases](https://github.com/shimo-office/office-suites/releases/latest):

| System | Intel / AMD x64 | ARM64 / Apple Silicon |
|---|---|---|
| macOS | `shimo-net_darwin_amd64` | `shimo-net_darwin_arm64` |
| Linux | `shimo-net_linux_amd64` | `shimo-net_linux_arm64` |
| Windows | `shimo-net_windows_amd64.exe` | `shimo-net_windows_arm64.exe` |

On macOS/Linux, `uname -m` reports the current terminal architecture: `x86_64` maps to amd64; `arm64` or `aarch64` maps to arm64. On Windows, check **System > About**.

**macOS/Linux:** create `~/.local/bin/` if necessary, rename the download to `shimo-net`, and move it there. Then run:

```bash
chmod u+x "$HOME/.local/bin/shimo-net"
export PATH="$HOME/.local/bin:$PATH"
shimo-net version
```

The PATH change applies to this shell only. Add the directory to your shell configuration for future sessions.

**Windows:** create `%LOCALAPPDATA%\shimo-net\` if necessary, rename the download to `shimo-net.exe`, and move it there. Add that directory to your user PATH, open a new terminal and run `shimo-net version`.

To update, replace the executable in the same location and check its version again. Skill ZIP packages configure AI assistants; they are separate from these native executables.

### Install from the repository

macOS / Linux:

```bash
git clone https://github.com/shimo-office/office-suites.git
cd office-suites
./install.command
```

Windows PowerShell:

```powershell
git clone https://github.com/shimo-office/office-suites.git
cd office-suites
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

The installer builds the current checkout when Go is available; otherwise it downloads the latest platform binary. The destination is `~/.local/bin/shimo-net` on macOS/Linux and `%LOCALAPPDATA%\shimo-net\shimo-net.exe` on Windows. If a DSH directory exists, the installer also copies the shared skill. Other assistants require [skill setup](agent/README.md).

### Assistant setup from a checkout

Start from the [assistant guide](agent/README.md) so your assistant can reuse a working CLI and skill. In an existing checkout on macOS/Linux, the same setup script is available locally:

```bash
bash bootstrap.sh codex  # Or workbuddy, dsh, doubao
```

This is the same flow as [curl setup](#setup-with-curl), not an additional required step.

### Build from source

From the repository root, with Go 1.26.11+ installed. On macOS / Linux:

```bash
go build -trimpath -ldflags "-s -w" -o bin/shimo-net ./cli
export PATH="$PWD/bin:$PATH"
shimo-net version
shimo-net schema --json
```

Windows PowerShell:

```powershell
go build -trimpath -ldflags "-s -w" -o bin/shimo-net.exe ./cli
$env:Path = "$PWD\bin;$env:Path"
shimo-net version
shimo-net schema --json
```

The PATH change is for the current development shell. The legacy `./shimo` wrapper runs `bin/shimo-net` when present and falls back to `go run ./cli` otherwise. Rebuild after changing source; an old binary does not validate new code. Check versions with `shimo-net version`, not directory names.

## Initialization and private deployments

Use the default address for the public cloud, or replace `<your-shimo-domain>` with your company's Shimo domain. Run **one** of these initialization commands:

```bash
shimo-net init --json
shimo-net init --origin "https://<your-shimo-domain>" --json
```

Read the address with `shimo-net config show --json`; change it with `shimo-net config set "https://<your-shimo-domain>" --json`.

`init`, `config set` and `login --origin` probe `GET /api/v1/auth/register/is_new` before saving the address. A JSON 200 response, 401 or 403 is accepted; a failed probe preserves the previous configuration. **A 401 proves the endpoint is reachable, not that you are signed in.** An expired login does not require initialization again.

To override the service for a single command on macOS/Linux:

```bash
SHIMO_NET_ORIGIN="https://<your-shimo-domain>" shimo-net whoami --json
```

In PowerShell, use `$env:SHIMO_NET_ORIGIN = 'https://<your-shimo-domain>'`. Environment variables take precedence over the saved configuration. Check account access again after changing domains; `config clear` clears only the saved service address.

## Sign-in, session reuse and account switching

A working session does not need another login. For a first or expired session, run the following, complete any browser sign-in, and verify the account:

```bash
shimo-net login
shimo-net whoami --json
shimo-net session status --json
```

The CLI automatically attempts browser session reuse only when there is no saved credential record. It does not silently switch accounts when credentials expire, belong to another domain or are corrupt. A 401 requires an explicit login.

To inspect a browser session explicitly:

```bash
shimo-net session harvest --no-save --json  # Validate without saving
shimo-net session harvest --json            # Validate and save
```

To switch accounts, use a fresh sign-in window:

```bash
shimo-net logout
shimo-net login --new
shimo-net whoami --json
```

`logout` removes CLI credentials and the old `chrome-profile`; it does not sign you out of your everyday browser or `open-profile`. Ordinary `login` may therefore reuse the original account. `login --new` skips reuse and uses a temporary profile that is removed afterward. Unset `SHIMO_NET_COOKIE` before signing in or switching accounts, or the CLI refuses the operation to avoid an environment override masking the selected account.

Never send passwords or cookies to an assistant. If a sandbox blocks credential or browser-profile access, grant only the required access; do not disable the entire sandbox or repeatedly initialize the service.

`--relogin` permits a new interactive sign-in and retry only for read commands. It never automatically replays writes or the fixed editing entry point `phase3`. If a write was accepted but verification failed, read the file back and inspect the evidence before taking recovery steps. Do not blindly repeat append, delete, publish or form-submission operations.

## Local data and removal

| Data | Default location |
|---|---|
| Service address, credential metadata and saved locations | macOS/Linux: `~/.shimo-net-cli/`; Windows: `%APPDATA%\shimo-net-cli\` |
| System credentials | macOS Keychain / Windows Credential Manager, with a `shimo-net-cli` prefix and origin-specific isolation |
| Credential fallback | `credentials.json` with mode 0600 when system storage is unavailable |
| CLI browser profiles | `open-profile` for `open`; temporary `login-*` for sign-in; `chrome-profile` is a legacy profile |

See the [security policy](../../SECURITY.md) (Chinese) for credential precedence and cleanup boundaries. To remove local sign-in credentials, run `shimo-net logout` before deleting the binary or [uninstalling the npm package](npm.md#uninstall). Removing the state directory or installed assistant skills is a separate choice. Never commit the state directory or cookies to a repository.

## Troubleshooting

Start with `shimo-net doctor --json`. When working through an assistant, send it the error and ask it to fix the connection and continue the original task.

| Symptom | What to do |
|---|---|
| 401 / not signed in | Inspect `session status --json` and `whoami --json`, then follow the [sign-in flow](#sign-in-session-reuse-and-account-switching). |
| Same account after logout | Your everyday browser is still signed in. Use `login --new`, then check `whoami`. |
| `command not found` | Add the directory for your installation method to PATH and reopen the terminal. See [npm troubleshooting](npm.md#troubleshooting) for npm installations. |
| Browser not found | Install Chrome, Chromium or Edge for your OS and rerun `doctor --json`; session reuse support does not imply new-window discovery support. |
| Go not found | Download a Release binary, or install Go 1.26.11+ to build from source. |
| Login page or blank page in a screenshot | This is not successful verification. Check the target-domain session and editor readiness, then follow the [browser rules](../scenarios.md#浏览器优先级) (Chinese). |
| Upload is not an editable document | `upload` preserves the original file; use `import` to convert supported formats. |
| Permission denied | Ask the owner or administrator to grant access in the Shimo webpage. The CLI does not change permissions. |
| Private deployment is unreachable | Check the address, network, certificates and deployed capabilities. Reachability does not guarantee every editing feature is available. |
| A script needs structured output | Add `--json` to data commands; inspect stderr and the exit code on failure, not just the HTTP status. |
