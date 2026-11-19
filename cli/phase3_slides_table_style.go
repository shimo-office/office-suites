package main

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

func preparePhase3SlideTableStyle(p map[string]any, s *phase3SlideTableSpec) error {
	for k, v := range p {
		switch k {
		case "slideId", "tableName", "operation":
		case "headerFillColor", "borderColor":
			checked, e := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": "cell", "attrs": map[string]any{"color": v}})
			if e != nil {
				return e
			}
			if k == "headerFillColor" {
				s.HeaderFillColor = checked.Attrs["color"].(string)
			} else {
				s.BorderColor = checked.Attrs["color"].(string)
			}
		case "borderWidth":
			n, ok := v.(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0.25 || n > 20 || n*12700 != math.Trunc(n*12700) {
				return fmt.Errorf("borderWidth 为 0.25–20 磅")
			}
			s.BorderWidth = n
		default:
			return fmt.Errorf("表头/边框样式不支持参数 %s", k)
		}
	}
	if s.HeaderFillColor == "" && s.BorderColor == "" {
		return fmt.Errorf("至少指定表头底色或全部边框色")
	}
	if s.BorderColor != "" && s.BorderWidth == 0 {
		s.BorderWidth = 1
	}
	if s.BorderColor == "" && s.BorderWidth != 0 {
		return fmt.Errorf("边框宽度必须与边框颜色一起设置")
	}
	return nil
}
func phase3MaskTableCellStyle(n *artifactNode, s phase3SlideTableSpec, header bool) *artifactNode {
	out := *n
	out.Children = nil
	for _, c := range n.Children {
		if n.Name.Space == artifactDrawingNS && n.Name.Local == "tcPr" && c.Name.Space == artifactDrawingNS {
			if header && s.HeaderFillColor != "" && (c.Name.Local == "solidFill" || c.Name.Local == "noFill" || c.Name.Local == "gradFill" || c.Name.Local == "pattFill") {
				continue
			}
			if s.BorderColor != "" && (c.Name.Local == "lnL" || c.Name.Local == "lnR" || c.Name.Local == "lnT" || c.Name.Local == "lnB") {
				line := phase3TableBorderRemainder(c)
				if len(line.Attr) > 0 || len(line.Children) > 0 || strings.TrimSpace(line.Text) != "" {
					out.Children = append(out.Children, line)
				}
				continue
			}
		}
		out.Children = append(out.Children, phase3MaskTableCellStyle(c, s, header))
	}
	return &out
}
func verifyPhase3SlideTableStyle(b, a phase3SlideTableGrid, s phase3SlideTableSpec) error {
	if len(a.Rows) != len(b.Rows) || !reflect.DeepEqual(a.Widths, b.Widths) || !reflect.DeepEqual(a.Heights, b.Heights) {
		return fmt.Errorf("设置样式改变表格尺寸")
	}
	for r, row := range a.Rows {
		if len(row) != len(b.Rows[r]) {
			return fmt.Errorf("设置样式改变单元格数")
		}
		for col, c := range row {
			pr := phase3SlideChild(c, artifactDrawingNS, "tcPr")
			if pr == nil {
				return fmt.Errorf("缺少单元格显式样式")
			}
			if r == 0 && s.HeaderFillColor != "" {
				if e := phase3ShapeColor(pr, s.HeaderFillColor); e != nil {
					return e
				}
			}
			if s.BorderColor != "" {
				for _, tag := range []string{"lnL", "lnR", "lnT", "lnB"} {
					ln := phase3SlideChild(pr, artifactDrawingNS, tag)
					if ln == nil {
						return fmt.Errorf("缺少单元格边框 %s", tag)
					}
					w, e := phase3ShapeInteger(ln, "w")
					exact := int64(s.BorderWidth * 12700)
					// Native ET converts pt to mm; md truncates mm * 36000.
					// Accept that exact conversion only, bounded to one EMU.
					native := int64((25.4 * s.BorderWidth / 72) * 36000)
					if e != nil || (w != exact && (w != native || native < exact-1 || native > exact+1)) {
						return fmt.Errorf("边框宽度不符")
					}
					if e := phase3ShapeColor(ln, s.BorderColor); e != nil {
						return e
					}
				}
			}
			if !phase3EqualTableCell(phase3MaskTableCellStyle(c, s, r == 0), phase3MaskTableCellStyle(b.Rows[r][col], s, r == 0)) {
				return fmt.Errorf("样式操作改动正文或非目标格式")
			}
		}
	}
	return nil
}

func phase3TableBorderRemainder(n *artifactNode) *artifactNode {
	line := *n
	line.Attr = nil
	line.Children = nil
	for _, a := range n.Attr {
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
			continue
		}
		if a.Name.Space == "" && (a.Name.Local == "w" || (a.Name.Local == "cap" && a.Value == "flat") || (a.Name.Local == "cmpd" && a.Value == "sng") || (a.Name.Local == "algn" && a.Value == "ctr")) {
			continue
		}
		line.Attr = append(line.Attr, a)
	}
	for _, child := range n.Children {
		if child.Name.Space == artifactDrawingNS {
			if child.Name.Local == "solidFill" || child.Name.Local == "noFill" || child.Name.Local == "gradFill" || child.Name.Local == "pattFill" {
				continue
			}
			if child.Name.Local == "prstDash" && child.attr("val") == "solid" && len(child.Children) == 0 && len(child.Attr) == 1 {
				continue
			}
			if (child.Name.Local == "headEnd" || child.Name.Local == "tailEnd") && child.attr("type") == "none" {
				defaults := len(child.Children) == 0
				for _, a := range child.Attr {
					defaults = defaults && a.Name.Space == "" && ((a.Name.Local == "type" && a.Value == "none") || ((a.Name.Local == "w" || a.Name.Local == "len") && a.Value == "med"))
				}
				if defaults {
					continue
				}
			}
		}
		line.Children = append(line.Children, child)
	}
	return &line
}
func phase3CheckTableBorderBaseline(grid phase3SlideTableGrid, s phase3SlideTableSpec) error {
	if s.Operation != "style" || s.BorderColor == "" {
		return nil
	}
	for _, row := range grid.Rows {
		for _, cell := range row {
			props := phase3SlideChild(cell, artifactDrawingNS, "tcPr")
			for _, tag := range []string{"lnL", "lnR", "lnT", "lnB"} {
				line := phase3SlideChild(props, artifactDrawingNS, tag)
				if line == nil {
					continue
				}
				rest := phase3TableBorderRemainder(line)
				if len(rest.Attr) > 0 || len(rest.Children) > 0 || strings.TrimSpace(rest.Text) != "" {
					return fmt.Errorf("原生边框设置可能重置既有线型/端点，当前拒绝修改带非默认线型的表格")
				}
			}
		}
	}
	return nil
}
