# Install and manage shimo-net with npm

[中文](../npm.md) | **English** · [Documentation](README.md)

This page covers npm installation, version management and removal. See the [installation guide](install.md) for platform requirements, sign-in, account switching and private deployments, or the [assistant guide](agent/README.md) to let an assistant configure everything.

## Install

Requires Node.js 18+; a supported LTS version is recommended. Supported platforms are macOS, Linux and Windows, on x64 or ARM64.

```bash
npm install -g shimo-net-cli --registry=https://registry.npmjs.org
shimo-net version
```

The package is [`shimo-net-cli`](https://www.npmjs.com/package/shimo-net-cli); the command is **`shimo-net`**. It bundles the native executables, so installation and first launch do not download the program from GitHub. An npm account is not required. npm does not configure assistant skills or sign in to Shimo; continue with [Shimo sign-in](install.md#sign-in-session-reuse-and-account-switching).

## Update or pin a version

Update to the latest version published on npm:

```bash
npm install -g shimo-net-cli@latest --registry=https://registry.npmjs.org
shimo-net version
```

For a pinned installation, replace `latest` with a published version. For example:

```bash
npm install -g shimo-net-cli@0.9.19 --registry=https://registry.npmjs.org
```

Check the [npm version list](https://www.npmjs.com/package/shimo-net-cli?activeTab=versions) for available releases. GitHub commits do not update an installed binary. If an assistant skill pins a CLI version, keep the executable aligned with that requirement.

## Install in a project

If you cannot install globally or want the dependency limited to one project, run these commands in that project:

```bash
npm install shimo-net-cli --registry=https://registry.npmjs.org
npm exec -- shimo-net version
npm exec -- shimo-net --help
```

Then use `npm exec -- shimo-net <command>`. This adds an npm dependency to the current project. If your assistant uses a separate workspace, it must also configure a durable command entry point using the [skill runtime instructions](../agent/runtime.md) (Chinese).

## Uninstall

For a global installation:

```bash
npm uninstall -g shimo-net-cli
```

For a project installation:

```bash
npm uninstall shimo-net-cli
```

Removing the package does not clear Shimo sign-in data. To remove CLI credentials too, run `shimo-net logout` **before** uninstalling. It does not sign you out of your everyday browser. See [local data and removal](install.md#local-data-and-removal) for storage locations.

## Troubleshooting

| Symptom | What to do |
|---|---|
| `command not found` | Open a new terminal and confirm npm's global executable directory is on PATH. For a project installation, use `npm exec -- shimo-net`. |
| Unsupported Node.js version | Upgrade to a supported LTS version, or use a [native binary](install.md#download-a-native-binary) without Node.js. |
| Download timeout | Check access to `registry.npmjs.org` and retry once connectivity is restored, or install a GitHub Release binary. |
| Old version after an update | Check `command -v shimo-net` on macOS/Linux or `Get-Command shimo-net` in PowerShell; another installation may be taking precedence. |
| `doctor` reports no account | A working installation and a working Shimo session are separate checks. Continue with the [sign-in flow](install.md#sign-in-session-reuse-and-account-switching). |

For commands and parameters, see the [command reference](cli-reference.md).
