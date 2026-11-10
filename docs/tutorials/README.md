# P0 四款 × 石墨办公 · 图文教程

从打开 Agent、接入 CLI、授权到 15 个场景的操作结果。每套教程包含：

- 实际系统与入口（WorkBuddy / 豆包工作 / DeepSeek Harness / Codex）
- 打开 Agent、安装 shimo-net CLI 与首次配置
- `shimo-net login` 授权
- 15 个场景的步骤、命令、预期输出、验收和排错

| 工具 | 形态 / 入口 | 教程 |
|---|---|---|
| WorkBuddy | 桌面客户端 · 左侧栏「专家 · 技能 · 连接器」 | [workbuddy.md](workbuddy.md) |
| 豆包工作 | 桌面客户端 · 技能 | [doubao.md](doubao.md) |
| DeepSeek Harness | 桌面 / Web · `~/.dsh/skills/` | [dsh.md](dsh.md) |
| Codex | 桌面 / CLI / IDE · `~/.codex/skills/` | [codex.md](codex.md) |

> 四款 Agent 的实测结果见 [validation.md](validation.md)（4/4 通过；Codex 需放宽沙箱）。

## 一句话让 Agent 自己配置

每篇教程开头都有一段可直接复制给 Agent 的 prompt；Agent 会运行 `bootstrap.sh`，下载 CLI、写入技能目录、打开登录页。手工步骤只作为兜底。

## 客户独立验收路径

1. 打开已安装的 Agent，截图主界面。
2. 在终端执行 `shimo-net version`，应看到 `shimo-net 0.9.1`。
3. 执行 `shimo-net whoami --json`，应返回账号；不想看账号就只看退出码 0。
4. 在 Agent 里输入：`用 shimo-net 执行 whoami --json 验证连通；只回复「已连通，账号可用」`，应返回「已连通」。
5. 再输入：`用 shimo-net info <一个你有权限的链接> 返回名称、类型、链接`，应返回三项。
6. 把每一步截图，按 [validation.md](validation.md) 的表格记录版本、入口、结果。

## 使用说明

1. 先打开已安装的 Agent，装好 `shimo-net` 和技能，`shimo-net doctor` 的 `checks` 全为 `OK`，最后一行 `结果：🟢 doctor: OK`。
2. 从 15 个场景里挑一个，照着「步骤 → 命令 → 预期结果 → 验收」做一遍。
3. 出错先看每个场景末尾的「出错怎么办」。

## 说明

- 截图来自本机实际界面，账号信息已脱敏。
- Codex 默认沙箱会拦截 Keychain，需要配置允许访问 Keychain 的沙箱策略；教程中的 bypass 只用于验证。
- 豆包工作需选择「本地电脑」，云电脑没有本机 CLI。

> 教程面向 shimo.net 客户及私有化部署客户：目标是从零装到跑通场景 01，不需要读代码。
