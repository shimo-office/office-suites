# shimo-net

石墨办公套件 CLI：把 shimo.net 与客户私有部署接入终端和 Agent，用于文件管理、内容读取和有限编辑操作。

[![Release](https://img.shields.io/github/v/release/shimo-office/office-suites?label=release)](https://github.com/shimo-office/office-suites/releases)
[![CI](https://github.com/shimo-office/office-suites/actions/workflows/ci.yml/badge.svg)](https://github.com/shimo-office/office-suites/actions/workflows/ci.yml)

## 30 秒安装

已有 Node.js 18+ / npm 时，macOS、Linux、Windows 均可安装已发布的完整程序（建议使用受支持的 Node.js LTS）：

```bash
npm install --global shimo-net-cli@0.9.19 --registry=https://registry.npmjs.org
shimo-net version
shimo-net login
shimo-net whoami --json
```

npm 包无需从 GitHub 二次下载；包名是 `shimo-net-cli`，使用命令是 `shimo-net`。无全局安装权限时见 [npm 分发说明](docs/npm.md)。也可通过下面的仓库安装器安装不依赖 Node 的原生程序。

macOS / Linux：

```bash
git clone https://github.com/shimo-office/office-suites.git
cd office-suites
./install.command
shimo-net init
shimo-net login
shimo-net whoami --json
```

Windows PowerShell 在仓库目录执行：

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
shimo-net init
shimo-net login
shimo-net whoami --json
```

安装器有 Go 时构建当前源码，否则下载最新 Release。也可使用仓库内的 `bootstrap.sh` 配置 Agent；具体步骤、跨平台要求、会话复用与 `login --new` 切账号见 [安装说明](docs/install.md)。WorkBuddy 的 shimo-net-cli 技能从 SkillHub 安装，见 [WorkBuddy 教程](docs/agent/workbuddy.md)。

## 文档导航

| 要做什么 | 权威文档 |
|---|---|
| 安装、登录、切账号、私有部署、排错 | [安装说明](docs/install.md) |
| 查找办公场景、自然语言话术、浏览器选择规则 | [场景清单](docs/scenarios.md) |
| 查询命令、参数、JSON、退出码与 @别名 | [命令参考](docs/cli-reference.md) |
| 配置 Agent | [技能包](docs/agent/SKILL.md) · [WorkBuddy](docs/agent/workbuddy.md) · [DSH](docs/agent/dsh.md) · [豆包工作](docs/agent/doubao.md) · [Codex](docs/agent/codex.md) |
| 查看能力计数和实测边界 | [能力矩阵](docs/internal/capability-matrix.md) · [扩展编辑验收记录](docs/phase3-acceptance.md) |
| 浏览全部文档 | [文档导航](docs/README.md) |

版本以 `shimo-net version` 为准，变更见 [CHANGELOG](CHANGELOG.md)。能力数量由目录生成，不在教程中重复维护。扩展编辑的环境阻塞和待验收项以验收记录为准，文档整理不代表这些项目已通过。

## 安全与许可

文件锁定、限制编辑、高级权限、成员级授权、彻底删除必须由用户在网页人工操作；CLI 删除只进入回收站。收回与其他权限变更同样由人工完成，详情见 [安全策略](SECURITY.md)。不要在对话、日志或截图中暴露 cookie。

本仓库默认保留所有权利；分发或商业使用需获得授权，见 [LICENSE](LICENSE)。产品及私有化部署介绍：[石墨办公套件](https://shimo.net/products/office-suites)。
