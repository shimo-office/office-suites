# 文档导航

**简体中文** · [English](en/README.md) · [项目首页](../README.md)

按你要完成的任务选一条路径，不必从头读完所有文档。

| 使用方式 | 建议阅读顺序 |
|---|---|
| 用 AI 助手办公 | [四步入门](agent/README.md) → [办公场景](scenarios.md) |
| 在终端或脚本中使用 | [安装与登录](install.md) → [命令参考](cli-reference.md) |
| 为助手接入工具 | [自动安装执行说明](agent/setup.md) → [Skill](agent/SKILL.md) → [运行说明](agent/runtime.md) |

<a id="用-agent-开始"></a>
<a id="按需查阅"></a>

## 用户指南

| 文档 | 只在这里查什么 |
|---|---|
| [AI 助手入门](agent/README.md) | 安装请求、登录配合、第一项任务和日常排错话术 |
| [各助手说明](agent/README.md#第-1-步打开你正在用的助手) | WorkBuddy、Codex、DSH、豆包工作的入口和接入范围 |
| [安装指南](install.md) | 平台选择、安装、服务地址、账号、环境与排错 |
| [npm 指南](npm.md) | npm 更新、固定版本、项目内安装和卸载 |
| [办公场景](scenarios.md) | 怎样描述任务、支持范围和浏览器选择规则 |
| [命令参考](cli-reference.md) | 参数、数据格式、退出码和失败恢复 |

<a id="给-agent-的执行参考"></a>

## 进阶编辑与 Agent 参考

- **固定编辑动作：**[总览](phase3-commands.md) · [文稿](phase3-document-commands.md) · [幻灯片](phase3-slide-commands.md) · [表单](phase3-form-commands.md)。先确认文件类型，再查动作与验证要求。
- **Agent 执行：**[自动安装](agent/setup.md)负责接入，[Skill](agent/SKILL.md)负责操作规则，[运行说明](agent/runtime.md)负责运行环境与首次使用。
- **安全与许可：**[安全策略](../SECURITY.md)集中维护权限和凭据边界；[LICENSE](../LICENSE)为使用条款。
- **维护与历史：**[维护导航](internal/README.md)集中收录验收、回归、能力目录与报告；[更新记录](../CHANGELOG.md)记录版本变化。

## 文档维护约定

中文主文档保留原路径；英文首页为 [`README.en.md`](../README.en.md)，英文用户指南放在 [`docs/en/`](en/README.md)。两种语言分页面阅读、顶部互链；命令、选项、JSON 字段和产品名保持一致，不翻译成另一套接口。

首页负责帮助读者选择入口，教程解释任务步骤，参考页维护参数，历史报告记录当时的证据。同一规则只维护一个主要出处，其他页面链接过去。高级执行说明和历史报告尚以中文维护，英文入口会明确标注。

图示使用仓库内相对路径、可编辑源文件与有意义的替代文字；示意图不冒充实际运行截图。修改后运行 `python3 scripts/check_docs.py`，并检查中英文页面及手机宽度的显示效果。旧入口继续保留短跳转，避免破坏已分享的链接。
