package main

import (
	"strconv"
	"strings"
)

// Native source 10621.updateWH derives effectExtent from bounds and half pen
// width; image.calcPen (88337 -> 94311.eP) uses a 3175 EMU no-fill pen and the
// image branch doubles its margin. Limit this equivalence to simple, ungrouped
// rectangles and default right arrows at right angles, without an inherited
// theme or any drawing effects. Wrapper layout is verified independently.
func documentGraphicNoTheme(resources map[string]string) bool {
	for key, value := range resources {
		if strings.HasPrefix(key, "part:word/theme/") || strings.HasPrefix(key, "binding:") && strings.Contains(value, "/theme") {
			return false
		}
	}
	return true
}

func documentGraphicDerivedMargin(d *artifactNode) (int64, bool) {
	if _, err := documentGraphicLayoutWrapper(d); err != nil {
		return 0, false
	}
	for _, name := range []string{"effectLst", "effectDag", "scene3d", "sp3d"} {
		if len(documentGraphicNodes(d, documentDrawingMainNS, name)) != 0 {
			return 0, false
		}
	}
	if _, ok := documentGraphicDefaultGeometry(d); !ok {
		return 0, false
	}
	transforms := documentGraphicNodes(d, documentDrawingMainNS, "xfrm")
	if len(transforms) != 1 {
		return 0, false
	}
	x := transforms[0]
	for _, attr := range x.Attr {
		if attr.Name.Space != "" || attr.Name.Local != "rot" {
			return 0, false
		}
	}
	rotation := x.attr("rot")
	if rotation == "" {
		rotation = "0"
	}
	rot, err := strconv.ParseInt(rotation, 10, 64)
	if err != nil || rot < 0 || rot >= 21600000 || rot%5400000 != 0 {
		return 0, false
	}
	offs := x.children("off")
	if len(offs) != 1 || offs[0].attr("x") != "0" || offs[0].attr("y") != "0" || len(offs[0].Attr) != 2 {
		return 0, false
	}
	for _, ref := range documentGraphicNodes(d, documentDrawingMainNS, "effectRef") {
		if ref.attr("idx") != "0" {
			return 0, false
		}
	}
	pictures := documentGraphicNodes(d, documentPictureNS, "pic")
	if len(pictures) == 1 && len(documentGraphicNodes(d, documentShapeNS, "wsp")) == 0 {
		for _, blip := range documentGraphicNodes(pictures[0], documentDrawingMainNS, "blip") {
			if len(blip.Children) != 0 {
				return 0, false
			}
		}
		props := pictures[0].child("spPr")
		if props == nil {
			return 0, false
		}
		for _, c := range props.Children {
			if c.Name.Space != documentDrawingMainNS || c.Name.Local != "xfrm" && c.Name.Local != "prstGeom" {
				return 0, false
			}
		}
		return 3175, true
	}
	props, err := documentGraphicShapeProperties(d)
	if err != nil {
		return 0, false
	}
	for _, c := range props.Children {
		if c.Name.Space != documentDrawingMainNS || c.Name.Local != "xfrm" && c.Name.Local != "prstGeom" && c.Name.Local != "solidFill" && c.Name.Local != "ln" {
			return 0, false
		}
	}
	for _, shape := range documentGraphicNodes(d, documentShapeNS, "wsp") {
		for _, c := range shape.Children {
			if c.Name.Space != documentShapeNS || c.Name.Local != "spPr" && c.Name.Local != "style" && c.Name.Local != "cNvSpPr" && c.Name.Local != "cNvPr" && c.Name.Local != "bodyPr" {
				return 0, false
			}
		}
	}
	line := documentGraphicChild(props, "ln")
	if line == nil {
		return 0, false
	}
	for _, c := range line.Children {
		if c.Name.Space != documentDrawingMainNS {
			return 0, false
		}
		switch c.Name.Local {
		case "solidFill", "prstDash", "miter":
		case "headEnd", "tailEnd":
			if c.attr("type") != "none" || len(c.Attr) != 1 || len(c.Children) != 0 {
				return 0, false
			}
		default:
			return 0, false
		}
	}
	width, err := strconv.ParseInt(line.attr("w"), 10, 64)
	if err != nil || width <= 0 || width > 304800 {
		return 0, false
	}
	dash := line.children("prstDash")
	if len(dash) > 1 || len(dash) == 1 && (dash[0].attr("val") != "solid" || len(dash[0].Attr) != 1 || len(dash[0].Children) != 0) {
		return 0, false
	}
	if len(dash) == 0 {
		for _, ref := range documentGraphicNodes(d, documentDrawingMainNS, "lnRef") {
			if ref.attr("idx") != "0" {
				return 0, false
			}
		}
	}
	return width / 2, true
}

func documentGraphicExactEffect(d *artifactNode, margin int64) (*artifactNode, bool) {
	nodes := documentGraphicNodes(d, documentDrawingNS, "effectExtent")
	if len(nodes) != 1 {
		return nil, false
	}
	n := nodes[0]
	wrapper, err := documentGraphicLayoutWrapper(d)
	if err != nil {
		return nil, false
	}
	direct := false
	for _, child := range wrapper.Children {
		if child == n {
			direct = true
		}
	}
	if !direct {
		return nil, false
	}
	if len(n.Attr) != 4 || len(n.Children) != 0 || strings.TrimSpace(n.Text) != "" {
		return nil, false
	}
	seen := map[string]bool{}
	for _, attr := range n.Attr {
		if attr.Name.Space != "" || seen[attr.Name.Local] {
			return nil, false
		}
		seen[attr.Name.Local] = true
		expected := margin
		value, err := strconv.ParseInt(attr.Value, 10, 64)
		if err != nil || value < 0 {
			return nil, false
		}
		switch attr.Name.Local {
		case "l", "t":
			if value != expected {
				return nil, false
			}
		case "r", "b":
			if value != expected && value != expected-1 {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return n, true
}
func documentGraphicDropNode(root, target *artifactNode) {
	out := make([]*artifactNode, 0, len(root.Children))
	for _, child := range root.Children {
		if child != target {
			documentGraphicDropNode(child, target)
			out = append(out, child)
		}
	}
	root.Children = out
}

// Preserve arbitrary existing effects. Only a newly materialized or recomputed
// native margin may differ; removal of an existing effect is never normalized.
func normalizeDocumentGraphicDerived(before, after *artifactNode, s documentGraphicSpec, resources map[string]string) {
	if !documentGraphicNoTheme(resources) || s.WidthCm == nil && s.RotationDegrees == nil && s.Fill == "" && s.Line == nil && s.Preset == 0 {
		return
	}
	bd, _, be := documentGraphicTarget(before, s)
	ad, _, ae := documentGraphicTarget(after, s)
	if be != nil || ae != nil {
		return
	}
	bm, bok := documentGraphicDerivedMargin(bd)
	am, aok := documentGraphicDerivedMargin(ad)
	if !bok || !aok {
		return
	}
	if s.Preset == 0 && s.Fill == "" && s.Line == nil {
		normalizeDocumentGraphicEmptyStyle(bd, ad)
	}
	bg, _ := documentGraphicDefaultGeometry(bd)
	ag, _ := documentGraphicDefaultGeometry(ad)
	if bg.attr("prst") == "rightArrow" && ag.attr("prst") == "rightArrow" && len(bg.Children[0].Children) == 0 && len(ag.Children[0].Children) == 2 {
		// Native geometry source initializes rightArrow adj1/adj2 to 50000;
		// serializing these defaults does not change the arrow's contour.
		ag.Children[0].Children = nil
	}
	bEffects := documentGraphicNodes(bd, documentDrawingNS, "effectExtent")
	aEffect, aExact := documentGraphicExactEffect(ad, am)
	if aExact {
		if len(bEffects) == 0 {
			documentGraphicDropNode(ad, aEffect)
		} else if bEffect, bExact := documentGraphicExactEffect(bd, bm); bExact {
			documentGraphicDropNode(bd, bEffect)
			documentGraphicDropNode(ad, aEffect)
		}
	}
	if s.Line == nil && s.Preset == 0 {
		return
	}
	bp, be := documentGraphicShapeProperties(bd)
	ap, ae := documentGraphicShapeProperties(ad)
	if be != nil || ae != nil {
		return
	}
	bl, al := documentGraphicChild(bp, "ln"), documentGraphicChild(ap, "ln")
	if bl == nil || al == nil {
		return
	}
	// An omitted preset dash is solid in DrawingML; preserve all custom dashes.
	if len(bl.children("prstDash")) == 0 && len(al.children("prstDash")) == 1 {
		documentGraphicDropNode(al, al.children("prstDash")[0])
	}
	// 94311 native built-in line style uses cap=0(flat), cmpd=1(single),
	// algn=0(center). These closed solid shapes have no line-cap endpoints.
	for name, value := range map[string]string{"cap": "flat", "cmpd": "sng", "algn": "ctr"} {
		if bl.attr(name) == "" && al.attr(name) == value {
			documentGraphicMask(al, name)
		}
	}
	for _, name := range []string{"headEnd", "tailEnd"} {
		bNodes, aNodes := bl.children(name), al.children(name)
		if len(bNodes) != 0 || len(aNodes) != 1 {
			continue
		}
		node := aNodes[0]
		if len(node.Attr) == 1 && node.Attr[0].Name.Space == "" && node.Attr[0].Name.Local == "type" && node.Attr[0].Value == "none" && len(node.Children) == 0 && strings.TrimSpace(node.Text) == "" {
			documentGraphicDropNode(al, node)
		}
	}
}

// Only the source-backed geometry defaults are equivalent. Custom adjustments,
// unknown guide attributes, formulas and other presets remain exact.
func documentGraphicDefaultGeometry(d *artifactNode) (*artifactNode, bool) {
	geometries := documentGraphicNodes(d, documentDrawingMainNS, "prstGeom")
	if len(geometries) != 1 {
		return nil, false
	}
	g := geometries[0]
	if len(g.Attr) != 1 || g.Attr[0].Name.Space != "" || g.Attr[0].Name.Local != "prst" || strings.TrimSpace(g.Text) != "" || len(g.Children) != 1 {
		return nil, false
	}
	list := g.Children[0]
	if list.Name.Space != documentDrawingMainNS || list.Name.Local != "avLst" || len(list.Attr) != 0 || strings.TrimSpace(list.Text) != "" {
		return nil, false
	}
	switch g.attr("prst") {
	case "rect":
		return g, len(list.Children) == 0
	case "rightArrow":
		if len(list.Children) == 0 {
			return g, true
		}
		if len(list.Children) != 2 {
			return nil, false
		}
		for i, n := range list.Children {
			if n.Name.Space != documentDrawingMainNS || n.Name.Local != "gd" || len(n.Attr) != 2 || len(n.Children) != 0 || strings.TrimSpace(n.Text) != "" || n.attr("name") != []string{"adj1", "adj2"}[i] || n.attr("fmla") != "val 50000" {
				return nil, false
			}
			for _, a := range n.Attr {
				if a.Name.Space != "" || a.Name.Local != "name" && a.Name.Local != "fmla" {
					return nil, false
				}
			}
		}
		return g, true
	default:
		return nil, false
	}
}
