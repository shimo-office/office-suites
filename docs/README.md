# 文档导航

每种信息只在对应文档维护；旧链接仍可通过迁移提示找到新位置。

| 读者 / 主题 | 文档 |
|---|---|
| 安装、登录、账号切换、环境与 FAQ | [安装说明](install.md) |
| npm 安装、更新、指定版本和卸载 | [npm 指南](npm.md) |
| 参数、JSON、退出码、别名 | [命令参考](cli-reference.md) |
| 办公场景、自然语言话术、浏览器顺序 | [唯一场景清单](scenarios.md) |
| Agent 指令包 | [SKILL.md](agent/SKILL.md) |
| 从配置到第一个结果 | [WorkBuddy](agent/workbuddy.md) · [DSH](agent/dsh.md) · [豆包工作](agent/doubao.md) · [Codex](agent/codex.md) |
| 宿主连通验收 | [Agent 验证记录](agent/validation.md) |
| 权限与凭据边界 | [安全策略](../SECURITY.md) |
| 版本和许可 | [CHANGELOG](../CHANGELOG.md) · [LICENSE](../LICENSE) |

## 扩展编辑

固定编辑动作的参数和逐项验收独立于通用办公场景。以下文档保留原需求编号和历史证据：

- [命令总览](phase3-commands.md)
- [文稿](phase3-document-commands.md)、[幻灯片](phase3-slide-commands.md)、[表单](phase3-form-commands.md)
- [实施与验收计划](phase3-plan.md)、[阶段实测记录](phase3-acceptance.md)
- [复测报告生成与检查](phase3-report/README.md)

## 维护者资料

- [能力矩阵](internal/capability-matrix.md)：场景与能力计数的生成结果。
- [能力目录源](internal/capabilities.json)：修改能力时的唯一计数来源。
- [历史回归记录](internal/test-results.md)：按版本保留真实测试事实。
- [改进清单核查](improvement-checklist-review.md)：逐项调整和检查结果。

- [Issue #1 逐项复核](issue-1-review.md)：运行行为与文档一致性修复。
