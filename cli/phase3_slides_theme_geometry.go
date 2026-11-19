package main

import (
	"fmt"
	"reflect"
	"strconv"
)

type phase3ThemeTransform struct {
	X, Y, CX, CY int64
	FlipH, FlipV bool
}

func phase3ThemePlaceholderKey(kind, index string) string {
	if kind == "" {
		kind = "obj"
	}
	if index == "" {
		index = "0"
	}
	return kind + ":" + index
}
func phase3ThemeSourceGeometry(ops any) (map[string]phase3ThemeTransform, error) {
	out := map[string]phase3ThemeTransform{}
	var visit func(any) error
	visit = func(v any) error {
		switch x := v.(type) {
		case []any:
			if len(x) > 0 {
				first, _ := x[0].(map[string]any)
				attrs, _ := first["attributes"].(map[string]any)
				if attrs["spType"] == "sp" {
					data, _ := first["data"].(map[string]any)
					nv, e := phase3ThemeOpObject(data["delta"], "nvSpPr")
					if e != nil {
						return e
					}
					props, _ := nv["nvSpPr"].(map[string]any)
					pr, _ := props["nvPr"].(map[string]any)
					ph, ok := pr["ph"].(map[string]any)
					if ok {
						sp, e := phase3ThemeOpObject(data["delta"], "spPr")
						if e != nil {
							return e
						}
						shape, _ := sp["spPr"].(map[string]any)
						xf, ok := shape["xfrm"].(map[string]any)
						if ok {
							t := phase3ThemeTransform{}
							for key, ptr := range map[string]*int64{"offX": &t.X, "offY": &t.Y, "extX": &t.CX, "extY": &t.CY} {
								value, ok := xf[key].(float64)
								if !ok || value != float64(int64(value)) {
									return fmt.Errorf("官方占位符几何无效")
								}
								*ptr = int64(value)
							}
							if rot, ok := xf["rot"].(float64); ok && rot != 0 {
								return fmt.Errorf("当前精确主题几何验证不支持旋转占位符")
							}
							t.FlipH, _ = xf["flipH"].(bool)
							t.FlipV, _ = xf["flipV"].(bool)
							key := phase3ThemePlaceholderKey(str(ph, "type"), str(ph, "idx"))
							if _, exists := out[key]; exists {
								return fmt.Errorf("官方占位符身份重复")
							}
							out[key] = t
						}
					}
				}
			}
			for _, n := range x {
				if e := visit(n); e != nil {
					return e
				}
			}
		case map[string]any:
			for _, n := range x {
				if e := visit(n); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e := visit(ops); e != nil {
		return nil, e
	}
	return out, nil
}
func phase3ThemeXMLTransform(n *artifactNode) (*phase3ThemeTransform, error) {
	if n == nil {
		return nil, nil
	}
	if n.attr("rot") != "" && n.attr("rot") != "0" {
		return nil, fmt.Errorf("占位符旋转不匹配官方模板")
	}
	t := phase3ThemeTransform{}
	for _, a := range n.Attr {
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
			continue
		}
		if a.Name.Space != "" || (a.Name.Local != "rot" && a.Name.Local != "flipH" && a.Name.Local != "flipV") {
			return nil, fmt.Errorf("占位符变换包含未知属性")
		}
	}
	for _, c := range n.Children {
		if c.Name.Space != artifactDrawingNS || (c.Name.Local != "off" && c.Name.Local != "ext") {
			return nil, fmt.Errorf("占位符变换包含非预期属性")
		}
	}
	for key, ptr := range map[string]*int64{"x": &t.X, "y": &t.Y, "cx": &t.CX, "cy": &t.CY} {
		tag := "off"
		if key == "cx" || key == "cy" {
			tag = "ext"
		}
		node := phase3SlideChild(n, artifactDrawingNS, tag)
		if node == nil {
			return nil, fmt.Errorf("占位符位置/尺寸不完整")
		}
		value, e := strconv.ParseInt(node.attr(key), 10, 64)
		if e != nil {
			return nil, e
		}
		*ptr = value
	}
	for key, ptr := range map[string]*bool{"flipH": &t.FlipH, "flipV": &t.FlipV} {
		value := n.attr(key)
		if value != "" && value != "0" && value != "1" && value != "true" && value != "false" {
			return nil, fmt.Errorf("占位符翻转标记无效")
		}
		*ptr = value == "1" || value == "true"
	}
	return &t, nil
}
func phase3ThemeXMLGeometry(root *artifactNode) (map[string]*phase3ThemeTransform, error) {
	tree, e := phase3ShapeTree(root)
	if e != nil {
		return nil, e
	}
	out := map[string]*phase3ThemeTransform{}
	for _, n := range tree.Children {
		if n.Name.Space != artifactSlideNS || n.Name.Local != "sp" {
			continue
		}
		ph := phase3SlideChild(phase3SlideChild(phase3SlideChild(n, artifactSlideNS, "nvSpPr"), artifactSlideNS, "nvPr"), artifactSlideNS, "ph")
		if ph == nil {
			continue
		}
		key := phase3ThemePlaceholderKey(ph.attr("type"), ph.attr("idx"))
		if _, ok := out[key]; ok {
			return nil, fmt.Errorf("本页占位符类型/索引重复")
		}
		transform, e := phase3ThemeXMLTransform(phase3SlideChild(phase3SlideChild(n, artifactSlideNS, "spPr"), artifactDrawingNS, "xfrm"))
		if e != nil {
			return nil, e
		}
		out[key] = transform
	}
	return out, nil
}
func phase3ThemeAllowedGeometry(before, after map[string]*phase3ThemeTransform, expected map[string]phase3ThemeTransform) (map[string]bool, error) {
	masks := map[string]bool{}
	if len(before) != len(after) {
		return nil, fmt.Errorf("主题改变了占位符数量")
	}
	for key, b := range before {
		a, exists := after[key]
		if !exists {
			return nil, fmt.Errorf("主题改变了占位符身份")
		}
		if reflect.DeepEqual(a, b) {
			continue
		}
		want, exists := expected[key]
		if !exists || (a != nil && !reflect.DeepEqual(*a, want)) {
			return nil, fmt.Errorf("占位符%s的新位置不匹配对应官方版式/母版", key)
		}
		masks[key] = true
	}
	return masks, nil
}
func phase3MaskThemeGeometry(n *artifactNode, masks map[string]bool) *artifactNode {
	if n == nil {
		return nil
	}
	copy := *n
	copy.Children = nil
	ph := phase3SlideChild(phase3SlideChild(phase3SlideChild(n, artifactSlideNS, "nvSpPr"), artifactSlideNS, "nvPr"), artifactSlideNS, "ph")
	allowed := ph != nil && masks[phase3ThemePlaceholderKey(ph.attr("type"), ph.attr("idx"))]
	for _, child := range n.Children {
		if allowed && child.Name.Space == artifactSlideNS && child.Name.Local == "spPr" {
			props := *child
			props.Children = nil
			for _, p := range child.Children {
				if p.Name.Space != artifactDrawingNS || p.Name.Local != "xfrm" {
					props.Children = append(props.Children, p)
				}
			}
			if len(props.Children) > 0 || len(props.Attr) > 0 {
				copy.Children = append(copy.Children, &props)
			}
			continue
		}
		copy.Children = append(copy.Children, phase3MaskThemeGeometry(child, masks))
	}
	return &copy
}
