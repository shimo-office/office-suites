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

## 0.9.22 — 2026-11-23

### 技能包

- 移除包内启动器 `scripts/shimo-net.sh` / `shimo-net.ps1`。它会在安装后从 GitHub Releases 下载二进制并执行，既触发市场安全扫描，在离线评测沙箱里也必然失败；程序改由 npm 包提供，包内已自带各平台程序。
- 新增 `scripts/selfcheck.py`：纯标准库、不联网、不读取凭据，输出环境状态与下一步命令，让首次运行拿到确定结果而不是 `command not found`。
- `SKILL.md` 重写：能力边界（能做/不做）、可离线执行的自检步骤、一次完整任务示例、失败分级处理（401/403/锁定/超时分别怎么办）、数据真实性约定。
- 不再要求程序版本与技能版本严格相等，改为最低版本要求；避免技能版本递增时把安装指令变成死链。

### 发布流程


- `Release` 工作流先校验 tag 是否位于 `main`，避免从过期分支发布旧代码。
- 构建加 `-buildvcs=false`：同一提交在任意 git 状态下产出相同字节，重跑不会改变哈希。
- 发布后自动运行 `scripts/verify_release.py`，把本地构建产物与 Release 实际资产的 SHA-256 逐项比对，缺项或不一致即失败。
- `action-gh-release` 显式声明 `overwrite_files: true` 与 `fail_on_unmatched_files: true`，重跑时整套替换资产，不留下新旧混合状态。
- 打 tag 时顺带发布 npm 包：已有 `NPM_TOKEN` 则自动 `npm publish` 并回读版本，未配置时给出警告并跳过；版本已存在则直接跳过，可安全重跑。
- 新增 `Mirror` 工作流：推 `main` 或 `v*` 标签时把分支和标签同步到 Gitee 镜像仓库。GitHub 仍是主仓库与唯一产物源，Gitee 只承接代码；未配置 `GITEE_TOKEN` / `GITEE_USER` 时跳过并告警。
- 镜像的 `main` 多一个提交：`scripts/render_mirror_readme.py` 把 README 中的仓库链接改指 Gitee（版本与 CI 徽标保留图片、去掉外链，Issue 与文档链接指向镜像）。
- 二进制下载地址仍指向 GitHub Releases，因为发布资产只在 GitHub；本地提交的 README 仍保留 GitHub 地址，改写只发生在镜像侧。
- Gitee 镜像地址统一为 `gitee.com/shimo-office/office-suites`；旧 `graphite-office` 路径会 302 重定向，镜像不再写入这个跳转地址。

## 0.9.23 — 2026-11-23

### 版本

- CLI、技能包与发布流水线统一为 0.9.23。该版本的 npm 包没有发出：发布命令的 tarball 参数被 npm 当成 GitHub 简写，详见 0.9.24。

### 发布流程

- npm 发布改用 Trusted Publishing（OIDC）：`release` job 声明 `id-token: write`，由 npm 校验“仓库 + workflow 文件”身份后签发一次性发布凭据，仓库不再依赖长期 `NPM_TOKEN`。NPM Token 只保留为迁移期兜底。
- 发布步骤改用 Node.js 24，并在 npm 版本低于 11.5.1 时升级，因为 OIDC 发布会话要求该版本；`package-manager-cache` 关闭，避免发布构建命中缓存。
- npm 发布失败不再中断整个发布：此时 Release 资产已上传并自校验通过，失败原因写入 run summary 并给出可信发布者登记字段；`-buildvcs=false` 保证重跑产物字节一致，补发不会改变已上传资产的哈希。

## 0.9.24 — 2026-11-23

### 版本

- npm 上首个由 OIDC 自动发布的版本，同时作为提审市场的版本；CLI、技能包与 npm 包统一为 0.9.24。

### 发布流程

- 修正 `npm publish` 的 tarball 参数：`dist/shimo-net-cli-<版本>.tgz` 会被 npm 解析成 GitHub 简写 `owner/repo` 并去 `git ls-remote`，报 `Permission denied (publickey)`；加上 `./` 前缀后才按本地文件发布。旧写法即使配置了 token 也发不出去。
- 修正 Gitee 镜像在标签触发时的分支清理：`git push --prune` 在只检出标签的运行里没有本地分支，会把镜像的默认分支 `main` 当成“多余分支”删除，被 Gitee 拒绝。改为按 GitHub 与镜像各自的分支列表做差集，跳过默认分支；标签清理同样改成两侧 `ls-remote` 比对。
- npm 上传成功后立刻回读版本会撞上 registry 传播延迟（`E404 No match found for version`），把发布步骤误判为失败；改为最多轮询 3 分钟，超时只告警不失败。

## 0.9.25 — 2026-11-23

针对市场沙箱评测（49.3 分 / 低质量）逐条修正，CLI 能力未变，改动都在技能侧：
没有任何命令被删除或改写，联网、登录后的完整链路与 0.9.24 完全一致。

### 技能包

- `SKILL.md` 重写流程骨架：先判定「有账号 / 无凭据」，再决定走哪条分支。登录不再是每个任务的硬前置，避免评测里一上来就卡在 `whoami` 失败。
- 新增「铁律：证据先于结论」：每条结论要带命令与原始输出；文中 JSON、路径、版本号一律标注为结构示例，禁止当成本机输出转述；不补全、不推测链接与路径。
- 自检脚本降级为「线索」：明确它只读本地文件、不发请求，`credentials` 只是按凭据文件存在推断，必须用 `command -v` / `version --json` / `doctor --json` 复核，矛盾时以实际命令为准。
- 新增无凭据交付路径：可交付环境报告、写入计划、解锁命令；命中清单、正文摘要、表格写入一律写「未执行」并说明原因，不再只回一句阻塞说明。
- 登录加硬门：打开浏览器前必须先征得用户同意；401 后只重试一次即停，并禁止 `session harvest`、翻浏览器配置、探测端口等绕过动作。
- 新增 `scripts/plan.py`：纯标准库、零网络，把本地 CSV 转成 `sheet-update` payload（`payload.json`）与执行脚本（`run.sh`），校验工作表名、range 语法、行列数与单元格长度，不合法直接报错。产物明确标注「未写入」，写入仍需账号与链接。

### 修正

- `selfcheck.py` 的 `ready` 忽略了版本过低：装的是 0.9.19 时仍返回 `true`，会让 Agent 把过期环境当可用。现在版本低于最低要求即 `ready: false`，`next.action` 为 `upgrade`。
- `selfcheck.py` 增加 `notProof` 与 `verifiedBy` 字段，自报「这只是本地推断」并给出复核命令。
- 打包清单纳入 `scripts/plan.py`，并保持「零可执行脚本」：包内只有 `.py`，没有 `.sh`/`.ps1`/二进制。

## 0.9.26 — 2026-11-24

### 命名与检索

- 中文显示名由「石墨办公 CLI」改为「石墨办公套件私有部署 CLI」，与需求文档里的正式名称一致；英文显示名同步为 `Shimo Office Suite Private Deployment CLI`。
- 描述字段补入包名 `shimo-net-cli` 与命令名 `shimo-net`，并说明公有云 `shimo.net` 与客户私有部署共用同一套命令，市场里搜中文名或 `shimo-net-cli` 都能命中。

## 0.9.27 — 2026-11-24

按 #4、#5 两条复核 issue 修正技能包文档，CLI 行为与命令集不变。

### 文档与代码对齐（#4）

- 删掉「退出码 401 / 403」的判据：CLI 只用 0/1/2，401/403 是 HTTP 状态。未登录、无权限、被锁定改为按 `ok:false` 加 `error` 文本判断。
- 修正 `doctor` 的读法：`checks` 是数组（`{"name":"credentials","ok":false,...}`），要看 `name` 为 `credentials` 那一项的 `ok`；任一项不过即退出 1，且 JSON 只写 stderr、stdout 为空，非 0 不代表环境损坏。
- 判定表补第三行「`shimo-net` 不可用（装不上）」；第 1 步命令补 `shimo-net whoami --json --no-browser`，并统一 `2>&1` 收集 stderr。
- 超时口径由 15 秒改为 90 秒，与 `cli/api.go` 的 HTTP 客户端超时一致。
- 登录说明改为「Chrome / Chromium / Edge 专用窗口」，不再写成系统默认浏览器；`doctor` 的 `chrome` 项为 `ok:false` 时不征求同意、不尝试登录，直接转第 3 步。
- 恢复 python3 兜底：没有 `python3` 时跳过自检与 `plan.py`，按实际命令输出判定；写入计划改为手写 payload 并标注「未经 `plan.py` 校验」。

### 示例（#5）

- `examples_zh` / `examples_en` 调整为 2 条离线可完成（环境自检、CSV 转写入计划）+ 1 条需登录（搜索文件）；原来 3 条里有 2 条必须有账号，评测按示例出题时任务完成度必然为零。
- 需要账号的示例写清「登录后」前提，写入计划示例写明「课表 CSV 我随后贴给你」，不再依赖评测输入里不存在的附件或链接。

## 0.9.28 — 2026-11-25

同一系统用户的 CLI 与石墨办公共用一份 net 会话；原有单机行为、命令集与输出格式保持不变。

### 共享会话

- 新增共享会话存储 `<状态目录>/sessions/<sha256(canonicalOrigin)>.json`（`0600`），字段含 `authState`、账号、带 Domain/Path/Expires 的 cookie、`revision`；文件锁串行化读改写，写入按 `revision` 做乐观锁。
- 鉴权顺序改为 `SHIMO_NET_COOKIE` > 共享会话 > 钥匙串/`credentials.json`。共享记录不存在时行为与 0.9.27 完全一致；记录为 `signedOut` 时直接拒绝，不静默回退旧凭据。
- `login`、`session harvest` 成功后把会话镜像到共享存储；`logout` 先写退出标记再清本地凭据。
- 响应里的 `Set-Cookie` 合并回共享会话（`source: renew`），续期对其他 CLI 可见。

### 新增子命令

- `session import [--no-verify] [--source NAME] [--expected-revision N]`：从 stdin 读 cookie 头或 `{"cookie":"..."}`，默认用 `/users/me` 校验，缺 `sid` 拒绝写入。
- `session watch [--interval 1s] [--once]`：revision 变化时输出一行元数据快照，不含 cookie 值。每轮只 stat 会话文件（约 3.4 µs），时间戳与大小未变不读取内容；每 30 轮兜底全量读一次，避免粗精度文件系统漏检。
- `session logout`：只写共享退出标记；`logout` 仍然清理本地凭据。
- `session status --json` 增加 `shared`、`authState`、`account`、`revision`、`cookieNames`、`store` 等字段。
- 新增 `SHIMO_NET_SESSION_DIR` 覆盖共享会话目录。

### 兼容

- 不读共享会话的旧版本不受影响；新版本在没有共享记录时同样按旧逻辑工作。
- 共享会话只保存请求所需 cookie，不保存浏览器 Local/Session Storage。

## 0.9.29 — 2026-11-25

修复共享会话在退出可靠性、并发优先级与 Cookie 属性上的三个问题。0.9.28 的接入文档在这三点上表述过强，以本版行为为准。

### 退出可靠性

- `logout` 不再忽略共享退出标记的写入结果：写入失败时返回错误，明确「本机凭据已清除，但共享退出未写入」，避免本机已退出而其他 Agent 继续使用旧会话。
- `login` 与 `session harvest` 镜像共享会话失败时输出提示，不再静默降级。

### 并发与退出优先级

- 共享记录为 `signedOut` 时，不带 `--expected-revision` 的 `session import` 被拒绝，防止旧会话把已完成的退出改回 `signedIn`。
- 带当前 revision 的导入视为一次明确的新登录，允许覆盖；`Set-Cookie` 续期合并仍不允许复活已退出的记录。

### Cookie 属性

- `session import` 支持 `{"cookies":[{...}]}` 输入，保留 Domain / Path / Secure / HttpOnly / SameSite / Expires；原有 `{"cookie":"..."}` 与纯 cookie 头仍然兼容。

## 0.9.30 — 2026-11-25

修复共享会话在 cookie 发送范围与写入前提上的两个问题。0.9.29 及更早版本会把整份 cookie 记录发给任意路径。

### Cookie 发送范围

- 共享会话的 cookie 改按实际请求地址筛选：Domain、Path 前缀（RFC 6265）、Secure（只发 https）、Expires。此前只检查 Domain 和过期时间，`Path=/admin` 的 cookie 会被发到 `/api/v1`，私有部署走 http 时 Secure cookie 也会明文发出。
- 记录存在但没有适用 cookie 时直接报错，不回退钥匙串或 `credentials.json`——那可能是另一个账号的旧凭据。
- `session import` 的默认校验也按 `/users/me` 的真实地址计算，属性不适用在导入时就失败。

### 写入前提

- 已有 `signedIn` 记录时，不带 `--expected-revision` 的 `session import` 只允许同一 `sid` 的属性刷新；`sid` 不同视为切号，必须带当前 revision。此前可以直接覆盖，并发切号会互相盖掉。
- `login` / `session harvest` 在开始前记下 revision，成功后以它为前提写入。登录期间有人退出或切号，这次镜像会冲突并提示，不会把对方的状态盖掉。
- 首次写入也带版本前提（revision 0），两个进程同时初始化时后一个会撞版本而不是覆盖。
- `Set-Cookie` 续期合并遇冲突会重读重试一次；仍冲突则放弃本次合并，不影响请求本身。

## 0.9.31 — 2026-11-25

修复浏览器调试链路失败时的可诊断性。行为不变，只改失败路径的结论与退出时机。

### 失败原因可见

- `login`、`session harvest` 与 Phase 3 编辑器启动时，Chrome 的 stdout/stderr 落到 0600 临时文件（退出即删）。此前直接丢弃，用户只能看到 `EOF`。
- CDP 失败时读日志判别原因：子进程沙箱无法初始化（`sandbox initialization failed`、GPU 进程退出并带崩浏览器）会给出「受限终端 / `SHIMO_NET_NO_SANDBOX=1`」的可执行结论；渲染进程崩溃会带上日志关键行。识别不出的错误原样返回，不改写。
- `login` 轮询期间连续 6 次 CDP 失败即带原因退出。此前浏览器已经退出，命令仍会白等满 10 分钟。

### 兼容性

- 命令、参数、退出码、成功路径输出均未改动；`SHIMO_NET_NO_SANDBOX=1` 仍是显式开关，不默认降级浏览器沙箱。

## 0.9.32 — 2026-11-25

轻文档 `/docs/` 的编辑动作补齐。此前只支持新建、导入、读取、改名、删除；字符格式、段落格式、查找替换、表格、图片只能人工在页面上点。轻文档没有对外编辑器 SDK（`window._sdk` 为 `undefined`），这五个动作走真实页面的菜单与粘贴通道，命令名与文稿的 `document-*` 分开为 `doc-*`。

### 新增动作

- `phase3 <轻文档> --action doc-format`：加粗、斜体、下划线、删除线、字号、字体、字色、高亮。按段落序号 + 文本 + 字符区间定位，改前改后各取一次 `hk-font-*` 标记值，刷新页面后复验；中文字体名走别名表（宋体 → `simsun`）。
- `phase3 <轻文档> --action doc-paragraph`：标题级别（0–6 / title / subtitle）、对齐、列表、缩进、行距。行距在 DOM 上是换算值（选 1.5 倍落成 `line-height: 2.5`），因此只核对「发生变化 + 刷新后保持一致」，不拿数值直接比。
- `phase3 <轻文档> --action doc-replace`：页面查找替换面板，真实点击按钮、原生 setter 填输入框。结果以替换前后导出全文的纯文本命中计数为证（`beforeOccurrences` / `afterOccurrences`），导出带强调标记会切断关键字，计数前去标记。
- `phase3 <轻文档> --action doc-table`：按 `rows`/`cells` 生成表格 HTML，走页面粘贴通道插入。以单元格 `data-cell-guid` 集合做差判定新增表格，核对行数与单元格文本，刷新后复验。
- `phase3 <轻文档> --action doc-image`：本地图片转 base64 后走粘贴通道插入，支持 png/jpg/jpeg/gif/webp，上限 4 MB。以行内图片节点计数判定，刷新后复验，并要求导出物含 `data:image/`。

### 页面通道

- 就绪判定、协作名单门禁、段落定位、标记断言、语义控件定位、真实点击与按键、刷新复验集中在 `cli/phase3_lightdoc.go`。
- 鼠标点击必须带 `buttons`，否则页面不认；浮层选项先单独取坐标再点，一步内取坐标会点到相邻项。
- 刷新用 `Page.navigate` 而非 `Page.reload`：后者替换 target，后续 CDP 调用直接 `i/o timeout`。

### 证据

每个动作在请求目录留 `request.json`、`before.png`、`submission.json`、`marks.json`、`marks-reloaded.json`、`reload.png`、`result.json`；失败额外留 `failure.json`。`verified` 只在刷新后复验通过时为 `true`。

### 边界

- 单元格合并、拆分、列宽，图片裁剪与环绕方式，整篇重排与页面宽度仍是人工页面操作。
- 场景 20（评论与提及）、21（预览与打印）未命令化，仍是页面操作。

## 0.9.33 — 2026-11-25

轻文档补全文样式：场景 22 的「全文标题与样式」里，样式部分现在可以直接下发。入口是 `doc-format` 加 `scope":"all"`，不带段落参数。

### 全文样式

- `phase3 <轻文档> --action doc-format --payload '{"scope":"all","attrs":{...}}'`：字体、字号、颜色、高亮、粗体、斜体、下划线、删除线作用于整篇正文。
- 分两类下发，因为页面行为不同：设值类（字体/字号/颜色/高亮）值是绝对的，整篇全选一次下发即可；勾选类（粗/斜/下划线/删除线）必须逐段选中再切换。整篇全选时标题也落在选区里，而标题的粗体来自级别样式，工具栏看到混合态，一次点击会把「取消加粗」执行成「继续加粗」。
- 标题段落不参与行内标记（H1–H6 的文字样式由级别决定，页面不接受行内字号/字体）。断言按「可承载行内标记的正文段落」计数并要求 100% 覆盖，标题的文本、数量单独写进 `marks.json` 的 `headingSkipped`/`headingSamples`，不做隐藏。

### 边界收窄

- `color`/`highlight` 不再接受 `NONE`。轻文档色板只有具体色值，没有「无」格，之前声明清除却改不动，属于假成功；现在直接拒绝并说明清除需人工在页面上操作。
- `verticalAlign`（上下标）在轻文档上没有工具栏入口，之前是收下参数、启动浏览器、点一圈再报错；现在参数层直接拒绝并指向文稿的 `document-format`。
- 全文改写中途某段失败即停，结果里写明停在第几段、已完成几段。前几段已经改过，因此不能当成整体失败重放。

### 验收

在实测文档上跑通：`{"scope":"all","attrs":{"bold":true,"fontSize":20,"color":"#BE1A1D","fontFamily":"黑体"}}` 四项覆盖 3/3 正文段落、标题 3 段单列，刷新后一致（`verified: true`）；`{"bold":false}` 清除同样 3/3。区间模式回归通过。

### 测试

轻文档页面动作此前只有参数校验用例，新增代码把整体语句覆盖率拉到 77%，低于 CI 的 80% 门槛，0.9.33 的发布流水线因此停在 verify 阶段。

补了两套离线测试，不依赖登录态与浏览器窗口：

- 假 CDP 端点（`net.Pipe` 承载 `WSConn`）驱动定位、标记断言、控件点击与按键、段落形态读取、表格与图片计数，验证的是真实的定位与断言逻辑。
- 本地 HTTP + 假登录态跑完五个动作的命令主流程：导出前后内容、刷新复验、截图落盘、`result.json` 与 `verified`。截图用全黑图，因为验收要求「非空白」。

整体覆盖率回到 81.6%（14749/18067 语句）。

## 0.9.34 — 2026-11-25

修复 0.9.33 的 Windows 发布测试失败，保留轻文档编辑与全文样式等既有业务能力。

- 图片测试路径通过 JSON 编码生成，正确处理 Windows 反斜杠；CI 失败时保留测试日志和具体失败用例。
- Cookie 续期绑定发起请求时的共享会话版本，切号或退出后到达的旧响应不能覆盖新会话。
- 轻文档选区把字符索引转换为浏览器使用的 UTF-16 偏移，避免 emoji 等字符导致定位错误。
- 全文样式在全选后的设值前、每段格式写入前重新检查协作者；检查失败即停止，并保留已写入段数，禁止自动重放部分写入。
- npm 发布失败或新版本在等待期内不可见时，发布任务明确失败，避免 GitHub 有附件但 npm 未发布仍显示成功。

验证：修复提交的 Windows/Linux 全量测试、80% 覆盖率门禁和 CodeQL 检查均通过；三项会话及浏览器日志的 P2 问题在独立 PR 跟进。

## 0.9.35 — 2026-11-25

完成独立 PR #7 的三项 P2 修复，保留现有文件管理、轻文档与各类编辑动作。

- 共享会话文件损坏或不可读时直接报错，不再回退本地旧账号；写入也拒绝把损坏记录当成 revision 0 覆盖。仅记录不存在时保留旧版回退行为。
- 登录、会话采集（session harvest）和编辑器退出后正确关闭浏览器日志句柄，并在读取失败诊断后删除临时日志；启动失败路径同样清理。
- 响应 Cookie 没有合法 Path 时使用实际请求目录，保留编码路径与同名 Cookie 的删除范围，避免凭据作用范围扩大。

验证：Windows/Linux 全量测试及 CodeQL 全部通过；本地 Go 覆盖率 81.59%，76 项 Python 测试、静态检查、构建和文档检查通过。损坏会话文件需要检查处理，不提供静默覆盖恢复；显式环境变量覆盖语义保持不变。

## 0.9.36 — 2026-11-26

- `share` 恢复为可执行：支持 7 档分享权限（private / readonly / commentable / editable / enterprise_*），写入后重新读取 shareMode 验收；`import --share` 同步生效。
- `get --format` 按文件类型导出：文档 md/docx/pdf/jpg（`--embed-images false` 关闭内嵌图片）、文稿 docx/wps/pdf、表格 xlsx、幻灯片 pptx/pdf；不支持的组合在请求前拒绝。
- `pull` 把服务端「暂不支持该导出类型」（3613）单独计数，不再让整个批次以失败退出。
- 新增 `preview`：输出固定 `/files/<guid>` 预览地址。
- `editor` 的 history-show / comments-show 改调套件根级 SDK 并等待面板渲染；新增 `save-version`（createRevision）与 `print`（sdk.print）动作。
- 新增轻文档 `doc-page-width`（标准/加宽/自适应），以纸张容器 `page-width-*` 类验收。
- 修正误报：`doc-image` 导出引用异步滞后不再判失败；`document-paragraph` 非目标段字符属性快照差异降级；`document-graphic` 仅在正文变化时失败，版式重排记为告警；幻灯片主题允许对象层级重排。

## 0.9.37 — 2026-11-26

- 修正套件前端 SDK 根：产品页实际门面是 `window.__SM__.editor`，此前只探测 `_sdk` / `smsdk.__DRIVE_SDK__` / `globalThis.sdk`，导致 history / comments / print 落到旧的 `actions.showViewHistory` 假成功。`history-show` / `comments-show` / `save-version` / `print` 现直接调用根级 facade，文档与表格均实测通过。
- 动作前等待编辑器真正渲染：`editor.initialized`、`sdk._rendered` 或首帧 canvas 就绪后再调用，并在就绪后留一帧稳定时间；面板类动作再用轮询确认面板出现，解决"接口调用成功但侧栏不渲染"。
- 新增 `cell-history`：在表格画布上发真实右键事件，深度遍历 shadow DOM 找到「查看单元格历史」并点击，面板可见才算成功。
- 页面动作默认保留浏览器窗口，方便直接查看面板；批量验收用 `--close` 关闭。返回增加 `browserKept` / `browserProfile`。
- 浏览器降级启动时加 `--test-type`，去掉 `--no-sandbox` 的黄色警告条，避免验收截图被无关提示干扰。
- `comment-add` 仍为页面操作：评论输入区由画布绘制，无法稳定定位输入焦点，自动提交会误写单元格，因此不提供命令。


## 0.9.38 — 2026-11-26

### 修复：写入成功却判失败

- 文稿 `document-page` 写页边距不再重置页眉/页脚距离：显式沿用原 `pgMar.header/footer`，被编辑器改写时自动恢复一次；导出与重开均核对。页面设置重写会显式写出 `cols.sep` 等默认值，验收改按等效值判定。
- 文稿 `document-replace` 继承被替换字符的字体、字号、颜色与行内标记：编辑器粘贴为纯文本，写入后按原 `rPr` 回填并逐字核对。带格式段落的替换不再丢失格式。
- 文稿 `document-mention` 回读兼容提及节点嵌套：提及节点常被包在超链接外壳内，改为按模型逐层遍历定位 `atType=1` 节点，不再报“提及段落模型不可读取”。
- 所有文稿布局动作验收失败时回显 `diff`（属性 → 旧值/新值）并写 `failure-diff.json`，不再只给一句“验收失败”。
- 幻灯片 `slide-theme` 验收失败定位到具体页与对象（文字、样式、几何、备注、页面尺寸）。

### 修复：应用表格

- `table-field-add` / `table-record-add` 的 `viewTargetGuids` 只提交 Grid 视图：表里同时存在看板视图时不再返回 HTTP 400（此前删掉看板才能写入）。
- SDK 非 2xx 响应回显服务端 `errorCode`/`error`/响应正文，不再只有一句“SDK 请求失败（HTTP 400）”。
- 表内没有任何 Grid 视图（只有看板/日历）时也能执行写动作：就绪判定接受任一原生视图渲染，不再空等 5 分钟。
- 视图展示验收按“可见记录子集”核对：每条已渲染记录都必须与独立 API 一致，长表格虚拟化未挂载的行不再判失败，结果用 `virtualized` 与记录数标注。
- `table-reference-add` 在字段菜单被重绘时重新悬停（最多 5 次），仍失败才报错。

### 修复：登录态与电子表格

- 就绪探测新增快速失败：页面加载完成仍无编辑器 SDK 且协作名单未就绪时，30 秒判定登录态失效并提示 `shimo-net login`；已收到协作名单的页面不受影响。
- `read` 的表格标题改用工作簿真实工作表名（此前固定输出 `Sheet 1`，与 API 名不一致，照抄会 404）。
- `schema` 收录 7 个电子表格动作（`sheet-values/update/append/insert/delete/format/merge`）及 payload 契约，`schema --action sheet-update` 可直接查。

### 其他

- `phase3` 未指定 `--out` 时自动创建受管证据目录并在结束输出路径；超过 7 天或 256 MB 的历史证据自动清理，`--out` 指定的交付物不受影响。

## 未发布

- 新增 `shimo-net update [--check] [--json]`：按 npm / 原生安装方式更新当前 CLI，只升级到最新稳定版本，不降级。
- 原生更新使用 GitHub Release SHA-256 校验及版本运行检查；下载或验证失败保留旧程序。npm 安装更新整个包，保留全局或项目安装位置。
- 更新不修改石墨登录、服务地址和助手技能；旧版 CLI 需要先按原安装方式升级一次才能使用新命令。

- `editor --action mentions-list/comments-list` 改为直接 HTTP 读取页面签名与 API，不启动浏览器；轻文档输出提及 `#atGuid` 和评论 `#selectionGuid` 定位链接。JSON 保留原始结果并增加链接与计数，同一评论位置去重；缺少定位信息时明确提示。
