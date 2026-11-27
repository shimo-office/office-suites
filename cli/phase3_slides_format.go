package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

type phase3SlideFormatSpec struct {
	Text  string         `json:"text"`
	Attrs map[string]any `json:"attrs"`
}

func preparePhase3SlideFormat(p map[string]any) (*phase3SlideFormatSpec, error) {
	outline, e := phase3PrepareOutline(p)
	if e != nil {
		return nil, e
	}
	attrs := map[string]any{}
	for k, v := range p {
		switch k {
		case "slideId", "text", "outlineColor", "outlineWidth":
		case "fontFamily", "fontSize", "color", "bold", "italic", "underline":
			attrs[k] = v
		default:
			return nil, fmt.Errorf("暂未证实文字格式参数 %s；文字轮廓不能用形状边框代替", k)
		}
	}
	outlineOnly := len(attrs) == 0 && outline != nil
	if outlineOnly {
		attrs["color"] = outline["outlineColor"]
	}
	// Reuse the document run validator for the shared font/size/color primitives.
	checked, err := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": p["text"], "attrs": attrs})
	if err != nil {
		return nil, err
	}
	if outlineOnly {
		delete(checked.Attrs, "color")
	}
	for k, v := range outline {
		checked.Attrs[k] = v
	}
	return &phase3SlideFormatSpec{Text: checked.Text, Attrs: checked.Attrs}, nil
}
func phase3VisibleShape(n *artifactNode) error {
	nv := phase3SlideChild(n, artifactSlideNS, "nvSpPr")
	identity := phase3SlideChild(nv, artifactSlideNS, "cNvPr")
	if identity == nil {
		return fmt.Errorf("缺少文本对象标识")
	}
	hidden := identity.attr("hidden")
	if hidden != "" && hidden != "0" && hidden != "false" {
		return fmt.Errorf("文本对象被隐藏")
	}
	return nil
}
func verifyPhase3SlideTextFormatNode(n *artifactNode, s phase3SlideFormatSpec) error {
	if n == nil || n.Name.Space != artifactSlideNS || n.Name.Local != "sp" || phase3SlideText(n) != s.Text {
		return fmt.Errorf("目标文本不符")
	}
	if err := phase3VisibleShape(n); err != nil {
		return err
	}
	runs := 0
	var walk func(*artifactNode) error
	walk = func(node *artifactNode) error {
		if node.Name.Space == artifactDrawingNS && node.Name.Local == "r" && phase3SlideText(node) != "" {
			runs++
			rp := phase3SlideChild(node, artifactDrawingNS, "rPr")
			if rp == nil {
				return fmt.Errorf("文本run未显式保存请求格式")
			}
			if e := phase3VerifyOutline(rp, s.Attrs); e != nil {
				return e
			}
			for key, value := range s.Attrs {
				switch key {
				case "fontSize":
					size, e := strconv.ParseInt(rp.attr("sz"), 10, 64)
					if e != nil || float64(size) != value.(float64)*100 {
						return fmt.Errorf("字号不符")
					}
				case "fontFamily":
					font := phase3SlideChild(rp, artifactDrawingNS, "latin")
					if font == nil || font.attr("typeface") != value.(string) {
						return fmt.Errorf("字体不符")
					}
					for _, tag := range []string{"ea", "cs"} {
						f := phase3SlideChild(rp, artifactDrawingNS, tag)
						if (f == nil && strings.IndexFunc(phase3SlideText(node), func(r rune) bool { return r > unicode.MaxASCII }) >= 0) || (f != nil && f.attr("typeface") != value.(string)) {
							return fmt.Errorf("多脚本字体不一致")
						}
					}
				case "color":
					fill := phase3SlideChild(rp, artifactDrawingNS, "solidFill")
					color := phase3SlideChild(fill, artifactDrawingNS, "srgbClr")
					if color == nil || strings.ToUpper(color.attr("val")) != strings.TrimPrefix(value.(string), "#") {
						return fmt.Errorf("文字填充色不符")
					}
					for _, child := range color.Children {
						if child.Name.Space != artifactDrawingNS || child.Name.Local != "alpha" || child.attr("val") != "100000" {
							return fmt.Errorf("文字填充被设透明")
						}
					}
				case "bold", "italic":
					key := map[string]string{"bold": "b", "italic": "i"}[key]
					v := rp.attr(key)
					if v != "" && v != "0" && v != "1" && v != "true" && v != "false" {
						return fmt.Errorf("未知布尔格式")
					}
					if v == "" || (v == "1" || v == "true") != value.(bool) {
						return fmt.Errorf("文字%s不符", key)
					}
				case "underline":
					v := rp.attr("u")
					if value.(bool) {
						if v != "sng" {
							return fmt.Errorf("下划线不符")
						}
					} else if v != "none" {
						return fmt.Errorf("下划线未取消")
					}
				}
			}
		}
		for _, c := range node.Children {
			if e := walk(c); e != nil {
				return e
			}
		}
		return nil
	}
	if e := walk(n); e != nil {
		return e
	}
	if runs == 0 {
		return fmt.Errorf("无可验证文本run")
	}
	return nil
}
func phase3MaskSlideTextFormat(n *artifactNode, attrs map[string]any) *artifactNode {
	out := *n
	out.Attr = append([]xml.Attr(nil), n.Attr...)
	out.Children = nil
	style := n.Name.Space == artifactDrawingNS && (n.Name.Local == "rPr" || n.Name.Local == "defRPr" || n.Name.Local == "endParaRPr")
	if style {
		out.Attr = nil
		for _, a := range n.Attr {
			key := map[string]string{"sz": "fontSize", "b": "bold", "i": "italic", "u": "underline"}[a.Name.Local]
			if _, remove := attrs[key]; a.Name.Space == "" && key != "" && remove {
				continue
			}
			out.Attr = append(out.Attr, a)
		}
	}
	for _, c := range n.Children {
		if style && c.Name.Space == artifactDrawingNS {
			if _, ok := attrs["outlineColor"]; ok && c.Name.Local == "ln" {
				continue
			}
			if _, ok := attrs["fontFamily"]; ok && (c.Name.Local == "latin" || c.Name.Local == "ea" || c.Name.Local == "cs" || c.Name.Local == "sym") {
				continue
			}
			if _, ok := attrs["color"]; ok && (c.Name.Local == "solidFill" || c.Name.Local == "gradFill" || c.Name.Local == "noFill" || c.Name.Local == "pattFill") {
				continue
			}
		}
		child := phase3MaskSlideTextFormat(c, attrs)
		emptyStyle := child.Name.Space == artifactDrawingNS && (child.Name.Local == "rPr" || child.Name.Local == "defRPr" || child.Name.Local == "endParaRPr") && len(child.Attr) == 0 && len(child.Children) == 0 && child.Text == ""
		if !emptyStyle {
			out.Children = append(out.Children, child)
		}
	}
	return &out
}
func phase3MapSlideShape(slide *artifactNode, match func(*artifactNode) bool, apply func(*artifactNode) (*artifactNode, error)) (*artifactNode, error) {
	cs := slide.child("cSld")
	if cs == nil {
		return nil, fmt.Errorf("缺少页面内容")
	}
	tree := cs.child("spTree")
	if tree == nil {
		return nil, fmt.Errorf("缺少形状树")
	}
	at := -1
	for i, n := range tree.Children {
		if n.Name.Space == artifactSlideNS && n.Name.Local == "sp" && match(n) {
			if at >= 0 {
				return nil, fmt.Errorf("目标对象不唯一，未执行写入")
			}
			at = i
		}
	}
	if at < 0 {
		return nil, fmt.Errorf("目标对象不存在；不支持定位到组合内的子对象")
	}
	value, err := apply(tree.Children[at])
	if err != nil {
		return nil, err
	}
	outTree := *tree
	outTree.Children = append([]*artifactNode(nil), tree.Children...)
	outTree.Children[at] = value
	outCS := *cs
	outCS.Children = append([]*artifactNode(nil), cs.Children...)
	for i, n := range outCS.Children {
		if n == tree {
			outCS.Children[i] = &outTree
		}
	}
	out := *slide
	out.Children = append([]*artifactNode(nil), slide.Children...)
	for i, n := range out.Children {
		if n == cs {
			out.Children[i] = &outCS
		}
	}
	return &out, nil
}
func phase3SlideTextFormatFilter(ids []string, s phase3SlidesSpec, verify, preflight bool) func(int, *artifactNode) (*artifactNode, error) {
	return func(i int, n *artifactNode) (*artifactNode, error) {
		if ids[i] != s.SlideID {
			return n, nil
		}
		return phase3MapSlideShape(n, func(shape *artifactNode) bool { return phase3SlideText(shape) == s.Format.Text }, func(shape *artifactNode) (*artifactNode, error) {
			if e := phase3VisibleShape(shape); e != nil {
				return nil, e
			}
			check := verifyPhase3SlideTextFormatNode(shape, *s.Format)
			if verify && check != nil {
				return nil, check
			}
			if preflight && check == nil {
				return nil, fmt.Errorf("目标文字已满足全部格式，未执行无效写入")
			}
			return phase3MaskSlideTextFormat(shape, s.Format.Attrs), nil
		})
	}
}
func verifyPhase3SlideTextFormatArtifacts(beforePath, afterPath string, beforeIDs, afterIDs []string, s phase3SlidesSpec) error {
	if s.Format == nil || !reflect.DeepEqual(beforeIDs, afterIDs) {
		return fmt.Errorf("文字格式不得改变页面ID/顺序")
	}
	found := false
	for _, id := range beforeIDs {
		if id == s.SlideID {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("目标页不存在")
	}
	if _, ok := s.Format.Attrs["outlineColor"]; ok {
		if e := phase3CheckNewOutlineArtifact(beforePath, beforeIDs, s); e != nil {
			return e
		}
	}
	bf, e := phase3SlideEditedDefaultsFilter(beforePath, beforeIDs, s, phase3SlideTextFormatFilter(beforeIDs, s, false, false))
	if e != nil {
		return e
	}
	af, e := phase3SlideEditedDefaultsFilter(afterPath, afterIDs, s, phase3SlideTextFormatFilter(afterIDs, s, true, false))
	if e != nil {
		return e
	}
	before, e := readPhase3SlideArtifactFiltered(beforePath, beforeIDs, bf)
	if e != nil {
		return e
	}
	after, e := readPhase3SlideArtifactFiltered(afterPath, afterIDs, af)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("除请求的文字格式外，其他内容/格式/资源发生变化")
	}
	return nil
}
func phase3SlideTextFormatMutationJS(s phase3SlidesSpec, expected []string) string {
	raw, _ := json.Marshal(s)
	ids, _ := json.Marshal(expected)
	return `(()=>{const request=` + string(raw) + `;const expected=` + string(ids) + `;` + phase3PresenceGuardJS() + `
const sdk=window.presentationSDK;const wb=sdk?.editor?.workbench;if(wb?.isReady()!==true||typeof wb.permissions?.canEdit!=='function'||wb.permissions.canEdit()!==true)return {ok:false,error:'未确认编辑权限'};const api=sdk.API.slides;const slides=api.getSlides();if(JSON.stringify(slides.map(s=>s.id))!==JSON.stringify(expected))return {ok:false,error:'页面列表变化'};const slide=slides.find(s=>s.id===request.slideId);if(!slide)return {ok:false,error:'页面不存在'};const targets=slide.getShapes().filter(s=>s.textContent===request.textFormat.text);if(targets.length!==1)return {ok:false,error:'目标文字不唯一'};const p=request.textFormat.attrs;const methods={fontFamily:'setFont',fontSize:'setFontSize',color:'setTextColor',bold:'setBold',italic:'setItalic',underline:'setUnderline'};if(Object.keys(p).some(k=>!['outlineColor','outlineWidth'].includes(k)&&(!methods[k]||typeof wb.core[methods[k]]!=='function')))return {ok:false,error:'缺少请求格式的已验证方法'};` + phase3SlideOutlinePreflightJS() + `api.setCurrentSlideIndex(slide.id);api.setSelectedSlides([slide.id]);wb.core.setTextRange(wb.editor,null);wb.core.selectElementByIds(wb.editor,[targets[0].id]);if(p.fontFamily!==undefined)wb.core.setFont(wb.editor,p.fontFamily);if(p.fontSize!==undefined)wb.core.setFontSize(wb.editor,p.fontSize);if(p.color!==undefined)wb.core.setTextColor(wb.editor,p.color);if(p.bold!==undefined)wb.core.setBold(wb.editor,p.bold);if(p.italic!==undefined)wb.core.setItalic(wb.editor,p.italic);if(p.underline!==undefined)wb.core.setUnderline(wb.editor,p.underline);` + phase3SlideOutlineApplyJS() + `return {ok:true,shapeId:targets[0].id,text:targets[0].textContent,attrs:p};})()`
}
