# shimo-net

在终端和 AI 助手中使用石墨：搜索文件、读取文档、管理目录和编辑表格。支持 **shimo.net** 与客户私有部署。

[![npm](https://img.shields.io/npm/v/shimo-net-cli)](https://www.npmjs.com/package/shimo-net-cli)
[![Release](https://img.shields.io/github/v/release/shimo-office/office-suites?label=release)](https://github.com/shimo-office/office-suites/releases)
[![CI](https://github.com/shimo-office/office-suites/actions/workflows/ci.yml/badge.svg)](https://github.com/shimo-office/office-suites/actions/workflows/ci.yml)

[安装](#安装) · [快速开始](#快速开始) · [常用命令](#常用命令) · [AI 助手](#在-ai-助手中使用) · [文档](#文档)

## 能做什么

- **文件管理**：搜索文件和空间，上传、导入、下载，新建和改名。
- **内容处理**：读取文档正文，读写表格数据，执行已支持的编辑操作。
- **脚本与 AI 助手**：通过 `--json` 获取结构化结果，使用 Skill 接入办公任务。
- **私有部署**：切换石墨服务地址，按当前账号权限访问文件。

具体操作与支持范围见 [场景清单](docs/scenarios.md) 和 [命令参考](docs/cli-reference.md)。

## 安装

推荐使用 npm，适用于 macOS、Linux 和 Windows。需要 Node.js 18+，建议使用受支持的 LTS 版本。

```bash
npm install -g shimo-net-cli --registry=https://registry.npmjs.org
shimo-net version
```

npm 包名是 `shimo-net-cli`，安装后的命令是 **`shimo-net`**。包内已包含完整程序，安装后无需再从 GitHub 下载运行程序。

| 其他安装方式 | 适用情况 | 说明 |
|---|---|---|
| [下载原生程序](docs/install.md#直接下载原生程序) | 不使用 Node.js | 从 GitHub Releases 下载对应系统与架构的文件 |
| [仓库安装器](docs/install.md#从仓库安装) | 已克隆仓库 | 有 Go 时构建源码，否则下载 Release |
| [从源码构建](docs/install.md#源码构建与本地开发) | 开发或定制 CLI | 需要 Go 1.26.11+ |

更新、指定版本安装和卸载见 [npm 安装指南](docs/npm.md)；完整平台说明见 [安装文档](docs/install.md)。

## 快速开始

登录石墨并搜索一个文件：

```bash
shimo-net login
shimo-net whoami --json
shimo-net search "季度报告" --json
```

没有可复用的登录状态时，CLI 会打开登录窗口，由你完成登录。客户私有部署请先按 [服务地址配置](docs/install.md#初始化与私有部署) 设置域名。

## 常用命令

将下面的 `<文档链接>` 或 `<文件链接>` 替换为你有权限访问的石墨链接。

```bash
# 读取文档正文
shimo-net read "<文档链接>" --json

# 下载文件
shimo-net get "<文件链接>" --out ./downloads --json

# 查看命令与环境
shimo-net --help
shimo-net schema --json
shimo-net doctor --json
```

更多文件、表格和编辑命令见 [命令参考](docs/cli-reference.md)。

## 在 AI 助手中使用

安装 CLI 并完成登录后，为助手配置 [shimo-net-cli Skill](docs/agent/SKILL.md)，即可用自然语言发起任务，例如：

> 帮我在石墨里找到“季度报告”，读取正文并总结重点，附上原文链接。

按你使用的助手选择教程：[WorkBuddy](docs/agent/workbuddy.md) · [Codex](docs/agent/codex.md) · [DSH](docs/agent/dsh.md) · [豆包工作](docs/agent/doubao.md)。

需要导入技能 ZIP 时，从 [Release](https://github.com/shimo-office/office-suites/releases/latest) 选择对应平台的 `skillhub.zip` 或 `workbuddy.zip` 结尾的资源；CLI 安装与技能配置的区别见 [运行与首次使用](docs/agent/runtime.md)。

## 文档

| 你想了解 | 文档 |
|---|---|
| 安装、登录、切账号与私有部署 | [安装指南](docs/install.md) |
| npm 安装、更新和卸载 | [npm 指南](docs/npm.md) |
| 具体参数与示例 | [命令参考](docs/cli-reference.md) |
| 办公场景与使用范围 | [场景清单](docs/scenarios.md) |
| 新版本变化 | [更新记录](CHANGELOG.md) |
| 其他文档 | [文档目录](docs/README.md) |

遇到问题可 [提交 Issue](https://github.com/shimo-office/office-suites/issues)，请附上 `shimo-net version`、操作系统和去除敏感信息后的错误输出。

## 安全与许可

CLI 按当前账号权限操作。权限变更、文件锁定、限制编辑和彻底删除由用户在石墨网页完成；CLI 删除只移入回收站。不要分享密码或 cookie，详见 [安全策略](SECURITY.md)。

本仓库保留所有权利；分发或商业使用需获得授权，见 [LICENSE](LICENSE)。[了解石墨办公套件](https://shimo.net/products/office-suites)。
