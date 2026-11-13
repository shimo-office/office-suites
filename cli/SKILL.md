---
name: shimo-net-cli
description: 操作 shimo.net（含客户私有部署）时使用：用 shimo-net 命令行直连，搜索文件与空间、上传/导入、拉取下载、权限、新建改名删除、读正文、常用文件夹别名。用户说“找一下 XX”“传上去”“导到 XX”“拉下来”“设成企业内可评论”时触发。
whenToUse: 用户要用石墨 shimo.net 找文件/空间、上传或导入、拉取下载、改权限、新建/改名/删除、读文档、记常用文件夹时。
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
| 导入+权限 | `shimo-net import <文件> --to <位置>` → 问权限 → `shimo-net share <链接> <模式>` |
| 拉取 | `shimo-net pull <链接> [--out <目录>] --json` |
| 新建 | `shimo-net new <名称> --type doc\|sheet\|slide --to <位置>` |
| 改名 | `shimo-net rename <链接> <新名字>` |
| 删除/回收站/恢复 | `delete`（只进回收站） / `trash` / `restore`；彻底删除不提供，需网页手动操作 |
| 权限巡检 | `shimo-net audit <文件夹\|@别名>` |
| 批量改权限 | `shimo-net share <链接1> <链接2> … <模式>` |
| 读正文摘要 | `shimo-net read <链接> [--max-chars 12000]` |
| 周报 | `shimo-net ls <链接> --since 7d` + `shimo-net read` |
| 附件下载 | `shimo-net get <链接> [--out <目录>]` |
| 常用文件夹别名 | `shimo-net fav add <别名> <链接> --note 特征` → 之后用 `@别名` |
| 只看与我相关的内容 | `shimo-net phase2 <链接> --action mentions-list\|comments-list`、`subscriptions` |
| 留痕与版本 | `shimo-net open <链接> --default` 后用 computer-use 操作历史面板 |
| 文字与段落 | `shimo-net open <链接> --default` 后由 computer-use 操作菜单和选区 |
| 文内表格与图片 | `shimo-net open <链接> --default` 后由 computer-use 操作 |
| 评论与提及 | `comments-list` 读取；添加评论和 @ 提及走页面 |
| 预览与打印 | `shimo-net open <链接> --default` 后由 computer-use 操作 |
| 文档重排全文 | `shimo-net open <链接> --default` 后由 computer-use 调整 |
| 表格写入 | `shimo-net open <链接> --default` 后用 computer-use 操作表格 |
| 表格整理 | `shimo-net open <链接> --default` 后用 computer-use 操作冻结、排序、筛选 |
| 表格清洗 | `shimo-net open <链接> --default` 后由 computer-use 操作 |
| 表格算与看数 | `shimo-net open <链接> --default` 后由 computer-use 操作 |
| 表格校验与格式 | `shimo-net open <链接> --default` 后用 computer-use 操作 |
| 表格输出 | 导出使用异步导出 API；打印由 computer-use 操作 |
| 打开链接（免登录） | `shimo-net open <链接>`：用 CLI 已登录的 Chrome 配置打开 |

## 必须遵守

1. **上传/导入先确认位置**：桌面 / 空间 / 文件夹；没有目标就列出候选让用户选，不要瞎猜。
2. **导入后必须问权限**（7 档），不要默认公开；`upload` 是云文件不参与分享设置。
3. 删除只进回收站；**CLI 不提供彻底删除**，遇到“彻底删”要引导用户去石墨网页回收站手动操作。
4. 只支持 md / doc(x) / xls(x) / ppt(x)，其他直说。
5. 无权限如实说；大目录先 `ls` 再 `pull`；`--json` 便于解析。
6. 用户提过的常用文件夹用 `fav add` 记成 `@别名`，下次直接用。

## 编辑器与协作能力

当前直接可用的是 @人/评论列表 API、异步文件导出和订阅文件扫描；历史、格式、评论添加、表格编辑、预览打印等操作使用默认浏览器配合 computer-use：

```bash
shimo-net phase2 <链接|guid|@别名> --action mentions-list|comments-list --json
```

`phase2` 是兼容入口。CLI 会在隔离 Chrome 中打开页面，通过 CDP 注入 Keychain 会话并调用同源 API，把 sid/cookie 从输出中排除；API 读取成功后返回截图绝对路径。页面 SDK 未暴露或动作没有可用 API 时，立即切换到 `shimo-net open <链接> --default`，按 `shimo.net/desktop` 的真实控件完成操作并返回截图。

订阅更新：

```bash
shimo-net subscriptions --json
shimo-net subscriptions --open 1 --default
```

该命令逐文件查询 `/api/v1/files/{fileGuid}/update_subscription`，先列已订阅文件，再按用户选择打开对应文档；没有服务端全量订阅列表或最近更新正文接口时，不要猜测结果。

浏览器兜底的固定步骤：先 `whoami --json`，401 时 `login` 后重试；没有 API 时使用 computer-use 直接操作页面菜单、工具栏和选区；单元格历史必须先让用户说明行号和列号；写操作先确认目标和影响范围；默认浏览器没有登录态时让用户在该浏览器登录一次或改用编辑器入口，命令名 `phase2` 仅作兼容调用，不要复制 sid；不要把 sid/cookie 写入日志或消息；截图返回绝对路径；失败要如实说明并保留失败截图。
