package main

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

func documentCellSpan(c documentTableCell) int {
	if c.GridSpan < 1 {
		return 1
	}
	return c.GridSpan
}
func documentRowGridWidth(row []documentTableCell) int {
	n := 0
	for _, c := range row {
		n += documentCellSpan(c)
	}
	return n
}
func prepareDocumentTableSpanParams(p map[string]any, s *documentTableSpec) error {
	for key, target := range map[string]*int{"endColumn": &s.EndColumn, "columns": &s.Columns} {
		value, exists := p[key]
		if !exists {
			continue
		}
		if (key == "endColumn" && s.Operation != "merge") || (key == "columns" && s.Operation != "split") {
			return fmt.Errorf("%s 不适用于此操作", key)
		}
		n, ok := value.(float64)
		if !ok || n < 2 || n > 1000 || n != math.Trunc(n) {
			return fmt.Errorf("%s 必须为2至1000的整数", key)
		}
		*target = int(n)
	}
	if s.Operation == "merge" && (s.EndColumn <= s.Column || s.EndColumn-s.Column >= 20) {
		return fmt.Errorf("merge需要同一行相邻2至20格的endColumn")
	}
	if s.Operation == "split" && (s.Columns < 2 || s.Columns > 20) {
		return fmt.Errorf("split需要2至20个columns，当前仅横向拆分")
	}
	return nil
}
func readDocumentSpanCell(cell *artifactNode, pIndex map[*artifactNode]int) (documentTableCell, error) {
	c := documentTableCell{GridSpan: 1, EmptyContent: true, Envelope: documentSpanEnvelope(cell, "p", "tcPr")}
	for _, child := range cell.Children {
		if child.Name.Space != artifactWordNS || (child.Name.Local != "tcPr" && child.Name.Local != "p") {
			return c, fmt.Errorf("单元格含未支持的非段落结构")
		}
	}
	props := cell.child("tcPr")
	if node := documentStyleChild(props, "gridSpan"); node != nil {
		n, err := strconv.Atoi(documentAttr(node, "val"))
		if err != nil || n < 1 || n > 1000 {
			return c, fmt.Errorf("单元格gridSpan无效")
		}
		c.GridSpan = n
	}
	var merges []*artifactNode
	if props != nil {
		for _, child := range props.Children {
			if child.Name.Local == "vMerge" {
				merges = append(merges, child)
			}
		}
	}
	if len(merges) > 0 {
		node := merges[0]
		if len(merges) != 1 || node.Name.Space != artifactWordNS || len(node.Children) != 0 || strings.TrimSpace(node.Text) != "" || len(node.Attr) > 1 {
			return c, fmt.Errorf("vMerge结构不唯一或无效")
		}
		if len(node.Attr) == 1 && (node.Attr[0].Name.Space != artifactWordNS || node.Attr[0].Name.Local != "val") {
			return c, fmt.Errorf("vMerge含未支持属性")
		}
		c.VMerge = documentAttr(node, "val")
		if c.VMerge == "" {
			c.VMerge = "continue"
		}
		if c.VMerge != "continue" && c.VMerge != "restart" {
			return c, fmt.Errorf("vMerge无效")
		}
	}
	width := documentStyleChild(props, "tcW")
	c.WidthProperties = documentTableExactKey(width)
	c.WidthType = documentAttr(width, "type")
	c.WidthValue, _ = strconv.ParseFloat(documentAttr(width, "w"), 64)
	if c.WidthType == "dxa" {
		value, err := strconv.ParseUint(documentAttr(width, "w"), 10, 32)
		if err != nil || value == 0 {
			return c, fmt.Errorf("dxa单元格宽度必须是有效正整数")
		}
		c.WidthValue = float64(value)
	}
	normalized := documentStyleClone(props)
	documentStyleStrip(normalized, "tcW")
	documentStyleStrip(normalized, "gridSpan")
	documentStyleStrip(normalized, "vMerge")
	if normalized != nil && len(normalized.Children) == 0 && len(normalized.Attr) == 0 {
		normalized = nil
	}
	c.CellProperties = documentTableExactKey(normalized)
	paras := documentChildren(cell, "p")
	if len(paras) == 0 {
		return c, fmt.Errorf("单元格没有可定位段落")
	}
	for _, p := range paras {
		c.Paragraphs = append(c.Paragraphs, pIndex[p])
		c.ParagraphKeys = append(c.ParagraphKeys, documentTableExactKey(p))
		c.MergeParagraphKeys = append(c.MergeParagraphKeys, documentMergeParagraphKey(p))
		var text strings.Builder
		for _, run := range documentChildren(p, "r") {
			for _, node := range run.Children {
				if node.Name.Space != artifactWordNS {
					continue
				}
				switch node.Name.Local {
				case "t":
					text.WriteString(node.Text)
				case "tab":
					text.WriteByte('\t')
				case "br", "cr":
					text.WriteByte('\n')
				}
			}
		}
		c.ParagraphTexts = append(c.ParagraphTexts, text.String())
		c.EmptyContent = c.EmptyContent && documentSpanEmptyParagraph(p)
	}
	c.Paragraph = c.Paragraphs[0]
	c.Text = strings.Join(c.ParagraphTexts, "\n")
	c.Style = documentTableNodeKey(cell, false)
	c.RebuiltStyle = documentTableRebuiltCellStyle(cell)
	return c, nil
}
func documentSpanEmptyParagraph(p *artifactNode) bool {
	for _, n := range p.Children {
		if n.Name.Space != artifactWordNS {
			return false
		}
		if n.Name.Local == "pPr" {
			continue
		}
		if n.Name.Local != "r" {
			return false
		}
		for _, r := range n.Children {
			if r.Name.Space != artifactWordNS {
				return false
			}
			if r.Name.Local == "rPr" {
				continue
			}
			if r.Name.Local != "t" || r.Text != "" {
				return false
			}
		}
	}
	return true
}
func validateDocumentTableSpan(before documentTablesSnapshot, s documentTableSpec) error {
	t := before.Tables[s.Table-1]
	row := t.Rows[s.Row-1]
	cell := row[s.Column-1]
	if len(t.GridWidths) == 0 || len(cell.Paragraphs) == 0 {
		return fmt.Errorf("合并拆分缺少真实grid和段落结构")
	}
	for _, table := range before.Tables {
		for _, r := range table.Rows {
			for _, c := range r {
				if c.VMerge == "continue" {
					return fmt.Errorf("当前合并拆分拒绝纵向合并结构")
				}
			}
		}
	}
	if s.Operation == "merge" {
		if err := validateDocumentMergePreferredWidths(t); err != nil {
			return err
		}
		if s.EndColumn > len(row) {
			return fmt.Errorf("合并终点超出行")
		}
		for _, c := range row[s.Column-1 : s.EndColumn] {
			if documentCellSpan(c) != 1 || c.VMerge != "" || c.EmptyContent {
				return fmt.Errorf("合并范围必须为未合并且非空的相邻单元格")
			}
		}
	} else if documentCellSpan(cell) != s.Columns {
		return fmt.Errorf("拆分列数必须等于目标现有横向gridSpan")
	}
	start := cell.StartGridCol
	count := s.Columns
	if s.Operation == "merge" {
		count = s.EndColumn - s.Column + 1
	}
	if start < 0 || start+count > len(t.GridWidths) {
		return fmt.Errorf("目标grid范围无效")
	}
	for _, width := range t.GridWidths[start : start+count] {
		if math.IsNaN(width) || math.IsInf(width, 0) || width <= 0 || math.Abs(width-t.GridWidths[start]) > 1 {
			return fmt.Errorf("只对既有等宽逻辑网格执行可逆横向合并拆分")
		}
	}
	for _, c := range row {
		if c.WidthType != "dxa" || math.IsNaN(c.WidthValue) || math.IsInf(c.WidthValue, 0) || c.WidthValue <= 0 {
			return fmt.Errorf("目标行单元格宽度必须为明确dxa")
		}
	}
	return nil
}

// Native mergeCells rewrites every cell's preferred width when a selected
// preferred width diverges from its grid width. Refuse that state before opening
// the editor. Keep the supported table narrower than the native >6-twip trigger:
// one absolute width type and agreement within the existing export rounding unit.
func validateDocumentMergePreferredWidths(t documentTable) error {
	for _, row := range t.Rows {
		for _, cell := range row {
			start, end := cell.StartGridCol, cell.StartGridCol+documentCellSpan(cell)
			if cell.WidthType != "dxa" || start < 0 || end > len(t.GridWidths) || start >= end {
				return fmt.Errorf("合并要求全表绝对首选宽度与实际网格一致，避免原生全表宽度重写")
			}
			width := 0.0
			for _, value := range t.GridWidths[start:end] {
				width += value
			}
			if !documentSpanValidWidth(cell.WidthValue) || !documentSpanValidWidth(width) || math.Abs(cell.WidthValue-width) > 1 {
				return fmt.Errorf("合并前首选宽度与网格不一致，原生合并可能重写非目标单元格宽度，未写入")
			}
		}
	}
	return nil
}
func verifyDocumentTableSpan(before, after documentTablesSnapshot, s documentTableSpec) error {
	if !reflect.DeepEqual(before.Assets, after.Assets) {
		return fmt.Errorf("表格操作改变了图片资源内容或关系绑定")
	}
	for _, table := range after.Tables {
		for _, row := range table.Rows {
			for _, c := range row {
				if c.VMerge == "continue" {
					return fmt.Errorf("横向表格操作产生了纵向合并延伸")
				}
			}
		}
	}
	if err := verifyDocumentResources(before.StyleResources, after.StyleResources); err != nil {
		return err
	}
	if _, err := documentTableAnchor(before, s); err != nil {
		return err
	}
	if len(before.Tables) != len(after.Tables) || before.OutsideTables != after.OutsideTables {
		return fmt.Errorf("合并拆分改变了表格以外的正文")
	}
	for ti, bt := range before.Tables {
		at := after.Tables[ti]
		if ti != s.Table-1 {
			if bt.RawStructure != at.RawStructure {
				return fmt.Errorf("合并拆分改变了其他表格")
			}
			continue
		}
		if bt.RawStructure == at.RawStructure {
			return fmt.Errorf("合并拆分没有实际结构变化")
		}
		if s.Operation == "split" {
			bt.RowStyles, _, bt.RowEnvelopes = documentTableRebuiltKeys(bt)
			at.RowStyles, _, at.RowEnvelopes = documentTableRebuiltKeys(at)
		}
		if bt.Envelope != at.Envelope || !reflect.DeepEqual(bt.RowEnvelopes, at.RowEnvelopes) || bt.Properties != at.Properties || bt.Grid != at.Grid || !reflect.DeepEqual(bt.RowStyles, at.RowStyles) || len(bt.Rows) != len(at.Rows) {
			return fmt.Errorf("表格grid、行数或其他表格属性意外变化")
		}
		for ri, brow := range bt.Rows {
			arow := at.Rows[ri]
			if ri != s.Row-1 {
				if !documentSpanSameRow(brow, arow) {
					return fmt.Errorf("非目标行改变")
				}
				continue
			}
			col := s.Column - 1
			removed, added := s.EndColumn-s.Column+1, 1
			if s.Operation == "split" {
				removed, added = 1, s.Columns
			}
			if len(arow) != len(brow)-removed+added {
				return fmt.Errorf("目标行物理单元格数不符合合并拆分")
			}
			if !documentSpanSameRow(brow[:col], arow[:col]) || !documentSpanSameRow(brow[col+removed:], arow[col+added:]) {
				return fmt.Errorf("目标范围外单元格改变")
			}
			old := brow[col]
			keys := []string{}
			width := 0.0
			for _, c := range brow[col : col+removed] {
				paragraphKeys := c.ParagraphKeys
				if s.Operation == "merge" {
					paragraphKeys = documentMergeParagraphKeys(c)
				}
				keys = append(keys, paragraphKeys...)
				width += c.WidthValue
			}
			head := arow[col]
			headKeys := head.ParagraphKeys
			if s.Operation == "merge" {
				headKeys = documentMergeParagraphKeys(head)
			}
			expectedSpan := removed
			if s.Operation == "split" {
				expectedSpan = 1
				width = old.WidthValue / float64(added)
			}
			if s.Operation == "split" && head.VMerge != old.VMerge {
				return fmt.Errorf("拆分改变了原有垂直标记")
			}
			if documentCellSpan(head) != expectedSpan || head.VMerge == "continue" || head.StartGridCol != old.StartGridCol || head.WidthType != "dxa" || !documentSpanValidWidth(head.WidthValue) || math.Abs(head.WidthValue-width) > 1 || head.Envelope != old.Envelope || head.CellProperties != old.CellProperties || !reflect.DeepEqual(headKeys, keys) {
				return fmt.Errorf("目标跨度、宽度、属性或完整段落未按预期保存")
			}
			for j := 1; j < added; j++ {
				c := arow[col+j]
				if documentCellSpan(c) != 1 || c.VMerge != old.VMerge || c.StartGridCol != old.StartGridCol+j || c.WidthType != "dxa" || !documentSpanValidWidth(c.WidthValue) || math.Abs(c.WidthValue-width) > 1 || c.CellProperties != old.CellProperties || !c.EmptyContent || len(c.Paragraphs) != 1 {
					return fmt.Errorf("拆分新增格不是预期空白等宽单元格")
				}
			}
		}
	}
	return nil
}
func documentSpanSameRow(a, b []documentTableCell) bool {
	if len(a) != len(b) {
		return false
	}
	for i, c := range a {
		if c.Style != b[i].Style || c.WidthProperties != b[i].WidthProperties || documentCellSpan(c) != documentCellSpan(b[i]) || c.VMerge != b[i].VMerge || c.StartGridCol != b[i].StartGridCol {
			return false
		}
	}
	return true
}

func documentSpanEnvelope(n *artifactNode, removed ...string) string {
	clone := documentStyleClone(n)
	for _, name := range removed {
		documentStyleStrip(clone, name)
	}
	return documentTableExactKey(clone)
}

func documentSpanValidWidth(width float64) bool {
	return !math.IsNaN(width) && !math.IsInf(width, 0) && width > 0 && width == math.Trunc(width)
}
