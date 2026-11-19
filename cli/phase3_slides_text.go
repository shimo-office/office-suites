package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

type phase3SlideTextSpec struct {
	Text     string  `json:"text"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	FontSize float64 `json:"fontSize"`
	Bold     bool    `json:"bold"`
}

func preparePhase3SlideText(p map[string]any) (*phase3SlideTextSpec, error) {
	s := &phase3SlideTextSpec{FontSize: 24}
	allowed := map[string]bool{"slideId": true, "text": true, "x": true, "y": true, "width": true, "height": true, "fontSize": true, "bold": true}
	for k := range p {
		if !allowed[k] {
			return nil, fmt.Errorf("未知文本框参数 %s", k)
		}
	}
	text, ok := p["text"].(string)
	if !ok || strings.TrimSpace(text) == "" || strings.ContainsAny(text, "\r\n\t\x00") || utf8.RuneCountInString(text) > 10000 {
		return nil, fmt.Errorf("text 必须为不含换行的非空文本，最多10000字符")
	}
	s.Text = text
	for _, f := range []struct {
		name     string
		dest     *float64
		min, max float64
	}{{"x", &s.X, 0, 10000}, {"y", &s.Y, 0, 10000}, {"width", &s.Width, 1, 10000}, {"height", &s.Height, 1, 10000}, {"fontSize", &s.FontSize, 1, 400}} {
		v, exists := p[f.name]
		if f.name == "fontSize" && !exists {
			continue
		}
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < f.min || n > f.max || math.Trunc(n*2) != n*2 {
			return nil, fmt.Errorf("%s 必须为 %.0f–%.0f 范围的数值，最小单位0.5", f.name, f.min, f.max)
		}
		*f.dest = n
	}
	if v, exists := p["bold"]; exists {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("bold 必须为bool")
		}
		s.Bold = b
	}
	return s, nil
}
func phase3SlideChild(n *artifactNode, space, local string) *artifactNode {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Name.Space == space && c.Name.Local == local {
			return c
		}
	}
	return nil
}
func phase3SlideText(n *artifactNode) string {
	if n == nil {
		return ""
	}
	s := ""
	if n.Name.Space == artifactDrawingNS && n.Name.Local == "t" {
		s = n.Text
	}
	for _, c := range n.Children {
		s += phase3SlideText(c)
	}
	return s
}
func verifyPhase3SlideTextNode(n *artifactNode, s phase3SlideTextSpec) error {
	if n == nil || n.Name.Space != artifactSlideNS || n.Name.Local != "sp" || phase3SlideText(n) != s.Text {
		return fmt.Errorf("新增文本框正文不符")
	}
	nv := phase3SlideChild(n, artifactSlideNS, "nvSpPr")
	identity := phase3SlideChild(nv, artifactSlideNS, "cNvPr")
	if identity == nil {
		return fmt.Errorf("新增文本框缺少可见性标记节点")
	}
	hidden := identity.attr("hidden")
	if hidden != "" && hidden != "0" && hidden != "false" {
		return fmt.Errorf("新增文本框被隐藏或可见性不明确")
	}
	props := phase3SlideChild(n, artifactSlideNS, "spPr")
	transform := phase3SlideChild(props, artifactDrawingNS, "xfrm")
	if transform != nil {
		for _, key := range []string{"rot", "flipH", "flipV"} {
			v := transform.attr(key)
			if v != "" && v != "0" && v != "false" {
				return fmt.Errorf("文本框含未请求的旋转/翻转")
			}
		}
	}
	off := phase3SlideChild(transform, artifactDrawingNS, "off")
	ext := phase3SlideChild(transform, artifactDrawingNS, "ext")
	if off == nil || ext == nil {
		return fmt.Errorf("新增文本框缺少明确位置/尺寸")
	}
	for _, v := range []struct {
		node *artifactNode
		key  string
		want float64
	}{{off, "x", s.X}, {off, "y", s.Y}, {ext, "cx", s.Width}, {ext, "cy", s.Height}} {
		n, e := strconv.ParseInt(v.node.attr(v.key), 10, 64)
		if e != nil || math.Abs(float64(n)-v.want*9525) > 0.5 {
			return fmt.Errorf("文本框 %s 与指定像素坐标/尺寸不符", v.key)
		}
	}
	body := phase3SlideChild(n, artifactSlideNS, "txBody")
	if body == nil {
		return fmt.Errorf("缺少文本内容")
	}
	paragraphs := 0
	runs := 0
	for _, p := range body.Children {
		if p.Name.Space != artifactDrawingNS || p.Name.Local != "p" {
			continue
		}
		paragraphs++
		for _, r := range p.Children {
			if r.Name.Space != artifactDrawingNS {
				continue
			}
			if r.Name.Local == "br" || r.Name.Local == "fld" {
				return fmt.Errorf("新增文本框包含未请求的换行/字段")
			}
			if r.Name.Local != "r" {
				continue
			}
			runs++
			rp := phase3SlideChild(r, artifactDrawingNS, "rPr")
			if rp == nil {
				return fmt.Errorf("文本字号/粗体未显式保存")
			}
			size, e := strconv.ParseInt(rp.attr("sz"), 10, 64)
			if e != nil || float64(size) != s.FontSize*100 {
				return fmt.Errorf("文本字号与请求不符")
			}
			bold := rp.attr("b")
			if bold != "" && bold != "0" && bold != "1" && bold != "true" && bold != "false" {
				return fmt.Errorf("未知粗体属性")
			}
			if (bold == "1" || bold == "true") != s.Bold {
				return fmt.Errorf("文本粗体与请求不符")
			}
		}
	}
	if paragraphs != 1 || runs == 0 {
		return fmt.Errorf("新增文本框不是单段可核对正文")
	}
	return nil
}
func phase3SlideTextFilter(s phase3SlidesSpec, remove bool) func(int, *artifactNode) (*artifactNode, error) {
	return func(index int, slide *artifactNode) (*artifactNode, error) {
		tree := slide.child("cSld")
		if tree == nil {
			return nil, fmt.Errorf("缺少页面结构")
		}
		shapes := tree.child("spTree")
		if shapes == nil {
			return nil, fmt.Errorf("缺少页面对象树")
		}
		candidates := []int{}
		for i, n := range shapes.Children {
			if n.Name.Space == artifactSlideNS && n.Name.Local == "sp" && phase3SlideText(n) == s.Text.Text {
				candidates = append(candidates, i)
			}
		}
		if !remove {
			if len(candidates) > 0 {
				return nil, fmt.Errorf("目标页已含相同正文，拒绝无法区分的重复插入")
			}
			return slide, nil
		}
		if len(candidates) != 1 {
			return nil, fmt.Errorf("导出页必须恰有一个新增正文匹配文本框")
		}
		at := candidates[0]
		if e := verifyPhase3SlideTextNode(shapes.Children[at], *s.Text); e != nil {
			return nil, e
		}
		// Rebuild only the three ancestor nodes; caller-owned trees stay unchanged.
		outShapes := *shapes
		outShapes.Children = append(append([]*artifactNode(nil), shapes.Children[:at]...), shapes.Children[at+1:]...)
		outTree := *tree
		outTree.Children = append([]*artifactNode(nil), tree.Children...)
		for i, n := range outTree.Children {
			if n == shapes {
				outTree.Children[i] = &outShapes
			}
		}
		outSlide := *slide
		outSlide.Children = append([]*artifactNode(nil), slide.Children...)
		for i, n := range outSlide.Children {
			if n == tree {
				outSlide.Children[i] = &outTree
			}
		}
		return &outSlide, nil
	}
}
func phase3SlideTextTargetFilter(ids []string, s phase3SlidesSpec, remove bool) func(int, *artifactNode) (*artifactNode, error) {
	return func(index int, n *artifactNode) (*artifactNode, error) {
		if ids[index] != s.SlideID {
			return n, nil
		}
		return phase3SlideTextFilter(s, remove)(index, n)
	}
}
func verifyPhase3SlideTextArtifacts(beforePath, afterPath string, beforeIDs, afterIDs []string, s phase3SlidesSpec) error {
	if s.Text == nil || !reflect.DeepEqual(beforeIDs, afterIDs) {
		return fmt.Errorf("文本插入不得改变页面 ID/顺序")
	}
	found := false
	for _, id := range beforeIDs {
		if id == s.SlideID {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("文本目标页不存在")
	}
	before, err := readPhase3SlideArtifactFiltered(beforePath, beforeIDs, phase3SlideTextTargetFilter(beforeIDs, s, false))
	if err != nil {
		return err
	}
	after, err := readPhase3SlideArtifactFiltered(afterPath, afterIDs, phase3SlideTextTargetFilter(afterIDs, s, true))
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("除指定新增文本框外，原页面内容/样式/主题出现变化")
	}
	return nil
}

func phase3SlideTextBounds(filename string, s phase3SlideTextSpec) error {
	z, entries, err := openArtifact(filename)
	if err != nil {
		return err
	}
	defer z.Close()
	root, err := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if err != nil {
		return err
	}
	size := root.child("sldSz")
	if size == nil {
		return fmt.Errorf("导出缺少页面尺寸，未执行写入")
	}
	cx, e1 := strconv.ParseInt(size.attr("cx"), 10, 64)
	cy, e2 := strconv.ParseInt(size.attr("cy"), 10, 64)
	if e1 != nil || e2 != nil || cx <= 0 || cy <= 0 {
		return fmt.Errorf("页面尺寸无效")
	}
	if (s.X+s.Width)*9525 > float64(cx) || (s.Y+s.Height)*9525 > float64(cy) {
		return fmt.Errorf("文本框将超出幻灯片范围，未执行写入")
	}
	return nil
}
func phase3SlideTextMutationJS(s phase3SlidesSpec, expected []string) string {
	request, _ := json.Marshal(s)
	ids, _ := json.Marshal(expected)
	return `(async()=>{const request=` + string(request) + `;const expected=` + string(ids) + `;` + phase3PresenceGuardJS() + `
const sdk=window.presentationSDK;const wb=sdk?.editor?.workbench;if(wb?.isReady()!==true||typeof wb.permissions?.canEdit!=='function'||wb.permissions.canEdit()!==true)return {ok:false,error:'未确认编辑权限，未写入'};const api=sdk.API?.slides;if(!api)return {ok:false,error:'缺少幻灯片SDK'};const slides=api.getSlides();if(JSON.stringify(slides.map(s=>s.id))!==JSON.stringify(expected))return {ok:false,error:'页面列表已变化'};const slide=slides.find(s=>s.id===request.slideId);if(!slide)return {ok:false,error:'指定页面不存在'};const p=request.textBox;const before=slide.getShapes().map(s=>({id:s.id,text:s.textContent||''}));if(before.some(s=>s.text===p.text))return {ok:false,error:'相同文本已存在，未重复插入'};
const shape=await slide.insertTextBox({offset:{x:p.x,y:p.y},size:{width:p.width,height:p.height},content:[{text:p.text,textStyle:{fontSize:p.fontSize,bold:p.bold}}]});if(!shape?.id||before.some(s=>s.id===shape.id))return {ok:false,error:'未取得新增文本框ID，禁止重放'};const after=slide.getShapes().map(s=>({id:s.id,text:s.textContent||''}));if(after.length!==before.length+1||!after.some(s=>s.id===shape.id&&s.text===p.text)||before.some(s=>!after.some(a=>a.id===s.id&&a.text===s.text)))return {ok:false,error:'新文本框读回与请求不符，禁止重放'};return {ok:true,shapeId:shape.id,before,after,presenceSource:'COLLABROOM/RECONNECT_READY'};})()`
}
