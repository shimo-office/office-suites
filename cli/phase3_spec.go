package main

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const phase3MaxCells = 100000

type phase3Spec struct {
	Action    string
	Sheet     string
	ReadRange string
	Range     string
	Payload   map[string]any
	Expected  artifactExpectation
	Index     int
	Count     int
}

func preparePhase3(action string, payload map[string]any, readRange string) (phase3Spec, error) {
	spec := phase3Spec{Action: action, Payload: map[string]any{}}
	if !phase3MutationActions[action] {
		return spec, fmt.Errorf("phase3 动作 %q 没有完整的写入闭环", action)
	}
	sheet, _, _, ok := parseCellRange(readRange)
	if !ok || sheet == "" {
		return spec, errors.New("--read-range 必须是完整范围，例如 工作表1!A1:D4")
	}
	if _, err := phase3RangeCells(readRange); err != nil {
		return spec, err
	}
	spec.Sheet, spec.ReadRange = sheet, readRange
	for key, value := range payload {
		spec.Payload[key] = value
	}
	allowed := map[string]bool{"sheetName": true, "range": true}
	switch action {
	case "sheet-update", "sheet-append":
		allowed["resource"] = true
	case "sheet-format":
		allowed["attrs"] = true
	case "sheet-merge":
		allowed["cell"] = true
		allowed["rowspan"] = true
		allowed["colspan"] = true
	case "sheet-insert", "sheet-delete":
		delete(allowed, "range")
		allowed["index"] = true
		allowed["count"] = true
	}
	for key := range payload {
		if !allowed[key] {
			return spec, fmt.Errorf("%s 不支持参数 %s", action, key)
		}
	}
	if action == "sheet-insert" || action == "sheet-delete" {
		if strings.TrimSpace(str(payload, "sheetName")) != sheet {
			return spec, errors.New("sheetName 与 --read-range 工作表必须相同")
		}
		var err error
		spec.Index, err = phase3Integer(payload["index"], 0, 1048575)
		if err != nil {
			return spec, errors.New("index 必须是 0 到 1048575 之间的整数（从 0 开始）")
		}
		spec.Count = 1
		if value, exists := payload["count"]; exists {
			spec.Count, err = phase3Integer(value, 1, 1048576)
			if err != nil {
				return spec, errors.New("count 必须是正整数")
			}
		}
		if spec.Index+spec.Count > 1048576 {
			return spec, errors.New("行操作超出工作表最大行数")
		}
		spec.Payload["count"] = spec.Count
		spec.Expected = artifactExpectation{SheetName: sheet}
		return spec, nil
	}
	rng := strings.TrimSpace(str(payload, "range"))
	if rng == "" && action == "sheet-merge" {
		ref, err := phase3MergeRef(payload)
		if err != nil {
			return spec, err
		}
		rng = ref
	}
	if !strings.Contains(rng, "!") {
		rng = strings.TrimSpace(str(payload, "sheetName")) + "!" + rng
	}
	rangeSheet, start, end, ok := parseCellRange(rng)
	if !ok || rangeSheet == "" || rangeSheet != sheet {
		return spec, errors.New("写入范围与 --read-range 必须是同一工作表的有效区域")
	}
	if name := strings.TrimSpace(str(payload, "sheetName")); name != "" && name != sheet {
		return spec, errors.New("sheetName 与 range 工作表不一致")
	}
	if _, err := phase3RangeCells(rng); err != nil {
		return spec, err
	}
	spec.Range = rng
	spec.Payload["range"] = rng
	spec.Expected = artifactExpectation{SheetName: sheet, Range: rng}
	switch action {
	case "sheet-update", "sheet-append":
		values, err := phase3PayloadValues(payload)
		if err != nil {
			return spec, err
		}
		col, row, _ := cellRef(start)
		endCol, endRow, _ := cellRef(end)
		if strings.Contains(rng, ":") && (colNumber(endCol)-colNumber(col)+1 != len(values[0]) || endRow-row+1 != len(values)) {
			return spec, errors.New("显式 range 的大小必须与 resource.values 一致")
		}
		if colNumber(col)+len(values[0])-1 > 16384 || row+len(values)-1 > 1048576 {
			return spec, errors.New("写入数据超出工作表边界")
		}
		spec.Range = sheet + "!" + start + ":" + cellName(colName(colNumber(col)+len(values[0])-1), row+len(values)-1)
		spec.Expected.Range = spec.Range
		spec.Expected.Values = values
		spec.Expected.NumericText = true
		if action == "sheet-append" && start != "A1" {
			return spec, errors.New("phase3 追加仅支持以 A1 为锚点的整表追加；其他追加范围尚不能可靠验收")
		}
	case "sheet-format":
		attrs, ok := payload["attrs"].(map[string]any)
		if !ok || len(attrs) == 0 {
			return spec, errors.New("attrs 必须包含至少一个格式属性")
		}
		for key, value := range attrs {
			switch key {
			case "bold", "italic", "underline":
				if _, ok := value.(bool); !ok {
					return spec, fmt.Errorf("%s 必须是布尔值", key)
				}
			case "background", "color":
				s, ok := value.(string)
				if !ok || !regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`).MatchString(s) {
					return spec, fmt.Errorf("%s 必须是 #RRGGBB 颜色", key)
				}
			default:
				return spec, fmt.Errorf("未验证的格式属性：%s", key)
			}
		}
		spec.Expected.Attrs = attrs
	case "sheet-merge":
		c1, r1, _ := cellRef(start)
		c2, r2, _ := cellRef(end)
		if c1 == c2 && r1 == r2 {
			return spec, errors.New("合并范围至少包含两个单元格")
		}
		if _, hasCell := payload["cell"]; hasCell {
			ref, err := phase3MergeRef(payload)
			if err != nil {
				return spec, err
			}
			if ref != start+":"+end {
				return spec, errors.New("cell/span 与 range 不一致")
			}
		}
		spec.Expected.MergeRef = start + ":" + end
		spec.Payload = map[string]any{"sheetName": sheet, "cell": start, "rowspan": r2 - r1 + 1, "colspan": colNumber(c2) - colNumber(c1) + 1}
	}
	if action != "sheet-append" && !phase3RangeContains(readRange, spec.Range) {
		return spec, errors.New("--read-range 必须完整覆盖操作范围")
	}
	return spec, nil
}

func phase3PayloadValues(payload map[string]any) ([][]any, error) {
	resource, ok := payload["resource"].(map[string]any)
	if !ok || len(resource) != 1 {
		return nil, errors.New("resource 必须仅包含 values 二维数组")
	}
	raw, ok := resource["values"].([]any)
	if !ok || len(raw) == 0 {
		return nil, errors.New("resource.values 必须是非空二维数组")
	}
	var values [][]any
	width := 0
	for _, item := range raw {
		row, ok := item.([]any)
		if !ok || len(row) == 0 {
			return nil, errors.New("resource.values 必须是非空二维数组")
		}
		if width == 0 {
			width = len(row)
		} else if width != len(row) {
			return nil, errors.New("resource.values 每行列数必须相同")
		}
		for _, v := range row {
			switch x := v.(type) {
			case string:
				if strings.HasPrefix(x, "=") {
					return nil, errors.New("phase3 暂不验收公式写入")
				}
			case bool:
			case float64:
				if math.IsNaN(x) || math.IsInf(x, 0) {
					return nil, errors.New("数值必须有限")
				}
			default:
				return nil, errors.New("单元格只接受字符串、数值或布尔值")
			}
		}
		values = append(values, append([]any(nil), row...))
	}
	if len(values) > phase3MaxCells/width {
		return nil, errors.New("单次验收最多 100000 个单元格")
	}
	return values, nil
}

func phase3Integer(value any, min, max int) (int, error) {
	switch v := value.(type) {
	case float64:
		if math.Trunc(v) != v || v < float64(min) || v > float64(max) {
			break
		}
		return int(v), nil
	case int:
		if v >= min && v <= max {
			return v, nil
		}
	case string:
		n, err := strconv.Atoi(v)
		if err == nil && n >= min && n <= max {
			return n, nil
		}
	}
	return 0, errors.New("无效整数")
}

func phase3RangeContains(outer, inner string) bool {
	os, a, b, ok := parseCellRange(outer)
	if !ok {
		return false
	}
	is, c, d, ok := parseCellRange(inner)
	if !ok || os != is {
		return false
	}
	ac, ar, _ := cellRef(a)
	bc, br, _ := cellRef(b)
	cc, cr, _ := cellRef(c)
	dc, dr, _ := cellRef(d)
	return colNumber(ac) <= colNumber(cc) && ar <= cr && colNumber(bc) >= colNumber(dc) && br >= dr
}

func phase3RangeCells(rng string) ([]string, error) {
	_, start, end, ok := parseCellRange(rng)
	if !ok {
		return nil, errors.New("无效单元格范围")
	}
	c1, r1, _ := cellRef(start)
	c2, r2, _ := cellRef(end)
	left, right := colNumber(c1), colNumber(c2)
	if left > right || r1 > r2 || right > 16384 || r2 > 1048576 {
		return nil, errors.New("单元格范围越界或逆序")
	}
	if (right - left + 1) > phase3MaxCells/(r2-r1+1) {
		return nil, errors.New("单次验收最多 100000 个单元格")
	}
	refs := make([]string, 0, (right-left+1)*(r2-r1+1))
	for row := r1; row <= r2; row++ {
		for col := left; col <= right; col++ {
			refs = append(refs, cellName(colName(col), row))
		}
	}
	return refs, nil
}

func phase3MergeRef(payload map[string]any) (string, error) {
	if cell := strings.TrimSpace(str(payload, "cell")); cell != "" {
		col, row, ok := cellRef(cell)
		if !ok {
			return "", errors.New("无效的合并起始单元格")
		}
		rows, e1 := phase3Integer(payload["rowspan"], 1, 1048576)
		cols, e2 := phase3Integer(payload["colspan"], 1, 16384)
		if e1 != nil || e2 != nil || row+rows-1 > 1048576 || colNumber(col)+cols-1 > 16384 {
			return "", errors.New("合并跨度无效或越界")
		}
		return cellName(col, row) + ":" + cellName(colName(colNumber(col)+cols-1), row+rows-1), nil
	}
	_, start, end, ok := parseCellRange(str(payload, "range"))
	if !ok {
		return "", errors.New("sheet-merge 需要 range 或 cell/rowspan/colspan")
	}
	return start + ":" + end, nil
}

func phase3SameOrigin(target, origin string) bool {
	t, e1 := url.Parse(target)
	o, e2 := url.Parse(origin)
	if e1 != nil || e2 != nil || t.User != nil || t.RawQuery != "" || t.Fragment != "" {
		return false
	}
	if !strings.EqualFold(t.Scheme, o.Scheme) || !strings.EqualFold(t.Host, o.Host) {
		return false
	}
	base := strings.TrimSuffix(o.Path, "/")
	return base == "" || strings.HasPrefix(t.Path, base+"/")
}
