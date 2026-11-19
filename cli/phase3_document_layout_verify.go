package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type documentLayoutSpec struct {
	Theme             *documentThemeFonts `json:"themeFonts,omitempty"`
	ResetSupplemental bool                `json:"resetSupplemental,omitempty"`
	Action            string              `json:"action"`
	Paragraph         int                 `json:"paragraph,omitempty"`
	Text              string              `json:"text,omitempty"`
	Attrs             map[string]any      `json:"attrs,omitempty"`
	Margins           map[string]float64  `json:"marginsCm,omitempty"`
	Columns           int                 `json:"columns,omitempty"`
	Background        string              `json:"background,omitempty"`
}

func prepareDocumentLayout(action string, p map[string]any) (documentLayoutSpec, error) {
	s := documentLayoutSpec{Action: action, Attrs: map[string]any{}, Margins: map[string]float64{}}
	allowed := map[string]bool{}
	switch action {
	case "document-paragraph":
		for _, k := range []string{"paragraph", "text", "attrs"} {
			allowed[k] = true
		}
		n, ok := p["paragraph"].(float64)
		if !ok || n < 1 || n > 100000 || math.Trunc(n) != n {
			return s, fmt.Errorf("paragraph 必须是从1开始的整数")
		}
		s.Paragraph = int(n)
		s.Text, ok = p["text"].(string)
		if !ok || s.Text == "" || strings.ContainsAny(s.Text, "\r\n") {
			return s, fmt.Errorf("text 必须为精确原段落")
		}
		attrs, ok := p["attrs"].(map[string]any)
		if !ok || len(attrs) == 0 {
			return s, fmt.Errorf("attrs 不得为空")
		}
		for k, v := range attrs {
			switch k {
			case "heading":
				n, ok := v.(float64)
				if !ok || n < 1 || n > 6 || math.Trunc(n) != n {
					return s, fmt.Errorf("heading 仅支持1–6")
				}
				s.Attrs[k] = n
			case "firstLineChars":
				n, ok := v.(float64)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 20 || math.Abs(n*100-math.Round(n*100)) > 1e-9 {
					return s, fmt.Errorf("firstLineChars 必须为0–20，精度0.01字符")
				}
				s.Attrs[k] = n
			case "lineMultiple":
				n, ok := v.(float64)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0.5 || n > 10 {
					return s, fmt.Errorf("lineMultiple 必须为0.5–10")
				}
				s.Attrs[k] = n
			case "shading":
				c, e := layoutColor(v)
				if e != nil {
					return s, e
				}
				s.Attrs[k] = c
			case "numbering":
				if v != "decimal" {
					return s, fmt.Errorf("numbering 仅支持decimal")
				}
				s.Attrs[k] = v
			default:
				return s, fmt.Errorf("未知段落格式 %s", k)
			}
		}
	case "document-theme":
		allowed["themeFonts"], allowed["resetSupplemental"] = true, true
		var err error
		s.Theme, err = prepareDocumentTheme(p)
		if err != nil {
			return s, err
		}
		s.ResetSupplemental = true
	case "document-page":
		for _, k := range []string{"marginsCm", "columns", "background"} {
			allowed[k] = true
		}
		if v, exists := p["marginsCm"]; exists {
			m, ok := v.(map[string]any)
			if !ok || len(m) != 4 {
				return s, fmt.Errorf("marginsCm 必须明确四边")
			}
			for _, k := range []string{"left", "top", "right", "bottom"} {
				n, ok := m[k].(float64)
				if !ok || n < 0 || n > 10 || math.IsNaN(n) || math.IsInf(n, 0) {
					return s, fmt.Errorf("页边距 %s 无效", k)
				}
				s.Margins[k] = n
			}
		}
		if v, exists := p["columns"]; exists {
			n, ok := v.(float64)
			if !ok || n < 1 || n > 4 || math.Trunc(n) != n {
				return s, fmt.Errorf("columns 必须为1–4")
			}
			s.Columns = int(n)
		}
		if v, exists := p["background"]; exists {
			c, e := layoutColor(v)
			if e != nil {
				return s, e
			}
			s.Background = c
		}
		if len(s.Margins) == 0 && s.Columns == 0 && s.Background == "" {
			return s, fmt.Errorf("缺少页面设置")
		}
	default:
		return s, fmt.Errorf("不支持的文稿布局动作")
	}
	for k := range p {
		if !allowed[k] {
			return s, fmt.Errorf("未知参数 %s", k)
		}
	}
	return s, nil
}
func layoutColor(v any) (string, error) {
	s, ok := v.(string)
	if !ok || len(s) != 7 || s[0] != '#' {
		return "", fmt.Errorf("颜色必须为 #RRGGBB")
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return "", fmt.Errorf("颜色必须为 #RRGGBB")
		}
	}
	if _, e := strconv.ParseUint(s[1:], 16, 24); e != nil {
		return "", fmt.Errorf("颜色必须为 #RRGGBB")
	}
	return strings.ToUpper(s), nil
}

type documentLayoutParagraph struct {
	NumberID     string            `json:"numberId,omitempty"`
	NumberLevel  string            `json:"numberLevel,omitempty"`
	NumberStart  string            `json:"numberStart,omitempty"`
	Text         string            `json:"text"`
	Props        map[string]string `json:"props"`
	StyleName    string            `json:"styleName,omitempty"`
	NumberFormat string            `json:"numberFormat,omitempty"`
	Values       map[string]string `json:"values"`
}
type documentLayoutSnapshot struct {
	StandardNoteDefaults  map[string]bool           `json:"standardNoteDefaults"`
	SectionFirstParagraph []int                     `json:"sectionFirstParagraph"`
	Theme                 *documentThemeSnapshot    `json:"theme,omitempty"`
	Paragraphs            []documentLayoutParagraph `json:"paragraphs"`
	Sections              []map[string]string       `json:"sections"`
	SectionValues         []map[string]string       `json:"sectionValues"`
	Background            string                    `json:"background,omitempty"`
	Text                  documentSnapshot          `json:"text"`
}

func layoutNodeCanonical(n *artifactNode) string {
	if n == nil {
		return ""
	}
	attrs := []string{}
	for _, a := range n.Attr {
		attrs = append(attrs, a.Name.Space+":"+a.Name.Local+"="+a.Value)
	}
	sort.Strings(attrs)
	children := []string{}
	for _, c := range n.Children {
		children = append(children, layoutNodeCanonical(c))
	}
	b, _ := json.Marshal([]any{n.Name.Space, n.Name.Local, attrs, n.Text, children})
	return string(b)
}
func layoutNodeProperties(n *artifactNode) (map[string]string, map[string]string) {
	props, values := map[string]string{}, map[string]string{}
	if n == nil {
		return props, values
	}
	for _, c := range n.Children {
		props[c.Name.Local] = layoutNodeCanonical(c)
		for _, a := range c.Attr {
			if a.Name.Space == artifactWordNS {
				values[c.Name.Local+"."+a.Name.Local] = a.Value
			}
		}
	}
	return props, values
}
func readDocumentLayout(filename string) (documentLayoutSnapshot, error) {
	result := documentLayoutSnapshot{}
	text, e := readDocumentSnapshot(filename)
	if e != nil {
		return result, e
	}
	result.Text = text
	z, parts, e := openArtifact(filename)
	if e != nil {
		return result, e
	}
	defer z.Close()
	root, e := artifactPart(parts, "word/document.xml", artifactWordNS, "document")
	if e != nil {
		return result, e
	}
	result.StandardNoteDefaults = map[string]bool{"footnotePr": true, "endnotePr": true}
	if parts["word/settings.xml"] != nil {
		settings, err := artifactPart(parts, "word/settings.xml", artifactWordNS, "settings")
		if err != nil {
			return result, err
		}
		for name, expected := range map[string]map[string]string{"footnotePr": {"pos": "pageBottom", "numFmt": "decimal", "numRestart": "continuous"}, "endnotePr": {"pos": "docEnd", "numFmt": "lowerRoman", "numRestart": "continuous"}} {
			if note := settings.child(name); note != nil {
				for key, value := range expected {
					if node := note.child(key); node != nil && (documentAttr(node, "val") != value || len(node.Attr) != 1) {
						result.StandardNoteDefaults[name] = false
					}
				}
			}
		}
	}
	result.Theme, e = readDocumentTheme(parts, root)
	if e != nil {
		return result, e
	}
	names := map[string]string{}
	if parts["word/styles.xml"] != nil {
		styles, e := artifactPart(parts, "word/styles.xml", artifactWordNS, "styles")
		if e != nil {
			return result, e
		}
		for _, s := range styles.children("style") {
			names[documentAttr(s, "styleId")] = documentAttr(s.child("name"), "val")
		}
	}
	nums := map[string]string{}
	starts := map[string]string{}
	if parts["word/numbering.xml"] != nil {
		numbering, e := artifactPart(parts, "word/numbering.xml", artifactWordNS, "numbering")
		if e != nil {
			return result, e
		}
		abstract := map[string]map[string]string{}
		abstractStarts := map[string]map[string]string{}
		for _, a := range numbering.children("abstractNum") {
			levels := map[string]string{}
			levelStarts := map[string]string{}
			for _, l := range a.children("lvl") {
				levels[documentAttr(l, "ilvl")] = documentAttr(l.child("numFmt"), "val")
				levelStarts[documentAttr(l, "ilvl")] = documentAttr(l.child("start"), "val")
			}
			abstract[documentAttr(a, "abstractNumId")] = levels
			abstractStarts[documentAttr(a, "abstractNumId")] = levelStarts
		}
		for _, n := range numbering.children("num") {
			id := documentAttr(n, "numId")
			ref := documentAttr(n.child("abstractNumId"), "val")
			for level, format := range abstract[ref] {
				nums[id+":"+level] = format
				starts[id+":"+level] = abstractStarts[ref][level]
			}
			for _, override := range n.children("lvlOverride") {
				if start := documentAttr(override.child("startOverride"), "val"); start != "" {
					starts[id+":"+documentAttr(override, "ilvl")] = start
				}
				if level := override.child("lvl"); level != nil {
					nums[id+":"+documentAttr(override, "ilvl")] = documentAttr(level.child("numFmt"), "val")
					if start := documentAttr(level.child("start"), "val"); start != "" && override.child("startOverride") == nil {
						starts[id+":"+documentAttr(override, "ilvl")] = start
					}
				}
			}
		}
	}
	sectionStart := 1
	for i, p := range documentChildren(root, "p") {
		if i >= len(text.Paragraphs) {
			return result, fmt.Errorf("段落结构不一致")
		}
		if pp := p.child("pPr"); pp != nil && pp.child("sectPr") != nil {
			result.SectionFirstParagraph = append(result.SectionFirstParagraph, sectionStart)
			sectionStart = i + 2
		}
		props, values := layoutNodeProperties(p.child("pPr"))
		np := p.child("pPr")
		format, numberID, numberLevel, numberStart := "", "", "", ""
		if np != nil && np.child("numPr") != nil {
			n := np.child("numPr")
			level := documentAttr(n.child("ilvl"), "val")
			if level == "" {
				level = "0"
			}
			numberID, numberLevel = documentAttr(n.child("numId"), "val"), level
			format = nums[numberID+":"+level]
			numberStart = starts[numberID+":"+level]
		}
		result.Paragraphs = append(result.Paragraphs, documentLayoutParagraph{NumberID: numberID, NumberLevel: numberLevel, NumberStart: numberStart, Text: text.Paragraphs[i].Text, Props: props, Values: values, StyleName: names[values["pStyle.val"]], NumberFormat: format})
	}
	if body := root.child("body"); body != nil && body.child("sectPr") != nil {
		result.SectionFirstParagraph = append(result.SectionFirstParagraph, sectionStart)
	}
	for _, s := range documentChildren(root, "sectPr") {
		p, v := layoutNodeProperties(s)
		result.Sections = append(result.Sections, p)
		result.SectionValues = append(result.SectionValues, v)
	}
	if b := root.child("background"); b != nil {
		result.Background = strings.ToUpper(documentAttr(b, "color"))
	}
	return result, nil
}
func layoutEqualExcept(before, after map[string]string, allowed map[string]bool) bool {
	a, b := map[string]string{}, map[string]string{}
	for k, v := range before {
		if !allowed[k] {
			a[k] = v
		}
	}
	for k, v := range after {
		if !allowed[k] {
			b[k] = v
		}
	}
	return reflect.DeepEqual(a, b)
}
func verifyDocumentLayout(before, after documentLayoutSnapshot, s documentLayoutSpec) error {
	if s.Action == "document-theme" {
		return verifyDocumentTheme(before, after, s)
	}
	if len(before.Paragraphs) != len(after.Paragraphs) || len(before.Sections) != len(after.Sections) {
		return fmt.Errorf("非预期段落/分节数量变化")
	}
	if !reflect.DeepEqual(before.SectionFirstParagraph, after.SectionFirstParagraph) {
		return fmt.Errorf("分节边界位置变化")
	}
	for i, p := range before.Paragraphs {
		q := after.Paragraphs[i]
		if p.Text != q.Text {
			return fmt.Errorf("第%d段正文被意外修改", i+1)
		}
		if s.Action != "document-paragraph" || i != s.Paragraph-1 {
			if p.NumberFormat != q.NumberFormat || p.NumberStart != q.NumberStart {
				return fmt.Errorf("非目标第%d段有效编号定义变化", i+1)
			}
			if !reflect.DeepEqual(p.Props, q.Props) {
				return fmt.Errorf("非目标第%d段格式变化", i+1)
			}
			if i < len(before.Text.Paragraphs) && i < len(after.Text.Paragraphs) && !reflect.DeepEqual(documentExpanded(before.Text.Paragraphs[i]), documentExpanded(after.Text.Paragraphs[i])) {
				return fmt.Errorf("非目标第%d段字符格式变化", i+1)
			}
		}
	}
	if s.Action == "document-paragraph" {
		if s.Paragraph < 1 || s.Paragraph > len(after.Paragraphs) || before.Paragraphs[s.Paragraph-1].Text != s.Text {
			return fmt.Errorf("原段落不匹配")
		}
		p := after.Paragraphs[s.Paragraph-1]
		allowed := map[string]bool{}
		allowedValues := map[string]bool{}
		for k, v := range s.Attrs {
			switch k {
			case "heading":
				allowed["pStyle"] = true
				allowedValues["pStyle.val"] = true
				want := fmt.Sprintf("heading%d", int(v.(float64)))
				got := strings.ToLower(strings.ReplaceAll(p.StyleName, " ", ""))
				if got != want {
					return fmt.Errorf("标题样式未持久化：%s", p.StyleName)
				}
			case "firstLineChars":
				allowed["ind"] = true
				for _, key := range []string{"ind.firstLineChars", "ind.firstLine", "ind.hanging", "ind.hangingChars"} {
					allowedValues[key] = true
				}
				want := int(math.Round(v.(float64) * 100))
				got, e := strconv.Atoi(p.Values["ind.firstLineChars"])
				if e != nil || got != want {
					return fmt.Errorf("首行字符缩进未持久化")
				}
			case "lineMultiple":
				allowed["spacing"] = true
				allowedValues["spacing.line"], allowedValues["spacing.lineRule"] = true, true
				got, e := strconv.ParseFloat(p.Values["spacing.line"], 64)
				if e != nil || math.Abs(got-v.(float64)*240) > 0.51 || (p.Values["spacing.lineRule"] != "auto" && p.Values["spacing.lineRule"] != "") {
					return fmt.Errorf("倍数行距未持久化")
				}
			case "shading":
				allowed["shd"] = true
				for key := range p.Values {
					if strings.HasPrefix(key, "shd.") {
						allowedValues[key] = true
					}
				}
				for key := range before.Paragraphs[s.Paragraph-1].Values {
					if strings.HasPrefix(key, "shd.") {
						allowedValues[key] = true
					}
				}
				if strings.ToUpper(p.Values["shd.fill"]) != strings.TrimPrefix(v.(string), "#") {
					return fmt.Errorf("段落底纹未持久化")
				}
			case "numbering":
				allowed["numPr"] = true
				// The native numbering command applies its List Paragraph
				// style and clears first-line character indent. Preserve all
				// other indentation and accept only this observed exact form.
				if strings.EqualFold(strings.ReplaceAll(p.StyleName, " ", ""), "ListParagraph") {
					allowed["pStyle"] = true
					allowedValues["pStyle.val"] = true
				}
				if p.Values["ind.firstLineChars"] == "0" {
					allowed["ind"] = true
					allowedValues["ind.firstLineChars"] = true
				}
				if p.NumberFormat != "decimal" {
					return fmt.Errorf("未引用有效decimal编号定义")
				}
			}
		}
		if !layoutEqualExcept(before.Paragraphs[s.Paragraph-1].Props, p.Props, allowed) {
			return fmt.Errorf("目标段落其他属性意外变化")
		}
		if !layoutEqualExcept(before.Paragraphs[s.Paragraph-1].Values, p.Values, allowedValues) {
			return fmt.Errorf("目标段落未请求的细项属性意外变化")
		}
		if _, heading := s.Attrs["heading"]; !heading && s.Paragraph <= len(before.Text.Paragraphs) && s.Paragraph <= len(after.Text.Paragraphs) && !reflect.DeepEqual(documentExpanded(before.Text.Paragraphs[s.Paragraph-1]), documentExpanded(after.Text.Paragraphs[s.Paragraph-1])) {
			return fmt.Errorf("段落布局意外改变字符格式")
		}
		if !reflect.DeepEqual(before.Sections, after.Sections) || before.Background != after.Background {
			return fmt.Errorf("段落动作意外改变页面设置")
		}
	} else {
		if len(after.Sections) != 1 {
			return fmt.Errorf("当前页面动作仅支持单节文稿")
		}
		section := 0
		v := after.SectionValues[section]
		allowed := map[string]bool{}
		if len(s.Margins) > 0 {
			allowed["pgMar"] = true
			for k, cm := range s.Margins {
				got, e := strconv.ParseFloat(v["pgMar."+k], 64)
				if e != nil || math.Abs(got-cm*1440/2.54) > 1 {
					return fmt.Errorf("%s 页边距与厘米请求不符", k)
				}
			}
			for _, key := range []string{"pgMar.header", "pgMar.footer", "pgMar.gutter"} {
				if before.SectionValues[section][key] != v[key] {
					return fmt.Errorf("未请求的页面边距属性 %s 变化", key)
				}
			}
		}
		if s.Columns > 0 {
			allowed["cols"] = true
			if v["cols.equalWidth"] != "" && v["cols.equalWidth"] != "1" && v["cols.equalWidth"] != "true" {
				return fmt.Errorf("分栏并非请求的等宽布局")
			}
			for _, key := range []string{"cols.space", "cols.sep"} {
				if before.SectionValues[section][key] != v[key] {
					return fmt.Errorf("未请求的分栏属性 %s 变化", key)
				}
			}
			got := v["cols.num"]
			if got == "" {
				got = "1"
			}
			if got != strconv.Itoa(s.Columns) {
				return fmt.Errorf("分栏数量未持久化")
			}
		}
		// Page settings serialize previously implicit standard note defaults.
		// Only accept these exact default nodes when they were absent before.
		for key, node := range documentLayoutDefaultNotes() {
			if before.Sections[section][key] == "" && after.Sections[section][key] == node && before.StandardNoteDefaults[key] {
				allowed[key] = true
			}
		}
		if !layoutEqualExcept(before.Sections[section], after.Sections[section], allowed) {
			return fmt.Errorf("非目标页面属性变化")
		}
		if s.Background != "" {
			if after.Background != strings.TrimPrefix(s.Background, "#") {
				return fmt.Errorf("页面背景颜色未持久化")
			}
		} else if before.Background != after.Background {
			return fmt.Errorf("页面背景意外变化")
		}
	}
	return nil
}

func documentLayoutDefaultNotes() map[string]string {
	out := map[string]string{}
	for name, attrs := range map[string][2]string{"footnotePr": {"pageBottom", "decimal"}, "endnotePr": {"docEnd", "lowerRoman"}} {
		raw := `<w:` + name + ` xmlns:w="` + artifactWordNS + `"><w:pos w:val="` + attrs[0] + `"/><w:numFmt w:val="` + attrs[1] + `"/><w:numRestart w:val="continuous"/></w:` + name + `>`
		n, e := parseArtifactXML([]byte(raw))
		if e == nil {
			n.Attr = nil
			out[name] = layoutNodeCanonical(n)
		}
	}
	return out
}
