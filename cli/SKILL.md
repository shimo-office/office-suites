---
name: shimo-net-cli
description: 操作 shimo.net（含客户私有部署）时使用：用 shimo-net 命令行直连，搜索文件与空间、上传/导入、拉取下载、权限巡检、新建改名、读正文、常用文件夹别名。涉及文件锁定、限制编辑、高级权限、成员级授权和彻底删除时必须转石墨网页人工操作。
whenToUse: 用户要用石墨 shimo.net 找文件/空间、上传或导入、拉取下载、权限巡检、新建/改名、读文档、记常用文件夹时；高风险权限和不可逆操作必须人工页面处理。
version: 0.9.1
agent_created: true
---

# shimo-net CLI

仓库内用根目录 `./shimo`；安装后用 PATH 里的 `shimo-net`。

## 检查与登录

```bash
shimo-net schema
shimo-net whoami        # 未登录或 401 -> shimo-net login（Chrome → Keychain / Windows 凭据管理器），成功后重试原命令
shimo-net doctor
```

私有部署：

```bash
shimo-net init --origin https://<客户域名>
# 或临时：SHIMO_NET_ORIGIN=https://<客户域名> shimo-net ...
```

任意命令加 `--relogin`，遇 401 会自动打开登录并在成功后重试原命令。不要把密码 / cookie 粘进对话。

## 当前能力（28 个场景、45 项能力）

| 场景 | 命令 |
|---|---|
| 搜索 | `shimo-net search "关键词" --json` |
| 空间/目录 | `shimo-net spaces`、`shimo-net ls <链接\|@别名> --json` |
| 文件出处 | `shimo-net info <链接\|@别名> --json` |
| 上传（问位置） | `shimo-net upload <文件> [--to desktop\|空间\|文件夹\|@别名]` |
| 导入+权限 | `shimo-net import <文件> --to <位置>`；权限必须在石墨网页人工设置 |
| 拉取 | `shimo-net pull <链接> [--out <目录>] --json` |
| 新建 | `shimo-net new <名称> --type doc\|sheet\|slide --to <位置>` |
| 改名 | `shimo-net rename <链接> <新名字>` |
| 删除/回收站/恢复 | `delete`（只进回收站） / `trash` / `restore`；彻底删除不提供，需网页手动操作 |
| 权限巡检 | `shimo-net audit <文件夹\|@别名>` |
| 批量改权限 | CLI 禁止处理；打开石墨网页逐项人工确认和设置 |
| 读正文摘要 | `shimo-net read <链接> [--max-chars 12000]` |
| 周报 | `shimo-net ls <链接> --since 7d` + `shimo-net read` |
| 附件下载 | `shimo-net get <链接> [--out <目录>]` |
| 常用文件夹别名 | `shimo-net fav add <别名> <链接> --note 特征` → 之后用 `@别名` |
| 只看与我相关的内容 | `shimo-net phase2 <链接> --action mentions-list\|comments-list`、`subscriptions` |
| 留痕与版本 | `shimo-net open <链接>` 后用 computer-use 操作历史面板 |
| 文字与段落 | `shimo-net open <链接>` 后由 computer-use 操作菜单和选区 |
| 文内表格与图片 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 评论与提及 | `comments-list` 读取；添加评论和 @ 提及走页面 |
| 预览与打印 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 文档重排全文 | `shimo-net open <链接>` 后由 computer-use 调整 |
| 表格写入 | `shimo-net open <链接>` 后用 computer-use 操作表格 |
| 表格整理 | `shimo-net open <链接>` 后用 computer-use 操作冻结、排序、筛选和单元格底色 |
| 表格清洗 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 表格算与看数 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 表格校验与格式 | `shimo-net open <链接>` 后用 computer-use 操作 |
| 表格输出 | 导出使用异步导出 API；打印由 computer-use 操作 |
| 打开链接（免登录） | `shimo-net open <链接>`：用 CLI 已登录的 Chrome 配置打开 |

## 必须遵守

1. **上传/导入先确认位置**：桌面 / 空间 / 文件夹；没有目标就列出候选让用户选，不要瞎猜。
2. **导入后必须转网页人工设置权限**（7 档），不要默认公开；`upload` 是云文件不参与分享设置。
3. 文件锁定、限制编辑、高级权限、成员级授权和彻底删除均**禁止由 CLI 处理**，遇到这些请求必须打开石墨网页，由人工确认和操作；删除命令只进回收站。
4. 只支持 md / doc(x) / xls(x) / ppt(x)，其他直说。
5. 无权限如实说；大目录先 `ls` 再 `pull`；`--json` 便于解析。
6. 用户提过的常用文件夹用 `fav add` 记成 `@别名`，下次直接用。

## 编辑器与协作能力

当前直接可用的是 @人/评论列表 API、异步文件导出和订阅文件扫描；历史、格式、评论添加、表格编辑（包括单元格底色）、预览打印等操作使用内联浏览器配合 computer-use。文件锁定、限制编辑、高级权限、成员级授权和彻底删除严格禁止 CLI 处理，必须由人工进入石墨网页操作：

```bash
shimo-net phase2 <链接|guid|@别名> --action mentions-list|comments-list --json
```

`phase2` 是兼容入口。CLI 会在隔离 Chrome 中打开页面，通过 CDP 注入 Keychain 会话并调用同源 API，把 sid/cookie 从输出中排除；API 读取成功后返回截图绝对路径。页面 SDK 未暴露或动作没有可用 API 时，优先在 agent 内联浏览器中打开目标页面并由 computer-use 操作；没有内联浏览器时使用 `shimo-net open <链接>`，CLI 会从 Keychain 注入会话；只有这两种方式都不可用时才使用 `shimo-net open <链接> --default`。

订阅更新：

```bash
shimo-net subscriptions --json
shimo-net subscriptions --open 1
```

该命令逐文件查询 `/api/v1/files/{fileGuid}/update_subscription`，先列已订阅文件，再按用户选择打开对应文档；没有服务端全量订阅列表或最近更新正文接口时，不要猜测结果。

浏览器操作固定步骤：先 `whoami --json`，401 时 `login` 后重试；所有 agent 优先使用内联浏览器；没有内联浏览器时使用 `shimo-net open <链接>`，不要直接把页面导航到登录地址；CLI 专用浏览器会读取 Keychain 并注入 cookie；只有最后才使用 `--default`，此模式连接操作系统现有浏览器，无法注入到已有进程，若仍显示登录页再让用户在该浏览器登录一次。没有 API 时使用 computer-use 直接操作页面菜单、工具栏和选区；单元格历史必须先让用户说明行号和列号；单元格底色直接通过表格格式菜单操作；写操作先确认目标和影响范围；不要复制 sid；不要把 sid/cookie 写入日志或消息；截图返回绝对路径；失败要如实说明并保留失败截图。
