package main

// Only target-table cells enter this normalizer. An absent optional style and
// its completely empty native serialization carry no explicit overrides. Keep
// nonempty styles, duplicate/out-of-order nodes, text, and foreign namespaces.
// This comparison proves preservation of existing content formatting. Empty
// endParaRPr does not prove identical defaults for text appended in the future.
func phase3TableNativeCell(cell *artifactNode) *artifactNode {
	if cell.Name.Space != artifactDrawingNS || cell.Name.Local != "tc" {
		return cell
	}
	out := *cell
	out.Children = append([]*artifactNode(nil), cell.Children...)
	for i, tx := range out.Children {
		if tx.Name.Space != artifactDrawingNS || tx.Name.Local != "txBody" {
			continue
		}
		if i != 0 || len(tx.Children) < 2 || tx.Children[0].Name.Space != artifactDrawingNS || tx.Children[0].Name.Local != "bodyPr" {
			return cell
		}
		body := *tx
		body.Children = []*artifactNode{tx.Children[0]}
		seenParagraph := false
		for j, child := range tx.Children[1:] {
			if child.Name.Space != artifactDrawingNS {
				return cell
			}
			if child.Name.Local == "lstStyle" {
				if j != 0 {
					return cell
				}
				if !phase3EmptyNativeNode(child) {
					body.Children = append(body.Children, child)
				}
			} else if child.Name.Local == "p" {
				seenParagraph = true
				body.Children = append(body.Children, phase3TableNativeParagraph(child))
			} else {
				return cell
			}
		}
		if !seenParagraph {
			return cell
		}
		out.Children[i] = &body
	}
	return &out
}

func phase3EmptyNativeNode(n *artifactNode) bool {
	return len(n.Attr) == 0 && len(n.Children) == 0 && n.Text == ""
}

func phase3TableNativeParagraph(p *artifactNode) *artifactNode {
	out := *p
	out.Children = nil
	for i, child := range p.Children {
		if child.Name.Space != artifactDrawingNS {
			return p
		}
		switch child.Name.Local {
		case "pPr":
			if i != 0 {
				return p
			}
		case "endParaRPr":
			if i != len(p.Children)-1 {
				return p
			}
		case "r", "br", "fld":
			out.Children = append(out.Children, child)
			continue
		default:
			return p
		}
		if !phase3EmptyNativeNode(child) {
			out.Children = append(out.Children, child)
		}
	}
	return &out
}

func phase3TableQualifiedName(n *artifactNode) string {
	if n.Name.Space == artifactSlideNS {
		return "p:" + n.Name.Local
	}
	if n.Name.Space == artifactDrawingNS {
		return "a:" + n.Name.Local
	}
	return "{" + n.Name.Space + "}" + n.Name.Local
}
