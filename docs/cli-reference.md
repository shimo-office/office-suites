# 命令参考

**中文** | [English](en/cli-reference.md) · [文档导航](README.md)

本页供终端用户、脚本开发者和 Agent 查参数。用 AI 助手时无需记命令，先看[一句话安装与使用](agent/README.md)；想知道可以怎么提需求，见[办公场景](scenarios.md)。

安装、账号切换与排错见[安装说明](install.md)。本页跟随仓库源码维护，可能包含尚未发布的命令；实际可用命令与动作以本机 `shimo-net schema --json` 为准。

`editor` 是编辑器接口与页面入口的公开命令；旧命令 `phase2` 保留为兼容别名，使用相同实现和参数。现有脚本无需改写，新增示例统一使用公开命令。

## 按任务查阅

[配置与诊断](#配置与诊断) · [文件与目录](#api文件与目录) · [常用位置](#别名) · [编辑器接口](#编辑器接口与页面入口) · [打开页面](#打开页面) · [扩展编辑](#固定文稿幻灯片应用表格和表单动作) · [退出码与恢复](#json退出码与失败恢复)

## 通用约定

```text
shimo-net <command> [位置参数] [--option value] [--json]
```

- `<链接>` 可用目标 guid；支持目标解析的命令也可使用 `@别名`。
- `--help` / `-h` 在命令参数中仅显示帮助，不执行命令；未知选项以用法错误退出。
- `--json` 可放在命令名前或后；数据命令输出结构化结果；`schema` 本身输出 JSON，`help` 输出文本。
- `--relogin` 只允许只读命令在打开全新登录窗口、由用户完成登录后重试；所有写命令和固定编辑入口 `phase3` 禁止自动重放。写入已受理、后验失败时，先读回和检查证据，不重提追加、删除、发布或答卷。
- `--payload` 是 JSON；macOS/Linux 示例使用 shell 单引号，PowerShell 按自身 JSON 引号规则传入。
- `SHIMO_NET_ORIGIN` 临时覆盖目标 origin。凭据配置见 [安全策略](../SECURITY.md)，不要在参数、对话或日志中输出 cookie。
- 共享会话默认位于 `<状态目录>/sessions/<sha256(origin)>.json`（`0600`），同一系统用户下的所有 CLI 共用；`SHIMO_NET_SESSION_DIR` 可整体改位置，`SHIMO_NET_STATE_DIR` 改变时共享范围随之改变。
- 共享会话里的 cookie 按实际请求地址投递：Domain、Path 前缀、Secure（只发 https）和 Expires 都会判断；都没有可用 cookie 时命令直接失败，不回退钥匙串。

## 配置与诊断

| 命令 | 参数与行为 |
|---|---|
| `init [--origin URL] [--login]` | 先用 `GET /api/v1/auth/register/is_new` 探测，成功后才保存网址；401 不代表已登录；`--login` 随后进入登录。 |
| `config show` | 显示 origin 配置。 |
| `config set URL` | 与 init 使用同一探测标准，失败不覆盖旧配置；切换 origin 后旧凭据不能跨域使用。 |
| `config clear` | 清理 origin 配置，不是退出浏览器账号。 |
| `login [--origin URL] [--new]` | 仅无已存凭据记录时，普通登录才尝试复用会话；`--new` 使用全新临时配置用于切账号。 |
| `logout` | 删除 CLI 本地凭据，不会退出日常浏览器会话。 |
| `session status` | 查看当前会话来源及状态；`--json` 含共享会话的 revision、账号与 cookie 名称（不含值）。 |
| `session harvest [--no-save]` | 验证并复用浏览器会话；`--no-save` 只验证不保存。 |
| `session import [--no-verify] [--source NAME] [--expected-revision N]` | 从 stdin 读 cookie 头、`{"cookie":"..."}` 或 `{"cookies":[...]}` 写入共享会话；默认先向 `/users/me` 校验。已有记录时，不带 `--expected-revision` 只允许同一 `sid` 的属性刷新，切号与退出后恢复必须带当前版本。 |
| `session watch [--interval 1s] [--once]` | 订阅共享会话变化；每次 revision 变化输出一条快照（无 cookie 值）。每轮只 stat 文件（约 3.4 µs），时间戳与大小未变不读取内容，每 30 轮兜底全量读一次。 |
| `session export [--out 文件]` | 把当前共享会话导出为 JSON（含 cookie 值与账号），供宿主浏览器注入同一 origin；文件 0600、不覆盖已有文件、只打印路径与指纹，绝不把凭据打到 stdout。 |
| `session logout` | 只标记共享会话已退出；`logout` 会同时清理本地凭据。 |
| `whoami` | 查询实际当前账号。 |
| `doctor` | 检查平台运行环境、浏览器、origin 和账号；失败返回非 0。 |
| `schema [--action 动作]` | 输出命令、类型、动作分类、场景与能力计数；`phase3.actionParams` 与 `editor.actionParams` 给出各自已列动作的 payload 契约（必填、可选、枚举、示例），带 `--action` 只输出该动作的契约。 |
| `update [--check] [--json]` | 更新当前安装；`--check` 只检查。npm 安装更新整个包，原生安装校验 GitHub Release 后替换；不降级、不修改登录和配置。旧版需先按[安装说明](install.md)升级一次。 |
| `version` | 输出本地二进制版本；支持 `--json`。 |
| `help` / `--help` / `-h` | 输出当前帮助。 |

### 登录态与「内置浏览器显示登录页」

CLI 的编辑器通道使用独立浏览器配置并注入 CLI 会话；宿主的**内置预览浏览器未必共享这份 cookie**。因此「内置预览停在登录页」不等于 CLI 登录失效，两者要分开判断：

| 现象 | 含义 | 正确处理 |
|---|---|---|
| 命令能读文件、`whoami` 正常，但内置预览显示登录页 | 宿主浏览器没有 CLI 的 cookie | 用截图/导出物展示；宿主支持安全会话导入时由适配程序导入同一 origin 并验证账号。`open` 使用自身配置，不能保证 CLI 会话 |
| 命令报 `AUTH_REQUIRED`，`loginRedirect:true`，页面路径为 `/login` | 会话确实失效 | `session status --json` 核对后 `login --new` |
| 命令报 `EDITOR_NOT_READY`，页面停在骨架屏/资源未加载 | 资源或网络问题，不是鉴权 | 保留现有会话重试；检查静态资源与代理，不要重复登录 |

`preview` 及已接入展示契约的 `editor` 结果带以下字段；其他扩展动作仍以自身结果为准：

- `auth`：`cliSession` 表示本地凭据可用，`credentialSource/sharedSession/authState/revision` 为本地记录；`verified:false`、`loginRequired:null` 表示本摘要未验证服务端账号，不能把凭据存在当作登录成功。
- `view`：`transport`（`link` / `cli-browser` / `download` / `artifact`）、`usesCLISession`、`browserOpened`。链接的 `builtInBrowser.usable/sessionShared` 为 `null`、`authState:unknown`，由宿主验证后才能声明可用；本地产物可直接展示。URL 的 `openWith` 是便捷命令，`openSessionShared:false` 明确它不注入 CLI 会话。

Agent 应按 `view.transport` 决定展示方式，不要用内置浏览器打开需要登录的编辑页再判断“登录失效”。

### 更新 CLI

```bash
shimo-net update --check --json  # 联网检查，不安装
shimo-net update --json          # 更新当前安装
shimo-net version --json         # 核对当前版本
```

`update` 不接受位置参数或指定版本；需要固定版本时使用 [npm 版本管理](npm.md#更新与指定版本)或指定版本的 Release 文件。npm 安装查询官方 npm registry，原生安装查询本仓库最新稳定 GitHub Release。已有版本相同或更高时成功退出，不执行安装；检查与更新均不需要石墨账号。

带 `--json` 时，成功在 stdout 输出一个 JSON 对象，失败将错误 JSON 写入 stderr，并以非 0 退出。字段说明：

| 字段 | 含义 |
|---|---|
| `ok`、`source` | 成功标记；来源为 `npm` 或 `github`。 |
| `currentVersion`、`latestVersion` | 本次操作开始时的版本、查询到的最新版本。 |
| `updateAvailable` | 查询时最新版本是否高于当前版本；更新成功后此字段仍可为 `true`。 |
| `updated`、`checkOnly` | 本次是否完成更新、是否只检查；`--check` 时 `updated` 必为 `false`。 |
| `path` | npm 包目录，或原生程序解析符号链接后的实际路径。 |
| `scope` | 仅 npm：`global` 或 `project`。 |
| `backupPath` | 仅原生更新按需返回：Windows 尚未能删除的旧程序路径；当前进程退出后可清理。 |

原生更新先检查下载大小、SHA-256 和新版程序输出的版本，再替换；下载或验证失败保留旧程序。npm 更新遵循包管理器的安装行为；无法确定安装范围时拒绝自动更新，改用原安装命令。详细限制与旧版升级方法见[安装说明](install.md)及 [npm 指南](npm.md)。

## API：文件与目录

| 命令 | 参数与边界 |
|---|---|
| `search <关键词> [--limit 20]` | 搜索可访问文件、文件夹和空间。 |
| `spaces [--limit 300]` | 列出当前账号参与的空间。 |
| `ls <目录> [--since 7d] [--limit 200]` | 列目录；since 按更新时间过滤；注意扫描上限。 |
| `info <链接>` | 读取类型、权限和可访问路径。 |
| `upload <本地文件> [--to 位置]` | 原样上传云文件；不提供目标时列出可选位置，确认后另发完整命令。 |
| `import <本地文件> --to <位置> [--title 名称] [--share 档位]` | 转换 md、doc(x)、xls(x)、ppt(x) 为在线格式；必须指定位置。仅在用户明确指定分享档位时使用 --share，导入后单独设置并读回权限。 |
| `new <名称> [--type 类型] [--to 位置]` | 新建在线文件；默认 doc，默认位置 desktop。完整类型见下表。 |
| `mkdir <名称> [--to 父文件夹]` | 新建文件夹，默认 desktop；不使用 `new --type folder`。 |
| `rename <链接> <新名字>` | 改名后保持原 GUID；批量调用逐项检查。 |
| `delete <链接>` | 直接移入回收站；不弹确认，不提供 `--yes`。 |
| `trash` | 列回收站。 |
| `restore <guid>` | 恢复回收站中的目标。 |
| `share <链接> <档位>` | 设置 private / readonly / commentable / editable 或企业内三档 enterprise_*，写后独立读取 shareMode 核对；不包含成员授权。 |
| `audit <目录> [--limit 200]` | 只读权限巡检；未知权限单独分组，未知或截断时非 0 退出，不计作私有。 |
| `read <链接> [--max-chars 12000]` | 按实际导出格式解析正文；可从 md 回退 docx，回退 PDF 时明确拒绝解析并提示 get。超长内容会截断。表格输出的工作表标题使用工作簿中的真实工作表名（可直接用于 `sheet-values` / `sheet-update` 的 `range`），不再固定显示 `Sheet 1`。 |
| `get <链接> [--out 目录]` | 附件原样下载，在线文件按类型首选格式导出，失败可回退 PDF；以 `exportType` 和实际扩展名为准。默认 `./shimo-pull`，同名或已有本地目标自动加序号保留，明确拒绝 `--format csv`。 |
| `pull <目录> [--out 目录] [--since 7d] [--limit N]` | 仅下载文件夹/空间当前层，默认 `./shimo-pull`；子目录记入 manifest 为 skipped，不递归。默认不限当前层数量；文件名冲突自动加序号；结束前核对落盘文件大小。清单使用独立隐藏文件，以结果 `manifest` 字段为准，不覆盖云端 `manifest.json` 或旧清单。有失败项非 0 退出，保留已下载文件、清单与逐项错误。 |
| `subscriptions [--limit 200] [--depth 3] [--open 序号或guid] [--default]` | limit 是扫描文件数上限，不是返回条数；仅对扫描范围内的订阅文件按更新时间排序，先列再选择。`--open` 会启动本地浏览器，先按浏览器规则确定该方式适用；无服务端全量“最近正文”接口。 |

位置可以是 `desktop`、可访问空间/文件夹链接或 `@别名`。上传原文件与导入在线格式是不同操作；PDF 扫描件等不支持直接导入时，可上传为云文件，不冒称已转成可编辑正文。

### 新建类型

| `--type` | 文件类型 |
|---|---|
| `doc` | 轻文档（document） |
| `docx` | 文稿（documentPro，Word 类编辑器） |
| `sheet` | 电子表格（spreadsheet） |
| `slide` | 幻灯片（presentation） |
| `table` | 应用表格（table），与 sheet 不同 |
| `form` | 表单 |
| `mindmap` | 思维导图 |
| `flowchart` | 流程图 |

这些是公开创建类型；可创建文件不等于该类型全部编辑功能都已封装或验收。创建后应核对实际文件类型、链接、位置与账号权限。

## @别名

```bash
shimo-net fav add project "https://shimo.net/folder/<guid>" --note "项目归档" --json
shimo-net fav list --json
shimo-net fav show project --json
shimo-net ls @project --json
shimo-net fav rm project --json
```

`fav add <别名> <链接或guid> [--note 说明]` 保存本地位置；`list/show/rm` 分别列出、查看、删除别名。别名支持中英文、数字、下划线和短横线，最长 40 字。`favorites.json` 只包含位置与说明，不含账号凭据。

## 编辑器接口与页面入口

```text
shimo-net editor <链接或guid或@别名> --action <动作>
  [--payload JSON] [--screenshot PATH] [--verify|--no-verify] [--json]
```

### API 动作

| 动作 | payload | 结果与核验 |
|---|---|---|
| `sheet-values` | `{"range":"工作表1!A1:C5"}` | 只读有值单元格；空行和合并后的位移不能只靠它判断。 |
| `sheet-update` | `{"range":"工作表1!A1","resource":{"values":[["值"]]}}` | 写指定区域，必须提供 `resource.values`；裸 `values` 不受支持。 |
| `sheet-append` | 同上，明确工作表范围和 `resource.values` | 表尾追加；读回核对目标记录和数量，失败后不盲目追加第二遍。 |
| `sheet-insert` | `{"sheetName":"工作表1","index":3,"count":1}` | 插入行；index 透传为接口行索引，不自动转换自然语言行号，执行前核对目标行。count 默认 1。 |
| `sheet-delete` | 与插入行相同 | 删除指定行；不是删除列或删除工作表。先确认目标及影响范围。 |
| `sheet-format` | `{"range":"工作表1!A1:C1","attrs":{"background":"#D32F2F","color":"#FFFFFF","bold":true}}` | 单元格底色、字体颜色等走 API。允许属性见下文；多属性分次提交，不宣称原子性。 |
| `sheet-merge` | `{"sheetName":"工作表1","cell":"A1","rowspan":1,"colspan":3}` | 合并指定单元格范围；核对导出结构和页面。 |
| `mentions-list` | 按动作默认读取目标文件 | 通过 HTTP 读取页面签名和提及列表；不启动浏览器。轻文档输出 `文档地址#atGuid`。 |
| `comments-list` | 按动作默认读取目标文件 | 通过 HTTP 读取页面签名和评论列表；不启动浏览器。轻文档输出 `文档地址#selectionGuid`，同一位置只输出一个链接。 |

### 表格页面动作（页面 façade / 页面菜单）

| 动作 | payload | 结果与核验 |
|---|---|---|
| `sheet-freeze` | `{"sheetName":"工作表1","rows":1,"columns":0}` | 调页面 `activeSheet.setFrozenRowCount/setFrozenColumnCount`，写入后**刷新页面重新读取**冻结行列，并要求导出工作簿的单元格值/样式/合并结构未变；不支持冻结尾行尾列。 |
| `sheet-filter` | `{"sheetName":"工作表1","range":"A1:C10"}` | 调页面 `tn.addFilter` 建立筛选区域；重开后读取筛选区域比对行列与范围。已有其它筛选区域时要求先 `sheet-filter-clear`。 |
| `sheet-filter-clear` | `{"sheetName":"工作表1"}` | 清除当前筛选；重开后必须读不到筛选区域。 |
| `sheet-export-pdf` | `{"sheetName":"工作表1"}`（可省略） | 驱动页面菜单：更多 → 下载 → 纯图PDF → 导出设置（等待「共 N 页」且不再「正在准备导出」）→ 下一步；浏览器下载被限制到临时目录，核验 `%PDF` 信封与大小后才落到 `--out`。服务端导出接口对该类型返回 3613，PDF 只能由页面生成。 |

这三个写入动作的失败结果带 `stage`、`mutationAttempted`、`mayHaveApplied`、`noReplay`：写入已提交但核验失败时禁止整条命令重放。PDF 导出是只读动作，重跑只会多产出一份文件。

`get <表格链接> --format pdf [--out 目录]` 自动走同一页面导出流程，不请求不支持的服务端 PDF 导出接口；`--no-browser` 会在启动前拒绝。`sheet-export-pdf` 的 `settings` 记录 `source: current-page-settings`、`modified:false`，以 `dialog.json` 和截图保存实际设置，不假定固定 A4、范围或网格线。未指定 `--out` 时 PDF 也会保留，不能随临时验证证据清理。

`mentions-list/comments-list` 使用已有登录态，未登录时提示登录，不自动打开浏览器；显式 `--relogin` 仍按全局规则执行登录。即使传入 `--verify` 或 `--keep-open`，这两个读取动作也不启动浏览器。文本输出条数和去重后的定位链接；空列表或缺少定位字段会明确提示。`--json` 保留 `result.ok` 和 `result.value` 原始数据，并增加 `links`、`count`、`unlocatedCount`；`verified:true` / `verifyWith:"http"` 仅表示 API 读取成功，不表示已验证页面跳转。

定位链接目前仅为 `/docs/` 轻文档生成：提及使用 `#atGuid`（不是 `?mentionAt=`），评论使用 `#selectionGuid`（字段已含 `comment-`，不是 `commentGuid`）。其他套件或缺少有效定位字段时不猜测链接，文本会提示并给出文档地址。建议新标签页打开或刷新链接，让编辑器初始化时执行定位。

区域格式为 `工作表名!A1:C5`；也可同时提供 `sheetName` 和不带表名的 `range`，CLI 会拼接。`sheet-format` 只接受已验证的 `background/color/bold/italic/underline`；字体大小、字体名称、对齐、数字格式、列宽等不在该动作支持范围内，会返回错误。文稿或幻灯片同名格式能力不能外推为电子表格 API 支持。

表格 API 写入默认不启动本地 Chrome，返回 `verified:false` 和页面核验提示。完成要求是 **读回 → 导出 XLSX 核对目标单元格/结构 → 真实页面截图**；HTTP 2xx、`applied` 或 values 中看不到变化都不是完整物证。需 CLI 截图且浏览器方案已确定时使用 `--verify`；`--no-verify` 只关闭截图验证；`--no-browser` / `SHIMO_NET_NO_BROWSER=1` 禁止此入口启动浏览器；mentions-list/comments-list 支持此选项。浏览器选择只遵循 [场景清单](scenarios.md#浏览器优先级)。

### 页面动作

| 动作 | 实际行为 |
|---|---|
| `history-show` / `history-hide` | 调套件根级 SDK（`window.__SM__.editor.history`）打开/关闭历史侧栏；必须等编辑器渲染完成后再调用，面板可见才算成功。 |
| `save-version` | 调 `sdk.version.createRevision` 创建版本；成功返回即已提交。 |
| `comments-show` / `comments-hide` | 调 `sdk.comments.show/hide` 打开/关闭评论面板；面板可见才算成功。 |
| `comment-add` | CLI 拒绝；评论输入区由画布绘制，无法稳定定位输入焦点，须在持久页面输入、提交并核对评论。 |
| `print` | 调 `sdk.print` 唤起套件打印设置页（表格验证通过）。 |
| `cell-history` | 表格页右键目标单元格 → 「查看单元格历史」；面板可见才算成功，当前定位 A1 附近的数据单元格。 |

页面动作依赖套件编辑器真正渲染完成：CLI 会等到 `editor.initialized`、`sdk._rendered` 或首帧 canvas 就绪后再调用，避免 `history.show()` 在 UI 层尚未挂载时被静默吞掉。

`history-show` / `comments-show` / `print` / `cell-history` 默认把浏览器窗口留在桌面，方便直接查看面板；批量验收需要自动关闭时加 `--close`，其他动作要保留窗口可加 `--keep-open`。返回 JSON 中 `browserKept: true` 表示窗口已保留，`browserProfile` 为其临时配置目录。

仅历史/评论的 show/hide 展示动作通过页面 SDK 执行；`--verify` 是这些动作的执行及截图开关，输出 `scope:panel-visibility-only`、`actionVerified:false`。临时浏览器完成截图后关闭，不能用于后续提交。编辑器未加载、方法不可用或截图空白时返回失败。没有可用 SDK 时由宿主按页面菜单操作，不以本地模拟结果代替。

仍为页面操作的入口会返回结构化拒绝：`code`、`bottleneck`（已验证的瓶颈）、`pagePath`（页面路径）、`next`（补齐条件），退出码 3，且 `mutationAttempted:false`。

| 动作 | code | 瓶颈（已实测） |
|---|---|---|
| `sheet-sort` | `UNSUPPORTED_SORT_VERIFICATION` | façade 有 `tn.sort/sortCustom/sortSheetByCol`，调用后单元格顺序未变化，也没有可读排序状态，无法区分「被忽略」和「已排序」 |
| `sheet-validation` | `UNSUPPORTED_VALIDATION_FACADE` | 数据验证模型在 `core.validations` 上延迟初始化（同一会话内时有时无），规则对象 schema 未公开、无稳定读回 |
| `comment-add` | `UNSUPPORTED_COMMENT_SERVICE` | 评论走云端评论服务并绑定作者身份；网格 `remarks` 只是单元格批注，不能当评论提交 |
| `find` / `replace-all` | `UNSUPPORTED_FIND_PANEL` | 查找替换是页面面板流程，没有可读回的命中集合 |
| `page-width` | `UNSUPPORTED_PAGE_WIDTH` | 视图状态，无稳定入口 |

数据透视表、工具菜单的重复值/清洗、行列管理与公式名称也仍是页面操作：透视表由页面向导生成并依赖服务端聚合模型，façade 上没有透视表对象；清洗命令依赖当前选区与交互确认，没有可核验的结果读回。**结论：能落到 façade 且能重开核验的（冻结、筛选、PDF 导出）已经命令化；只有调用成功但没有证据的，一律不接。**

## 打开页面

`open <链接> [--default]` 使用独立持久 `open-profile`，不自动注入 CLI 会话，不开启远程调试端口；可以复用该配置内由用户登录的会话，首次打开可能需要登录。`--default` 选择操作系统默认浏览器应用：支持的 Chromium 应用仍使用 CLI 隔离配置，其他浏览器交给系统打开，不保证登录。检测到该配置遗留调试端口标记时拒绝复用，应先关闭相关浏览器并排查。升级前已打开的旧版调试浏览器不会自动消失，须用户自行关闭。

## 固定文稿、幻灯片、应用表格和表单动作

扩展固定动作保持 `phase3` 命令入口，接受动作白名单及受约束的 JSON 参数；不接受任意 JavaScript。参数与示例见 [总览](phase3-commands.md)、[文稿](phase3-document-commands.md)、[幻灯片](phase3-slide-commands.md)、[表单](phase3-form-commands.md)；轻文档（`/docs/`）的 `doc-format/doc-paragraph/doc-replace` 见[总览的轻文档一节](phase3-commands.md#轻文档)。

电子表格补强动作 `sheet-update/append/insert/delete/format/merge` 必须提供 `--read-range`；`--out` 保存逐次运行证据。不指定 `--out` 的普通动作成功后清理临时证据；失败证据限期、限量保留，PDF 和表单答卷导出保留为正式结果，详见[临时文件规则](phase3-commands.md)。`phase3 artifact <本地文件>` 只检查文件包结构，明确 `actionVerified:false`，不证明线上写入，不能将本地检查或只读查询当成整项需求通过。

## 人工禁用

`purge` 以及编辑器的锁定、限制编辑、高级权限、成员授权、彻底删除动作都返回拒绝提示。`share` 与 `import --share` 支持用户明确指定的七档文件分享权限变更，不属于此禁用列表。详情由 [安全策略](../SECURITY.md) 唯一维护；`audit` 可以读权限，不能修改。

可读权限值：`private`、`enterprise_readonly`、`enterprise_commentable`、`enterprise_editable`、`readonly`、`commentable`、`editable`。后三档表示公开链接权限；读取这个值不授权 CLI 扩大或收回权限。

## JSON、退出码与失败恢复

| 退出码 | 含义 |
|---:|---|
| 0 | 当前命令执行成功；仍应查看 `verified`、验证范围和业务断言，不能据此推断全部需求完成。 |
| 1 | 命令错误、参数无效、权限拒绝或验证失败；带 `--json` 时错误写 stderr，结构为 `{"ok":false,"error":"..."}`。 |
| 2 | 未知命令；stderr 输出错误，带 `--json` 时同样是单个 JSON 错误。 |
| 3 | 当前动作未封装，或需要页面浏览器但被禁止；按 `code`、`bottleneck` 与 `mutationAttempted` 判断，不当作已执行成功。 |
| 4 | 会话或编辑器未就绪；按 `AUTH_REQUIRED`、`EDITOR_NOT_READY`、`SESSION_EMPTY` 等 `code` 区分原因，不一律要求重新登录。 |

成功 JSON 与 stderr 应分别捕获，保留退出码。stderr 也可能包含浏览器诊断或临时目录清理警告，不要把 `2>&1` 的混合内容直接当成单个 JSON。操作成功且 `verified:true` 时，临时浏览器配置清理警告不代表写入失败；其他警告按内容判断，不能为消除警告重放写入。面向用户默认摘要结果、核验范围和正式产物，完整 entries/哈希仅用于排错；未来手动操作可省略 `--json` 看简短输出。

`doctor`、`audit`、`pull` 的失败结果仅在 stderr 输出一个 JSON，包含诊断/分组/manifest，不先在 stdout 打印成功。导入成功而改名失败时，结果保留真实 name/guid/url、`imported:true`、`renamed:false`；分享设置失败会返回 `stage:"share"` 和已导入文件标识。两者都禁止重新上传，应先读取实际状态，再单独处理未完成步骤。

验证失败可能发生在服务端已接受写入之后。保留原命令、构建标识、返回及证据，先只读核对实际内容，再决定恢复步骤；不把新的成功结果覆盖到旧失败记录，也不把追加/删除/发布自动重放当作“重试读取”。

## 浏览器调试链路失败

`login`、`session harvest` 和编辑器动作都通过本机回环 CDP 端口操作 Chrome。命令失败时 CLI 会读 Chrome 自己的 stderr（0600 临时文件，退出即删）判断原因：

| 现象 | 结论 |
|---|---|
| 「浏览器的子进程沙箱无法初始化」 | 当前终端本身处在受限/沙箱环境里，macOS 不允许再嵌套 `sandbox_init`；Chrome 的 GPU 与渲染进程随即退出并带崩浏览器。改用系统终端直接运行，或显式设置 `SHIMO_NET_NO_SANDBOX=1`。 |
| 「浏览器进程异常退出」 | 渲染进程崩溃；错误里带 Chrome 日志关键行，按该行进一步排查。 |
| 识别不出的错误 | 原样返回，不改写成猜测结论。 |

`SHIMO_NET_NO_SANDBOX=1` 会关闭 Chrome 沙箱，只在受限环境里临时使用；日常不设置。`login` 在调试连接连续 6 次失败后会直接带原因退出，不再白等满 10 分钟。
