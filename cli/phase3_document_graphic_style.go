package main

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

const documentShapeNS = "http://schemas.microsoft.com/office/word/2010/wordprocessingShape"

var documentGraphicColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type documentGraphicLineSpec struct {
	Color   string  `json:"color"`
	WidthPt float64 `json:"widthPt"`
}

func documentGraphicStyleInput(p map[string]any, s *documentGraphicSpec) error {
	if v, exists := p["fill"]; exists {
		color, ok := v.(string)
		if !ok || !documentGraphicColor.MatchString(color) {
			return fmt.Errorf("fill 必须为 #RRGGBB")
		}
		s.Fill = strings.ToUpper(color)
	}
	if v, exists := p["line"]; exists {
		line, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("line 必须包含color/widthPt")
		}
		for k := range line {
			if k != "color" && k != "widthPt" {
				return fmt.Errorf("line 未知参数 %s", k)
			}
		}
		color, ok := line["color"].(string)
		width, number := line["widthPt"].(float64)
		if !ok || !documentGraphicColor.MatchString(color) || !number || math.IsNaN(width) || math.IsInf(width, 0) || width < 0.25 || width > 24 || math.Trunc(width*8) != width*8 {
			return fmt.Errorf("line需要#RRGGBB颜色、0.25–24磅且1/8磅精度宽度")
		}
		s.Line = &documentGraphicLineSpec{strings.ToUpper(color), width}
	}
	return nil
}
func documentGraphicShapeProperties(drawing *artifactNode) (*artifactNode, error) {
	shapes := documentGraphicNodes(drawing, documentShapeNS, "wsp")
	if len(shapes) != 1 {
		return nil, fmt.Errorf("填充/线条目标必须为真实原生形状")
	}
	props := shapes[0].child("spPr")
	if props == nil {
		return nil, fmt.Errorf("原生形状缺少spPr")
	}
	lines := 0
	for _, child := range props.Children {
		if child.Name.Space == documentDrawingMainNS && child.Name.Local == "ln" {
			lines++
		}
	}
	if lines > 1 {
		return nil, fmt.Errorf("原生形状线条节点不唯一")
	}
	if documentGraphicFillCount(props) > 1 || documentGraphicFillCount(documentGraphicChild(props, "ln")) > 1 {
		return nil, fmt.Errorf("原生形状填充表示冲突")
	}
	return props, nil
}
func documentGraphicChild(n *artifactNode, name string) *artifactNode {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Name.Space == documentDrawingMainNS && c.Name.Local == name {
			return c
		}
	}
	return nil
}
func documentGraphicStripFill(n *artifactNode) {
	if n == nil {
		return
	}
	out := []*artifactNode{}
	for _, c := range n.Children {
		if c.Name.Space == documentDrawingMainNS {
			switch c.Name.Local {
			case "solidFill", "noFill", "gradFill", "blipFill", "pattFill", "grpFill":
				continue
			}
		}
		out = append(out, c)
	}
	n.Children = out
}
func documentGraphicFillCount(n *artifactNode) int {
	if n == nil {
		return 0
	}
	fills := 0
	for _, c := range n.Children {
		if c.Name.Space == documentDrawingMainNS {
			switch c.Name.Local {
			case "solidFill", "noFill", "gradFill", "blipFill", "pattFill", "grpFill":
				fills++
			}
		}
	}
	return fills
}
func documentGraphicOpaqueColor(n *artifactNode, want string) bool {
	if n == nil {
		return false
	}
	fills := documentGraphicFillCount(n)
	solid := documentGraphicChild(n, "solidFill")
	if fills != 1 || solid == nil || len(solid.Children) != 1 {
		return false
	}
	color := solid.Children[0]
	if color.Name.Space != documentDrawingMainNS || color.Name.Local != "srgbClr" || strings.ToUpper(color.attr("val")) != strings.TrimPrefix(want, "#") {
		return false
	}
	for _, c := range color.Children {
		if c.Name.Space != documentDrawingMainNS || c.Name.Local != "alpha" || c.attr("val") != "100000" {
			return false
		}
	}
	return true
}
