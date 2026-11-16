# 内部真实回归结果

测试日期：2026-11-13（Asia/Shanghai）  
测试环境：macOS、当前 Codex 会话、本机 Keychain 登录态、shimo.net 公有云。  
测试二进制：`/tmp/shimo-net-current`，由当前 `cli` 源码构建。

## 结论

- REST/API 能力可用：登录态、@人列表、评论列表、订阅扫描、在线文件导出、默认浏览器唤起均已验证。
- 当前页面没有暴露 `globalThis.sdk`。依赖页面 SDK 的动作仍快速失败并返回网页操作提示；表格内容、单元格样式和合并已改走后端 `/lizard-api`，不再依赖页面 SDK，也不再要求本地生成 XLSX 再导入。
- 无 SDK 的操作优先在 agent 内联浏览器中由 computer-use 完成；没有内联浏览器时使用 `shimo-net open <链接>`，由 CLI 专用浏览器注入 Keychain 会话后操作。
- 文件锁定、限制编辑、高级权限、成员级授权、彻底删除已列为 CLI 禁止项；必须打开石墨网页由人工确认和操作。
- 失败重试已改为遇到明确的“页面未暴露 editor SDK”立即返回，单次约 5 秒完成浏览器启动和诊断，不再等待约 30 秒重试。

## CLI 与 API

| 能力 | 命令 | 结果 | 证据 |
|---|---|---|---|
| 登录态 | `whoami --json` | 通过 | 返回当前账号信息，退出码 0 |
| @人列表 | `phase2 <文档> --action mentions-list --json` | 通过 | 返回 `{ "ok": true, "value": { "mentionAtList": [] } }`，并生成截图 |
| 评论列表 | `phase2 <文档> --action comments-list --json` | 通过 | 返回 `{ "ok": true, "value": { "list": [] } }`，并生成截图 |
| 订阅扫描 | `subscriptions --limit 5 --depth 1 --json` | 通过 | 返回 `count: 0`，无已订阅文件时正常返回空列表 |
| 文件导出 | `get <文档> --out /tmp/shimo-export-test --json` | 通过 | 导出 `md`，文件 2 bytes，成功写入本地 |
| 默认浏览器 | `open <文档> --default --json` | 通过 | 返回 `mode: default`、`launcher: open` |
| 高风险人工项 | `share` / `purge` | 通过拒绝 | CLI 返回人工页面操作提示，不调用权限或不可逆变更 API |

## 页面 SDK 动作

在临时创建的文档 `lvQlJLlo7pUrMyq2` 和表格 `5WaxLdx8n8CLVbvg` 上执行。测试文档、表格、幻灯片已移入回收站清理。

| 动作 | 结果 | 返回 |
|---|---|---|
| `history-show` / `history-hide` | 网页操作 | 页面未暴露 `globalThis.sdk` |
| `save-version` | 网页操作 | 页面未暴露 `globalThis.sdk` |
| `comments-show` / `comments-hide` | 网页操作 | 页面未暴露 `globalThis.sdk` |
| `find` / `replace-all` | 网页操作 | 页面未暴露 `globalThis.sdk` |
| `page-width` | 网页操作 | 页面未暴露 `globalThis.sdk` |
| `print` | 网页操作 | 页面未暴露 `globalThis.sdk` |
| `sheet-append` / `sheet-insert` / `sheet-delete` | API 可用 | `PUT /lizard-api/files/{fileGuid}/sheets/values`；`POST/DELETE /lizard-api/files/{fileGuid}/sheets/{sheetName}/rows/{index}` |
| `sheet-freeze` / `sheet-sort` / `sheet-filter` | 网页操作 | 页面未暴露 `globalThis.sdk` |
| `sheet-validation` | 网页操作 | 页面未暴露 `globalThis.sdk` |
| `sheet-format` | API 可用 | `POST /lizard-api/files/{fileGuid}/sheets/attrs`；支持 `range`、背景色、字体色、粗体、斜体、下划线和边框字段 |
| `sheet-merge` | API 可用 | `POST /lizard-api/files/{fileGuid}/sheets/mergeCells`；支持工作表、起始单元格、行跨度和列跨度 |
| `sheet-values` / `sheet-update` | API 可用 | `/lizard-api/files/{fileGuid}/sheets/values` 读写区域内容 |

### 本轮回归补充

已从 `drive-be` 的 OpenAPI 与路由实现核对并封装以下真实接口：

- `POST /lizard-api/files/{fileGuid}/sheets/attrs`
- `POST /lizard-api/files/{fileGuid}/sheets/mergeCells`
- `GET/POST/PUT /lizard-api/files/{fileGuid}/sheets/values`

WorkBuddy 处理表格样式时应直接调用：

```bash
shimo-net phase2 <表格链接> --action sheet-format \
  --payload '{"range":"A1:C1","attrs":{"background":"#D32F2F","color":"#FFFFFF","bold":true}}' --json
```

这样“单元格底色不支持、先本地生成 XLSX 再导入”的旧结论已失效；只有没有对应 API 的页面功能才进入内联浏览器/computer-use。

computer-use 已在真实轻文档页面验证“正文”菜单可打开，并显示“标题、副标题、标题 1–6”等操作入口；后续页面写操作按用户给出的目标、范围和单元格坐标执行。

## 本地检查

```text
go test ./...       通过
go vet ./...        通过
go build ./cli      通过
git diff --check     通过
```
