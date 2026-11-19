package main

import "encoding/xml"

// These are schema defaults from CT_NonVisualDrawingProps,
// CT_ApplicationNonVisualDrawingProps, CT_Placeholder and
// CT_Transform2D. Native text editing may serialize them explicitly. Normalize
// only the exact default on its qualified element; non-default values and
// attributes from other namespaces remain part of the collateral-change proof.
func phase3SlideExplicitDefault(element xml.Name, attribute xml.Attr) bool {
	if attribute.Name.Space != "" {
		return false
	}
	name, value := attribute.Name.Local, attribute.Value
	booleanDefault := value == "0" || value == "false"
	if element.Space == artifactSlideNS {
		switch element.Local {
		case "cNvPr":
			return name == "hidden" && booleanDefault
		case "nvPr":
			return (name == "isPhoto" || name == "userDrawn") && booleanDefault
		case "ph":
			return name == "hasCustomPrompt" && booleanDefault
		}
	}
	if element.Space == artifactDrawingNS && element.Local == "xfrm" {
		return name == "rot" && value == "0" || (name == "flipH" || name == "flipV") && booleanDefault
	}
	return false
}

// Called only for the explicitly edited shape, after package-wide non-auto BW
// modes have been rejected. Empty paragraph properties have no overrides to
// inheritance. Preserve any attribute, child, text, foreign node or actual run.
func phase3ShapeNativeEmptyDefaults(n *artifactNode) *artifactNode {
	out := phase3PlaceholderNativeBW(n)
	for i, tx := range out.Children {
		if tx.Name.Space != artifactSlideNS || tx.Name.Local != "txBody" {
			continue
		}
		body := *tx
		body.Children = append([]*artifactNode(nil), tx.Children...)
		for j, p := range body.Children {
			if p.Name.Space != artifactDrawingNS || p.Name.Local != "p" || p.Text != "" {
				continue
			}
			empty := true
			seenParagraph, seenEnd := false, false
			for _, style := range p.Children {
				if style.Name.Space != artifactDrawingNS || (style.Name.Local != "pPr" && style.Name.Local != "endParaRPr") || len(style.Attr) != 0 || len(style.Children) != 0 || style.Text != "" {
					empty = false
				}
				if style.Name.Local == "pPr" {
					if seenParagraph || seenEnd {
						empty = false
					}
					seenParagraph = true
				} else if style.Name.Local == "endParaRPr" {
					if seenEnd {
						empty = false
					}
					seenEnd = true
				}
			}
			if empty {
				paragraph := *p
				paragraph.Children = nil
				body.Children[j] = &paragraph
			}
		}
		out.Children[i] = &body
	}
	return out
}
