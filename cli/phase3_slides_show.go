package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"reflect"
)

type phase3SlideShowSpec struct {
	LoopShow     *bool          `json:"loopShow,omitempty"`
	UseTiming    *bool          `json:"useTiming,omitempty"`
	NativeBefore map[string]any `json:"nativeBefore,omitempty"`
}
type phase3SlideShowState struct {
	LoopShow  bool `json:"loopShow"`
	UseTiming bool `json:"useTiming"`
	Other     any  `json:"otherProperties"`
}

func preparePhase3SlideShow(p map[string]any) (*phase3SlideShowSpec, error) {
	if len(p) < 1 || len(p) > 2 {
		return nil, fmt.Errorf("至少指定 loopShow 或 useTiming")
	}
	s := &phase3SlideShowSpec{}
	for k, v := range p {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("演示设置必须为布尔值")
		}
		switch k {
		case "loopShow":
			s.LoopShow = &b
		case "useTiming":
			s.UseTiming = &b
		default:
			return nil, fmt.Errorf("原生演示设置不支持 %s", k)
		}
	}
	return s, nil
}
func phase3ReadShowProperties(filename string) (phase3SlideShowState, error) {
	state := phase3SlideShowState{UseTiming: true}
	z, entries, e := openArtifact(filename)
	if e != nil {
		return state, e
	}
	defer z.Close()
	root := &artifactNode{Name: xml.Name{Space: artifactSlideNS, Local: "presentationPr"}}
	if entries["ppt/presProps.xml"] != nil {
		root, e = artifactPart(entries, "ppt/presProps.xml", artifactSlideNS, "presentationPr")
		if e != nil {
			return state, e
		}
	}
	out := *root
	out.Children = nil
	count := 0
	for _, n := range root.Children {
		if n.Name.Space != artifactSlideNS || n.Name.Local != "showPr" {
			out.Children = append(out.Children, n)
			continue
		}
		count++
		if count > 1 {
			return state, fmt.Errorf("重复演示设置节点")
		}
		copy := *n
		copy.Attr = nil
		for _, a := range n.Attr {
			if a.Name.Space == "" && (a.Name.Local == "loop" || a.Name.Local == "useTimings") {
				if a.Value != "0" && a.Value != "1" && a.Value != "false" && a.Value != "true" {
					return state, fmt.Errorf("演示设置布尔属性无效")
				}
				value := a.Value == "1" || a.Value == "true"
				if a.Name.Local == "loop" {
					state.LoopShow = value
				} else {
					state.UseTiming = value
				}
				continue
			}
			copy.Attr = append(copy.Attr, a)
		}
		if len(copy.Attr) > 0 || len(copy.Children) > 0 || copy.Text != "" {
			out.Children = append(out.Children, &copy)
		}
	}
	state.Other, e = phase3SlideCanonical(&out, map[string]string{})
	return state, e
}
func verifyPhase3SlideShowProperties(before, after string, s phase3SlideShowSpec) error {
	b, e := phase3ReadShowProperties(before)
	if e != nil {
		return e
	}
	a, e := phase3ReadShowProperties(after)
	if e != nil {
		return e
	}
	want := b
	if s.LoopShow != nil {
		want.LoopShow = *s.LoopShow
	}
	if s.UseTiming != nil {
		want.UseTiming = *s.UseTiming
	}
	if reflect.DeepEqual(b, want) {
		return fmt.Errorf("演示设置已满足目标，未执行无效写入")
	}
	if !reflect.DeepEqual(a, want) {
		return fmt.Errorf("PPTX 演示设置未持久化或改动了其他属性")
	}
	return nil
}
func verifyPhase3SlideShowArtifacts(before, after string, ids, next []string, s phase3SlidesSpec) error {
	if s.Show == nil || !reflect.DeepEqual(ids, next) {
		return fmt.Errorf("演示设置不得改变页面列表")
	}
	b, e := readPhase3SlideArtifact(before, ids)
	if e != nil {
		return e
	}
	a, e := readPhase3SlideArtifact(after, next)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(b, a) {
		return fmt.Errorf("演示设置改变了页面正文、动画或其他资源")
	}
	if e = verifyPhase3SlideShowProperties(before, after, *s.Show); e == nil {
		return nil
	}
	old, err := phase3ReadShowProperties(before)
	if err != nil {
		return err
	}
	nextProperties, err := phase3ReadShowProperties(after)
	if err != nil {
		return err
	}
	// This exporter currently omits native showPr updates. An unchanged export
	// proves collateral preservation only; the fresh-profile native check is
	// mandatory before the command can report success.
	if reflect.DeepEqual(old, nextProperties) {
		return nil
	}
	return e
}
func phase3SlideShowReadJS() string {
	return `(()=>{const wb=window.presentationSDK?.editor?.workbench,e=wb?.editor;if(wb?.isReady()!==true||typeof wb.core?.isLoopShow!=='function'||typeof wb.core?.isUseTiming!=='function'||typeof e?._$collaborationManager$_?._$currentPptIOData$_!=='function'||typeof e?._$ioAdapter$_?.fromOperations!=='function')return {ok:false,error:'缺少原生协同设置读取接口'};const roots=e._$ioAdapter$_.fromOperations(e._$collaborationManager$_._$currentPptIOData$_()).toJSON().filter(x=>x.attributes?.opType==='presentation');if(roots.length!==1||!Array.isArray(roots[0].data?.delta))return {ok:false,error:'原生演示根节点无效'};const props=roots[0].data.delta.filter(x=>x.attributes?.opType==='PresentationPr');if(props.length>1||props.length===1&&(!props[0].data?.object||typeof props[0].data.object!=='object'||Array.isArray(props[0].data.object)))return {ok:false,error:'原生演示属性无效'};return {ok:true,target:location.origin+location.pathname,loopShow:wb.core.isLoopShow(e),useTiming:wb.core.isUseTiming(e),nativePresentationPr:props[0]?.data.object??{},persistenceSource:'collaboration.PresentationPr'}})()`
}
func phase3SlideShowMutationJS(s phase3SlidesSpec, ids []string) string {
	raw, _ := json.Marshal(s.Show)
	list, _ := json.Marshal(ids)
	return `(()=>{const p=` + string(raw) + `;const expected=` + string(list) + `;` + phase3PresenceGuardJS() + `
 const sdk=window.presentationSDK,wb=sdk?.editor?.workbench,c=wb?.core,e=wb?.editor;if(wb?.isReady()!==true||typeof wb.permissions?.canEdit!=='function'||wb.permissions.canEdit()!==true)return {ok:false,error:'未确认编辑权限'};if(JSON.stringify(sdk.API.slides.getSlides().map(s=>s.id))!==JSON.stringify(expected))return {ok:false,error:'页面已变化'};const methods=['isLoopShow','isUseTiming'];if(p.loopShow!==undefined)methods.push('setLoopShow');if(p.useTiming!==undefined)methods.push('setUseTiming');if(methods.some(k=>typeof c[k]!=='function'))return {ok:false,error:'缺少原生演示设置接口'};const canonical=x=>x&&typeof x==='object'?(Array.isArray(x)?x.map(canonical):Object.fromEntries(Object.keys(x).sort().map(k=>[k,canonical(x[k])]))):x;const fresh=` + phase3SlideShowReadJS() + `;if(!p.nativeBefore||fresh.ok!==true||fresh.target!==p.nativeBefore.target||fresh.loopShow!==p.nativeBefore.loopShow||fresh.useTiming!==p.nativeBefore.useTiming||JSON.stringify(canonical(fresh.nativePresentationPr))!==JSON.stringify(canonical(p.nativeBefore.nativePresentationPr)))return {ok:false,error:'原生设置基线已变化，未写入'};const before={loopShow:c.isLoopShow(e),useTiming:c.isUseTiming(e)};if((p.loopShow===undefined||before.loopShow===p.loopShow)&&(p.useTiming===undefined||before.useTiming===p.useTiming))return {ok:false,error:'原生设置已满足目标，未写入'};if(p.loopShow!==undefined)c.setLoopShow(e,p.loopShow);if(p.useTiming!==undefined)c.setUseTiming(e,p.useTiming);const after={loopShow:c.isLoopShow(e),useTiming:c.isUseTiming(e)};if((p.loopShow!==undefined&&after.loopShow!==p.loopShow)||(p.useTiming!==undefined&&after.useTiming!==p.useTiming))return {ok:false,error:'演示设置未读回，禁止重放'};return {ok:true,before,after};})()`
}
func phase3VerifyReloadedShow(b *phase3EditorBrowser, s phase3SlideShowSpec) (any, error) {
	v, e := b.Eval(phase3SlideShowReadJS())
	if e != nil {
		return nil, e
	}
	m, ok := v.(map[string]any)
	if !ok {
		return v, fmt.Errorf("重开演示设置读回无效")
	}
	if (s.LoopShow != nil && m["loopShow"] != *s.LoopShow) || (s.UseTiming != nil && m["useTiming"] != *s.UseTiming) {
		return v, fmt.Errorf("重开演示设置未持久化")
	}
	if s.NativeBefore == nil {
		return v, fmt.Errorf("缺少写入前原生设置快照，不能证明非目标设置保持")
	}
	if e = phase3VerifyNativeShowState(s.NativeBefore, m, s); e != nil {
		return v, e
	}
	return v, nil
}
