package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type xlsxCell struct {
	Ref, Type, Value, Formula, StyleKey string
	Style                               int
}
type artifactSheetSnapshot struct {
	Name   string
	Cells  map[string]xlsxCell
	Merges []string
}
type artifactWorkbook struct {
	Sheets map[string]artifactSheetSnapshot
	Order  []string
	Styles *artifactNode
}

func readArtifactSheet(filename, sheetName string) (artifactSheetSnapshot, error) {
	z, entries, err := openArtifact(filename)
	if err != nil {
		return artifactSheetSnapshot{}, err
	}
	defer z.Close()
	if err = validateArtifactPackage(entries, "xl/workbook.xml"); err != nil {
		return artifactSheetSnapshot{}, err
	}
	wb, err := readArtifactWorkbook(entries)
	if err != nil {
		return artifactSheetSnapshot{}, err
	}
	sheet, ok := wb.Sheets[sheetName]
	if !ok {
		return artifactSheetSnapshot{}, fmt.Errorf("导出工作簿没有工作表 %q", sheetName)
	}
	return sheet, nil
}

func readArtifactWorkbook(entries map[string]*zip.File) (artifactWorkbook, error) {
	wb := artifactWorkbook{Sheets: map[string]artifactSheetSnapshot{}}
	root, err := artifactPart(entries, "xl/workbook.xml", artifactSheetNS, "workbook")
	if err != nil {
		return wb, err
	}
	rels, err := artifactRelations(entries, "xl/_rels/workbook.xml.rels", "xl")
	if err != nil {
		return wb, err
	}
	styles, err := artifactPart(entries, "xl/styles.xml", artifactSheetNS, "styleSheet")
	if err != nil {
		return wb, err
	}
	wb.Styles = styles
	stringsTable, err := artifactSharedStrings(entries)
	if err != nil {
		return wb, err
	}
	sheets := root.child("sheets")
	if sheets == nil || len(sheets.children("sheet")) == 0 {
		return wb, fmt.Errorf("工作簿没有工作表")
	}
	for _, n := range sheets.children("sheet") {
		name := n.attr("name")
		if name == "" {
			return wb, fmt.Errorf("工作表没有名称")
		}
		if _, exists := wb.Sheets[name]; exists {
			return wb, fmt.Errorf("重复工作表名称")
		}
		rel, ok := rels[relationID(n)]
		if !ok || rel.Kind != artifactOfficeRelNS+"/worksheet" {
			return wb, fmt.Errorf("工作表 %s 的关系不存在或类型错误", name)
		}
		body, e := artifactPart(entries, rel.Target, artifactSheetNS, "worksheet")
		if e != nil {
			return wb, e
		}
		snapshot, e := parseArtifactSheet(body, styles, stringsTable)
		if e != nil {
			return wb, fmt.Errorf("工作表 %s: %w", name, e)
		}
		snapshot.Name = name
		wb.Sheets[name] = snapshot
		wb.Order = append(wb.Order, name)
	}
	return wb, nil
}

func artifactSharedStrings(entries map[string]*zip.File) ([]string, error) {
	if entries["xl/sharedStrings.xml"] == nil {
		return nil, nil
	}
	root, err := artifactPart(entries, "xl/sharedStrings.xml", artifactSheetNS, "sst")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range root.children("si") {
		out = append(out, artifactRichText(n))
	}
	return out, nil
}
func artifactRichText(n *artifactNode) string {
	s := n.textOf("t")
	for _, run := range n.children("r") {
		s += run.textOf("t")
	}
	return s
}

func parseArtifactSheet(root, styles *artifactNode, stringsTable []string) (artifactSheetSnapshot, error) {
	out := artifactSheetSnapshot{Cells: map[string]xlsxCell{}, Merges: []string{}}
	data := root.child("sheetData")
	if data == nil || len(root.children("sheetData")) != 1 {
		return out, fmt.Errorf("缺少 sheetData")
	}
	rows := map[int]bool{}
	for _, row := range data.children("row") {
		rn, err := strconv.Atoi(row.attr("r"))
		if err != nil || rn < 1 || rn > 1048576 || rows[rn] {
			return out, fmt.Errorf("行号无效或重复")
		}
		rows[rn] = true
		for _, n := range row.children("c") {
			ref := n.attr("r")
			col, r, ok := cellRef(ref)
			if !ok || r != rn {
				return out, fmt.Errorf("单元格坐标与行号不一致: %s", ref)
			}
			ref = cellName(col, r)
			if _, exists := out.Cells[ref]; exists {
				return out, fmt.Errorf("重复单元格 %s", ref)
			}
			style := 0
			if raw := n.attr("s"); raw != "" {
				style, err = strconv.Atoi(raw)
				if err != nil || style < 0 {
					return out, fmt.Errorf("样式索引无效")
				}
			}
			key, e := artifactStyleKey(styles, style)
			if e != nil {
				return out, e
			}
			cell, e := artifactCellValue(n, stringsTable)
			if e != nil {
				return out, fmt.Errorf("%s: %w", ref, e)
			}
			cell.Ref = ref
			cell.Style = style
			cell.StyleKey = key
			out.Cells[ref] = cell
		}
	}
	if merges := root.child("mergeCells"); merges != nil {
		seen := map[string]bool{}
		for _, n := range merges.children("mergeCell") {
			name, start, end, ok := parseCellRange(n.attr("ref"))
			if !ok || name != "" || start == end {
				return out, fmt.Errorf("合并范围无效")
			}
			ref := start + ":" + end
			if seen[ref] {
				return out, fmt.Errorf("重复合并范围")
			}
			seen[ref] = true
			out.Merges = append(out.Merges, ref)
		}
	}
	sort.Strings(out.Merges)
	return out, nil
}

func artifactCellValue(n *artifactNode, stringsTable []string) (xlsxCell, error) {
	cell := xlsxCell{Formula: n.textOf("f")}
	v := n.textOf("v")
	if f := n.child("f"); f != nil && cell.Formula == "" {
		return cell, fmt.Errorf("不支持没有正文的共享公式")
	}
	switch n.attr("t") {
	case "s":
		index, err := strconv.Atoi(v)
		if err != nil || index < 0 || index >= len(stringsTable) {
			return cell, fmt.Errorf("共享字符串索引无效")
		}
		cell.Type = "string"
		cell.Value = stringsTable[index]
	case "inlineStr":
		text := n.child("is")
		if text == nil {
			return cell, fmt.Errorf("inlineStr 缺少 is")
		}
		cell.Type = "string"
		cell.Value = artifactRichText(text)
	case "str":
		cell.Type = "string"
		cell.Value = v
	case "b":
		cell.Type = "bool"
		if v == "1" {
			cell.Value = "true"
		} else if v == "0" {
			cell.Value = "false"
		} else {
			return cell, fmt.Errorf("布尔单元格值无效")
		}
	case "e":
		cell.Type = "error"
		cell.Value = v
	case "", "n":
		if v == "" {
			cell.Type = "blank"
		} else {
			normalized, ok := artifactNumber(v)
			if !ok {
				return cell, fmt.Errorf("数字单元格值无效")
			}
			cell.Type = "number"
			cell.Value = normalized
		}
	default:
		return cell, fmt.Errorf("不支持的单元格类型 %q", n.attr("t"))
	}
	return cell, nil
}

func artifactNumber(s string) (string, bool) {
	n, ok := new(big.Rat).SetString(s)
	if !ok {
		return "", false
	}
	return n.RatString(), true
}
func artifactScalarEqual(cell xlsxCell, expected any) bool {
	if cell.Formula != "" {
		return false
	}
	switch v := expected.(type) {
	case nil:
		return cell.Type == "blank" || (cell.Type == "string" && cell.Value == "")
	case string:
		return cell.Type == "string" && cell.Value == v
	case bool:
		return cell.Type == "bool" && cell.Value == strconv.FormatBool(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	switch expected.(type) {
	case float64, float32, int, int64, json.Number:
		v, ok := artifactNumber(canonicalScalar(expected))
		return ok && cell.Type == "number" && cell.Value == v
	default:
		return false
	}
}

// artifactScalarEqualWithNumericText accepts only the exporter's canonical
// decimal representation. It never converts user strings, booleans, formula
// caches, whitespace, leading zeros, or alternate exponent spellings.
func artifactScalarEqualWithNumericText(cell xlsxCell, expected any, allow bool) bool {
	if artifactScalarEqual(cell, expected) {
		return true
	}
	if !allow || cell.Type != "string" || cell.Formula != "" {
		return false
	}
	var text string
	switch v := expected.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
		text = strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
		text = strconv.FormatFloat(float64(v), 'f', -1, 32)
	case int:
		text = strconv.Itoa(v)
	case int64:
		text = strconv.FormatInt(v, 10)
	default:
		return false
	}
	return cell.Value == text
}

func verifyWorkbookExpectation(wb artifactWorkbook, expected artifactExpectation, ev *ArtifactEvidence) error {
	hasTarget := len(expected.Values) > 0 || len(expected.Attrs) > 0 || expected.MergeRef != ""
	if !hasTarget {
		return nil
	}
	name, start, end, rangeOK := parseCellRange(expected.Range)
	if expected.SheetName != "" {
		if name != "" && name != expected.SheetName {
			return fmt.Errorf("工作表名称与 Range 不一致")
		}
		name = expected.SheetName
	}
	if name == "" {
		return fmt.Errorf("目标校验必须提供工作表名称")
	}
	sheet, ok := wb.Sheets[name]
	if !ok {
		return fmt.Errorf("导出工作簿没有工作表 %q", name)
	}
	ev.Details["sheet-name"] = name
	ev.Details["cell-nodes"] = strconv.Itoa(len(sheet.Cells))
	ev.Details["scope"] = "target-state"
	if len(expected.Values) > 0 {
		if !rangeOK {
			return fmt.Errorf("值校验必须提供有效 Range")
		}
		sc, sr, _ := cellRef(start)
		ec, er, _ := cellRef(end)
		width := colNumber(ec) - colNumber(sc) + 1
		if len(expected.Values) > er-sr+1 {
			return fmt.Errorf("请求行数超过校验范围")
		}
		for r, row := range expected.Values {
			if len(row) == 0 || len(row) > width {
				return fmt.Errorf("请求列数为空或超过校验范围")
			}
			for c, v := range row {
				ref := cellName(colName(colNumber(sc)+c), sr+r)
				cell, present := sheet.Cells[ref]
				if !present {
					cell = xlsxCell{Type: "blank"}
				}
				if !artifactScalarEqualWithNumericText(cell, v, expected.NumericText) {
					return fmt.Errorf("物证单元格 %s!%s 与请求不一致", name, ref)
				}
			}
		}
		ev.Checks["values-evidence"] = true
		if expected.NumericText {
			ev.Details["numeric-text"] = "exporter-compatibility; numeric types require independent API verification"
		}
	}
	if len(expected.Attrs) > 0 {
		if !rangeOK {
			return fmt.Errorf("格式校验必须提供有效 Range")
		}
		sc, sr, _ := cellRef(start)
		ec, er, _ := cellRef(end)
		width := colNumber(ec) - colNumber(sc) + 1
		if (er-sr+1)*width > 100000 {
			return fmt.Errorf("格式校验范围超过 100000 个单元格")
		}
		for r := sr; r <= er; r++ {
			for c := colNumber(sc); c <= colNumber(ec); c++ {
				ref := cellName(colName(c), r)
				cell, present := sheet.Cells[ref]
				if !present {
					return fmt.Errorf("目标格式单元格 %s 不存在，不能验证继承格式", ref)
				}
				for attr, want := range expected.Attrs {
					ok, e := artifactCellAttr(wb.Styles, cell.Style, attr, want)
					if e != nil {
						return e
					}
					if !ok {
						return fmt.Errorf("物证单元格 %s!%s 的 %s 未匹配", name, ref, attr)
					}
				}
			}
		}
		ev.Checks["target-styles"] = true
	}
	if expected.MergeRef != "" {
		_, a, b, ok := parseCellRange(expected.MergeRef)
		if !ok || a == b {
			return fmt.Errorf("合并校验范围无效")
		}
		ref := a + ":" + b
		found := false
		for _, got := range sheet.Merges {
			if got == ref {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("工作表 %s 缺少合并范围 %s", name, ref)
		}
		ev.Checks["merge-evidence"] = true
	}
	return nil
}

func canonicalScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(x)
	}
}

var artifactCellPattern = regexp.MustCompile(`^\$?([A-Za-z]{1,3})\$?([1-9][0-9]{0,6})$`)

func cellRef(ref string) (col string, row int, ok bool) {
	m := artifactCellPattern.FindStringSubmatch(ref)
	if m == nil {
		return "", 0, false
	}
	row, err := strconv.Atoi(m[2])
	col = strings.ToUpper(m[1])
	return col, row, err == nil && row <= 1048576 && colNumber(col) <= 16384
}
func parseCellRange(rng string) (sheet, start, end string, ok bool) {
	rng = strings.TrimSpace(rng)
	if strings.Count(rng, "!") > 1 {
		return "", "", "", false
	}
	if i := strings.IndexByte(rng, '!'); i >= 0 {
		sheet = rng[:i]
		rng = rng[i+1:]
		if sheet == "" {
			return "", "", "", false
		}
		if strings.HasPrefix(sheet, "'") || strings.HasSuffix(sheet, "'") {
			if len(sheet) < 3 || sheet[0] != '\'' || sheet[len(sheet)-1] != '\'' {
				return "", "", "", false
			}
			sheet = strings.ReplaceAll(sheet[1:len(sheet)-1], "''", "'")
		}
	}
	parts := strings.Split(rng, ":")
	if len(parts) < 1 || len(parts) > 2 {
		return "", "", "", false
	}
	start = parts[0]
	end = start
	if len(parts) == 2 {
		end = parts[1]
	}
	sc, sr, sok := cellRef(start)
	ec, er, eok := cellRef(end)
	if !sok || !eok || colNumber(sc) > colNumber(ec) || sr > er {
		return "", "", "", false
	}
	return sheet, cellName(sc, sr), cellName(ec, er), true
}
func cellName(col string, row int) string { return strings.ToUpper(col) + strconv.Itoa(row) }
func colNumber(col string) int {
	n := 0
	for _, r := range strings.ToUpper(col) {
		n = n*26 + int(r-'A'+1)
	}
	return n
}
func colName(n int) string {
	if n < 1 || n > 16384 {
		return ""
	}
	out := ""
	for n > 0 {
		n--
		out = string(rune('A'+n%26)) + out
		n /= 26
	}
	return out
}
