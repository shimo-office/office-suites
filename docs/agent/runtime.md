# 运行环境与首次使用

本页是 **Agent 执行说明**。用户从 [一句话接入](README.md) 开始后，下面的检查、安装、调用入口配置和验证均由 Agent 完成，不要求用户打开终端或复制命令。用户只需在确有需要时完成网页登录与宿主授权；连通后直接用自然语言操作。

先按[浏览器规则](../scenarios.md#浏览器优先级)区分展示与操作：文件链接先在内置预览展示；新建、导入或编辑成功后再展示实际结果链接。预览只能展示时，页面操作使用当前可用的外部 Computer Use / Browser Use，复用会话。CLI 不能接管这些浏览器；纯页面任务跳过下面的 CLI 初始化。需要辅助 API 且未选定 CLI 浏览器方案时，每条命令加 `--no-browser`，包括 `doctor`、`whoami`；该参数不转交页面命令。各浏览器登录态与 CLI 凭据独立。

技能包只包含技能定义、自检脚本和参考资料，**不携带程序，也不在安装时下载任何可执行文件**。完整 `shimo-net` 由 npm 包 `shimo-net-cli` 提供，包内已自带各平台程序；没有 Node.js 时才改用官方 Release 的原生二进制。两种方式都是完整 CLI，访问业务文件仍需连接目标石墨服务。

需要 API 通道时，Agent 要分别确认技能可用、CLI 可执行和账号已连通；不能把 ZIP 导入成功或技能显示在列表中当作全部配置完成。

## 第 0 步：自检

```bash
python3 <技能目录>/scripts/selfcheck.py --pretty
```

输出一个 JSON：`ready` 表示能否直接干活，`next.action` 是下一步该做的事（`install` / `init` / `login` / `none`），`next.command` 给出对应命令。这一步不需要联网，也不读取凭据内容，任何环境都能跑。

没有 python3 时跳过自检，直接执行 `shimo-net version --json`；返回版本号即视为可用。

## 复用已有的命令

先检查宿主是否已有 `shimo-net`（Bash/Zsh 用 `command -v shimo-net`，PowerShell 用 `Get-Command shimo-net`），存在时执行 `shimo-net version --json`。版本不低于本包要求就直接使用，不重复安装。

缺少命令或版本过低时走下面的安装渠道。不要静默执行版本不匹配的程序，也不要仅因需要某个命令就卸载用户已有版本。

## 用 npm 安装

若已有受支持的 Node.js（至少 18）和 npm，由 Agent 在当前获准使用的工作区内安装，`<工作区>` 换成实际绝对路径，不把占位符留给用户填写。Bash/Zsh：

```bash
npm install --prefix "<工作区>/.shimo-net-cli" --engine-strict --ignore-scripts --no-audit --no-fund --registry=https://registry.npmjs.org shimo-net-cli
export PATH="<工作区>/.shimo-net-cli/node_modules/.bin:$PATH"
shimo-net version --json
```

PowerShell：

```powershell
npm install --prefix "<工作区>\.shimo-net-cli" --engine-strict --ignore-scripts --no-audit --no-fund --registry=https://registry.npmjs.org shimo-net-cli
$env:PATH = "<工作区>\.shimo-net-cli\node_modules\.bin;" + $env:PATH
shimo-net version --json
```

每一步检查退出码；安装或版本检查失败时停止该路径并保留错误，不能用下一步输出冒充成功。PATH 只作用于当前终端，Agent 新建命令会话时须自行重新添加该目录，不要求用户每次配置。npm 包内已含程序，安装后不再联网下载二进制。

## 没有 Node.js：用官方二进制

从[官方 Releases](https://github.com/shimo-office/office-suites/releases) 下载与本包版本对应的文件，按当前架构选择：

| 系统 | 文件名 |
|---|---|
| macOS | `shimo-net_darwin_amd64` 或 `shimo-net_darwin_arm64` |
| Linux | `shimo-net_linux_amd64` 或 `shimo-net_linux_arm64` |
| Windows | `shimo-net_windows_amd64.exe` 或 `shimo-net_windows_arm64.exe` |

macOS/Linux 按 `uname -m` 选择：`x86_64` 对应 `amd64`，`arm64`/`aarch64` 对应 `arm64`；Rosetta 终端可能需要 `amd64`。Windows 按 `PROCESSOR_ARCHITECTURE` 选择。

放到用户可执行的目录（例如 `~/.local/bin/`），macOS/Linux 先 `chmod u+x`，再执行 `shimo-net version --json` 确认。不要使用其他版本或第三方镜像；下载失败就报告网络错误，不把超时当成功。

## 登录与真实验收

以下是确需 CLI/API 的任务才做的检查；CLI 登录要求已选定且获准使用 CLI 浏览器方案。由 Agent 运行版本、schema、doctor 和 whoami。doctor 报未登录不是安装失败；若 whoami 返回 401，按 [安装与会话](../install.md) 显式执行 `login`。切账号使用 `login --new`，由用户完成浏览器登录，再由 Agent 继续检查；不索取密码或 cookie，不自动改变文件权限。

用户给出已有权限的文件链接后，由 Agent 执行 `info <链接> --no-browser --json`，核对文件名、类型和链接；也可以先响应用户的搜索需求。不需要为安装检查创建或改写业务文件。页面编辑能力仍依赖宿主提供且获准使用的真实浏览器；仅 API 成功不代表页面验收通过。完整命令见 [命令参考](../cli-reference.md)，操作范围见 [场景清单](../scenarios.md)。

宿主没有本地命令执行工具时，不能宣称 CLI/API 通道可用；需要该通道时应切换到支持本地工具的会话。已有可操作浏览器工具的页面任务及内置预览不受此限制。不能把“代码包已解析”当作完整功能已经可用。
