package main

import (
	"encoding/xml"
	"fmt"
	"reflect"
	"strings"
)

// Runs may split during formatting. Keep text positions while preserving every
// non-text node, wrapper, unknown property and attribute outside that split.
type documentStructureToken struct {
	Kind  string `json:"kind"`
	Text  string `json:"text,omitempty"`
	Guard string `json:"guard,omitempty"`
}

func validateDocumentStructure(n *artifactNode) error {
	return validateDocumentStructureNode(n, false)
}

func validateDocumentStructureNode(n *artifactNode, preserveSpace bool) error {
	for _, a := range n.Attr {
		if a.Name == (xml.Name{Space: "http://www.w3.org/XML/1998/namespace", Local: "space"}) {
			preserveSpace = a.Value == "preserve"
		}
	}
	// artifactNode aggregates direct text. It cannot establish its order
	// relative to children, so do not certify unknown mixed-content XML.
	if len(n.Children) > 0 && (strings.TrimSpace(n.Text) != "" || n.Text != "" && (preserveSpace || n.Name.Space != artifactWordNS && n.Name.Space != documentDrawingMainNS)) {
		return fmt.Errorf("不支持无法严格核对位置的文稿混合 XML 内容")
	}
	for _, child := range n.Children {
		if err := validateDocumentStructureNode(child, preserveSpace); err != nil {
			return err
		}
	}
	return nil
}

func documentStructureSkeleton(n *artifactNode) *artifactNode {
	c := *n
	c.Children = nil
	if n.Name == (xml.Name{Space: artifactWordNS, Local: "p"}) {
		return &c
	}
	for _, child := range n.Children {
		c.Children = append(c.Children, documentStructureSkeleton(child))
	}
	return &c
}

func documentRunStructureGuard(n *artifactNode) string {
	if n == nil {
		return ""
	}
	c := *n
	c.Children = nil
	allowed := map[string]map[string]bool{
		"rFonts": {"ascii": true, "hAnsi": true, "eastAsia": true, "cs": true, "asciiTheme": true, "hAnsiTheme": true, "eastAsiaTheme": true, "cstheme": true},
		"sz":     {"val": true}, "szCs": {"val": true}, "color": {"val": true}, "vertAlign": {"val": true}, "b": {"val": true}, "i": {"val": true}, "u": {"val": true}, "strike": {"val": true},
	}
	for _, child := range n.Children {
		x := *child
		x.Attr = nil
		keys, known := allowed[child.Name.Local]
		if child.Name.Space != artifactWordNS || !known {
			c.Children = append(c.Children, child)
			continue
		}
		for _, a := range child.Attr {
			if a.Name.Space == artifactWordNS && (keys[a.Name.Local] || child.Name.Local == "u" && a.Name.Local == "color" && a.Value == "auto") {
				continue
			}
			x.Attr = append(x.Attr, a)
		}
		if len(x.Attr) > 0 || len(x.Children) > 0 || x.Text != "" {
			c.Children = append(c.Children, &x)
		}
	}
	if len(c.Attr) == 0 && len(c.Children) == 0 && c.Text == "" {
		return ""
	}
	return documentResourceKey(&c)
}

func documentParagraphSpaceModes(root *artifactNode) map[*artifactNode]string {
	out := map[*artifactNode]string{}
	var walk func(*artifactNode, string)
	walk = func(n *artifactNode, inherited string) {
		mode := documentXMLSpaceMode(n, inherited)
		if n.Name == (xml.Name{Space: artifactWordNS, Local: "p"}) {
			out[n] = mode
		}
		for _, child := range n.Children {
			walk(child, mode)
		}
	}
	walk(root, "")
	return out
}

func documentXMLSpaceMode(n *artifactNode, inherited string) string {
	for _, a := range n.Attr {
		if a.Name == (xml.Name{Space: "http://www.w3.org/XML/1998/namespace", Local: "space"}) {
			return a.Value
		}
	}
	return inherited
}

func documentParagraphStructure(p *artifactNode, inheritedSpace string) []documentStructureToken {
	var out []documentStructureToken
	var walk func(*artifactNode, string, string)
	walk = func(n *artifactNode, guard, space string) {
		space = documentXMLSpaceMode(n, space)
		if n.Name.Space == artifactWordNS {
			switch n.Name.Local {
			case "r":
				header := *n
				header.Children = nil
				if len(n.Attr) > 0 || n.Text != "" {
					guard += documentResourceKey(&header)
				}
				exactGuard := guard
				propertyCount := 0
				for _, child := range n.Children {
					if child.Name == (xml.Name{Space: artifactWordNS, Local: "rPr"}) {
						propertyCount++
						if len(child.Attr) > 0 || len(child.Children) > 0 || child.Text != "" {
							exactGuard += documentResourceKey(child)
						}
					}
				}
				guard += documentRunStructureGuard(n.child("rPr"))
				if propertyCount > 1 {
					guard = exactGuard
				}
				count := len(out)
				for _, child := range n.Children {
					if child.Name != (xml.Name{Space: artifactWordNS, Local: "rPr"}) {
						childGuard := guard
						if child.Name != (xml.Name{Space: artifactWordNS, Local: "t"}) {
							childGuard = exactGuard
						}
						walk(child, childGuard, space)
					}
				}
				if count == len(out) && exactGuard != "" {
					out = append(out, documentStructureToken{Kind: "empty-run", Guard: exactGuard})
				}
				return
			case "t":
				header := *n
				header.Text = ""
				header.Attr = nil
				for _, a := range n.Attr {
					if a.Name != (xml.Name{Space: "http://www.w3.org/XML/1998/namespace", Local: "space"}) || (a.Value != "preserve" && a.Value != "default") {
						header.Attr = append(header.Attr, a)
					}
				}
				if len(header.Attr) > 0 || len(header.Children) > 0 {
					guard += documentResourceKey(&header)
				}
				if n.Text == "" && guard != "" {
					out = append(out, documentStructureToken{Kind: "empty-text", Guard: guard})
				}
				runes := []rune(n.Text)
				left, right := 0, len(runes)
				for left < len(runes) && strings.ContainsRune(" \t\r\n", runes[left]) {
					left++
				}
				for right > 0 && strings.ContainsRune(" \t\r\n", runes[right-1]) {
					right--
				}
				for i, r := range runes {
					charGuard := guard
					if space != "preserve" && (i < left || i >= right) {
						charGuard += "unpreserved-boundary-space"
					}
					out = append(out, documentStructureToken{Kind: "text", Text: string(r), Guard: charGuard})
				}
				return
			case "hyperlink":
				header := *n
				header.Children = nil
				out = append(out, documentStructureToken{Kind: "hyperlink-start", Guard: guard + documentResourceKey(&header)})
				for _, child := range n.Children {
					walk(child, guard, space)
				}
				out = append(out, documentStructureToken{Kind: "hyperlink-end"})
				return
			}
		}
		out = append(out, documentStructureToken{Kind: "node", Guard: guard + documentResourceKey(n)})
	}
	for _, n := range p.Children {
		if n.Name == (xml.Name{Space: artifactWordNS, Local: "pPr"}) {
			if g := documentRunStructureGuard(n.child("rPr")); g != "" {
				out = append(out, documentStructureToken{Kind: "paragraph-mark", Guard: g})
			}
			continue
		}
		walk(n, "", inheritedSpace)
	}
	return out
}

func verifyDocumentStructure(before, after documentSnapshot) error {
	if before.Structure != after.Structure {
		return fmt.Errorf("请求外的正文、表格或分节结构改变")
	}
	if len(before.Paragraphs) != len(after.Paragraphs) {
		return fmt.Errorf("段落数改变")
	}
	for i, p := range before.Paragraphs {
		if !reflect.DeepEqual(p.Structure, after.Paragraphs[i].Structure) {
			return fmt.Errorf("第 %d 段出现请求外非文本结构或对象位置变化", i+1)
		}
	}
	return nil
}

func documentTrimMentionStructure(tokens []documentStructureToken, text, label string) ([]documentStructureToken, error) {
	// Hand-built snapshots used by unit callers predate structural capture.
	if len(tokens) == 0 {
		return nil, nil
	}
	n := len([]rune(text))
	start, count := -1, 0
	for i, t := range tokens {
		if t.Kind == "text" {
			if count == n {
				start = i
				break
			}
			count++
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("提及结构缺少新增文字")
	}
	end := start
	for _, r := range label {
		if end >= len(tokens) || tokens[end] != (documentStructureToken{Kind: "text", Text: string(r)}) {
			return nil, fmt.Errorf("提及新增区域包含未验证结构")
		}
		end++
	}
	emptyLink := documentResourceKey(&artifactNode{Name: xml.Name{Space: artifactWordNS, Local: "hyperlink"}})
	if start > 0 && tokens[start-1] == (documentStructureToken{Kind: "hyperlink-start", Guard: emptyLink}) && end < len(tokens) && tokens[end].Kind == "hyperlink-end" {
		start--
		end++
	}
	result := append([]documentStructureToken(nil), tokens[:start]...)
	return append(result, tokens[end:]...), nil
}
