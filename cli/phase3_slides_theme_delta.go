package main

import (
	"fmt"
	"sort"
	"strings"
)

func phase3ThemeMap(ops any) (map[string]any, error) {
	list, ok := ops.([]any)
	if !ok || len(list) != 1 {
		return nil, fmt.Errorf("主题Delta必须有一个根插入")
	}
	op, ok := list[0].(map[string]any)
	if !ok || op["action"] != "insert" {
		return nil, fmt.Errorf("主题Delta不是完整文档")
	}
	data, _ := op["data"].(map[string]any)
	group, ok := data["slide"].(map[string]any)
	if !ok || len(group) == 0 {
		return nil, fmt.Errorf("主题Delta映射缺失")
	}
	return group, nil
}
func phase3ThemeOpObject(ops any, kind string) (map[string]any, error) {
	list, ok := ops.([]any)
	if !ok {
		return nil, fmt.Errorf("主题操作列表无效")
	}
	var result map[string]any
	for _, value := range list {
		op, _ := value.(map[string]any)
		attr, _ := op["attributes"].(map[string]any)
		if attr["opType"] != kind {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("重复主题属性组%s", kind)
		}
		data, _ := op["data"].(map[string]any)
		result, _ = data["object"].(map[string]any)
	}
	if result == nil {
		return nil, fmt.Errorf("主题缺少属性组%s", kind)
	}
	return result, nil
}
func phase3ThemeLayoutName(ops any) (string, error) {
	list, ok := ops.([]any)
	if !ok {
		return "", fmt.Errorf("版式操作无效")
	}
	for _, value := range list {
		op, _ := value.(map[string]any)
		attr, _ := op["attributes"].(map[string]any)
		if attr["opType"] != "csld" {
			continue
		}
		data, _ := op["data"].(map[string]any)
		props, e := phase3ThemeOpObject(data["delta"], "attrs")
		if e != nil {
			return "", e
		}
		return str(props, "name"), nil
	}
	return "", fmt.Errorf("版式缺少内容")
}

// Normalize only schema identity slots. Shape text and arbitrary string values
// are never rewritten just because their contents happen to equal an ID.
func phase3CanonicalThemeData(v any, assets map[string]string) (any, error) {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, n := range x {
			c, e := phase3CanonicalThemeData(n, assets)
			if e != nil {
				return nil, e
			}
			out[i] = c
		}
		return out, nil
	case map[string]any:
		out := map[string]any{}
		for key, value := range x {
			if key == "rasterImageId" {
				id, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("主题图片引用无效")
				}
				hash := assets[id]
				if hash == "" {
					return nil, fmt.Errorf("主题图片缺少实际SHA256绑定")
				}
				out[key] = hash
				continue
			}
			// data.slide is an ordered shape map inside a master/layout. IDs are keys;
			// the source-defined order remains part of the canonical output.
			if key == "data" {
				data, ok := value.(map[string]any)
				if ok {
					if group, ok := data["slide"].(map[string]any); ok {
						if len(data) != 1 {
							return nil, fmt.Errorf("主题形状data含其他未归一化字段")
						}
						type pair struct {
							order int
							ops   any
						}
						items := []pair{}
						seen := map[int]bool{}
						for _, ops := range group {
							list, ok := ops.([]any)
							if !ok || len(list) != 1 {
								return nil, fmt.Errorf("主题形状操作不唯一")
							}
							op, _ := list[0].(map[string]any)
							attrs, _ := op["attributes"].(map[string]any)
							n, ok := attrs["order"].(float64)
							if !ok || n < 0 || n != float64(int(n)) || seen[int(n)] {
								return nil, fmt.Errorf("主题形状顺序不明确")
							}
							seen[int(n)] = true
							items = append(items, pair{int(n), ops})
						}
						sort.Slice(items, func(i, j int) bool { return items[i].order < items[j].order })
						ordered := []any{}
						for _, item := range items {
							c, e := phase3CanonicalThemeData(item.ops, assets)
							if e != nil {
								return nil, e
							}
							ordered = append(ordered, c)
						}
						out[key] = map[string]any{"orderedShapes": ordered}
						continue
					}
				}
			}
			c, e := phase3CanonicalThemeData(value, assets)
			if e != nil {
				return nil, e
			}
			out[key] = c
		}
		return out, nil
	default:
		return v, nil
	}
}
func phase3CanonicalThemePart(ops any, role, themeID, themeRef, masterRef string, layoutRefs []string, assets map[string]string) (any, error) {
	list, ok := ops.([]any)
	if !ok {
		return nil, fmt.Errorf("主题组件不是操作列表")
	}
	out := []any{}
	for _, v := range list {
		op, ok := v.(map[string]any)
		if !ok || op["action"] != "insert" {
			return nil, fmt.Errorf("主题包含非文档操作")
		}
		attrs, _ := op["attributes"].(map[string]any)
		kind := str(attrs, "opType")
		data, _ := op["data"].(map[string]any)
		copy := map[string]any{}
		for k, v := range op {
			copy[k] = v
		}
		if kind == "attrs" {
			props, ok := data["object"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("主题属性不是对象")
			}
			result := map[string]any{}
			for k, v := range props {
				switch k {
				case "themeId":
					if role != "master" || v != themeID {
						return nil, fmt.Errorf("官方主题身份不符")
					}
					continue
				case "themeRef":
					if role != "master" || v != themeRef {
						return nil, fmt.Errorf("母版主题引用不符")
					}
					result[k] = "$theme"
					continue
				case "masterRef":
					if role != "layout" || v != masterRef {
						return nil, fmt.Errorf("版式母版引用不符")
					}
					continue
				}
				result[k] = v
			}
			copy["data"] = map[string]any{"object": result}
		}
		if kind == "layoutRefs" {
			if role != "master" {
				return nil, fmt.Errorf("非法版式列表")
			}
			props, _ := data["object"].(map[string]any)
			refs, ok := props["layoutRefs"].([]any)
			if !ok || len(refs) != len(layoutRefs) || len(props) != 1 {
				return nil, fmt.Errorf("主题版式引用数不符")
			}
			normalized := []any{}
			for i, r := range refs {
				if r != layoutRefs[i] {
					return nil, fmt.Errorf("主题版式引用顺序不符")
				}
				normalized = append(normalized, fmt.Sprintf("$layout%d", i))
			}
			copy["data"] = map[string]any{"object": map[string]any{"layoutRefs": normalized}}
		}
		normalized, e := phase3CanonicalThemeData(copy, assets)
		if e != nil {
			return nil, e
		}
		out = append(out, normalized)
	}
	return out, nil
}
func phase3PrepareThemeDelta(s *phase3SlideThemeSpec, value any, current any) error {
	decoded, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("官方主题Delta解码缺失")
	}
	masters, e := phase3ThemeMap(decoded["masters"])
	if e != nil || len(masters) != 1 {
		return fmt.Errorf("主题必须具有一个明确母版")
	}
	layouts, e := phase3ThemeMap(decoded["layouts"])
	if e != nil {
		return e
	}
	themes, e := phase3ThemeMap(decoded["themes"])
	if e != nil || len(themes) != 1 {
		return fmt.Errorf("主题必须具有一个主题定义")
	}
	var masterRef, themeRef string
	for k := range masters {
		masterRef = k
	}
	for k := range themes {
		themeRef = k
	}
	refs, e := phase3ThemeOpObject(masters[masterRef], "layoutRefs")
	if e != nil {
		return e
	}
	raw, ok := refs["layoutRefs"].([]any)
	if !ok || len(raw) != len(layouts) {
		return fmt.Errorf("官方版式关系不完整")
	}
	ordered := []string{}
	seen := map[string]bool{}
	for _, v := range raw {
		r, ok := v.(string)
		if !ok || layouts[r] == nil || seen[r] {
			return fmt.Errorf("官方版式关系无效")
		}
		seen[r] = true
		ordered = append(ordered, r)
	}
	masterGeometry, e := phase3ThemeSourceGeometry(masters[masterRef])
	if e != nil {
		return e
	}
	for _, ref := range ordered {
		geometry := map[string]phase3ThemeTransform{}
		for k, v := range masterGeometry {
			geometry[k] = v
		}
		local, e := phase3ThemeSourceGeometry(layouts[ref])
		if e != nil {
			return e
		}
		for k, v := range local {
			geometry[k] = v
		}
		s.Geometry = append(s.Geometry, geometry)
	}
	s.LayoutCount = len(ordered)
	s.Master, e = phase3CanonicalThemePart(masters[masterRef], "master", s.ID, themeRef, masterRef, ordered, s.ImageKeys)
	if e != nil {
		return e
	}
	for _, ref := range ordered {
		canonical, e := phase3CanonicalThemePart(layouts[ref], "layout", s.ID, themeRef, masterRef, nil, s.ImageKeys)
		if e != nil {
			return e
		}
		s.Layouts = append(s.Layouts, canonical)
	}
	slides, ok := current.([]any)
	if !ok || len(slides) == 0 {
		return fmt.Errorf("当前页面版式信息缺失")
	}
	s.SlideLayouts = map[string]int{}
	for _, v := range slides {
		slide, _ := v.(map[string]any)
		id := str(slide, "id")
		props, e := phase3ThemeOpObject(slide["definition"], "attrs")
		if e != nil {
			return e
		}
		name, e := phase3ThemeLayoutName(slide["definition"])
		if e != nil {
			return e
		}
		if matching := str(props, "matchingName"); matching != "" {
			name = matching
		}
		matches := []int{}
		named := []int{}
		for i, ref := range ordered {
			p, e := phase3ThemeOpObject(layouts[ref], "attrs")
			if e != nil {
				return e
			}
			n, e := phase3ThemeLayoutName(layouts[ref])
			if e != nil {
				return e
			}
			if matching := str(p, "matchingName"); matching != "" {
				n = matching
			}
			if p["type"] == props["type"] && p["type"] != nil {
				matches = append(matches, i)
				if strings.EqualFold(n, name) {
					named = append(named, i)
				}
			}
		}
		chosen := -1
		if len(matches) == 1 {
			chosen = matches[0]
		} else if len(named) == 1 {
			chosen = named[0]
		}
		if id == "" || chosen < 0 {
			return fmt.Errorf("无法唯一核验页面%s对应的官方版式，未写入", id)
		}
		s.SlideLayouts[id] = chosen
	}
	return nil
}
func phase3VerifyThemeDelta(value any, s phase3SlideThemeSpec) error {
	m, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("重开主题操作树缺失")
	}
	masterRef := str(m, "masterRef")
	themeRef := str(m, "themeRef")
	raw, ok := m["layoutRefs"].([]any)
	if !ok || len(raw) != s.LayoutCount {
		return fmt.Errorf("重开主题版式引用缺失")
	}
	refs := []string{}
	for _, r := range raw {
		ref, ok := r.(string)
		if !ok {
			return fmt.Errorf("版式引用无效")
		}
		refs = append(refs, ref)
	}
	assets := map[string]string{}
	for _, hash := range s.Images {
		assets[hash] = hash
	}
	master, e := phase3CanonicalThemePart(m["master"], "master", s.ID, themeRef, masterRef, refs, assets)
	if e != nil {
		return e
	}
	if !phase3ThemeDeltaEqual(phase3ThemeResolvedFonts(s.Master, s.Expected), phase3ThemeResolvedFonts(master, s.Expected)) {
		return fmt.Errorf("重开后的完整母版与官方源码不符")
	}
	layouts, ok := m["layouts"].([]any)
	if !ok || len(layouts) != len(s.Layouts) {
		return fmt.Errorf("重开版式数不符")
	}
	for i, ops := range layouts {
		canonical, e := phase3CanonicalThemePart(ops, "layout", s.ID, themeRef, masterRef, nil, assets)
		if e != nil {
			return e
		}
		if !phase3ThemeDeltaEqual(phase3ThemeResolvedFonts(s.Layouts[i], s.Expected), phase3ThemeResolvedFonts(canonical, s.Expected)) {
			return fmt.Errorf("重开后的第%d个版式与官方源码不符", i+1)
		}
	}
	return nil
}

func phase3SlideThemeReloadJS() string {
	return `(async()=>{const native=` + phase3SlideThemeNativeJS() + `;const wb=window.presentationSDK.editor.workbench,p=wb.editor.presentation;const masters=[...new Set(p.slides.map(s=>s.layout._$master$_))];if(masters.length!==1)return {ok:false,error:'全套主题仍有多个母版'};const master=masters[0],ops=wb.editor._$ioAdapter$_.fromOperations(wb.editor._$collaborationManager$_._$currentPptIOData$_()).toJSON();const map=kind=>ops.find(x=>x.attributes?.opType===kind)?.data?.slide;const masterOps=map('slideMasters')?.[master.refId],layouts=map('layouts');if(!masterOps||!layouts)return {ok:false,error:'重开持久化母版/版式操作缺失'};const hashes=new Map;const normalize=async value=>{if(Array.isArray(value))return Promise.all(value.map(normalize));if(value&&typeof value==='object'){const result={};for(const [k,v]of Object.entries(value)){if(k==='rasterImageId'){if(typeof v!=='string')throw Error('主题图片引用不是字符串');if(!hashes.has(v)){const url=wb.media.checkMediaUrl(v);if(!url)throw Error('主题图片URL不可访问');const response=await fetch(url);if(!response.ok)throw Error('主题图片读回失败');const bytes=await response.arrayBuffer();if(!bytes.byteLength||bytes.byteLength>20000000)throw Error('主题图片大小无效');hashes.set(v,[...new Uint8Array(await crypto.subtle.digest('SHA-256',bytes))].map(n=>n.toString(16).padStart(2,'0')).join(''))}result[k]=hashes.get(v)}else result[k]=await normalize(v)}return result}return value};const layoutRefs=master.layouts.map(l=>l.refId);native.delta={masterRef:master.refId,themeRef:master.theme.refId,layoutRefs,master:await normalize(masterOps),layouts:await Promise.all(layoutRefs.map(id=>normalize(layouts[id])))};native.slides=native.slides.map((s,i)=>({...s,layoutIndex:master.layouts.indexOf(p.slides[i].layout)}));return native})()`
}
