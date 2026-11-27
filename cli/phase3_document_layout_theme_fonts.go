package main

import (
	"encoding/xml"
	"fmt"
	"reflect"
)

// Include unused styles and document defaults: checking visible characters alone
// cannot prove that future uses of a style retain their original script fonts.
func documentThemeStyleFonts(styles *artifactNode) map[string]string {
	fonts := map[string]string{}
	var walk func(*artifactNode, string)
	walk = func(n *artifactNode, path string) {
		if n.Name.Space == artifactWordNS && n.Name.Local == "rFonts" {
			copy := *n
			copy.Attr = append([]xml.Attr(nil), n.Attr...)
			for i, attr := range copy.Attr {
				if attr.Name.Space == artifactWordNS && (attr.Name.Local == "asciiTheme" || attr.Name.Local == "hAnsiTheme") {
					switch attr.Value {
					case "majorHAnsi":
						copy.Attr[i].Value = "majorAscii"
					case "minorHAnsi":
						copy.Attr[i].Value = "minorAscii"
					}
				}
			}
			fonts[path] = layoutNodeCanonical(&copy)
		}
		counts := map[xml.Name]int{}
		for _, child := range n.Children {
			index := counts[child.Name]
			counts[child.Name]++
			walk(child, fmt.Sprintf("%s/{%s}%s[%d]", path, child.Name.Space, child.Name.Local, index))
		}
	}
	if styles != nil {
		walk(styles, "")
	}
	return fonts
}

func verifyDocumentThemeStyleFonts(before, after map[string]string) error {
	// Older synthetic snapshots without a styles part have no font assignments.
	if len(before) == 0 && len(after) == 0 {
		return nil
	}
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("主题动作改变了样式或文档默认的字体覆盖/主题引用")
	}
	return nil
}
