# P0 Agent 浏览器怎么开

四款 P0 Agent 的浏览器能力不同。技能按宿主选择打开方式，不要统一假设都有内置浏览器。

| Agent | 内置浏览器 | 默认打开方式 | 回退 |
|---|---|---|---|
| WorkBuddy | **有**：右上角「打开浏览器」的右侧面板 | `present_files` 把 URL 作为 `files` 项 | 用户同意后用 `agent-browser` 或 `shimo-net open` |
| 豆包工作 | 无 Agent 可直接调用的内置浏览器 | `shimo-net open <url>`（本机浏览器，已注入登录态） | 豆包浏览器 / 默认浏览器 |
| DeepSeek Harness | 无 | `computer-use`（Orca）操作浏览器，或 `shimo-net open` | 本机 Chrome |
| Codex | 无 | `shimo-net open <url>` | `agent-browser` / 默认浏览器 |

## WorkBuddy

WorkBuddy 的内置浏览器入口是 `present_files`，不是 `agent-browser`：

```json
{"files":["https://shimo.net/sheets/<guid>"],"explanation":"打开表格检查写入结果"}
```

- `http/https` URL 会由 WorkBuddy 在右侧内置浏览器面板打开。
- 不要在 WorkBuddy 里用 `shimo-net open` 或 `phase2 --verify` 启动本地 Chrome。
- `agent-browser` 是本地/无头 Chrome 自动化插件，不是 WorkBuddy 的内置浏览器。

## 豆包工作

豆包工作没有可供 Agent 直接调用的内置浏览器。用：

```bash
shimo-net open https://shimo.net/sheets/<guid>
```

CLI 会用本机浏览器并注入 Keychain 登录态。页面需要点击、格式或菜单操作时，用豆包工作的网页操作/computer-use；没有相应能力时由用户操作。不要声称豆包有内置浏览器。

## DeepSeek Harness

DSH 没有内置浏览器工具。优先用 `computer-use`（Orca）打开和操作浏览器；Orca 不可用时用：

```bash
shimo-net open https://shimo.net/sheets/<guid>
```

`computer-use` 控制的是本机浏览器窗口，不是 DSH 内置浏览器。

## Codex

Codex 没有内置浏览器。用 `shimo-net open <url>` 打开本机浏览器。Codex 默认沙箱可能拦截 Keychain 和浏览器配置；先按 [codex.md](codex.md) 的沙箱说明放宽，或运行 `shimo-net login`。

## 通用规则

1. 有内置浏览器就优先内置：WorkBuddy 用 `present_files`。
2. 没有内置浏览器才用 `shimo-net open`；它会复用本机登录态。
3. `phase2` 的表格 API 写入默认不启动本地 Chrome；需要 CLI 截图才加 `--verify`。
4. 不要在用户没同意时静默弹本地 Chrome；没有内置浏览器时先说明会打开本机浏览器。
5. 其他 Agent 后续按同一套接口补；当前 P0 覆盖 WorkBuddy、豆包工作、DeepSeek Harness、Codex。
