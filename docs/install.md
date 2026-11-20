# 安装、登录与环境

`shimo-net` 原生程序是 Go 单二进制，不需要 Python 或 Node；使用 npm 包的命令入口时需要 Node.js 18+（建议受支持的 LTS）。命令参数见 [命令参考](cli-reference.md)，Agent 接入见 [文档导航](README.md)。

## 环境要求

| 项目 | 要求 |
|---|---|
| 系统 | macOS、Linux、Windows 10/11 |
| 浏览器 | 可从 Chrome / Chromium / Microsoft Edge / Brave / Vivaldi 复用会话；CLI 专用新登录和普通 `open` 的自动发现当前支持 Chrome / Chromium / Edge |
| 构建 | 仅从源码构建时需要 Go 1.26.11+；下载 Release 不需要 Go |
| 网络 | 能访问目标 shimo.net 或客户私有部署域名；npm 安装需访问官方 npm registry，仓库/Release 安装需访问 GitHub |

仅安装 Brave 时可以复用其已登录会话，但 `login --new` 与普通 `open` 所需的浏览器发现尚不覆盖 Brave；新登录需另有 Chrome / Chromium / Edge。不要把会话复用支持误写成所有浏览器启动路径都支持。

安装后使用 `shimo-net doctor --json` 检查当前平台的浏览器发现、配置和会话状态。浏览器存在不代表账号已登录；新安装未登录时 doctor 返回非 0 应按具体检查项处理。浏览器控制选择遵循 [场景清单中的唯一规则](scenarios.md#浏览器优先级)。

## 安装

### 从 npm 安装

`shimo-net-cli@0.9.19` 已发布并通过官方源重新安装验收。已有 Node.js 18+ / npm 时执行：

```bash
npm install --global shimo-net-cli@0.9.19 --registry=https://registry.npmjs.org
shimo-net version
shimo-net schema --json
```

npm 包包含六平台完整程序，不会通过安装脚本再次从 GitHub 下载。无全局目录写权限的安装方式与维护者发布步骤见 [npm 分发说明](npm.md)。

### 从仓库安装

macOS / Linux：

```bash
git clone https://github.com/shimo-office/office-suites.git
cd office-suites
./install.command
```

Windows PowerShell：

```powershell
git clone https://github.com/shimo-office/office-suites.git
cd office-suites
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

有 Go 时安装器构建当前源码，否则下载 GitHub Releases 的最新平台二进制。macOS/Linux 入口为 `~/.local/bin/shimo-net`，Windows 为 `%LOCALAPPDATA%\shimo-net\shimo-net.exe`。安装器检测到 DSH 目录时安装通用技能；WorkBuddy 使用 SkillHub 版技能，见 [对应教程](agent/workbuddy.md)。

### Agent 一键配置

已有 CLI 或宿主已安装技能时直接复用。需要配置新环境时，macOS/Linux 可在仓库内执行：

```bash
bash bootstrap.sh codex  # 也可选择 workbuddy、dsh、doubao
```

脚本会下载发行包、初始化网址、检查登录。它不替代 WorkBuddy 的 SkillHub 技能安装。完整宿主步骤见 [Agent 教程](README.md)。

### 源码构建与本地开发

```bash
go build -trimpath -ldflags "-s -w" -o bin/shimo-net ./cli
export PATH="$PWD/bin:$PATH"
shimo-net version
shimo-net schema --json
```

上面的 PATH 仅用于当前开发终端。`./shimo` 仅为旧开发脚本兼容入口：优先运行 `bin/shimo-net`，缺少二进制才 `go run ./cli`；普通使用与技能示例统一为 `shimo-net`。修改源码后应重新构建，不能用旧二进制代替当前代码验证。安装版本用 `shimo-net version` 检查，不根据目录名猜测。

## 初始化与私有部署

```bash
shimo-net init                          # 默认 https://shimo.net
shimo-net init --origin https://<客户域名>
shimo-net config show
shimo-net config set https://<客户域名>
```

`init`、`config set` 和 `login --origin` 都先探测 `GET /api/v1/auth/register/is_new`，只接受 200 JSON、401 或 403，成功后才保存 origin；失败保留旧配置。HTTP 401 只能证明接口可达，不能证明登录成功。普通登录过期无需重跑 `init`。临时覆盖环境使用：

```bash
SHIMO_NET_ORIGIN=https://<客户域名> shimo-net whoami --json
```

PowerShell 使用 `$env:SHIMO_NET_ORIGIN = 'https://<客户域名>'`。环境变量优先于配置文件，切换域名后需要核对该域名的账号权限。`config clear` 只清理网址配置。

## 登录、复用会话与切账号

```bash
shimo-net login
shimo-net whoami --json
shimo-net session status --json
```

仅在完全没有已存凭据时，CLI 才尝试复用 Chrome / Chromium / Edge / Brave / Vivaldi 的目标域名会话并保存。已有凭据过期、域名不匹配或记录损坏时不会静默换浏览器账号；401 直接失败，需显式登录。普通 login 在没有已存记录时仍可能复用浏览器账号，login --new 总是使用全新临时配置。

手动检查浏览器会话：

```bash
shimo-net session harvest --no-save --json  # 只验证，不保存
shimo-net session harvest --json            # 验证后保存
```

切换账号必须使用新登录流程：

```bash
shimo-net logout
shimo-net login --new
shimo-net whoami --json
```

`logout` 删除 CLI 本地凭据并清理旧 chrome-profile，不会退出日常浏览器或 open-profile 中用户手动登录的账号。因此 `logout` 后运行普通 `login` 可能再次收取同一个旧账号；`login --new` 明确跳过会话复用，登录窗口使用全新临时配置并在结束后清理。设置了 SHIMO_NET_COOKIE 时须先取消环境覆盖才能登录/切账号，否则 CLI 会拒绝，避免实际账号仍被环境变量覆盖。始终用 `whoami` 核对实际账号。

如果没有可复用会话，CLI 会打开专用登录窗口，用户自行完成登录，成功后保存会话。不要向 Agent 提供密码或 cookie。沙箱可能阻止读取系统凭据或浏览器配置，应按宿主最小权限要求放行，不要因“未登录”反复初始化或默认关闭所有沙箱限制。

`--relogin` 仅对只读命令允许用户在全新登录窗口完成登录后重试；所有写命令和固定编辑入口 `phase3` 禁止自动重放。写入已受理但后验失败时，应先读回原文件与检查证据，不能再次提交追加、删除、发布或答卷来“试一次”。

## 本地数据与卸载

| 内容 | 默认位置 |
|---|---|
| origin、凭据元数据、常用位置 | macOS/Linux `~/.shimo-net-cli/`；Windows `%APPDATA%\shimo-net-cli\` |
| 系统凭据 | macOS Keychain / Windows Credential Manager；以 shimo-net-cli 为前缀，按 origin 摘要隔离 |
| 凭据回退 | 系统存储不可用时，`credentials.json` 以 0600 权限保存 cookie |
| CLI 浏览器配置 | open 使用状态目录内 open-profile；登录用临时 login-*；旧 chrome-profile 仅为历史配置 |

凭据优先级、风险与清理边界见 [安全策略](../SECURITY.md)。卸载时先执行 `shimo-net logout`，再删除已安装二进制；是否清理状态目录与已安装技能由用户选择。不要把状态目录或 cookie 提交到仓库。

## FAQ

| 现象 | 处理 |
|---|---|
| 401 / 未登录 | 先看 `session status --json` 与 `whoami --json`；完全没有已存凭据时可自动尝试复用；401/损坏/域名不匹配时不会自动换号，须显式 login。要换账号必须 `login --new`，普通重登可能复用旧账号。 |
| logout 后还是原账号 | 日常浏览器仍登录原账号；执行上面的新登录流程，然后核对 `whoami`。 |
| command not found | macOS/Linux 将 `~/.local/bin` 加入 PATH；Windows 重开终端并核对安装目录是否在用户 PATH。 |
| 找不到浏览器 | 在当前操作系统安装 Chrome / Chromium / Edge，再运行 `doctor --json`；Brave 已登录会话可复用，但不代表专用新登录的自动发现已支持。不要只检查 macOS 的固定应用路径。 |
| 找不到 Go | 使用平台 Release 安装，或安装 Go 1.26.11+ 后从源码构建。 |
| 进入登录页 / 页面空白 | 不能算截图核验成功。先确认目标域名会话与编辑器加载，再按 [浏览器规则](scenarios.md#浏览器优先级) 处理。 |
| 上传后不是在线文档 | `upload` 原样上传云文件；需转换为在线格式时用 `import`。 |
| 无权限 | 如实返回权限错误；用户或管理员在网页授权后再核对，CLI 不代改权限。 |
| 私有部署连不上 | 核对 origin、网络、证书及部署实际能力；网址可达不代表所有编辑能力存在。 |
| 脚本需要解析结果 | 数据命令加 `--json`；失败看 stderr 和退出码，不只看 HTTP 状态。 |
