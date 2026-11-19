package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// The live values API omits empty rows. It can corroborate ordered values,
// but cannot prove physical coordinates. Coordinates are checked separately
// against the exported XLSX; no metadata or unordered membership is evidence.
func phase3ReadbackRows(value any, requestedRange string) ([][]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("sheet-values 未返回 values 对象")
	}
	if data, ok := object["data"].(map[string]any); ok {
		object = data
	}
	if rng, exists := object["range"]; exists {
		text, ok := rng.(string)
		if !ok || text != requestedRange {
			return nil, errors.New("sheet-values 返回范围与请求范围不一致")
		}
	}
	raw, exists := object["values"]
	if !exists {
		return nil, errors.New("sheet-values 缺少 values")
	}
	if raw == nil {
		return [][]any{}, nil
	}
	rows, ok := raw.([]any)
	if !ok {
		return nil, errors.New("sheet-values 不是二维数组")
	}
	_, start, end, ok := parseCellRange(requestedRange)
	if !ok {
		return nil, errors.New("无效读回范围")
	}
	col, row, _ := cellRef(start)
	endCol, endRow, _ := cellRef(end)
	if len(rows) > endRow-row+1 {
		return nil, errors.New("读回行数超出请求范围")
	}
	result := [][]any{}
	for _, item := range rows {
		columns, ok := item.([]any)
		if !ok || len(columns) > colNumber(endCol)-colNumber(col)+1 {
			return nil, errors.New("读回不是范围内的二维数组")
		}
		last := -1
		for c, v := range columns {
			switch v.(type) {
			case nil, string, float64, bool:
			default:
				return nil, errors.New("读回包含不支持的单元格值")
			}
			if v != nil && v != "" {
				last = c
			}
		}
		if last >= 0 {
			result = append(result, columns[:last+1])
		}
	}
	return result, nil
}

func readbackContainsExpected(readback any, payload map[string]any) bool {
	expected, err := phase3PayloadValues(payload)
	if err != nil {
		return false
	}
	actual, err := phase3ReadbackRows(readback, str(payload, "range"))
	if err != nil {
		return false
	}
	return reflect.DeepEqual(actual, expected)
}

func phase3ValidateSnapshot(before artifactSheetSnapshot, spec *phase3Spec) error {
	switch spec.Action {
	case "sheet-append", "sheet-insert", "sheet-delete":
		if len(before.Merges) > 0 {
			return errors.New("结构操作的工作表包含合并单元格，当前无法完整验证，未执行")
		}
		maxRow := 0
		for _, cell := range before.Cells {
			if cell.Formula != "" {
				return errors.New("结构操作的工作表包含公式，当前无法完整验证引用位移，未执行")
			}
			if phase3EmptyCell(cell) {
				continue
			}
			_, row, _ := cellRef(cell.Ref)
			if row > maxRow {
				maxRow = row
			}
		}
		if spec.Action == "sheet-append" {
			rows, cols := len(spec.Expected.Values), len(spec.Expected.Values[0])
			if maxRow+rows > 1048576 {
				return errors.New("追加行超出工作表边界")
			}
			spec.Expected.Range = spec.Sheet + "!A" + strconv.Itoa(maxRow+1) + ":" + cellName(colName(cols), maxRow+rows)
			spec.Range = spec.Expected.Range
			if !phase3RangeContains(spec.ReadRange, spec.Range) {
				return errors.New("--read-range 未覆盖实际追加区域：" + spec.Range)
			}
			return nil
		}
		// An insert/delete beyond the last nonblank row is not observable in XLSX
		// because empty rows may be omitted. Reject it before submitting anything.
		if maxRow < spec.Index+1 {
			return errors.New("操作位置之后没有可观察数据，无法证明空行操作，未执行")
		}
		if spec.Action == "sheet-insert" && maxRow+spec.Count > 1048576 {
			return errors.New("插行将使已有数据超出工作表边界")
		}
	}
	return nil
}

func phase3EmptyCell(cell xlsxCell) bool {
	return cell.Formula == "" && (cell.Type == "blank" || cell.Value == "")
}

func phase3CellEquivalent(a, b xlsxCell) bool {
	return a.Type == b.Type && a.Value == b.Value && a.Formula == b.Formula && a.StyleKey == b.StyleKey
}

func phase3VerifyStructural(before, after artifactSheetSnapshot, spec phase3Spec) error {
	if spec.Action != "sheet-append" && spec.Action != "sheet-insert" && spec.Action != "sheet-delete" {
		return nil
	}
	// Value-empty cells can still carry fills, borders or number formats.
	// Retain every explicit cell, including style 0: its actual definition
	// need not be unformatted and may change across exports.
	expected := map[string]xlsxCell{}
	for ref, cell := range before.Cells {
		col, row, ok := cellRef(ref)
		if !ok {
			return errors.New("导出物证包含无效坐标")
		}
		if spec.Action == "sheet-insert" && row >= spec.Index+1 {
			row += spec.Count
		}
		if spec.Action == "sheet-delete" && row >= spec.Index+1 {
			if row < spec.Index+1+spec.Count {
				continue
			}
			row -= spec.Count
		}
		expected[cellName(col, row)] = cell
	}
	actual := map[string]xlsxCell{}
	for ref, cell := range after.Cells {
		actual[ref] = cell
	}
	if spec.Action == "sheet-append" {
		refs, err := phase3RangeCells(spec.Range)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			// Appending values into an explicitly formatted blank cell must
			// preserve that cell's pre-existing style.
			if old, exists := expected[ref]; exists {
				added, present := actual[ref]
				if !present || added.StyleKey != old.StyleKey {
					return fmt.Errorf("%s 追加写入未保留原空单元格样式", ref)
				}
				delete(expected, ref)
			}
			delete(actual, ref)
		}
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("结构变化与请求不符：剩余显式单元格 %d，期望 %d", len(actual), len(expected))
	}
	for ref, want := range expected {
		got, ok := actual[ref]
		if !ok || !phase3CellEquivalent(got, want) {
			return fmt.Errorf("%s 的值、类型或样式未按预期移动/保留", ref)
		}
	}
	// Insert/delete must actually change meaningful coordinates or data; identical
	// before/after packages cannot prove a structural write even if HTTP is 204.
	if spec.Action != "sheet-append" {
		changed := false
		for ref, cell := range before.Cells {
			afterCell, ok := after.Cells[ref]
			if !ok || !phase3CellEquivalent(cell, afterCell) {
				changed = true
				break
			}
		}
		if !changed {
			return errors.New("前后单元格状态没有变化，不能证明行操作")
		}
	}
	if !reflect.DeepEqual(before.Merges, after.Merges) {
		return errors.New("结构操作意外改变了合并区域")
	}
	return nil
}

func phase3VerifyReadback(result any, after artifactSheetSnapshot, rng string) error {
	actual, err := phase3ReadbackRows(result, rng)
	if err != nil {
		return err
	}
	_, start, end, ok := parseCellRange(rng)
	if !ok {
		return errors.New("无效读回范围")
	}
	c1, r1, _ := cellRef(start)
	c2, r2, _ := cellRef(end)
	expected := [][]xlsxCell{}
	for row := r1; row <= r2; row++ {
		cells := []xlsxCell{}
		last := -1
		for col := colNumber(c1); col <= colNumber(c2); col++ {
			ref := cellName(colName(col), row)
			cell, exists := after.Cells[ref]
			if !exists {
				cell = xlsxCell{Ref: ref, Type: "blank"}
			}
			if cell.Formula != "" {
				return fmt.Errorf("%s 含公式，不能以计算值代替公式验证", ref)
			}
			if !phase3EmptyCell(cell) {
				last = len(cells)
			}
			cells = append(cells, cell)
		}
		if last >= 0 {
			expected = append(expected, cells[:last+1])
		}
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("读回非空行数 %d，与导出范围非空行数 %d 不一致", len(actual), len(expected))
	}
	for row, want := range expected {
		if len(actual[row]) != len(want) {
			return fmt.Errorf("读回第 %d 个非空行的列数与导出不一致", row+1)
		}
		for col, cell := range want {
			got := actual[row][col]
			if phase3EmptyCell(cell) && (got == nil || got == "") {
				continue
			}
			if !artifactScalarEqualWithNumericText(cell, got, true) {
				return fmt.Errorf("读回值/顺序/类型与导出中的 %s 不一致", cell.Ref)
			}
		}
	}
	return nil
}

func phase3VerifyRequestedReadback(readback any, after artifactSheetSnapshot, spec phase3Spec) error {
	if len(spec.Expected.Values) == 0 {
		return nil
	}
	rows, err := phase3ReadbackRows(readback, spec.ReadRange)
	if err != nil {
		return err
	}
	_, start, end, _ := parseCellRange(spec.ReadRange)
	c1, r1, _ := cellRef(start)
	c2, r2, _ := cellRef(end)
	actual := map[string]any{}
	next := 0
	for row := r1; row <= r2; row++ {
		nonempty := false
		for col := colNumber(c1); col <= colNumber(c2); col++ {
			cell, exists := after.Cells[cellName(colName(col), row)]
			if exists && !phase3EmptyCell(cell) {
				nonempty = true
				break
			}
		}
		if !nonempty {
			continue
		}
		if next >= len(rows) {
			return errors.New("API 读回缺少目标数据行")
		}
		for col, value := range rows[next] {
			actual[cellName(colName(colNumber(c1)+col), row)] = value
		}
		next++
	}
	_, target, _, _ := parseCellRange(spec.Expected.Range)
	col, row, _ := cellRef(target)
	for r, values := range spec.Expected.Values {
		for c, want := range values {
			ref := cellName(colName(colNumber(col)+c), row+r)
			got, present := actual[ref]
			if want == "" && (!present || got == nil || got == "") {
				continue
			}
			if !present || !reflect.DeepEqual(got, want) {
				return fmt.Errorf("%s API 读回类型/值与原始请求不一致", ref)
			}
		}
	}
	return nil
}

func phase3WriteEvidence(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0600)
}

// Keep success output distinct from a package's structural validity.
func phase3ProofSummary(spec phase3Spec) []string {
	checks := []string{"validated-request", "same-origin-spreadsheet", "ordered-sheet-values-vs-export", "targeted-xlsx-postcondition"}
	if strings.HasSuffix(spec.Action, "append") || strings.HasSuffix(spec.Action, "insert") || strings.HasSuffix(spec.Action, "delete") {
		checks = append(checks, "before-after-cell-transition")
	}
	return checks
}
