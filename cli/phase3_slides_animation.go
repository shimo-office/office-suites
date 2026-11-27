package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

type phase3SlideAnimationTarget struct {
	SlideID string `json:"slideId"`
	Text    string `json:"text"`
}
type phase3SlideAnimationSpec struct {
	Targets  []phase3SlideAnimationTarget `json:"targets"`
	FontSize float64                      `json:"fontSize,omitempty"`
}

func preparePhase3SlideAnimation(p map[string]any) (*phase3SlideAnimationSpec, error) {
	for key := range p {
		if key != "targets" && key != "fontSize" {
			return nil, fmt.Errorf("仅接受 targets 与可选 fontSize；动画固定为单击进入的出现效果")
		}
	}
	targets, ok := p["targets"].([]any)
	if !ok || len(targets) == 0 || len(targets) > 100 {
		return nil, fmt.Errorf("targets 需要 1–100 个明确页面标题")
	}
	s := &phase3SlideAnimationSpec{}
	seen := map[string]bool{}
	for _, v := range targets {
		target, ok := v.(map[string]any)
		if !ok || len(target) != 2 {
			return nil, fmt.Errorf("每个目标需要 slideId 和唯一完整文字 text")
		}
		id, _ := target["slideId"].(string)
		if id == "" || len(id) > 200 || strings.ContainsAny(id, " \r\n\t\x00") || seen[id] {
			return nil, fmt.Errorf("页面 ID 无效或重复")
		}
		text, ok := target["text"].(string)
		if !ok || text == "" || strings.ContainsAny(text, "\r\n") || utf8.RuneCountInString(text) > 100000 {
			return nil, fmt.Errorf("text 必须精确指定非空标题，不含换行")
		}
		if size, exists := p["fontSize"]; exists {
			format, e := preparePhase3SlideFormat(map[string]any{"text": text, "fontSize": size})
			if e != nil {
				return nil, e
			}
			s.FontSize = format.Attrs["fontSize"].(float64)
		}
		s.Targets = append(s.Targets, phase3SlideAnimationTarget{SlideID: id, Text: text})
		seen[id] = true
	}
	return s, nil
}
func phase3WalkSlideNode(n *artifactNode, fn func(*artifactNode)) {
	if n == nil {
		return
	}
	fn(n)
	for _, c := range n.Children {
		phase3WalkSlideNode(c, fn)
	}
}
func verifyPhase3AppearTiming(n *artifactNode, shapeID string) error {
	if n == nil || shapeID == "" {
		return fmt.Errorf("缺少目标对象动画")
	}
	presets, targets, sets := 0, 0, 0
	var failure error
	phase3WalkSlideNode(n, func(x *artifactNode) {
		if x.Name.Space != artifactSlideNS {
			return
		}
		if x.Name.Local == "cTn" && x.attr("presetClass") != "" {
			presets++
			if x.attr("presetClass") != "entr" || x.attr("presetID") != "1" || x.attr("nodeType") != "clickEffect" {
				failure = fmt.Errorf("不是单击进入的出现动画")
			}
		}
		if x.Name.Local == "spTgt" {
			targets++
			if x.attr("spid") != shapeID {
				failure = fmt.Errorf("动画绑定了错误形状")
			}
		}
		switch x.Name.Local {
		case "anim", "animClr", "animEffect", "animMotion", "animRot", "animScale", "cmd", "audio", "video":
			failure = fmt.Errorf("出现动画中含额外行为")
		}
		if x.Name.Local == "set" {
			sets++
			bhvr := phase3SlideChild(x, artifactSlideNS, "cBhvr")
			dur := phase3SlideChild(bhvr, artifactSlideNS, "cTn")
			attr := phase3SlideChild(phase3SlideChild(bhvr, artifactSlideNS, "attrNameLst"), artifactSlideNS, "attrName")
			value := phase3SlideChild(phase3SlideChild(x, artifactSlideNS, "to"), artifactSlideNS, "strVal")
			target := phase3SlideChild(phase3SlideChild(bhvr, artifactSlideNS, "tgtEl"), artifactSlideNS, "spTgt")
			if target == nil || target.attr("spid") != shapeID || dur == nil || dur.attr("dur") != "1" || attr == nil || attr.Text != "style.visibility" || value == nil || value.attr("val") != "visible" {
				failure = fmt.Errorf("动画未实际设置对象可见状态")
			}
		}
	})
	if failure != nil {
		return failure
	}
	if presets != 1 || targets < 1 || sets != 1 {
		return fmt.Errorf("对象动画数量/目标/行为不完整")
	}
	return nil
}
func phase3AnimationFilter(ids []string, s phase3SlidesSpec, verify bool) func(int, *artifactNode) (*artifactNode, error) {
	return func(i int, n *artifactNode) (*artifactNode, error) {
		var target *phase3SlideAnimationTarget
		for j := range s.Animation.Targets {
			if s.Animation.Targets[j].SlideID == ids[i] {
				target = &s.Animation.Targets[j]
				break
			}
		}
		if target == nil {
			return n, nil
		}
		timing := phase3SlideChild(n, artifactSlideNS, "timing")
		if !verify && timing != nil && len(timing.Children) > 0 {
			return nil, fmt.Errorf("目标页已有时序数据，禁止覆盖已有动画/媒体时序")
		}
		format := phase3SlideFormatSpec{Text: target.Text, Attrs: map[string]any{}}
		if s.Animation.FontSize != 0 {
			format.Attrs["fontSize"] = s.Animation.FontSize
		}
		out, e := phase3MapSlideShape(n, func(x *artifactNode) bool { return phase3SlideText(x) == target.Text }, func(x *artifactNode) (*artifactNode, error) {
			if e := phase3VisibleShape(x); e != nil {
				return nil, e
			}
			if verify {
				if s.Animation.FontSize != 0 {
					if e := verifyPhase3SlideTextFormatNode(x, format); e != nil {
						return nil, e
					}
				}
				id := phase3SlideChild(phase3SlideChild(x, artifactSlideNS, "nvSpPr"), artifactSlideNS, "cNvPr")
				if e := verifyPhase3AppearTiming(timing, id.attr("id")); e != nil {
					return nil, e
				}
			}
			if s.Animation.FontSize == 0 {
				return x, nil
			}
			return phase3MaskSlideTextFormat(x, format.Attrs), nil
		})
		if e != nil {
			return nil, e
		}
		result := *out
		result.Children = nil
		for _, c := range out.Children {
			if c.Name.Space == artifactSlideNS && c.Name.Local == "timing" {
				continue
			}
			result.Children = append(result.Children, c)
		}
		return &result, nil
	}
}
func phase3PrepareSlideAnimation(filename string, ids []string, s phase3SlidesSpec) error {
	for _, target := range s.Animation.Targets {
		found := false
		for _, id := range ids {
			found = found || id == target.SlideID
		}
		if !found {
			return fmt.Errorf("动画目标页不存在")
		}
	}
	_, e := readPhase3SlideArtifactFiltered(filename, ids, phase3AnimationFilter(ids, s, false))
	return e
}
func verifyPhase3SlideAnimationArtifacts(before, after string, ids, next []string, s phase3SlidesSpec) error {
	if s.Animation == nil || !reflect.DeepEqual(ids, next) {
		return fmt.Errorf("标题批处理不得改变页面列表")
	}
	if e := phase3PrepareSlideAnimation(before, ids, s); e != nil {
		return e
	}
	b, e := readPhase3SlideArtifactFiltered(before, ids, phase3AnimationFilter(ids, s, false))
	if e != nil {
		return e
	}
	a, e := readPhase3SlideArtifactFiltered(after, next, phase3AnimationFilter(next, s, true))
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(b, a) {
		return fmt.Errorf("批处理改变了非目标内容或其他样式")
	}
	return nil
}
func phase3SlideAnimationMutationJS(s phase3SlidesSpec, ids []string) string {
	raw, _ := json.Marshal(s)
	list, _ := json.Marshal(ids)
	return `(()=>{const r=` + string(raw) + `;const expected=` + string(list) + `;` + phase3PresenceGuardJS() + `
 const sdk=window.presentationSDK,wb=sdk?.editor?.workbench,api=sdk?.API?.slides,c=wb?.core,e=wb?.editor;if(wb?.isReady()!==true||typeof wb.permissions?.canEdit!=='function'||wb.permissions.canEdit()!==true)return {ok:false,error:'未确认编辑权限'};if(JSON.stringify(api.getSlides().map(s=>s.id))!==JSON.stringify(expected))return {ok:false,error:'页面已变化'};const methods=['getSlideAnimation','applySlideAnimation','setTextRange','selectElementByIds'];if(r.animation.fontSize!==undefined)methods.push('setFontSize');if(methods.some(k=>typeof c[k]!=='function'))return {ok:false,error:'缺少对象动画接口'};const resolved=[];for(const t of r.animation.targets){const slide=api.getSlides().find(s=>s.id===t.slideId),shapes=slide?.getShapes().filter(s=>s.textContent===t.text);if(shapes?.length!==1||c.getSlideAnimation(e,slide.getIndex()).length!==0)return {ok:false,error:'标题不唯一或页面已有动画'};resolved.push({slide,shape:shapes[0]})}const results=[];for(const {slide,shape} of resolved){api.setCurrentSlideIndex(slide.id);c.setTextRange(e,null);c.selectElementByIds(e,[shape.id]);if(r.animation.fontSize!==undefined)c.setFontSize(e,r.animation.fontSize);c.applySlideAnimation(e,[{ShapeId:shape.id,AnimClass:'Entrance',AnimType:1,AnimDelay:0,AnimDuration:1,AnimTriggerType:'ClickEffect'}]);const readback=c.getSlideAnimation(e,slide.getIndex());if(readback.length!==1||readback[0].ShapeId!==shape.id||readback[0].AnimClass!=='Entrance'||readback[0].AnimType!==1)return {ok:false,error:'对象动画读回失败，禁止重放'};results.push({slideId:slide.id,shapeId:shape.id,animations:readback})}return {ok:true,results};})()`
}

func phase3VerifyReloadedAnimation(b *phase3EditorBrowser, s phase3SlidesSpec) (any, error) {
	raw, _ := json.Marshal(s.Animation)
	return b.Eval(`(()=>{const spec=` + string(raw) + `;const sdk=window.presentationSDK,wb=sdk.editor.workbench,results=[];for(const t of spec.targets){const slide=sdk.API.slides.getSlides().find(s=>s.id===t.slideId),shapes=slide?.getShapes().filter(s=>s.textContent===t.text);if(shapes?.length!==1)return {ok:false,error:'重开后标题目标不唯一'};const animation=wb.core.getSlideAnimation(wb.editor,slide.getIndex());if(animation.length!==1)return {ok:false,error:'重开后动画数量不符'};const a=animation[0];if(a.ShapeId!==shapes[0].id||a.AnimClass!=='Entrance'||a.AnimType!==1||a.AnimDuration!==1||a.AnimDelay!==0||a.AnimTriggerType!=='ClickEffect')return {ok:false,error:'重开后目标出现动画未持久化'};results.push({slideId:slide.id,shapeId:shapes[0].id,animation})}return {ok:true,results};})()`)
}
