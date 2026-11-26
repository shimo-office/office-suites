# Scenarios and example requests

[中文](../scenarios.md) | **English** · [Documentation](README.md)

Tell your configured assistant what you want to do. It selects the appropriate CLI command or page operation and checks the result. Start with the [assistant guide](agent/README.md) if you have not connected Shimo yet.

[Try a request](#start-with-these-requests) · [Files and folders](#files-and-folders) · [Editing](#editing-and-collaboration) · [Browser rules](#browser-selection) · [Advanced editing](#advanced-editing-scope)

## Start with these requests

| Your task | Example request | Expected result |
|---|---|---|
| Find a file | “Find the pricing policy and list names and links. Let me choose if several files match.” | A list of real, accessible files. |
| Read a document | “Summarize this document's key points and action items, with a link to the source.” | A summary based on its text, with any reading limits stated. |
| Download a file | “Download this file into the current working directory.” | A local file and its saved location. |
| Import a proposal | “Import proposal.md as an online Shimo document in this folder.” | A new document link and a check of its content. |
| Format a spreadsheet | “Make Sheet1!A1:C1 blue with white text, then verify the result.” | The requested range updated and checked. |
| Create a weekly report | “Create a Word-style document named Weekly report in this folder.” | The new document's name, location, and link. |

Include the actual file or folder link. For editing, specify the worksheet and cell range, paragraph, or object. Replace `Sheet1` with the file's real worksheet name; worksheet names are not automatically translated. If the target is ambiguous, the assistant should ask you to choose.

The tables below define support and verification. **API** means a direct service call. **Page** requires an available browser. **User only** means the user must complete the action on the website; the assistant cannot perform it on their behalf.

## Safety boundaries

File locking, editing restrictions, advanced permissions, member authorization, and permanent deletion are user-only operations. The CLI also refuses other permission changes, including revoking access. Import only creates the file; confirm any later permission intent and let the user apply it on the website.

`delete` moves an item directly to the recycle bin without prompting. `restore` recovers it. See the [security policy (Chinese)](../../SECURITY.md) for the full rules.

## Files and folders

| # | Scenario and request | Route and command | What to verify |
|---:|---|---|---|
| 1 | Search: “Find the pricing policy.” | API: `search <keyword> [--limit N]` | Real matching files and links. Resolve duplicate names by path; do not invent body-text matches. |
| 2 | Browse: “Which spaces am I in? What is in this folder?” | API: `spaces`, `ls <url-or-@alias>` | Target and item counts; disclose results limited by the scan cap. |
| 3 | Locate: “Which space contains this file?” | API: `info <url>` | File type, permissions, and accessible path; report access failures. |
| 4 | Import: “Import proposal.md into the project folder.” | API: `import <file> --to <location>`; permissions: user only | Link, online file type, and content after conversion. Permission changes remain a user action. |
| 5 | Archive: “Download the files in this project folder.” | API: `pull <folder> [--out directory] [--since 7d] [--limit N]` | Current level only; no recursion. Manifest marks subfolders as skipped and lists failures. Any failed item causes a nonzero exit. |
| 6 | Create: “Create a weekly report document and an archive folder.” | API: `new <name> --type <type> [--to location]`, `mkdir <name> [--to parent]` | `new` creates files and `mkdir` creates folders. `doc` is lightweight; `docx` is Word-style. See [creation types](cli-reference.md#creation-types) for all eight types. Check each item's type and destination. |
| 7 | Rename: “Rename this draft to Proposal v2.” | API: `rename <url> <name>` | New name with the same GUID; record individual failures in batches. |
| 8 | Recycle and restore: “Move this test file to the recycle bin, then restore it.” | API: `delete`, `trash`, `restore`; permanent deletion: user only | Restored GUID and location; permanent deletion is unavailable through the CLI. |
| 9 | Audit permissions: “Find public links in this space.” | API: `audit <folder> [--limit N]` | Read-only results. Unknown permissions and read failures have their own groups, not “private”; unknown/truncated results exit nonzero. |
| 10 | Change permissions: “Make these files readable within the company.” | User only; CLI refuses permission writes | The CLI may run `audit`; the user then checks and changes access on the website. |
| 11 | Read: “Read this document and give me three key points.” | API: `read <url> [--max-chars N]` | Summarize exported text and link to the source. Disclose truncation, scanned content, and parsing failures. |
| 12 | Weekly summary: “Summarize project documents updated this week.” | API: `ls <folder> --since 7d`, then `read` each file | Use actual updates and content. Distinguish evidence from inference; titles alone do not establish content. |
| 13 | Download: “Download this original attachment.” | API: `get <url> [--out directory]` | Attachments stay unchanged; online files are exported. Check the local file exists and is readable. |
| 14 | Upload: “Upload report.pdf to the project folder.” | API: `upload <file> --to <location>` | An original cloud file. If no destination is given, show choices before uploading. |
| 15 | Save a location: “Use @project for this folder from now on.” | Local configuration: `fav add/list/show/rm` | Store only the alias, link, and note. Verify the target with `ls @project`; do not store credentials in aliases. |

## Editing and collaboration

Payloads, readback, and screenshot requirements for `editor` actions are maintained in the [CLI reference](cli-reference.md#editor-api-and-page-actions). Page operations follow the browser rules below.

| # | Scenario and request | Route and entry point | What to verify |
|---:|---|---|---|
| 16 | Updates: “Who mentioned me? Which files am I subscribed to?” | API: `editor --action mentions-list/comments-list`, `subscriptions`; returns navigation links | List real data and let the user select a file. There is no all-files feed of recent document text. Mentions/comments read directly over HTTP without launching a browser; `/docs/` results include navigation links. |
| 17 | History: “Open history and save a version before delivery.” | Page: `history-show/history-hide`; persistent page for saving | History display requires `--verify` and proves only panel visibility. CLI `save-version` is refused. Specify worksheet, row, and column for cell history. |
| 18 | Text: “Find the old title, replace it, and format the body.” | Page: menus and selections | Generic `find/replace-all` are not exposed SDK methods. Use page operations or supported fixed document actions. |
| 19 | Tables and pictures: “Insert a table and adjust text wrapping.” | Page: the target editor's menus, objects, and selections | Check each object's structure and position; lightweight and Word-style document capabilities differ. |
| 20 | Comments: “Read the comments and comment on this paragraph.” | API: `comments-list` returns navigation links; persistent page for adding comments and mentions | CLI `comment-add` is refused. Submit and verify in a persistent page; notification recipients require user authorization. |
| 21 | Preview and print: “Preview the document and export a printable file.” | Persistent page: preview/print menus | CLI `print` is refused. If a file is requested, verify that an actual PDF was produced. |
| 22 | Document layout: “Adjust heading levels and page width.” | Page: styles, selections, and page settings | Generic `page-width` has no exposed SDK method. Verify the page and exported content. |
| 23 | Spreadsheet data: “Write this week's data, append rows, then insert or delete the specified rows.” | API: `sheet-update/sheet-append/sheet-insert/sheet-delete` | Row operations use the API; columns and worksheet renaming/hiding use the page. Check readback, export, and screenshot; values alone cannot prove empty-row structure. |
| 24 | Spreadsheet organization: “Freeze the first row, filter by amount, and make the header blue.” | API: `sheet-format` with `attrs.background`; page: freezing, sorting, filtering, conditional formatting, views | Use the API for supported formatting. Generic freeze/sort/filter CLI actions refuse unexposed SDK operations. |
| 25 | Data cleanup: “Remove duplicates, split columns, and check file size.” | Page: data and file tools | Compare ranges and retained records before and after. Do not claim an API operation where no stable interface exists. |
| 26 | Analysis: “Check formulas and create a pivot table.” | Page: formulas, named ranges, pivot tables, and analysis menus | Read formulas and results; check references and filters. Do not substitute demonstration data for actual results. |
| 27 | Validation and formatting: “Set input rules, change the fill, and merge the header.” | API: `sheet-format`, `sheet-merge`; page: data validation, row height, column width | Format only supported attributes. Data validation has no exposed SDK action. Verify each subtask separately. |
| 28 | Spreadsheet output: “Download this spreadsheet and print it.” | API: `get` for XLSX export; page: printing | `get --format csv` is unsupported. XLSX failure may fall back to PDF; inspect `exportType`. A requested print file must exist and be readable. |

## Browser selection

This translates the [canonical browser rules](../scenarios.md#浏览器优先级). **Choose display and execution separately: a built-in preview does not need editing controls.**

| When | What to show |
|---|---|
| A file link is provided, or before page work | Open the original file in the built-in browser or preview so the user can see the target |
| Creation or import succeeds | Show the real file URL returned by the service; do not invent a URL or import another copy for display |
| Editing or organizing finishes | Show the same file's result link; reuse an existing preview instead of opening duplicate tabs |

1. **Display the link first.** Check the host's actual link-opening or preview tools. Show the original URL when supported; a PDF export, CLI installation or CLI login is not a prerequisite. Preview-only tools are useful for display. If the URL is unsupported, return a clickable link and continue choosing an execution path without repeatedly opening it. A login page, blank page or stale preview does not verify file contents or results.
2. **Choose how to operate.** Use the CLI for supported API actions. For page work, prefer built-in browser tools with the required reading, clicking and screenshot capabilities. If the built-in surface only previews, use available Computer Use / Browser Use tools to control an external browser, preferably reusing its session and tab. Check the current tools instead of inferring capabilities from the WorkBuddy name. Do not require installation of `agent-browser`; it is one possible external tool. If no browser can be controlled, explain the limitation and leave the link for the user.
3. **Open external pages only as needed.** Reuse existing authorization; explain and obtain consent if launching an external browser is not yet authorized. Prefer the selected control tool's own open or reuse operation. `shimo-net open <url>` only opens a page: it uses an independent open-profile, does not inject the CLI session and leaves no remote debugging port. It does not grant control to the agent. `--default` selects the default browser app; supported Chromium apps still use an isolated profile, while others are opened by the system. Existing login is not guaranteed.
4. **Verify through the execution path.** After a write, wait for saving, read back and inspect the page, then show the result in the built-in preview. That preview may not update automatically. Opening it does not prove saving succeeded; do not repeatedly navigate or reopen windows just to update the display. An unavailable preview does not block authorized external work. Return the original file link and verification result.

**Keep the operating session throughout the task when possible.** Do not close the browser, stop its daemon or create a new profile after each step. For slow loading, perform spaced, read-only checks in the same page within a total time limit. On timeout, report the stage and original error without automatically refreshing, reopening or restarting the task. Read back writes that may have succeeded instead of replaying creation, deletion or submission. If a tool recycles idle sessions, inspect its actual documentation and session state; use only supported keepalive or session settings. Do not invent timeout flags or repeatedly relaunch closed windows. Slow loading or missing login does not mean the tool is unavailable.

**The CLI cannot manage another tool's browser lifecycle.** Page-based `phase3` actions, `editor --verify`, `open`, `login`, comment/mention reads and session harvesting can launch a local browser. They cannot take over host previews, built-in tabs or Computer Use sessions. Some fixed actions open a separate window to verify persistence; use them only when choosing the external CLI path. A flashing window alone does not identify a CLI retry; check the actual calls and errors.

- For supporting API calls before a CLI browser path is selected, pass `--no-browser` on every call, including `doctor` and `whoami`, or set `SHIMO_NET_NO_BROWSER=1` in each task shell. This blocks CLI launches and session harvesting without disabling host tools. It does not hand page commands to another browser. Use the selected operation tools when a command is rejected; do not simply remove the guard and retry.
- The preview, external browser and CLI may have separate login sessions. Let the user sign in on the selected page when needed. Do not request, manually inject or transfer cookies, or clear the user's browser profile as a troubleshooting step. Missing CLI credentials do not make the page unavailable.
- Do not request existing browser authorization again or bypass a refusal by changing tools. Browser availability does not authorize operations reserved for the user.

## Login and sessions

These checks apply to the CLI/API path; pure page tasks use the selected browser session. Until a CLI browser path is selected, use `whoami --no-browser --json` and `session status --no-browser --json`. CLI login and session harvesting require an authorized CLI browser login path. Ordinary login may reuse an existing browser account. To switch accounts, use `login --new`, then verify with `whoami`. The complete workflow and authentication troubleshooting are in [Installation](install.md).

## Describing a precise task

Include **the file link + target location or range + expected result**. For example: “In this sales spreadsheet, make Sheet1!A1:C1 blue with white text, then verify the export and page screenshot.” See the [request examples](#start-with-these-requests) for other starting points.

## Advanced editing scope

Scenarios 29–42 cover fixed actions for Word-style documents, presentations, application tables, and forms. Their [command reference](../phase3-commands.md), [acceptance record](../phase3-acceptance.md), and [subsequent fixes](../issue-3-review.md) are currently in Chinese.

Consult those records for supported scope and deferred items. Historical acceptance does not verify the current operation, and these advanced actions do not increase the general capability count.
