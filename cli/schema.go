package main

func schema() map[string]any {
	cmds := []map[string]any{
		{"name": "init", "usage": "shimo-net init [--origin https://域名] [--login]", "desc": "初始化产品网址并校验接口可达"},
		{"name": "config", "usage": "shimo-net config [show|set <url>|clear]", "desc": "查看/修改产品网址（config.json）"},
		{"name": "search", "usage": "shimo-net search <keyword> [--limit 20]", "desc": "搜文件/文件夹/空间，返回地址"},
		{"name": "spaces", "usage": "shimo-net spaces [--limit 300]", "desc": "列出我参与的空间"},
		{"name": "ls", "usage": "shimo-net ls <链接|guid|@别名> [--since 7d] [--limit 200]", "desc": "列文件夹/空间里的文件"},
		{"name": "info", "usage": "shimo-net info <链接|guid|@别名>", "desc": "文件类型/权限/所在路径"},
		{"name": "upload", "usage": "shimo-net upload <本地文件> [--to desktop|空间|文件夹|@别名]", "desc": "原样上传云文件；不给 --to 会列出可选位置"},
		{"name": "import", "usage": "shimo-net import <本地文件> --to <位置> [--title NAME] [--share MODE]", "desc": "md→文档, doc→文稿, xlsx→表格, pptx→幻灯片"},
		{"name": "new", "usage": "shimo-net new <name> [--type doc|sheet|slide] [--to <位置>]", "desc": "新建空白文件"},
		{"name": "rename", "usage": "shimo-net rename <链接|guid|@别名> <新名字>", "desc": "改名"},
		{"name": "share", "usage": "shimo-net share <链接|guid|@别名> [更多…] <MODE>", "desc": "设置权限（可批量）"},
		{"name": "audit", "usage": "shimo-net audit <文件夹|@别名>", "desc": "权限巡检"},
		{"name": "delete", "usage": "shimo-net delete <链接|guid|@别名>", "desc": "删除到回收站"},
		{"name": "trash", "usage": "shimo-net trash", "desc": "回收站列表"},
		{"name": "restore", "usage": "shimo-net restore <guid>", "desc": "从回收站恢复"},
		{"name": "read", "usage": "shimo-net read <链接|guid|@别名> [--max-chars 12000]", "desc": "读正文（文档 md；文稿/表格/幻灯片本地解析）"},
		{"name": "pull", "usage": "shimo-net pull <链接|@别名> --out <dir> [--since 7d]", "desc": "下载整个文件夹"},
		{"name": "get", "usage": "shimo-net get <链接|guid|@别名> --out <dir>", "desc": "下载单个文件"},
		{"name": "fav", "usage": "shimo-net fav <list|add|rm|show> …", "desc": "常用文件夹别名（下次直接说 @别名）"},
		{"name": "open", "usage": "shimo-net open <链接|guid|@别名>", "desc": "用 CLI 已登录的 Chrome 配置打开链接（免重复登录）"},
		{"name": "whoami", "usage": "shimo-net whoami [--json]", "desc": "当前账号"},
		{"name": "doctor", "usage": "shimo-net doctor [--json]", "desc": "自检（python/chrome/凭据/origin）"},
	}
	return map[string]any{
		"name": "shimo-net", "version": version, "transport": "cli", "runtime": "go",
		"auth":       map[string]any{"login": "shimo-net login", "whoami": "shimo-net whoami", "storage": "macOS Keychain", "service": keychainService, "envOverride": "SHIMO_NET_COOKIE"},
		"origin":     map[string]any{"default": defaultOrigin, "envOverride": "SHIMO_NET_ORIGIN", "configFile": configPath(), "commands": []string{"init", "config show", "config set <url>"}},
		"favorites":  map[string]any{"file": favPath(), "use": "@别名", "commands": []string{"fav list", "fav add", "fav rm", "fav show"}},
		"commands":   cmds,
		"shareModes": keys(shareModes), "shareModeLabels": shareModes,
		"importTypes": importTypes, "createTypes": createTypes,
		"scenarios": []string{"搜索", "空间/目录", "文件出处", "导入+权限", "拉取", "新建", "改名", "删除/回收站/恢复",
			"权限巡检", "批量改权限", "读正文摘要", "周报", "附件下载", "上传（问位置）", "常用文件夹别名"},
		"errors": map[string]any{"format": "非 0 退出码 + stderr 文本；--json 时 stderr 为 {\"ok\":false,\"error\":\"...\"}"},
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func helpText() string {
	return `shimo-net ` + version + ` —— shimo.net 接口（15 个场景）

用法:
  shimo-net init [--origin URL]                    初始化产品网址并校验可达
  shimo-net config [show|set URL]                  查看 / 修改产品网址
  shimo-net login [--origin URL]                   登录（Chrome → Keychain）
  shimo-net logout                                 退出并删除本机凭据
  shimo-net whoami | doctor                        账号 / 自检
  shimo-net search <关键词> [--limit 20]            1 搜索
  shimo-net spaces [--limit 300]                   2 我的空间清单
  shimo-net ls <链接|@别名> [--since 7d]             2/12 列目录
  shimo-net info <链接|@别名>                       3 文件类型/权限/路径
  shimo-net upload <本地文件> [--to <位置>]          14 上传（不给 --to 会问：桌面/空间/文件夹）
  shimo-net import <本地文件> --to <位置>            4 导入并转在线文档
  shimo-net pull <文件夹|空间> --out <目录>           5 拉取
  shimo-net new <名称> [--type doc|sheet|slide]      6 新建
  shimo-net rename <链接> <新名字>                   7 改名
  shimo-net delete / trash / restore                 8 删除（只进回收站）/ 回收站 / 恢复
  shimo-net audit <文件夹|@别名>                     9 权限巡检
  shimo-net share <链接> [更多…] <权限>              10 设置权限（可批量）
  shimo-net read <链接> [--max-chars N]              11 读正文
  shimo-net get <链接> --out <目录>                  13 下载单个文件
  shimo-net fav <list|add|rm|show> …                15 常用文件夹别名（下次直接 @别名）
  shimo-net open <链接|guid>                        用已登录的浏览器打开（免重复登录）
  shimo-net schema                                  命令/参数/JSON 结构

权限: private | enterprise_readonly | enterprise_commentable | enterprise_editable
      readonly | commentable | editable

全局: --json；SHIMO_NET_ORIGIN 指定私有部署域名；--relogin 遇 401 自动登录后重试
`
}
