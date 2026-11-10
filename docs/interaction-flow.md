# 与 interaction-flow.html 的对照

> 规格：`interaction-flow.html`（15 个场景 + 初始化 + 401 重登）
> 实现：`cli/bin/shimo.py` 0.7.0

## 初始化

| 规格 | 实现 | 状态 |
|---|---|---|
| 首次输入产品网址 | `shimo-net init` 提示输入；`--origin` 可直接指定 | ✅ |
| `GET /api/v1/users/me` 校验可达 | `init` 请求该接口，200 / 401 / 403 均视为可达 | ✅ |
| Chrome 登录 `{base}/login` | `login` 启动 Chrome 专用窗口 | ✅ |
| 保存 `config.json` + Keychain | `~/.shimo-net-cli/config.json` + Keychain | ✅ |
| 登录过期就地重登、重试本条 | `--relogin` 自动登录并重试；Agent 指令也包含 `login` → 重试 | ✅ |

## 15 个场景

| # | 规格里的 CLI | 实现 | 状态 |
|---|---|---|---|
| 1 | `shimo-net search "关键词" --json` | `search` → `POST /search/files` | ✅ |
| 2 | `shimo-net spaces` / `ls` | `spaces`、`ls` → `GET /files?folder=` | ✅ |
| 3 | `shimo-net info` | `info` → `GET /files/{guid}/ancestors` | ✅ |
| 4 | `shimo-net import --to` + `share` | `import`（upload → import）→ 问权限 → `share` | ✅ |
| 5 | `shimo-net pull --out` | `pull` → 导出 / 下载 + `manifest.json` | ✅ |
| 6 | `shimo-net new --type` | `new` → `POST /files/create/{type}` | ✅ |
| 7 | `shimo-net rename` | `rename` → `PATCH /files/{guid}` | ✅ |
| 8 | `shimo-net delete` / `trash` / `restore` | 同上；彻底删除拒绝并引导网页 | ✅ |
| 9 | `shimo-net audit` | `audit` → 目录 + shareMode 分组 | ✅ |
| 10 | `shimo-net share` 批量 | 循环 `PATCH shareMode`，逐条结果 | ✅ |
| 11 | `shimo-net get --out` + 解析 | `read` / `get` → 导出或下载后本地解析 | ✅ |
| 12 | `shimo-net ls --since` + 读 | `ls --since` + `read`，输出一页纸 | ✅ |
| 13 | `shimo-net get --out` | `get` → `GET /files/{guid}/download` | ✅ |
| 14 | `shimo-net upload` 无 `--to` 列位置 | `upload` 未给位置时列桌面 / 空间 / 文件夹 / @别名 | ✅ |
| 15 | `shimo-net fav add` + `@别名` | `fav` + 所有链接参数解析 `@别名` | ✅ |

## 与规格的差异

- **搜索的「按内容再搜一遍」**：规格里是一句可能的回复；当前 CLI 只做标题/关键词搜索，没有单独的“内容搜索”开关。Agent 可以换关键词重试。
- **登录重试的触发方式**：规格描述 Agent 自动重登；CLI 提供 `--relogin` 显式开关，并在 `SKILL.md` / README 的 Agent 指令里要求“401 → login → 重试”。默认自动弹 Chrome 会干扰脚本，所以没有设为默认。
- **彻底删除**：规格与实现一致，CLI 只进回收站，彻底删除到网页手动操作。

## 验收方式

```bash
make test
shimo-net schema | python3 -m json.tool | head
```

真实接口回归需要登录环境：`shimo-net init` → `shimo-net login` → 按 `cli/scenarios.md` 逐条跑。离线 CI 只验证命令结构、版本、退出码和文档链接。
