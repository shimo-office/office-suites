# shimo-net CLI 完整方法附录

附件 A，配套：[跨 Agent 共享登录态与接入方案](harness-cli-net-integration.md)。本附录由当前源码构建的 `0.9.32 schema --json` 核对，供 Harness 开发侧映射工具。能力目录含 API、页面动作和禁用项，不代表全部能力都能无浏览器执行。

## 命令目录

命令统一形式：`shimo-net <command> [参数] [--json]`。位置、可选参数、payload 和输出限制以[命令参考](../cli-reference.md)、[安装说明](../install.md)及运行版本 schema 为准。下表保留该版本的完整 32 项 schema 目录；另补 `help/schema`。

| 命令 | 参数入口 |
|---|---|
| `init` | `shimo-net init [--origin https://域名] [--login]` |
| `config` | `shimo-net config [show\|set <url>\|clear]` |
| `login` | `shimo-net login [--origin https://域名] [--new]` |
| `logout` | `shimo-net logout` |
| `search` | `shimo-net search <keyword> [--limit 20]` |
| `spaces` | `shimo-net spaces [--limit 300]` |
| `ls` | `shimo-net ls <链接\|guid\|@别名> [--since 7d] [--limit 200]` |
| `info` | `shimo-net info <链接\|guid\|@别名>` |
| `upload` | `shimo-net upload <本地文件> [--to desktop\|空间\|文件夹\|@别名]` |
| `import` | `shimo-net import <本地文件> --to <位置> [--title NAME]` |
| `new` | `shimo-net new <name> [--type doc\|docx\|sheet\|slide\|table\|form\|mindmap\|flowchart] [--to <位置>]` |
| `mkdir` | `shimo-net mkdir <名称> [--to <父文件夹\|@别名>]` |
| `rename` | `shimo-net rename <链接\|guid\|@别名> <新名字>` |
| `share` | `shimo-net share …` |
| `audit` | `shimo-net audit <文件夹\|@别名> [--limit 200]` |
| `delete` | `shimo-net delete <链接\|guid\|@别名>` |
| `trash` | `shimo-net trash` |
| `restore` | `shimo-net restore <guid>` |
| `read` | `shimo-net read <链接\|guid\|@别名> [--max-chars 12000]` |
| `pull` | `shimo-net pull <链接\|@别名> [--out <dir>] [--since 7d] [--limit N]` |
| `get` | `shimo-net get <链接\|guid\|@别名> [--out <dir>]` |
| `fav` | `shimo-net fav <list\|add\|rm\|show> …` |
| `open` | `shimo-net open <链接\|guid\|@别名> [--default]` |
| `editor` | `shimo-net editor <链接\|guid\|@别名> --action <动作> [--payload JSON] [--screenshot PATH] [--verify\|--no-verify]` |
| `phase2` | `shimo-net phase2 …` |
| `purge` | `shimo-net purge <链接>` |
| `version` | `shimo-net version [--json]` |
| `phase3` | `shimo-net phase3 <链接\|guid\|@别名> --action <动作> [--payload JSON] [--read-range 工作表!A1:D4] [--out 目录] [--verify]` |
| `subscriptions` | `shimo-net subscriptions [--limit 200] [--depth 3] [--open 序号\|guid] [--default]` |
| `session` | `shimo-net session <status\|harvest\|import\|watch\|logout> [--no-save] [--no-verify] [--interval 1s] [--once] [--source NAME] [--expected-revision N]` |
| `whoami` | `shimo-net whoami [--json]` |
| `doctor` | `shimo-net doctor [--json]` |
| `help` | `shimo-net help`；`--help` / `-h`，文本帮助 |
| `schema` | `shimo-net schema --json`，机器可读命令及动作目录 |

`version` 另支持 `--version` / `-V`；`phase2` 是 `editor` 的兼容别名。`share/purge` 按设计拒绝执行；不得将它们注册成自动授权或彻底删除工具。

## editor 全部动作

调用：`shimo-net editor <guid或链接> --action <动作> [--payload JSON] [--json]`。

| 分类 | 动作 |
|---|---|
| API 路径 | `sheet-append`, `sheet-delete`, `sheet-format`, `sheet-insert`, `sheet-merge`, `sheet-update`, `sheet-values` |
| 需要浏览器上下文 | `mentions-list`, `comments-list`, `history-show`, `history-hide`, `comments-show`, `comments-hide` |
| 仅页面处理；CLI 不直接执行 | `comment-add`, `find`, `page-width`, `print`, `replace-all`, `save-version`, `sheet-filter`, `sheet-freeze`, `sheet-sort`, `sheet-validation` |

`sheet-values` 读取；`sheet-update/append/insert/delete/format/merge` 写入。`mentions-list/comments-list` 虽是读取，当前实现仍依赖临时浏览器。历史/评论面板 show/hide 只是面板显示，不代表保存版本或提交评论。参数见[编辑器接口](../cli-reference.md#编辑器接口与页面入口)。

## phase3 全部动作

调用：`shimo-net phase3 <guid或链接> --action <动作> [--payload JSON] [--read-range 范围] [--out 证据目录] [--json]`。

| 分类 | 动作 |
|---|---|
| 读取 | `document-layout-inspect`, `table-inspect`, `form-inspect`, `form-draft`, `form-results`, `slide-inspect`, `slide-design-inspect` |
| 页面展示 | `document-goto`, `form-preview`, `form-respondent` |
| 导出 | `slide-print-pdf` |
| 电子表格补强 | `sheet-update`, `sheet-append`, `sheet-insert`, `sheet-delete`, `sheet-format`, `sheet-merge` |
| 文稿编辑 | `document-format`, `document-graphic`, `document-table`, `document-table-style`, `document-mention`, `document-paragraph`, `document-page`, `document-theme`, `document-replace` |
| 幻灯片编辑 | `slide-add`, `slide-duplicate`, `slide-hide`, `slide-delete`, `slide-text-insert`, `slide-text-format`, `slide-placeholder-fill`, `slide-layout`, `slide-replace-all`, `slide-shape-format`, `slide-table-edit`, `slide-title-animate`, `slide-show-settings`, `slide-theme`, `slide-media-insert` |
| 应用表格编辑 | `table-create`, `table-duplicate`, `table-delete`, `table-update`, `table-field-add`, `table-field-update`, `table-field-convert`, `table-field-delete`, `table-record-add`, `table-record-update`, `table-reference-add`, `table-reference-set`, `table-view-create`, `table-view-update`, `table-view-delete`, `table-chart-create`, `table-chart-style` |
| 表单编辑/发布/提交 | `form-edit`, `form-settings`, `form-publish`, `form-submit` |


另有 `shimo-net phase3 artifact <本地文件> --json`，仅检查本地文件包结构，不证明线上写入。

动作 payload 与约束：

- [总览](../phase3-commands.md)
- [文稿](../phase3-document-commands.md)
- [幻灯片](../phase3-slide-commands.md)
- [表单](../phase3-form-commands.md)
- [逐项验收边界](../phase3-acceptance.md)

schema 将 `37.2` 关联字段列为 deferred；动作名存在不等于任意场景都已支持。电子表格补强需 `--read-range`；写操作失败后先读回，禁止盲目重放。

## 接入相关环境变量

| 变量 | 当前作用 |
|---|---|
| `SHIMO_NET_ORIGIN` | 覆盖当前目标网址，优先于 config |
| `SHIMO_NET_COOKIE` | 覆盖当前请求 Cookie，非空时优先于已存凭据；不会自动持久化 |
| `SHIMO_NET_STATE_DIR` | 覆盖配置、凭据元数据、别名、共享会话和浏览器配置所在目录 |
| `SHIMO_NET_SESSION_DIR` | 单独覆盖共享会话目录（默认 `<状态目录>/sessions`） |
| `SHIMO_NET_KEYCHAIN_SERVICE` | 覆盖系统凭据存储 service 前缀 |
| `SHIMO_NET_DISABLE_HARVEST=1` | 禁止从日常 Chromium 浏览器收取会话，不等同于禁止所有浏览器启动 |
| `SHIMO_NET_NO_BROWSER=1` | 禁止 CLI 启动浏览器；对应 `--no-browser` |
| `SHIMO_NET_VERIFY` | 控制部分编辑器验证默认行为，以命令实现为准 |
| `SHIMO_NET_NO_SANDBOX` | 改变 CLI 浏览器启动沙箱；宿主集成不应默认启用 |

## 打包和工具注册

1. 按平台打包原生二进制；锁定版本并用 `version/schema` 核对。npm 包名为 `shimo-net-cli`，可执行名为 `shimo-net`，安装不是登录。
2. 随包提供[权威技能说明](../agent/SKILL.md)，保证与 CLI 版本匹配；本构建 Harness 设置自己的 `DSH_HOME`，不能假设写入用户 `~/.dsh/skills` 就已被宿主识别。通过实际技能加载机制注册并验证。
3. 建议工具映射分成查询、文件写入、编辑器动作和打开页面；不要把任意 shell 字符串当工具参数。登录态通过主文档的适配器取得。
4. CLI 当前没有稳定的逐命令 JSON Schema/payload 类型声明；`schema` 是能力发现目录，适配层仍需按动作校验参数，不要直接把目录当成完整工具参数校验器。
5. 所有返回保留退出码、业务结果、验证状态与产物路径；记录中不包含 Cookie、环境变量快照或私密账号明细。

## 调用协议及浏览器边界

- 数据命令成功通常在 stdout 输出 JSON；失败非 0，在 stderr 输出 {"ok":false,"error":"..."}，可能附带部分结果。退出码 1 为一般失败，2 为未知命令。登录/退出/help 可能仍输出文本。
- 处理超时、取消和有界输出；成功不等于编辑已核验，仍检查 verified/actionVerified 及证据范围。写请求失败后先读回，禁止盲目重放。
- --no-browser 适用于 API 路径；部分 editor 与 phase3 动作依赖临时浏览器，当前没有附着 Harness WebContents 的公开参数。open 使用独立 open-profile，不注入 CLI 会话。
- session 有 status/harvest/import/watch/logout。import 从 stdin 读 cookie 头或带 Domain/Path 属性的 cookies 数组，缺 sid 拒绝写入；记录已存在时，不带 --expected-revision 只允许同一 sid 的属性刷新，切号与退出后恢复必须带当前 revision。watch 轮询 revision，输出不含 cookie 值。status 不证明在线有效，需用 whoami。
- 共享会话的 cookie 按实际请求地址过滤（Domain、Path 前缀、Secure 只发 https、Expires）；sid 不适用时命令直接失败，不回退钥匙串或环境变量。
- 响应 Set-Cookie 会合并回共享会话（source: renew），续期对其他端可见。没有常驻进程，也没有从 CLI 到宿主的主动回调；宿主靠 watch 或读文件感知变化。
- 正式共享模式必须让所有 Agent 与 Harness 使用同一个状态目录；不得用独立状态目录、常驻注入 SHIMO_NET_COOKIE 或单向注入代替同步。SHIMO_NET_COOKIE 优先于共享会话，常驻注入会让共享退出失效。石墨办公关闭后会话文件仍可供 CLI 使用。
