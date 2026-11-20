# 使用指南

第一次使用，先选一种方式：

| 你怎么使用石墨 | 从这里开始 |
|---|---|
| 在 AI 助手里说一句话，让它帮你办公 | [四步图文教程](agent/README.md) |
| 自己在终端安装和运行 CLI | [安装说明](install.md) |

## 用 Agent 开始

1. 打开 [WorkBuddy](agent/workbuddy.md)、[Codex](agent/codex.md)、[DSH](agent/dsh.md) 或 [豆包工作](agent/doubao.md)。
2. 复制教程里的安装请求，Agent 自动完成 CLI 和技能接入。
3. 按提示完成必要的登录或授权。
4. 直接用自然语言找文件、读文档、下载或编辑；从[办公话术](scenarios.md#从这些话开始)选一个任务试试。

已经安装过就直接发任务，不用每次从头配置。报错时先让 Agent 检查并修复，参照[常见问题与排错话术](agent/README.md#下次使用与常见问题)。

## 按需查阅

| 你想了解 | 文档 |
|---|---|
| 能做哪些办公任务，哪些需要人工处理 | [场景与支持范围](scenarios.md) |
| npm / curl / 原生程序安装、登录和私有部署 | [安装说明](install.md) |
| npm 更新、指定版本和卸载 | [npm 指南](npm.md) |
| 命令、参数和脚本调用 | [命令参考](cli-reference.md) |
| 新版本变化 | [更新记录](../CHANGELOG.md) |
| 凭据、权限和安全边界 | [安全策略](../SECURITY.md) |
| 软件使用许可 | [LICENSE](../LICENSE) |

## 给 Agent 的执行参考

用户只需发安装请求，以下内容由 Agent 按需读取：

- [自动安装与技能接入](agent/setup.md)
- [技能指令](agent/SKILL.md)
- [平台技能包的 CLI 运行说明](agent/runtime.md)
- [页面操作的浏览器选择](scenarios.md#浏览器优先级)
- 扩展编辑：[命令总览](phase3-commands.md) · [文稿](phase3-document-commands.md) · [幻灯片](phase3-slide-commands.md) · [表单](phase3-form-commands.md)

<details>
<summary>维护与历史验收资料</summary>

以下记录用于追溯实现和验收，不是新用户的安装步骤，也不代表当前环境的操作已验证。

- [Agent 历史连通记录](agent/validation.md)
- [能力矩阵](internal/capability-matrix.md)与[能力目录源](internal/capabilities.json)
- [历史回归记录](internal/test-results.md)
- [扩展编辑实施计划](phase3-plan.md)与[验收记录](phase3-acceptance.md)
- [报告生成与检查](phase3-report/README.md)
- [改进清单核查](improvement-checklist-review.md)
- [Issue #1 复核](issue-1-review.md) · [Issue #3 复核](issue-3-review.md)

</details>
