# shimo-net

`shimo-net` 是 shimo.net（含客户私有部署）的命令行工具，覆盖 **28 个办公场景、50 项能力**。当前为最新版，Go 单二进制。

- 直连 `/api/v1`，不开端口、不跑常驻服务。
- 登录一次：Chrome 专用窗口 → macOS Keychain（Windows 为 Credential Manager）；过期就地重登。
- 数据类命令支持 `--json`，适合 Agent / 脚本调用。

> 仓库根目录的 [README](../README.md) 有完整的环境要求、Agent 接入和安全设计；本文件聚焦 CLI 用法。

## 安装

**macOS / Linux**

```bash
./install.command
```

**Windows（PowerShell）**

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

安装器会：

1. 有 Go 时本地构建，否则下载最新 release 二进制；
2. 安装到 `~/.local/bin/shimo-net`（Windows 为 `%LOCALAPPDATA%\shimo-net\shimo-net.exe`）；
3. 如存在 `~/.dsh`，把技能装到 `~/.dsh/skills/shimo-net-cli`；
4. 跑一次 `doctor`。

从源码构建：

```bash
go build -trimpath -o bin/shimo-net ./cli
bin/shimo-net version
```

需要 Go 1.22+；运行二进制本身不需要 Go。

如果提示 `command not found`，把 `~/.local/bin` 加入 `PATH`。

## 第一次配置

```bash
shimo-net init                    # 默认 https://shimo.net
shimo-net init --origin https://<客户域名>
```

`init` 会把网址写入 `~/.shimo-net-cli/config.json`，并请求 `GET /api/v1/users/me` 确认接口可达（返回 401 也算可达）。

查看 / 修改：

```bash
shimo-net config show
shimo-net config set https://<客户域名>
shimo-net config clear
```

`SHIMO_NET_ORIGIN` 环境变量优先级最高，适合临时切换。

## 登录

```bash
shimo-net login
shimo-net whoami
shimo-net doctor
```

- `login` 打开无地址栏的 Chrome 窗口，停在 `{origin}/login`；
- 首次使用没有 `config.json` 时，会提示输入产品网址；
- 登录成功后窗口关闭，会话写入 macOS Keychain（Windows 为 Credential Manager）；
- 登录过期：重新 `shimo-net login`，成功后重试原命令，网址不用重配；
- 切账号必须 `shimo-net login --new`；普通 `login` 会复用本机 Chrome 会话；
- 任意命令加 `--relogin`，遇 401 自动登录并重试；
- 不要把密码 / cookie 贴进对话、Issue 或日志。

### 登录态复用

Keychain 没有会话、或现有会话返回 401 时，CLI 会自动从本机 Chrome / Chromium / Edge / Brave 的已登录配置复用 shimo.net 会话：读取完整 cookie jar（`sid`、`smdev-ws-route`、`deviceId` 等），验证后写回 Keychain（写回失败时回退到 0600 文件）。不需要用户二次登录。

```bash
shimo-net session status --json     # 查看 Keychain / 文件 / 浏览器来源
shimo-net session harvest --json    # 手动从浏览器复用；--no-save 只验证
```

`open` 与 `open --default` 都会注入完整 cookie jar；`--default` 在默认浏览器是 Chromium 内核时使用 CLI 专用配置启动，非 Chromium 浏览器会如实提示无法注入。

退出并删除本机凭据：

```bash
shimo-net logout
```

## 当前能力（28 个场景、50 项能力）

### 页面操作的浏览器顺序

没有对应 API 的页面动作按 [docs/tutorials/browser.md](../docs/tutorials/browser.md) 的 P0 矩阵选浏览器。WorkBuddy 用 `present_files` 把 URL 放到右侧内置浏览器；豆包工作、DSH、Codex 没有可调用的内置浏览器，用 `shimo-net open` 或 `computer-use`。WorkBuddy 内置浏览器可用时不要先调用 `shimo-net open`，也不要启动 agent-browser 的 Chrome。

如果宿主没有内置浏览器，才使用 `shimo-net open <链接>` 或 `--default` 调起本机浏览器。CLI 会尝试注入本机 Keychain / Credential Manager 会话；不支持安全注入的浏览器会明确提示登录状态问题，且不会输出 sid/cookie。仓库里的 `cli/SKILL.md` 只供没有 SkillHub 技能的 Agent 使用，WorkBuddy 不需要上传或复制它。

| # | 场景 | 命令 |
|---|---|---|
| 1 | 搜索文件 / 文件夹 / 空间 | `shimo-net search "<关键词>" [--limit 20]` |
| 2 | 我的空间 / 空间里有什么 | `shimo-net spaces [--limit 300]`、`shimo-net ls <空间\|文件夹\|@别名> [--since 7d]` |
| 3 | 文件出处（类型 / 权限 / 路径） | `shimo-net info <链接\|@别名>` |
| 4 | 导入 + 权限 | `shimo-net import <文件> --to <位置>`；权限必须在石墨网页人工设置 |
| 5 | 拉取文件夹 | `shimo-net pull <链接\|@别名> [--out <目录>] [--since 7d]`（省略时 `./shimo-pull`） |
| 6 | 新建空白文档 / 文稿 / 表格 / 幻灯片 / 文件夹 | `shimo-net new <名称> --type doc\|docx\|sheet\|slide --to <位置>`、`shimo-net mkdir <名称> --to <父文件夹>`；`doc`=文档（document），`docx`=文稿（documentPro） |
| 7 | 改名 | `shimo-net rename <链接> "<新名字>"` |
| 8 | 删除 / 回收站 / 恢复 | `shimo-net delete`（只进回收站）、`trash`、`restore`；彻底删除到石墨网页手动操作 |
| 9 | 权限巡检 | `shimo-net audit <文件夹\|@别名> [--limit 200]` |
| 10 | 批量改权限 | CLI 禁止处理；打开石墨网页逐项人工确认和设置 |
| 11 | 读正文 / 摘要 | `shimo-net read <链接> [--max-chars 12000]` |
| 12 | 周报 | `shimo-net ls <链接> --since 7d` + `shimo-net read` |
| 13 | 附件 / 云文件下载 | `shimo-net get <链接> [--out <目录>]`（省略时 `./shimo-pull`） |
| 14 | 上传（问传到哪里） | `shimo-net upload <文件> [--to desktop\|空间\|文件夹\|@别名]`；不给 `--to` 会列出可选位置 |
| 15 | 常用文件夹别名 | `shimo-net fav add <别名> <链接> --note 特征`，之后任意命令用 `@别名` |
| 16 | 只看与我相关的内容 | `shimo-net phase2 <链接> --action mentions-list\|comments-list`；跨文件订阅用 `subscriptions` |
| 17 | 留痕与版本 | `shimo-net open <链接>` 后用 computer-use 操作历史面板 |
| 18 | 文字与段落 | `shimo-net open <链接>` 后用 computer-use 操作菜单和选区 |
| 19 | 文内表格与图片 | `shimo-net open <链接>` 后由 computer-use 插入、调整并截图 |
| 20 | 评论与提及 | `comments-list` 读取；添加评论和 @ 提及走页面操作 |
| 21 | 预览与打印 | `shimo-net open <链接>` 后用 computer-use 操作预览/打印 |
| 22 | 文档重排全文 | `shimo-net open <链接>` 后由 computer-use 调整标题、段落和页面样式 |
| 23 | 表格写入 | `phase2 --action sheet-update|sheet-append|sheet-insert|sheet-delete` 直接调用表格 API |
| 24 | 表格整理 | `shimo-net open <链接>` 后用 computer-use 操作冻结、排序、筛选和单元格底色 |
| 25 | 表格清洗 | 删除重复项、分列、填充等通过表格页面操作 |
| 26 | 表格算与看数 | `shimo-net open <链接>` 后由 computer-use 操作公式、定义名称和透视表 |
| 27 | 表格校验与格式 | `phase2 --action sheet-format|sheet-merge` 直接调用表格 API；数据验证仍用页面 |
| 28 | 表格输出 | 导出使用异步导出 API；打印用 `open` + computer-use |
| — | 用已登录浏览器打开链接 | `shimo-net open <链接\|guid>`（注入 Keychain/浏览器会话，免重复登录）；`--default` 用系统默认浏览器 |

## 编辑器与协作能力

当前可直接使用的编辑器能力包括 @人/评论列表 API、异步文件导出、订阅文件扫描，以及表格内容/样式/合并 API；历史、文字与段落格式、评论添加、预览打印等页面操作使用 `open` 配合 computer-use。涉及文件锁定、限制编辑、高级权限、成员级授权和彻底删除的操作禁止由 CLI 处理，必须由人工在石墨网页完成：

```bash
shimo-net phase2 <链接> --action sheet-format --payload '{"sheetName":"工作表1","range":"A1:C1","attrs":{"background":"#D32F2F","color":"#FFFFFF","bold":true}}' --json
shimo-net phase2 <链接> --action sheet-merge --payload '{"sheetName":"工作表1","cell":"A1","rowspan":1,"colspan":3}' --json
```

`phase2` 是扩展动作入口（保留该命令名以兼容已有 Agent）：`sheet-format`、`sheet-merge`、`sheet-values`、`sheet-update` 直接调用稳定的同源表格 API。表格 API 写入默认不启动 CLI 本地 Chrome；WorkBuddy 用 `present_files` 打开 `url`；其他宿主按浏览器矩阵；需要 CLI 截图时加 `--verify`，`--no-verify` 可显式关闭。`sheet-format` 的 `range` 必须带工作表名，推荐写成 `{"sheetName":"工作表1","range":"A1:C1",...}`；CLI 会拼成 `工作表1!A1:C1`，并把多个属性拆成多次写入（服务端同时提交 `background+color` 时可能只生效一部分），因此不会因为页面未暴露 `globalThis.sdk` 而退回本地 XLSX。API 成功后仍必须打开目标文件做视觉核验并截图；CLI 会先从 macOS Keychain / Windows Credential Manager 读取 cookie，注入目标域名后刷新页面，不输出 sid/cookie。页面动作没有 `--verify` 时直接失败；有 `--verify` 时会等编辑器挂载并检查截图，空白截图不算成功。其他动作如果没有对应 API，先由宿主内置浏览器按页面菜单、工具栏和选区完成操作并返回截图；WorkBuddy 用 `present_files`，其他宿主用 `shimo-net open`。没有截图不能报告完成。

订阅更新文件：`shimo-net subscriptions --json` 会扫描逐文件订阅状态；用户选定序号后用 `shimo-net subscriptions --open <序号>` 打开对应文件。打开前先使用宿主内置浏览器；没有内置浏览器时才使用 `shimo-net open <链接> --default`。

通用命令：`init` / `config` / `login` / `logout` / `whoami` / `doctor` / `session` / `schema` / `version`；全局 `--json`、`--relogin`。

## 常用位置（@别名）

```bash
shimo-net fav add ai https://shimo.net/folder/xxxx --note "市场部常用 · AI 方案"
shimo-net ls @ai
shimo-net upload 报告.pdf --to @ai
shimo-net fav list
shimo-net fav show ai
shimo-net fav rm ai
```

别名存在 `~/.shimo-net-cli/favorites.json`，只有别名和链接，不含账号信息。任何接受 `链接|guid` 的参数都可以用 `@别名`。

## 权限（仅查看；变更必须人工页面操作）

| 值 | 含义 |
|---|---|
| `private` | 私有（仅自己） |
| `enterprise_readonly` | 企业内只读 |
| `enterprise_commentable` | 企业内可评论 |
| `enterprise_editable` | 企业内可编辑 |
| `readonly` | 公开链接 · 只读 |
| `commentable` | 公开链接 · 可评论 |
| `editable` | 公开链接 · 可编辑 |

CLI 不提供权限变更命令。`share` 仅保留为兼容入口，调用时会明确拒绝并提示转到石墨网页。

后三档是公开链接，任何拿到链接的人都能访问；导入后先确认再设置。

## JSON 与退出码

```bash
shimo-net search "关键词" --json
shimo-net whoami --json
shimo-net doctor --json
```

- 成功：`exit 0`，stdout 为 JSON；
- 失败：`exit 1`，stderr 为 `{"ok":false,"error":"..."}`；
- 查看全部命令与参数：`shimo-net schema`。

## 行为约定

- `upload` = 原样上传云文件；`import` = 转成在线文档 / 文稿 / 表格 / 幻灯片。
- `upload` 不给 `--to` 会先列出位置；`import` 必须给 `--to`；权限变更转到石墨网页人工操作。
- **删除只进回收站；CLI 不提供彻底删除**，要彻底删请在石墨网页回收站手动操作。
- 文件锁定、限制编辑、高级权限、成员级授权均禁止由 CLI 处理，必须在石墨网页人工确认和操作。
- 只支持 `md / doc(x) / xls(x) / ppt(x)`；其他格式直说。
- 无权限如实说；大目录先 `ls` 再 `pull`；批量操作控制并发。
- 凭据：macOS 走 Keychain，Windows 走 Credential Manager；不可用时回退到 `~/.shimo-net-cli/credentials.json`（`0600`），需自行评估风险。CI 可用 `SHIMO_NET_COOKIE`。

> 完整安装 / 登录 / 排错见 [安装使用说明.md](安装使用说明.md)；场景话术见 [scenarios.md](scenarios.md)。
