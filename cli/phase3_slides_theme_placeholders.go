package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"reflect"
)

func phase3ThemeShapeID(n *artifactNode) string {
	for _, key := range []string{"nvSpPr", "nvPicPr", "nvGraphicFramePr", "nvGrpSpPr"} {
		if props := phase3SlideChild(phase3SlideChild(n, artifactSlideNS, key), artifactSlideNS, "cNvPr"); props != nil {
			return props.attr("id")
		}
	}
	return ""
}
func phase3ThemeShapePlaceholder(n *artifactNode) *artifactNode {
	return phase3SlideChild(phase3SlideChild(phase3SlideChild(n, artifactSlideNS, "nvSpPr"), artifactSlideNS, "nvPr"), artifactSlideNS, "ph")
}
func phase3ThemeCloneNode(n *artifactNode) *artifactNode {
	if n == nil {
		return nil
	}
	out := *n
	out.Attr = append([]xml.Attr(nil), n.Attr...)
	out.Children = nil
	for _, c := range n.Children {
		out.Children = append(out.Children, phase3ThemeCloneNode(c))
	}
	return &out
}
func phase3ThemeSetPlaceholder(n, ph *artifactNode) *artifactNode {
	out := phase3ThemeCloneNode(n)
	nv := phase3SlideChild(phase3SlideChild(out, artifactSlideNS, "nvSpPr"), artifactSlideNS, "nvPr")
	for i, c := range nv.Children {
		if c.Name.Space == artifactSlideNS && c.Name.Local == "ph" {
			nv.Children[i] = phase3ThemeCloneNode(ph)
		}
	}
	return out
}
func phase3ThemeEmptyLayoutClone(n *artifactNode) *artifactNode {
	out := phase3ThemeCloneNode(n)
	for i, c := range out.Children {
		switch {
		case c.Name.Space == artifactSlideNS && c.Name.Local == "spPr":
			children := []*artifactNode{}
			for _, p := range c.Children {
				if p.Name.Space != artifactDrawingNS || p.Name.Local != "xfrm" {
					children = append(children, p)
				}
			}
			c.Children = children
		case c.Name.Space == artifactSlideNS && c.Name.Local == "txBody":
			c.Attr = nil
			c.Text = ""
			c.Children = []*artifactNode{{Name: xml.Name{Space: artifactDrawingNS, Local: "bodyPr"}}, {Name: xml.Name{Space: artifactDrawingNS, Local: "lstStyle"}}, {Name: xml.Name{Space: artifactDrawingNS, Local: "p"}, Children: []*artifactNode{{Name: xml.Name{Space: artifactDrawingNS, Local: "pPr"}}, {Name: xml.Name{Space: artifactDrawingNS, Local: "endParaRPr"}}}}}
			out.Children[i] = c
		}
	}
	return out
}

// Native mU copies the chosen layout's ph metadata onto matching existing
// placeholders. Missing placeholders are exact layout clones with reset text,
// body/list style and inherited geometry. Source order and exact ph identity
// reproduce native matching while retaining every original object.
func phase3ReconcileThemePage(before, after, layout *artifactNode, references ...map[string]string) (*artifactNode, *artifactNode, error) {
	bt, e := phase3ShapeTree(before)
	if e != nil {
		return nil, nil, e
	}
	at, e := phase3ShapeTree(after)
	if e != nil {
		return nil, nil, e
	}
	lt, e := phase3ShapeTree(layout)
	if e != nil {
		return nil, nil, e
	}
	actualRefs, sourceRefs := map[string]string{}, map[string]string{}
	if len(references) != 0 {
		if len(references) != 2 {
			return nil, nil, fmt.Errorf("占位符关系参数无效")
		}
		actualRefs, sourceRefs = references[0], references[1]
	}
	sources := map[string]*artifactNode{}
	sourceOrder := []string{}
	for _, n := range lt.Children {
		ph := phase3ThemeShapePlaceholder(n)
		if ph == nil {
			continue
		}
		kind := ph.attr("type")
		if kind != "title" && kind != "body" && kind != "pic" {
			continue
		}
		key := phase3ThemePlaceholderKey(kind, ph.attr("idx"))
		if sources[key] != nil {
			return nil, nil, fmt.Errorf("官方版式占位符身份重复")
		}
		sources[key] = n
		sourceOrder = append(sourceOrder, key)
	}
	old := map[string]*artifactNode{}
	oldOrder := []string{}
	for _, n := range bt.Children {
		id := phase3ThemeShapeID(n)
		if id != "" {
			if old[id] != nil {
				return nil, nil, fmt.Errorf("原页面对象ID重复")
			}
			old[id] = n
			oldOrder = append(oldOrder, id)
		}
	}
	// Native mU visits source placeholders in order. pc prefers exact index,
	// treats title as index-independent, then uses the first unused same type.
	assigned := map[string]string{}
	consumed := map[string]bool{}
	for _, key := range sourceOrder {
		sourcePH := phase3ThemeShapePlaceholder(sources[key])
		kind := sourcePH.attr("type")
		match, fallback := "", ""
		for _, candidate := range bt.Children {
			id := phase3ThemeShapeID(candidate)
			ph := phase3ThemeShapePlaceholder(candidate)
			if id == "" || consumed[id] || ph == nil || ph.attr("type") != kind {
				continue
			}
			if fallback == "" {
				fallback = id
			}
			if kind == "title" || phase3ThemePlaceholderKey(kind, ph.attr("idx")) == key {
				match = id
				break
			}
		}
		if match == "" {
			match = fallback
		}
		if match != "" {
			assigned[match] = key
			consumed[match] = true
		}
	}
	replacements := map[string]*artifactNode{}
	added := map[string]bool{}
	seen := map[string]bool{}
	usedSources := map[string]bool{}
	addedOrder := []string{}
	encounteredOriginal := false
	actualOldOrder := []string{}
	for _, n := range at.Children {
		id := phase3ThemeShapeID(n)
		if id == "" {
			continue
		}
		if seen[id] {
			return nil, nil, fmt.Errorf("新页面对象ID重复")
		}
		seen[id] = true
		prior := old[id]
		ph := phase3ThemeShapePlaceholder(n)
		if prior != nil {
			encounteredOriginal = true
			actualOldOrder = append(actualOldOrder, id)
			oldPH := phase3ThemeShapePlaceholder(prior)
			if (oldPH == nil) != (ph == nil) {
				return nil, nil, fmt.Errorf("主题改变原对象占位符类型")
			}
			if ph != nil {
				key := phase3ThemePlaceholderKey(ph.attr("type"), ph.attr("idx"))
				source := sources[key]
				if source == nil || oldPH.attr("type") != ph.attr("type") || assigned[id] != key {
					return nil, nil, fmt.Errorf("占位符无唯一同类型官方来源")
				}
				if usedSources[key] {
					return nil, nil, fmt.Errorf("多个对象占用了同一官方占位符")
				}
				usedSources[key] = true
				want := phase3ThemeShapePlaceholder(source)
				a, _ := phase3SlideCanonical(ph, map[string]string{})
				b, _ := phase3SlideCanonical(want, map[string]string{})
				if !reflect.DeepEqual(a, b) {
					return nil, nil, fmt.Errorf("占位符属性不匹配官方版式")
				}
				replacements[id] = phase3ThemeSetPlaceholder(prior, want)
			}
			continue
		}
		if encounteredOriginal {
			return nil, nil, fmt.Errorf("新增占位符未位于原对象之前")
		}
		if ph == nil || n.Name.Space != artifactSlideNS || n.Name.Local != "sp" {
			return nil, nil, fmt.Errorf("主题新增了非占位对象")
		}
		key := phase3ThemePlaceholderKey(ph.attr("type"), ph.attr("idx"))
		source := sources[key]
		if source == nil {
			return nil, nil, fmt.Errorf("新增占位符无官方来源")
		}
		if usedSources[key] {
			return nil, nil, fmt.Errorf("重复新增官方占位符")
		}
		usedSources[key] = true
		want, e := phase3SlideCanonical(phase3ThemeEmptyLayoutClone(source), sourceRefs)
		if e != nil {
			return nil, nil, e
		}
		actual, e := phase3SlideCanonical(n, actualRefs)
		if e != nil || !reflect.DeepEqual(want, actual) {
			return nil, nil, fmt.Errorf("新增占位符不是官方版式的空克隆")
		}
		added[id] = true
		addedOrder = append(addedOrder, key)
	}
	expectedAdded := []string{}
	assignedSources := map[string]bool{}
	for _, key := range assigned {
		assignedSources[key] = true
	}
	for _, key := range sourceOrder {
		if !assignedSources[key] {
			expectedAdded = append(expectedAdded, key)
		}
	}
	// 主题切换会按官方版式重排对象层级，原有对象前后顺序变化属于正常副作用；
	// 只要对象没有被删除（下面的 old/seen 检查）且新增占位符集合正确，就不判失败。
	if len(actualOldOrder) != len(oldOrder) || !sameStringSet(actualOldOrder, oldOrder) {
		return nil, nil, fmt.Errorf("主题删除或替换了原有对象")
	}
	if !reflect.DeepEqual(addedOrder, expectedAdded) {
		return nil, nil, fmt.Errorf("新增占位符层级顺序不符合官方版式")
	}
	for kind := range sources {
		if !usedSources[kind] {
			return nil, nil, fmt.Errorf("缺少官方版式要求的占位符")
		}
	}
	for id := range old {
		if !seen[id] {
			return nil, nil, fmt.Errorf("主题删除原有对象")
		}
	}
	apply := func(root *artifactNode, original bool) *artifactNode {
		out := phase3ThemeCloneNode(root)
		tree, _ := phase3ShapeTree(out)
		children := []*artifactNode{}
		for _, n := range tree.Children {
			id := phase3ThemeShapeID(n)
			if !original && added[id] {
				continue
			}
			if original && replacements[id] != nil {
				n = replacements[id]
			}
			children = append(children, n)
		}
		tree.Children = children
		return out
	}
	return apply(before, true), apply(after, false), nil
}
func phase3ThemePageRoots(entries map[string]*zip.File) ([]*artifactNode, []string, error) {
	root, e := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if e != nil {
		return nil, nil, e
	}
	rels, e := phase3ThemeRelations(entries, "ppt/presentation.xml")
	if e != nil {
		return nil, nil, e
	}
	list := phase3SlideChild(root, artifactSlideNS, "sldIdLst")
	if list == nil {
		return nil, nil, fmt.Errorf("页面列表缺失")
	}
	nodes := []*artifactNode{}
	parts := []string{}
	for _, item := range list.Children {
		part := rels[relationID(item)].Target
		n, e := artifactPart(entries, part, artifactSlideNS, "sld")
		if e != nil {
			return nil, nil, e
		}
		nodes = append(nodes, n)
		parts = append(parts, part)
	}
	return nodes, parts, nil
}
func phase3ThemeReconciledPages(before, after string) ([]*artifactNode, []*artifactNode, error) {
	bz, be, e := openArtifact(before)
	if e != nil {
		return nil, nil, e
	}
	defer bz.Close()
	az, ae, e := openArtifact(after)
	if e != nil {
		return nil, nil, e
	}
	defer az.Close()
	b, _, e := phase3ThemePageRoots(be)
	if e != nil {
		return nil, nil, e
	}
	a, parts, e := phase3ThemePageRoots(ae)
	if e != nil {
		return nil, nil, e
	}
	if len(a) != len(b) {
		return nil, nil, fmt.Errorf("页数变化")
	}
	outB, outA := []*artifactNode{}, []*artifactNode{}
	for i, n := range a {
		part, e := phase3ThemeRelated(ae, parts[i], "slideLayout")
		if e != nil {
			return nil, nil, e
		}
		layout, e := artifactPart(ae, part, artifactSlideNS, "sldLayout")
		if e != nil {
			return nil, nil, e
		}
		pageRefs, e := phase3SlideRelationValues(ae, parts[i])
		if e != nil {
			return nil, nil, e
		}
		layoutRefs, e := phase3SlideRelationValues(ae, part)
		if e != nil {
			return nil, nil, e
		}
		x, y, e := phase3ReconcileThemePage(b[i], n, layout, pageRefs, layoutRefs)
		if e != nil {
			return nil, nil, e
		}
		outB = append(outB, x)
		outA = append(outA, y)
	}
	return outB, outA, nil
}

// sameStringSet 比较两个字符串切片是否包含相同元素（元素本身唯一）。
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := set[v]; !ok {
			return false
		}
	}
	return true
}
