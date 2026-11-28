package main

// Browser-only editor actions share the same discovery entry as phase3, while
// retaining their actual command and transport. This prevents agents from
// guessing payloads or dispatching a browser action through the wrong command.
var editorActionContracts = map[string]actionContract{
	"sheet-freeze": {
		Summary:   "冻结或取消冻结指定工作表的前若干行/列，重开核验",
		Transport: "browser-facade",
		Required:  []string{"sheetName"},
		Optional:  []string{"rows", "columns"},
		Enums:     map[string]string{"rows": "0–1048575 整数", "columns": "0–16383 整数"},
		Example:   `{"sheetName":"工作表1","rows":1,"columns":0}`,
		Notes:     "rows/columns 至少一项；0 表示取消对应冻结。不得超过目标表当前大小。保存后重开读取状态，并比对导出单元格未变。",
	},
	"sheet-filter": {
		Summary:   "为指定工作表建立筛选区域，重开核验范围",
		Transport: "browser-facade",
		Required:  []string{"sheetName", "range"},
		Example:   `{"sheetName":"工作表1","range":"A1:C10"}`,
		Notes:     "range 为 A1 范围，可带一致的工作表名前缀；至少两行，不超过20万格。已有其他范围时拒绝覆盖；此动作只建立筛选区域，不配置值筛选条件。",
	},
	"sheet-filter-clear": {
		Summary:   "清除指定工作表的筛选，重开核验",
		Transport: "browser-facade",
		Required:  []string{"sheetName"},
		Example:   `{"sheetName":"工作表1"}`,
		Notes:     "未设置筛选时返回 changed:false；不接受 range。",
	},
	"sheet-export-pdf": {
		Summary:   "通过更多→下载→纯图PDF菜单下载真实PDF",
		Transport: "browser-ui",
		ReadOnly:  true,
		Optional:  []string{"sheetName"},
		Example:   `{"sheetName":"工作表1"}`,
		Notes:     "省略 sheetName 使用页面活动工作表；不修改页面记忆的纸张/范围/缩放等导出设置，以对话框物证为准。可用 get --format pdf 调用相同流程。核验文件信封与下载完整性，不代表视觉验收。",
	},
}
