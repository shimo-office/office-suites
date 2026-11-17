# Agent 连通与 CLI 能力验证（2026-11-17）

目标：确认四个 Agent 能调用 `shimo-net`，并记录 0.9.4 扩展能力的真实结果。场景对外按当前能力描述，不按内部阶段命名。

| Agent | 本机版本 | 入口 | 结果 | 证据 |
|---|---|---|---|---|
| WorkBuddy | 5.11.3 | 桌面客户端 | ✅ 通过（本地 0.9.4 包） | `images/workbuddy/real-whoami.jpg` |
| DeepSeek Harness | 0.2.1-rc.3 | 桌面客户端 | ✅ 通过 | `images/dsh/real-whoami.jpg` |
| Codex | 0.144.0-alpha.4 | CLI | ⚠️ 默认沙箱拦截 Keychain；加 `--dangerously-bypass-approvals-and-sandbox` 后通过 | `images/codex/real-whoami.jpg` |
| 豆包工作 | 豆包 2.35.8（工作模式） | 桌面客户端 | ✅ 通过 | `images/doubao/real-whoami.jpg` |

## 1. WorkBuddy

- 任务：`用 shimo-net 执行 whoami --json 验证连通；只回复「已连通，账号可用」`
- 结果：12 秒完成，回复「已连通，账号可用」，没有输出账号明细。
- 说明：WorkBuddy 能直接执行本机 `~/.local/bin/shimo-net`，并读取 Keychain；使用官方市场版 shimo-net 技能。
- 截图：`images/workbuddy/real-installed.jpg`（市场技能已安装）、`images/workbuddy/real-whoami.jpg`（执行结果）。

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

- 4/4 完成：WorkBuddy、DeepSeek Harness、豆包工作、Codex（Codex 需放宽沙箱）。WorkBuddy 另完成 0.9.4 的 schema、表格 API、订阅、导出和浏览器兜底回归，详细清单见 [internal-capability-test-results.md](../internal-capability-test-results.md)。
- 共同前提：CLI 已安装到 `~/.local/bin/shimo-net`，已完成 `shimo-net login`，Keychain 有有效会话。

## 当前场景的验收状态

当前场景是**功能范围**，具体结果按 API、页面操作和人工禁用三类记录：

| 状态 | 场景 |
|---|---|
| 真实验证过基础管理命令 | 找文件、看目录、文件出处、导入、拉取、新建、改名、删除/回收站/恢复、权限巡检、读正文、周报（`ls --since 7d`）、附件下载、上传、常用别名 |
| 页面操作 | 历史、文字/段落、评论添加、冻结/排序/筛选、数据验证、打印等由内联浏览器 computer-use 完成，并返回截图 |
| 严格人工 | 文件锁定、限制编辑、高级权限、成员级授权、彻底删除；CLI 拒绝处理 |
| 需用户参数 | 单元格历史必须先说明行号和列号；订阅更新先列文件，再按序号打开 |

结论：0.9.4 的 API 入口和浏览器兜底规则已在 WorkBuddy 本地包回归；页面动作仍以截图作为结果证据，严格人工项不会由 CLI 代办。


### 本轮补充记录（2026-11-11）

- 14 上传：真实上传 `接口验证.docx` 到「我的桌面」，返回 `isCloudFile: true`；验收后已删除到回收站。
- 12 周报：对空间 `loqeMwnXXzTwjBqn` 执行 `shimo-net ls ... --since 7d --json`，返回 2 个最近更新目录。
- 10 批量权限：当前 CLI 拒绝执行 `share`，要求打开石墨网页逐项人工确认和设置。
- 10 指定成员：`zhaojingjing` 对应企业成员 Alice11（id 98725579）；成员级授权必须在石墨网页人工操作。
