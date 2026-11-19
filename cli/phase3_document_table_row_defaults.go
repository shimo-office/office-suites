package main

// Native rebuildFromPositions writes explicit row gridBefore/gridAfter=0 and
// wBefore/wAfter dxa=0 even when these defaults were omitted in OOXML. Only
// distribution, column edits and split verification use these keys; exact keys stay intact.
// readDocumentTables disables this equivalence if any captured style resource
// (including relationship-bound custom parts) contains these row/cell properties:
// explicit defaults can override inherited values.
func documentTableRebuiltRow(row *artifactNode, stripHeight bool) *artifactNode {
	clone := documentStyleClone(row)
	if clone == nil {
		return nil
	}
	for i, props := range clone.Children {
		if props.Name.Space != artifactWordNS || props.Name.Local != "trPr" {
			continue
		}
		counts := map[string]int{}
		for _, n := range props.Children {
			if n.Name.Space == artifactWordNS {
				counts[n.Name.Local]++
			}
		}
		kept := []*artifactNode{}
		for _, n := range props.Children {
			if stripHeight && n.Name.Space == artifactWordNS && n.Name.Local == "trHeight" {
				continue
			}
			if counts[n.Name.Local] == 1 && documentTableExplicitZeroRowDefault(n) {
				continue
			}
			kept = append(kept, n)
		}
		props.Children = kept
		if len(props.Children) == 0 && len(props.Attr) == 0 && props.Text == "" {
			clone.Children = append(clone.Children[:i], clone.Children[i+1:]...)
		}
		break
	}
	return clone
}
func documentTableExplicitZeroRowDefault(n *artifactNode) bool {
	if n.Name.Space != artifactWordNS || n.Text != "" || len(n.Children) != 0 {
		return false
	}
	expected := map[string]string{}
	switch n.Name.Local {
	case "gridBefore", "gridAfter":
		expected["val"] = "0"
	case "wBefore", "wAfter":
		expected["w"] = "0"
		expected["type"] = "dxa"
	default:
		return false
	}
	if len(n.Attr) != len(expected) {
		return false
	}
	seen := map[string]bool{}
	for _, a := range n.Attr {
		value, ok := expected[a.Name.Local]
		if a.Name.Space != artifactWordNS || !ok || value != a.Value || seen[a.Name.Local] {
			return false
		}
		seen[a.Name.Local] = true
	}
	return true
}
func documentTableRebuiltKeys(t documentTable) (styles, other, envelopes []string) {
	// Manually constructed unit fixtures predate readback-derived normalized keys.
	styles, other, envelopes = t.RebuiltRowStyles, t.RebuiltRowOtherStyles, t.RebuiltRowEnvelopes
	if len(styles) == 0 {
		styles = t.RowStyles
	}
	if len(other) == 0 {
		other = t.RowOtherStyles
	}
	if len(envelopes) == 0 {
		envelopes = t.RowEnvelopes
	}
	return
}

// A rebuilt unmerged column also explicitly serializes the default gridSpan=1.
// No unknown attributes, nested nodes, duplicate spans or other values are masked.
func documentTableRebuiltCellStyle(cell *artifactNode) string {
	clone := documentStyleClone(cell)
	props := clone.child("tcPr")
	if props != nil {
		count := 0
		for _, n := range props.Children {
			if n.Name.Space == artifactWordNS && n.Name.Local == "gridSpan" {
				count++
			}
		}
		if count == 1 {
			for i, n := range props.Children {
				if n.Name.Space == artifactWordNS && n.Name.Local == "gridSpan" && n.Text == "" && len(n.Children) == 0 && len(n.Attr) == 1 && n.Attr[0].Name.Space == artifactWordNS && n.Attr[0].Name.Local == "val" && n.Attr[0].Value == "1" {
					props.Children = append(props.Children[:i], props.Children[i+1:]...)
					break
				}
			}
		}
	}
	return documentTableNodeKey(clone, false)
}
