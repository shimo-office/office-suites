---
name: shimo-net-cli
display_name: 石墨办公 CLI
display_name_en: shimo-net CLI
description: 用 shimo-net 命令行操作石墨（shimo.net）的文件与表格，支持搜索、上传下载、新建改名、读文稿和写表格。
description_zh: 在终端或 AI 助手里操作石墨文件与表格，覆盖搜索、上传下载、整理目录和读写表格。
description_en: Operate Shimo (shimo.net) files and sheets from a terminal or an AI assistant.
version: 0.9.22
author: shimo-office
allowed-tools: Bash, Read, Write, Edit
examples_zh:
  - "帮我在石墨里找“季度方案”，把命中的文件和链接列出来。"
  - "把这份本地报告上传到“资料”文件夹，然后给我文件链接。"
  - "把这个表格的工作表1 A1:C3 改成这份课表，改完读回来核对。"
examples_en:
  - "Find the file named Quarterly Plan in Shimo and list the matches with links."
  - "Upload this local report to the Documents folder and give me the link."
  - "Write this timetable into Sheet1 A1:C3, then read it back to verify."
---

# 石墨办公 CLI

`shimo-net` 是石墨（shimo.net，以及客户的私有部署）的命令行程序。本技能让 Agent 调用它完成文件与表格操作，并把真实结果和链接交回用户。

**依赖两样东西**：命令行程序 `shimo-net`（npm 包 `shimo-net-cli`），以及用户自己的石墨账号。技能本身不携带凭据，也不代替用户登录。

## 能做与不做

| 能做 | 不做 |
|---|---|
| 搜索、列目录、看文件信息 | 修改权限、成员授权、解除锁定 |
| 读文稿正文、读表格数据 | 绕过石墨的编辑限制 |
| 上传、导入、下载、新建、改名、移入回收站 | 彻底删除（只能移入回收站） |
| 写表格单元格、追加行、改格式 | 代替用户输入密码或接收验证码 |
| 按当前账号权限做只读巡检 | 猜测用户没有权限访问的链接 |

越界时直接说明限制并给出人工操作入口，不尝试用脚本绕开。

## 第 0 步：先自检环境（不需要联网）

```bash
python3 <技能目录>/scripts/selfcheck.py --pretty
```

输出是一个 JSON，`ready` 表示能否直接干活，`next` 说明下一步该做什么：

```json
{"ready": false,
 "skillVersion": "0.9.22",
 "cli": {"found": false, "path": null, "version": null, "minimum": "0.9.20", "outdated": false},
 "profile": {"initialized": false, "origin": null, "credentials": false},
 "next": {"action": "install", "command": "npm install -g shimo-net-cli", "reason": "shimo-net is not on PATH"}}
```

按 `next.action` 分三种情况：`install` 去第 1 步，`init` 或 `login` 去第 2 步，`none` 直接干活。

没有 python3 就跳过这一步，直接执行 `shimo-net version --json`，返回版本号即视为可用。

## 第 1 步：装好命令行

```bash
npm install -g shimo-net-cli
shimo-net version --json
```

npm 包自带各平台程序，安装后不再联网下载。升级用 `npm install -g shimo-net-cli@latest`。没有 Node.js 时，改按[安装说明](../install.md)使用平台二进制或源码构建。

## 第 2 步：连接账号

```bash
shimo-net config show --json
shimo-net whoami --no-browser --json
```

地址没配置过时执行 `shimo-net init --origin https://shimo.net`；私有部署换成客户自己的域名，不要改回公有云。`whoami` 返回 401 表示还没登录，请用户在自己的浏览器里完成登录——不要索取密码、验证码或 cookie。

## 干活的命令

默认都加 `--json`，便于解析；没有选择 CLI 浏览器方案时再加 `--no-browser`。

| 需求 | 命令 |
|---|---|
| 搜索文件 | `shimo-net search "关键词" --json` |
| 列目录 / 看空间 | `shimo-net ls "<文件夹链接>" --json`、`shimo-net spaces --json` |
| 看文件信息 | `shimo-net info "<链接>" --no-browser --json` |
| 读文稿正文 | `shimo-net read "<链接>" --json` |
| 上传 / 导入 | `shimo-net upload ./报告.pdf --to "<文件夹链接>" --json` |
| 下载 | `shimo-net get "<链接>" --out ./downloads --json` |
| 新建 / 改名 | `shimo-net new "名称" --type doc --to "<位置>"`、`shimo-net rename "<链接>" "新名字"` |
| 移入回收站 / 恢复 | `shimo-net delete "<链接>"`、`shimo-net restore "<链接>"` |
| 写表格 | `shimo-net editor "<表格链接>" --action sheet-update --payload '{"..."}' --json` |

参数、返回字段和退出码见[命令参考](../cli-reference.md)。

## 完整示例：一次真实任务

用户说：「帮我在石墨里找季度方案，然后总结一下要点。」

```bash
shimo-net search "季度方案" --json
```

返回若干命中项，每项带 `name` 和 `url`。挑最相关的一条继续读：

```bash
shimo-net read "<上一步返回的链接>" --json
```

拿到正文后按用户要的角度总结，并在回复里带上文件链接和「读自哪个文件」。如果搜索没有命中，直接说没找到，并建议换个关键词或让用户确认文件在哪——不要编一个看起来合理的链接。

写操作同理，只是最后多一步核对：

```bash
shimo-net editor "<表格链接>" --action sheet-update --payload '{"sheetName":"工作表1","range":"A1:C3","values":[[...]]}' --json
shimo-net read "<表格链接>" --json   # 读回来确认真的写进去了
```

## 出错怎么办

| 现象 | 含义 | 怎么处理 |
|---|---|---|
| `command not found: shimo-net` | 还没装 | 回到第 1 步；装完再跑一次自检 |
| 退出码 401 | 未登录或凭据过期 | 请用户登录，然后重试**一次**；不要循环重试 |
| 退出码 403 或提示无权限 | 账号没有该文件权限 | 如实告知，请用户在网页上调整权限 |
| 提示文件被锁定 / 限制编辑 | 服务端限制 | 说明需要在网页人工操作，不要反复尝试 |
| 15 秒无响应 | 网络或服务慢 | 最多重试 2 次，仍失败就报错并给出原始错误 |
| 写操作返回成功但读不到 | 只是受理，未落盘 | 报告「已提交但未核验」，不要宣称已完成 |

失败时保留原始错误文本和失败阶段，说明是否可能已经写入，不要用「大概」「应该成功了」这类说法。

## 数据真实性

- 文件链接只能来自命令返回，不能拼、不能猜、不能沿用示例里的占位符。
- 没执行过的命令不要说执行过；没读回的内容不要说已确认。
- 版本号、字段名、错误码以实际返回为准；不确定就说不确定。

详细场景和话术见[场景清单](../scenarios.md)，凭据边界见[安全策略](../../SECURITY.md)。
