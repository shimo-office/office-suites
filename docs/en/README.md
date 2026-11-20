# Documentation

[简体中文](../README.md) · **English** · [Project home](../../README.en.md)

Choose a path for your task. You do not need to read every guide in order.

| How you work | Reading path |
|---|---|
| Through an AI assistant | [Four-step guide](agent/README.md) → [Workflows](scenarios.md) |
| In a terminal or script | [Installation and login](install.md) → [CLI reference](cli-reference.md) |
| Integrating an assistant | [Agent setup](../agent/setup.md) → [Skill](../agent/SKILL.md) → [Runtime guide](../agent/runtime.md) — agent instructions in Chinese |

## User guides

| Guide | What it covers |
|---|---|
| [AI assistant guide](agent/README.md) | Setup request, login, first task and troubleshooting prompts |
| [Assistant notes](agent/README.md#step-1-open-your-assistant) | WorkBuddy, Codex, DSH and Doubao Work entry points and scope |
| [Installation](install.md) | Platforms, installation methods, service URL, accounts and troubleshooting |
| [npm guide](npm.md) | Updates, version pinning, project installation and removal |
| [Workflows](scenarios.md) | Task examples, supported scope and browser rules |
| [CLI reference](cli-reference.md) | Parameters, formats, exit codes and recovery |

## Advanced references

The following resources are currently maintained in Chinese:

- **Fixed editing actions:** [overview](../phase3-commands.md), [documents](../phase3-document-commands.md), [slides](../phase3-slide-commands.md) and [forms](../phase3-form-commands.md). Check the file type and verification requirements before choosing an action.
- **Agent execution:** [setup](../agent/setup.md) covers integration; the [Skill](../agent/SKILL.md) defines operating rules; the [runtime guide](../agent/runtime.md) covers packaged runners and caches.
- **Security and licensing:** [security policy](../../SECURITY.md) and [LICENSE](../../LICENSE).
- **Maintenance:** [maintainer index](../internal/README.md), including historical evidence and capability data; [changelog](../../CHANGELOG.md).

## Documentation conventions

Chinese pages keep their existing paths. English user guides live under `docs/en/`, with `README.en.md` as the English project entry. Paired pages link to each other. Command names, flags, JSON fields and product names stay unchanged across languages.

The project README introduces the tool, tutorials explain tasks, reference pages define parameters, and historical reports record past evidence. Link to the main source of a rule instead of copying it into every page. Links to untranslated material are labeled Chinese.

Use repository-relative images with editable sources and useful alternative text. Label diagrams as illustrations, not real execution evidence. After editing, run `python3 scripts/check_docs.py` and inspect both languages at desktop and mobile widths. Keep old entry points as short redirects.
