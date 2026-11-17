# 内部真实回归结果

## 2026-11-17 0.9.6 一二期真实回归

测试环境：macOS、Google Chrome 157、shimo.net 公有云、WorkBuddy 5.11.3。WorkBuddy 已安装技能 `~/.workbuddy/skills/shimo-net-cli`，二进制与 `SKILL.md` 已同步到 0.9.6；CLI 由本仓库 `cli` 构建。

> 本轮在 DSH 沙箱内执行，沙箱不允许写 `~/.shimo-net-cli` 和 Keychain，所以本地写入统一用 `SHIMO_NET_STATE_DIR` 指到工作区；真实终端不受这个限制。Keychain 读取、Chrome cookie 复用、浏览器注入均为真实调用。

证据：

- 验证文件夹：<https://shimo.net/folder/M12Y8VYXolUbK5Wg>（内含文档、云文件和表格）
- 验证文档：<https://shimo.net/docs/VWwQZXQeO2TrDeq3>
- 验证表格：<https://shimo.net/sheets/mqAD2JDw8pCYyw1E>
- 二期演示视频：<https://shimo.net/folder/rlvQlJLljpmieaq2>（28 个 14 秒 WorkBuddy 高保真 MOV + 说明文档 + 总览图 + 封面 + H5 源包 zip，共 32 项）
- 表格格式截图：[sheet-format.png](images/0.9.6/sheet-format.png)
- 文档页面截图：[doc.png](images/0.9.6/doc.png)
- 文件夹页面截图：[folder.png](images/0.9.6/folder.png)
- WorkBuddy 演示画面：[workbuddy-demo.png](images/0.9.6/workbuddy-demo.png)

### 登录态桥接（本轮修复重点）

| 检查 | 命令/动作 | 结果 |
|---|---|---|
| 无 Keychain、仅 Chrome 已登录 | `shimo-net whoami --json`（临时 state + 临时 Keychain 服务） | 通过。自动复用 Chrome (Default)，返回 `徐依娴 <xuyixian@shimo.im>`，`editionName` 已修复为「企业高级版」 |
| 完整 cookie jar | 注入 `sid` + `smdev-ws-route` + `deviceId` 等 8 个 cookie | 通过。只注入 sid 会 401，完整 jar 才能通过 |
| WorkBuddy 自带二进制 | `~/.workbuddy/skills/shimo-net-cli/bin/shimo-net whoami --json` | 通过，版本 0.9.6 |
| WorkBuddy 无本机 Keychain | 同上 + 临时 state/Keychain 服务 | 通过，stderr 输出「已从 Chrome (Default) 复用登录态，无需二次登录」 |
| 默认浏览器注入 | `shimo-net open <链接> --default --json` | 通过，`sessionInjected: true`，页面直接进入「二期」文件夹，不出现登录页 |
| 专用浏览器注入 | `shimo-net open <链接> --json` | 通过，`sessionInjected: true` |
| 手动检查 | `shimo-net session status --json` / `session harvest --json` | 通过，可看到 Keychain/文件/浏览器来源与账号信息，不输出 cookie |

### 一期 20 项

| # | 能力 | 结果 | 证据/说明 |
|---|---|---|---|
| 1 | 关键词搜索 | 通过 | `search "验证文档" --limit 5 --json` 返回 5 条 |
| 2 | 空间清单 | 通过 | `spaces --limit 5 --json` 返回「产研、Excel 重构项目、产研知识库、周报、石墨大家庭」 |
| 3 | 列目录、按更新时间过滤 | 通过 | `ls M12Y8VYXolUbK5Wg --json`；`ls --since 7d` |
| 4 | 文件信息与所在路径 | 通过 | `info VWwQZXQeO2TrDeq3 --json` 返回完整 ancestors、权限、大小 |
| 5 | 导入并转在线文档 | 通过 | `import 验证文档.md --to M12Y8VYXolUbK5Wg` → `docs/VWwQZXQeO2TrDeq3` |
| 6 | 设置权限（7 档） | 人工页面 | `share` 明确拒绝；按设计必须网页人工确认 |
| 7 | 整个文件夹拉取 | 通过 | `pull M12Y8VYXolUbK5Wg --out /tmp/verify/pull` 下载 2 个文件并生成 `manifest.json` |
| 8 | 新建空白文件 | 通过 | `new 验证表格 --type sheet` → `sheets/mqAD2JDw8pCYyw1E`；`mkdir` 新建「二期」→ `folder/rlvQlJLljpmieaq2` |
| 9 | 改名 | 通过 | `rename VWwQZXQeO2TrDeq3 "验证文档-改名"`，链接不变 |
| 10 | 删除到回收站 | 通过 | `delete VWwQZXQeO2TrDeq3`，提示可 `restore` |
| 11 | 回收站列表 | 通过 | `trash --json` 共 268 项，能按 guid 找到刚删除的文档 |
| 12 | 从回收站恢复 | 通过 | `restore VWwQZXQeO2TrDeq3`，`info` 回到原位置 |
| 13 | 权限巡检 | 通过 | `audit M12Y8VYXolUbK5Wg` 分组 private/public/enterprise |
| 14 | 批量改权限 | 人工页面 | `share` 拒绝；必须人工逐项确认 |
| 15 | 读正文 | 通过 | `read VWwQZXQeO2TrDeq3` 返回 97 字正文 |
| 16 | 导出常见格式 | 通过 | `get VWwQZXQeO2TrDeq3 --out /tmp/verify/export` 导出 md（217 B） |
| 17 | 按时间筛选后汇总 | 通过（汇总可由 Agent 生成） | `ls --since 7d` + `read` 可用；一页纸由 Agent 组织 |
| 18 | 单个文件下载 | 通过 | 云文件 `附件.txt` 46 B 原样下载 |
| 19 | 原样上传 | 通过 | `upload 附件.txt --to M12Y8VYXolUbK5Wg` → `files/r1N7eV7wpPCGbVqo` |
| 20 | 常用位置别名 | 通过 | 在工作区 state 下 `fav add/show/rm verify096` 与 `ls @verify096` 均通过 |

### 二期 30 项

| # | 能力 | 结果 | 证据/说明 |
|---|---|---|---|
| 1 | 谁 @ 了我、与我相关的评论 | API 通过，定位到段落需页面 | `phase2 VWwQZXQeO2TrDeq3 --action mentions-list` → `mentionAtList: []`；`comments-list` → `list: []` |
| 2 | 跨文件汇总待回复 | 部分：可逐文件读评论，扫描需浏览器 | `comments-list` 单文件可用；跨空间扫描未发现服务端全量接口 |
| 3 | 我订阅的更新 | 通过 | `subscriptions --limit 5 --depth 1` 正常返回空列表（当前无订阅） |
| 4 | 保存版本 | 页面操作 | `phase2 --action save-version` 返回「页面未暴露 editor SDK」，需 computer-use |
| 5 | 查看历史 | 页面操作 | 同上，`history-show` 转页面 |
| 6 | 单元格历史 | 页面操作 | 需先给出工作表、行、列 |
| 7 | 字符格式 | 页面操作 | 轻文档字体/颜色/样式菜单 |
| 8 | 段落格式 | 页面操作 | 正文/标题/列表菜单 |
| 9 | 查找替换 | 页面操作 | `find` / `replace-all` 无稳定 API |
| 10 | 文内表格 | 页面操作 | 插入菜单 |
| 11 | 图片处理 | 页面操作 | 图片工具栏 |
| 12 | 添加评论 | 页面操作 | 选中文字后挂评论 |
| 13 | @ 提及与段落链接 | 页面操作 | 评论输入框/链接菜单 |
| 14 | 预览 | 页面操作 | 各套件预览入口 |
| 15 | 打印 | 页面操作 | 系统打印对话框需人工确认 |
| 16 | 设置标题与文档样式 | 页面操作 | 正文/标题/文档样式 |
| 17 | 页面宽度 | 页面操作 | 文档样式 → 页面宽度 |
| 18 | 区域写入、表尾追加 | API 通过 + 截图 | `sheet-update` 写 A1:C2 后 `sheet-values` 读回；`sheet-append` 写入第 3 行；截图核验 |
| 19 | 插删行列、工作表管理 | 行操作 API 通过；工作表管理页面 | `sheet-insert` / `sheet-delete` 返回 204；工作表改名/排序/隐藏需页面 |
| 20 | 冻结与高亮行列 | 页面操作 | 表格冻结/格式菜单 |
| 21 | 条件格式 | 页面操作 | 未确认 API |
| 22 | 套表格样式、网格线、缩放 | 页面操作 | 表格视图菜单 |
| 23 | 删除重复项、分列、填充 | 页面操作 | 数据工具 |
| 24 | 文件瘦身 | 页面操作 | 文件工具 |
| 25 | 公式与定义名称 | 页面操作 | 公式栏/名称管理 |
| 26 | 透视表 | 页面操作 | 未确认 API |
| 27 | 排序、筛选、分组 | 页面操作 | 表格菜单 |
| 28 | 数据验证 | 页面操作 | 数据验证菜单 |
| 29 | 单元格格式、行高列宽 | 格式 API 通过；行高列宽页面 | `sheet-format` 拆分单属性写入后，导出 xlsx 验证 A1:C1 fill `#D32F2F`、font `#FFFFFF`、bold true；`sheet-merge` 204 |
| 30 | 导出与打印 | 导出 API 通过；打印页面 | `get mqAD2JDw8pCYyw1E` 导出 xlsx（3.4 KB）；打印走页面 |

### 本轮修复

- `sheet-format` 的 `range` 必须带工作表名；CLI 现在接受 `{"sheetName":"工作表1","range":"A1:C1"}` 并自动拼成 `工作表1!A1:C1`，裸 `A1:C1` 直接给出可读错误。
- `sheet-format` 的多个属性改为逐项 POST。实测 `background + color` 同时提交时服务端只生效一部分；拆分后导出 xlsx 确认底色、字体色、加粗全部生效。
- 视觉核验改用临时 Chrome 配置，避免持久配置里的旧渲染快照导致「API 成功但截图为旧状态」；同时放宽到「canvas 就绪」，表格页约 15–20 秒可完成截图。
- 页面未渲染完成时 `browser.verify` 返回失败，不再把空白白图当成功证据。
- `whoami` 的双重编码字段（如 `ä¼ä¸šé«˜çº§ç‰ˆ`）修复为「企业高级版」。

### 仍需人工/后续

- 历史、保存版本、查找替换、字符/段落格式、评论添加、@ 提及、预览、打印、冻结/排序/筛选/条件格式/数据验证/透视表等页面功能，CLI 不伪造成功；统一转 `shimo-net open` + computer-use，并在最终答复返回截图。
- 文件锁定、限制编辑、高级权限、成员级授权、彻底删除仍为人工页面操作。
- 跨文件评论汇总、单元格历史定位需要更多页面上下文，暂不承诺一键完成。


测试日期：2026-11-13（Asia/Shanghai）  
测试环境：macOS、当前 Codex 会话、本机 Keychain 登录态、shimo.net 公有云。  
测试二进制：`/tmp/shimo-net-current`，由当前 `cli` 源码构建。

## 2026-11-17 WorkBuddy 本地 0.9.4 回归

WorkBuddy 使用本地构建包 `/tmp/shimo-net-cli-0.9.4-local.zip` 安装技能，CLI 版本和 schema 均已切换到 `0.9.4`。以下清单记录本轮真实执行结果；账号、cookie 和 sid 不写入报告。

| 检查项 | 命令/动作 | 结果 |
|---|---|---|
| 版本 | `shimo-net version --json` | 通过，`0.9.4` |
| 能力清单 | `shimo-net schema --json` | 通过，28 场景、49 能力、22 扩展动作 |
| 登录态 | `shimo-net whoami --json` | 通过，WorkBuddy 会话可用 |
| 表格写入 | `sheet-update`、`sheet-append` | 通过，同源表格 API；现版本继续打开结果页做视觉核验并保存截图 |
| 表格行操作 | `sheet-insert`、`sheet-delete` | 通过，同源表格 API |
| 表格整理 | `sheet-freeze`、`sheet-sort`、`sheet-filter` | 页面操作入口已确认；由 computer-use 完成 |
| 表格校验/格式 | `sheet-validation`、`sheet-format`、`sheet-merge` | 格式/合并 API 通过并触发结果页截图；数据验证页面操作 |
| 留痕与版本 | `save-version`、`history-show` | 页面操作入口已确认；由 computer-use 完成 |
| 文本查找替换 | `find`、`replace-all` | 页面操作入口已确认；由 computer-use 完成 |
| 页面设置/输出 | `page-width`、`print` | 页面操作入口已确认；由 computer-use 完成 |
| 订阅 | `subscriptions --json`、`subscriptions --open` | 通过；先列文件，再按用户选择打开 |
| 文件导出 | `get <文件> --out <目录>` | 通过，使用异步导出 API |
| 浏览器兜底 | `open <文件>`、`open --default` | 通过；优先注入 Keychain 会话，默认浏览器为最后兜底 |
| 高风险项 | `share`、`purge`、锁定/限制编辑/成员授权 | 通过拒绝；必须人工网页操作 |

单元格历史仍要求用户先提供行号和列号；没有 API 的文字、段落、评论添加、打印等动作不会由 CLI 伪造成功，统一交给 Agent 浏览器人工操作并返回截图。

## 结论

- REST/API 能力可用：登录态、@人列表、评论列表、订阅扫描、在线文件导出、默认浏览器唤起均已验证。
- 当前页面没有暴露 `globalThis.sdk`。依赖页面 SDK 的动作仍快速失败并返回网页操作提示；表格内容、单元格样式和合并已改走后端 `/lizard-api`，不再依赖页面 SDK，也不再要求本地生成 XLSX 再导入。
- 无 SDK 的操作优先在宿主 agent 内联浏览器中完成；WorkBuddy 需要先启用官方市场 `agent-browser` 技能。确认宿主没有内联浏览器时才使用 `shimo-net open <链接> --default`，由 CLI 在可安全注入的默认 Chromium 隔离配置中带入 Keychain 会话后操作。
- 文件锁定、限制编辑、高级权限、成员级授权、彻底删除已列为 CLI 禁止项；必须打开石墨网页由人工确认和操作。
- 失败重试已改为遇到明确的“页面未暴露 editor SDK”立即返回，单次约 5 秒完成浏览器启动和诊断，不再等待约 30 秒重试。
- API 写入不再以 HTTP 200 作为唯一完成条件；会从 Keychain / Windows Credential Manager 注入会话，刷新真实编辑器页面并返回截图路径，登录页或截图失败会报告失败。

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
  --payload '{"sheetName":"工作表1","range":"A1:C1","attrs":{"background":"#D32F2F","color":"#FFFFFF","bold":true}}' --json
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
