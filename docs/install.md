# 安装、登录与环境

**中文** | [English](en/install.md) · [文档导航](README.md)

本页负责 CLI 的安装、服务地址、登录和环境排错。在 AI 助手中使用时，从 [Agent 接入教程](agent/README.md) 开始，由助手执行配置；以下命令供终端用户和 Agent 查阅。

| 你现在要做什么 | 从哪里开始 |
|---|---|
| 让 Agent 帮我装好并接入技能 | [Agent 接入教程](agent/README.md) |
| 自己安装命令行工具 | [选择安装方式](#安装) |
| 连接公司的私有部署 | [设置服务地址](#初始化与私有部署) |
| 登录或换一个石墨账号 | [登录与切账号](#登录复用会话与切账号) |
| 安装或使用时遇到问题 | [FAQ](#faq) |

`shimo-net` 原生程序不需要 Python 或 Node；使用 npm 包入口时需要 Node.js 18+（建议受支持的 LTS）。安装后的命令统一为 `shimo-net`，完整参数见 [命令参考](cli-reference.md)。

## 环境要求

| 项目 | 要求 |
|---|---|
| 系统 | macOS、Linux、Windows 10/11 |
| 浏览器 | 可从 Chrome / Chromium / Microsoft Edge / Brave / Vivaldi 复用会话；CLI 专用新登录和普通 `open` 的自动发现当前支持 Chrome / Chromium / Edge |
| 构建 | 仅从源码构建时需要 Go 1.26.11+；下载 Release 不需要 Go |
| 网络 | 能访问目标 shimo.net 或客户私有部署域名；npm 安装需访问官方 npm registry，仓库/Release 安装需访问 GitHub |

`shimo-net doctor --json` 检查浏览器、配置和账号状态；未登录时可能返回非 0，按具体检查项处理。仅安装 Brave / Vivaldi 时可复用已有会话，但新登录仍需 Chrome / Chromium / Edge。Agent 使用浏览器时遵循 [浏览器选择规则](scenarios.md#浏览器优先级)。

## 安装

选一种即可，不需要依次执行全部方式。npm 与原生程序负责安装 CLI；在 Agent 中使用时，技能由 Agent 继续配置。

| 方式 | 适用情况 | 完成后得到什么 |
|---|---|---|
| [npm](#从-npm-安装) | 已有 Node.js 18+，支持三种系统 | 可直接调用的 `shimo-net` 命令 |
| [curl 配置脚本](#curl-一键配置) | macOS/Linux，希望同时初始化并检查登录 | CLI、服务地址和登录检查；技能范围见下文 |
| [原生程序](#直接下载原生程序) | 不使用 Node.js 或 Go | 当前系统对应的独立程序 |
| [仓库安装器](#从仓库安装) | 已克隆仓库，或需要本地构建 | 从源码构建或从 Release 下载的程序 |

### 从 npm 安装

```bash
npm install -g shimo-net-cli --registry=https://registry.npmjs.org
shimo-net version
```

包名为 `shimo-net-cli`，命令名为 `shimo-net`。包内包含完整程序，运行时无需从 GitHub 再下载；npm 不安装助手技能。输出版本号后，继续 [登录](#登录复用会话与切账号)。更新、固定版本、项目安装和卸载见 [npm 指南](npm.md)。

### curl 一键配置

macOS/Linux 可从正式 Release 下载脚本并执行；不需要预先安装 Node.js 或 Go：

```bash
curl -fL --retry 2 --connect-timeout 20 \
  https://github.com/shimo-office/office-suites/releases/latest/download/bootstrap.sh \
  -o shimo-net-bootstrap.sh &&
bash shimo-net-bootstrap.sh codex
```

`codex` 可替换为 `workbuddy`、`dsh` 或 `doubao`。脚本下载当前系统的 CLI，初始化服务地址、检查登录，并在需要时打开登录窗口，由你完成登录。

脚本只为 Codex / DSH 写入预设技能目录（`~/.codex/skills/shimo-net-cli/`、`~/.dsh/skills/shimo-net-cli/`），不安装 WorkBuddy / 豆包技能。Agent 还需按 [接入教程](agent/README.md) 核对当前助手实际加载的位置和生效范围；脚本成功不等于技能已加载。

程序安装在 `~/.local/bin/shimo-net`。如果该目录不在 PATH，完成后在当前终端执行：

```bash
export PATH="$HOME/.local/bin:$PATH"
shimo-net version
```

长期使用时将该目录加入终端的 PATH 配置。默认 `latest` 会重新下载程序，不要用反复运行脚本代替状态检查。

### 直接下载原生程序

不使用 Node.js 或 Go 时，从 [GitHub Releases](https://github.com/shimo-office/office-suites/releases/latest) 下载对应文件：

| 系统 | Intel / AMD x64 | ARM64 / Apple Silicon |
|---|---|---|
| macOS | `shimo-net_darwin_amd64` | `shimo-net_darwin_arm64` |
| Linux | `shimo-net_linux_amd64` | `shimo-net_linux_arm64` |
| Windows | `shimo-net_windows_amd64.exe` | `shimo-net_windows_arm64.exe` |

macOS/Linux 可用 `uname -m` 检查当前终端架构：`x86_64` 对应 amd64，`arm64` / `aarch64` 对应 arm64。Windows 在系统“关于”页面查看系统类型。

macOS/Linux：将下载的文件重命名为 `shimo-net`，放到 `~/.local/bin/`（目录不存在时先创建），然后执行：

```bash
chmod u+x "$HOME/.local/bin/shimo-net"
export PATH="$HOME/.local/bin:$PATH"
shimo-net version
```

上面的 PATH 设置只作用于当前终端；长期使用时将该目录加入终端的 PATH 配置。

Windows：将下载的文件重命名为 `shimo-net.exe`，放到 `%LOCALAPPDATA%\shimo-net\`（目录不存在时先创建），把该目录加入用户 PATH，重新打开终端后运行 `shimo-net version`。

升级时下载新版本并替换同一位置的程序，再用 `shimo-net version` 核对。这里下载的是原生程序；技能 ZIP 用于配置 AI 助手。

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

有 Go 时安装器构建当前源码，否则下载 GitHub Releases 的最新平台二进制。macOS/Linux 入口为 `~/.local/bin/shimo-net`，Windows 为 `%LOCALAPPDATA%\shimo-net\shimo-net.exe`。安装器检测到 DSH 目录时安装通用技能；其他宿主的技能由 Agent 按 [接入教程](agent/README.md) 配置。

### Agent 一键配置

从 [Agent 接入教程](agent/README.md) 发起安装；助手应先复用已有 CLI 与技能。已克隆仓库时也可使用本地脚本：

```bash
bash bootstrap.sh codex  # 也可选择 workbuddy、dsh、doubao
```

与 [curl 一键配置](#curl-一键配置) 是同一个流程，无需重复执行。

### 源码构建与本地开发

在仓库根目录使用 Go 1.26.11+ 构建。macOS / Linux：

```bash
go build -trimpath -ldflags "-s -w" -o bin/shimo-net ./cli
export PATH="$PWD/bin:$PATH"
shimo-net version
shimo-net schema --json
```

Windows PowerShell：

```powershell
go build -trimpath -ldflags "-s -w" -o bin/shimo-net.exe ./cli
$env:Path = "$PWD\bin;$env:Path"
shimo-net version
shimo-net schema --json
```

上面的 PATH 仅用于当前开发终端。`./shimo` 仅为旧开发脚本兼容入口：优先运行 `bin/shimo-net`，缺少二进制才 `go run ./cli`；普通使用与技能示例统一为 `shimo-net`。修改源码后应重新构建，不能用旧二进制代替当前代码验证。安装版本用 `shimo-net version` 检查，不根据目录名猜测。

## 初始化与私有部署

公有云使用默认地址；私有部署请将 `<客户域名>` 替换为公司的石墨域名。以下两条 `init` 命令选一条执行：

```bash
shimo-net init --json                          # 默认 https://shimo.net
shimo-net init --origin "https://<客户域名>" --json
```

用 `shimo-net config show --json` 查看地址，用 `shimo-net config set "https://<客户域名>" --json` 修改地址。

`init`、`config set` 和 `login --origin` 都先探测 `GET /api/v1/auth/register/is_new`，只接受 200 JSON、401 或 403，成功后才保存 origin；失败保留旧配置。HTTP 401 只能证明接口可达，不能证明登录成功。普通登录过期无需重跑 `init`。临时覆盖环境使用：

```bash
SHIMO_NET_ORIGIN="https://<客户域名>" shimo-net whoami --json
```

PowerShell 使用 `$env:SHIMO_NET_ORIGIN = 'https://<客户域名>'`。环境变量优先于配置文件，切换域名后需要核对该域名的账号权限。`config clear` 只清理网址配置。

## 登录、复用会话与切账号

已有可用会话时无需重登。首次使用或登录过期时，执行登录并在网页完成验证，再核对当前账号：

```bash
shimo-net login
shimo-net whoami --json
shimo-net session status --json
```

仅在没有已存凭据时，CLI 才自动尝试复用目标域名的浏览器会话。凭据过期、域名不匹配或记录损坏时，不会静默换号；401 需要显式登录。

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

`logout` 删除 CLI 凭据及旧 `chrome-profile`，不会退出日常浏览器或 `open-profile` 的账号。普通 `login` 因而可能复用原账号；`login --new` 跳过复用，使用结束后清理的临时配置。若设置了 `SHIMO_NET_COOKIE`，须先取消该环境变量，否则 CLI 拒绝登录或切账号。

不要向 Agent 提供密码或 cookie。沙箱阻止读取系统凭据或浏览器配置时，仅申请必要权限；登录过期无需重复初始化或关闭全部沙箱限制。

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

先运行 `shimo-net doctor --json`。使用 Agent 时可直接发送错误提示，让它检查并继续原任务。

| 现象 | 处理 |
|---|---|
| 401 / 未登录 | 检查 `session status --json` 和 `whoami --json`，按 [登录流程](#登录复用会话与切账号) 重新登录。 |
| logout 后还是原账号 | 日常浏览器仍登录原账号；执行上面的新登录流程，然后核对 `whoami`。 |
| command not found | 将所选安装方式的命令目录加入 PATH；原生安装路径见上文，npm 安装见 [npm 排错](npm.md#常见问题)。重开终端再试。 |
| 找不到浏览器 | 在当前操作系统安装 Chrome / Chromium / Edge，再运行 `doctor --json`；Brave 已登录会话可复用，但不代表专用新登录的自动发现已支持。不要只检查 macOS 的固定应用路径。 |
| 找不到 Go | 使用平台 Release 安装，或安装 Go 1.26.11+ 后从源码构建。 |
| 进入登录页 / 页面空白 | 不能算截图核验成功。先确认目标域名会话与编辑器加载，再按 [浏览器规则](scenarios.md#浏览器优先级) 处理。 |
| 上传后不是在线文档 | `upload` 原样上传云文件；需转换为在线格式时用 `import`。 |
| 无权限 | 如实返回权限错误；用户或管理员在网页授权后再核对，CLI 不代改权限。 |
| 私有部署连不上 | 核对 origin、网络、证书及部署实际能力；网址可达不代表所有编辑能力存在。 |
| 脚本需要解析结果 | 数据命令加 `--json`；失败看 stderr 和退出码，不只看 HTTP 状态。 |
