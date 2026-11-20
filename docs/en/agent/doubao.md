# Doubao Work integration notes

[简体中文](../../agent/doubao.md) · **English** · [Assistant guide](README.md)

This page covers the Doubao Work entry point and integration scope. Follow the shared [four-step guide](README.md) for setup, login and your first task.

## Open a local task

Open the Doubao desktop app, choose “Work” (工作), then “Local computer” (本地电脑). Cloud tasks cannot directly use your local CLI or login session.

## Integration scope

The agent reads matching Skill instructions in the current local task and calls the CLI. This flow requires no separate Skill directory and does not promise persistence across sessions. The agent rereads the instructions when a new task does not retain them.

## Start working

Send the [shared setup request](README.md#step-2-send-this-setup-request) to your assistant, then log in or authorize when prompted. The agent checks that **the CLI runs, the Skill is connected, and your account works** before starting your task.

Next: [first task](README.md#step-4-start-your-first-task) · [troubleshooting](README.md#next-time-and-troubleshooting) · [workflows](../scenarios.md).
