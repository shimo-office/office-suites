# Use Shimo with an AI assistant

[简体中文](../../agent/README.md) · **English** · [Documentation](../README.md)

Tell your assistant what you want to do. It installs the CLI, connects the Skill, checks your account and runs the task. You complete browser login and authorization when needed. The assistant must have internet access and local command execution; a cloud session cannot directly use programs or sessions on your computer.

![Four steps: open a local assistant, request setup, log in when prompted and start working](../../images/readme/agent-steps-en.svg)

*Workflow illustration, not a client screenshot. The four steps below contain the same instructions in text.*

## Step 1: Open your assistant

| Assistant | Where to start | Integration scope |
|---|---|---|
| [WorkBuddy](workbuddy.md) | Start a local desktop task and select a working folder | Depends on the client; project Skills apply only to that project |
| [Codex](codex.md) | Open a task with local command execution and choose a working folder | The agent checks the user or project Skills actually loaded |
| [DeepSeek Harness](dsh.md) | Start a local session and select a working folder | Uses the configured Skill directory |
| [Doubao Work](doubao.md) | Choose “Work” (工作), then “Local computer” (本地电脑) | Reads the Skill in the current task; persistence across sessions is not assumed |

## Step 2: Send this setup request

```text
Please install and configure shimo-net using this guide:
https://github.com/shimo-office/office-suites/blob/main/docs/agent/setup.md
Connect the Skill to this assistant, reuse any working installation,
and verify connectivity so I can work with Shimo in natural language.
```

For private deployments, add your company's Shimo URL. The agent follows the setup guide, maintained in Chinese, and handles installation and Skill integration. You do not need to locate directories, copy files or run commands yourself.

## Step 3: Log in or authorize when prompted

If your account already works, continue. Otherwise, complete login on the Shimo website and return to the conversation. Never send passwords, cookies or verification codes to the assistant. Confirm any required personal authorization in the client.

Setup is complete when **the CLI runs, the Skill is connected, and the account can access the target service**. The agent should report each result and explain which projects or sessions have the Skill. Downloading files alone does not establish readiness.

## Step 4: Start your first task

Begin with search and reading:

```text
Find “Quarterly report” in Shimo and list the file names and links.
After I choose a file, summarize its main points and action items.
```

Expect real files and a summary based on their content. The agent should explain missing files, access errors or incomplete reads. Then ask to download the selected file or provide a folder URL and request a new weekly report. See [workflows](../scenarios.md) for more tasks.

## Next time and troubleshooting

In the same environment, send your task directly. After changing projects, computers or assistants, ask the agent to check integration scope and restore anything missing.

| Situation | What to say |
|---|---|
| Command missing or download failed | “Check the installation, command path and download error; fix them and continue.” |
| Session expired | “Help me log in again. If the previous write may have succeeded, check its result before continuing.” |
| Different account needed | “Start a fresh login for another account; do not reuse the current account.” |
| Skill not loaded | “Check the Skill location and scope, finish integration and continue.” |
| File inaccessible | “Check the link and service URL, then tell me what permission is missing.” |

Permission changes, file locking and permanent deletion require your manual action on the Shimo website. See the [security policy (Chinese)](../../../SECURITY.md). The agent's implementation details are in the [setup instructions (Chinese)](../../agent/setup.md); terminal users can follow the [installation guide](../install.md).
