package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"
)

type phase3SlidesSpec struct {
	Media       *phase3SlideMediaSpec       `json:"media,omitempty"`
	Theme       *phase3SlideThemeSpec       `json:"theme,omitempty"`
	Show        *phase3SlideShowSpec        `json:"show,omitempty"`
	Animation   *phase3SlideAnimationSpec   `json:"animation,omitempty"`
	Table       *phase3SlideTableSpec       `json:"table,omitempty"`
	Placeholder *phase3SlidePlaceholderSpec `json:"placeholder,omitempty"`
	Shape       *phase3SlideShapeSpec       `json:"shape,omitempty"`
	Replace     *phase3SlideReplaceSpec     `json:"replace,omitempty"`
	Format      *phase3SlideFormatSpec      `json:"textFormat,omitempty"`
	Action      string                      `json:"action"`
	SlideID     string                      `json:"slideId"`
	Text        *phase3SlideTextSpec        `json:"textBox,omitempty"`
	Layout      *phase3SlideLayoutSpec      `json:"layout,omitempty"`
}
type phase3SlidePage struct {
	FixedObjects []string `json:"nonPlaceholderObjectSHA256,omitempty"`
	Resources    string   `json:"resourcesSHA256,omitempty"`
	Notes        string   `json:"notesSHA256,omitempty"`
	ID           string   `json:"id"`
	Content      string   `json:"contentSHA256"`
	Hidden       bool     `json:"hidden"`
	Layout       string   `json:"layoutSHA256,omitempty"`
	Texts        []string `json:"texts,omitempty"`
}

func preparePhase3Slides(action string, payload map[string]any) (phase3SlidesSpec, error) {
	s := phase3SlidesSpec{Action: action}
	if action == "slide-theme" {
		var e error
		s.Theme, e = preparePhase3SlideTheme(payload)
		return s, e
	}
	if action == "slide-show-settings" {
		var e error
		s.Show, e = preparePhase3SlideShow(payload)
		return s, e
	}
	if action == "slide-title-animate" {
		var e error
		s.Animation, e = preparePhase3SlideAnimation(payload)
		if e == nil {
			s.SlideID = s.Animation.Targets[0].SlideID
		}
		return s, e
	}
	if action == "slide-replace-all" {
		var e error
		s.Replace, e = preparePhase3SlideReplace(payload)
		return s, e
	}
	switch action {
	case "slide-add", "slide-duplicate", "slide-hide", "slide-delete", "slide-text-insert", "slide-layout", "slide-text-format", "slide-shape-format", "slide-placeholder-fill", "slide-table-edit", "slide-media-insert":
	default:
		return s, fmt.Errorf("未实现幻灯片动作 %s", action)
	}
	id, ok := payload["slideId"].(string)
	if !ok || id == "" || len(id) > 200 || strings.ContainsAny(id, "\r\n\t\x00 ") {
		return s, fmt.Errorf("slideId 必须为现有页面的精确 ID")
	}
	if action == "slide-media-insert" {
		s.SlideID = id
		var e error
		s.Media, e = preparePhase3SlideMedia(payload)
		return s, e
	}
	if action == "slide-table-edit" {
		s.SlideID = id
		var e error
		s.Table, e = preparePhase3SlideTable(payload)
		return s, e
	}
	if action == "slide-placeholder-fill" {
		s.SlideID = id
		var e error
		s.Placeholder, e = preparePhase3SlidePlaceholder(payload)
		return s, e
	}
	if action == "slide-shape-format" {
		s.SlideID = id
		var e error
		s.Shape, e = preparePhase3SlideShape(payload)
		return s, e
	}
	if action == "slide-text-format" {
		s.SlideID = id
		var e error
		s.Format, e = preparePhase3SlideFormat(payload)
		return s, e
	}
	if action == "slide-layout" {
		if len(payload) != 2 {
			return s, fmt.Errorf("slide-layout 仅接受slideId与layoutId")
		}
		layoutID, e := phase3LayoutID(payload["layoutId"])
		if e != nil {
			return s, e
		}
		s.SlideID = id
		s.Layout = &phase3SlideLayoutSpec{ID: layoutID}
		return s, nil
	}
	if action == "slide-text-insert" {
		s.SlideID = id
		var err error
		s.Text, err = preparePhase3SlideText(payload)
		return s, err
	}
	if len(payload) != 1 {
		return s, fmt.Errorf("仅接受 slideId 参数")
	}
	s.SlideID = id
	return s, nil
}
func verifyPhase3Slides(before, after []phase3SlidePage, s phase3SlidesSpec) error {
	index := -1
	seen := map[string]bool{}
	for i, p := range before {
		if p.ID == "" || seen[p.ID] {
			return fmt.Errorf("原页面 ID 无效")
		}
		seen[p.ID] = true
		if p.ID == s.SlideID {
			index = i
		}
	}
	if index < 0 {
		return fmt.Errorf("指定页面不存在")
	}
	seenAfter := map[string]bool{}
	for _, p := range after {
		if p.ID == "" || seenAfter[p.ID] {
			return fmt.Errorf("结果页面 ID 无效")
		}
		seenAfter[p.ID] = true
	}
	want := append([]phase3SlidePage(nil), before...)
	switch s.Action {
	case "slide-add", "slide-duplicate":
		if len(after) != len(before)+1 || index+1 >= len(after) {
			return fmt.Errorf("页面数未精确增加 1")
		}
		added := after[index+1]
		if seen[added.ID] {
			return fmt.Errorf("插入位置不符或不是新页面")
		}
		if s.Action == "slide-duplicate" && (added.Content != before[index].Content || added.Hidden != before[index].Hidden) {
			return fmt.Errorf("复制页内容或隐藏状态不符")
		}
		if s.Action == "slide-add" && added.Hidden {
			return fmt.Errorf("新增页面意外隐藏")
		}
		want = append(want[:index+1], append([]phase3SlidePage{added}, before[index+1:]...)...)
	case "slide-delete":
		if len(before) < 2 {
			return fmt.Errorf("禁止删除最后一页")
		}
		want = append(want[:index], before[index+1:]...)
	case "slide-hide":
		if before[index].Hidden {
			return fmt.Errorf("页面原本已隐藏，无法证明本次修改")
		}
		want[index].Hidden = true
	default:
		return fmt.Errorf("未知动作")
	}
	if !reflect.DeepEqual(want, after) {
		return fmt.Errorf("PPTX 页面顺序、内容或隐藏状态与目标差分不一致")
	}
	return nil
}
func readPhase3SlideArtifact(filename string, ids []string) ([]phase3SlidePage, error) {
	return readPhase3SlideArtifactFiltered(filename, ids, nil)
}
func readPhase3SlideArtifactFiltered(filename string, ids []string, filter func(int, *artifactNode) (*artifactNode, error)) ([]phase3SlidePage, error) {
	z, entries, err := openArtifact(filename)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	root, err := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if err != nil {
		return nil, err
	}
	rels, err := artifactRelations(entries, "ppt/_rels/presentation.xml.rels", "ppt")
	if err != nil {
		return nil, err
	}
	list := root.child("sldIdLst")
	if list == nil || len(list.children("sldId")) != len(ids) {
		return nil, fmt.Errorf("导出页数与编辑器 ID 列表不一致")
	}
	resources := map[string]string{}
	for name := range entries {
		if strings.HasPrefix(name, "ppt/theme/") || strings.HasPrefix(name, "ppt/slideMasters/") || name == "ppt/tableStyles.xml" {
			digest, e := phase3SlideResourceFingerprint(entries, name)
			if e != nil {
				return nil, e
			}
			resources[name] = digest
		}
	}
	out := make([]phase3SlidePage, 0, len(ids))
	seen := map[string]bool{}
	for i, node := range list.children("sldId") {
		rel, ok := rels[relationID(node)]
		if !ok || rel.Kind != artifactOfficeRelNS+"/slide" || seen[rel.Target] {
			return nil, fmt.Errorf("幻灯片关系无效")
		}
		seen[rel.Target] = true
		slide, e := artifactPart(entries, rel.Target, artifactSlideNS, "sld")
		if e != nil {
			return nil, e
		}
		show := slide.attr("show")
		if show != "" && show != "0" && show != "1" && show != "true" && show != "false" {
			return nil, fmt.Errorf("隐藏标记不明确")
		}
		refs, e := phase3SlideRelationValues(entries, rel.Target)
		if e != nil {
			return nil, e
		}
		if filter != nil {
			slide, e = filter(i, slide)
			if e != nil {
				return nil, e
			}
		}
		canonical, e := phase3SlideCanonical(slide, refs)
		if e != nil {
			return nil, e
		}
		layout := refs["__layout"]
		data, _ := json.Marshal([]any{canonical, layout, refs["__notes"], resources})
		fixed := []string{}
		if c := slide.child("cSld"); c != nil {
			if tree := c.child("spTree"); tree != nil {
				for _, object := range tree.Children {
					placeholder := false
					if object.Name.Space == artifactSlideNS && object.Name.Local == "sp" {
						nv := object.child("nvSpPr")
						if nv != nil {
							nvpr := nv.child("nvPr")
							placeholder = nvpr != nil && nvpr.child("ph") != nil
						}
					}
					if !placeholder {
						canonical, e := phase3SlideCanonical(object, refs)
						if e != nil {
							return nil, e
						}
						encoded, _ := json.Marshal(canonical)
						fixed = append(fixed, fmt.Sprintf("%x", sha256.Sum256(encoded)))
					}
				}
			}
		}
		resourceBytes, _ := json.Marshal(resources)
		texts := []string{}
		var collect func(*artifactNode)
		collect = func(n *artifactNode) {
			if n.Name.Space == artifactDrawingNS && n.Name.Local == "t" && n.Text != "" {
				texts = append(texts, n.Text)
			}
			for _, c := range n.Children {
				collect(c)
			}
		}
		collect(slide)
		sort.Strings(texts)
		out = append(out, phase3SlidePage{FixedObjects: fixed, Resources: fmt.Sprintf("%x", sha256.Sum256(resourceBytes)), Notes: refs["__notes"], Layout: layout, Texts: texts, ID: ids[i], Hidden: show == "0" || show == "false", Content: fmt.Sprintf("%x", sha256.Sum256(data))})
	}
	return out, nil
}

// Relationship ids and package file numbering are export implementation details.
// Compare referenced content bytes instead, including layout and media content.
func phase3SlideRelationValues(entries map[string]*zip.File, slide string) (map[string]string, error) {
	name := path.Join(path.Dir(slide), "_rels", path.Base(slide)+".rels")
	out := map[string]string{}
	if entries[name] == nil {
		return out, nil
	}
	root, err := artifactPart(entries, name, artifactRelNS, "Relationships")
	if err != nil {
		return nil, err
	}
	rels, err := artifactRelations(entries, name, path.Dir(slide))
	if err != nil {
		return nil, err
	}
	for _, n := range root.children("Relationship") {
		id := n.attr("Id")
		if n.attr("TargetMode") == "External" {
			out[id] = n.attr("Type") + ":" + n.attr("Target")
			continue
		}
		rel := rels[id]
		digest, e := phase3SlideResourceFingerprint(entries, rel.Target)
		if e != nil {
			return nil, e
		}
		out[id] = rel.Kind + ":" + digest
		if rel.Kind == artifactOfficeRelNS+"/notesSlide" {
			out["__notes"] = digest
		}

		if rel.Kind == artifactOfficeRelNS+"/slideLayout" {
			out["__layout"] = out[id]
		}
	}
	return out, nil
}
func phase3SlideCanonical(n *artifactNode, refs map[string]string) (any, error) {
	attrs := []string{}
	for _, a := range n.Attr {
		if phase3SlideExplicitDefault(n.Name, a) {
			continue
		}
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
			continue
		}
		if a.Name.Space == "" && ((n.Name.Space == artifactSlideNS && n.Name.Local == "sld" && a.Name.Local == "show") || (n.Name.Space == artifactSlideNS && n.Name.Local == "cNvPr" && a.Name.Local == "id")) {
			continue
		}
		value := a.Value
		if a.Name.Space == artifactOfficeRelNS {
			var ok bool
			value, ok = refs[value]
			if !ok {
				return nil, fmt.Errorf("页面引用缺失 %s", a.Value)
			}
		}
		attrs = append(attrs, a.Name.Space+":"+a.Name.Local+"="+value)
	}
	sort.Strings(attrs)
	children := []any{}
	for _, c := range n.Children {
		v, e := phase3SlideCanonical(c, refs)
		if e != nil {
			return nil, e
		}
		children = append(children, v)
	}
	text := n.Text
	if len(n.Children) > 0 && strings.TrimSpace(text) == "" {
		text = ""
	}
	return []any{n.Name.Space, n.Name.Local, attrs, text, children}, nil
}
