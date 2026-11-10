# P0 四款 × 石墨办公 · 图文教程

从安装、授权、技能接入到 15 个场景的操作结果。每套教程包含：

- 实际系统与入口（WorkBuddy / 豆包工作 / DeepSeek Harness / Codex）
- 安装与首次配置
- `shimo-net login` 授权
- 15 个场景的步骤、命令、预期输出、验收和排错

| 工具 | 形态 / 入口 | 教程 |
|---|---|---|
| WorkBuddy | 桌面客户端 · 左侧栏「专家 · 技能 · 连接器」 | [workbuddy.md](workbuddy.md) |
| 豆包工作 | 桌面客户端 · 技能 | [doubao.md](doubao.md) |
| DeepSeek Harness | 桌面 / Web · `~/.dsh/skills/` | [dsh.md](dsh.md) |
| Codex | 桌面 / CLI / IDE · `~/.codex/skills/` | [codex.md](codex.md) |

## 使用说明

1. 先按第 1 节装好客户端、`shimo-net` 和技能，`shimo-net doctor` 四项全绿。
2. 从 15 个场景里挑一个，照着「步骤 → 命令 → 预期结果 → 验收」做一遍。
3. 出错先看每个场景末尾的「出错怎么办」。

## 截图口径

- 终端步骤图由当前 CLI 版本生成，命令和输出是**脱敏示例**；实际域名、文件名、数量会不同。
- 工具入口以教程中写明的菜单为准；WorkBuddy 5.9.3 / DeepSeek Harness 0.2.1-rc.3 / Codex 0.144.0-alpha.4 为本机核对版本。
- 豆包工作未在本机安装，入口与截图按官方版本描述整理；安装后如菜单有差异，以实际版本为准。

> 教程是给非开发同事用的：目标是从零装到跑通场景 01，不需要读代码。
