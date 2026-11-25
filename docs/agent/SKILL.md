---
name: shimo-net-cli
display_name: 石墨办公套件私有部署 CLI
display_name_en: Shimo Office Suite Private Deployment CLI
description: 石墨办公套件私有部署的命令行工具 shimo-net-cli（命令名 shimo-net），在终端或 AI 助手里操作石墨文件与表格，支持搜索、上传下载、新建改名、读文稿和写表格；未登录时先产出可执行的写入计划与命令清单。
description_zh: 石墨办公套件私有部署 CLI（包名 shimo-net-cli，命令 shimo-net）覆盖搜索文件、读文稿、上传下载、整理目录和读写表格，公有云 shimo.net 与客户私有部署同一套命令；没有账号权限时先交出可复核的环境结论与写入计划。
description_en: shimo-net-cli (command shimo-net) is the CLI for the Shimo office suite private deployment, covering search, upload and download, rename, reading documents and writing sheets on shimo.net or a customer deployment; without a signed-in account it still returns a verified environment report and a ready-to-run write plan.
version: 0.9.31
author: shimo-office
allowed-tools: Bash, Read, Write, Edit
examples_zh:
  - "先检查这台机器能不能用石墨 CLI：看命令在不在、配置和登录状态，缺什么就告诉我下一步该跑哪条命令。"
  - "把这份课表整理成石墨表格的写入计划，先别写入，给我 payload 和要执行的命令；课表 CSV 我随后贴给你。"
  - "登录后帮我在石墨里找“季度方案”，把命中的文件和链接列出来。"
examples_en:
  - "Check whether this machine can use the Shimo CLI: command, profile and sign-in state, and the exact next command for whatever is missing."
  - "Turn this timetable into a Shimo sheet write plan without writing yet; give me the payload and the command. I will paste the CSV next."
  - "Once signed in, find the file named Quarterly Plan in Shimo and list the matches with links."
---

# 石墨办公套件私有部署 CLI

`shimo-net` 是石墨（shimo.net，以及客户的私有部署）的命令行程序。本技能负责把它调起来，把真实结果和链接交回用户。

**完整任务需要两样**：命令 `shimo-net`（npm 包 `shimo-net-cli`），和**用户自己**的石墨账号。技能不携带凭据、不代替用户输入密码或验证码，也不会把"没登录"写成"已完成"；缺账号时按第 3 步交付可复核的环境结论与写入计划，不空手回一句"做不了"。

## 铁律：证据先于结论

1. 每条结论后面必须跟**命令 + 原始输出**。只有转述、没有原始输出的说法，一律写成「未验证」。
2. 本文所有 JSON、路径、版本号、错误码都是**结构示例，数值虚构**。禁止把它们当成本机输出转述；本机结论只能来自本会话真实执行过的命令。
3. 输出里没有的路径、链接、文件名，不补全、不推测、不拼接。
4. 拿不到数据就说拿不到，并写清卡在哪一步、解锁需要什么。**宁可交一份"做不了"的清单，也不要交一份看起来完整的编造。**
5. 同一件事只报告一次。没有重新执行，就不要改写上一次的结论。

## 第 1 步：判定现在能干什么

按顺序执行，把原始输出留在对话里。`--json` 出错时 JSON 只写 stderr，所以统一 `2>&1` 收集：

```bash
command -v shimo-net
shimo-net version --json 2>&1
shimo-net doctor --json --no-browser 2>&1
shimo-net whoami --json --no-browser 2>&1
```

`shimo-net` 不在 PATH 时，先按[安装说明](../install.md)安装（npm 包 `shimo-net-cli`），装完再回来跑上面四条。安装本身失败（离线、无 npm、无权限）就**不要重试**，直接走第 3 步，把 `selfcheck.py` 的输出（有 `python3` 时）和安装命令的原始报错一起写进环境结论。

**自检脚本只是线索，不是证据**：`scripts/selfcheck.py` 只读本机文件、不发网络请求，「有没有登录」是按凭据文件存在与否推断的。必须用上面四条命令复核；两者矛盾时以实际命令为准，并把矛盾原样写进报告。没有 `python3` 时跳过自检，以 `command -v` / `version` / `doctor` 的实际输出为准，不要因此判定环境不可用。

**读法的三个前提**：

1. `doctor` 的 `checks` 是**数组**，元素形如 `{"name":"credentials","ok":false,"detail":"未登录"}`；要看的是 `name` 为 `credentials` 那一项的 `ok`，不是顶层有没有 `credentials` 字段。
2. `doctor` 只要有任何一项没过（没账号、没 Chrome）就退出 1，且 JSON 只写 stderr、stdout 是空的。**非 0 不代表环境坏了**，要按 `checks` 逐项读。
3. 判断未登录、无权限、被锁定，看 `ok:false` 和 `error` 文本，**不要看退出码**——CLI 只用 0/1/2（见[命令参考](../cli-reference.md#json退出码与失败恢复)），401/403 是 HTTP 状态，不会成为进程退出码。

判读三条路：

| 看到的输出 | 走哪条 |
|---|---|
| `command -v shimo-net` 无输出，且安装失败 | **环境不可用**：第 3 步，只交环境结论 + 安装报错 |
| `checks` 里 `credentials` 的 `ok:true`，`whoami` 返回 `ok:true` 且带账号 | **有账号分支**：第 2 步 |
| `credentials` 的 `ok:false`，或 `whoami` 返回 `ok:false`、`error` 含「未登录」「登录已过期」 | **无凭据分支**：第 3 步 |

进入无凭据分支后，**不要**执行 `login`、**不要**执行 `session harvest`，也不要翻浏览器配置目录、探测端口或查进程——这些都属于绕过，且会把一次可解释的阻塞变成一串无法复现的尝试。


## 第 2 步：有账号时干活

命令默认加 `--json`；没有选定 CLI 浏览器方案时再加 `--no-browser`。

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
| 写表格 | `shimo-net editor "<表格链接>" --action sheet-update --payload '{"range":"工作表1!A1:C3","resource":{"values":[["…"]]}}' --json` |

参数、返回字段和退出码见[命令参考](../cli-reference.md)。

一次完整任务的顺序是：**搜索 → 从返回里取链接 → 读正文或写表格 → 复跑一次读回核对**。搜索没命中就说没找到，并建议换关键词；不要编一个看起来合理的链接。写操作返回成功只代表服务端受理，必须 `read` 读回才算核验，读不到就写「已提交但未核验」。

## 第 3 步：没有账号、或装不上时怎么交付

沙箱、新机器、用户还没登录、`shimo-net` 装不上时，**仍然要交出确定的东西**，但业务数据一律挂起，不得虚构：

| 能交付 | 怎么做 |
|---|---|
| 环境结论 | 写成 `./output/environment.md`：第 1 步四条命令 + 原始输出（含 stderr）+ 判读结论，异常逐条注明；装不上时附安装命令的报错 |
| 写入计划 | 先把用户给的表格内容存成 CSV（用户直接粘贴的内容就写进 `./课表.csv`），再跑 `python3 <技能目录>/scripts/plan.py --csv ./课表.csv --sheet 工作表1 --range A1 --out ./output` |
| 解锁命令 | 一条：`shimo-net login`（装不上时先 `npm install -g shimo-net-cli`），并说明解锁后执行哪几条命令即可完成 |

`plan.py` 是纯标准库、零网络的本地预演：读取 CSV，产出 `payload.json`（石墨表格接口的 payload）、`run.sh`（带目标链接参数的执行脚本）和校验结果；遇到工作表名为空、range 不合法、行列对不上、单元格超长会直接报错而不是猜测。它**不写入石墨**，输出里必须如实标注"未写入"。

没有 `python3` 时不要停在这里：直接按 `{"range":"工作表1!A1:C3","resource":{"values":[[...]]}}` 的结构手写 payload 并给出完整命令，明确标注「未经 `plan.py` 校验」和校验点（行列数、工作表名、range 语法）。这仍然算「写入计划」已交付，只是少了一层自动校验。

| 必须挂起（写「未执行」，不得虚构） | 原因 |
|---|---|
| 搜索命中清单、文件名、链接、改名位置 | 需要账号 |
| 文稿正文与要点摘要 | 需要账号 |
| 表格写入与读回核对 | 需要账号 |

交付固定三段，缺一段就算没交付：

1. **已实测**：命令 + 原始输出；
2. **未执行**：挂起项 + 卡在哪一步；
3. **解锁**：一条可复制的命令 + 解锁后要跑的步骤。

不要用「稍后可以」「应该能」「大概写进去了」这类说法。

## 第 4 步：登录必须用户点头

登录会弹出一个 **Chrome / Chromium / Edge 专用窗口**（不是系统默认浏览器；只装了 Brave / Vivaldi 时能复用已有会话，但新登录做不了）。这属于会打断用户的动作，**执行前必须先问，并得到明确同意**：

- 先看 `doctor` 的 `checks` 里 `chrome` 项：`ok:false` 说明本机没有可用浏览器，**这时不要征求同意、不要尝试登录**，直接转第 3 步，写明「缺 Chrome / Chromium / Edge，装好后才能登录」；
- 未得到同意：不要运行 `shimo-net login`，把命令给用户自己执行；
- 得到同意后再登录，且只执行一次；
- 首次使用先 `shimo-net config show --json`，地址没配过再 `shimo-net init --origin https://shimo.net`（私有部署换客户域名）；
- **失败终止条件**：返回 `ok:false` 且 `error` 含「未登录」「登录已过期」→ 请用户登录后重试**一次**；仍失败就停下，写明「卡在鉴权」，不再尝试任何绕过手段。

## 出错怎么办

| 现象 | 含义 | 怎么处理 |
|---|---|---|
| `command not found: shimo-net` | 还没装 | 回第 1 步安装；装好重跑判定命令，装不上就走第 3 步 |
| `ok:false` + `error` 含「未登录」「登录已过期」 | 没有凭据或已过期 | 要用户登录；重试一次即止，转第 3 步交付 |
| `ok:false` + `error` 含「无权限」「权限不足」 | 账号没有该文件权限 | 如实告知，请用户在网页上调整权限 |
| 文件被锁定 / 限制编辑 | 服务端限制 | 说明需网页人工操作，不要反复尝试 |
| 90 秒无响应 | 客户端超时（`api.go` 的 HTTP 超时就是 90 秒），网络或服务慢 | 最多重试 2 次，仍失败就报原始错误 |
| 写成功但读不到 | 只是受理，未落盘 | 报「已提交但未核验」，不要宣称完成 |

失败时保留原始错误文本和失败阶段，说明是否可能已写入。带 `--json` 时错误只在 stderr，记得 `2>&1` 一起留证。


## 数据真实性

- 文件链接只能来自命令返回，不能拼、不能猜、不能沿用示例里的占位符。
- 没执行过的命令不要说执行过；没读回的内容不要说已确认。
- 版本号、路径、字段名、错误码以实际返回为准；不确定就说不确定，并给出复核命令。

详细场景见[场景清单](../scenarios.md)，凭据边界见[安全策略](../../SECURITY.md)。
