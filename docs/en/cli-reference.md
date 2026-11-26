# CLI reference

[中文](../cli-reference.md) | **English** · [Documentation](README.md)

Use this page to look up commands, arguments, and output. For setup and account changes, see [Installation](install.md). For natural-language requests, see [Scenarios](scenarios.md).

This reference follows the repository source and may include commands not yet released. Run `shimo-net schema --json` to check the commands and actions available in your installed version. The public editor command is `editor`; `phase2` remains a compatibility alias with the same implementation and arguments.

[Configuration](#configuration-and-diagnostics) · [Files](#files-and-folders) · [Aliases](#location-aliases) · [Editor](#editor-api-and-page-actions) · [Browser](#opening-a-page) · [Advanced editing](#advanced-editing) · [Errors](#json-exit-codes-and-recovery)

## Conventions

```text
shimo-net <command> [arguments] [--option value] [--json]
```

- Replace `<url>` with a Shimo link you can access. Commands that resolve a target also accept its GUID or an `@alias`.
- `--help` / `-h` displays help without executing the requested command. Unknown options fail with a usage error.
- `--json` can appear before or after the command. Data commands return structured output; `schema` already outputs JSON and `help` outputs text. The CLI's help and diagnostic messages may be in Chinese; command names, option names, and JSON keys remain unchanged.
- `--relogin` permits only read-only commands to retry after the user completes a fresh login. Writes and all `phase3` commands never replay automatically. If verification fails after a write, inspect the current content before deciding how to recover.
- `--payload` takes JSON. Shell examples use macOS/Linux quoting; use the appropriate JSON quoting for your PowerShell version.
- `SHIMO_NET_ORIGIN` temporarily overrides the service address. See the [security policy (Chinese)](../../SECURITY.md) for credential handling. Keep cookies out of arguments, conversations, and logs.

## Configuration and diagnostics

| Command | Arguments and behavior |
|---|---|
| `init [--origin URL] [--login]` | Probes `GET /api/v1/auth/register/is_new` before saving the address. A 401 response proves reachability, not authentication. `--login` starts login afterward. |
| `config show` | Shows the configured service address. |
| `config set URL` | Uses the same probe as `init`; failure preserves the old configuration. Credentials cannot be reused across origins. |
| `config clear` | Clears the address configuration; does not sign out a browser. |
| `login [--origin URL] [--new]` | Ordinary login may reuse a browser session only when no saved credential record exists. `--new` uses a fresh temporary profile for account switching. |
| `logout` | Removes CLI credentials; does not sign out your everyday browser. |
| `session status` | Reports the current session source and status. |
| `session harvest [--no-save]` | Explicitly validates and reuses a browser session. `--no-save` validates without saving. |
| `whoami` | Queries the actual signed-in account. |
| `doctor` | Checks runtime, browser, origin, and account; exits nonzero on failure. |
| `schema` | Returns commands, file types, action categories, scenarios, and capability counts. |
| `update [--check] [--json]` | Updates the current installation; `--check` only checks. npm updates the whole package; native installs validate a GitHub Release before replacing the executable. No downgrade or login/config changes. Older versions need one upgrade using the [installation guide](install.md). |
| `version` | Shows the local binary version; supports `--json`. |
| `help` / `--help` / `-h` | Shows help. |

### Update the CLI

```bash
shimo-net update --check --json  # Query releases without installing
shimo-net update --json          # Update the current installation
shimo-net version --json         # Verify the installed version
```

`update` accepts no positional arguments or version selector. To pin a version, use [npm version management](npm.md#update-or-pin-a-version) or a specific Release executable. npm installations query the official npm registry; native installations query this repository’s latest stable GitHub Release. Equal or newer installed versions exit successfully without installing. Neither checking nor updating requires a Shimo account.

With `--json`, success writes one JSON object to stdout. Failures write error JSON to stderr and exit nonzero.

| Field | Meaning |
|---|---|
| `ok`, `source` | Success flag; the source is `npm` or `github`. |
| `currentVersion`, `latestVersion` | Version at the start of this operation and latest version found. |
| `updateAvailable` | Whether the latest version was newer when checked; may remain `true` after a successful update. |
| `updated`, `checkOnly` | Whether this run completed an update and whether it only checked. `--check` always returns `updated: false`. |
| `path` | npm package directory, or the resolved native executable path. |
| `scope` | npm only: `global` or `project`. |
| `backupPath` | Native updates only, when needed: an old executable that Windows could not yet delete. It can be removed after this process exits. |

Native updates validate download size, SHA-256, and the new executable’s version before replacing the old file. Download or validation failures preserve the old executable. npm follows its own installation behavior; ambiguous installation scopes are refused and require the original installation command. See [Installation](install.md) and the [npm guide](npm.md) for limits and upgrading an older CLI.

## Files and folders

| Command | Arguments and limits |
|---|---|
| `search <keyword> [--limit 20]` | Searches accessible files, folders, and spaces. |
| `spaces [--limit 300]` | Lists spaces the current account belongs to. |
| `ls <folder> [--since 7d] [--limit 200]` | Lists a folder or space, optionally filtered by update time. Results may be limited by the scan cap. |
| `info <url>` | Returns file type, permissions, and accessible paths. |
| `upload <file> [--to location]` | Uploads the original file. Without a destination, lists choices; run again with the selected destination to upload. |
| `import <file> --to <location> [--title name]` | Converts md, doc(x), xls(x), or ppt(x) into an online file. Destination is required; permissions are not changed. |
| `new <name> [--type type] [--to location]` | Creates an online file; defaults to `doc` on `desktop`. Types are listed below. |
| `mkdir <name> [--to parent]` | Creates a folder, on `desktop` by default. Do not use `new --type folder`. |
| `rename <url> <name>` | Renames a file while preserving its GUID. Check each result when processing a batch. |
| `delete <url>` | Moves directly to the recycle bin, without a confirmation prompt. There is no `--yes` option. |
| `trash` | Lists the recycle bin. |
| `restore <guid>` | Restores an item from the recycle bin. |
| `audit <folder> [--limit 200]` | Reads permissions without changing them. Unknown permissions are reported separately; unknown or truncated results cause a nonzero exit. |
| `read <url> [--max-chars 12000]` | Extracts text from the actual export. May fall back from Markdown to DOCX; rejects PDF text parsing and recommends `get`. Long output is truncated. |
| `get <url> [--out directory]` | Downloads attachments unchanged or exports online files. May fall back to PDF; inspect `exportType` and the actual extension. Defaults to `./shimo-pull`; adds a number to avoid overwriting existing files. `--format csv` is unsupported. |
| `pull <folder> [--out directory] [--since 7d] [--limit N]` | Downloads the current level only, with no recursive traversal. Defaults to `./shimo-pull` and no item limit. Subfolders are recorded as skipped; name conflicts get numbered. Checks downloaded sizes and returns nonzero if any item fails, preserving successful downloads and per-item errors. |
| `subscriptions [--limit 200] [--depth 3] [--open index-or-guid] [--default]` | Scans accessible files and sorts discovered subscriptions by update time. `limit` caps files scanned, not results returned. List first, then select. `--open` launches a local browser under the [browser rules](scenarios.md#browser-selection). |

A location can be `desktop`, an accessible space/folder link, or an `@alias`. `upload` preserves the original file; `import` converts a supported format. Uploading a scanned PDF does not make its text editable.

`pull` writes a separate hidden manifest; use the returned `manifest` path. It does not overwrite a downloaded file named `manifest.json` or an older manifest. Subscriptions do not provide a server-side feed of all recent document text.

### Creation types

| `--type` | File type |
|---|---|
| `doc` | Lightweight document (`document`) |
| `docx` | Word-style document (`documentPro`) |
| `sheet` | Spreadsheet (`spreadsheet`) |
| `slide` | Presentation (`presentation`) |
| `table` | Application table (`table`), distinct from a spreadsheet |
| `form` | Form |
| `mindmap` | Mind map |
| `flowchart` | Flowchart |

Creation support does not imply every editing feature is implemented or verified. Check the created file's type, link, location, and permissions.

## Location aliases

Replace the example GUID before running:

```bash
shimo-net fav add project "https://shimo.net/folder/<guid>" --note "Project archive" --json
shimo-net fav list --json
shimo-net fav show project --json
shimo-net ls @project --json
shimo-net fav rm project --json
```

`fav add <alias> <url-or-guid> [--note text]` saves a local location. `list`, `show`, and `rm` list, inspect, and remove aliases. Names support Chinese/English letters, digits, underscores, and hyphens, up to 40 characters. `favorites.json` contains locations and notes, not credentials.

## Editor API and page actions

```text
shimo-net editor <url-or-guid-or-@alias> --action <action>
  [--payload JSON] [--screenshot PATH] [--verify|--no-verify] [--json]
```

### API actions

Use the actual worksheet name in place of `Sheet1`.

| Action | JSON payload | Behavior and verification |
|---|---|---|
| `sheet-values` | `{"range":"Sheet1!A1:C5"}` | Reads populated cells. This alone cannot establish empty-row structure or positions after merging. |
| `sheet-update` | `{"range":"Sheet1!A1","resource":{"values":[["Value"]]}}` | Writes the specified range. `resource.values` is required; top-level `values` is unsupported. |
| `sheet-append` | Same structure as `sheet-update` | Appends rows. Read back records and counts; do not blindly append again after a failure. |
| `sheet-insert` | `{"sheetName":"Sheet1","index":3,"count":1}` | Inserts rows. `index` is passed to the API unchanged; confirm the target index before writing. `count` defaults to 1. |
| `sheet-delete` | Same structure as `sheet-insert` | Deletes rows, not columns or worksheets. Confirm the target and affected range first. |
| `sheet-format` | `{"range":"Sheet1!A1:C1","attrs":{"background":"#D32F2F","color":"#FFFFFF","bold":true}}` | Sets supported cell attributes through the API. Multiple attributes are submitted separately, not atomically. |
| `sheet-merge` | `{"sheetName":"Sheet1","cell":"A1","rowspan":1,"colspan":3}` | Merges the specified cells. Verify both exported structure and the page. |
| `mentions-list` | Uses the target file's default action context | Reads the page signature and list over HTTP without launching a browser. For `/docs/`, returns `document URL#atGuid`. |
| `comments-list` | Uses the target file's default action context | Reads over HTTP without launching a browser. For `/docs/`, returns `document URL#selectionGuid`, deduplicated by location. |

`mentions-list/comments-list` require an existing login and never automatically launch a browser to obtain one; explicit `--relogin` retains the global login behavior. `--verify` and `--keep-open` do not launch a browser for these reads. Text output shows counts and unique links, with explicit empty-list and missing-location messages. `--json` preserves the original `result.ok` and `result.value`, adding `links`, `count`, and `unlocatedCount`. `verified:true` / `verifyWith:"http"` confirms the API read only, not browser navigation.

Deep links are generated only for `/docs/`: mentions use `#atGuid` (not `?mentionAt=`); comments use `#selectionGuid` (already prefixed with `comment-`, not `commentGuid`). Other suites or missing valid location fields produce an explanation and the document URL instead of guessed anchors. Open the link in a new tab or reload so the editor runs its initial navigation.

Ranges use `Worksheet!A1:C5`. Alternatively, provide `sheetName` together with a range lacking the worksheet prefix. The CLI combines them.

`sheet-format` accepts only `background`, `color`, `bold`, `italic`, and `underline`. Font size, font family, alignment, number format, and column width are not supported by this action and produce an error. Document or presentation formatting support does not imply spreadsheet API support.

Spreadsheet API writes do not launch local Chrome by default; they return `verified:false` and a page-verification reminder. Complete verification requires **readback → XLSX export inspection → a real page screenshot**. HTTP 2xx, `applied`, or a values response alone is insufficient.

Use `--verify` for CLI screenshots after selecting an approved browser route. `--no-verify` disables screenshot verification only. `--no-browser` or `SHIMO_NET_NO_BROWSER=1` prevents this entry point from launching a browser; `mentions-list` and `comments-list` support this option. Follow the [browser selection rules](scenarios.md#browser-selection).

### Page actions

| Action | Actual behavior |
|---|---|
| `history-show` / `history-hide` | Shows/hides the history panel; does not read the entire history. |
| `save-version` | Rejected by the CLI; save and verify the version in a persistent page. |
| `comments-show` / `comments-hide` | Shows/hides the comments panel. |
| `comment-add` | Rejected by the CLI; enter, submit, and verify the comment in a persistent page. |
| `print` | Rejected by the CLI; finish printing or generate an actual PDF in a persistent page. |

Only history/comment show/hide actions run through the page SDK. `--verify` enables execution and a screenshot for those actions; results state `scope:panel-visibility-only` and `actionVerified:false`. The temporary browser closes afterward and cannot support later submission. An unloaded editor, unavailable method, or blank screenshot is a failure.

`find`, `replace-all`, `page-width`, `sheet-freeze`, `sheet-sort`, `sheet-filter`, and `sheet-validation` are **not directly executable SDK capabilities**. Their CLI actions instruct you to use the page. Worksheet renaming/hiding, column insertion/deletion, formulas, named ranges, and pivot tables also depend on page capabilities.

## Opening a page

`open <url> [--default]` uses a separate persistent `open-profile`. It does not inject the CLI session or enable a remote debugging port. It can reuse a session that the user signed into within that profile; first use may require login.

`--default` selects the operating system's default browser application. Supported Chromium applications still use the isolated CLI profile; other browsers are opened by the system, with no login guarantee. Reuse is refused if a stale debugging-port marker is detected. Close and inspect the affected browser first; upgrading does not close older debugging browsers automatically.

## Advanced editing

The `phase3` entry point provides fixed actions for Word-style documents, presentations, application tables, and forms. It accepts allowlisted actions and constrained JSON, not arbitrary JavaScript.

Detailed references are currently in Chinese: [Overview](../phase3-commands.md) · [Documents](../phase3-document-commands.md) · [Presentations](../phase3-slide-commands.md) · [Forms](../phase3-form-commands.md).

Spreadsheet actions `sheet-update/append/insert/delete/format/merge` through this entry point require `--read-range`. `--out` stores evidence for each run. `phase3 artifact <local-file>` checks package structure only and reports `actionVerified:false`; it does not prove an online write. See the [acceptance record (Chinese)](../phase3-acceptance.md) for limits and deferred capabilities.

## Actions reserved for the user

The CLI refuses `share`, `purge`, `import --share`, file locking, editing restrictions, advanced permissions, member authorization, and permanent deletion. These require the user to act in the Shimo website. `audit` reads permissions but cannot change them. The [security policy (Chinese)](../../SECURITY.md) maintains the full rules.

Readable permission values are `private`, `enterprise_readonly`, `enterprise_commentable`, `enterprise_editable`, `readonly`, `commentable`, and `editable`. The last three describe public-link permissions; reading these values does not authorize changing access.

## JSON, exit codes, and recovery

| Exit code | Meaning |
|---:|---|
| 0 | The command succeeded. Check `verified`, verification scope, and task-specific assertions before considering the entire task complete. |
| 1 | Command error, invalid arguments, permission denial, or verification failure. With `--json`, stderr contains `{"ok":false,"error":"..."}`. |
| 2 | Unknown command. The error goes to stderr, as one JSON object when `--json` is set. |

Failed `doctor`, `audit`, and `pull` commands emit one JSON result on stderr, including diagnostics, permission groups, or manifest details. They do not first print a success result to stdout.

If import succeeds but renaming fails, the result preserves the real `name`, `guid`, and `url`, with `imported:true` and `renamed:false`. Retry only the failed rename; do not upload again.

Verification may fail after a server has accepted a write. Preserve the command, build identity, response, and evidence; read the actual content before choosing a recovery step. Do not overwrite failed evidence with a later successful result or automatically replay append, delete, publish, or form submission operations.
