# 使用 npm 安装 shimo-net

**中文** | [English](en/npm.md) · [文档导航](README.md)

本页只介绍 npm 的安装、版本管理和卸载。平台要求、登录、切账号和私有部署统一见 [安装指南](install.md)；让助手代为配置则从 [Agent 接入](agent/README.md) 开始。

## 安装

需要 Node.js 18+（建议受支持的 LTS），支持 macOS / Linux / Windows 的 x64 和 ARM64。

```bash
npm install -g shimo-net-cli --registry=https://registry.npmjs.org
shimo-net version
```

包名是 [`shimo-net-cli`](https://www.npmjs.com/package/shimo-net-cli)，命令名是 **`shimo-net`**。包内含完整原生程序，安装及首次运行无需从 GitHub 再下载，也无需登录 npm。npm 不会配置助手技能或登录石墨；安装完成后继续 [登录石墨](install.md#登录复用会话与切账号)。

## 更新与指定版本

更新到 npm 已发布的最新版本：

```bash
npm install -g shimo-net-cli@latest --registry=https://registry.npmjs.org
shimo-net version
```

需要固定版本时，将 `latest` 改成版本号，例如：

```bash
npm install -g shimo-net-cli@0.9.20 --registry=https://registry.npmjs.org
```

可用版本见 [npm 版本列表](https://www.npmjs.com/package/shimo-net-cli?activeTab=versions)。已安装的程序不会随 GitHub 提交自动更新；使用固定版本的技能包时，CLI 版本应与技能要求一致。

## 仅安装到当前项目

没有全局安装权限，或只想在一个项目中使用时，在项目目录执行：

```bash
npm install shimo-net-cli --registry=https://registry.npmjs.org
npm exec -- shimo-net version
npm exec -- shimo-net --help
```

后续通过 `npm exec -- shimo-net <命令>` 调用。这会在当前项目添加 npm 依赖。助手使用独立工作区时，还需按 [技能运行说明](agent/runtime.md) 配置调用入口。

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

| 问题 | 处理 |
|---|---|
| `command not found` | 重新打开终端，确认 npm 全局命令目录在 PATH；项目安装使用 `npm exec -- shimo-net`。 |
| Node.js 版本不符合要求 | 升级到受支持的 Node.js LTS，或使用不依赖 Node 的原生程序。 |
| 下载超时 | 确认能访问 `registry.npmjs.org`，恢复网络后重试；也可选择 GitHub Release 安装。 |
| 更新后还是旧版本 | 检查 `command -v shimo-net`（macOS/Linux）或 `Get-Command shimo-net`（PowerShell），确认当前调用的是 npm 安装的入口。 |
| `doctor` 报未登录 | 安装成功与账号可用是两件事，继续 [登录流程](install.md#登录复用会话与切账号)。 |

命令与参数见 [命令参考](cli-reference.md)。
