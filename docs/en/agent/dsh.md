# DeepSeek Harness integration notes

[简体中文](../../agent/dsh.md) · **English** · [Assistant guide](README.md)

This page covers the DeepSeek Harness entry point and integration scope. Follow the shared [four-step guide](README.md) for setup, login and your first task.

## Open a local task

Open DSH, start a local session and choose a working folder.

## Integration scope

The agent installs or reuses the Skill according to the current DSH configuration and checks that it is loaded. Start a new session only if the existing session cannot refresh Skills.

## Start working

Send the [shared setup request](README.md#step-2-send-this-setup-request) to your assistant, then log in or authorize when prompted. The agent checks that **the CLI runs, the Skill is connected, and your account works** before starting your task.

Next: [first task](README.md#step-4-start-your-first-task) · [troubleshooting](README.md#next-time-and-troubleshooting) · [workflows](../scenarios.md).
