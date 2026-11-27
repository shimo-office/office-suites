package main

import (
	"fmt"
	"sort"
)

// actionContract 是 phase3 动作的机器可读参数契约。
// 目的：agent 与用户不必再靠试错猜 payload；未列出的参数会被各命令拒绝。
// 这里只描述"怎么调用"，是否已在线上通过验收见 docs/phase3-acceptance.md。
type actionContract struct {
	Summary   string            `json:"summary"`
	Transport string            `json:"transport"`
	ReadOnly  bool              `json:"readOnly"`
	Required  []string          `json:"required,omitempty"`
	Optional  []string          `json:"optional,omitempty"`
	Enums     map[string]string `json:"enums,omitempty"`
	Example   string            `json:"example,omitempty"`
	Notes     string            `json:"notes,omitempty"`
}

func enum(values ...string) map[string]string {
	out := map[string]string{}
	for _, v := range values {
		out[v] = v
	}
	return out
}

var phase3ActionContracts = map[string]actionContract{
	// ---------- 文稿 ----------
	"document-format": {
		Summary:   "设置指定段落内精确字符范围的字体、字号、颜色、上下标与行内标记",
		Transport: "browser",
		Required:  []string{"paragraph", "text", "attrs"},
		Optional:  []string{"match", "offset", "length"},
		Enums: map[string]string{
			"attrs.verticalAlign": "baseline|superscript|subscript",
			"attrs.fontSize":      "1–400 磅，0.5 步长",
			"attrs.color":         "#RRGGBB",
		},
		Example: `{"paragraph":1,"text":"测试 x2 H2O","match":"x2","offset":1,"length":1,"attrs":{"verticalAlign":"superscript"}}`,
		Notes:   "paragraph 从 1 起；text 必须是该段完整正文且不含换行；match 必须全文唯一；offset/length 成对、按 Unicode 码点。已符合请求会返回无修改错误，不冒充本次编辑。上下标只在文稿有此入口。",
	},
	"document-paragraph": {
		Summary:   "设置文稿段落属性：标题级别、首行缩进字符、行距倍数、底纹、编号",
		Transport: "browser",
		Required:  []string{"paragraph", "text", "attrs"},
		Enums: map[string]string{
			"attrs.heading":        "1|2|3|4|5|6",
			"attrs.firstLineChars": "0–20，精度 0.01 字符",
			"attrs.lineMultiple":   "0.5–10（倍数）",
			"attrs.shading":        "#RRGGBB",
			"attrs.numbering":      "decimal（仅编号）",
		},
		Example: `{"paragraph":2,"text":"精确原段落","attrs":{"firstLineChars":2,"lineMultiple":1.5,"shading":"#EEEEEE"}}`,
		Notes:   "attrs 至少一项；只接受上面五类键，align/list/indent/lineHeight 在文稿上会被拒绝（对齐与列表尚未实现，需要时用轻文档 doc-paragraph）。",
	},
	"document-page": {
		Summary:   "页面设置：页边距、分栏、页面背景色",
		Transport: "browser",
		Optional:  []string{"marginsCm", "columns", "background"},
		Example:   `{"marginsCm":{"top":2,"bottom":2,"left":1.5,"right":1.5},"columns":2,"background":"#EAF3FF"}`,
		Notes:     "当前仅支持单节文稿；marginsCm 必须同时给出四边，单位厘米；columns 1–4；各组设置可独立使用。",
	},
	"document-theme": {
		Summary:   "替换主题字体（major/minor 两语言组）",
		Transport: "browser",
		Required:  []string{"themeFonts"},
		Optional:  []string{"resetSupplemental"},
		Example:   `{"themeFonts":{"major":{"latin":"Arial","ea":"宋体","cs":"Arial"},"minor":{"latin":"Arial","ea":"宋体","cs":"Arial"}},"resetSupplemental":true}`,
		Notes:     "必须完整提供两组字体；resetSupplemental 明确允许重建补充语言映射。",
	},
	"document-replace": {
		Summary:   "按精确字面文本替换，并核对预期命中次数",
		Transport: "browser",
		Required:  []string{"find", "replacement", "expectedCount"},
		Example:   `{"find":"旧版测试","replacement":"新版测试","expectedCount":3}`,
		Notes:     "expectedCount 是替换前应命中的精确次数，用于防止误改；不做正则。",
	},
	"document-table": {
		Summary:   "文内表格行列插删、均分、横向合并与拆分",
		Transport: "browser",
		Required:  []string{"operation", "table", "row", "column", "expectedCell"},
		Optional:  []string{"endColumn", "columns"},
		Enums: map[string]string{
			"operation": "insert-row-before|insert-row-after|delete-row|insert-column-before|insert-column-after|delete-column|distribute-rows|distribute-columns|merge|split",
			"table":     "从 1 起",
			"row":       "从 1 起",
			"column":    "从 1 起",
		},
		Example: `{"operation":"merge","table":1,"row":1,"column":1,"endColumn":2,"expectedCell":"P3_A1"}`,
		Notes:   "只接受非嵌套、无合并的矩形表格；merge 用 endColumn（同一行连续 2–20 格）；split 用 columns 且必须等于当前横向 gridSpan；expectedCell 是该格完整文本，多段用 \\n 连接。",
	},
	"document-table-style": {
		Summary:   "文内表格单格填充/文字色，整表内边距、对齐与外边框",
		Transport: "browser",
		Required:  []string{"table", "row", "column"},
		Optional:  []string{"expectedCell", "fill", "color", "marginCm", "align", "border"},
		Enums: map[string]string{
			"align":       "left|center|right",
			"marginCm":    ">0 且 ≤2，四边相同",
			"border.pt":   "0.25–12，1/8 步长",
			"fill, color": "#RRGGBB，只改指定单格",
		},
		Example: `{"table":1,"row":1,"column":1,"expectedCell":"P3_A1","fill":"#0000FF","color":"#FFFFFF","marginCm":0.2,"align":"center","border":{"color":"#000000","pt":1}}`,
		Notes:   "marginCm、align、border 作用于整张表；align 必须同时给出 marginCm；请求文字色时 expectedCell 不得为空；无属性或已符合请求会被拒绝。",
	},
	"document-graphic": {
		Summary:   "图形定位/尺寸/旋转/环绕/填充/线条，或套用原生预设样式",
		Transport: "browser",
		Optional:  []string{"name", "description", "paragraph", "expectedParagraph", "drawing", "widthCm", "heightCm", "rotationDegrees", "wrap", "anchor", "horizontalAlign", "horizontalRelativeTo", "fill", "line", "preset"},
		Enums: map[string]string{
			"wrap":             "square|topAndBottom|behind|inFront",
			"anchor":           "paragraph|page",
			"horizontalAlign":  "center",
			"preset":           "1–6",
			"widthCm/heightCm": "0.01–50，必须成对",
		},
		Example: `{"name":"PHASE3_ARROW","description":"PHASE3_ARROW_20261118","widthCm":6,"heightCm":4,"rotationDegrees":90,"wrap":"square","anchor":"paragraph","horizontalAlign":"center","horizontalRelativeTo":"margin"}`,
		Notes:   "目标二选一：{name,description} 精确同名同描述，或 {paragraph,expectedParagraph,drawing}（均从 1 起）；preset 与直接格式互斥；线宽单位磅；图片不能冒充原生形状。",
	},
	"document-mention": {
		Summary:   "在指定段落末尾追加当前账号本人的真实提及",
		Transport: "browser",
		Required:  []string{"paragraph", "text"},
		Example:   `{"paragraph":1,"text":"PHASE3_SELF_MENTION_20261118"}`,
		Notes:     "仅本人；不接受 userId 或姓名参数；不支持提及他人；该段已有提及则拒绝。",
	},
	"document-layout-inspect": {
		Summary:   "只读导出段落、节与主题结构，取得真实段落号与原文",
		Transport: "browser",
		ReadOnly:  true,
	},
	"document-goto": {
		Summary:   "跳转到指定物理页并核验，不持久化光标",
		Transport: "browser",
		Required:  []string{"page"},
		Example:   `{"page":2}`,
		Notes:     "page 从 1 起。",
	},

	// ---------- 轻文档 ----------
	"doc-format": {
		Summary:   "轻文档字符格式：粗斜下划删除线、字号、字体、颜色、高亮（可全文）",
		Transport: "browser",
		Optional:  []string{"paragraph", "text", "start", "end", "scope", "attrs"},
		Enums: map[string]string{
			"scope":           "all|（省略则需 paragraph+text）",
			"attrs.fontSize":  "6–200",
			"attrs.color":     "#RRGGBB",
			"attrs.highlight": "#RRGGBB，不接受 NONE",
		},
		Example: `{"paragraph":3,"text":"整段原文","start":4,"end":10,"attrs":{"bold":true,"fontSize":18}}`,
		Notes:   "start/end 必须成对、按 Unicode 码点；scope=all 表示全文（不带段落参数）；上下标在轻文档被拒绝，请用 document-format。",
	},
	"doc-paragraph": {
		Summary:   "轻文档段落格式：标题、对齐、列表、缩进、行距",
		Transport: "browser",
		Required:  []string{"paragraph", "text", "attrs"},
		Enums: map[string]string{
			"attrs.heading":    "0|1|2|3|4|5|6|title|subtitle",
			"attrs.align":      "left|center|right|justify",
			"attrs.list":       "ordered|unordered|todo|none",
			"attrs.indent":     "±1–8",
			"attrs.lineHeight": "1.0|1.15|1.5|2.0|2.5|3.0",
		},
		Example: `{"paragraph":3,"text":"整段原文","attrs":{"align":"center","indent":2,"lineHeight":1.5}}`,
	},
	"doc-replace": {
		Summary:   "轻文档按字面替换文本，可限定首次或全文",
		Transport: "browser",
		Required:  []string{"find", "replace"},
		Optional:  []string{"scope"},
		Enums:     map[string]string{"scope": "all|first"},
		Example:   `{"find":"旧文本","replace":"新文本","scope":"all"}`,
		Notes:     "参数名是 replace（不是 replacement）；replace 必须显式给出，允许空串。",
	},
	"doc-page-width": {
		Summary:   "设置轻文档版心宽度",
		Transport: "browser",
		Required:  []string{"width"},
		Enums:     map[string]string{"width": "standard|wide|full"},
		Example:   `{"width":"standard"}`,
		Notes:     "以纸张容器 page-width-* 类验收。",
	},
	"doc-table": {
		Summary:   "在指定段落前后通过粘贴通道插入原生表格",
		Transport: "browser",
		Required:  []string{"paragraph", "text"},
		Optional:  []string{"position", "rows", "cols", "values"},
		Enums:     map[string]string{"position": "after|before"},
		Example:   `{"paragraph":1,"text":"整段原文","position":"after","values":[["A","B"],["1","2"]]}`,
		Notes:     "rows/cols 或非空 values 二选一；单次上限 50 行 20 列；必须有新表格节点与导出物核对。",
	},
	"doc-image": {
		Summary:   "在指定段落前后插入本地图片",
		Transport: "browser",
		Required:  []string{"paragraph", "text", "file"},
		Optional:  []string{"position"},
		Enums:     map[string]string{"position": "after|before", "file": "png|jpg|jpeg|gif|webp，≤4MB"},
		Example:   `{"paragraph":1,"text":"整段原文","position":"after","file":"/tmp/pixel.png"}`,
		Notes:     "file 必须是本地绝对路径且能解码。",
	},

	// ---------- 幻灯片 ----------
	"slide-inspect": {
		Summary:   "只读读取页面 ID 列表",
		Transport: "browser",
		ReadOnly:  true,
	},
	"slide-design-inspect": {
		Summary:   "只读读取页面/母版/版式、可用版式与官方主题、形状与表格白名单",
		Transport: "browser",
		ReadOnly:  true,
		Notes:     "写动作的 slideId、layoutId、themeId、shapeName、tableName 必须来自这里，不要猜。",
	},
	"slide-add": {
		Summary:   "在指定页后新增一页",
		Transport: "browser",
		Required:  []string{"slideId"},
		Example:   `{"slideId":"actual-page-id"}`,
	},
	"slide-duplicate": {
		Summary:   "复制指定页，插在其后",
		Transport: "browser",
		Required:  []string{"slideId"},
	},
	"slide-hide": {
		Summary:   "隐藏指定页",
		Transport: "browser",
		Required:  []string{"slideId"},
		Notes:     "已隐藏的页会拒绝。",
	},
	"slide-delete": {
		Summary:   "删除指定页",
		Transport: "browser",
		Required:  []string{"slideId"},
		Notes:     "拒绝删除最后一页；只操作传入的那一页。",
	},
	"slide-text-insert": {
		Summary:   "在页面像素坐标处插入单行文本框",
		Transport: "browser",
		Required:  []string{"slideId", "text", "x", "y", "width", "height"},
		Optional:  []string{"fontSize", "bold"},
		Example:   `{"slideId":"actual-page-id","text":"补充说明","x":120,"y":300,"width":650,"height":80}`,
		Notes:     "x/y/width/height 为像素；fontSize 默认 24 磅，bold 默认 false；只支持单行，越界拒绝；不追加到已有形状。",
	},
	"slide-text-format": {
		Summary:   "整形状文本格式：字体、字号、文字填充色、粗斜下划线、文字轮廓",
		Transport: "browser",
		Required:  []string{"slideId", "text"},
		Optional:  []string{"fontFamily", "fontSize", "color", "bold", "italic", "underline", "outlineColor", "outlineWidth"},
		Example:   `{"slideId":"actual-page-id","text":"exact unique full shape text","fontFamily":"Arial","fontSize":28,"color":"#0000FF"}`,
		Notes:     "text 必须全文唯一；至少给一项格式；color 是文字填充而非形状填充；outlineWidth 单位磅，默认 1；形状边框不等于文字轮廓。",
	},
	"slide-placeholder-fill": {
		Summary:   "向空白占位符（标题/正文）写入文本",
		Transport: "browser",
		Required:  []string{"slideId", "shapeName", "text"},
		Example:   `{"slideId":"actual-page-id","shapeName":"actual unique placeholder name","text":"三期验收"}`,
		Notes:     "接受空的 title/ctrTitle/body/subTitle 及带文本容器的 obj（含省略 type 的内容占位符）；多段用 \\n；已有文本会被拒绝。",
	},
	"slide-theme": {
		Summary:   "整册套用官方主题",
		Transport: "browser",
		Required:  []string{"themeId"},
		Notes:     "themeId 必须来自 slide-design-inspect 的可用官方主题；仅支持原生 16:9；目标主题母版已存在则拒绝。",
	},
	"slide-layout": {
		Summary:   "在当前母版内切换页面版式",
		Transport: "browser",
		Required:  []string{"slideId", "layoutId"},
		Notes:     "layoutId 必须属于该页当前母版；不跨母版换主题。",
	},
	"slide-shape-format": {
		Summary:   "形状填充/线条/旋转/水平居中/层级后移",
		Transport: "browser",
		Required:  []string{"slideId", "shapeName"},
		Optional:  []string{"fillColor", "lineColor", "rotation", "horizontalCenter", "behindName"},
		Example:   `{"slideId":"actual-page-id","shapeName":"actual unique direct shape name","fillColor":"#0000FF","rotation":45,"horizontalCenter":true}`,
		Notes:     "至少一项属性；填充、边框颜色、层级可独立修改，无需显式几何；旋转/水平居中仍需显式几何；lineColor 需要已有显式线宽且保持不变（无改线宽接口）；rotation 为绝对角度 [0,360)；分组子形状与图片不可作为目标。",
	},
	"slide-table-edit": {
		Summary:   "幻灯片表格插行、删列、合并、拆分、表头填充与边框",
		Transport: "browser",
		Required:  []string{"slideId", "tableName", "operation"},
		Optional:  []string{"index", "row", "column", "headerFillColor", "borderColor", "borderWidth"},
		Enums: map[string]string{
			"operation":   "insert-row|delete-column|merge|split|style",
			"index":       "insert-row/delete-column：从 1 起，插入在该行之前",
			"row,column":  "merge/split：从 1 起左上角",
			"borderWidth": "0.25–20 磅，默认 1",
		},
		Example: `{"slideId":"actual-page-id","tableName":"actual unique table name","operation":"insert-row","index":2}`,
		Notes:   "每次只做一个操作，各自需要中间导出证据；拒绝合并/不规则表格；style 至少要给一种颜色。",
	},
	"slide-replace-all": {
		Summary:   "整册（含隐藏页）字面查找替换",
		Transport: "browser",
		Required:  []string{"old", "new"},
		Example:   `{"old":"旧版测试","new":"新版测试"}`,
		Notes:     "区分大小写、不支持正则与换行；old/new 必须不同，可互相包含；只做一次字面替换，按预期完整正文及残留旧词计数核验；跨格式 run 命中会在写前拒绝。",
	},
	"slide-title-animate": {
		Summary:   "批量给多个页面的标题加原生 Appear 入场动画",
		Transport: "browser",
		Required:  []string{"targets"},
		Optional:  []string{"fontSize"},
		Example:   `{"targets":[{"slideId":"page1-id","text":"exact full title1"}]}`,
		Notes:     "targets 1–100 个唯一页面，每页一个唯一标题形状；省略 fontSize 只加动画并保留文字格式；单击触发、时长 1ms；页面已有动画则拒绝；不暴露自定义动画参数。",
	},
	"slide-show-settings": {
		Summary:   "放映设置：循环播放、使用排练计时",
		Transport: "browser",
		Optional:  []string{"loopShow", "useTiming"},
		Example:   `{"loopShow":true,"useTiming":false}`,
		Notes:     "两个布尔至少给一个；没有页码范围选项；隐藏页面不算等价替代。",
	},
	"slide-media-insert": {
		Summary:   "向页面插入音频或视频并核对持久化字节",
		Transport: "browser",
		Required:  []string{"slideId", "kind", "file"},
		Optional:  []string{"triggerType"},
		Enums:     map[string]string{"kind": "audio|video", "triggerType": "click|auto（仅音频）", "file": "本地绝对路径，≤8MiB"},
		Example:   `{"slideId":"actual-page-id","kind":"audio","file":"/absolute/path/test.wav","triggerType":"click"}`,
		Notes:     "文件必须能在浏览器内解码；视频用原生播放控件且不接受纯音频触发；页面已有动画则拒绝。",
	},
	"slide-print-pdf": {
		Summary:   "导出可见页面为 PDF（无需 payload）",
		Transport: "api",
		ReadOnly:  true,
		Notes:     "不要传 {}，该动作不接受 payload。返回 exported:true、verified:false，另需独立 PDF 解析验收。",
	},

	// ---------- 应用表格 ----------
	"table-inspect": {
		Summary:   "只读盘点数据表、字段、记录与视图 GUID",
		Transport: "api",
		ReadOnly:  true,
		Optional:  []string{"table", "records-limit"},
		Notes:     "不给 --table 时返回每个数据表的 fields/views/recordGuids 清单；给 --table 时追加该表完整快照。所有后续字段键都必须用 GUID。",
	},
	"table-create": {
		Summary:   "新建数据表（含主文本字段、空记录与表格视图）",
		Transport: "api",
		Required:  []string{"name"},
		Optional:  []string{"desc"},
		Example:   `{"name":"测试表","desc":"合成测试数据"}`,
	},
	"table-update": {
		Summary:   "修改数据表名称或说明",
		Transport: "api",
		Required:  []string{"tableGuid"},
		Optional:  []string{"name", "desc"},
		Example:   `{"tableGuid":"实际GUID","name":"新名称","desc":"新说明"}`,
	},
	"table-duplicate": {
		Summary:   "复制数据表（保留记录值）",
		Transport: "api",
		Required:  []string{"tableGuid", "name"},
		Notes:     "只接受可验证的简单字段；同步、表单关联及不支持的复杂结构会被拒绝。",
	},
	"table-delete": {
		Summary:   "删除数据表",
		Transport: "api",
		Required:  []string{"tableGuid"},
		Notes:     "拒绝最后一个表、被引用目标及同步/表单关联表；其他表必须保持不变。",
	},
	"table-field-add": {
		Summary:   "新增字段",
		Transport: "api",
		Required:  []string{"tableGuid", "fields"},
		Enums:     map[string]string{"fields[].type": "SingleLineText|Number|SingleChoice|Date"},
		Example:   `{"tableGuid":"实际GUID","fields":[{"name":"优先级","type":"SingleChoice","options":["高","低"]}]}`,
		Notes:     "日期字段接受带时区 RFC3339；SingleChoice 需 options。",
	},
	"table-field-update": {
		Summary:   "修改字段名称",
		Transport: "api",
		Required:  []string{"tableGuid", "fieldGuid", "name"},
		Notes:     "当前只支持改名。",
	},
	"table-field-convert": {
		Summary:   "文本与数值之间无损回转字段类型",
		Transport: "api",
		Required:  []string{"tableGuid", "fieldGuid", "type"},
		Enums:     map[string]string{"type": "Number|SingleLineText"},
		Notes:     "会丢失格式的值（如 001）及有依赖的字段会被拒绝。",
	},
	"table-field-delete": {
		Summary:   "删除字段",
		Transport: "api",
		Required:  []string{"tableGuid", "fieldGuids"},
		Notes:     "拒绝主字段及已识别依赖。",
	},
	"table-record-add": {
		Summary:   "批量新增记录",
		Transport: "api",
		Required:  []string{"tableGuid", "records"},
		Example:   `{"tableGuid":"实际GUID","records":[{"实际字段GUID":"合成测试值"}]}`,
		Notes:     "records 每项的键必须是字段 GUID（不是字段名）；单选值用选项 GUID。",
	},
	"table-record-update": {
		Summary:   "按非空等值条件批量改值",
		Transport: "api",
		Required:  []string{"tableGuid", "where", "values"},
		Example:   `{"tableGuid":"实际GUID","where":{"条件字段GUID":"旧值"},"values":{"目标字段GUID":"新值"}}`,
		Notes:     "where 是 AND 等值条件，拒绝隐式全表修改与零命中。",
	},
	"table-reference-add": {
		Summary:   "建立双向关联字段",
		Transport: "api",
		Required:  []string{"tableGuid", "referenceTableGuid", "name"},
		Notes:     "同一应用表格内；写前会探测原生字段类型菜单，未提供 ReferenceRecord 则拒绝写入。",
	},
	"table-reference-set": {
		Summary:   "设置某条记录的关联目标",
		Transport: "api",
		Required:  []string{"tableGuid", "fieldGuid", "recordGuid", "referenceRecordGuids"},
		Notes:     "空数组表示清空；拒绝已有不对称、孤立、跨库或视图限定引用。",
	},
	"table-view-create": {
		Summary:   "新建视图（表格/看板/日历）",
		Transport: "api",
		Required:  []string{"tableGuid", "viewGuid", "name", "type"},
		Optional:  []string{"groupFieldGuid", "startFieldGuid", "endFieldGuid"},
		Enums:     map[string]string{"type": "Grid|Kanban|Calendar"},
		Example:   `{"tableGuid":"实际GUID","viewGuid":"已有视图GUID","name":"测试看板","type":"Kanban","groupFieldGuid":"单选字段GUID"}`,
		Notes:     "viewGuid 是插入锚点（新视图插在其后）；Kanban 需要单选类型 groupFieldGuid；Calendar 需要日期类型 startFieldGuid/endFieldGuid。",
	},
	"table-view-update": {
		Summary:   "修改视图名称、筛选、分组与排序",
		Transport: "api",
		Required:  []string{"tableGuid", "viewGuid"},
		Optional:  []string{"name", "filters", "groups", "sort"},
		Enums: map[string]string{
			"filters.conjunction": "And|Or",
			"filters[].operator":  "Equal|NotEqual|GreaterThan|GreaterThanEqual|LessThan|LessThanEqual",
			"groups[].order":      "Asc|Desc（至多 3 项）",
			"sort.rules[].order":  "Asc|Desc（至多 3 项）",
		},
		Example: `{"tableGuid":"实际GUID","viewGuid":"实际视图GUID","filters":{"conjunction":"And","conditions":[{"fieldGuid":"数值字段GUID","operator":"GreaterThanEqual","value":10}]}}`,
		Notes:   "filters:null 清空筛选；大小比较只接受数值字段；排序当前只支持数值字段，空值排末尾。",
	},
	"table-view-delete": {
		Summary:   "删除视图",
		Transport: "api",
		Required:  []string{"tableGuid", "viewGuid"},
		Notes:     "拒绝默认、最后共享、个人/锁定/已共享视图及跨表、仪表盘依赖。",
	},
	"table-chart-create": {
		Summary:   "用分组字段创建计数图表",
		Transport: "api",
		Required:  []string{"tableGuid", "fieldGuid", "name", "type"},
		Enums:     map[string]string{"type": "Column|Line|Bar|Pie"},
		Example:   `{"tableGuid":"实际GUID","fieldGuid":"分组字段GUID","name":"测试计数","type":"Column"}`,
		Notes:     "拒绝空数据；核对图表身份、来源与真实渲染聚合值。",
	},
	"table-chart-style": {
		Summary:   "修改仪表盘组件的原生配色方案",
		Transport: "api",
		Required:  []string{"dashboardGuid", "componentGuid", "themeColor"},
		Enums:     map[string]string{"themeColor": "0–10"},
		Notes:     "只改一个组件；不是任意 RGB 接口；无变化会被拒绝。",
	},

	// ---------- 表单 ----------
	"form-draft": {
		Summary:   "只读解码服务端已保存草稿",
		Transport: "api",
		ReadOnly:  true,
		Notes:     "题目 GUID 与索引从这里取，不要猜。",
	},
	"form-inspect": {
		Summary:   "只读检查表单结构、状态与发布信息",
		Transport: "api",
		ReadOnly:  true,
	},
	"form-edit": {
		Summary:   "按 typed operation 修改题目、顺序、条件规则、标题/描述/结果页",
		Transport: "browser",
		Required:  []string{"operation 或 operations"},
		Optional:  []string{"question", "guid", "patch", "from", "to", "trigger", "options", "targets"},
		Enums: map[string]string{
			"operation":     "question-add|question-update|question-reorder|question-delete|meta|rule-add|rule-update|rule-delete",
			"question.type": "text|multiple_text|single_choice|multiple_choice|dropdown_list|rate",
			"operations":    "1–20 步批量；第一步写入前验完整计划，无回滚",
		},
		Example: `{"operation":"question-add","question":{"guid":"text01","type":"text","required":true,"subject":"测试答卷标记","question":{}}}`,
		Notes:   "GUID 与选项 value 为 4–64 位字母/数字/下划线/短横线且唯一；rate 需 {\"shape\":\"star\",\"sum\":1-10}；改 question 时要给该题型完整 question 对象；只证明草稿，不代表已发布。",
	},
	"form-settings": {
		Summary:   "设置未来截止时间与 IANA 时区",
		Transport: "api",
		Required:  []string{"deadline"},
		Optional:  []string{"timezone"},
		Example:   `{"deadline":"2026-11-20T18:00:00+08:00","timezone":"Asia/Shanghai"}`,
		Notes:     "只能设未来时间；时区与日期偏移必须相符；不改填写权限。",
	},
	"form-publish": {
		Summary:   "发布或重新发布已验证草稿",
		Transport: "api",
		Notes:     "无需 payload；配置了通知/提醒或测验模式时拒绝。",
	},
	"form-preview": {
		Summary:   "在未提交预览页验证单选分支可见性与题序",
		Transport: "browser",
		Required:  []string{"trigger", "branches"},
		Optional:  []string{"branches[].sequence"},
		Example:   `{"trigger":"pick01","branches":[{"option":"yes01","visible":["more01"],"hidden":[],"sequence":["text01","pick01","more01","rate01"]}]}`,
		Notes:     "trigger 必须是单选题；visible/hidden 均必填且至少一个断言；不提交答卷。",
	},
	"form-respondent": {
		Summary:   "在已发布填写页验证分支与题序",
		Transport: "browser",
		Required:  []string{"trigger", "branches"},
		Notes:     "与 form-preview 同形状；要求已开放收集。",
	},
	"form-submit": {
		Summary:   "提交一份合成答卷并做独立结果核验",
		Transport: "api+browser",
		Required:  []string{"answers"},
		Optional:  []string{"--confirm-text"},
		Example:   `{"answers":{"text01":"合成测试A","pick01":"yes01","more01":"补充说明","rate01":5}}`,
		Notes:     "答案键是题目 GUID，单选值是选项 value；须有管理权限做结果核验；只提交一次，禁止重试。",
	},
	"form-results": {
		Summary:   "导出答卷统计（CSV/JSON）",
		Transport: "api",
		ReadOnly:  true,
		Notes:     "空草稿没有答卷属正常；导出物是正式结果，未指定 --out 也会保留。",
	},

	// ---------- 电子表格（spreadsheet） ----------
	// 写动作必须带 --read-range，CLI 会导出写前/写后 xlsx 做结构核对后才报 verified；
	// 只读的 sheet-values 必须走 editor 命令，phase3 会以“没有完整写入闭环”拒绝。
	"sheet-values": {
		Summary:   "读取指定区域的单元格值（只读）",
		Transport: "api",
		ReadOnly:  true,
		Required:  []string{"range"},
		Example:   `{"range":"工作表1!A1:C5"}`,
		Notes:     "必须用 shimo-net editor 调用；range 必须带工作表名，空行与合并后的位移不能只靠它判断。工作表名以文件为准（常见为“工作表1”），不要用导出文件名推断。",
	},
	"sheet-update": {
		Summary:   "写入指定区域的值",
		Transport: "api",
		Required:  []string{"range", "resource"},
		Optional:  []string{"sheetName"},
		Example:   `{"range":"工作表1!A1","resource":{"values":[["项目","负责人"],["验收","合成"]]}}`,
		Notes:     "resource.values 为二维数组，按 range 起始格展开；裸 values 会被服务端静默忽略。必须同时给 --read-range 才能完成结构核对。",
	},
	"sheet-append": {
		Summary:   "在表格末尾追加若干行",
		Transport: "api",
		Required:  []string{"range", "resource"},
		Example:   `{"range":"工作表1!A1","resource":{"values":[["追加一","a"],["追加二","b"]]}}`,
		Notes:     "只接受 A1 锚点（工作表1!A1）；其他锚点会被显式拒绝。追加后按读回核对行数与顺序，失败不要盲目重放。",
	},
	"sheet-insert": {
		Summary:   "在指定行位置插入空行",
		Transport: "api",
		Required:  []string{"sheetName", "index"},
		Optional:  []string{"count"},
		Example:   `{"sheetName":"工作表1","index":3,"count":1}`,
		Notes:     "index 透传为接口行索引，不做自然语言行号换算；count 默认 1。插入会使下方行整体位移，核对时按位移后的值判断。",
	},
	"sheet-delete": {
		Summary:   "删除指定行",
		Transport: "api",
		Required:  []string{"sheetName", "index"},
		Optional:  []string{"count"},
		Example:   `{"sheetName":"工作表1","index":3,"count":1}`,
		Notes:     "只删行，不删列、不删工作表；先确认目标行与影响范围。",
	},
	"sheet-format": {
		Summary:   "设置单元格底色、字色、加粗、斜体、下划线",
		Transport: "api",
		Required:  []string{"range", "attrs"},
		Example:   `{"range":"工作表1!A1:C1","attrs":{"background":"#D32F2F","color":"#FFFFFF","bold":true}}`,
		Notes:     "attrs 仅支持 background/color/bold/italic/underline；颜色为 #RRGGBB；多属性一次提交，不宣称原子性。",
	},
	"sheet-merge": {
		Summary:   "合并单元格范围",
		Transport: "api",
		Required:  []string{"sheetName", "cell", "rowspan", "colspan"},
		Example:   `{"sheetName":"工作表1","cell":"A8","rowspan":1,"colspan":3}`,
		Notes:     "cell 为左上角单元格；合并后同一区域的结构操作（插行/追加）会被拒绝，需先拆分或换区域。",
	},
}

func phase3ContractNames() []string {
	names := make([]string, 0, len(phase3ActionContracts))
	for name := range phase3ActionContracts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// schemaAction 支持 `shimo-net schema --action <动作>`：只看一个动作的参数契约。
func schemaAction(name string) (map[string]any, error) {
	contract, ok := phase3ActionContracts[name]
	if !ok {
		return nil, fmt.Errorf("未知 phase3 动作 %q；可用动作见 shimo-net schema 的 phase3.actions", name)
	}
	return map[string]any{"ok": true, "action": name, "contract": contract}, nil
}
