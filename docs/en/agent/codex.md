# Codex integration notes

[简体中文](../../agent/codex.md) · **English** · [Assistant guide](README.md)

This page covers the Codex entry point and integration scope. Follow the shared [four-step guide](README.md) for setup, login and your first task.

## Open a local task

Open a Codex desktop, IDE or CLI task that can run local commands, and choose a working folder.

## Integration scope

The agent checks the Skill locations and scope actually loaded by the client. Grant required access to system credentials or Skill directories when prompted. Start a new session only if the current one cannot refresh Skills.

## Start working

Send the [shared setup request](README.md#step-2-send-this-setup-request) to your assistant, then log in or authorize when prompted. The agent checks that **the CLI runs, the Skill is connected, and your account works** before starting your task.

Next: [first task](README.md#step-4-start-your-first-task) · [troubleshooting](README.md#next-time-and-troubleshooting) · [workflows](../scenarios.md).
