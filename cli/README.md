# shimo-net

`shimo-net` 是 shimo.net（含客户私有部署）的命令行工具，覆盖 **15 个办公场景**。当前版本 **0.8.0**，Go 单二进制。

- 直连 `/api/v1`，不开端口、不跑常驻服务。
- 登录一次：Chrome 专用窗口 → macOS Keychain；过期就地重登。
- 数据类命令支持 `--json`，适合 Agent / 脚本调用。

> 仓库根目录的 [README](../README.md) 有完整的环境要求、Agent 接入和安全设计；本文件聚焦 CLI 用法。

## 安装

从仓库根目录：

```bash
./install.command
```

安装器会：

1. 有 Go 时本地构建，否则下载最新 release 二进制；
2. 安装到 `~/.local/bin/shimo-net`；
3. 如存在 `~/.dsh`，把技能装到 `~/.dsh/skills/shimo-net-cli`；
4. 跑一次 `doctor`。

从源码构建：

```bash
go build -trimpath -o bin/shimo-net ./cli
bin/shimo-net version
```

需要 Go 1.22+；运行二进制本身不需要 Go。

如果提示 `command not found`，把 `~/.local/bin` 加入 `PATH`。

## 第一次配置

```bash
shimo-net init                    # 默认 https://shimo.net
shimo-net init --origin https://<客户域名>
```

`init` 会把网址写入 `~/.shimo-net-cli/config.json`，并请求 `GET /api/v1/users/me` 确认接口可达（返回 401 也算可达）。

查看 / 修改：

```bash
shimo-net config show
shimo-net config set https://<客户域名>
shimo-net config clear
```

`SHIMO_NET_ORIGIN` 环境变量优先级最高，适合临时切换。

## 登录

```bash
shimo-net login
shimo-net whoami
shimo-net doctor
```

- `login` 打开无地址栏的 Chrome 窗口，停在 `{origin}/login`；
- 首次使用没有 `config.json` 时，会提示输入产品网址；
- 登录成功后窗口关闭，会话写入 Keychain；
- 登录过期：重新 `shimo-net login`，成功后重试原命令，网址不用重配；
- 任意命令加 `--relogin`，遇 401 自动登录并重试；
- 不要把密码 / cookie 贴进对话、Issue 或日志。

退出并删除本机凭据：

```bash
shimo-net logout
```

## 15 个场景

| # | 场景 | 命令 |
|---|---|---|
| 1 | 搜索文件 / 文件夹 / 空间 | `shimo-net search "<关键词>" [--limit 20]` |
| 2 | 我的空间 / 空间里有什么 | `shimo-net spaces [--limit 300]`、`shimo-net ls <空间\|文件夹\|@别名> [--since 7d]` |
| 3 | 文件出处（类型 / 权限 / 路径） | `shimo-net info <链接\|@别名>` |
| 4 | 导入 + 权限 | `shimo-net import <文件> --to <位置>` + `shimo-net share <链接> <权限>` |
| 5 | 拉取文件夹 | `shimo-net pull <链接\|@别名> --out <目录> [--since 7d]` |
| 6 | 新建空白文档 / 表格 / 幻灯片 | `shimo-net new <名称> --type doc\|sheet\|slide --to <位置>` |
| 7 | 改名 | `shimo-net rename <链接> "<新名字>"` |
| 8 | 删除 / 回收站 / 恢复 | `shimo-net delete`（只进回收站）、`trash`、`restore`；彻底删除到石墨网页手动操作 |
| 9 | 权限巡检 | `shimo-net audit <文件夹\|@别名>` |
| 10 | 批量改权限 | `shimo-net share <链接1> <链接2> … <权限>` |
| 11 | 读正文 / 摘要 | `shimo-net read <链接> [--max-chars 12000]` |
| 12 | 周报 | `shimo-net ls <链接> --since 7d` + `shimo-net read` |
| 13 | 附件 / 云文件下载 | `shimo-net get <链接> --out <目录>` |
| 14 | 上传（问传到哪里） | `shimo-net upload <文件> [--to desktop\|空间\|文件夹\|@别名]`；不给 `--to` 会列出可选位置 |
| 15 | 常用文件夹别名 | `shimo-net fav add <别名> <链接> --note 特征`，之后任意命令用 `@别名` |
| — | 用已登录浏览器打开链接 | `shimo-net open <链接\|guid>`（复用 CLI 的 Chrome 配置，免重复登录） |

通用命令：`init` / `config` / `login` / `logout` / `whoami` / `doctor` / `schema` / `version`；全局 `--json`、`--relogin`。

## 常用位置（@别名）

```bash
shimo-net fav add ai https://shimo.net/folder/xxxx --note "市场部常用 · AI 方案"
shimo-net ls @ai
shimo-net upload 报告.pdf --to @ai
shimo-net fav list
shimo-net fav show ai
shimo-net fav rm ai
```

别名存在 `~/.shimo-net-cli/favorites.json`，只有别名和链接，不含账号信息。任何接受 `链接|guid` 的参数都可以用 `@别名`。

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

```bash
shimo-net share <链接> enterprise_commentable
shimo-net share <链接1> <链接2> readonly     # 批量
```

后三档是公开链接，任何拿到链接的人都能访问；导入后先确认再设置。

## JSON 与退出码

```bash
shimo-net search "关键词" --json
shimo-net whoami --json
shimo-net doctor --json
```

- 成功：`exit 0`，stdout 为 JSON；
- 失败：`exit 1`，stderr 为 `{"ok":false,"error":"..."}`；
- 查看全部命令与参数：`shimo-net schema`。

## 行为约定

- `upload` = 原样上传云文件；`import` = 转成在线文档 / 文稿 / 表格 / 幻灯片。
- 上传 / 导入前先确认目标位置；导入后先问权限。
- **删除只进回收站；CLI 不提供彻底删除**，要彻底删请在石墨网页回收站手动操作。
- 只支持 `md / doc(x) / xls(x) / ppt(x)`；其他格式直说。
- 无权限如实说；大目录先 `ls` 再 `pull`；批量操作控制并发。
- 凭据：macOS 走 Keychain；非 macOS / Keychain 不可用时回退到 `~/.shimo-net-cli/credentials.json`（`0600`），需自行评估风险。CI 可用 `SHIMO_NET_COOKIE`。

> 完整安装 / 登录 / 排错见 [安装使用说明.md](安装使用说明.md)；场景话术见 [scenarios.md](scenarios.md)。
