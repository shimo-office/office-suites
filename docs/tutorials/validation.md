# P0 Agent 连通验证（2026-11-10）

目标：四个 P0 Agent 是否都能调用 `shimo-net` 完成一次真实请求，并留下可复核的截图。

| Agent | 本机版本 | 入口 | 结果 | 证据 |
|---|---|---|---|---|
| WorkBuddy | 5.9.3 | 桌面客户端 | ✅ 通过 | `images/workbuddy/real-whoami.jpg` |
| DeepSeek Harness | 0.2.1-rc.3 | 桌面客户端 | ✅ 通过 | `images/dsh/real-whoami.jpg` |
| Codex | 0.144.0-alpha.4 | CLI | ⚠️ 默认沙箱拦截 Keychain；加 `--dangerously-bypass-approvals-and-sandbox` 后通过 | `images/codex/real-whoami.jpg` |
| 豆包工作 | 豆包 2.35.8（工作模式） | 桌面客户端 | ✅ 通过 | `images/doubao/real-whoami.jpg` |

## 1. WorkBuddy

- 任务：`用 shimo-net 执行 whoami --json 验证连通；只回复「已连通，账号可用」`
- 结果：12 秒完成，回复「已连通，账号可用」，没有输出账号明细。
- 说明：WorkBuddy 能直接执行本机 `~/.local/bin/shimo-net`，并读取 Keychain。
- 截图：`images/workbuddy/real-installed.jpg`（技能已安装）、`images/workbuddy/real-whoami.jpg`（执行结果）。

## 2. DeepSeek Harness

- 任务：同上。
- 结果：4 秒完成，回复「已连通，账号可用」。
- 说明：技能目录 `~/.dsh/skills/shimo-net-cli` 已安装；DSH 能直接执行本机 CLI 和 Keychain。
- 截图：`images/common/skill-dirs.jpg`（技能目录）、`images/dsh/real-whoami.jpg`（执行结果）。

## 3. Codex

- 首次用 `codex exec --full-auto`：失败，`shimo-net whoami` 返回「未登录」——Codex 默认沙箱拦截了 Keychain。
- 改用 `codex exec --dangerously-bypass-approvals-and-sandbox`：成功，490ms 返回「已连通，账号可用」。
- 说明：Codex 侧需要配置允许读取 Keychain 的沙箱策略，或由用户显式授权；`--dangerously-bypass` 只用于本次验证，不应写进日常脚本。
- 截图：`images/codex/real-whoami.jpg`（CLI 输出）。

## 4. 豆包工作

- 本机豆包 2.35.8，「工作」模式选择「本地电脑」。
- 任务：打开终端执行 `shimo-net whoami --json`，只回复是否连通。
- 结果：10 秒完成，回复「已连通，账号身份正常」。
- 截图：`images/doubao/real-entry.jpg`（工作入口）、`images/doubao/real-whoami.jpg`（执行结果）。

## 结论

- 4/4 完成：WorkBuddy、DeepSeek Harness、豆包工作、Codex（Codex 需放宽沙箱）。
- 共同前提：CLI 已安装到 `~/.local/bin/shimo-net`，已完成 `shimo-net login`，Keychain 有有效会话。

## 当前场景的验收状态

当前场景是**功能范围**，不等于全部能力都已完成可复核验收。当前真实账号回归的覆盖情况：

| 状态 | 场景 |
|---|---|
| 真实验证过基础管理命令 | 找文件、看目录、文件出处、导入、拉取、新建、改名、删除/回收站/恢复、权限巡检、读正文、周报（`ls --since 7d`）、附件下载、上传、常用别名 |
| 部分验证 / 待补 | 10 批量改权限（两份新文件批量 `share editable` 成功；指定成员 zhaojingjing 的单用户授权 CLI 不支持，需网页/管理 API） |
| 未单独验收的边界 | `open`、`logout`、`--relogin` 的完整交互 |

结论：基础管理命令已有真实账号回归记录；当前能力矩阵中的编辑器与协作场景仍需按具体套件、文件和页面入口补充逐项截图/JSON 记录，不能据此宣称全部能力已验收。


### 本轮补充记录（2026-11-11）

- 14 上传：真实上传 `接口验证.docx` 到「我的桌面」，返回 `isCloudFile: true`；验收后已删除到回收站。
- 12 周报：对空间 `loqeMwnXXzTwjBqn` 执行 `shimo-net ls ... --since 7d --json`，返回 2 个最近更新目录。
- 10 批量权限：新建「（权限验证A）」「（权限验证B）」，执行 `shimo-net share A B editable --json`，两份均返回 `shareMode: editable`；验收后已删除到回收站。
- 10 指定成员：`zhaojingjing` 对应企业成员 Alice11（id 98725579）；CLI 当前 `share` 只支持链接/企业权限档位，不能按成员授权，单用户「可编辑」仍需网页端或管理 API。
