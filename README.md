# 石墨办公套件 · CLI

**shimo-net 命令行接入层：15 个办公场景，直连 `/api/v1`，零 Node 依赖。**

`shimo-net` 把石墨办公套件（含客户私有部署）的日常操作做成命令：找文件、看空间、上传、导入、拉取、权限巡检、新建改名删除、读正文、周报、常用文件夹别名。适合人直接在终端用，也适合 Agent / 脚本通过 `--json` 调用。

- **不依赖 Node.js**：Python 3.9+ 标准库即可，无第三方包。
- **不走 MCP**：CLI 直连 API，不开端口、不跑常驻服务。
- **一次登录**：Chrome 专用窗口登录，会话存 macOS Keychain；TTL 过期就地重登。
- **支持私有化**：`SHIMO_NET_ORIGIN=https://<客户域名>` 切换，不另装一套。
- **可脚本化**：数据命令支持 `--json`，失败时 stderr 输出结构化错误。

> 当前版本 **0.6.0**。macOS 为主支持平台；Linux 为实验性支持（需自行准备 Chrome / Python）。版本变更见 [CHANGELOG.md](CHANGELOG.md)。

---

## 快速开始

### 环境要求

| 项 | 要求 |
|---|---|
| 操作系统 | macOS 12+（主支持）；Linux（实验） |
| Python | 3.9 及以上（macOS 通常自带 `/usr/bin/python3`） |
| 浏览器 | Google Chrome / Chromium / Microsoft Edge（登录与 `open` 用） |
| 网络 | 能访问 `shimo.net` 或客户私有部署域名 |

```bash
python3 --version          # 需要 3.9+
```

### 安装

```bash
git clone https://github.com/shimo-office/office-suites.git
cd office-suites
./install.command
```

安装器会：

1. 把程序复制到 `~/.shimo-net-py/app`；
2. 生成命令 `~/.local/bin/shimo-net`；
3. 如存在 `~/.dsh`，把技能装到 `~/.dsh/skills/shimo-net-cli`；
4. 跑一次 `doctor` 自检。

如果 `shimo-net: command not found`：

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

不安装、直接在仓库内运行：

```bash
./shimo version
./shimo schema
./shimo help
```

### 登录与自检

```bash
shimo-net login          # 打开 Chrome 专用窗口，登录石墨
shimo-net whoami         # 确认账号
shimo-net doctor         # python / chrome / 凭据 / origin 四项
```

`login` 会打开一个无地址栏的 Chrome 窗口停在 `{origin}/login`；登录成功后窗口关闭，凭据写入 Keychain。**不要把密码或 cookie 粘进对话 / issue / 日志。**

### 第一个场景

```bash
shimo-net search "AI Agent"
shimo-net spaces
shimo-net fav add ai https://shimo.net/folder/xxxx --note "示例文件夹"
shimo-net ls @ai
shimo-net import 周报.md --to @ai
shimo-net share <链接> enterprise_commentable
```

---

## 15 个场景

| # | 场景 | 命令 | 说明 |
|---|---|---|---|
| 1 | 搜索文件 / 文件夹 / 空间 | `shimo-net search "<关键词>" [--limit 20]` | 返回标题、类型、路径、链接 |
| 2 | 我的空间 / 目录 | `shimo-net spaces [--limit 300]`、`shimo-net ls <链接\|@别名> [--since 7d]` | 先看全局再看局部 |
| 3 | 文件类型 / 权限 / 路径 | `shimo-net info <链接\|@别名>` | 类型、七档权限、完整路径 |
| 4 | 导入并设置权限 | `shimo-net import <文件> --to <位置>` + `shimo-net share <链接> <权限>` | md / doc(x) / xls(x) / ppt(x) |
| 5 | 拉取文件夹到本地 | `shimo-net pull <链接\|@别名> --out <目录> [--since 7d]` | 在线文档导出 + 附件原样 + `manifest.json` |
| 6 | 新建空白文档 / 表格 / 幻灯片 | `shimo-net new <名称> --type doc\|sheet\|slide --to <位置>` | 默认私有 |
| 7 | 改名 | `shimo-net rename <链接> "<新名字>"` | 链接不变 |
| 8 | 删除 / 回收站 / 恢复 | `shimo-net delete`、`shimo-net trash`、`shimo-net restore <guid>` | **只进回收站**，不提供彻底删除 |
| 9 | 权限巡检 | `shimo-net audit <文件夹\|@别名>` | 公开 / 企业内 / 私有分组 |
| 10 | 批量改权限 | `shimo-net share <链接1> <链接2> … <权限>` | 逐条返回成功 / 失败 |
| 11 | 读正文 / 摘要 | `shimo-net read <链接> [--max-chars 12000]` | 文档导出 md；文稿 / 表格 / 幻灯片本地解析 |
| 12 | 周报 / 一页纸 | `shimo-net ls <链接> --since 7d` + `shimo-net read` | 按更新时间筛 |
| 13 | 附件 / 云文件下载 | `shimo-net get <链接> --out <目录>` | 原样下载，不转格式 |
| 14 | 上传（先问传到哪里） | `shimo-net upload <文件> [--to desktop\|空间\|文件夹\|@别名]` | 不给 `--to` 会列出可选位置 |
| 15 | 常用文件夹别名 | `shimo-net fav add <别名> <链接> --note 特征` | 之后任意命令用 `@别名` |

通用命令：`login`、`logout`、`whoami`、`doctor`、`schema`、`version`、`open`。完整参数见 [cli/scenarios.md](cli/scenarios.md) 或 `shimo-net schema`。

### 上传与导入的区别

| | `upload` | `import` |
|---|---|---|
| 结果 | 原样云文件（内容不变） | 转在线文档 / 文稿 / 表格 / 幻灯片 |
| 链接 | `/files/<guid>` | `/docs`、`/docx`、`/sheets`、`/presentation/<guid>` |
| 适用 | 附件、压缩包、图片等 | md、doc(x)、xls(x)、ppt(x) |

`upload` 不给 `--to` 时会列出桌面 / 空间 / 文件夹 / `@别名`，确认后再传。

---

## 权限（7 档）

| 值 | 含义 |
|---|---|
| `private` | 私有（仅自己） |
| `enterprise_readonly` | 企业内只读 |
| `enterprise_commentable` | 企业内可评论 |
| `enterprise_editable` | 企业内可编辑 |
| `readonly` | 公开链接 · 只读 |
| `commentable` | 公开链接 · 可评论 |
| `editable` | 公开链接 · 可编辑 |

后三档是**公开链接**，任何拿到链接的人都能访问；导入后先确认再设置：

```bash
shimo-net share <链接> enterprise_commentable
shimo-net share <链接1> <链接2> readonly
```

---

## 客户私有部署

默认连 `https://shimo.net`。客户环境只改环境变量：

```bash
export SHIMO_NET_ORIGIN=https://<客户域名>
shimo-net login
shimo-net whoami
shimo-net doctor
```

登录页、API、输出里的链接都会使用该域名。建议写进 `~/.zshrc`，或每次命令前临时加：

```bash
SHIMO_NET_ORIGIN=https://<客户域名> shimo-net search "关键词"
```

私有化环境与 `shimo.net` 的数据不互通；切换域名后需要重新登录。

---

## JSON、退出码与自动化

```bash
shimo-net search "关键词" --json
shimo-net whoami --json
shimo-net doctor --json
```

- 成功：`exit 0`，stdout 为 JSON；
- 失败：`exit 1`，stderr 为 `{"ok":false,"error":"..."}`；
- 数据类命令都支持 `--json`（`schema` 本身输出 JSON）；`login` / `logout` / `help` 以交互为主。

适合 Agent / 脚本的调用顺序：

1. `shimo-net doctor --json` 判断环境；
2. `shimo-net whoami --json` 判断登录态；
3. 执行业务命令；
4. 读回 `info` / `ls` / `audit` 验证结果，而不是只看命令退出码。

---

## 安全设计

- **凭据不落明文（macOS）**：cookie 存 `shimo-net-cli` 这个 Keychain generic password；`~/.shimo-net-cli/credentials.json` 只记录 origin / 保存时间，不记录账号。
- **非 macOS / Keychain 不可用时**：回退到 `~/.shimo-net-cli/credentials.json`（目录 `0700`、文件 `0600`），此时 cookie 会落盘，需自行评估风险。
- **CI / 临时环境**：可用 `SHIMO_NET_COOKIE` 环境变量传入会话，避免写盘；注意不要进 shell history / 日志。
- **删除只进回收站**：CLI 不提供彻底删除；`purge` 命令会明确拒绝并提示到石墨网页回收站手动操作。
- **最小权限 + 先确认**：上传 / 导入先确认位置；导入后先问权限；批量改权限逐条返回结果。
- **资源提醒**：搜索、批量读取 / 导出、批量改权限、批量下载会持续调用服务端接口，产生计算、存储、带宽开销。避免高频轮询、全量扫描、大批量并发；建议在网关侧做频率 / 并发 / 批量上限。

安全漏洞报告方式见 [SECURITY.md](SECURITY.md)。

---

## 仓库结构

```text
.
├── README.md                 # 本文件：仓库概览、快速开始、命令总览
├── install.command           # 根目录安装入口（转调 cli/install.command）
├── shimo                     # 根目录开发入口（转调 cli/shimo）
├── Makefile                  # make test
├── SECURITY.md               # 安全策略与漏洞报告
├── CHANGELOG.md              # 版本记录
└── cli/
    ├── README.md             # CLI 详细用法
    ├── install.command       # 安装器（复制到 ~/.shimo-net-py/app）
    ├── shimo                 # 仓库内启动器
    ├── bin/shimo.py          # 单文件实现（Python 3.9+ 标准库）
    ├── SKILL.md              # Agent 技能定义（DSH / Codex 等）
    ├── scenarios.md          # 15 个场景与自然语言示例
    ├── 安装使用说明.md        # 面向同事的安装 / 登录 / 排错说明
    ├── examples/             # 导入验证用的 md / docx / xlsx / pptx
    └── tests/smoke.sh        # 无网络冒烟测试
```

---

## 开发

```bash
# 仓库内直接运行
./cli/shimo version
./cli/shimo schema
./cli/shimo help

# 冒烟测试（不访问网络、不需要登录）
make test
```

实现约束：

- 只用 Python 3.9+ 标准库，不引入第三方依赖；
- 新增命令时同步更新 `schema()`、`HELP`、`DISPATCH`、`cli/README.md`、`cli/scenarios.md`；
- 写操作保持“先确认、可恢复、可读回”的原则；
- 凭据只走 Keychain / 0600 文件 / 环境变量三种来源，不写入日志。

---

## 已知限制

- **平台**：macOS 主支持；Linux 实验性支持（Chrome 路径、Keychain 行为需按发行版验证）。
- **彻底删除不提供**：删除只进回收站，彻底删除需到石墨网页手动操作。
- **部分接口未接入**：公开链接密码 / 有效期、邀请协作者、复制 / 移动、语义检索、空间公告等。
- **私有化数据隔离**：与 `shimo.net` 数据不互通，切换域名需重新登录。
- **登录依赖本机 Chrome / Chromium / Edge**；无浏览器环境可用 `SHIMO_NET_COOKIE`（需自行获取会话）。

---

## 文档

- [CLI 详细用法](cli/README.md)
- [安装与使用说明](cli/安装使用说明.md)
- [15 个场景与自然语言示例](cli/scenarios.md)
- [Agent 技能定义](cli/SKILL.md)
- 命令结构：`shimo-net schema`

---

## 许可与联系

本仓库代码用于石墨办公套件交付与客户私有部署，默认保留所有权利；对外分发或商用授权请联系石墨办公套件团队。

问题反馈：在本仓库提 Issue；安全相关问题请按 [SECURITY.md](SECURITY.md) 私下报告。
