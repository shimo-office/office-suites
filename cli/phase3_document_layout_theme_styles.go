package main

import (
	"fmt"
	"regexp"
)

type documentThemeStyleTable struct {
	Kind            string `json:"kind"`
	BasedOn         string `json:"basedOn"`
	TableProperties string `json:"tableProperties"`
	EligibleDefault bool   `json:"eligibleDefault"`
}
type documentThemeStyleDefaults struct {
	Styles          map[string]documentThemeStyleTable `json:"styles"`
	WithoutDefaults string                             `json:"withoutDefaults"`
}

var documentThemeNativeHeadingName = regexp.MustCompile(`^(Heading [1-9]|Title|Subtitle)( Char)?$`)

func documentThemeZeroTableIndent(n *artifactNode) bool {
	if n == nil || n.Name.Space != artifactWordNS || n.Name.Local != "tblPr" || len(n.Attr) != 0 || n.Text != "" || len(n.Children) != 1 {
		return false
	}
	c := n.Children[0]
	return c.Name.Space == artifactWordNS && c.Name.Local == "tblInd" && len(c.Attr) == 1 && c.Attr[0].Name.Space == artifactWordNS && c.Attr[0].Name.Local == "w" && c.Attr[0].Value == "0" && c.Text == "" && len(c.Children) == 0
}

// The SDK reserializes existing heading/linked-character styles when installing
// theme references. Only an exact absent -> zero table-indent default is eligible;
// the verifier separately proves no table style can inherit it.
func readDocumentThemeStyleDefaults(styles *artifactNode) (*documentThemeStyleDefaults, error) {
	out := &documentThemeStyleDefaults{Styles: map[string]documentThemeStyleTable{}}
	clone := documentThemeWithoutFonts(styles, false)
	for _, st := range clone.Children {
		if st.Name.Space != artifactWordNS || st.Name.Local != "style" {
			continue
		}
		id := documentAttr(st, "styleId")
		if id == "" {
			return nil, fmt.Errorf("样式缺少唯一ID")
		}
		if _, exists := out.Styles[id]; exists {
			return nil, fmt.Errorf("样式ID重复")
		}
		info := documentThemeStyleTable{Kind: documentAttr(st, "type")}
		if b := st.child("basedOn"); b != nil {
			info.BasedOn = documentAttr(b, "val")
		}
		name := ""
		if n := st.child("name"); n != nil {
			name = documentAttr(n, "val")
		}
		properties := st.child("tblPr")
		info.TableProperties = layoutNodeCanonical(properties)
		info.EligibleDefault = (info.Kind == "paragraph" || info.Kind == "character") && documentThemeNativeHeadingName.MatchString(name) && documentThemeZeroTableIndent(properties)
		out.Styles[id] = info
		if info.EligibleDefault {
			children := make([]*artifactNode, 0, len(st.Children))
			for _, c := range st.Children {
				if c != properties {
					children = append(children, c)
				}
			}
			st.Children = children
		}
	}
	out.WithoutDefaults = layoutNodeCanonical(clone)
	return out, nil
}
func verifyDocumentThemeStyleDefaults(before, after *documentThemeStyleDefaults) error {
	if before == nil || after == nil || before.WithoutDefaults != after.WithoutDefaults {
		return fmt.Errorf("主题动作产生非字体样式变化")
	}
	added := map[string]bool{}
	for id, a := range after.Styles {
		b, exists := before.Styles[id]
		if !exists || a.Kind != b.Kind || a.BasedOn != b.BasedOn {
			return fmt.Errorf("主题动作改变样式身份或继承")
		}
		if a.TableProperties == b.TableProperties {
			continue
		}
		if b.TableProperties != "" || !a.EligibleDefault {
			return fmt.Errorf("主题动作改变既有或非缺省表格样式属性")
		}
		added[id] = true
	}
	if len(added) == 0 {
		return fmt.Errorf("非字体样式差异不是已证实的默认序列化")
	}
	for id, st := range after.Styles {
		if st.Kind != "table" {
			continue
		}
		visited := map[string]bool{}
		for current := id; current != ""; {
			if added[current] {
				return fmt.Errorf("表格样式%s会继承新增缩进，不能视为无影响默认值", id)
			}
			if visited[current] {
				return fmt.Errorf("表格样式继承循环，无法验证默认值影响")
			}
			visited[current] = true
			parent, exists := after.Styles[current]
			if !exists {
				return fmt.Errorf("表格样式继承不完整，无法验证默认值影响")
			}
			current = parent.BasedOn
		}
	}
	return nil
}
