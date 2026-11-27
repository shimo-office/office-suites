package main

import (
	"errors"
	"fmt"
	"strings"
)

// Browser grid operations use the deployed editor façade; callers never supply
// arbitrary scripts or editor method names. Every field here is validated
// before a page is opened.
type sheetBrowserSpec struct {
	Action      string `json:"action"`
	Sheet       string `json:"sheetName"`
	Range       string `json:"range,omitempty"`
	Row         int    `json:"row,omitempty"`
	Column      int    `json:"column,omitempty"`
	RowCount    int    `json:"rowCount,omitempty"`
	ColumnCount int    `json:"columnCount,omitempty"`

	FrozenRows    *int `json:"rows,omitempty"`
	FrozenColumns *int `json:"columns,omitempty"`
}

func sheetBrowserAction(action string) bool {
	switch action {
	case "sheet-freeze", "sheet-filter", "sheet-filter-clear":
		return true
	}
	return false
}

func prepareSheetBrowser(action string, payload map[string]any) (sheetBrowserSpec, error) {
	spec := sheetBrowserSpec{Action: action, Sheet: strings.TrimSpace(str(payload, "sheetName"))}
	if !sheetBrowserAction(action) {
		return spec, fmt.Errorf("未知表格浏览器动作 %q；可用：sheet-freeze / sheet-filter / sheet-filter-clear", action)
	}
	if spec.Sheet == "" {
		return spec, errors.New("sheetName 必填，必须使用真实工作表名")
	}
	allowed := map[string]bool{"sheetName": true}
	switch action {
	case "sheet-freeze":
		allowed["rows"], allowed["columns"] = true, true
		for _, item := range []struct {
			key   string
			value **int
			max   int
		}{{"rows", &spec.FrozenRows, 1048575}, {"columns", &spec.FrozenColumns, 16383}} {
			if v, exists := payload[item.key]; exists {
				n, err := phase3Integer(v, 0, item.max)
				if err != nil {
					return spec, fmt.Errorf("%s 必须是 0 到 %d 的整数冻结数量", item.key, item.max)
				}
				*item.value = &n
			}
		}
		if spec.FrozenRows == nil && spec.FrozenColumns == nil {
			return spec, errors.New("rows/columns 至少指定一项；0 表示取消对应冻结")
		}
	case "sheet-filter":
		allowed["range"] = true
		rng := strings.TrimSpace(str(payload, "range"))
		if rng == "" {
			return spec, errors.New("range 必填，例如 工作表1!A1:C10")
		}
		if !strings.Contains(rng, "!") {
			rng = spec.Sheet + "!" + rng
		}
		sheet, start, end, ok := parseCellRange(rng)
		if !ok || sheet == "" {
			return spec, errors.New("range 必须形如 工作表1!A1:C10")
		}
		if sheet != spec.Sheet {
			return spec, errors.New("sheetName 与 range 不一致")
		}
		startCol, startRow, _ := cellRef(start)
		endCol, endRow, _ := cellRef(end)
		spec.Range = rng
		spec.Row, spec.Column = startRow-1, colNumber(startCol)-1
		spec.RowCount, spec.ColumnCount = endRow-startRow+1, colNumber(endCol)-colNumber(startCol)+1
		if spec.RowCount < 2 {
			return spec, errors.New("筛选区域至少包含表头行和一行数据")
		}
		if spec.RowCount*spec.ColumnCount > 200000 {
			return spec, errors.New("筛选区域过大（超过 20 万单元格），请缩小范围")
		}
	case "sheet-filter-clear":
		// Only the sheet name is meaningful; the current filter range is read
		// from the page instead of being supplied by the caller.
	}
	for key := range payload {
		if !allowed[key] {
			return spec, fmt.Errorf("%s 不支持参数 %s", action, key)
		}
	}
	return spec, nil
}
