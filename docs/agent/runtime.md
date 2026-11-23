# 技能包的运行与首次使用

本页是 **Agent 执行说明**。用户从 [一句话接入](README.md) 开始后，下面的检查、下载、调用入口配置和验证均由 Agent 完成，不要求用户打开终端或复制命令。用户只需在确有需要时完成网页登录与宿主授权；连通后直接用自然语言操作。

先按[浏览器规则](../scenarios.md#浏览器优先级)区分展示与操作：文件链接先在内置预览展示；新建、导入或编辑成功后再展示实际结果链接。预览只能展示时，页面操作使用当前可用的外部 Computer Use / Browser Use，复用会话。包内运行器不能接管这些浏览器；纯页面任务跳过下面的 CLI 初始化。需要辅助 API 且未选定 CLI 浏览器方案时，每条命令加 `--no-browser`，包括 `doctor`、`whoami`；该参数不转交页面命令。各浏览器登录态与 CLI 凭据独立。

本页针对从 SkillHub 或 WorkBuddy 导入的完整技能 ZIP；仅有 `SKILL.md` 的本地技能没有下文的包内启动脚本，应按 [安装说明](../install.md) 获取 CLI。

技能 ZIP 包含技能定义、调用脚本和参考资料，完整 `shimo-net` 程序单独获取。Agent 依次选择：复用同版本 CLI → 安装已发布的同版本 npm 包 → 使用固定版本的包内启动器。npm 包内已有完整程序；启动器仅在缺少缓存时从 GitHub Releases 下载。两种方式均使用完整 CLI，访问业务文件仍需连接目标石墨服务。

需要 API 通道时，Agent 要分别确认技能可用、CLI 可执行和账号已连通；不能把 ZIP 导入成功或技能显示在列表中当作全部配置完成。

## 先复用已安装的同版本命令

先检查宿主是否已有 `shimo-net`（Bash/Zsh 用 `command -v shimo-net`，PowerShell 用 `Get-Command shimo-net`），存在时执行 `shimo-net version --json`，与本包 SKILL.md 的 version 比较。版本一致就直接使用该命令，不再绑定下面的函数，也不触发包内 GitHub 下载。官方 npm 渠道安装的命令同样适用；npm 包内已包含程序，安装和首次运行均无需从 GitHub 下载。

缺少命令或版本不匹配时，继续检查下面的 npm 渠道；不可用时再选择包内启动器。不要静默执行版本不匹配的程序，也不要仅因需要某个命令就卸载用户已有版本。npm 安装与可用版本见 [npm 安装指南](../npm.md)；目标版本尚未发布 npm 时不能声称该渠道可用。

## 新环境优先使用已发布的 npm 包

仅当本仓库正式 Release 说明确认 npm 渠道已由维护者发布并验收，才启用本节安装；不能把一个同名 registry 条目的出现当作官方身份。没有匹配版本的命令时，若已有受支持的 Node.js（至少 18）和 npm，再确认官方 registry 已有本技能版本：`npm view shimo-net-cli@<技能版本> version --registry=https://registry.npmjs.org`。将 `<技能版本>` 替换为 SKILL.md 的 version；404 表示该渠道尚不可用，网络超时也不能当作查询成功。

确认版本存在后，由 Agent 在当前获准使用的工作区内安装。Agent 将 `<工作区>` 替换为实际绝对路径、`<技能版本>` 替换为技能声明的版本，再执行命令，不把占位符留给用户填写。Bash/Zsh：

```bash
npm install --prefix "<工作区>/.shimo-net-cli" --engine-strict --ignore-scripts --no-audit --no-fund --registry=https://registry.npmjs.org "shimo-net-cli@<技能版本>"
export PATH="<工作区>/.shimo-net-cli/node_modules/.bin:$PATH"
shimo-net version --json
```

PowerShell：

```powershell
npm install --prefix "<工作区>\.shimo-net-cli" --engine-strict --ignore-scripts --no-audit --no-fund --registry=https://registry.npmjs.org "shimo-net-cli@<技能版本>"
$env:PATH = "<工作区>\.shimo-net-cli\node_modules\.bin;" + $env:PATH
shimo-net version --json
```

每一步检查退出码；安装或版本检查失败时停止该路径并保留错误，不能用下一步输出冒充成功。PATH 只作用于当前终端，Agent 新建命令会话时须自行重新添加该目录，不要求用户每次配置。此渠道依赖 npm registry，不下载 GitHub 二进制；不要执行下面的函数覆盖已成功安装的命令。

没有 npm、Node 版本不满足，或目标版本尚未发布 npm 时，可使用下面的固定版本 GitHub 启动器；报告所选渠道与前提，不自动安装 Node 或改变安全策略。

## 使用包内入口

由 Agent 读取宿主提供的技能路径，把下方 `<技能目录>` 换成实际安装目录的绝对路径；含空格时保留引号，不猜测客户端私有目录。先确认 `scripts/shimo-net.sh` 或 `scripts/shimo-net.ps1` 确实存在。

macOS / Linux 的 Bash 或 Zsh 终端：

```bash
shimo-net() { sh "<技能目录>/scripts/shimo-net.sh" "$@"; }
shimo-net --version
shimo-net schema --json
shimo-net doctor --no-browser --json
shimo-net whoami --no-browser --json
```

Windows PowerShell：

```powershell
function shimo-net { & "<技能目录>\scripts\shimo-net.ps1" @args }
shimo-net --version
shimo-net schema --json
shimo-net doctor --no-browser --json
shimo-net whoami --no-browser --json
```

这个绑定仅在当前终端会话有效；Agent 每次新建 shell 时，须自行先绑定再调用，用户仍然只需描述任务。不要把它写入全局配置，也不要用 `./shimo` 替代。仅有 POSIX sh 的环境可直接调用完整路径的包内 `.sh` 脚本，命令参数与 `shimo-net` 相同。PowerShell 宿主通过独立进程调用时，命令后用 `exit $LASTEXITCODE` 传回真实退出码，不用最后一条成功的展示命令覆盖失败。

macOS/Linux 需要系统 shell、curl 及 shasum 或 sha256sum；Windows 需要 PowerShell 5.1 或更新版本。不需要 Python、Node 或 Go。若组织策略不允许运行脚本，报告限制并由用户/管理员处理，不自动关闭执行策略或安全保护。

每次调用都验证缓存二进制的 SHA-256；只有缺少缓存时才从包内固定版本的官方 Release 下载，并在执行前校验。哈希不符立即失败，不执行也不静默换来源。该机制不会替换已有 PATH 上的 CLI。`SHIMO_NET_RUNTIME_DIR` 可为运行器指定独立缓存位置；它不改变石墨账户配置目录。

## 登录与真实验收

以下检查仅用于确需 CLI/API 的任务；CLI 登录或会话收取还要求已选定且获准使用 CLI 浏览器方案。由 Agent 运行版本、schema、doctor 和 whoami。doctor 报未登录不是安装失败；若 whoami 返回 401，按 [安装与会话](../install.md) 使用上述入口显式执行 `login`。切账号使用 `login --new`，由用户完成浏览器登录，再由 Agent 继续检查；不索取密码或 cookie，不自动改变文件权限。

用户给出已有权限的文件链接后，由 Agent 执行 `info <链接> --no-browser --json`，核对文件名、类型和链接；也可以先响应用户的搜索需求。不需要为安装检查创建或改写业务文件。页面编辑能力仍依赖宿主提供且获准使用的真实浏览器；仅 API 成功不代表页面验收通过。完整命令见 [命令参考](../cli-reference.md)，操作范围见 [场景清单](../scenarios.md)。

首次下载失败应报告网络错误；保留有效缓存。宿主没有本地命令执行工具时，不能宣称 CLI/API 通道可用；需要该通道时应切换到支持本地工具的会话。已有可操作浏览器工具的页面任务及内置预览不受此限制。不能把“代码包已解析”当作完整功能已经可用。

## 自动下载超时时

由 Agent 先确认网络错误，再选择可用下载方式。需要浏览器下载时，从[官方 Releases](https://github.com/shimo-office/office-suites/releases)获取与本包 `SKILL.md` 中 `version` 完全相同的二进制，并由 Agent 放入运行器缓存。不要使用其他版本、第三方镜像或跳过哈希检查；只有当前工具无法完成下载或需要用户授权时才请求用户配合。

| 系统 | 文件名 | 默认缓存目录 |
|---|---|---|
| macOS | `shimo-net_darwin_amd64` 或 `shimo-net_darwin_arm64` | `~/.local/share/shimo-net-cli/runtime/<版本>/` |
| Linux | `shimo-net_linux_amd64` 或 `shimo-net_linux_arm64` | `~/.local/share/shimo-net-cli/runtime/<版本>/` |
| Windows | `shimo-net_windows_amd64.exe` 或 `shimo-net_windows_arm64.exe` | `%LOCALAPPDATA%\shimo-net-cli\runtime\<版本>\` |

macOS/Linux 按当前终端的 `uname -m` 选择：`x86_64` 对应 `amd64`，`arm64`/`aarch64` 对应 `arm64`。Rosetta 终端可能需要 `amd64`。Windows 按启动器读取的 `PROCESSOR_ARCHITEW6432`（未设置时用 `PROCESSOR_ARCHITECTURE`）选择。已设置 `SHIMO_NET_RUNTIME_DIR` 时，目录为该值下的 `<版本>/`；macOS/Linux 仅设置 `XDG_DATA_HOME` 时则为其下的 `shimo-net-cli/runtime/<版本>/`。

保留下载文件的原名；macOS/Linux 为该文件增加用户执行权限（`chmod u+x "<缓存文件的绝对路径>"`）。随后仍通过包内启动器执行 `--version`，它会先验证固定 SHA-256 再运行；不要直接执行尚未验证的下载文件。再按上面的步骤检查 schema、登录和真实文件访问。手动放入缓存只解决下载，不代表账号或业务操作已验收。
