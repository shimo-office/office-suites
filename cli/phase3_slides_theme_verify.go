package main

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"path"
	"reflect"
	"strings"
)

func phase3ThemeRelations(entries map[string]*zip.File, part string) (map[string]artifactRelation, error) {
	return artifactRelations(entries, path.Join(path.Dir(part), "_rels", path.Base(part)+".rels"), path.Dir(part))
}
func phase3ThemeRelated(entries map[string]*zip.File, part, kind string) (string, error) {
	rels, e := phase3ThemeRelations(entries, part)
	if e != nil {
		return "", e
	}
	found := ""
	for _, r := range rels {
		if r.Kind == artifactOfficeRelNS+"/"+kind {
			if found != "" {
				return "", fmt.Errorf("重复%s关系", kind)
			}
			found = r.Target
		}
	}
	if found == "" {
		return "", fmt.Errorf("缺少%s关系", kind)
	}
	return found, nil
}
func phase3VerifyThemeXML(n *artifactNode, s phase3SlideThemeSpec) error {
	if n.attr("name") != str(s.Expected, "name") {
		return fmt.Errorf("PPTX主题名不符")
	}
	elements, _ := s.Expected["themeElements"].(map[string]any)
	colors, _ := elements["clrScheme"].(map[string]any)
	fonts, _ := elements["fontScheme"].(map[string]any)
	actual := phase3SlideChild(n, artifactDrawingNS, "themeElements")
	clr := phase3SlideChild(actual, artifactDrawingNS, "clrScheme")
	font := phase3SlideChild(actual, artifactDrawingNS, "fontScheme")
	if clr == nil || font == nil {
		return fmt.Errorf("PPTX主题颜色或字体缺失")
	}
	for _, key := range []string{"dk1", "lt1", "dk2", "lt2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hlink", "folHlink"} {
		value, ok := colors[key].(map[string]any)
		if !ok {
			return fmt.Errorf("官方主题颜色%s缺失", key)
		}
		color, ok := value["color"].(map[string]any)
		mods, _ := value["mods"].([]any)
		if !ok || color["type"] != float64(1) || len(mods) != 0 {
			return fmt.Errorf("官方主题颜色%s并非可核验RGB", key)
		}
		node := phase3SlideChild(clr, artifactDrawingNS, key)
		rgb := phase3SlideChild(node, artifactDrawingNS, "srgbClr")
		if node == nil || len(node.Children) != 1 || rgb == nil || len(rgb.Children) != 0 || !strings.EqualFold(rgb.attr("val"), str(color, "val")) {
			return fmt.Errorf("PPTX主题颜色%s不符", key)
		}
	}
	for _, kind := range []string{"majorFont", "minorFont"} {
		expected, ok := fonts[kind].(map[string]any)
		if !ok {
			return fmt.Errorf("官方主题字体%s缺失", kind)
		}
		node := phase3SlideChild(font, artifactDrawingNS, kind)
		for _, script := range []string{"latin", "ea", "cs"} {
			want, ok := expected[script].(map[string]any)
			got := phase3SlideChild(node, artifactDrawingNS, script)
			if !ok || got == nil || got.attr("typeface") != str(want, "typeface") {
				return fmt.Errorf("PPTX主题字体%s/%s不符", kind, script)
			}
		}
	}
	return nil
}
func phase3VerifyThemeMaster(entries map[string]*zip.File, layout string, s phase3SlideThemeSpec, expectedIndex int) error {
	master, e := phase3ThemeRelated(entries, layout, "slideMaster")
	if e != nil {
		return e
	}
	n, e := artifactPart(entries, master, artifactSlideNS, "sldMaster")
	if e != nil {
		return e
	}
	list := phase3SlideChild(n, artifactSlideNS, "sldLayoutIdLst")
	if list == nil || len(list.Children) != s.LayoutCount {
		return fmt.Errorf("PPTX主题版式数量不符")
	}
	rels, e := phase3ThemeRelations(entries, master)
	if e != nil {
		return e
	}
	seen := map[string]bool{}
	for i, item := range list.Children {
		r, ok := rels[relationID(item)]
		if !ok || r.Kind != artifactOfficeRelNS+"/slideLayout" || seen[r.Target] {
			return fmt.Errorf("主题版式引用不完整或重复")
		}
		if r.Target == layout && i != expectedIndex {
			return fmt.Errorf("页面版式索引与官方目标不符")
		}
		seen[r.Target] = true
		back, e := phase3ThemeRelated(entries, r.Target, "slideMaster")
		if e != nil || back != master {
			return fmt.Errorf("主题版式未指向同一母版")
		}
	}
	if !seen[layout] {
		return fmt.Errorf("当前页面版式不属于目标母版")
	}
	geometry, e := phase3ThemeXMLGeometry(n)
	if e != nil {
		return e
	}
	layoutRoot, e := artifactPart(entries, layout, artifactSlideNS, "sldLayout")
	if e != nil {
		return e
	}
	local, e := phase3ThemeXMLGeometry(layoutRoot)
	if e != nil {
		return e
	}
	for k, v := range local {
		if v != nil {
			geometry[k] = v
		}
	}
	if expectedIndex < 0 || expectedIndex >= len(s.Geometry) {
		return fmt.Errorf("官方版式几何预期缺失")
	}
	for k, want := range s.Geometry[expectedIndex] {
		actual := geometry[k]
		if actual == nil || !reflect.DeepEqual(*actual, want) {
			return fmt.Errorf("导出模板占位符%s几何不匹配官方源码", k)
		}
	}

	theme, e := phase3ThemeRelated(entries, master, "theme")
	if e != nil {
		return e
	}
	root, e := artifactPart(entries, theme, artifactDrawingNS, "theme")
	if e != nil {
		return e
	}
	if e = phase3VerifyThemeXML(root, s); e != nil {
		return e
	}
	// Every official picture must be reachable through the actual master/layout
	// graph. Merely appending unused image bytes to the ZIP cannot satisfy proof.
	hashes := map[string]bool{}
	visited := map[string]bool{}
	var walk func(string) error
	walk = func(part string) error {
		if visited[part] {
			return nil
		}
		if len(visited) > 2000 {
			return fmt.Errorf("主题资源闭包过大")
		}
		visited[part] = true
		relPath := path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
		if entries[relPath] == nil {
			return nil
		}
		rs, e := phase3ThemeRelations(entries, part)
		if e != nil {
			return e
		}
		for _, r := range rs {
			if r.Kind == artifactOfficeRelNS+"/image" {
				raw, e := zipEntryBytes(entries[r.Target])
				if e != nil {
					return e
				}
				hashes[fmt.Sprintf("%x", sha256.Sum256(raw))] = true
			}
			if e = walk(r.Target); e != nil {
				return e
			}
		}
		return nil
	}
	if e = walk(master); e != nil {
		return e
	}
	for _, hash := range s.Images {
		if !hashes[hash] {
			return fmt.Errorf("官方主题图片未嵌入母版/版式闭包：%s", hash)
		}
	}
	return nil
}

type phase3ThemeLocalPage struct {
	ID         string
	Hidden     bool
	Content    any
	Transforms map[string]*phase3ThemeTransform
	Notes      string
}

func phase3ReadThemeLocalPages(filename string, ids []string, expected *phase3SlideThemeSpec, masks map[string]map[string]bool, normalized ...[]*artifactNode) ([]phase3ThemeLocalPage, any, error) {
	z, entries, e := openArtifact(filename)
	if e != nil {
		return nil, nil, e
	}
	defer z.Close()
	root, e := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if e != nil {
		return nil, nil, e
	}
	rels, e := phase3ThemeRelations(entries, "ppt/presentation.xml")
	if e != nil {
		return nil, nil, e
	}
	list := phase3SlideChild(root, artifactSlideNS, "sldIdLst")
	if list == nil || len(list.Children) != len(ids) {
		return nil, nil, fmt.Errorf("主题页数不符")
	}
	sizeNode := phase3SlideChild(root, artifactSlideNS, "sldSz")
	if sizeNode == nil {
		return nil, nil, fmt.Errorf("缺少页面尺寸")
	}
	size, e := phase3SlideCanonical(sizeNode, map[string]string{})
	if e != nil {
		return nil, nil, e
	}
	pages := []phase3ThemeLocalPage{}
	for i, item := range list.Children {
		r, ok := rels[relationID(item)]
		if !ok || r.Kind != artifactOfficeRelNS+"/slide" {
			return nil, nil, fmt.Errorf("页面引用无效")
		}
		n, e := artifactPart(entries, r.Target, artifactSlideNS, "sld")
		if e != nil {
			return nil, nil, e
		}
		if len(normalized) > 0 {
			if len(normalized) != 1 || len(normalized[0]) != len(ids) {
				return nil, nil, fmt.Errorf("占位符比对基线数量无效")
			}
			n = normalized[0][i]
		}
		refs, e := phase3SlideRelationValues(entries, r.Target)
		if e != nil {
			return nil, nil, e
		}
		local, e := phase3SlideCanonical(phase3MaskThemeGeometry(n, masks[ids[i]]), refs)
		if e != nil {
			return nil, nil, e
		}
		if expected != nil {
			layout, e := phase3ThemeRelated(entries, r.Target, "slideLayout")
			if e != nil {
				return nil, nil, e
			}
			expectedIndex, exists := expected.SlideLayouts[ids[i]]
			if !exists {
				return nil, nil, fmt.Errorf("缺少目标页面版式索引")
			}
			if e = phase3VerifyThemeMaster(entries, layout, *expected, expectedIndex); e != nil {
				return nil, nil, e
			}
		}
		transforms, e := phase3ThemeXMLGeometry(n)
		if e != nil {
			return nil, nil, e
		}
		if masks[ids[i]] != nil {
			for key := range masks[ids[i]] {
				delete(transforms, key)
			}
		}
		pages = append(pages, phase3ThemeLocalPage{Transforms: transforms, ID: ids[i], Hidden: n.attr("show") == "0" || n.attr("show") == "false", Content: local, Notes: refs["__notes"]})
	}
	return pages, size, nil
}
func verifyPhase3SlideThemeArtifacts(before, after string, ids, next []string, s phase3SlidesSpec) error {
	if s.Theme == nil || !reflect.DeepEqual(ids, next) {
		return fmt.Errorf("应用主题不得改变页面列表")
	}
	reconciledBefore, reconciledAfter, e := phase3ThemeReconciledPages(before, after)
	if e != nil {
		return e
	}
	b, bs, e := phase3ReadThemeLocalPages(before, ids, nil, nil, reconciledBefore)
	if e != nil {
		return e
	}
	a, as, e := phase3ReadThemeLocalPages(after, next, s.Theme, nil, reconciledAfter)
	if e != nil {
		return e
	}
	masks := map[string]map[string]bool{}
	for i := range b {
		index, ok := s.Theme.SlideLayouts[b[i].ID]
		if !ok || index < 0 || index >= len(s.Theme.Geometry) {
			return fmt.Errorf("缺少每页对应的官方版式")
		}
		allowed, e := phase3ThemeAllowedGeometry(b[i].Transforms, a[i].Transforms, s.Theme.Geometry[index])
		if e != nil {
			return e
		}
		masks[b[i].ID] = allowed
	}
	b, _, e = phase3ReadThemeLocalPages(before, ids, nil, masks, reconciledBefore)
	if e != nil {
		return e
	}
	a, _, e = phase3ReadThemeLocalPages(after, next, s.Theme, masks, reconciledAfter)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(b, a) || !reflect.DeepEqual(bs, as) {
		return fmt.Errorf("应用主题改变了原文字、局部样式、非占位对象、备注或页面尺寸：%s", phase3ThemePageDiff(b, a, bs, as))
	}
	bp, e := phase3ReadShowProperties(before)
	if e != nil {
		return e
	}
	ap, e := phase3ReadShowProperties(after)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(bp, ap) {
		return fmt.Errorf("应用主题改变了演示设置")
	}
	return nil
}

// phase3ThemePageDiff names the exact page and property that broke theme
// acceptance, so a failure points at a slide/shape instead of the whole deck.
func phase3ThemePageDiff(before, after []phase3ThemeLocalPage, bs, as any) string {
	if len(before) != len(after) {
		return fmt.Sprintf("页面数 %d → %d", len(before), len(after))
	}
	for i := range before {
		b, a := before[i], after[i]
		switch {
		case b.ID != a.ID:
			return fmt.Sprintf("第%d页页面 ID %s → %s", i+1, b.ID, a.ID)
		case b.Hidden != a.Hidden:
			return fmt.Sprintf("第%d页（%s）隐藏状态被改变", i+1, b.ID)
		case b.Notes != a.Notes:
			return fmt.Sprintf("第%d页（%s）备注被改变", i+1, b.ID)
		case !reflect.DeepEqual(b.Content, a.Content):
			return fmt.Sprintf("第%d页（%s）文字或局部样式被改变", i+1, b.ID)
		case !reflect.DeepEqual(b.Transforms, a.Transforms):
			return fmt.Sprintf("第%d页（%s）图形位置/尺寸被改变：%s", i+1, b.ID, phase3ThemeTransformDiff(b.Transforms, a.Transforms))
		}
	}
	if !reflect.DeepEqual(bs, as) {
		return "页面尺寸或母版集合被改变"
	}
	return "差异未定位到具体页面"
}

func phase3ThemeTransformDiff(before, after map[string]*phase3ThemeTransform) string {
	for key, b := range before {
		a, ok := after[key]
		if !ok {
			return fmt.Sprintf("对象 %s 丢失", key)
		}
		if !reflect.DeepEqual(b, a) {
			return fmt.Sprintf("对象 %s 的几何数据变化", key)
		}
	}
	for key := range after {
		if _, ok := before[key]; !ok {
			return fmt.Sprintf("新增对象 %s", key)
		}
	}
	return "对象集合未对齐"
}
