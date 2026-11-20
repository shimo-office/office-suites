# 使用 npm 安装 shimo-net

在 AI 助手中使用时，直接从 [一句话接入](agent/README.md) 开始，由 Agent 完成安装、技能配置和验证。本页是自行使用终端或 Agent 自动执行时的 npm 参考。

npm 包名是 [`shimo-net-cli`](https://www.npmjs.com/package/shimo-net-cli)，安装后的命令是 **`shimo-net`**。支持 macOS、Linux、Windows 的 x64 / ARM64，需要 Node.js 18+（建议受支持的 LTS）。

包内包含各平台完整程序，安装和首次运行均无需再从 GitHub 下载运行程序。不使用 Node.js 时，可选择 [原生程序或其他安装方式](install.md#安装)。

## 安装

```bash
npm install -g shimo-net-cli --registry=https://registry.npmjs.org
shimo-net version
```

能输出真实版本号说明 CLI 可以启动。npm 只安装 CLI，不会自动安装助手技能或登录石墨；使用 Agent 时由它继续完成这两步。终端用户接着登录并尝试搜索：

```bash
shimo-net login
shimo-net whoami --json
shimo-net search "季度报告" --json
```

下载这个公开 npm 包无需注册或登录 npm；访问石墨文件需要你的石墨账号。需要网页登录时，由你在浏览器中完成；`whoami` 成功后才表示账号已连通。私有部署、登录与切账号见 [安装说明](install.md)。

## 更新与指定版本

使用 Agent 时可以说：“请更新 shimo-net，并检查技能版本和登录状态。”Agent 会先检查现有版本，再选择匹配的 CLI 与技能；以下是终端更新命令。

更新到 npm 已发布的最新版本：

```bash
npm install -g shimo-net-cli@latest --registry=https://registry.npmjs.org
shimo-net version
```

需要固定版本时，将 `latest` 改成版本号，例如：

```bash
npm install -g shimo-net-cli@0.9.19 --registry=https://registry.npmjs.org
```

可用版本见 [npm 版本列表](https://www.npmjs.com/package/shimo-net-cli?activeTab=versions)。已安装的程序不会随 GitHub 提交自动更新；使用固定版本的技能包时，CLI 版本应与技能要求一致。

## 仅安装到当前项目

没有全局安装权限，或只想在一个项目中使用时，在项目目录执行：

```bash
npm install shimo-net-cli --registry=https://registry.npmjs.org
npm exec -- shimo-net version
npm exec -- shimo-net --help
```

后续同样通过 `npm exec -- shimo-net <命令>` 调用。这会在当前项目中添加 npm 依赖；技能宿主使用独立工作区时，由 Agent 按 [技能运行说明](agent/runtime.md) 配置可持续使用的调用入口，用户继续用自然语言提需求。

## 卸载

全局安装的版本：

```bash
npm uninstall -g shimo-net-cli
```

项目内安装的版本：

```bash
npm uninstall shimo-net-cli
```

卸载 npm 包不会自动清理石墨登录状态。若也要清除 CLI 保存的登录凭据，请在卸载前运行 `shimo-net logout`；它不会退出你的日常浏览器。数据位置见 [本地数据与卸载](install.md#本地数据与卸载)。

## 常见问题

在 Agent 中出现以下问题时，把错误提示交给 Agent 处理即可；无需自己照表逐项运行命令。

| 问题 | 处理 |
|---|---|
| `command not found` | 重新打开终端，确认 npm 全局命令目录在 PATH；项目安装使用 `npm exec -- shimo-net`。 |
| Node.js 版本不符合要求 | 升级到受支持的 Node.js LTS，或使用不依赖 Node 的原生程序。 |
| 下载超时 | 确认能访问 `registry.npmjs.org`，恢复网络后重试；也可选择 GitHub Release 安装。 |
| 更新后还是旧版本 | 检查 `command -v shimo-net`（macOS/Linux）或 `Get-Command shimo-net`（PowerShell），确认当前调用的是 npm 安装的入口。 |
| `doctor` 报未登录 | 安装成功与石墨登录是两件事；运行 `shimo-net login` 后用 `whoami --json` 核对。 |

命令与参数见 [命令参考](cli-reference.md)。
