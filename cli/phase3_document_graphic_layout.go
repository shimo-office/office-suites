package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Layout uses the target's existing paragraph; anchor selects the vertical
// reference origin (zero offset), not another paragraph or a locked anchor.
func prepareDocumentGraphicLayout(p map[string]any, s *documentGraphicSpec) error {
	for key, dst := range map[string]*string{"wrap": &s.Wrap, "anchor": &s.Anchor, "horizontalAlign": &s.HorizontalAlign, "horizontalRelativeTo": &s.HorizontalRelativeTo} {
		if raw, exists := p[key]; exists {
			v, ok := raw.(string)
			if !ok || v == "" {
				return fmt.Errorf("%s 必须为非空字符串", key)
			}
			*dst = v
		}
	}
	if s.Wrap != "" && s.Wrap != "square" && s.Wrap != "topAndBottom" && s.Wrap != "behind" && s.Wrap != "inFront" {
		return fmt.Errorf("wrap 支持 square/topAndBottom/behind/inFront")
	}
	if s.Anchor != "" && s.Anchor != "paragraph" && s.Anchor != "page" {
		return fmt.Errorf("anchor 支持 paragraph/page，表示现有承载段落的垂直参照零点")
	}
	if s.HorizontalAlign != "" && s.HorizontalAlign != "center" {
		return fmt.Errorf("horizontalAlign 仅支持 center")
	}
	if (s.HorizontalAlign == "") != (s.HorizontalRelativeTo == "") {
		return fmt.Errorf("horizontalAlign 和 horizontalRelativeTo 必须一起指定")
	}
	if s.HorizontalRelativeTo != "" && s.HorizontalRelativeTo != "page" && s.HorizontalRelativeTo != "margin" {
		return fmt.Errorf("horizontalRelativeTo 支持 page/margin")
	}
	return nil
}
func documentGraphicHasLayout(s documentGraphicSpec) bool {
	return s.Wrap != "" || s.Anchor != "" || s.HorizontalAlign != ""
}
func documentGraphicLayoutPreflightJS(s documentGraphicSpec) string {
	if !documentGraphicHasLayout(s) {
		return ""
	}
	raw, _ := json.Marshal(map[string]string{"wrap": s.Wrap, "anchor": s.Anchor, "align": s.HorizontalAlign, "relative": s.HorizontalRelativeTo})
	return `const graphicLayout=` + string(raw) + `;if(typeof drawing.isThisInline!=='function'||typeof drawing.getProps!=='function')throw Error('图形布局读取方法未就绪');const graphicWasInline=drawing.isThisInline();const graphicLayoutBefore=drawing.getProps();if(graphicWasInline){if(typeof drawing.getDistance!=='function')throw Error('嵌入图形原始距离不可读');const distance=drawing.getDistance();if(!distance||!['L','R','T','B'].every(k=>Number.isFinite(distance[k])&&distance[k]===0))throw Error('嵌入图形具有非默认距离，拒绝转换');}if(graphicLayoutBefore.canFlow===false)throw Error('此位置不支持浮动图形布局');if(graphicWasInline&&(!graphicLayout.wrap||!graphicLayout.anchor||!graphicLayout.align))throw Error('嵌入图形转浮动必须明确wrap、anchor及水平对齐参照');if(graphicLayout.wrap)props.wrappingType=({square:1,topAndBottom:4,behind:5,inFront:6})[graphicLayout.wrap];if(graphicLayout.anchor)props.posV={relativeFrom:graphicLayout.anchor==='paragraph'?6:5,UseAlign:false,Value:0,Percent:false};if(graphicLayout.align&&typeof api.setShapeAlign!=='function')throw Error('图形水平对齐方法未就绪');`
}
func documentGraphicLayoutMutationJS(s documentGraphicSpec) string {
	if s.HorizontalAlign == "" {
		return ""
	}
	target := 3
	if s.HorizontalRelativeTo == "page" {
		target = 2
	}
	return fmt.Sprintf("api.setShapeAlign(4,%d);", target)
}
func documentGraphicLayoutWrapper(drawing *artifactNode) (*artifactNode, error) {
	var found *artifactNode
	for _, c := range drawing.Children {
		if c.Name.Space == documentDrawingNS && (c.Name.Local == "inline" || c.Name.Local == "anchor") {
			if found != nil {
				return nil, fmt.Errorf("图形布局容器不唯一")
			}
			found = c
		}
	}
	if found == nil {
		return nil, fmt.Errorf("图形布局容器不存在")
	}
	return found, nil
}
func graphicLayoutPosition(w *artifactNode, name, relative, valueName, value string) error {
	nodes := documentGraphicNodes(w, documentDrawingNS, name)
	if len(nodes) != 1 || nodes[0].attr("relativeFrom") != relative || len(nodes[0].Attr) != 1 || len(nodes[0].Children) != 1 {
		return fmt.Errorf("图形%s参照未持久化", name)
	}
	c := nodes[0].Children[0]
	if c.Name.Space != documentDrawingNS || c.Name.Local != valueName || strings.TrimSpace(c.Text) != value || len(c.Attr) != 0 || len(c.Children) != 0 {
		return fmt.Errorf("图形%s目标位置未持久化", name)
	}
	return nil
}
func graphicLayoutWrap(w *artifactNode, wrap string) error {
	names := map[string]string{"square": "wrapSquare", "topAndBottom": "wrapTopAndBottom", "behind": "wrapNone", "inFront": "wrapNone"}
	count := 0
	for _, c := range w.Children {
		if c.Name.Space == documentDrawingNS && strings.HasPrefix(c.Name.Local, "wrap") {
			count++
			if c.Name.Local != names[wrap] || len(c.Children) > 0 {
				return fmt.Errorf("环绕类型不符")
			}
			if wrap == "square" {
				if c.attr("wrapText") != "bothSides" || len(c.Attr) != 1 {
					return fmt.Errorf("四周环绕未覆盖两侧")
				}
			} else if len(c.Attr) != 0 {
				return fmt.Errorf("环绕包含未请求属性")
			}
		}
	}
	if count != 1 {
		return fmt.Errorf("环绕节点不唯一")
	}
	behind := "0"
	if wrap == "behind" {
		behind = "1"
	}
	if w.attr("behindDoc") != behind {
		return fmt.Errorf("图形前后层级与环绕请求不符")
	}
	return nil
}

// Compare layout metadata independently before the outer full-document proof
// removes it. This protects unrequested positions, distance and overlap flags.
func graphicLayoutMetadata(w *artifactNode, s documentGraphicSpec) string {
	c := documentStyleClone(w)
	c.Children = nil
	for _, n := range w.Children {
		if n.Name.Space != documentDrawingNS {
			continue
		}
		name := n.Name.Local
		if name == "extent" || name == "effectExtent" || name == "docPr" || name == "cNvGraphicFramePr" {
			continue
		}
		if (s.Wrap != "" && strings.HasPrefix(name, "wrap")) || (s.Anchor != "" && name == "positionV") || (s.HorizontalAlign != "" && name == "positionH") {
			continue
		}
		c.Children = append(c.Children, n)
	}
	if s.Wrap != "" {
		documentGraphicMask(c, "behindDoc")
	}
	return layoutNodeCanonical(c)
}
func graphicLayoutConversionDefaults(w *artifactNode) error {
	expected := map[string]string{"distT": "0", "distB": "0", "distL": "115200", "distR": "115200", "simplePos": "0", "locked": "0", "layoutInCell": "1", "allowOverlap": "1"}
	for k, v := range expected {
		if w.attr(k) != v {
			return fmt.Errorf("嵌入转浮动的原生缺省%s异常", k)
		}
	}
	for _, a := range w.Attr {
		if a.Name.Space != "" {
			return fmt.Errorf("新增未验证布局属性")
		}
		if _, ok := expected[a.Name.Local]; !ok && a.Name.Local != "relativeHeight" && a.Name.Local != "behindDoc" {
			return fmt.Errorf("新增未验证布局属性%s", a.Name.Local)
		}
	}
	z, e := strconv.ParseUint(w.attr("relativeHeight"), 10, 32)
	if e != nil || z > 4294967295 {
		return fmt.Errorf("图形层级无效")
	}
	simple := documentGraphicNodes(w, documentDrawingNS, "simplePos")
	if len(simple) != 1 || simple[0].attr("x") != "0" || simple[0].attr("y") != "0" || len(simple[0].Attr) != 2 || len(simple[0].Children) != 0 {
		return fmt.Errorf("浮动绝对位置缺省异常")
	}
	for _, c := range w.Children {
		if c.Name.Space == documentDrawingNS {
			switch c.Name.Local {
			case "simplePos", "positionH", "positionV", "extent", "effectExtent", "docPr", "cNvGraphicFramePr", "wrapSquare", "wrapTopAndBottom", "wrapNone":
			default:
				return fmt.Errorf("新增未验证布局节点%s", c.Name.Local)
			}
		}
	}
	return nil
}

// The independent export rejects unsupported conversion inputs before Chrome
// opens. The same check is repeated by the final proof and native preflight.
func documentGraphicLayoutBaseline(drawing *artifactNode, s documentGraphicSpec) error {
	if !documentGraphicHasLayout(s) {
		return nil
	}
	w, e := documentGraphicLayoutWrapper(drawing)
	if e != nil {
		return e
	}
	if w.Name.Local != "inline" {
		return nil
	}
	if s.Wrap == "" || s.Anchor == "" || s.HorizontalAlign == "" {
		return fmt.Errorf("嵌入转浮动缺少明确位置请求")
	}
	for _, attr := range w.Attr {
		if attr.Name.Space != "" || (attr.Name.Local != "distT" && attr.Name.Local != "distB" && attr.Name.Local != "distL" && attr.Name.Local != "distR") {
			return fmt.Errorf("嵌入对象带有未验证布局属性，拒绝转换")
		}
		value, e := strconv.ParseUint(attr.Value, 10, 64)
		if e != nil || value != 0 {
			return fmt.Errorf("嵌入对象%s不是已验证的零默认距离，拒绝转换", attr.Name.Local)
		}
	}
	return nil
}
func verifyDocumentGraphicLayout(beforeDrawing, afterDrawing *artifactNode, s documentGraphicSpec) error {
	if !documentGraphicHasLayout(s) {
		return nil
	}
	if e := documentGraphicLayoutBaseline(beforeDrawing, s); e != nil {
		return e
	}
	before, e := documentGraphicLayoutWrapper(beforeDrawing)
	if e != nil {
		return e
	}
	after, e := documentGraphicLayoutWrapper(afterDrawing)
	if e != nil {
		return e
	}
	if after.Name.Local != "anchor" {
		return fmt.Errorf("请求的浮动布局未持久化")
	}
	if s.Wrap != "" {
		if e = graphicLayoutWrap(after, s.Wrap); e != nil {
			return e
		}
	}
	if s.Anchor != "" {
		if e = graphicLayoutPosition(after, "positionV", s.Anchor, "posOffset", "0"); e != nil {
			return e
		}
	}
	if s.HorizontalAlign != "" {
		if e = graphicLayoutPosition(after, "positionH", s.HorizontalRelativeTo, "align", "center"); e != nil {
			return e
		}
	}
	if before.Name.Local == "inline" {
		return graphicLayoutConversionDefaults(after)
	}
	if graphicLayoutMetadata(before, s) != graphicLayoutMetadata(after, s) {
		return fmt.Errorf("图形动作改变未请求的布局属性")
	}
	return nil
}
func normalizeDocumentGraphicLayout(drawing *artifactNode, s documentGraphicSpec) error {
	if !documentGraphicHasLayout(s) {
		return nil
	}
	w, e := documentGraphicLayoutWrapper(drawing)
	if e != nil {
		return e
	}
	w.Name.Local = "verifiedLayout"
	w.Attr = nil
	children := make([]*artifactNode, 0, len(w.Children))
	for _, c := range w.Children {
		if c.Name.Space == documentDrawingNS && (c.Name.Local == "simplePos" || c.Name.Local == "positionH" || c.Name.Local == "positionV" || strings.HasPrefix(c.Name.Local, "wrap")) {
			continue
		}
		children = append(children, c)
	}
	w.Children = children
	return nil
}
