package main

import (
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"strings"
)

type phase3SlideLayoutSpec struct {
	CurrentIndex int    `json:"currentIndex"`
	ID           string `json:"layoutId"`
	Index        int    `json:"index"`
	Expected     string `json:"expectedLayout,omitempty"`
}

func phase3SlideDesignJS() string {
	return `(()=>{const sdk=window.presentationSDK;const wb=sdk?.editor?.workbench;if(wb?.isReady()!==true||typeof wb.core?.getSlideLayout!=='function'||typeof wb.core?.getSlideMasters!=='function'||typeof wb.core?.getSlideElements!=='function')return {ok:false,error:'缺少可验证版式接口'};const masters=wb.core.getSlideMasters(wb.editor).map(m=>({id:String(m.masterId),theme:m.theme,layouts:m.layouts.map(l=>({id:String(l.layoutId),index:l.index,name:l.name}))}));return {ok:true,slides:sdk.API.slides.getSlides().map(s=>{const layout=wb.core.getSlideLayout(wb.editor,s.getIndex());return {id:s.id,masterId:String(layout.masterId),layoutId:String(layout.layoutId),shapes:s.getShapes().map(shape=>({id:shape.id,name:shape.name,type:shape.type,text:typeof shape.textContent==='string'?shape.textContent:null})),tables:wb.core.getSlideElements(wb.editor,s.getIndex()).filter(n=>n.type==='Table').map(t=>({id:t.id,name:t.name,rowCount:t.rowCount,columnCount:t.columnCount}))}}),masters,themes:wb.theme?.getOfficialThemes?.().map(t=>({id:t.id,name:t.name,colors:t.colors,bodyFont:t.bodyFont,titleFont:t.titleFont}))||[]}})()`
}
func phase3ResolveSlideLayout(b *phase3EditorBrowser, s *phase3SlidesSpec) error {
	v, err := b.Eval(phase3SlideDesignJS())
	if err != nil {
		return err
	}
	return phase3ResolveSlideLayoutValue(v, s)
}
func phase3ResolveSlideLayoutValue(v any, s *phase3SlidesSpec) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("版式快照无效")
	}
	slides, _ := m["slides"].([]any)
	master := ""
	current := ""
	for _, raw := range slides {
		n, _ := raw.(map[string]any)
		if str(n, "id") == s.SlideID {
			master = str(n, "masterId")
			current = str(n, "layoutId")
		}
	}
	if master == "" {
		return fmt.Errorf("目标页母版未确认")
	}
	if current == s.Layout.ID {
		return fmt.Errorf("页面已经使用目标版式，未执行无效写入")
	}
	masters, _ := m["masters"].([]any)
	for _, raw := range masters {
		n, _ := raw.(map[string]any)
		if str(n, "id") != master {
			continue
		}
		ls, _ := n["layouts"].([]any)
		desired, currentIndex := -1, -1
		for _, item := range ls {
			l, _ := item.(map[string]any)
			index, ok := l["index"].(float64)
			if !ok || index < 0 || index != float64(int(index)) {
				return fmt.Errorf("版式索引无效")
			}
			if str(l, "id") == s.Layout.ID {
				desired = int(index)
			}
			if str(l, "id") == current {
				currentIndex = int(index)
			}
		}
		if desired >= 0 && currentIndex >= 0 {
			s.Layout.Index = desired
			s.Layout.CurrentIndex = currentIndex
			return nil
		}
	}

	return fmt.Errorf("layoutId 必须属于目标页当前母版；跨母版换主题请使用独立动作")
}
func phase3SlideExpectedLayout(filename string, ids []string, s phase3SlidesSpec) (string, error) {
	z, entries, err := openArtifact(filename)
	if err != nil {
		return "", err
	}
	defer z.Close()
	root, err := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if err != nil {
		return "", err
	}
	rels, err := artifactRelations(entries, "ppt/_rels/presentation.xml.rels", "ppt")
	if err != nil {
		return "", err
	}
	list := root.child("sldIdLst")
	if list == nil || len(list.children("sldId")) != len(ids) {
		return "", fmt.Errorf("页面列表不符")
	}
	index := -1
	for i, id := range ids {
		if id == s.SlideID {
			index = i
		}
	}
	if index < 0 {
		return "", fmt.Errorf("目标页不存在")
	}
	slideRel, ok := rels[relationID(list.children("sldId")[index])]
	if !ok {
		return "", fmt.Errorf("页面关系缺失")
	}
	next := func(part, kind string) (string, error) {
		rs, e := artifactRelations(entries, path.Join(path.Dir(part), "_rels", path.Base(part)+".rels"), path.Dir(part))
		if e != nil {
			return "", e
		}
		found := ""
		for _, r := range rs {
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
	layout, err := next(slideRel.Target, "slideLayout")
	if err != nil {
		return "", err
	}
	master, err := next(layout, "slideMaster")
	if err != nil {
		return "", err
	}
	mr, err := artifactPart(entries, master, artifactSlideNS, "sldMaster")
	if err != nil {
		return "", err
	}
	ml := mr.child("sldLayoutIdLst")
	if ml == nil || s.Layout.Index < 0 || s.Layout.Index >= len(ml.children("sldLayoutId")) {
		return "", fmt.Errorf("导出母版版式索引不存在")
	}
	mrels, err := artifactRelations(entries, path.Join(path.Dir(master), "_rels", path.Base(master)+".rels"), path.Dir(master))
	if err != nil {
		return "", err
	}
	if s.Layout.CurrentIndex < 0 || s.Layout.CurrentIndex >= len(ml.children("sldLayoutId")) {
		return "", fmt.Errorf("原版式索引无效")
	}
	currentRel, ok := mrels[relationID(ml.children("sldLayoutId")[s.Layout.CurrentIndex])]
	if !ok || currentRel.Target != layout {
		return "", fmt.Errorf("SDK版式顺序与导出母版关系不一致，未写入")
	}
	selected, ok := mrels[relationID(ml.children("sldLayoutId")[s.Layout.Index])]
	if !ok || selected.Kind != artifactOfficeRelNS+"/slideLayout" {
		return "", fmt.Errorf("导出目标版式关系无效")
	}
	digest, err := phase3SlideResourceFingerprint(entries, selected.Target)
	if err != nil {
		return "", err
	}
	return selected.Kind + ":" + digest, nil
}
func verifyPhase3SlideLayout(before, after []phase3SlidePage, s phase3SlidesSpec) error {
	if s.Layout == nil || s.Layout.Expected == "" || len(before) != len(after) {
		return fmt.Errorf("缺少版式预期或页数改变")
	}
	found := false
	for i, b := range before {
		a := after[i]
		if b.ID != a.ID {
			return fmt.Errorf("页面ID/顺序变化")
		}
		if b.ID != s.SlideID {
			if !reflect.DeepEqual(b, a) {
				return fmt.Errorf("非目标页出现变化")
			}
			continue
		}
		found = true
		if b.Layout == s.Layout.Expected {
			return fmt.Errorf("版式导出原本相同，不能证明实际修改")
		}
		if a.Layout != s.Layout.Expected || a.Hidden != b.Hidden || a.Resources != b.Resources || a.Notes != b.Notes || !reflect.DeepEqual(a.FixedObjects, b.FixedObjects) || !reflect.DeepEqual(a.Texts, b.Texts) {
			return fmt.Errorf("目标版式不符、正文丢失或隐藏状态改变")
		}
	}
	if !found {
		return fmt.Errorf("目标页不存在")
	}
	return nil
}
func phase3SlideLayoutMutationJS(s phase3SlidesSpec, ids []string) string {
	p, _ := json.Marshal(s)
	expected, _ := json.Marshal(ids)
	return `(()=>{const request=` + string(p) + `;const expected=` + string(expected) + `;` + phase3PresenceGuardJS() + `
const sdk=window.presentationSDK;const wb=sdk?.editor?.workbench;if(wb?.isReady()!==true||typeof wb.permissions?.canEdit!=='function'||wb.permissions.canEdit()!==true)return {ok:false,error:'未确认编辑权限'};const api=sdk.API.slides;const slides=api.getSlides();if(JSON.stringify(slides.map(s=>s.id))!==JSON.stringify(expected))return {ok:false,error:'页面列表变化'};const slide=slides.find(s=>s.id===request.slideId);if(!slide)return {ok:false,error:'页面不存在'};const before=wb.core.getSlideLayout(wb.editor,slide.getIndex());const master=wb.core.getSlideMasters(wb.editor).find(m=>m.masterId===before.masterId);const layout=master?.layouts.find(l=>String(l.layoutId)===request.layout.layoutId);if(!layout||layout.index!==request.layout.index||String(before.layoutId)===request.layout.layoutId)return {ok:false,error:'目标版式已变或不属于当前母版'};api.setCurrentSlideIndex(slide.id);api.setSelectedSlides([slide.id]);wb.core.changeLayout(wb.editor,layout.layoutId);const after=wb.core.getSlideLayout(wb.editor,slide.getIndex());if(String(after.layoutId)!==request.layout.layoutId)return {ok:false,error:'目标版式未读回，禁止重放'};return {ok:true,before,after};})()`
}
func phase3VerifyReloadedLayout(b *phase3EditorBrowser, s phase3SlidesSpec) error {
	v, err := b.Eval(phase3SlideDesignJS())
	if err != nil {
		return err
	}
	m, _ := v.(map[string]any)
	slides, _ := m["slides"].([]any)
	for _, raw := range slides {
		n, _ := raw.(map[string]any)
		if str(n, "id") == s.SlideID {
			if str(n, "layoutId") == s.Layout.ID {
				return nil
			}
			return fmt.Errorf("重开后的layoutId未持久化")
		}
	}
	return fmt.Errorf("重开后目标页缺失")
}
func phase3LayoutID(value any) (string, error) {
	id, ok := value.(string)
	if !ok || id == "" || len(id) > 200 || strings.ContainsAny(id, "\r\n\t \x00") {
		return "", fmt.Errorf("layoutId 必须为当前母版中已发现的精确ID字符串")
	}
	return id, nil
}
