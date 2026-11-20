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
  <a href="#让-agent-一句话安装">Agent 一句话安装</a> · <a href="#安装">安装方式</a> · <a href="#快速开始">快速开始</a> · <a href="#常用命令">常用命令</a> · <a href="#在-ai-助手中使用">AI 助手</a> · <a href="#文档">文档</a>
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

可以让 Agent 帮你安装，也可以自己使用 npm、curl 或下载原生程序。

### 让 Agent 一句话安装

把下面这段话复制给能访问网络并执行本机命令的 AI 助手（如 Codex、WorkBuddy、DSH 或豆包工作）：

```text
请帮我安装并配置 shimo-net，按这份说明执行：
https://github.com/shimo-office/office-suites/blob/main/docs/agent/setup.md
请自动接入当前助手的技能并验证连通，已有可用安装就复用。
完成后让我直接用自然语言操作石墨。
```

Agent 会完成安装、技能接入和环境检查。你只需在提示时完成网页登录或授权，之后直接说要做什么。客户私有部署请在话术后补充石墨服务地址。

第一次使用可跟着[四步图文教程](docs/agent/README.md)操作，或直接选择 [WorkBuddy](docs/agent/workbuddy.md) · [Codex](docs/agent/codex.md) · [DSH](docs/agent/dsh.md) · [豆包工作](docs/agent/doubao.md)。

<details>
<summary>自己在终端安装：npm · curl · 原生程序</summary>

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

可将 `codex` 换成 `workbuddy`、`dsh` 或 `doubao`。脚本下载 CLI、初始化服务地址并检查登录；Codex/DSH 会写入脚本预设的技能目录，仍需核对当前助手是否加载。安装目录为 `~/.local/bin`，PATH 设置与各助手的后续步骤见 [curl 一键配置](docs/install.md#curl-一键配置)。

### 更多方式

| 其他安装方式 | 适用情况 | 说明 |
|---|---|---|
| [下载原生程序](docs/install.md#直接下载原生程序) | 不使用 Node.js | 从 GitHub Releases 下载对应系统与架构的文件 |
| [仓库安装器](docs/install.md#从仓库安装) | 已克隆仓库 | 有 Go 时构建源码，否则下载 Release |
| [从源码构建](docs/install.md#源码构建与本地开发) | 开发或定制 CLI | 需要 Go 1.26.11+ |

更新、指定版本安装和卸载见 [npm 安装指南](docs/npm.md)；完整平台说明见 [安装文档](docs/install.md)。

</details>

## 快速开始

Agent 确认配置可用后，直接发：

```text
帮我在石墨里找到“季度报告”，先列出文件名和链接；我选好后，再总结主要内容和待办事项。
```

你会得到基于真实文件的结果。找到多份时先选文件，没有权限时 Agent 会说明。之后可以继续说：

| 你想做什么 | 直接对 Agent 说 |
|---|---|
| 阅读文档 | “总结这份文档的五个重点，并附上原文链接。” |
| 下载文件 | “把这份文件下载到当前工作目录。” |
| 创建周报 | “在这个文件夹新建一份名为本周周报的文稿。” |
| 整理表格 | “把这个表格工作表1的 A1:C1 设为蓝底白字，完成后核对。” |

提供对应文件或文件夹链接即可；更多例子见[办公场景](docs/scenarios.md)。

## 常用命令

Agent 用户无需手动执行以下命令；它们供终端用户和脚本调用。

<details>
<summary>展开终端命令示例</summary>

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

</details>

## 在 AI 助手中使用

上面的安装请求已包含技能接入，无需再手动配置。[Skill](docs/agent/SKILL.md) 告诉 Agent 如何正确调用 CLI、处理登录并核对结果；日常使用直接说任务即可。

按你使用的助手选择教程：[WorkBuddy](docs/agent/workbuddy.md) · [Codex](docs/agent/codex.md) · [DSH](docs/agent/dsh.md) · [豆包工作](docs/agent/doubao.md)。

技能会按宿主安装到用户或项目范围，或在当前任务读取说明；Agent 会说明生效范围。换电脑、助手或项目后，让它检查并补齐配置。使用平台技能包时，版本和运行器由 Agent 按[运行说明](docs/agent/runtime.md)处理。

## 文档

| 你想了解 | 文档 |
|---|---|
| 第一次用 Agent 操作石墨 | [四步图文教程](docs/agent/README.md) |
| 自己安装、登录、切账号与私有部署 | [安装指南](docs/install.md) |
| npm 安装、更新和卸载 | [npm 指南](docs/npm.md) |
| 具体参数与示例 | [命令参考](docs/cli-reference.md) |
| 办公场景与使用范围 | [场景清单](docs/scenarios.md) |
| 新版本变化 | [更新记录](CHANGELOG.md) |
| 其他文档 | [文档目录](docs/README.md) |

遇到问题先让 Agent 检查并继续原任务，常见情况见[排错话术](docs/agent/README.md#下次使用与常见问题)。仍无法解决时可[提交 Issue](https://github.com/shimo-office/office-suites/issues)，让 Agent 整理版本、操作系统和去除敏感信息后的错误信息。

## 安全与许可

CLI 按当前账号权限操作。权限变更、文件锁定、限制编辑和彻底删除由用户在石墨网页完成；CLI 删除只移入回收站。不要分享密码或 cookie，详见 [安全策略](SECURITY.md)。

### 使用许可

本仓库保留所有权利；复制、修改、分发、再许可或商业使用须事先获得 shimo-office 团队的书面授权。完整条款见 [LICENSE](LICENSE)。[了解石墨办公套件](https://shimo.net/products/office-suites)。
