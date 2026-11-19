package main

import "encoding/xml"

// The native merge moves paragraphs and materializes an empty run property
// container. Normalize only direct paragraph runs, and only one truly empty,
// qualified w:rPr. Raw paragraph keys remain available for every other scope.
func documentMergeParagraphKey(p *artifactNode) string {
	clone := documentStyleClone(p)
	for _, run := range clone.Children {
		if run.Name != (xml.Name{Space: artifactWordNS, Local: "r"}) {
			continue
		}
		count := 0
		for _, child := range run.Children {
			if child.Name == (xml.Name{Space: artifactWordNS, Local: "rPr"}) {
				count++
			}
		}
		if count != 1 {
			continue
		}
		for i, child := range run.Children {
			if child.Name == (xml.Name{Space: artifactWordNS, Local: "rPr"}) && len(child.Attr) == 0 && len(child.Children) == 0 && child.Text == "" {
				run.Children = append(run.Children[:i], run.Children[i+1:]...)
				break
			}
		}
	}
	return documentTableExactKey(clone)
}

func documentMergeParagraphKeys(cell documentTableCell) []string {
	if len(cell.MergeParagraphKeys) == len(cell.ParagraphKeys) {
		return cell.MergeParagraphKeys
	}
	return cell.ParagraphKeys
}
