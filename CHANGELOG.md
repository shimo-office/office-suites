# 版本记录

历史版本按版本号从旧到新记录；本地未发布改动单列。版本字符串不等于已发布状态。

## 0.6.0 — 2026-11-10

首个公开仓库版本。

### 新增

- 15 个办公场景：搜索、空间 / 目录、文件出处、导入 + 权限、拉取、新建、改名、删除 / 回收站 / 恢复、权限巡检、批量改权限、读正文、周报、附件下载、上传、常用文件夹别名。
- 单文件 Python 实现（`cli/bin/shimo.py`），零第三方依赖，Python 3.9+。
- Chrome 专用窗口登录，macOS Keychain 保存会话；支持 `SHIMO_NET_COOKIE` 与 0600 文件回退。
- `SHIMO_NET_ORIGIN` 切换客户私有部署域名。
- 全局 `--json`，统一成功 / 失败输出与退出码。
- `schema` / `doctor` / `whoami` / `open` 等辅助命令。
- `@别名` 常用文件夹，任意接受链接 / guid 的参数都可使用。
- 导入与上传分离：`import` 转在线文档，`upload` 原样云文件。
- 删除只进回收站；`purge` 明确拒绝，彻底删除需网页手动操作。
- Agent 技能定义 `cli/SKILL.md`。

### 文档与测试

- 根 README：环境要求、命令总览、私有化、安全设计、开发与限制。
- `cli/README.md`、`cli/安装使用说明.md`、`cli/scenarios.md`。
- `cli/tests/smoke.sh` 无网络冒烟测试；`make test`。
- 安全策略 `SECURITY.md`；仓库忽略规则与编辑器配置。

### 兼容性

- macOS 为主支持平台。
- Linux 支持：新增 Chrome / Chromium 常见路径；Keychain 不可用时回退到 0600 文件。

## 0.7.0 — 2026-11-10

### 新增

- `init`：首次配置产品网址，`GET /api/v1/users/me` 校验可达后写入 `config.json`。
- `config show / set / clear`：查看、修改、清除产品网址；`SHIMO_NET_ORIGIN` 仍可临时覆盖。
- `login --origin <url>`：首次登录可直接指定私有部署域名；无配置时也会提示输入。
- `--relogin`：遇 401 / 未登录时自动打开 Chrome 登录，成功后重试原命令。
- 根 README 重写为面向使用者的版本：30 秒上手、常见任务、Agent 接入、私有部署、排错。
- 终端演示图 `docs/images/demo.png`。

### 变更

- 版本号 0.7.0；`doctor` 增加 `originSource` / `configFile`。
- `schema` 增加 `init` / `config` 命令与 `configFile` 字段。
- 未知命令退出码为 2；`--version` 可用。
- 安装器 Python 版本检查统一为 3.9。

## 0.8.0 — 2026-11-10

### 变更

- CLI 重写为 **Go 单二进制**（Go 1.22+），运行时不再依赖 Python。
- 15 个场景、`init` / `config` / `login --origin` / `--relogin` 行为与 0.7.0 对齐。
- Chrome 登录改为 Go 实现的 CDP WebSocket 客户端；会话仍写 macOS Keychain。
- docx / xlsx / pptx 正文解析改为 Go 标准库 zip + xml。
- 安装器：有 Go 时本地构建，否则下载 release 二进制。
- CI 改为 Go 测试；新增 tag 触发的 macOS / Linux 交叉编译与 release 资产。

### 移除

- Python 实现与 `~/.shimo-net-py` 安装目录（历史版本仍可从 git 记录获取）。

## 0.9.0 — 2026-11-10

### 新增

- Windows 10/11 支持：Chrome 路径、`%APPDATA%\shimo-net-cli` 状态目录、Windows Credential Manager 凭据存储。
- `install.ps1`：Windows 安装脚本，有 Go 时构建，否则下载 `shimo-net_windows_<arch>.exe`。
- Release 增加 Windows amd64 / arm64 资产；CI 增加 Windows 测试。
- 图文教程入口：`docs/tutorials/`（P0 四款 × 安装授权到操作结果）。

## 0.9.1 — 2026-11-11

- 修复 `init` 对 `shimo.net` 的误判：改用无需登录的公开接口探测，不再把正常实例判成官网。
- 修复 `bootstrap.sh` 未先初始化网址，导致 `login` 在无 stdin 环境下等待输入、后台挂起。
- `import` 强制要求 `--to`；`pull` 默认拉取全部分页。
- `doctor` 增加 `checks` / `ok` 与 🟢/🔴 输出，未通过返回非 0。
- `schema` 补 `login` / `logout`；`audit` 增加 `--limit` 与截断提示。
- 文档与实现对齐：Windows 路径/凭据、`GET /files/create`、删除与彻底删除边界、`pull/get --out` 可选。

## 0.9.2 — 2026-11-16

- 按标签 `v0.9.2`（`e63598750e34603fef3d08bf3bf760bf53acbf55`）的提交记录补录。
- 表格格式、合并、区域读取/写入、追加和插删行改走同源 `/lizard-api` 接口。
- 补充 API 目标校验、参数说明及对应测试。这里记录历史变更，不代表当时尚未完善的结果验证已达到当前门槛。

## 0.9.3 — 2026-11-17

- 统一发布资产在 Ubuntu runner 构建，补齐跨平台 release 资产流水线。

## 0.9.4 — 2026-11-17

### 新增

- 扩展到 28 个办公场景、49 项能力；`schema` 同时列出 22 个可执行扩展动作。
- 表格区域读写、追加、插入/删除行、格式/底色、合并和异步导出走已验证的同源 API；不再通过生成本地 XLSX 替换原文件。
- 增加订阅文件扫描与按序号打开、@人/评论列表读取，以及默认浏览器兜底入口。
- 没有稳定 API 的编辑功能统一转 Agent 内联浏览器或 CLI 专用浏览器，由 computer-use 按页面入口操作并返回截图。

### 安全边界

- 文件锁定、限制编辑、高级权限、成员级授权和彻底删除继续禁止 CLI 处理，必须由人工在石墨网页确认和操作。
- CLI 专用浏览器从本机 Keychain 注入登录态；不会输出 sid/cookie。默认浏览器仅作为最后兜底。

### 验证

- `go test ./...`、`go vet ./...`、CLI 构建和 `git diff --check` 通过。
- WorkBuddy 使用本地 `0.9.4` 技能包完成 schema、whoami、表格 API、订阅、导出和浏览器入口回归；结果见 `docs/internal-capability-test-results.md`。

## 0.9.5 — 2026-11-17

### 修复

- 表格 API 动作在当时的版本中会打开真实编辑器、注入已保存会话，并在报告完成前验证页面截图。
- 登录页或截图失败会明确报错，不再以 HTTP 200 掩盖验证失败。
- 更新 WorkBuddy 使用说明与高保真演示生成流程。

## 0.9.6 — 2026-11-17

### 修复

- 登录态不再要求用户二次登录：Keychain 没有会话时，自动从本机 Chrome / Chromium / Edge / Brave 的已登录配置复用 shimo.net 会话，验证后写回 Keychain（写回失败时回退到 0600 文件）。
- 注入浏览器时使用完整 cookie jar（sid 之外的 `smdev-ws-route`、`deviceId` 等一起注入），并保留 HttpOnly/Secure 属性，避免“只注入 sid 仍然未登录”。
- `open --default` 在默认浏览器为 Chromium 内核时，使用 CLI 专用配置启动并注入会话；非 Chromium 浏览器如实提示无法注入。
- 修复 `whoami` 中 `editionName` 等字段的双重编码乱码（例如 `ä¼ä¸šé«˜çº§ç‰ˆ` → `企业高级版`）。
- 修复 Keychain 写入失败后 0600 文件凭据被旧 Keychain 条目遮蔽的问题；现在优先使用实际带有 cookie 的文件凭据。

### 新增

- `shimo-net session status|harvest`：查看登录态来源，或手动从浏览器复用会话；`--no-save` 只验证不写入。
- `shimo-net mkdir <名称> [--to <父文件夹>]`：新建文件夹；能力清单更新为 28 个场景、50 项能力。
- `SHIMO_NET_STATE_DIR` / `SHIMO_NET_KEYCHAIN_SERVICE` 测试隔离变量；`SHIMO_NET_DISABLE_HARVEST=1` 可关闭浏览器复用。

### 验证

- 在 Keychain 无会话、仅 Chrome 已登录的环境下，`whoami` 自动复用 Chrome 会话并返回账号；`open --default` 注入后页面直接进入目标文件夹，不再出现登录页。
- `go test ./...`、`go vet ./...`、CLI 构建通过。

## 0.9.7 — 2026-11-17

### 新增

- `new` 支持新建空白**文稿**（`documentPro`）：`--type docx`、`docpro`、`documentpro`、`word`、`文稿` 均可；`--type doc` 仍然是轻文档（`document`）。
- 文稿创建走 `GET /api/v1/files/create/documentPro`，返回 `/docx/<guid>`；`info` 显示 `type=documentPro`、`typeLabel=文稿`。

### 验证

- `shimo-net new "验证文稿" --type docx --to <文件夹>` 返回 `type: documentPro`、`typeLabel: 文稿`、`url: https://shimo.net/docx/<guid>`；`info` 一致；测试文稿已移入回收站。

## 0.9.8 — 2026-11-17

### 修复

- `phase2` 新增 `--no-verify` / `--no-browser` / `SHIMO_NET_NO_BROWSER=1`：表格 API 写入后跳过 CLI 本地 Chrome 核验，返回 `verified:false`、`verifyWith:"agent-browser"`，由 WorkBuddy 内置浏览器打开 `url` 截图。避免 WorkBuddy 里 Chrome 反复开关、Chrome 意外退出，以及 `RlzStore.plist` 的“删除文件命中询问规则”。
- 带 `--no-verify` 调用页面类动作时明确报错并提示改用内置浏览器，不再启动本地 Chrome。

## 0.9.9 — 2026-11-17

### 变更

- `phase2` 的表格 API 写入默认**不再启动 CLI 本地 Chrome**：返回 `verified:false`、`verifyWith:"host-browser"`，由宿主内置浏览器（WorkBuddy `agent-browser`）打开 `url` 截图。
- 需要 CLI 本地截图时显式加 `--verify`；`--no-verify` 保留为显式跳过，`SHIMO_NET_VERIFY=1` 可全局开启。
- 页面类动作仍需浏览器；没有内置浏览器时不要加 `--no-verify`，由 CLI 本地 Chrome 或 `shimo-net open` 完成。

## 0.9.10 — 2026-11-17

### 新增

- `doctor` 增加 `agentBrowser` 路径信息（非阻塞检查），方便确认宿主的内置浏览器依赖是否安装。
- 技能和 WorkBuddy 教程增加内置浏览器预检：先安装/启用 `agent-browser` 插件和浏览器运行时，用 `agent-browser open` + `screenshot` 验证通过后再执行石墨操作；预检失败先报告缺哪个依赖，不静默退回本地 Chrome。

## 0.9.11 — 2026-11-18

### 修复

- WorkBuddy 的内置浏览器改为通过宿主 `present_files` 工具展示 URL 打开；不再把 `agent-browser` 当作 WorkBuddy 内置浏览器，也不再因此启动本地 Chrome。
- `agent-browser` 预检移到“没有 WorkBuddy 内置浏览器的宿主”；WorkBuddy 无需安装 `agent-browser` 就能使用右侧内置浏览器。

## 0.9.12 — 2026-11-18

### 新增

- 浏览器覆盖扩展到 P0 四款 Agent：WorkBuddy 用 `present_files` 打开右侧内置浏览器；豆包工作、DeepSeek Harness、Codex 没有 Agent 可调用的内置浏览器，分别用 `shimo-net open` 或 `computer-use`，不再把 WorkBuddy 的内置浏览器方案套到其他宿主。
- 新增 [docs/tutorials/browser.md](docs/tutorials/browser.md) 作为四款 Agent 的唯一浏览器矩阵，四篇教程和 `SKILL.md` 都链接到它。

## 0.9.13 — 2026-11-18

### 修复

- 页面动作在编辑器异步挂载时最多重试 **5 次**、每次间隔 2 秒；不再因为第一秒读不到 SDK 就直接报错。
- SkillHub 技能包补齐 WorkBuddy 要求的作者和中英文使用案例元数据，并在执行前检查 CLI、登录态和宿主浏览器，避免沙箱误报成功。
- SDK 实际入口改为 `window.smsdk.__DRIVE_SDK__.editor`（`globalThis.sdk` 在该页面为 undefined）：`history-show/hide`、`save-version`、`comments-show/hide`、`comment-add`、`print` 通过 `editor.actions` 打开对应面板。
- `find`、`replace-all`、`page-width`、`sheet-freeze`、`sheet-sort`、`sheet-filter`、`sheet-validation` 明确标为页面动作，调用时返回可读提示，不再伪装成 SDK 方法。

### 文档

- 场景清单、安全边界、安装说明、技能包与能力矩阵对齐当前实现；修正 49/50 计数、`login --new`、`mkdir`/`docx`、`new` 支持 `table/form/mindmap/flowchart`、SkillHub 提示与 LICENSE。
- 浏览器优先级统一到 [docs/tutorials/browser.md](docs/tutorials/browser.md) 的 P0 矩阵。

## 0.9.14 — 2026-11-18

### 修复

- **假成功**：页面动作（`history-*`、`save-version`、`comments-*`、`comment-add`、`print`）没有 `--verify` 时直接失败；有 `--verify` 时会等待编辑器挂载并校验截图，空白截图返回失败，不再返回 `ok:true` 但没有页面动作。
- `mentions-list` / `comments-list` 继续走页面同源 fetch，但 `--no-verify` 不再误拦截只读 API。
- `sheet-format` 只接受真正支持的属性；`columnWidth`、`numberFormat` 等返回明确“不支持”，不再把入参 key 回显到 `applied`。
- `sheet-update` / `sheet-append` 缺 `range` 或 `resource.values` 时直接报错，不再 204 静默成功。
- `get --format csv` 明确报不支持，不再静默导出 xlsx。
- 页面动作最多重试 5 次、每次 2 秒；编辑器未挂载或截图为空白时不报告成功。

## 0.9.15 — 2026-11-19

### 修复

- `sheet-format` 只保留导出 xlsx 已验证落地的 `background/color/bold/italic/underline`；`fontSize`、`fontFamily`、`align`、`valign` 从白名单移除并明确报不支持，不再出现“白名单内合法假成功”。
- 文档补充内置浏览器会话前提：WorkBuddy 面板打开是登录页时先登录一次，不能让 Agent 在登录页执行页面动作。
- 文档补充表格读回边界：`sheet-values` 看不到插入的空行和 merge 位移，必须导出 xlsx 看 XML 或用截图核验，`applied` 不是物证。

### 三期实现与分发

- 文档迁入 `docs/`，旧入口保留短跳转；安装与发布包改用唯一 Agent 指令源。
- 新增 `editor` 正式入口，保留旧命令兼容；能力目录生成矩阵、场景与计数，避免多个手写数字漂移。
- 修正文档中的底色 API、登录切账号、浏览器顺序、类型清单和人工权限边界。
- 文稿、幻灯片、应用表格与表单实现及真实样本见[阶段验收](docs/phase3-acceptance.md)：27项通过，37.2关联字段经用户确认保留；G1–G5列明样本已通过，不把受控故障注入说成自然服务事件。
- 0.9.6 的历史代码版本存在，但此前缺少远端标签；来源与发布状态见[发布来源核对](docs/internal/release-provenance.md)。补来源标签不等于已生成或发布历史二进制。

### Issue #1 修复

- 修复配置先保存后校验、诊断假成功、导出兜底和导入改名错误；pull 明确当前层范围、记录跳过目录，部分失败非 0 退出。
- 全局 --json 支持命令名前后；未知命令与诊断/批量失败输出单一结构化错误。所有写命令禁止自动登录后重放。
- 保存版本、评论提交和打印改为持久页面工作流；临时面板入口明确拒绝，no-browser 生效。
- 凭据按 origin 隔离，认证失败不静默切号；open 不留调试端口、不自动注入会话。存储和会话边界见 [SECURITY](SECURITY.md)。
- 逐项修复和测试入口见 [Issue #1 修复记录](docs/issue-1-review.md)。

> 历史标识说明：本节 Issue #1 修复对应 e576fb9；此前 f2293db 也自报 0.9.15，不能仅凭版本号确认包含修复，须核对提交/哈希。详见 [发布来源](docs/internal/release-provenance.md)。

### 发布前回归修复与门槛

- 文件列表分页兼容带 `/api/v1` 的同源链接，拒绝跨站、换端口和非列表接口，避免重复拼接接口前缀。
- 页面动作缺少有效 `Runtime.result` 或明确成功标记时失败；无请求体的表格 API 不再发送 `null`。
- 下载先写同目录临时文件，完整完成后再替换目标；下载中断保留原文件。`read --max-chars` 明确拒绝无效值。
- XLSX 正文读取支持内联中文与真实工作表关系，保留 Strict 格式及含图表工作表的合法文件兼容；损坏 XML、外部或缺失关系不再作为成功正文。
- 关联字段菜单在异步重绘时最多等待 5 秒，只重试读取；绑定异常与能力缺失仍拒绝写入。
- 补充文稿、幻灯片、应用表格、表单的生产流程集成回归；隔离浏览器接口只供包内测试替换，默认生产流程保持原有门禁。
- CI 强制运行 JavaScript 回归并检查至少 80% 语句覆盖率；标签发布先通过同一验证任务。门槛不代替线上逐项验收；发行状态以对应GitHub Release为准。

- 最终干净源码回归1,604条测试通过、0失败、79跳过，语句覆盖率81.14%；完整本地回归81.35%。23项工具测试与19项报告测试通过，保留项不能绕过G1–G5。

## 0.9.16 — 2026-11-19

- 下载请求、重定向及流读取失败时不再将临时签名链接带入命令错误、批量清单或验收失败记录；下载失败继续保留已有目标文件。
- 文本界面的文件名、账户、权限与错误等元数据转义换行、终端控制和双向控制字符；正文保留换行/tab，JSON 与下载文件保留原始数据。
- 源码构建最低 Go 版本与 CI/发行工具链升级到 1.26.11；CI 增加固定版本 govulncheck 的可达漏洞检查。
- 发布 Action 固定到核验过的完整提交 SHA，避免可变标签带来的供应链漂移。

## 0.9.17 — 2026-11-19

- 修复 [Issue #2](https://github.com/shimo-office/office-suites/issues/2)：pull 使用独立的隐藏清单文件，结果 manifest 字段返回实际路径，云端 manifest.json 不再被下载清单覆盖。
- get/pull 对同名、清洗后重名和已有本地目标自动加序号；原子预留文件名，下载失败清理本次占位，保留原文件。
- pull 结束前核对成功文件是否存在、为普通文件且大小匹配；核对失败计入 failed 并非零退出，不再仅按下载请求结束宣称全部成功。
- 下载前统一清洗完整文件名（含扩展名），阻止扩展名兜底把冒号等 Windows 特殊路径语法传入文件操作。

## 0.9.18 — 2026-11-20

- 修复 [Issue #3](https://github.com/shimo-office/office-suites/issues/3)：帮助参数优先退出；未知选项按命令校验，不再触发原操作。
- 编辑/创建后验失败保留已执行状态、GUID 和禁止重放提示；格式分步失败列出已受理属性；上传缺少有效对象或 GUID 时拒绝成功。
- 订阅按实际文件计数，目录分页不消耗文件预算；输出 scanned/truncated/partial/complete 及失败明细。验证导出采用严格格式，不再回退 PDF。
- login --origin 先探测后保存；统一浏览器发现和实际名称；Windows 已识别默认 Chromium 使用隔离配置；父域 cookie 给出明确诊断且不放宽主机绑定。
- 统一浏览器授权、三期保留项和凭据文件回退说明，修正旧发布状态；用户命令统一为 shimo-net，./shimo 仅为开发兼容入口。
- 新增自包含 shimo-net-cli npm 产物：六平台原生二进制、SHA-256 校验与命令入口，不含第三方 npm 依赖或安装下载脚本。生成包不等于已发布 npm，实际状态见 [npm 文档](docs/npm.md)。

逐项证据、测试范围与发布状态见 [Issue #3 修复记录](docs/issue-3-review.md)。

## 0.9.19 — 2026-11-20

- 修复 Windows 原生 npm 安装测试中 `.cmd` 路径被重复转义的问题，从带空格的隔离安装目录执行实际生成的命令，保留真实退出码与输出断言。
- 0.9.18 标签已被发布门槛拦截、未生成正式 Release；0.9.19 包含相同的 Issue #3 业务修复与 npm 分发功能，不覆盖旧标签。

## 0.9.20 — 2026-11-20

- CLI 编辑器初始加载在原页面使用最多 300 秒的总预算等待，不再因慢加载自动刷新，也不在启动连接失败后重建窗口；连接中断最多重连同一页面一次，不重放写操作。
- Agent 指令区分内置预览与实际操作：文件链接、新建或导入结果、编辑前后优先内置展示；预览没有操作能力时使用可用的外部 Computer Use / Browser Use，并复用会话。预览成功不代替保存核验，CLI 不控制外部 Agent 工具的窗口生命周期。
- CLI 检查使用 `--no-browser` 避免隐式启动浏览器；统一 Skill、技能包与 npm 包内的浏览器及登录指引。
- 重整中英文入门、安装、场景与命令文档，补充流程图并精简重复入口。

## 0.9.21 — 2026-11-23

- 技能包按市场规范调整为 `shimo-net-cli/` 顶层目录，`scripts/*.sh` 携带可执行位；打包前校验 frontmatter，未加引号的冒号直接失败，避免上传后才报 YAML 解析错误。
- 技能正文改为三步快速开始、常用命令表与执行边界；不再注入打包期内部说明，不再随包携带内部验收记录和 `docs/internal` 链接。
- 许可证明确允许通过 WorkBuddy 技能市场下载、安装、使用，并允许原样转发作安装用途。
- 技能描述包含 `shimo-net-cli` 检索词，便于在市场中按包名搜索。
- 版本号统一为 0.9.21：技能包版本、CLI 版本与启动器钉住的 Release 标签保持一致。

## 未发布

### 发布流程

- `Release` 工作流先校验 tag 是否位于 `main`，避免从过期分支发布旧代码。
- 构建加 `-buildvcs=false`：同一提交在任意 git 状态下产出相同字节，重跑不会改变哈希。
- 发布后自动运行 `scripts/verify_release.py`，把包内钉住的 SHA-256 与 Release 实际资产逐项比对，不一致即失败。
- `action-gh-release` 显式声明 `overwrite_files: true` 与 `fail_on_unmatched_files: true`，重跑时整套替换资产，不留下新旧混合状态。
- 打 tag 时顺带发布 npm 包：已有 `NPM_TOKEN` 则自动 `npm publish` 并回读版本，未配置时给出警告并跳过；版本已存在则直接跳过，可安全重跑。
- 新增 `Mirror` 工作流：推 `main` 或 `v*` 标签时把分支和标签同步到 Gitee 镜像仓库。GitHub 仍是主仓库与唯一产物源，Gitee 只承接代码；未配置 `GITEE_TOKEN` / `GITEE_USER` 时跳过并告警。
