# Agent 连通验收（2026-11-17）

记录范围：2026-11-17 的四款客户端与本机 CLI 连通结果；这是历史验收记录，不是当前全部功能或新版本的验收结论。

> 注：教程内图片是按当前界面做的 H5 还原示意，账号、文件名、文件链接已脱敏；真实执行结果以本表记录和 [内部回归记录](../internal/test-results.md) 为准。

| Agent | 本机版本 | 入口 | 结果 | 证据 |
|---|---|---|---|---|
| WorkBuddy | 5.11.3 | 桌面客户端 | ✅ 通过（本地最新版包） | `../images/tutorials/workbuddy/real-whoami.jpg` |
| DeepSeek Harness | 0.2.1-rc.3 | 桌面客户端 | ✅ 通过 | `../images/tutorials/dsh/real-whoami.jpg` |
| Codex | 0.144.0-alpha.4 | CLI | ⚠️ 默认沙箱拦截 Keychain；加 `--dangerously-bypass-approvals-and-sandbox` 后通过 | `../images/tutorials/codex/real-whoami.jpg` |
| 豆包工作 | 豆包 2.35.8（工作模式） | 桌面客户端 | ✅ 通过 | `../images/tutorials/doubao/real-whoami.jpg` |

## 1. WorkBuddy

- 任务：`用 shimo-net 执行 whoami --json 验证连通；只回复「已连通，账号可用」`
- 结果：12 秒完成，回复「已连通，账号可用」，没有输出账号明细。
- 说明：WorkBuddy 能直接执行本机 `~/.local/bin/shimo-net`，并读取 Keychain；使用 SkillHub 版 shimo-net-cli 技能。
- 截图：`../images/tutorials/workbuddy/real-installed.jpg`（SkillHub 技能已安装）、`../images/tutorials/workbuddy/real-whoami.jpg`（执行结果）。

## 2. DeepSeek Harness

- 任务：同上。
- 结果：4 秒完成，回复「已连通，账号可用」。
- 说明：技能目录 `~/.dsh/skills/shimo-net-cli` 已安装；DSH 能直接执行本机 CLI 和 Keychain。
- 截图：`../images/tutorials/common/skill-dirs.jpg`（技能目录）、`../images/tutorials/dsh/real-whoami.jpg`（执行结果）。

## 3. Codex

- 首次用 `codex exec --full-auto`：失败，`shimo-net whoami` 返回「未登录」——Codex 默认沙箱拦截了 Keychain 和浏览器配置；最新版会先尝试复用本机 Chrome 登录态。
- 改用 `codex exec --dangerously-bypass-approvals-and-sandbox`：成功，490ms 返回「已连通，账号可用」。
- 说明：Codex 侧需要配置允许读取 Keychain 的沙箱策略，或由用户显式授权；`--dangerously-bypass` 只用于本次验证，不应写进日常脚本。
- 截图：`../images/tutorials/codex/real-whoami.jpg`（CLI 输出）。

## 4. 豆包工作

- 本机豆包 2.35.8，「工作」模式选择「本地电脑」。
- 任务：打开终端执行 `shimo-net whoami --json`，只回复是否连通。
- 结果：10 秒完成，回复「已连通，账号身份正常」。
- 截图：`../images/tutorials/doubao/real-entry.jpg`（工作入口）、`../images/tutorials/doubao/real-whoami.jpg`（执行结果）。

## 结论

- 4/4 完成：WorkBuddy、DeepSeek Harness、豆包工作、Codex（Codex 需放宽沙箱）。功能回归另见 [内部回归记录](../internal/test-results.md)，不混入本页连通统计。
- 共同前提：CLI 已安装到 `~/.local/bin/shimo-net`，已完成 `shimo-net login`，Keychain 有有效会话。


浏览器选择与日常权限处理以[当前场景规则](../scenarios.md#浏览器优先级)和[安装说明](../install.md)为准。历史记录中的绕过沙箱命令只描述当时执行，不是日常安装建议。旧版功能状态与早期补充记录已迁入[内部历史档案](../internal/agent-validation-history.md)。
