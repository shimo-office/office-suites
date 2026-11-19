package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type phase3SlidePlaceholderSpec struct {
	Name string `json:"shapeName"`
	Text string `json:"text"`
}

func preparePhase3SlidePlaceholder(p map[string]any) (*phase3SlidePlaceholderSpec, error) {
	if len(p) != 3 {
		return nil, fmt.Errorf("占位符填充仅接受 slideId、shapeName、text")
	}
	name, nok := p["shapeName"].(string)
	text, tok := p["text"].(string)
	if !nok || name == "" || len(name) > 250 || strings.ContainsAny(name, "\r\n\x00") || !tok || text == "" || len(text) > 20000 || strings.ContainsAny(text, "\r\t\x00") {
		return nil, fmt.Errorf("请提供明确占位符名称及非空正文，以换行分段")
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			return nil, fmt.Errorf("暂不接受空段落")
		}
	}
	return &phase3SlidePlaceholderSpec{Name: name, Text: text}, nil
}
func phase3PlaceholderTarget(n *artifactNode, s phase3SlidePlaceholderSpec, verify bool) (*artifactNode, error) {
	if phase3ShapeName(n) != s.Name {
		return nil, fmt.Errorf("占位符名称不符")
	}
	if e := phase3VisibleShape(n); e != nil {
		return nil, e
	}
	nv := phase3SlideChild(n, artifactSlideNS, "nvSpPr")
	pr := phase3SlideChild(nv, artifactSlideNS, "nvPr")
	ph := phase3SlideChild(pr, artifactSlideNS, "ph")
	if ph == nil {
		return nil, fmt.Errorf("目标并非真实占位符")
	}
	switch ph.attr("type") {
	case "title", "ctrTitle", "body", "subTitle":
	default:
		return nil, fmt.Errorf("仅填充标题或正文占位符，不处理图片/日期/页码占位符")
	}
	tx := phase3SlideChild(n, artifactSlideNS, "txBody")
	if tx == nil {
		return nil, fmt.Errorf("缺少占位符正文容器")
	}
	paragraphs := []string{}
	for _, p := range tx.Children {
		if p.Name.Space == artifactDrawingNS && p.Name.Local == "p" {
			paragraphs = append(paragraphs, phase3SlideText(p))
		}
	}
	if verify {
		if e := phase3PlaceholderVisibleText(tx); e != nil {
			return nil, e
		}
		if !reflect.DeepEqual(paragraphs, strings.Split(s.Text, "\n")) {
			return nil, fmt.Errorf("占位符文本或分段不符")
		}
	} else {
		for _, p := range paragraphs {
			if strings.TrimSpace(p) != "" {
				return nil, fmt.Errorf("只填充空占位符，禁止覆盖现有正文")
			}
		}
	}
	// The new paragraphs replace the empty placeholder content. Retain bodyPr,
	// list style, placeholder identity/type, transform and all other shape data.
	outTx := *tx
	outTx.Children = nil
	for _, c := range tx.Children {
		if c.Name.Space == artifactDrawingNS && c.Name.Local == "p" {
			continue
		}
		outTx.Children = append(outTx.Children, c)
	}
	out := *n
	out.Children = append([]*artifactNode(nil), n.Children...)
	for i, c := range out.Children {
		if c == tx {
			out.Children[i] = &outTx
		}
	}
	return phase3PlaceholderNativeBW(&out), nil
}
func phase3PlaceholderFilter(ids []string, s phase3SlidesSpec, verify bool) func(int, *artifactNode) (*artifactNode, error) {
	return func(i int, n *artifactNode) (*artifactNode, error) {
		if ids[i] != s.SlideID {
			return n, nil
		}
		return phase3MapSlideShape(n, func(x *artifactNode) bool { return phase3ShapeName(x) == s.Placeholder.Name }, func(x *artifactNode) (*artifactNode, error) {
			return phase3PlaceholderTarget(x, *s.Placeholder, verify)
		})
	}
}
func verifyPhase3SlidePlaceholderArtifacts(before, after string, ids, next []string, s phase3SlidesSpec) error {
	for _, filename := range []string{before, after} {
		if e := phase3PlaceholderBWPreflight(filename); e != nil {
			return e
		}
	}
	if s.Placeholder == nil || !reflect.DeepEqual(ids, next) {
		return fmt.Errorf("占位符填充不得改变页面列表")
	}
	b, e := readPhase3SlideArtifactFiltered(before, ids, phase3PlaceholderFilter(ids, s, false))
	if e != nil {
		return e
	}
	a, e := readPhase3SlideArtifactFiltered(after, next, phase3PlaceholderFilter(next, s, true))
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(b, a) {
		return fmt.Errorf("占位符类型/关系或其余内容发生变化")
	}
	return nil
}
func phase3SlidePlaceholderMutationJS(s phase3SlidesSpec, ids []string) string {
	raw, _ := json.Marshal(s)
	list, _ := json.Marshal(ids)
	// SDK setContent joins strings with a soft break. Multi-paragraph content
	// instead uses the same history-aware action as the native Enter handler.
	// Check every required method before clearing the empty target.
	return `(()=>{const r=` + string(raw) + `;const expected=` + string(list) + `;` + phase3PresenceGuardJS() + `
 const sdk=window.presentationSDK,wb=sdk?.editor?.workbench,api=sdk?.API?.slides;if(wb?.isReady()!==true||typeof wb.permissions?.canEdit!=='function'||wb.permissions.canEdit()!==true)return {ok:false,error:'未确认编辑权限'};if(JSON.stringify(api.getSlides().map(s=>s.id))!==JSON.stringify(expected))return {ok:false,error:'页面已变化'};const slide=api.getSlides().find(s=>s.id===r.slideId),shapes=slide?.getShapes().filter(s=>s.name===r.placeholder.shapeName);if(shapes?.length!==1)return {ok:false,error:'占位符名称不唯一'};const shape=shapes[0];if(typeof shape.textContent!=='string'||shape.textContent.trim()!==''||typeof shape.setContent!=='function')return {ok:false,error:'只填充空文本占位符'};const lines=r.placeholder.text.split('\n');if(lines.length===1){shape.setContent(lines[0])}else{const c=wb.core,e=wb.editor,handler=e?._$editorUI$_?._$editorHandler$_;if(['setTextRange','selectElementByIds','clearContent','addText'].some(k=>typeof c?.[k]!=='function')||typeof handler?._$addNewParagraph$_!=='function'||typeof api.setCurrentSlideIndex!=='function'||typeof api.setSelectedSlides!=='function')return {ok:false,error:'缺少真实段落插入接口，未写入'};api.setCurrentSlideIndex(slide.id);api.setSelectedSlides([slide.id]);c.setTextRange(e,null);c.selectElementByIds(e,[shape.id]);c.clearContent(e);for(let i=0;i<lines.length;i++){if(i>0)handler._$addNewParagraph$_();c.addText(e,lines[i])}}return {ok:true,shapeId:shape.id,shapeName:shape.name,text:shape.textContent};})()`
}

// Reject explicit invisible text. Inherited typography remains attached to the
// original placeholder and is also inspected in the rendered live acceptance.
func phase3PlaceholderVisibleText(n *artifactNode) error {
	if n.Name.Space == artifactDrawingNS {
		if n.Name.Local == "rPr" || n.Name.Local == "defRPr" || n.Name.Local == "endParaRPr" {
			if size := n.attr("sz"); size != "" {
				v, e := strconv.Atoi(size)
				if e != nil || v <= 0 {
					return fmt.Errorf("占位符文字字号不可见")
				}
			}
			if phase3SlideChild(n, artifactDrawingNS, "noFill") != nil {
				return fmt.Errorf("占位符文字未填充")
			}
		}
		if n.Name.Local == "alpha" && n.attr("val") != "100000" {
			return fmt.Errorf("占位符文字透明度未经验证")
		}
	}
	for _, c := range n.Children {
		if e := phase3PlaceholderVisibleText(c); e != nil {
			return e
		}
	}
	return nil
}
