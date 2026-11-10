---
name: shimo-net-cli
description: 操作 shimo.net（含客户私有部署）时使用：用 shimo-net 命令行直连（Python 版，零 Node），搜索文件与空间、上传/导入、拉取下载、权限、新建改名删除、读正文、常用文件夹别名。用户说“找一下 XX”“传上去”“导到 XX”“拉下来”“设成企业内可评论”时触发。
whenToUse: 用户要用石墨 shimo.net 找文件/空间、上传或导入、拉取下载、改权限、新建/改名/删除、读文档、记常用文件夹时。
---

# shimo-net CLI（Python 版，无 MCP）

仓库内用 `./cli/shimo`（或根目录 `./shimo`）；安装后用 PATH 里的 `shimo-net`。只依赖 Python 3.9+，**不需要 Node**。

## 检查与登录

```bash
shimo-net schema
shimo-net whoami        # 未登录或 401 -> shimo-net login（Chrome → Keychain），成功后重试原命令
shimo-net doctor
```

私有部署：`SHIMO_NET_ORIGIN=https://<客户域名> shimo-net ...`。不要把密码/cookie 粘进对话。

## 15 个场景

| 场景 | 命令 |
|---|---|
| 搜索 | `shimo-net search "关键词" --json` |
| 空间/目录 | `shimo-net spaces`、`shimo-net ls <链接\|@别名> --json` |
| 文件出处 | `shimo-net info <链接\|@别名> --json` |
| 上传（问位置） | `shimo-net upload <文件> [--to desktop\|空间\|文件夹\|@别名]` |
| 导入+权限 | `shimo-net import <文件> --to <位置>` → 问权限 → `shimo-net share <链接> <模式>` |
| 拉取 | `shimo-net pull <链接> --out <目录> --json` |
| 新建 | `shimo-net new <名称> --type doc\|sheet\|slide --to <位置>` |
| 改名 | `shimo-net rename <链接> <新名字>` |
| 删除/回收站/恢复 | `delete`（只进回收站） / `trash` / `restore`；彻底删除不提供，需网页手动操作 |
| 权限巡检 | `shimo-net audit <文件夹\|@别名>` |
| 批量改权限 | `shimo-net share <链接1> <链接2> … <模式>` |
| 读正文摘要 | `shimo-net read <链接> [--max-chars 12000]` |
| 周报 | `shimo-net ls <链接> --since 7d` + `shimo-net read` |
| 附件下载 | `shimo-net get <链接> --out <目录>` |
| 常用文件夹别名 | `shimo-net fav add <别名> <链接> --note 特征` → 之后用 `@别名` |
| 打开链接（免登录） | `shimo-net open <链接>`：用 CLI 已登录的 Chrome 配置打开 |

## 必须遵守

1. **上传/导入先确认位置**：桌面 / 空间 / 文件夹；没有目标就列出候选让用户选，不要瞎猜。
2. **导入后必须问权限**（7 档），不要默认公开；`upload` 是云文件不参与分享设置。
3. 删除只进回收站；**CLI 不提供彻底删除**，遇到“彻底删”要引导用户去石墨网页回收站手动操作。
4. 只支持 md / doc(x) / xls(x) / ppt(x)，其他直说。
5. 无权限如实说；大目录先 `ls` 再 `pull`；`--json` 便于解析。
6. 用户提过的常用文件夹用 `fav add` 记成 `@别名`，下次直接用。
