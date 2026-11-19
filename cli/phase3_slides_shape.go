package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

type phase3SlideShapeSpec struct {
	Name             string   `json:"shapeName"`
	FillColor        string   `json:"fillColor,omitempty"`
	LineColor        string   `json:"lineColor,omitempty"`
	Rotation         *float64 `json:"rotation,omitempty"`
	HorizontalCenter bool     `json:"horizontalCenter,omitempty"`
	BehindName       string   `json:"behindName,omitempty"`
	Delta            float64  `json:"rotationDelta,omitempty"`
	CenterX          int64    `json:"centerXEMU,omitempty"`
	BackwardSteps    int      `json:"backwardSteps,omitempty"`
}

func preparePhase3SlideShape(p map[string]any) (*phase3SlideShapeSpec, error) {
	s := &phase3SlideShapeSpec{}
	s.Name, _ = p["shapeName"].(string)
	if s.Name == "" || len(s.Name) > 250 || strings.ContainsAny(s.Name, "\r\n\x00") {
		return nil, fmt.Errorf("shapeName 必须是唯一的现有形状名")
	}
	changes := 0
	for k, v := range p {
		switch k {
		case "slideId", "shapeName":
		case "fillColor", "lineColor":
			checked, e := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": "shape", "attrs": map[string]any{"color": v}})
			if e != nil {
				return nil, e
			}
			color := checked.Attrs["color"].(string)
			if k == "fillColor" {
				s.FillColor = color
			} else {
				s.LineColor = color
			}
			changes++
		case "rotation":
			n, ok := v.(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n >= 360 || n*60000 != math.Trunc(n*60000) {
				return nil, fmt.Errorf("rotation 必须为 [0,360) 内可表达的绝对角度")
			}
			s.Rotation = &n
			changes++
		case "horizontalCenter":
			if v != true {
				return nil, fmt.Errorf("horizontalCenter 仅接受 true")
			}
			s.HorizontalCenter = true
			changes++
		case "behindName":
			name, ok := v.(string)
			if !ok || name == "" || name == s.Name || len(name) > 250 || strings.ContainsAny(name, "\r\n\x00") {
				return nil, fmt.Errorf("behindName 必须指定另一个现有对象")
			}
			s.BehindName = name
			changes++
		default:
			return nil, fmt.Errorf("未证实形状参数 %s", k)
		}
	}
	if changes == 0 {
		return nil, fmt.Errorf("至少指定一项形状格式")
	}
	return s, nil
}
func phase3ShapeName(n *artifactNode) string {
	for _, tag := range []string{"nvSpPr", "nvPicPr", "nvGraphicFramePr", "nvCxnSpPr", "nvGrpSpPr"} {
		nv := phase3SlideChild(n, artifactSlideNS, tag)
		id := phase3SlideChild(nv, artifactSlideNS, "cNvPr")
		if id != nil {
			return id.attr("name")
		}
	}
	return ""
}
func phase3ShapeInteger(n *artifactNode, key string) (int64, error) {
	if n == nil {
		return 0, fmt.Errorf("缺少形状几何节点")
	}
	v, e := strconv.ParseInt(n.attr(key), 10, 64)
	if e != nil {
		return 0, fmt.Errorf("形状几何参数 %s 无效", key)
	}
	return v, nil
}
func phase3ShapeColor(n *artifactNode, want string) error {
	fill := phase3SlideChild(n, artifactDrawingNS, "solidFill")
	c := phase3SlideChild(fill, artifactDrawingNS, "srgbClr")
	if c == nil || strings.ToUpper(c.attr("val")) != strings.TrimPrefix(want, "#") {
		return fmt.Errorf("形状颜色不符")
	}
	for _, x := range c.Children {
		if x.Name.Space != artifactDrawingNS || x.Name.Local != "alpha" || x.attr("val") != "100000" {
			return fmt.Errorf("形状颜色含未验证的透明度/变换")
		}
	}
	for _, x := range n.Children {
		if x.Name.Space == artifactDrawingNS && (x.Name.Local == "noFill" || x.Name.Local == "gradFill" || x.Name.Local == "pattFill" || x.Name.Local == "blipFill") {
			return fmt.Errorf("形状包含冲突填充")
		}
	}
	return nil
}
func verifyPhase3SlideShapeNode(n *artifactNode, s phase3SlideShapeSpec) error {
	if n == nil || n.Name.Space != artifactSlideNS || n.Name.Local != "sp" || phase3ShapeName(n) != s.Name {
		return fmt.Errorf("目标形状不符")
	}
	if e := phase3VisibleShape(n); e != nil {
		return e
	}
	sp := phase3SlideChild(n, artifactSlideNS, "spPr")
	if sp == nil {
		return fmt.Errorf("缺少显式形状属性")
	}
	if s.FillColor != "" {
		if e := phase3ShapeColor(sp, s.FillColor); e != nil {
			return e
		}
	}
	if s.LineColor != "" {
		line := phase3SlideChild(sp, artifactDrawingNS, "ln")
		if line == nil {
			return fmt.Errorf("缺少显式形状边框")
		}
		if e := phase3ShapeColor(line, s.LineColor); e != nil {
			return e
		}
	}
	xf := phase3SlideChild(sp, artifactDrawingNS, "xfrm")
	if s.Rotation != nil {
		r := int64(0)
		if xf == nil {
			return fmt.Errorf("缺少旋转节点")
		}
		if xf.attr("rot") != "" {
			var e error
			r, e = phase3ShapeInteger(xf, "rot")
			if e != nil {
				return e
			}
		}
		// Native rotate converts degrees using a truncated PI constant, then its
		// DrawingML serializer truncates radians to an integer angle. Bound this
		// target-only quantization to one unit (1/60000 degree), never two.
		if math.Abs(float64(r)-*s.Rotation*60000) > 1 {
			return fmt.Errorf("绝对旋转角度不符")
		}
	}
	if s.HorizontalCenter {
		x, e := phase3ShapeInteger(phase3SlideChild(xf, artifactDrawingNS, "off"), "x")
		if e != nil || x != s.CenterX {
			return fmt.Errorf("未相对页面水平居中")
		}
	}
	return nil
}

// Mask only the explicitly requested properties. Geometry, other styling and text remain covered.
func phase3MaskShape(n *artifactNode, s phase3SlideShapeSpec, parent string) *artifactNode {
	out := *n
	out.Attr = nil
	out.Children = nil
	for _, a := range n.Attr {
		if a.Name.Space == "" && ((n.Name.Local == "xfrm" && a.Name.Local == "rot" && s.Rotation != nil) || (n.Name.Local == "off" && parent == "xfrm" && a.Name.Local == "x" && s.HorizontalCenter)) {
			continue
		}
		out.Attr = append(out.Attr, a)
	}
	for _, c := range n.Children {
		fill := (n.Name.Space == artifactSlideNS && n.Name.Local == "spPr" && s.FillColor != "") || (n.Name.Space == artifactDrawingNS && n.Name.Local == "ln" && parent == "spPr" && s.LineColor != "")
		if fill && c.Name.Space == artifactDrawingNS && (c.Name.Local == "solidFill" || c.Name.Local == "noFill" || c.Name.Local == "gradFill" || c.Name.Local == "pattFill" || c.Name.Local == "blipFill") {
			continue
		}
		out.Children = append(out.Children, phase3MaskShape(c, s, n.Name.Local))
	}
	return &out
}
func phase3ShapeTree(slide *artifactNode) (*artifactNode, error) {
	c := phase3SlideChild(slide, artifactSlideNS, "cSld")
	tree := phase3SlideChild(c, artifactSlideNS, "spTree")
	if tree == nil {
		return nil, fmt.Errorf("缺少形状树")
	}
	return tree, nil
}
func phase3ShapePositions(tree *artifactNode, s phase3SlideShapeSpec) (int, int, error) {
	at, behind := -1, -1
	for i, n := range tree.Children {
		if phase3ShapeName(n) == s.Name {
			if at >= 0 {
				return 0, 0, fmt.Errorf("目标形状名重复")
			}
			at = i
		}
		if s.BehindName != "" && phase3ShapeName(n) == s.BehindName {
			if behind >= 0 {
				return 0, 0, fmt.Errorf("参照对象名重复")
			}
			behind = i
		}
	}
	if at < 0 || (s.BehindName != "" && behind < 0) {
		return 0, 0, fmt.Errorf("目标形状或参照对象不存在")
	}
	return at, behind, nil
}
func phase3SlideShapeFilter(ids []string, s phase3SlidesSpec, verify bool) func(int, *artifactNode) (*artifactNode, error) {
	return func(i int, n *artifactNode) (*artifactNode, error) {
		if ids[i] != s.SlideID {
			return n, nil
		}
		tree, e := phase3ShapeTree(n)
		if e != nil {
			return nil, e
		}
		at, behind, e := phase3ShapePositions(tree, *s.Shape)
		if e != nil {
			return nil, e
		}
		if verify && s.Shape.BehindName != "" && at+1 != behind {
			return nil, fmt.Errorf("目标未精确置于参照对象紧邻后方")
		}
		out, e := phase3MapSlideShape(n, func(x *artifactNode) bool { return phase3ShapeName(x) == s.Shape.Name }, func(x *artifactNode) (*artifactNode, error) {
			if verify {
				if e := verifyPhase3SlideShapeNode(x, *s.Shape); e != nil {
					return nil, e
				}
			}
			return phase3MaskShape(phase3ShapeNativeEmptyDefaults(x), *s.Shape, ""), nil
		})
		if e != nil {
			return nil, e
		}
		if s.Shape.BehindName != "" { // Normalize only target position; every other object's relative order remains checked.
			t, _ := phase3ShapeTree(out)
			objects := append([]*artifactNode(nil), t.Children...)
			target := objects[at]
			objects = append(objects[:at], objects[at+1:]...)
			objects = append(objects, target)
			t.Children = objects
		}
		return out, nil
	}
}
func verifyPhase3SlideShapeArtifacts(before, after string, ids, next []string, s phase3SlidesSpec) error {
	for _, filename := range []string{before, after} {
		if err := phase3PlaceholderBWPreflight(filename); err != nil {
			return err
		}
	}
	if s.Shape == nil || !reflect.DeepEqual(ids, next) {
		return fmt.Errorf("形状格式不得改变页面列表")
	}
	b, e := readPhase3SlideArtifactFiltered(before, ids, phase3SlideShapeFilter(ids, s, false))
	if e != nil {
		return e
	}
	a, e := readPhase3SlideArtifactFiltered(after, next, phase3SlideShapeFilter(next, s, true))
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(b, a) {
		return fmt.Errorf("形状以外的内容或未请求属性发生变化")
	}
	return nil
}
func phase3PrepareSlideShape(filename string, ids []string, s *phase3SlidesSpec) error {
	if err := phase3PlaceholderBWPreflight(filename); err != nil {
		return err
	}
	z, entries, e := openArtifact(filename)
	if e != nil {
		return e
	}
	defer z.Close()
	root, e := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if e != nil {
		return e
	}
	width, e := phase3ShapeInteger(phase3SlideChild(root, artifactSlideNS, "sldSz"), "cx")
	if e != nil {
		return e
	}
	found := false
	_, e = readPhase3SlideArtifactFiltered(filename, ids, func(i int, n *artifactNode) (*artifactNode, error) {
		if ids[i] != s.SlideID {
			return n, nil
		}
		found = true
		tree, e := phase3ShapeTree(n)
		if e != nil {
			return nil, e
		}
		at, behind, e := phase3ShapePositions(tree, *s.Shape)
		if e != nil {
			return nil, e
		}
		shape := tree.Children[at]
		if shape.Name.Local != "sp" {
			return nil, fmt.Errorf("目前仅支持直接形状，不支持组合或图片")
		}
		if e = phase3VisibleShape(shape); e != nil {
			return nil, e
		}
		sp := phase3SlideChild(shape, artifactSlideNS, "spPr")
		xf := phase3SlideChild(sp, artifactDrawingNS, "xfrm")
		if xf == nil {
			return nil, fmt.Errorf("需要显式几何，不能猜测母版继承值")
		}
		if s.Shape.Rotation != nil {
			r := int64(0)
			if xf.attr("rot") != "" {
				r, e = phase3ShapeInteger(xf, "rot")
				if e != nil {
					return nil, e
				}
			}
			s.Shape.Delta = *s.Shape.Rotation - float64(r)/60000
		}
		if s.Shape.HorizontalCenter {
			cx, e := phase3ShapeInteger(phase3SlideChild(xf, artifactDrawingNS, "ext"), "cx")
			if e != nil || cx <= 0 || cx > width || (width-cx)%2 != 0 {
				return nil, fmt.Errorf("形状宽度无法精确居中")
			}
			s.Shape.CenterX = (width - cx) / 2
		}
		if s.Shape.LineColor != "" {
			ln := phase3SlideChild(sp, artifactDrawingNS, "ln")
			w, e := phase3ShapeInteger(ln, "w")
			if e != nil || w <= 0 {
				return nil, fmt.Errorf("边框颜色仅支持已有显式正线宽，不能猜测继承边框")
			}
		}
		if s.Shape.BehindName != "" {
			if at < behind {
				return nil, fmt.Errorf("形状已位于参照对象后方")
			}
			s.Shape.BackwardSteps = at - behind
		}
		if s.Shape.BehindName == "" && verifyPhase3SlideShapeNode(shape, *s.Shape) == nil {
			return nil, fmt.Errorf("形状已经满足目标，未执行无效写入")
		}
		return n, nil
	})
	if e != nil {
		return e
	}
	if !found {
		return fmt.Errorf("目标页面不存在")
	}
	return nil
}
func phase3SlideShapeMutationJS(s phase3SlidesSpec, ids []string) string {
	raw, _ := json.Marshal(s)
	list, _ := json.Marshal(ids)
	return `(()=>{const r=` + string(raw) + `;const expected=` + string(list) + `;` + phase3PresenceGuardJS() + `
 const sdk=window.presentationSDK,wb=sdk?.editor?.workbench,api=sdk?.API?.slides;if(wb?.isReady()!==true||typeof wb.permissions?.canEdit!=="function"||wb.permissions.canEdit()!==true)return {ok:false,error:"未确认编辑权限"};if(JSON.stringify(api.getSlides().map(s=>s.id))!==JSON.stringify(expected))return {ok:false,error:"页面已变化"};const slide=api.getSlides().find(s=>s.id===r.slideId);const shapes=slide?.getShapes().filter(s=>s.name===r.shape.shapeName);if(shapes?.length!==1)return {ok:false,error:"形状名不唯一"};const shape=shapes[0],p=r.shape,c=wb.core,e=wb.editor;const methods=['setTextRange','selectElementByIds'];if(p.fillColor)methods.push('setShapeFillSolid');if(p.lineColor)methods.push('setShapeStrokeColor');if(p.rotation!==undefined)methods.push('rotate');if(p.horizontalCenter)methods.push('setShapesAlign');if(p.backwardSteps)methods.push('sendBackward');if(methods.some(k=>typeof c[k]!=='function'))return {ok:false,error:"缺少经过证实的形状操作接口"};api.setCurrentSlideIndex(r.slideId);c.setTextRange(e,null);c.selectElementByIds(e,[shape.id]);if(p.fillColor)c.setShapeFillSolid(e,p.fillColor,0);if(p.lineColor)c.setShapeStrokeColor(e,p.lineColor,0);if(p.rotation!==undefined)c.rotate(e,p.rotationDelta||0);if(p.horizontalCenter)c.setShapesAlign(e,4,1);for(let i=0;i<(p.backwardSteps||0);i++)c.sendBackward(e);return {ok:true,shapeId:shape.id,shapeName:shape.name,applied:p};})()`
}
