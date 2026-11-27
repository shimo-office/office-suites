package main

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

func documentStyleRoot(path string) (*artifactNode, error) {
	z, entries, err := openArtifact(path)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	return artifactPart(entries, "word/document.xml", artifactWordNS, "document")
}
func documentStyleChild(n *artifactNode, name string) *artifactNode {
	if n == nil {
		return nil
	}
	return n.child(name)
}
func documentStyleStrip(n *artifactNode, names ...string) {
	if n == nil {
		return
	}
	children := make([]*artifactNode, 0, len(n.Children))
	for _, c := range n.Children {
		remove := false
		for _, name := range names {
			if c.Name.Space == artifactWordNS && c.Name.Local == name {
				remove = true
				break
			}
		}
		if !remove {
			children = append(children, c)
		}
	}
	n.Children = children
}
func documentStyleMaskAttrs(n *artifactNode, names ...string) {
	if n == nil {
		return
	}
	attrs := n.Attr[:0:0]
	for _, a := range n.Attr {
		remove := false
		for _, name := range names {
			if a.Name.Space == artifactWordNS && a.Name.Local == name {
				remove = true
				break
			}
		}
		if !remove {
			attrs = append(attrs, a)
		}
	}
	n.Attr = attrs
}
func documentStyleMaskBorders(n *artifactNode, zeroSpaceSafe bool, sides ...string) {
	for _, side := range sides {
		border := documentStyleChild(n, side)
		if zeroSpaceSafe && border != nil && border.Name.Space == artifactWordNS {
			count := 0
			for _, attr := range border.Attr {
				if attr.Name.Space == artifactWordNS && attr.Name.Local == "space" {
					count++
				}
			}
			if count == 1 && documentAttr(border, "space") == "0" {
				documentStyleMaskAttrs(border, "space")
			}
		}
		documentStyleMaskAttrs(border, "val", "sz", "color", "themeColor", "themeTint", "themeShade")
	}
}

// Omitted border spacing may serialize as zero. Keep exact comparison whenever
// a style resource supplies spacing or the selected table has a nonzero value
// that an explicit border default could hide.
func documentStyleZeroSpaceSafe(root *artifactNode, table int, resources map[string]string) bool {
	for _, key := range resources {
		if strings.Contains(key, artifactWordNS+"|space=") {
			return false
		}
	}
	var safe func(*artifactNode) bool
	safe = func(n *artifactNode) bool {
		for _, a := range n.Attr {
			if a.Name.Space == artifactWordNS && a.Name.Local == "space" && a.Value != "0" {
				return false
			}
		}
		for _, c := range n.Children {
			if !safe(c) {
				return false
			}
		}
		return true
	}
	return safe(documentChildren(root, "tbl")[table-1])
}

// Saving any cell style can make the target table's existing six borders
// serialize their omitted zero-spacing default. Normalize only that one added
// attribute when the whole border otherwise matches its untouched baseline.
func documentStyleNormalizeTableBorderDefaults(before, after *artifactNode, table int) {
	child := func(n *artifactNode, name string) *artifactNode {
		if n == nil || n.Name.Space != artifactWordNS {
			return nil
		}
		var found *artifactNode
		for _, c := range n.Children {
			if c.Name.Space == artifactWordNS && c.Name.Local == name {
				if found != nil {
					return nil
				}
				found = c
			}
		}
		return found
	}
	b := child(child(documentChildren(before, "tbl")[table-1], "tblPr"), "tblBorders")
	a := child(child(documentChildren(after, "tbl")[table-1], "tblPr"), "tblBorders")
	for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		old, next := child(b, side), child(a, side)
		if old == nil || next == nil {
			continue
		}
		count, originalCount := 0, 0
		for _, attr := range old.Attr {
			if attr.Name.Space == artifactWordNS && attr.Name.Local == "space" {
				originalCount++
			}
		}
		for _, attr := range next.Attr {
			if attr.Name.Space == artifactWordNS && attr.Name.Local == "space" {
				count++
			}
		}
		if originalCount != 0 || count != 1 || documentAttr(next, "space") != "0" {
			continue
		}
		candidate := documentStyleClone(next)
		documentStyleMaskAttrs(candidate, "space")
		if documentTableExactKey(old) == documentTableExactKey(candidate) {
			documentStyleMaskAttrs(next, "space")
		}
	}
}
func documentStyleMaskMargins(n *artifactNode) {
	for _, side := range []string{"top", "right", "bottom", "left"} {
		documentStyleMaskAttrs(documentStyleChild(n, side), "w", "type")
	}
}
func documentStyleClone(n *artifactNode) *artifactNode {
	if n == nil {
		return nil
	}
	out := *n
	out.Children = nil
	for _, c := range n.Children {
		out.Children = append(out.Children, documentStyleClone(c))
	}
	return &out
}
func documentStylePrune(n *artifactNode) {
	if n == nil {
		return
	}
	children := make([]*artifactNode, 0, len(n.Children))
	for _, c := range n.Children {
		documentStylePrune(c)
		if c.Name.Space == artifactWordNS && len(c.Children) == 0 && len(c.Attr) == 0 && strings.TrimSpace(c.Text) == "" && (c.Name.Local == "rPr" || c.Name.Local == "pPr" || c.Name.Local == "tcPr" || c.Name.Local == "tblPr" || c.Name.Local == "tcBorders" || c.Name.Local == "tblBorders" || c.Name.Local == "tcMar" || c.Name.Local == "tblCellMar" || c.Name.Local == "top" || c.Name.Local == "right" || c.Name.Local == "bottom" || c.Name.Local == "left") {
			continue
		}
		children = append(children, c)
	}
	n.Children = children
}
func documentStyleNormalize(root *artifactNode, s documentTableStyleSpec, zeroSpaceSafe bool) string {
	out := documentStyleClone(root)
	tables := documentChildren(out, "tbl")
	t := tables[s.Table-1]
	tp := t.child("tblPr")
	rows := t.children("tr")
	if s.Align != "" {
		documentStyleStrip(tp, "jc")
	}
	if s.MarginCm != nil {
		documentStyleMaskMargins(documentStyleChild(tp, "tblCellMar"))
	}
	if s.Border != nil {
		documentStyleMaskBorders(documentStyleChild(tp, "tblBorders"), zeroSpaceSafe, "top", "bottom", "left", "right")
	}
	for ri, r := range rows {
		cells := r.children("tc")
		for ci, c := range cells {
			cp := c.child("tcPr")
			if s.MarginCm != nil {
				documentStyleMaskMargins(documentStyleChild(cp, "tcMar"))
			}
			if s.Border != nil {
				b := documentStyleChild(cp, "tcBorders")
				if ri == 0 {
					documentStyleMaskBorders(b, zeroSpaceSafe, "top")
				}
				if ri == len(rows)-1 {
					documentStyleMaskBorders(b, zeroSpaceSafe, "bottom")
				}
				if ci == 0 {
					documentStyleMaskBorders(b, zeroSpaceSafe, "left")
				}
				if ci == len(cells)-1 {
					documentStyleMaskBorders(b, zeroSpaceSafe, "right")
				}
			}
			if ri == s.Row-1 && ci == s.Column-1 {
				if s.Fill != "" {
					documentStyleStrip(cp, "shd")
				}
				if s.Color != "" {
					for _, p := range documentChildren(c, "rPr") {
						documentStyleStrip(p, "color")
					}
				}
			}
		}
	}
	documentStylePrune(out)
	return documentTableExactKey(out)
}
func documentStyleMeasure(n *artifactNode, attribute string, want float64, tolerance float64) bool {
	v, err := strconv.ParseFloat(documentAttr(n, attribute), 64)
	return err == nil && math.Abs(v-want) <= tolerance
}
func verifyDocumentTableStyle(beforePath, afterPath string, s documentTableStyleSpec) error {
	before, err := readDocumentTables(beforePath)
	if err != nil {
		return err
	}
	after, err := readDocumentTables(afterPath)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before.Assets, after.Assets) {
		return fmt.Errorf("表格样式操作改变了非目标资源内容或关系绑定")
	}
	if err := verifyDocumentResources(before.StyleResources, after.StyleResources); err != nil {
		return err
	}
	anchor, err := documentTableAnchor(before, s.documentTableSpec)
	if err != nil {
		return err
	}
	if len(before.Tables) != len(after.Tables) {
		return fmt.Errorf("表格数改变")
	}
	bt, at := before.Tables[s.Table-1], after.Tables[s.Table-1]
	if len(bt.Rows) != len(at.Rows) {
		return fmt.Errorf("样式动作改变行数")
	}
	for ri, row := range bt.Rows {
		if len(row) != len(at.Rows[ri]) {
			return fmt.Errorf("样式动作改变列数")
		}
		for ci, c := range row {
			if c.Text != at.Rows[ri][ci].Text {
				return fmt.Errorf("样式动作改变单元格正文")
			}
		}
	}
	br, err := documentStyleRoot(beforePath)
	if err != nil {
		return err
	}
	ar, err := documentStyleRoot(afterPath)
	if err != nil {
		return err
	}
	zeroSpaceSafe := documentStyleZeroSpaceSafe(br, s.Table, before.StyleResources)
	comparisonAfter := documentStyleClone(ar)
	if zeroSpaceSafe {
		documentStyleNormalizeTableBorderDefaults(br, comparisonAfter, s.Table)
	}
	if documentStyleNormalize(br, s, zeroSpaceSafe) != documentStyleNormalize(comparisonAfter, s, zeroSpaceSafe) {
		return fmt.Errorf("表格样式请求之外的结构或属性改变")
	}
	table := documentChildren(ar, "tbl")[s.Table-1]
	tp := table.child("tblPr")
	rows := table.children("tr")
	target := rows[s.Row-1].children("tc")[s.Column-1]
	if s.Align != "" && documentAttr(documentStyleChild(tp, "jc"), "val") != s.Align {
		return fmt.Errorf("表格对齐未持久化")
	}
	if s.Fill != "" {
		shade := documentStyleChild(target.child("tcPr"), "shd")
		if strings.ToUpper(documentAttr(shade, "fill")) != strings.TrimPrefix(s.Fill, "#") || documentAttr(shade, "themeFill") != "" || documentAttr(shade, "val") != "clear" {
			return fmt.Errorf("目标单元格填充未持久化")
		}
	}
	for ri, r := range rows {
		cells := r.children("tc")
		for ci, c := range cells {
			cp := c.child("tcPr")
			if s.MarginCm != nil {
				for _, side := range []string{"top", "right", "bottom", "left"} {
					n := documentStyleChild(documentStyleChild(cp, "tcMar"), side)
					if n == nil {
						n = documentStyleChild(documentStyleChild(tp, "tblCellMar"), side)
					}
					if documentAttr(n, "type") != "dxa" || !documentStyleMeasure(n, "w", *s.MarginCm*1440/2.54, 1) {
						return fmt.Errorf("单元格%d:%d的%s内边距未持久化", ri+1, ci+1, side)
					}
				}
			}
			if s.Border != nil {
				edges := []string{}
				if ri == 0 {
					edges = append(edges, "top")
				}
				if ri == len(rows)-1 {
					edges = append(edges, "bottom")
				}
				if ci == 0 {
					edges = append(edges, "left")
				}
				if ci == len(cells)-1 {
					edges = append(edges, "right")
				}
				for _, side := range edges {
					n := documentStyleChild(documentStyleChild(cp, "tcBorders"), side)
					if n == nil {
						n = documentStyleChild(documentStyleChild(tp, "tblBorders"), side)
					}
					if documentAttr(n, "val") != "single" || strings.ToUpper(documentAttr(n, "color")) != strings.TrimPrefix(s.Border.Color, "#") || documentAttr(n, "themeColor") != "" || !documentStyleMeasure(n, "sz", s.Border.Pt*8, .01) {
						return fmt.Errorf("外边框%d:%d/%s颜色或磅值不符", ri+1, ci+1, side)
					}
				}
			}
		}
	}
	bs, err := readDocumentSnapshot(beforePath)
	if err != nil {
		return err
	}
	as, err := readDocumentSnapshot(afterPath)
	if err != nil {
		return err
	}
	if s.Color != "" {
		anchor.Attrs = map[string]any{"color": s.Color}
	} else {
		anchor.Attrs = nil
	}
	// documentStyleNormalize above guards the full XML outside the requested
	// table properties. The generic structural guard would reject those edits.
	return verifyDocumentCharacterFormat(bs, as, anchor)
}
