package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type documentFormatSpec struct {
	Paragraph int            `json:"paragraph"`
	Text      string         `json:"text"`
	Match     string         `json:"match,omitempty"`
	Start     int            `json:"start"`
	End       int            `json:"end"`
	Attrs     map[string]any `json:"attrs"`
}

func prepareDocumentFormat(payload map[string]any) (documentFormatSpec, error) {
	s := documentFormatSpec{Attrs: map[string]any{}}
	for k := range payload {
		if k != "paragraph" && k != "text" && k != "match" && k != "attrs" && k != "offset" && k != "length" {
			return s, fmt.Errorf("document-format 未知参数 %s", k)
		}
	}
	n, ok := payload["paragraph"].(float64)
	if !ok || n < 1 || n > 100000 || n != math.Trunc(n) {
		return s, fmt.Errorf("paragraph 必须为从 1 开始的段落序号")
	}
	s.Paragraph = int(n)
	s.Text, ok = payload["text"].(string)
	if !ok || s.Text == "" || strings.ContainsAny(s.Text, "\r\n") || utf8.RuneCountInString(s.Text) > 100000 {
		return s, fmt.Errorf("text 必须精确指定非空原段落，不含换行")
	}
	if v, exists := payload["match"]; exists {
		s.Match, ok = v.(string)
		if !ok || s.Match == "" || strings.Count(s.Text, s.Match) != 1 {
			return s, fmt.Errorf("match 必须在原段落中恰好出现一次")
		}
	}
	s.End = utf8.RuneCountInString(s.Text)
	if s.Match != "" {
		i := strings.Index(s.Text, s.Match)
		s.Start = utf8.RuneCountInString(s.Text[:i])
		s.End = s.Start + utf8.RuneCountInString(s.Match)
	}
	if off, has := payload["offset"]; has || payload["length"] != nil {
		offset, ok1 := off.(float64)
		length, ok2 := payload["length"].(float64)
		if !ok1 || !ok2 || offset < 0 || length < 1 || offset != math.Trunc(offset) || length != math.Trunc(length) || offset+length > float64(s.End-s.Start) {
			return s, fmt.Errorf("offset/length 必须为匹配文本内的有效 Unicode 字符区间")
		}
		s.Start += int(offset)
		s.End = s.Start + int(length)
	}
	attrs, ok := payload["attrs"].(map[string]any)
	if !ok || len(attrs) == 0 {
		return s, fmt.Errorf("attrs 必须是非空格式对象")
	}
	for k, v := range attrs {
		switch k {
		case "fontFamily":
			x, ok := v.(string)
			if !ok || strings.TrimSpace(x) == "" || len(x) > 200 || strings.ContainsAny(x, "\r\n\x00") {
				return s, fmt.Errorf("fontFamily 无效")
			}
			s.Attrs[k] = x
		case "fontSize":
			x, ok := v.(float64)
			if !ok || math.IsNaN(x) || math.IsInf(x, 0) || x < 1 || x > 400 || math.Trunc(x*2) != x*2 {
				return s, fmt.Errorf("fontSize 必须为 1–400 磅且最小单位 0.5")
			}
			s.Attrs[k] = x
		case "color":
			x, ok := v.(string)
			if !ok || !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(x) {
				return s, fmt.Errorf("color 必须为 #RRGGBB")
			}
			s.Attrs[k] = strings.ToUpper(x)
		case "verticalAlign":
			x, ok := v.(string)
			if !ok || (x != "baseline" && x != "superscript" && x != "subscript") {
				return s, fmt.Errorf("verticalAlign 必须为 baseline/superscript/subscript")
			}
			s.Attrs[k] = x
		case "bold", "italic", "underline", "strike":
			x, ok := v.(bool)
			if !ok {
				return s, fmt.Errorf("%s 必须为 bool", k)
			}
			s.Attrs[k] = x
		default:
			return s, fmt.Errorf("document-format 不支持格式 %s", k)
		}
	}
	return s, nil
}

type documentChar struct {
	Text  string            `json:"text"`
	Attrs map[string]string `json:"attrs"`
}
type documentParagraph struct {
	Structure  []documentStructureToken `json:"structure,omitempty"`
	Text       string                   `json:"text"`
	Chars      []documentChar           `json:"runs"`
	Properties string                   `json:"paragraphProperties,omitempty"`
	MarkAttrs  map[string]string        `json:"paragraphMark,omitempty"`
}
type documentSnapshot struct {
	Structure      string              `json:"structure,omitempty"`
	Assets         map[string]string   `json:"assets,omitempty"`
	StyleResources map[string]string   `json:"styleResources,omitempty"`
	Paragraphs     []documentParagraph `json:"paragraphs"`
}

func documentAttr(n *artifactNode, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Name.Space == artifactWordNS && a.Name.Local == key {
			return a.Value
		}
	}
	return ""
}
func documentChildren(n *artifactNode, name string) []*artifactNode {
	var out []*artifactNode
	var visit func(*artifactNode)
	visit = func(v *artifactNode) {
		if v.Name.Space == artifactWordNS && v.Name.Local == name {
			out = append(out, v)
			return
		}
		for _, c := range v.Children {
			visit(c)
		}
	}
	visit(n)
	return out
}
func documentCopyAttrs(src map[string]string) map[string]string {
	m := map[string]string{}
	for k, v := range src {
		m[k] = v
	}
	return m
}
func documentRunAttrs(base map[string]string, p *artifactNode) map[string]string {
	m := documentCopyAttrs(base)
	if p == nil {
		return m
	}
	for _, n := range p.Children {
		if n.Name.Space != artifactWordNS {
			m["property."+n.Name.Space+"."+n.Name.Local] = documentNodeKey(n)
			continue
		}
		v := documentAttr(n, "val")
		switch n.Name.Local {
		case "rFonts":
			// A nearer declaration replaces the inherited font choice for that
			// script slot. Same-level literal/theme pairs retain theme precedence.
			for _, pair := range [][2]string{{"ascii", "asciiTheme"}, {"hAnsi", "hAnsiTheme"}, {"eastAsia", "eastAsiaTheme"}, {"cs", "cstheme"}} {
				literal, theme := documentAttr(n, pair[0]), documentAttr(n, pair[1])
				if literal != "" || theme != "" {
					delete(m, "fontFamily."+pair[0])
					delete(m, "fontFamily."+pair[1])
				}
				if literal != "" {
					m["fontFamily."+pair[0]] = literal
				}
				if theme != "" {
					m["fontFamily."+pair[1]] = theme
				}
			}
		case "sz", "szCs":
			if f, e := strconv.ParseFloat(v, 64); e == nil {
				key := "fontSize"
				if n.Name.Local == "szCs" {
					key = "fontSizeCS"
				}
				m[key] = strconv.FormatFloat(f/2, 'f', -1, 64)
			}
		case "color":
			if documentAttr(n, "themeColor") == "" {
				m["color"] = "#" + strings.ToUpper(v)
			} else {
				m["color"] = "theme:" + documentAttr(n, "themeColor")
			}
		case "vertAlign":
			m["verticalAlign"] = v
		case "b", "i", "u", "strike":
			key := map[string]string{"b": "bold", "i": "italic", "u": "underline", "strike": "strike"}[n.Name.Local]
			m[key] = strconv.FormatBool(v != "0" && v != "false" && v != "off" && v != "none")
			if n.Name.Local == "u" {
				style := v
				if style == "" || style == "1" || style == "true" || style == "on" {
					style = "single"
				}
				if m[key] == "false" {
					style = "none"
				}
				m["underlineStyle"] = style
			}
		default:
			m["property."+n.Name.Space+"."+n.Name.Local] = documentNodeKey(n)
		}
	}
	return m
}

func readDocumentSnapshot(filename string) (documentSnapshot, error) {
	var result documentSnapshot
	graphic, err := readDocumentGraphic(filename)
	if err != nil {
		return result, err
	}
	result.Assets = graphic.Assets
	result.StyleResources = graphic.Resources
	z, entries, err := openArtifact(filename)
	if err != nil {
		return result, err
	}
	defer z.Close()
	root, err := artifactPart(entries, "word/document.xml", artifactWordNS, "document")
	if err != nil {
		return result, err
	}
	if err = validateDocumentStructure(root); err != nil {
		return result, err
	}
	result.Structure = documentResourceKey(documentStructureSkeleton(root))
	base := map[string]string{}
	styles := map[string]*artifactNode{}
	defaultParagraphStyle := ""
	if entries["word/styles.xml"] != nil {
		r, e := artifactPart(entries, "word/styles.xml", artifactWordNS, "styles")
		if e != nil {
			return result, e
		}
		if d := r.child("docDefaults"); d != nil {
			if p := d.child("rPrDefault"); p != nil {
				base = documentRunAttrs(base, p.child("rPr"))
			}
		}
		for _, s := range r.children("style") {
			styles[documentAttr(s, "styleId")] = s
			if documentAttr(s, "type") == "paragraph" && (documentAttr(s, "default") == "1" || documentAttr(s, "default") == "true") {
				defaultParagraphStyle = documentAttr(s, "styleId")
			}
		}
	}
	var applyStyle func(map[string]string, string, map[string]bool) (map[string]string, error)
	applyStyle = func(m map[string]string, id string, seen map[string]bool) (map[string]string, error) {
		if id == "" {
			return m, nil
		}
		if seen[id] {
			return nil, fmt.Errorf("文稿样式循环 %s", id)
		}
		s := styles[id]
		if s == nil {
			return nil, fmt.Errorf("文稿引用不存在的样式 %s", id)
		}
		seen[id] = true
		var e error
		m, e = applyStyle(m, documentAttr(s.child("basedOn"), "val"), seen)
		if e != nil {
			return nil, e
		}
		return documentRunAttrs(m, s.child("rPr")), nil
	}
	body := root.child("body")
	if body == nil {
		return result, fmt.Errorf("文稿无正文")
	}
	spaceModes := documentParagraphSpaceModes(root)
	for _, p := range documentChildren(body, "p") {
		para := documentParagraph{Structure: documentParagraphStructure(p, spaceModes[p])}
		if pp := p.child("pPr"); pp != nil {
			clone := *pp
			clone.Children = nil
			for _, c := range pp.Children {
				if c.Name.Space == artifactWordNS && c.Name.Local == "rPr" {
					para.MarkAttrs = documentRunAttrs(nil, c)
				} else {
					clone.Children = append(clone.Children, c)
				}
			}
			para.Properties = documentNodeKey(&clone)
		}
		defaults := documentCopyAttrs(base)
		paragraphStyle := defaultParagraphStyle
		if pp := p.child("pPr"); pp != nil {
			if id := documentAttr(pp.child("pStyle"), "val"); id != "" {
				paragraphStyle = id
			}
		}
		defaults, err = applyStyle(defaults, paragraphStyle, map[string]bool{})
		if err != nil {
			return result, err
		}
		for _, r := range documentChildren(p, "r") {
			rp := r.child("rPr")
			attrs := documentCopyAttrs(defaults)
			if rp != nil {
				attrs, err = applyStyle(attrs, documentAttr(rp.child("rStyle"), "val"), map[string]bool{})
				if err != nil {
					return result, err
				}
			}
			attrs = documentRunAttrs(attrs, rp)
			var text strings.Builder
			for _, c := range r.Children {
				if c.Name.Space != artifactWordNS {
					continue
				}
				switch c.Name.Local {
				case "t":
					text.WriteString(c.Text)
				case "tab":
					text.WriteByte('\t')
				case "br", "cr":
					text.WriteByte('\n')
				}
			}
			if text.Len() > 0 {
				para.Text += text.String()
				para.Chars = append(para.Chars, documentChar{Text: text.String(), Attrs: attrs})
			}
		}
		result.Paragraphs = append(result.Paragraphs, para)
	}
	return result, nil
}

func documentExpanded(p documentParagraph) []documentChar {
	var out []documentChar
	for _, run := range p.Chars {
		for _, r := range run.Text {
			out = append(out, documentChar{Text: string(r), Attrs: run.Attrs})
		}
	}
	return out
}
func documentValue(attrs map[string]string, key string) string {
	if v, ok := attrs[key]; ok {
		return v
	}
	switch key {
	case "bold", "italic", "underline", "strike":
		return "false"
	case "verticalAlign":
		return "baseline"
	case "underlineStyle":
		if documentValue(attrs, "underline") == "true" {
			return "single"
		}
		return "none"
	}
	return ""
}
func verifyDocumentFormat(before, after documentSnapshot, s documentFormatSpec) error {
	if err := verifyDocumentStructure(before, after); err != nil {
		return err
	}
	return verifyDocumentCharacterFormat(before, after, s)
}

func verifyDocumentCharacterFormat(before, after documentSnapshot, s documentFormatSpec) error {
	if err := verifyDocumentResources(before.Assets, after.Assets); err != nil {
		return fmt.Errorf("请求外的文稿包资源改变: %w", err)
	}
	if err := verifyDocumentResources(before.StyleResources, after.StyleResources); err != nil {
		return err
	}
	if len(before.Paragraphs) != len(after.Paragraphs) || s.Paragraph > len(before.Paragraphs) {
		return fmt.Errorf("段落数改变或目标不存在")
	}
	if before.Paragraphs[s.Paragraph-1].Text != s.Text {
		return fmt.Errorf("原段落不匹配请求 text")
	}
	for i, p := range before.Paragraphs {
		a := after.Paragraphs[i]
		if p.Properties != a.Properties {
			return fmt.Errorf("第 %d 段出现请求外段落格式变化", i+1)
		}
		for k := range documentAttrKeys(p.MarkAttrs, a.MarkAttrs) {
			allow := i == s.Paragraph-1 && s.Start == 0 && s.End == utf8.RuneCountInString(s.Text) && documentRequestedAttr(s, k)
			if !allow && documentValue(p.MarkAttrs, k) != documentValue(a.MarkAttrs, k) {
				return fmt.Errorf("第 %d 段段落标记格式意外改变 %s", i+1, k)
			}
		}
		if p.Text != a.Text {
			return fmt.Errorf("第 %d 段正文改变", i+1)
		}
		bc, ac := documentExpanded(p), documentExpanded(a)
		if len(bc) != len(ac) {
			return fmt.Errorf("字符读回不完整")
		}
		for j, c := range ac {
			target := i == s.Paragraph-1 && j >= s.Start && j < s.End
			all := map[string]bool{}
			for k := range bc[j].Attrs {
				all[k] = true
			}
			for k := range c.Attrs {
				all[k] = true
			}
			for k := range all {
				requested := documentRequestedAttr(s, k)
				if target && requested {
					continue
				}
				if documentValue(bc[j].Attrs, k) != documentValue(c.Attrs, k) {
					return fmt.Errorf("第 %d 段第 %d 字符出现请求外格式变化 %s", i+1, j+1, k)
				}
			}
			if !target {
				continue
			}
			for k, w := range s.Attrs {
				if k == "fontFamily" {
					matched := false
					for key, value := range c.Attrs {
						if strings.HasPrefix(key, "fontFamily.") && strings.Contains(strings.ToLower(key), "theme") && value != "" {
							return fmt.Errorf("目标字符含未解析主题字体，不声称字体已验证")
						}
					}
					for _, f := range []string{"ascii", "hAnsi", "eastAsia", "cs"} {
						v := c.Attrs["fontFamily."+f]
						if v != "" {
							if v != w {
								return fmt.Errorf("目标字体 %s=%s，不符请求 %v", f, v, w)
							}
							matched = true
						}
					}
					if !matched {
						return fmt.Errorf("目标字符缺少可验证字体")
					}
					continue
				}
				want := fmt.Sprint(w)
				if k == "underline" {
					style := "none"
					if w == true {
						style = "single"
					}
					if documentValue(c.Attrs, "underlineStyle") != style {
						return fmt.Errorf("目标下划线样式与请求不符")
					}
				}
				if k == "fontSize" && c.Attrs["fontSizeCS"] != "" && c.Attrs["fontSizeCS"] != want {
					return fmt.Errorf("目标复杂文字字号与请求不符")
				}
				if documentValue(c.Attrs, k) != want {
					return fmt.Errorf("第 %d 段第 %d 字符 %s=%s，不符请求 %s", i+1, j+1, k, documentValue(c.Attrs, k), want)
				}
			}
		}
	}
	return nil
}

func documentRequestedAttr(s documentFormatSpec, k string) bool {
	return s.Attrs[k] != nil || (k == "underlineStyle" && s.Attrs["underline"] != nil) || (strings.HasPrefix(k, "fontFamily.") && s.Attrs["fontFamily"] != nil) || (k == "fontSizeCS" && s.Attrs["fontSize"] != nil)
}
func documentAttrKeys(a, b map[string]string) map[string]bool {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	return keys
}
func documentNodeKey(n *artifactNode) string {
	if n == nil {
		return ""
	}
	attrs := make([]string, 0, len(n.Attr))
	for _, a := range n.Attr {
		attrs = append(attrs, a.Name.Space+"|"+a.Name.Local+"="+a.Value)
	}
	sort.Strings(attrs)
	children := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		children = append(children, documentNodeKey(c))
	}
	data, _ := json.Marshal([]any{n.Name.Space, n.Name.Local, attrs, strings.TrimSpace(n.Text), children})
	return string(data)
}
