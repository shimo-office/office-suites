<h1 align="center">shimo-net</h1>

<p align="center">
  <strong>在终端和 AI 助手中使用石墨办公</strong><br />
  搜索文件 · 读取文档 · 管理目录 · 编辑表格<br />
  支持 shimo.net 与客户私有部署
</p>

<p align="center">
  <a href="https://www.npmjs.com/package/shimo-net-cli"><img src="https://img.shields.io/npm/v/shimo-net-cli?color=cb3837&amp;labelColor=334155" alt="npm 版本" /></a>
  <a href="https://github.com/shimo-office/office-suites/releases"><img src="https://img.shields.io/github/v/release/shimo-office/office-suites?label=release&amp;color=2563eb&amp;labelColor=334155" alt="GitHub Release" /></a>
  <a href="https://github.com/shimo-office/office-suites/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/shimo-office/office-suites/ci.yml?branch=main&amp;label=CI&amp;labelColor=334155" alt="CI 状态" /></a>
</p>

<p align="center">
  <a href="#安装">安装</a> · <a href="#快速开始">快速开始</a> · <a href="#常用命令">常用命令</a> · <a href="#在-ai-助手中使用">AI 助手</a> · <a href="#文档">文档</a>
  <br />
  <a href="#使用许可">使用许可</a> · <a href="https://github.com/shimo-office/office-suites/blob/main/SECURITY.md">安全策略</a>
</p>

## 能做什么

| 文件与内容 | 编辑与协作 |
|:---|:---|
| **文件管理**<br />搜索、上传、下载与目录管理 | **表格处理**<br />读取数据、写入单元格与批量处理 |
| **内容读取**<br />提取文档正文，供检索与总结 | **文稿与演示**<br />文稿排版与幻灯片内容编辑，范围见场景清单 |
| **私有部署**<br />指定石墨服务地址，按账号权限操作 | **AI 助手**<br />搭配 Skill，用自然语言发起办公任务 |

<details>
<summary>查看能力概览图</summary>

<p align="center">
  <img src="https://cdn.jsdelivr.net/gh/shimo-office/office-suites@f185192/docs/images/readme/capabilities.png" width="960" alt="shimo-net 能力概览：文件管理、内容读取、表格处理、文稿与演示、AI 助手和私有部署" />
</p>

</details>

具体操作与支持范围见 [场景清单](docs/scenarios.md) 和 [命令参考](docs/cli-reference.md)。

## 安装

选择适合你的安装方式：已有 Node.js 可用 npm；macOS/Linux 可用 curl 配置 CLI 与助手；也可直接下载原生程序。

### npm · macOS / Linux / Windows

需要 Node.js 18+，建议使用受支持的 LTS 版本。

```bash
npm install -g shimo-net-cli --registry=https://registry.npmjs.org
shimo-net version
```

npm 包名是 `shimo-net-cli`，安装后的命令是 **`shimo-net`**。包内已包含完整程序，安装后无需再从 GitHub 下载运行程序。

### curl · macOS / Linux

下载一键配置脚本，以 Codex 为例：

```bash
curl -fL --retry 2 --connect-timeout 20 \
  https://github.com/shimo-office/office-suites/releases/latest/download/bootstrap.sh \
  -o shimo-net-bootstrap.sh &&
bash shimo-net-bootstrap.sh codex
```

可将 `codex` 换成 `workbuddy`、`dsh` 或 `doubao`。脚本下载 CLI、初始化服务地址并检查登录；Codex/DSH 会配置对应技能。安装目录为 `~/.local/bin`，PATH 设置与各助手的后续步骤见 [curl 一键配置](docs/install.md#curl-一键配置)。

### 更多方式

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

### 使用许可

本仓库保留所有权利；复制、修改、分发、再许可或商业使用须事先获得 shimo-office 团队的书面授权。完整条款见 [LICENSE](LICENSE)。[了解石墨办公套件](https://shimo.net/products/office-suites)。
