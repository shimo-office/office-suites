package main

import (
	"fmt"
	"reflect"
	"strings"
)

func preparePhase3SlideTableSpan(p map[string]any, s *phase3SlideTableSpec) error {
	if len(p) != 5 {
		return fmt.Errorf("合并/拆分仅接受 slideId/tableName/operation/row/column；仅处理一行相邻两格")
	}
	for k, v := range map[string]*int{"row": &s.Row, "column": &s.Column} {
		n, ok := p[k].(float64)
		if !ok || n < 1 || n > 100 || n != float64(int(n)) {
			return fmt.Errorf("%s 必须为 1–100 的整数", k)
		}
		*v = int(n) - 1
	}
	return nil
}
func phase3CellUnmerged(c *artifactNode) bool {
	for _, k := range []string{"gridSpan", "rowSpan"} {
		v := c.attr(k)
		if v != "" && v != "1" {
			return false
		}
	}
	for _, k := range []string{"hMerge", "vMerge"} {
		v := c.attr(k)
		if v != "" && v != "0" && v != "false" {
			return false
		}
	}
	return true
}
func phase3HorizontalPair(a, b *artifactNode) bool {
	if a.attr("gridSpan") != "2" || (b.attr("hMerge") != "1" && b.attr("hMerge") != "true") {
		return false
	}
	for _, c := range []*artifactNode{a, b} {
		if v := c.attr("rowSpan"); v != "" && v != "1" {
			return false
		}
		if v := c.attr("vMerge"); v != "" && v != "0" && v != "false" {
			return false
		}
	}
	if v := a.attr("hMerge"); v != "" && v != "0" && v != "false" {
		return false
	}
	if v := b.attr("gridSpan"); v != "" && v != "1" {
		return false
	}
	return true
}
func phase3CellParagraphs(c *artifactNode) []*artifactNode {
	tx := phase3SlideChild(c, artifactDrawingNS, "txBody")
	if tx == nil {
		return nil
	}
	out := []*artifactNode{}
	for _, p := range tx.Children {
		if p.Name.Space == artifactDrawingNS && p.Name.Local == "p" {
			out = append(out, p)
		}
	}
	return out
}
func phase3MaskCellSpan(c *artifactNode) *artifactNode {
	out := *c
	out.Attr = nil
	out.Children = nil
	for _, a := range c.Attr {
		if a.Name.Space == "" && (a.Name.Local == "gridSpan" || a.Name.Local == "rowSpan" || a.Name.Local == "hMerge" || a.Name.Local == "vMerge") {
			continue
		}
		out.Attr = append(out.Attr, a)
	}
	for _, n := range c.Children {
		if c.Name.Space == artifactDrawingNS && c.Name.Local == "txBody" && n.Name.Space == artifactDrawingNS && n.Name.Local == "p" {
			continue
		}
		out.Children = append(out.Children, phase3MaskCellSpan(n))
	}
	return &out
}
func validatePhase3SpanBaseline(g phase3SlideTableGrid, s phase3SlideTableSpec) error {
	if s.Row < 0 || s.Row >= len(g.Rows) || s.Column < 0 || s.Column+1 >= len(g.Widths) {
		return fmt.Errorf("相邻两格范围超出表格")
	}
	for r, row := range g.Rows {
		for col, c := range row {
			if s.Operation == "split" && r == s.Row && (col == s.Column || col == s.Column+1) {
				continue
			}
			if !phase3CellUnmerged(c) {
				return fmt.Errorf("范围外存在合并格，无法安全验证")
			}
		}
	}
	for _, col := range []int{s.Column, s.Column + 1} {
		if s.Operation == "split" && col == s.Column+1 {
			if !phase3EmptyCellPlaceholder(g.Rows[s.Row][col]) {
				return fmt.Errorf("被合并右格包含内容，不能安全拆分")
			}
			continue
		}
		paragraphs := phase3CellParagraphs(g.Rows[s.Row][col])
		if len(paragraphs) == 0 {
			return fmt.Errorf("目标需要非空段落")
		}
		for _, p := range paragraphs {
			if strings.TrimSpace(phase3SlideText(p)) == "" {
				return fmt.Errorf("目标含空段落/纯空白段落，当前不执行合并拆分")
			}
		}
	}
	if s.Operation == "split" && !phase3HorizontalPair(g.Rows[s.Row][s.Column], g.Rows[s.Row][s.Column+1]) {
		return fmt.Errorf("目标不是恰好两列宽的水平合并单元格")
	}
	// Native merge removes the right physical cell and the PPTX writer emits
	// an empty hMerge continuation. Restrict input to matching cell properties
	// so this cannot silently discard a distinct fill, border, margin or bodyPr.
	if s.Operation == "merge" && !phase3EqualTableCell(phase3MaskCellSpan(g.Rows[s.Row][s.Column]), phase3MaskCellSpan(g.Rows[s.Row][s.Column+1])) {
		return fmt.Errorf("合并两格的非文本样式必须相同，避免原生合并丢失右格样式")
	}
	return nil
}
func verifyPhase3SlideTableSpan(b, a phase3SlideTableGrid, s phase3SlideTableSpec) error {
	if e := validatePhase3SpanBaseline(b, s); e != nil {
		return e
	}
	if len(a.Rows) != len(b.Rows) || !reflect.DeepEqual(a.Widths, b.Widths) || !reflect.DeepEqual(a.Heights, b.Heights) {
		return fmt.Errorf("合并/拆分改变网格宽度、行数或行高")
	}
	for r, row := range a.Rows {
		if len(row) != len(b.Rows[r]) {
			return fmt.Errorf("合并/拆分物理单元格数不符")
		}
		for col, c := range row {
			if r == s.Row && (col == s.Column || col == s.Column+1) {
				baseline := b.Rows[r][col]
				if col == s.Column+1 {
					if s.Operation == "merge" && phase3NativeMergeContinuation(c) {
						continue
					}
					if s.Operation == "split" && phase3NativeMergeContinuation(baseline) {
						baseline = b.Rows[r][s.Column]
					}
				}
				if !phase3EqualTableCell(phase3MaskCellSpan(c), phase3MaskCellSpan(baseline)) {
					return fmt.Errorf("合并/拆分改变目标单元格非文本样式")
				}
				continue
			}
			if !phase3EqualTableCell(c, b.Rows[r][col]) {
				return fmt.Errorf("合并/拆分改动范围外单元格")
			}
		}
	}
	left, right := a.Rows[s.Row][s.Column], a.Rows[s.Row][s.Column+1]
	var expected []*artifactNode
	if s.Operation == "merge" {
		if !phase3HorizontalPair(left, right) {
			return fmt.Errorf("目标两格未保存水平合并标记")
		}
		expected = append(expected, phase3CellParagraphs(b.Rows[s.Row][s.Column])...)
		expected = append(expected, phase3CellParagraphs(b.Rows[s.Row][s.Column+1])...)
	} else {
		if !phase3CellUnmerged(left) || !phase3CellUnmerged(right) {
			return fmt.Errorf("目标两格未真正拆开")
		}
		expected = phase3CellParagraphs(b.Rows[s.Row][s.Column])
	}
	actual := phase3CellParagraphs(left)
	if len(expected) != len(actual) || !phase3EmptyCellPlaceholder(right) {
		return fmt.Errorf("合并/拆分文字丢失或分布不符")
	}
	for i, p := range expected {
		if !phase3EqualTableCell(p, actual[i]) {
			return fmt.Errorf("合并/拆分改变正文及格式")
		}
	}
	return nil
}

func phase3EmptyCellPlaceholder(c *artifactNode) bool {
	if phase3NativeMergeContinuation(c) {
		return true
	}
	empty := true
	tx := phase3SlideChild(c, artifactDrawingNS, "txBody")
	if tx == nil {
		return false
	}
	phase3WalkSlideNode(tx, func(n *artifactNode) {
		if n.Name.Space == artifactDrawingNS && (n.Name.Local == "r" || n.Name.Local == "br" || n.Name.Local == "fld" || n.Name.Local == "t") {
			empty = false
		}
	})
	return empty
}

func phase3NativeMergeContinuation(c *artifactNode) bool {
	if c.Name.Space != artifactDrawingNS || c.Name.Local != "tc" || c.Text != "" || len(c.Children) != 0 || len(c.Attr) != 1 {
		return false
	}
	a := c.Attr[0]
	return a.Name.Space == "" && a.Name.Local == "hMerge" && (a.Value == "1" || a.Value == "true")
}
