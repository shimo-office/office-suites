# WorkBuddy integration notes

[简体中文](../../agent/workbuddy.md) · **English** · [Assistant guide](README.md)

This page covers the WorkBuddy entry point and integration scope. Follow the shared [four-step guide](README.md) for setup, login and your first task.

## Open a local task

Open the WorkBuddy desktop app, start a local task and select your working folder.

## Integration scope

The agent reuses an enabled Skill or integrates it through a supported client mechanism. Project Skills apply only to that project; check again when switching projects. Project installation does not imply availability in every task.

## Preview inside WorkBuddy; operate where tools are available

Prefer showing file links in WorkBuddy's built-in browser or preview panel. **Preview-only is enough for display.** Use built-in controls when they can perform the task; otherwise use available Computer Use / Browser Use tools with an external browser, reusing its session where possible. Check the current tools instead of assuming capabilities from the product name. See the [browser policy](../scenarios.md#browser-selection).

For “make the slide background yellow and delete the last slide,” show the original file first, confirm the slide count and editing scope through the controllable browser, edit, wait for saving and verify. Then reuse the built-in preview to show the same file. After creating or importing a document, show the real URL returned by the service too. If preview is unavailable, return a clickable link. A PDF export is not needed just for display, and a stale preview does not prove saving succeeded.

For slow loading, wait within a time limit in the operating page without repeated refreshes or restarts. If windows still cycle, inspect the agent's calls, whether its tool session exited, and whether the active Skill includes “先选执行通道，再检查 CLI” (choose the execution path before CLI checks). The CLI cannot manage idle recycling by WorkBuddy or other browser tools. Consult the actual tool documentation; do not invent timeout settings or manually transfer cookies. CLI page commands launch their own browser and are not required when using external Computer Use.

Repository updates do not automatically replace installed Skills or instructions already read in a session. Have the agent update and reread the active Skill while preserving user customizations, instead of repeatedly reinstalling the CLI.

## Start working

For a pure page task, show the file first, then choose a controllable browser as described above. For full CLI/API integration, send the [shared setup request](README.md#step-2-send-this-setup-request). The agent checks that **the CLI runs, the Skill is connected, and the CLI account works** before performing the corresponding API task.

Next: [first task](README.md#step-4-start-your-first-task) · [troubleshooting](README.md#next-time-and-troubleshooting) · [workflows](../scenarios.md).
