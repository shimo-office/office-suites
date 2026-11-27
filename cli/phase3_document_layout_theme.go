package main

import (
	"archive/zip"
	"fmt"
	"reflect"
	"strings"
)

type documentThemeFamily struct {
	Latin string `json:"latin"`
	EA    string `json:"ea"`
	CS    string `json:"cs"`
}
type documentThemeFonts struct {
	Major documentThemeFamily `json:"major"`
	Minor documentThemeFamily `json:"minor"`
}
type documentThemeSnapshot struct {
	StyleDefaults   *documentThemeStyleDefaults `json:"styleDefaults,omitempty"`
	DirectRunFonts  []string                    `json:"directRunFonts"`
	StyleFonts      map[string]string           `json:"styleFonts"`
	Fonts           documentThemeFonts          `json:"fonts"`
	Supplemental    map[string]string           `json:"supplemental"`
	NonFontDocument string                      `json:"nonFontDocument"`
	NonFontStyles   string                      `json:"nonFontStyles"`
	NonFontTheme    string                      `json:"nonFontTheme"`
	RawTheme        string                      `json:"rawTheme"`
	RawStyles       string                      `json:"rawStyles"`
}

func prepareDocumentTheme(p map[string]any) (*documentThemeFonts, error) {
	if p["resetSupplemental"] != true {
		return nil, fmt.Errorf("document-theme 必须明确 resetSupplemental:true：整套主题会清空补充语言字体映射，保留现有段落样式与显式字体")
	}
	raw, ok := p["themeFonts"].(map[string]any)
	if !ok || len(raw) != 2 {
		return nil, fmt.Errorf("themeFonts 必须完整提供 major 和 minor")
	}
	out := &documentThemeFonts{}
	for name, dest := range map[string]*documentThemeFamily{"major": &out.Major, "minor": &out.Minor} {
		family, ok := raw[name].(map[string]any)
		if !ok || len(family) != 3 {
			return nil, fmt.Errorf("%s 必须完整提供 latin/ea/cs", name)
		}
		for key, ptr := range map[string]*string{"latin": &dest.Latin, "ea": &dest.EA, "cs": &dest.CS} {
			v, ok := family[key].(string)
			if !ok || strings.TrimSpace(v) != v || v == "" || len(v) > 160 || strings.ContainsAny(v, "\r\n\t\x00") {
				return nil, fmt.Errorf("%s.%s 字体名称无效", name, key)
			}
			*ptr = v
		}
	}
	return out, nil
}

// Font-only normalization is deliberately narrow: theme mutation must leave all
// content and every non-font property intact. Empty rPr nodes are equivalent.
func documentThemeWithoutFonts(n *artifactNode, theme bool) *artifactNode {
	if n == nil {
		return nil
	}
	c := *n
	c.Attr = append(c.Attr[:0:0], n.Attr...)
	// Word hex colors are case insensitive; preserve every non-color attribute.
	if !theme && n.Name.Space == artifactWordNS {
		for i, attr := range c.Attr {
			if attr.Name.Space == artifactWordNS && (attr.Name.Local == "color" || (n.Name.Local == "color" && attr.Name.Local == "val") || (n.Name.Local == "shd" && attr.Name.Local == "fill")) {
				if hex, err := layoutColor("#" + attr.Value); err == nil {
					c.Attr[i].Value = strings.TrimPrefix(hex, "#")
				}
			}
		}
	}
	c.Children = nil
	for _, child := range n.Children {
		if (!theme && child.Name.Space == artifactWordNS && child.Name.Local == "rFonts") || (theme && child.Name.Space == artifactDrawingNS && child.Name.Local == "fontScheme") {
			continue
		}
		v := documentThemeWithoutFonts(child, theme)
		if !theme && v.Name.Space == artifactWordNS && v.Name.Local == "rPr" && len(v.Attr) == 0 && len(v.Children) == 0 && v.Text == "" {
			continue
		}
		c.Children = append(c.Children, v)
	}
	return &c
}
func readDocumentTheme(parts map[string]*zip.File, root *artifactNode) (*documentThemeSnapshot, error) {
	if parts["word/theme/theme1.xml"] == nil {
		return nil, nil
	}
	theme, e := artifactPart(parts, "word/theme/theme1.xml", artifactDrawingNS, "theme")
	if e != nil {
		return nil, e
	}
	var scheme *artifactNode
	var walk func(*artifactNode)
	walk = func(n *artifactNode) {
		if n.Name.Space == artifactDrawingNS && n.Name.Local == "fontScheme" {
			scheme = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(theme)
	if scheme == nil {
		return nil, fmt.Errorf("主题缺少 fontScheme")
	}
	out := &documentThemeSnapshot{Supplemental: map[string]string{}, RawTheme: layoutNodeCanonical(theme), NonFontTheme: layoutNodeCanonical(documentThemeWithoutFonts(theme, true)), NonFontDocument: layoutNodeCanonical(documentThemeWithoutFonts(root, false))}
	for name, dest := range map[string]*documentThemeFamily{"majorFont": &out.Fonts.Major, "minorFont": &out.Fonts.Minor} {
		family := scheme.child(name)
		if family == nil {
			return nil, fmt.Errorf("主题缺少 %s", name)
		}
		for _, c := range family.Children {
			switch c.Name.Local {
			case "latin":
				dest.Latin = c.attr("typeface")
			case "ea":
				dest.EA = c.attr("typeface")
			case "cs":
				dest.CS = c.attr("typeface")
			case "font":
				out.Supplemental[name+"."+c.attr("script")] = c.attr("typeface")
			}
		}
	}
	for _, paragraph := range documentChildren(root, "p") {
		for _, run := range documentChildren(paragraph, "r") {
			fontNode := (*artifactNode)(nil)
			if props := run.child("rPr"); props != nil {
				fontNode = props.child("rFonts")
			}
			out.DirectRunFonts = append(out.DirectRunFonts, layoutNodeCanonical(fontNode))
		}
	}
	if parts["word/styles.xml"] != nil {
		styles, e := artifactPart(parts, "word/styles.xml", artifactWordNS, "styles")
		if e != nil {
			return nil, e
		}
		out.RawStyles = layoutNodeCanonical(styles)
		out.StyleFonts = documentThemeStyleFonts(styles)
		out.NonFontStyles = layoutNodeCanonical(documentThemeWithoutFonts(styles, false))
		out.StyleDefaults, e = readDocumentThemeStyleDefaults(styles)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func verifyDocumentTheme(before, after documentLayoutSnapshot, s documentLayoutSpec) error {
	if s.Theme == nil || before.Theme == nil || after.Theme == nil {
		return fmt.Errorf("无法独立验证主题字体")
	}
	if !reflect.DeepEqual(after.Theme.Fonts, *s.Theme) {
		return fmt.Errorf("DOCX 整套主题字体与请求不符")
	}
	if len(after.Theme.Supplemental) > 0 {
		return fmt.Errorf("DOCX 仍含未明确请求的补充语言字体映射")
	}
	if !reflect.DeepEqual(before.Theme.DirectRunFonts, after.Theme.DirectRunFonts) {
		return fmt.Errorf("主题动作改变了正文显式字体覆盖")
	}
	if before.Theme.NonFontDocument != after.Theme.NonFontDocument || before.Theme.NonFontTheme != after.Theme.NonFontTheme {
		return fmt.Errorf("主题动作产生非字体内容或属性变化")
	}
	if err := verifyDocumentThemeStyleFonts(before.Theme.StyleFonts, after.Theme.StyleFonts); err != nil {
		return err
	}
	if before.Theme.NonFontStyles != after.Theme.NonFontStyles {
		if e := verifyDocumentThemeStyleDefaults(before.Theme.StyleDefaults, after.Theme.StyleDefaults); e != nil {
			return e
		}
	}
	return documentThemeCharacterProof(before, after)
}

// Resolve the exported theme references, not the editor's reported font name.
// Theme references take precedence over fixed fallback names in OOXML.
func documentThemeEffectiveFonts(attrs map[string]string, fonts documentThemeFonts) (map[string]string, error) {
	out := map[string]string{}
	for slot, themeKey := range map[string]string{"ascii": "asciiTheme", "hAnsi": "hAnsiTheme", "eastAsia": "eastAsiaTheme", "cs": "cstheme"} {
		ref := attrs["fontFamily."+themeKey]
		if ref == "" {
			out[slot] = attrs["fontFamily."+slot]
			continue
		}
		var family documentThemeFamily
		switch {
		case strings.HasPrefix(ref, "major"):
			family = fonts.Major
		case strings.HasPrefix(ref, "minor"):
			family = fonts.Minor
		default:
			return nil, fmt.Errorf("未知主题字体引用 %s", ref)
		}
		switch strings.TrimPrefix(strings.TrimPrefix(ref, "major"), "minor") {
		case "Ascii", "HAnsi":
			out[slot] = family.Latin
		case "EastAsia":
			out[slot] = family.EA
		case "Bidi":
			out[slot] = family.CS
		default:
			return nil, fmt.Errorf("未知主题字体映射 %s", ref)
		}
	}
	return out, nil
}
func documentThemeCharacterProof(before, after documentLayoutSnapshot) error {
	if len(before.Text.Paragraphs) != len(after.Text.Paragraphs) {
		return fmt.Errorf("主题动作正文段落变化")
	}
	for i, p := range before.Text.Paragraphs {
		bc, ac := documentExpanded(p), documentExpanded(after.Text.Paragraphs[i])
		if len(bc) != len(ac) {
			return fmt.Errorf("主题动作正文长度变化")
		}
		for j, c := range bc {
			if c.Text != ac[j].Text {
				return fmt.Errorf("主题动作正文变化")
			}
			fonts, e := documentThemeEffectiveFonts(ac[j].Attrs, after.Theme.Fonts)
			if e != nil {
				return e
			}
			for slot, value := range fonts {
				if value == "" && ac[j].Attrs["fontFamily."+map[string]string{"ascii": "asciiTheme", "hAnsi": "hAnsiTheme", "eastAsia": "eastAsiaTheme", "cs": "cstheme"}[slot]] != "" {
					return fmt.Errorf("第%d段存在无有效字体的主题引用", i+1)
				}
			}
			for slot, key := range map[string]string{"ascii": "asciiTheme", "hAnsi": "hAnsiTheme", "eastAsia": "eastAsiaTheme", "cs": "cstheme"} {
				if ac[j].Attrs["fontFamily."+key] == "" && c.Attrs["fontFamily."+slot] != ac[j].Attrs["fontFamily."+slot] {
					return fmt.Errorf("第%d段不继承主题的字体意外变化", i+1)
				}
			}
			for key, value := range c.Attrs {
				if strings.HasPrefix(key, "fontFamily.") && strings.Contains(strings.ToLower(key), "theme") && value != "" && !documentThemeReferenceEqual(value, ac[j].Attrs[key]) {
					return fmt.Errorf("第%d段现有主题引用 %s 意外变化", i+1, key)
				}
			}
		}
	}
	return nil
}

// Both Latin references resolve to the same family's latin typeface in OOXML.
// Keep major/minor and all other script assignments distinct.
func documentThemeReferenceEqual(before, after string) bool {
	normalize := func(ref string) string {
		switch ref {
		case "majorHAnsi":
			return "majorAscii"
		case "minorHAnsi":
			return "minorAscii"
		}
		return ref
	}
	return normalize(before) == normalize(after)
}
