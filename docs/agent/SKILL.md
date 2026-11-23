---
name: shimo-net-cli
display_name: 石墨办公 CLI
display_name_en: shimo-net CLI
description: 用 shimo-net-cli 操作石墨（shimo.net）的文件与表格；用户提到石墨、shimo.net 或 shimo-net-cli，或要搜索、上传、下载、新建、改名、读文档、写表格时使用。
description_zh: 用一句话操作石墨：搜索文件、上传下载、新建改名、读文稿、写表格，并回传文件链接与核验结果。
description_en: Operate Shimo (shimo.net) files and sheets with the shimo-net-cli command — search, upload, download, create, rename, read documents, and write spreadsheets.
examples_zh:
  - "帮我搜索石墨里名为‘季度方案’的文件，并返回链接。"
  - "在这个表格的工作表1 A1:C3 写入课表内容，然后打开页面核对结果。"
  - "列出我订阅的文件，选择一个打开查看最近更新。"
examples_en:
  - "Search Shimo for the file named 'Quarterly Plan' and return its link."
  - "Write the timetable to Sheet1 A1:C3 and open the page to verify it."
  - "List my subscribed files and open one to review its latest update."
version: 0.9.21
author: shimo-office
---

# 石墨办公 CLI

`shimo-net` 是石墨（shimo.net 及客户私有部署）的命令行工具。安装本技能后，用户用自然语言描述任务，Agent 调用 `shimo-net` 完成，并回传文件链接与核验结果。

正文只保留执行步骤与硬性边界。参数见 [命令参考](../cli-reference.md)，话术与边界见 [场景清单](../scenarios.md)，安装见 [安装说明](../install.md)，包内运行器见 [运行说明](runtime.md)。

## 快速开始

**第 1 步：确认命令可用**

```bash
shimo-net version --json
```

没有该命令时按 [安装说明](../install.md) 安装固定版本 `shimo-net-cli@0.9.21`。不便使用 npm 时，按 [运行说明](runtime.md) 绑定包内启动器：Bash/Zsh 用 `sh <技能目录>/scripts/shimo-net.sh`，PowerShell 用 `<技能目录>\scripts\shimo-net.ps1`。程序版本必须与本文件 `version`（0.9.21）一致，不一致不要混用。

**第 2 步：确认服务地址与账号**

```bash
shimo-net config show --json
shimo-net whoami --no-browser --json
```

地址未初始化时执行 `shimo-net init --origin https://shimo.net`；客户私有部署换成其域名，不要改回公有云。`whoami` 返回 401 表示 API 通道未登录，页面预览仍可用；需要 API 时由用户完成登录，Agent 不索取密码或 cookie。

**第 3 步：按任务执行命令并核验**

```bash
shimo-net search "季度方案" --json
shimo-net ls <文件夹链接> --json
shimo-net info <链接> --no-browser --json
shimo-net read <链接> --json
shimo-net upload ./报告.pdf --to <文件夹链接> --json
shimo-net get <链接> --out ./downloads --json
shimo-net editor <表格链接> --action sheet-update --payload '{"..."}' --json
```

所有命令默认加 `--json`；未选定 CLI 浏览器登录方案时再加 `--no-browser`。写命令执行后必须读回、导出或看真实页面核对，不能只凭退出码报告成功。

## 常用命令

| 需求 | 命令 |
|---|---|
| 搜索文件 | `search <关键词> [--limit N]` |
| 空间与目录 | `spaces`、`ls <链接或 @别名>` |
| 文件信息 | `info <链接>` |
| 读正文 | `read <链接> [--max-chars N]` |
| 上传 / 导入 | `upload <文件> --to <位置>`、`import <文件> --to <位置>` |
| 下载 / 拉取 | `get <链接> [--out 目录]`、`pull <目录> [--out 路径]` |
| 新建 | `new <名称> --type <类型> [--to 位置]`、`mkdir <名称>` |
| 改名 | `rename <链接> <新名字>` |
| 回收站 | `delete`、`trash`、`restore` |
| 权限巡检 | `audit <目录> [--limit N]`（只读） |
| 表格读写 | `editor` 的 `sheet-update` / `sheet-append` / `sheet-insert` / `sheet-delete` / `sheet-format` / `sheet-merge` |
| 常用位置 | `fav add/list/show/rm`，之后用 `ls @别名` |

## 执行边界

- **人工操作**：文件锁定、限制编辑、高级权限、成员级授权、权限变更、彻底删除必须由用户在石墨网页完成。CLI 拒绝不是改用自动化的许可。
- **删除语义**：`delete` 直接移入回收站、不弹确认；恢复用 `restore`。导入后默认私有，调整权限先与用户确认再人工设置。
- **写入核验**：API 返回成功只代表受理。写后核对读回值、导出结构或真实页面截图；没有截图只能报告“已提交，待页面核验”。
- **不盲目重放**：操作可能已生效时先只读核对，不重跑新增、删除、发布或答卷提交；失败须保留原错误、阶段和可能已写入的提示。
- **凭据**：不把 cookie、sid、Authorization 或账号明细写进对话、示例、截图文件名和报告；不索取、手动注入或跨浏览器搬运 cookie。
- **页面操作**：用具备读取、点击、截图能力的内置浏览器；只能预览时用实际可用的外部 Computer Use／Browser Use。规则见[浏览器优先级](../scenarios.md#浏览器优先级)。没有可操作工具就说明限制，不强制安装 `agent-browser`。
- **截图**：必须来自实际页面，排除登录页、加载页和空白页；不用模拟终端图或 `verified:true` 单独宣称通过。
- **通道独立**：纯页面任务不以 CLI 初始化为前提；纯 API 查询与写入仍用 CLI。预览打开成功不等于已登录、编辑器就绪或保存成功。

## 会话与输出

私有部署通过 `SHIMO_NET_ORIGIN=https://<客户域名>` 或 `init --origin` 指定。API 通道用已存 CLI 凭据，页面通道用所选浏览器的会话，两者相互独立。默认用 `--json` 解析结果，只报告用户需要的内容、文件链接、可核对的结果与限制；无权限和验证失败如实说明。会话与 401 处理细节见[安装说明](../install.md)。
