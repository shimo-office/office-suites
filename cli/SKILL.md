---
name: shimo-net-cli
display_name: 石墨办公 CLI
display_name_en: shimo-net CLI
description_zh: 操作 shimo.net（含客户私有部署）时使用：用 shimo-net 命令行直连，搜索文件与空间、上传/导入、拉取下载、权限巡检、新建改名、读正文、常用文件夹别名；高风险权限和不可逆操作转石墨网页人工处理。
description_en: Use the shimo-net CLI for shimo.net (including private deployments) to search files and spaces, upload or import, pull or download, audit permissions, create or rename, read documents, and save favorite folders. High-risk permission changes and irreversible actions must be done in the Shimo web UI.
description: 操作 shimo.net（含客户私有部署）时使用：用 shimo-net 命令行直连，搜索文件与空间、上传/导入、拉取下载、权限巡检、新建改名、读正文、常用文件夹别名。涉及文件锁定、限制编辑、高级权限、成员级授权和彻底删除时必须转石墨网页人工操作。
whenToUse: 用户要用石墨 shimo.net 找文件/空间、上传或导入、拉取下载、权限巡检、新建/改名、读文档、记常用文件夹时；高风险权限和不可逆操作必须人工页面处理。
version: 0.9.9
agent_created: true
---

# shimo-net CLI

仓库内用根目录 `./shimo`；安装后用 PATH 里的 `shimo-net`。

## 检查与登录

```bash
shimo-net schema
shimo-net whoami        # 未登录或 401 时自动复用本机 Chrome 登录态；仍失败才 shimo-net login
shimo-net session status --json   # 查看会话来自 Keychain / 文件 / 浏览器
shimo-net doctor
```

### 登录态桥接

- 只要本机 Chrome / Chromium / Edge / Brave 里已经登录 shimo.net，CLI 不需要再弹一次登录：`whoami`、`open`、`phase2` 等命令会在 Keychain 没有会话或返回 401 时，自动从浏览器配置中读取完整 cookie jar，验证后写回 Keychain。
- 需要手动触发或只想验证时：`shimo-net session harvest --json`；加 `--no-save` 只验证不落盘。
- `shimo-net open <链接>` 会用 CLI 专用浏览器并注入会话；`--default` 在系统默认浏览器是 Chromium 内核时使用专用配置启动并注入，非 Chromium 浏览器会如实提示无法注入。
- 不要把 cookie / sid 粘进对话，也不要让用户为了“复用登录态”重新登录。

私有部署：

```bash
shimo-net init --origin https://<客户域名>
# 或临时：SHIMO_NET_ORIGIN=https://<客户域名> shimo-net ...
```

任意命令加 `--relogin`，遇 401 会自动打开登录并在成功后重试原命令。不要把密码 / cookie 粘进对话。

## 当前能力（28 个场景、50 项能力）

| 场景 | 命令 |
|---|---|
| 搜索 | `shimo-net search "关键词" --json` |
| 空间/目录 | `shimo-net spaces`、`shimo-net ls <链接\|@别名> --json` |
| 文件出处 | `shimo-net info <链接\|@别名> --json` |
| 上传（问位置） | `shimo-net upload <文件> [--to desktop\|空间\|文件夹\|@别名]` |
| 导入+权限 | `shimo-net import <文件> --to <位置>`；权限必须在石墨网页人工设置 |
| 拉取 | `shimo-net pull <链接> [--out <目录>] --json` |
| 新建 | `shimo-net new <名称> --type doc\|docx\|sheet\|slide --to <位置>`；`doc`=文档，`docx`=文稿（documentPro） |
| 新建文件夹 | `shimo-net mkdir <名称> --to <父文件夹\|@别名>` |
| 改名 | `shimo-net rename <链接> <新名字>` |
| 删除/回收站/恢复 | `delete`（只进回收站） / `trash` / `restore`；彻底删除不提供，需网页手动操作 |
| 权限巡检 | `shimo-net audit <文件夹\|@别名>` |
| 批量改权限 | CLI 禁止处理；打开石墨网页逐项人工确认和设置 |
| 读正文摘要 | `shimo-net read <链接> [--max-chars 12000]` |
| 周报 | `shimo-net ls <链接> --since 7d` + `shimo-net read` |
| 附件下载 | `shimo-net get <链接> [--out <目录>]` |
| 常用文件夹别名 | `shimo-net fav add <别名> <链接> --note 特征` → 之后用 `@别名` |
| 只看与我相关的内容 | `shimo-net phase2 <链接> --action mentions-list\|comments-list`、`subscriptions` |
| 留痕与版本 | `shimo-net open <链接>` 后用 computer-use 操作历史面板 |
| 文字与段落 | `shimo-net open <链接>` 后由 computer-use 操作菜单和选区 |
| 文内表格与图片 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 评论与提及 | `comments-list` 读取；添加评论和 @ 提及走页面 |
| 预览与打印 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 文档重排全文 | `shimo-net open <链接>` 后由 computer-use 调整 |
| 表格写入 | `phase2 --action sheet-update|sheet-append|sheet-insert|sheet-delete` 直接调用表格 API |
| 表格整理 | `shimo-net open <链接>` 后用 computer-use 操作冻结、排序、筛选和单元格底色 |
| 表格清洗 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 表格算与看数 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 表格校验与格式 | `phase2 --action sheet-format|sheet-merge` 直接调用表格 API；数据验证仍用页面 |
| 表格输出 | 导出使用异步导出 API；打印由 computer-use 操作 |
| 打开链接（免登录） | `shimo-net open <链接>`：注入 Keychain/浏览器会话；`--default` 用系统默认浏览器 |

## 必须遵守

1. **上传/导入先确认位置**：桌面 / 空间 / 文件夹；没有目标就列出候选让用户选，不要瞎猜。
2. **导入后必须转网页人工设置权限**（7 档），不要默认公开；`upload` 是云文件不参与分享设置。
3. 文件锁定、限制编辑、高级权限、成员级授权和彻底删除均**禁止由 CLI 处理**，遇到这些请求必须打开石墨网页，由人工确认和操作；删除命令只进回收站。
4. 只支持 md / doc(x) / xls(x) / ppt(x)，其他直说。
5. 无权限如实说；大目录先 `ls` 再 `pull`；`--json` 便于解析。
6. 用户提过的常用文件夹用 `fav add` 记成 `@别名`，下次直接用。

## 编辑器与协作能力

### 宿主浏览器与 Computer Use 优先级

页面编辑、评论、历史、预览、打印和其他没有稳定 API 的动作，先检查当前 Agent 是否提供内置浏览器和网页操作能力。WorkBuddy 对应的官方市场技能名称是 **agent-browser**（它提供网页打开、点击、输入和截图能力；不要求复制本仓库的 `cli/SKILL.md`）。

按下面顺序处理：

1. **有内置浏览器/网页操作能力**：直接用宿主提供的内置浏览器打开目标文件，在该页面完成操作并截图；不要先运行 `shimo-net open`，也不要先调用系统默认浏览器。表格 API 动作（`sheet-update` / `sheet-append` / `sheet-insert` / `sheet-delete` / `sheet-format` / `sheet-merge`）默认不启动 CLI 本地 Chrome，把视觉核验交给宿主内置浏览器。
2. **WorkBuddy 没有该能力或技能未启用**：先询问用户是否允许安装/启用 WorkBuddy 市场里的官方 `agent-browser` 技能（已安装时只开启开关）。用户允许后才执行安装或启用，并用一次能力检查确认能打开页面和截图；用户拒绝时停止页面操作并返回市场链接与手动步骤。安装技能属于宿主配置，`shimo-net` 不会自行复制仓库技能或伪造 Computer Use。
3. **用户允许但宿主仍没有内置浏览器**：才使用 `shimo-net open <链接> --default` 打开本机默认浏览器。CLI 会先从 Keychain / Credential Manager 取得会话，并在 Chromium 系默认浏览器的隔离配置中注入 cookie；非 Chromium 浏览器无法安全注入时如实提示，不要打印 sid/cookie。

`shimo-net open <链接>` 是 CLI 自己的浏览器会话入口，适合没有宿主内置浏览器的 Agent；它不是 WorkBuddy 内置浏览器的替代检测。Agent 不得因为 CLI 命令可用就跳过宿主能力检查。

当前直接可用的是 @人/评论列表 API、异步文件导出、订阅文件扫描，以及表格内容/样式/合并 API。历史、评论添加、文字与段落格式、预览打印等操作使用内联浏览器配合 computer-use。文件锁定、限制编辑、高级权限、成员级授权和彻底删除严格禁止 CLI 处理，必须由人工进入石墨网页操作：

```bash
# 默认不启动本地 Chrome；用宿主内置浏览器打开 url 截图
shimo-net phase2 <链接|guid|@别名> --action sheet-format --payload '{"sheetName":"工作表1","range":"A1:C1","attrs":{"background":"#D32F2F","color":"#FFFFFF","bold":true}}' --json
shimo-net phase2 <链接|guid|@别名> --action sheet-merge --payload '{"sheetName":"工作表1","cell":"A1","rowspan":1,"colspan":3}' --json
# 需要 CLI 本地 Chrome 截图时
shimo-net phase2 <链接|guid|@别名> --action sheet-format --payload '…' --verify --json
shimo-net phase2 <链接|guid|@别名> --action mentions-list --json

> 有内置浏览器的宿主（WorkBuddy `agent-browser`）：不要加 `--verify`，默认由宿主浏览器截图。没有内置浏览器、需要 CLI 截图时才加 `--verify`。`--no-verify` 仍可显式关闭本地浏览器。
```

`phase2` 是兼容入口。表格 `sheet-format`、`sheet-merge`、`sheet-values`、`sheet-update` 直接调用稳定的同源 `/lizard-api` 接口，不依赖页面 `globalThis.sdk`，因此不会再退回本地生成 XLSX。API 返回成功只代表服务端接受了请求，**不是最终完成条件**。`phase2` 的表格 API 写入默认不启动本地 Chrome，返回 `verified:false`、`verifyWith:"host-browser"`；接下来用宿主内置浏览器（WorkBuddy 的 `agent-browser`）打开 `url` 截图。只有宿主没有内置浏览器、需要 CLI 截图时，才加 `--verify`，由 CLI 读取 Keychain / Credential Manager 的 cookie、注入本地浏览器并截图。页面 SDK 未暴露或动作没有可用 API 时，先按上面的宿主浏览器优先级执行；确认没有内置浏览器后再使用 `shimo-net open <链接> --default`。如果本机凭据存储中没有会话，必须如实报告无法完成视觉核验，不能只凭 HTTP 200 宣称成功。

订阅更新：

```bash
shimo-net subscriptions --json
shimo-net subscriptions --open 1
```

该命令逐文件查询 `/api/v1/files/{fileGuid}/update_subscription`，先列已订阅文件，再按用户选择打开对应文档；没有服务端全量订阅列表或最近更新正文接口时，不要猜测结果。

浏览器操作固定步骤：先 `whoami --json`；浏览器显示登录页时，先检查本机 Keychain / Windows Credential Manager，再检查本机 Chrome 是否已登录（`shimo-net session harvest --json`）；有会话就由 CLI 注入目标域名完整 cookie jar 并刷新，禁止要求用户二次登录；只有两处都没有会话时才提示用户登录。页面动作先检查宿主内置浏览器；WorkBuddy 未安装或未启用 `agent-browser` 时，先询问用户是否允许安装/启用，获准后再操作并验证截图能力；用户拒绝时不要弹外部浏览器。确认宿主没有内置浏览器且用户允许兜底后才用 `shimo-net open <链接> --default`，不要直接把页面导航到登录地址；没有 API 时使用宿主网页操作或 computer-use 直接操作页面菜单、工具栏和选区；单元格历史必须先让用户说明行号和列号；单元格底色直接通过表格格式菜单操作；写操作先确认目标和影响范围；不要复制 sid；不要把 sid/cookie 写入日志或消息；**每个成功场景必须保存截图并在最终答复中返回绝对路径和 Markdown 图片**；截图失败或仍停留在登录页时必须如实说明并保留失败截图，不能报告完成。
