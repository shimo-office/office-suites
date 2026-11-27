package main

import (
	"archive/zip"
	"strconv"
)

// A shape edit can materialize inherited geometry and empty paragraph nodes.
// Resolve the actual layout/master before removing a rectangle, and normalize
// only the requested shape. Unrelated objects and non-default geometry remain
// byte-semantic parts of the preservation check.
func phase3SlideEditedDefaultsFilter(filename string, ids []string, s phase3SlidesSpec, inner func(int, *artifactNode) (*artifactNode, error)) (func(int, *artifactNode) (*artifactNode, error), error) {
	z, entries, err := openArtifact(filename)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	rects := map[string]bool{}
	lines := map[string]*artifactNode{}
	root, err := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if err != nil {
		return nil, err
	}
	rels, err := artifactRelations(entries, "ppt/_rels/presentation.xml.rels", "ppt")
	if err != nil {
		return nil, err
	}
	list := root.child("sldIdLst")
	if list != nil {
		for i, item := range list.children("sldId") {
			if i >= len(ids) || ids[i] != s.SlideID {
				continue
			}
			part := rels[relationID(item)].Target
			slide, e := artifactPart(entries, part, artifactSlideNS, "sld")
			if e != nil {
				return nil, e
			}
			tree, e := phase3ShapeTree(slide)
			if e != nil {
				return nil, e
			}
			for _, shape := range tree.Children {
				if phase3SlideEditedShape(shape, s) {
					if phase3InheritedRectangle(entries, part, shape) {
						rects[phase3ShapeName(shape)] = true
					}
					if s.Shape != nil && s.Shape.LineColor != "" {
						lines[phase3ShapeName(shape)] = phase3InheritedThemeLine(entries, part, shape)
					}
				}
			}
		}
	}
	return func(i int, n *artifactNode) (*artifactNode, error) {
		if i < len(ids) && ids[i] == s.SlideID {
			var e error
			n, e = phase3MapSlideShape(n, func(shape *artifactNode) bool { return phase3SlideEditedShape(shape, s) }, func(shape *artifactNode) (*artifactNode, error) {
				shape = phase3ShapeNativeEmptyDefaults(shape)
				if rects[phase3ShapeName(shape)] {
					shape = phase3RemoveInheritedRectangle(shape)
				}
				shape = phase3RemoveInheritedLineDefaults(shape, lines[phase3ShapeName(shape)])
				return shape, nil
			})
			if e != nil {
				return nil, e
			}
		}
		return inner(i, n)
	}, nil
}
func phase3SlideEditedShape(n *artifactNode, s phase3SlidesSpec) bool {
	if n.Name.Space != artifactSlideNS || n.Name.Local != "sp" {
		return false
	}
	switch {
	case s.Format != nil:
		return phase3SlideText(n) == s.Format.Text
	case s.Shape != nil:
		return phase3ShapeName(n) == s.Shape.Name
	case s.Placeholder != nil:
		return phase3ShapeName(n) == s.Placeholder.Name
	}
	return false
}
func phase3BareRectangle(n *artifactNode) bool {
	if n == nil || n.Name.Space != artifactDrawingNS || n.Name.Local != "prstGeom" || n.attr("prst") != "rect" || n.Text != "" || len(n.Children) != 1 {
		return false
	}
	attrs := 0
	for _, a := range n.Attr {
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
			continue
		}
		if a.Name.Space != "" || a.Name.Local != "prst" {
			return false
		}
		attrs++
	}
	return attrs == 1 && n.Children[0].Name.Space == artifactDrawingNS && n.Children[0].Name.Local == "avLst" && phase3EmptyNativeNode(n.Children[0])
}
func phase3RemoveInheritedRectangle(shape *artifactNode) *artifactNode {
	out := *shape
	out.Children = append([]*artifactNode(nil), shape.Children...)
	for i, part := range out.Children {
		if part.Name.Space != artifactSlideNS || part.Name.Local != "spPr" {
			continue
		}
		pr := *part
		pr.Children = nil
		for _, c := range part.Children {
			if !phase3BareRectangle(c) {
				pr.Children = append(pr.Children, c)
			}
		}
		out.Children[i] = &pr
	}
	return &out
}
func phase3InheritedRectangle(entries map[string]*zip.File, part string, shape *artifactNode) bool {
	ph := phase3ThemeShapePlaceholder(shape)
	if ph == nil {
		return false
	}
	layout, e := phase3ThemeRelated(entries, part, "slideLayout")
	if e != nil {
		return false
	}
	n, e := artifactPart(entries, layout, artifactSlideNS, "sldLayout")
	if e != nil {
		return false
	}
	tree, e := phase3ShapeTree(n)
	if e != nil {
		return false
	}
	index := func(p *artifactNode) string {
		if p.attr("idx") == "" {
			return "0"
		}
		return p.attr("idx")
	}
	var inherited *artifactNode
	for _, candidate := range tree.Children {
		p := phase3ThemeShapePlaceholder(candidate)
		if p != nil && index(p) == index(ph) {
			if inherited != nil {
				return false
			}
			inherited = candidate
		}
	}
	if inherited == nil {
		return false
	}
	geometry := func(n *artifactNode) (bool, bool) {
		pr := phase3SlideChild(n, artifactSlideNS, "spPr")
		if pr == nil {
			return false, false
		}
		var found *artifactNode
		for _, c := range pr.Children {
			if c.Name.Space == artifactDrawingNS && (c.Name.Local == "prstGeom" || c.Name.Local == "custGeom") {
				if found != nil {
					return true, false
				}
				found = c
			}
		}
		return found != nil, phase3BareRectangle(found)
	}
	if found, rect := geometry(inherited); found {
		return rect
	}
	kind := phase3ThemeShapePlaceholder(inherited).attr("type")
	switch kind {
	case "title", "ctrTitle":
		kind = "title"
	case "", "obj", "subTitle", "body":
		kind = "body"
	}
	master, e := phase3ThemeRelated(entries, layout, "slideMaster")
	if e != nil {
		return false
	}
	n, e = artifactPart(entries, master, artifactSlideNS, "sldMaster")
	if e != nil {
		return false
	}
	tree, e = phase3ShapeTree(n)
	if e != nil {
		return false
	}
	inherited = nil
	for _, candidate := range tree.Children {
		p := phase3ThemeShapePlaceholder(candidate)
		if p != nil && p.attr("type") == kind {
			if inherited != nil {
				return false
			}
			inherited = candidate
		}
	}
	if inherited == nil {
		return false
	}
	_, rect := geometry(inherited)
	return rect
}

// Resolve only the explicit line-style reference. A missing/ambiguous ancestor
// is not evidence that the default line style is safe to erase.
func phase3InheritedThemeLine(entries map[string]*zip.File, part string, shape *artifactNode) *artifactNode {
	style := phase3SlideChild(shape, artifactSlideNS, "style")
	ref := phase3SlideChild(style, artifactDrawingNS, "lnRef")
	if ref == nil {
		return nil
	}
	index, e := strconv.Atoi(ref.attr("idx"))
	if e != nil || index < 1 {
		return nil
	}
	layout, e := phase3ThemeRelated(entries, part, "slideLayout")
	if e != nil {
		return nil
	}
	master, e := phase3ThemeRelated(entries, layout, "slideMaster")
	if e != nil {
		return nil
	}
	theme, e := phase3ThemeRelated(entries, master, "theme")
	if e != nil {
		return nil
	}
	root, e := artifactPart(entries, theme, artifactDrawingNS, "theme")
	if e != nil {
		return nil
	}
	list := phase3SlideChild(phase3SlideChild(phase3SlideChild(root, artifactDrawingNS, "themeElements"), artifactDrawingNS, "fmtScheme"), artifactDrawingNS, "lnStyleLst")
	if list == nil || index > len(list.Children) {
		return nil
	}
	line := list.Children[index-1]
	if line.Name.Space != artifactDrawingNS || line.Name.Local != "ln" {
		return nil
	}
	return line
}
func phase3RemoveInheritedLineDefaults(shape, inherited *artifactNode) *artifactNode {
	if inherited == nil {
		return shape
	}
	out := *shape
	out.Children = append([]*artifactNode(nil), shape.Children...)
	for i, pr := range out.Children {
		if pr.Name.Space != artifactSlideNS || pr.Name.Local != "spPr" {
			continue
		}
		props := *pr
		props.Children = append([]*artifactNode(nil), pr.Children...)
		for j, line := range props.Children {
			if line.Name.Space != artifactDrawingNS || line.Name.Local != "ln" {
				continue
			}
			value := *line
			value.Attr = nil
			value.Children = nil
			for _, a := range line.Attr {
				defaults := map[string]string{"cap": "flat", "cmpd": "sng", "algn": "ctr"}
				if a.Name.Space == "" && defaults[a.Name.Local] == a.Value && inherited.attr(a.Name.Local) == a.Value {
					continue
				}
				value.Attr = append(value.Attr, a)
			}
			for _, c := range line.Children {
				source := phase3SlideChild(inherited, artifactDrawingNS, "prstDash")
				if phase3SolidLineDash(c) && phase3SolidLineDash(source) {
					continue
				}
				value.Children = append(value.Children, c)
			}
			props.Children[j] = &value
		}
		out.Children[i] = &props
	}
	return &out
}
func phase3SolidLineDash(n *artifactNode) bool {
	if n == nil || n.Name.Space != artifactDrawingNS || n.Name.Local != "prstDash" || len(n.Attr) != 1 || len(n.Children) != 0 || n.Text != "" {
		return false
	}
	return n.Attr[0].Name.Space == "" && n.Attr[0].Name.Local == "val" && n.Attr[0].Value == "solid"
}
