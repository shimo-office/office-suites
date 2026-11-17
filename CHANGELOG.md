# Changelog

## 0.9.7 — 2026-11-17

### 新增

- `new` 支持新建空白**文稿**（`documentPro`）：`--type docx`、`docpro`、`documentpro`、`word`、`文稿` 均可；`--type doc` 仍然是轻文档（`document`）。
- 文稿创建走 `GET /api/v1/files/create/documentPro`，返回 `/docx/<guid>`；`info` 显示 `type=documentPro`、`typeLabel=文稿`。

### 验证

- `shimo-net new "验证文稿" --type docx --to <文件夹>` 返回 `type: documentPro`、`typeLabel: 文稿`、`url: https://shimo.net/docx/<guid>`；`info` 一致；测试文稿已移入回收站。


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


## 0.9.5 — 2026-11-17

### 修复

- API-backed spreadsheet actions now open the real editor, inject the saved Keychain/Credential Manager cookie, and require a successful visual screenshot before reporting completion.
- A login page or failed screenshot is now reported as a failure instead of being hidden behind an HTTP 200 response.
- Refreshed WorkBuddy guidance and the high-fidelity demo generation workflow.

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

## 0.9.3 — 2026-11-17

- 统一发布资产在 Ubuntu runner 构建，补齐跨平台 release 资产流水线。

## 0.9.1 — 2026-11-11

- 修复 `init` 对 `shimo.net` 的误判：改用无需登录的公开接口探测，不再把正常实例判成官网。
- 修复 `bootstrap.sh` 未先初始化网址，导致 `login` 在无 stdin 环境下等待输入、后台挂起。
- `import` 强制要求 `--to`；`pull` 默认拉取全部分页。
- `doctor` 增加 `checks` / `ok` 与 🟢/🔴 输出，未通过返回非 0。
- `schema` 补 `login` / `logout`；`audit` 增加 `--limit` 与截断提示。
- 文档与实现对齐：Windows 路径/凭据、`GET /files/create`、删除与彻底删除边界、`pull/get --out` 可选。

## 0.9.0 — 2026-11-10

### 新增

- Windows 10/11 支持：Chrome 路径、`%APPDATA%\shimo-net-cli` 状态目录、Windows Credential Manager 凭据存储。
- `install.ps1`：Windows 安装脚本，有 Go 时构建，否则下载 `shimo-net_windows_<arch>.exe`。
- Release 增加 Windows amd64 / arm64 资产；CI 增加 Windows 测试。
- 图文教程入口：`docs/tutorials/`（P0 四款 × 安装授权到操作结果）。

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
