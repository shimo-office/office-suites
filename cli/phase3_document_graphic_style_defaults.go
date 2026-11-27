package main

// The native writer materializes this style on imported shapes. It has no
// visible effect only when explicit fill/line win, effectRef is zero, and there
// is no text to inherit fontRef. Keep the complete observed style exact.
const documentGraphicNativeEmptyStyle = `<wps:style xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:lnRef idx="0"/><a:fillRef idx="1"><a:srgbClr val="B1B7BE"/></a:fillRef><a:effectRef idx="0"><a:schemeClr val="accent1"/></a:effectRef><a:fontRef idx="minor"><a:srgbClr val="000000"/></a:fontRef></wps:style>`

func normalizeDocumentGraphicEmptyStyle(before, after *artifactNode) {
	bs := documentGraphicNodes(before, documentShapeNS, "wsp")
	as := documentGraphicNodes(after, documentShapeNS, "wsp")
	if len(bs) != 1 || len(as) != 1 || len(bs[0].children("style")) != 0 || len(as[0].children("style")) != 1 {
		return
	}
	// No text body, links, extensions or other descendants may inherit the
	// newly serialized font or use its effects. This only covers empty shapes.
	for _, shape := range []*artifactNode{bs[0], as[0]} {
		for _, child := range shape.Children {
			if child.Name.Space != documentShapeNS {
				return
			}
			switch child.Name.Local {
			case "cNvSpPr", "cNvPr", "bodyPr", "spPr", "style":
			default:
				return
			}
		}
	}
	style := as[0].child("style")
	expected, err := parseArtifactXML([]byte(documentGraphicNativeEmptyStyle))
	if err != nil {
		return
	}
	// Namespace declarations belong to the template wrapper, not style data.
	expected.Attr = nil
	if layoutNodeCanonical(style) != layoutNodeCanonical(expected) {
		return
	}
	bp, be := documentGraphicShapeProperties(before)
	ap, ae := documentGraphicShapeProperties(after)
	if be != nil || ae != nil {
		return
	}
	// Explicit opaque fill and line are unchanged and override the style refs.
	for _, pair := range [][2]*artifactNode{{bp, ap}, {documentGraphicChild(bp, "ln"), documentGraphicChild(ap, "ln")}} {
		if pair[0] == nil || pair[1] == nil {
			return
		}
		bfill, afill := documentGraphicChild(pair[0], "solidFill"), documentGraphicChild(pair[1], "solidFill")
		if bfill == nil || afill == nil || layoutNodeCanonical(bfill) != layoutNodeCanonical(afill) || len(bfill.Children) != 1 {
			return
		}
		color := bfill.Children[0]
		if color.Name.Space != documentDrawingMainNS || color.Name.Local != "srgbClr" || len(color.Attr) != 1 || color.Attr[0].Name.Space != "" || color.Attr[0].Name.Local != "val" || !documentGraphicOpaqueAlpha(color) {
			return
		}
		if _, err := layoutColor("#" + color.attr("val")); err != nil || documentGraphicFillCount(pair[0]) != 1 || documentGraphicFillCount(pair[1]) != 1 {
			return
		}
	}
	if layoutNodeCanonical(documentGraphicChild(bp, "ln")) != layoutNodeCanonical(documentGraphicChild(ap, "ln")) {
		return
	}
	documentGraphicDropNode(as[0], style)
}
