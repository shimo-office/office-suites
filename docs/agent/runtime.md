# 技能包的运行与首次使用

SkillHub 和 WorkBuddy 上传的是技能定义、调用脚本和参考资料。完整 `shimo-net` 程序按操作系统单独获取，不在受限 ZIP 中裁剪功能。首次需要访问 GitHub Releases；完成缓存后，CLI 本身仍需要访问目标石墨服务。上传成功、技能启用、CLI 安装和账号登录是四个不同状态，必须分别验证。

## 使用包内入口

把下方 `<技能目录>` 换成宿主实际安装该技能的绝对目录，含空格时保留引号，不猜测客户端私有目录。

macOS / Linux：

```bash
sh "<技能目录>/scripts/shimo-net.sh" --version
sh "<技能目录>/scripts/shimo-net.sh" schema --json
sh "<技能目录>/scripts/shimo-net.sh" doctor --json
sh "<技能目录>/scripts/shimo-net.sh" whoami --json
```

Windows PowerShell：

```powershell
& "<技能目录>\scripts\shimo-net.ps1" --version
& "<技能目录>\scripts\shimo-net.ps1" schema --json
& "<技能目录>\scripts\shimo-net.ps1" doctor --json
& "<技能目录>\scripts\shimo-net.ps1" whoami --json
```

macOS/Linux 需要系统 shell、curl 及 shasum 或 sha256sum；Windows 需要 PowerShell 5.1 或更新版本。不需要 Python、Node 或 Go。若组织策略不允许运行脚本，报告限制并由用户/管理员处理，不自动关闭执行策略或安全保护。

每次调用都验证缓存二进制的 SHA-256；只有缺少缓存时才从包内固定版本的官方 Release 下载，并在执行前校验。哈希不符立即失败，不执行也不静默换来源。该机制不会替换已有 PATH 上的 CLI。`SHIMO_NET_RUNTIME_DIR` 可为运行器指定独立缓存位置；它不改变石墨账户配置目录。

## 登录与真实验收

先运行版本、schema、doctor 和 whoami。doctor 报未登录不是安装失败；若 whoami 返回 401，按 [安装与会话](../install.md) 使用上述入口显式执行 `login`。切账号使用 `login --new`，由用户完成浏览器登录，不索取密码或 cookie，不自动改变文件权限。

随后用用户已有权限的一个文件执行 `info <链接> --json`，核对文件名、类型和链接。页面编辑能力仍依赖宿主提供且获准使用的真实浏览器；仅 API 成功不代表页面验收通过。完整命令见 [命令参考](../cli-reference.md)，操作范围见 [场景清单](../scenarios.md)，已验收与保留项见 [阶段记录](../phase3-acceptance.md)。

首次下载失败应报告网络错误；保留有效缓存。宿主没有本地命令执行工具时，本技能不能宣称可执行；应切换到支持本地工具的会话。不能把“代码包已解析”当作完整功能已经可用。

## 自动下载超时时

可以用浏览器从[官方 Releases](https://github.com/shimo-office/office-suites/releases)下载与本包 `SKILL.md` 中 `version` 完全相同的二进制，然后放入运行器缓存。不要使用其他版本、第三方镜像或跳过哈希检查。

| 系统 | 文件名 | 默认缓存目录 |
|---|---|---|
| macOS | `shimo-net_darwin_amd64` 或 `shimo-net_darwin_arm64` | `~/.local/share/shimo-net-cli/runtime/<版本>/` |
| Linux | `shimo-net_linux_amd64` 或 `shimo-net_linux_arm64` | `~/.local/share/shimo-net-cli/runtime/<版本>/` |
| Windows | `shimo-net_windows_amd64.exe` 或 `shimo-net_windows_arm64.exe` | `%LOCALAPPDATA%\shimo-net-cli\runtime\<版本>\` |

macOS/Linux 按当前终端的 `uname -m` 选择：`x86_64` 对应 `amd64`，`arm64`/`aarch64` 对应 `arm64`。Rosetta 终端可能需要 `amd64`。Windows 按启动器读取的 `PROCESSOR_ARCHITEW6432`（未设置时用 `PROCESSOR_ARCHITECTURE`）选择。已设置 `SHIMO_NET_RUNTIME_DIR` 时，目录为该值下的 `<版本>/`；macOS/Linux 仅设置 `XDG_DATA_HOME` 时则为其下的 `shimo-net-cli/runtime/<版本>/`。

保留下载文件的原名；macOS/Linux 为该文件增加用户执行权限（`chmod u+x "<缓存文件的绝对路径>"`）。随后仍通过包内启动器执行 `--version`，它会先验证固定 SHA-256 再运行；不要直接执行尚未验证的下载文件。再按上面的步骤检查 schema、登录和真实文件访问。手动放入缓存只解决下载，不代表账号或业务操作已验收。
