package main

import "strconv"

func schema() map[string]any {
	scenarios := append([]string(nil), capabilityScenarioNames...)
	cmds := []map[string]any{
		{"name": "init", "usage": "shimo-net init [--origin https://域名] [--login]", "desc": "初始化产品网址并校验接口可达"},
		{"name": "config", "usage": "shimo-net config [show|set <url>|clear]", "desc": "查看/修改产品网址（config.json）"},
		{"name": "login", "usage": "shimo-net login [--origin https://域名] [--new]", "desc": "浏览器登录并按 origin 保存凭据；--new 使用全新临时配置，不复用旧账号"},
		{"name": "logout", "usage": "shimo-net logout", "desc": "退出并删除本机凭据"},
		{"name": "search", "usage": "shimo-net search <keyword> [--limit 20]", "desc": "搜文件/文件夹/空间，返回地址"},
		{"name": "spaces", "usage": "shimo-net spaces [--limit 300]", "desc": "列出我参与的空间"},
		{"name": "ls", "usage": "shimo-net ls <链接|guid|@别名> [--since 7d] [--limit 200]", "desc": "列文件夹/空间里的文件"},
		{"name": "info", "usage": "shimo-net info <链接|guid|@别名>", "desc": "文件类型/权限/所在路径"},
		{"name": "upload", "usage": "shimo-net upload <本地文件> [--to desktop|空间|文件夹|@别名]", "desc": "原样上传云文件；不给 --to 会列出可选位置"},
		{"name": "import", "usage": "shimo-net import <本地文件> --to <位置> [--title NAME]", "desc": "md→文档, doc→文稿, xlsx→表格, pptx→幻灯片；权限需人工页面操作"},
		{"name": "new", "usage": "shimo-net new <name> [--type doc|docx|sheet|slide|table|form|mindmap|flowchart] [--to <位置>]", "desc": "新建空白文件；doc=文档（document），docx=文稿（documentPro）"},
		{"name": "mkdir", "usage": "shimo-net mkdir <名称> [--to <父文件夹|@别名>]", "desc": "新建文件夹"},
		{"name": "rename", "usage": "shimo-net rename <链接|guid|@别名> <新名字>", "desc": "改名"},
		{"name": "share", "usage": "shimo-net share <链接|guid> <private|readonly|commentable|editable|enterprise_*>", "desc": "设置 7 档分享权限，写完重新读取核对"},
		{"name": "audit", "usage": "shimo-net audit <文件夹|@别名> [--limit 200]", "desc": "权限巡检"},
		{"name": "delete", "usage": "shimo-net delete <链接|guid|@别名>", "desc": "删除到回收站"},
		{"name": "trash", "usage": "shimo-net trash", "desc": "回收站列表"},
		{"name": "restore", "usage": "shimo-net restore <guid>", "desc": "从回收站恢复"},
		{"name": "read", "usage": "shimo-net read <链接|guid|@别名> [--max-chars 12000]", "desc": "读正文（文档 md；文稿/表格/幻灯片本地解析）"},
		{"name": "pull", "usage": "shimo-net pull <链接|@别名> [--out <dir>] [--since 7d] [--limit N]", "desc": "下载当前层文件；不支持的导出类型单独计数，不算失败"},
		{"name": "get", "usage": "shimo-net get <链接|guid|@别名> [--out <dir>] [--format md|docx|wps|pdf|jpg|xlsx|pptx] [--embed-images false]", "desc": "下载单个文件；--format 按文件类型导出（文档 md/docx/pdf/jpg，文稿 docx/wps/pdf，表格 xlsx，幻灯片 pptx/pdf）"},
		{"name": "preview", "usage": "shimo-net preview <链接|guid|@别名>", "desc": "输出 /files/<guid> 预览地址，供内置浏览器打开"},
		{"name": "fav", "usage": "shimo-net fav <list|add|rm|show> …", "desc": "常用文件夹别名（下次直接说 @别名）"},
		{"name": "open", "usage": "shimo-net open <链接|guid|@别名> [--default]", "desc": "打开链接；使用独立 open-profile，不注入会话或开启调试端口；--default 选择默认浏览器应用"},
		{"name": "editor", "usage": "shimo-net editor <链接|guid|@别名> --action <动作> [--payload JSON] [--screenshot PATH] [--verify|--no-verify]", "desc": "调用同源 API 或套件前端 SDK；历史/评论面板、保存版本、打印需 --verify；提交评论仍需页面操作"},
		{"name": "phase2", "usage": "shimo-net phase2 …", "aliasOf": "editor", "desc": "editor 的兼容命令别名"},
		{"name": "purge", "usage": "shimo-net purge <链接>", "desc": "按设计拒绝；彻底删除须到网页回收站人工操作"},
		{"name": "update", "usage": "shimo-net update [--check] [--json]", "desc": "检查或安装最新稳定版；原生程序校验 SHA-256 后替换，npm 安装通过 npm 更新"},
		{"name": "version", "usage": "shimo-net version [--json]", "desc": "输出 CLI 版本"},
		{"name": "phase3", "usage": "shimo-net phase3 <链接|guid|@别名> --action <动作> [--payload JSON] [--read-range 工作表!A1:D4] [--out 目录] [--verify]", "desc": "三期列明范围已验收，37.2 关联字段保留；逐项证据另列，artifact 仅检查本地包结构"},
		{"name": "subscriptions", "usage": "shimo-net subscriptions [--limit 200] [--depth 3] [--open 序号|guid] [--default]", "desc": "limit 限制扫描文件数，仅对扫描到的订阅文件排序；可打开浏览器"},
		{"name": "session", "usage": "shimo-net session <status|harvest|import|watch|logout> [--no-save] [--no-verify] [--interval 1s] [--once] [--source NAME] [--expected-revision N]", "desc": "查看登录态来源与共享会话；harvest 从本机浏览器复用；import 由宿主写入会话；watch 订阅变化；logout 标记共享退出"},
		{"name": "whoami", "usage": "shimo-net whoami [--json]", "desc": "当前账号"},
		{"name": "doctor", "usage": "shimo-net doctor [--json]", "desc": "自检（运行时/Chrome/凭据/origin）"},
	}
	editor := map[string]any{"status": "api-backed", "actions": phase2ActionNames(), "verifiedActionsMeaning": "historically exercised API routes, not verification of the current invocation", "browserRequiredActions": []string{"history-show", "history-hide", "comments-show", "comments-hide", "save-version", "print", "cell-history"}, "verifiedActions": []string{"mentions-list", "comments-list", "history-show", "comments-show", "save-version", "print", "cell-history", "sheet-format", "sheet-merge", "sheet-values", "sheet-update", "sheet-append", "sheet-insert", "sheet-delete"}, "pageOnlyActions": keys(phase2PageOnlyActions), "transport": "same-origin-api", "fallback": "按 docs/scenarios.md#浏览器优先级 选择宿主能力；内置浏览器优先，外部浏览器需用户授权"}
	return map[string]any{
		"name": "shimo-net", "version": version, "transport": "cli", "runtime": "go",
		"auth":       map[string]any{"login": "shimo-net login", "whoami": "shimo-net whoami", "storage": "macOS Keychain / Windows Credential Manager; fallback: 0600 credentials.json (Linux uses file storage)", "service": keychainService, "envOverride": "SHIMO_NET_COOKIE"},
		"origin":     map[string]any{"default": defaultOrigin, "envOverride": "SHIMO_NET_ORIGIN", "configFile": configPath(), "commands": []string{"init", "config show", "config set <url>"}},
		"favorites":  map[string]any{"file": favPath(), "use": "@别名", "commands": []string{"fav list", "fav add", "fav rm", "fav show"}},
		"commands":   cmds,
		"shareModes": keys(shareModes), "shareModeLabels": shareModes,
		"importTypes": importTypes, "createTypes": createTypes,
		"scenarios": scenarios, "scenarioCount": len(scenarios), "capabilityCount": capabilityCount, "capabilityCatalog": "docs/internal/capabilities.json", "capabilityCatalogSHA256": capabilityCatalogSHA256, "manualOnly": manualOnlyOperations,
		"editor": editor,
		"phase2": editor,
		"phase3": map[string]any{
			"status": "accepted-with-deferred-capability", "deferredCapability": "37.2", "acceptanceScope": "historical listed samples; not verification of the current invocation", "requiredScenes": "29–42", "requiredCapabilities": 28,
			"readOnlyActions":    []string{"document-layout-inspect", "table-inspect", "form-inspect", "form-draft", "form-results", "slide-inspect", "slide-design-inspect"},
			"viewActions":        []string{"document-goto", "form-preview", "form-respondent"},
			"exportActions":      []string{"slide-print-pdf"},
			"editorActions":      []string{"document-format", "document-graphic", "document-table", "document-table-style", "document-mention", "document-paragraph", "document-page", "document-theme", "document-replace", "slide-add", "slide-duplicate", "slide-hide", "slide-delete", "slide-text-insert", "slide-text-format", "slide-placeholder-fill", "slide-layout", "slide-replace-all", "slide-shape-format", "slide-table-edit", "slide-title-animate", "slide-show-settings", "slide-theme", "slide-media-insert", "table-create", "table-duplicate", "table-delete", "table-update", "table-field-add", "table-field-update", "table-field-convert", "table-field-delete", "table-record-add", "table-record-update", "table-reference-add", "table-reference-set", "table-view-create", "table-view-update", "table-view-delete", "table-chart-create", "table-chart-style", "form-edit", "form-settings", "form-publish", "form-submit"},
			"editorActionStatus": "bounded implementation; per-capability live acceptance recorded separately",
			"documentGraphicTargets": map[string]any{
				"mutuallyExclusive": true,
				"metadata":          []string{"name", "description"},
				"position":          []string{"paragraph", "expectedParagraph", "drawing"},
				"positionIndexBase": 1,
				"positionScope":     "paragraph-only body; complete paragraph text and every drawing count/type/geometry must match export and native editor before selection",
			},
			"documentGraphicPreset": map[string]any{
				"parameter": "preset", "values": []int{1, 2, 3, 4, 5, 6},
				"scope": "native shapes only; selects a visible toolbar option after checking its actual value and preview; cannot combine with direct formatting",
			},
			"spreadsheetRegressionActions": []string{"sheet-update", "sheet-append", "sheet-insert", "sheet-delete", "sheet-format", "sheet-merge"},
			"spreadsheetScope":             "requested spreadsheet action only; not phase3 application-table acceptance",
			"localArtifactScope":           "package structure only; actionVerified=false", "automaticMutationReplay": false,
			"acceptance": "docs/phase3-acceptance.md",
		},
		"subscriptions": map[string]any{"status": "supported", "endpoint": "GET /api/v1/files/{fileGuid}/update_subscription", "mode": "scan accessible files; no server-side all-subscriptions endpoint"},
		"errors":        map[string]any{"format": "非 0 退出码 + stderr 文本；--json 时 stderr 为 {\"ok\":false,\"error\":\"...\"}"},
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
	return `shimo-net ` + version + ` —— shimo.net 接口（` + strconv.Itoa(len(capabilityScenarioNames)) + ` 个场景，` + strconv.Itoa(capabilityCount) + ` 项能力；含 API、页面操作和人工禁用目录）

用法:
  shimo-net init [--origin URL]                    初始化产品网址并校验可达
  shimo-net config [show|set URL|clear]            查看 / 修改产品网址
  shimo-net login [--origin URL] [--new]            登录；--new 全新临时配置，凭据按 origin 隔离
  shimo-net logout                                 退出并删除本机凭据
  shimo-net whoami | doctor                        账号 / 自检
  shimo-net session status|harvest|import|watch|logout  登录态来源 / 复用浏览器 / 宿主写入 / 订阅变化 / 标记退出
  shimo-net search <关键词> [--limit 20]            1 搜索
  shimo-net spaces [--limit 300]                   2 我的空间清单
  shimo-net ls <链接|@别名> [--since 7d]             2/12 列目录
  shimo-net info <链接|@别名>                       3 文件类型/权限/路径
  shimo-net upload <本地文件> [--to <位置>]          14 上传（不给 --to 会问：桌面/空间/文件夹）
  shimo-net import <本地文件> --to <位置>            4 导入并转在线文档；权限需页面人工操作
  shimo-net pull <文件夹|空间> [--out <目录>]          5 拉取当前层文件（子目录记为跳过；--limit N 可限制）
  shimo-net new <名称> [--type doc|docx|sheet|slide|table|form|mindmap|flowchart]  6 新建（doc=文档，docx=文稿）
  shimo-net mkdir <名称> [--to <父文件夹>]             6 新建文件夹
  shimo-net rename <链接> <新名字>                   7 改名
  shimo-net delete / trash / restore                 8 删除（只进回收站）/ 回收站 / 恢复
  shimo-net audit <文件夹|@别名> [--limit 200]        9 权限巡检（默认 200，可能截断）
  shimo-net share …                                  权限变更已禁用，必须页面人工操作
  shimo-net read <链接> [--max-chars N]              11 读正文
  shimo-net get <链接> [--out <目录>]                13 下载单个文件
  shimo-net fav <list|add|rm|show> …                15 常用文件夹别名（下次直接 @别名）
  shimo-net update [--check]                       更新到最新稳定版；--check 仅检查
  shimo-net version                                输出版本
  shimo-net open <链接|guid> [--default]            用独立配置打开，不注入会话；--default 选择默认浏览器应用
  shimo-net purge <链接>                            彻底删除（按设计拒绝，请到网页回收站操作）
  shimo-net editor <链接> --action sheet-format --payload '{"range":"工作表1!A1:C1","attrs":{"background":"#D32F2F","color":"#FFFFFF","bold":true}}'
  shimo-net phase3 <链接> --action sheet-update --payload '{"range":"工作表1!A1:B2","resource":{"values":[["产品","价格"],["A",10]]}}' --read-range 工作表1!A1:B2 --out ./evidence --json
  shimo-net phase3 artifact ./export.docx --json
  shimo-net phase3 <应用表格链接> --action table-inspect [--table 数据表guid] --json
  shimo-net phase3 <表单链接> --action form-inspect|form-results [--out 目录] --json
  扩展动作的逐项状态见 docs/phase3-acceptance.md；只读检查和本地包检查不代表整体验收通过
  shimo-net editor <链接> --action sheet-merge --payload '{"sheetName":"工作表1","cell":"A1","rowspan":1,"colspan":3}'
  shimo-net editor <链接> --action sheet-values --payload '{"range":"工作表1!A1:C5"}'
  shimo-net editor <链接> --action sheet-update --payload '{"range":"工作表1!A1","resource":{"values":[["值"]]}}'
  shimo-net editor <链接> --action sheet-append --payload '{"range":"工作表1!A1","resource":{"values":[["值"]]}}'
  shimo-net editor <链接> --action mentions-list|comments-list  HTTP 只读，不启动浏览器；输出定位链接，--json 保留原始数据
  shimo-net subscriptions [--limit 200] [--depth 3]   limit 为扫描文件数上限，仅排序扫描结果；--open 序号|guid 后打开

场景目录（执行方式与限制以能力源为准）:
` + capabilityScenarioHelp + `

  shimo-net schema                                  命令/参数/JSON 结构

兼容命令: phase2 是 editor 的兼容别名。

人工页面操作（CLI 禁止）: 文件锁定 / 限制编辑 / 高级权限 / 成员级授权 / 彻底删除

权限（仅用于读取当前值）: private | enterprise_readonly | enterprise_commentable | enterprise_editable
      readonly | commentable | editable

共享会话: 同一系统用户的 CLI 共用 sessions/<origin>.json（0600，含 revision）；session import/watch/logout 供宿主接入

全局: --json；SHIMO_NET_ORIGIN 指定私有部署域名；--relogin 仅允许只读命令在显式新登录后重试；所有写命令及 phase3 禁止自动重放
`
}
