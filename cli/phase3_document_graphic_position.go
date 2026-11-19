package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type documentGraphicPositionParagraph struct {
	Text     string                    `json:"text"`
	Drawings []documentGraphicGeometry `json:"drawings"`
}
type documentGraphicGeometry struct {
	Kind            string  `json:"kind"`
	Preset          string  `json:"preset"`
	WidthMM         float64 `json:"widthMM"`
	HeightMM        float64 `json:"heightMM"`
	RotationRadians float64 `json:"rotationRadians"`
}

func prepareDocumentGraphicTarget(p map[string]any, s *documentGraphicSpec) error {
	_, hasParagraph := p["paragraph"]
	_, hasText := p["expectedParagraph"]
	_, hasDrawing := p["drawing"]
	if hasParagraph || hasText || hasDrawing {
		if _, ok := p["name"]; ok {
			return fmt.Errorf("位置目标不能同时指定name/description")
		}
		if _, ok := p["description"]; ok {
			return fmt.Errorf("位置目标不能同时指定name/description")
		}
		text, ok := p["expectedParagraph"].(string)
		if !ok || strings.ContainsAny(text, "\r\n\x00") || len(text) > 400000 {
			return fmt.Errorf("expectedParagraph必须明确给出完整段落文字（可为空），不含换行")
		}
		for key, dst := range map[string]*int{"paragraph": &s.Paragraph, "drawing": &s.Drawing} {
			n, ok := p[key].(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 1 || n > 100000 || n != math.Trunc(n) {
				return fmt.Errorf("%s必须为从1开始的整数", key)
			}
			*dst = int(n)
		}
		s.ExpectedParagraph = text
		return nil
	}
	for key, dst := range map[string]*string{"name": &s.Name, "description": &s.Description} {
		v, ok := p[key].(string)
		if !ok || strings.TrimSpace(v) == "" || len(v) > 1000 || strings.ContainsAny(v, "\r\n\x00") {
			return fmt.Errorf("%s必须精确指定非空图形名称/描述", key)
		}
		*dst = v
	}
	return nil
}

// Positional targets are restricted to simple body paragraphs. Their complete
// text and every drawing's type/geometry are matched before selecting anything.
func documentGraphicBodyParagraphs(root *artifactNode) ([]*artifactNode, error) {
	body := root.child("body")
	if body == nil {
		return nil, fmt.Errorf("文稿body缺失")
	}
	out := []*artifactNode{}
	for _, n := range body.Children {
		if n.Name.Space != artifactWordNS {
			return nil, fmt.Errorf("位置图形目标不支持未知正文结构")
		}
		switch n.Name.Local {
		case "p":
			out = append(out, n)
		case "sectPr":
		default:
			return nil, fmt.Errorf("位置图形目标仅支持纯段落正文，不支持表格或嵌套容器")
		}
	}
	return out, nil
}
func documentGraphicParagraphText(p *artifactNode) string {
	var b strings.Builder
	var visit func(*artifactNode)
	visit = func(n *artifactNode) {
		if n.Name.Space == artifactWordNS {
			switch n.Name.Local {
			case "drawing":
				return
			case "t":
				b.WriteString(n.Text)
				return
			case "tab":
				b.WriteByte('\t')
				return
			case "br", "cr":
				b.WriteByte('\n')
				return
			}
		}
		for _, c := range n.Children {
			visit(c)
		}
	}
	visit(p)
	return b.String()
}
func documentGraphicPositionTarget(root *artifactNode, s documentGraphicSpec) (*artifactNode, error) {
	paragraphs, err := documentGraphicBodyParagraphs(root)
	if err != nil {
		return nil, err
	}
	if s.Paragraph < 1 || s.Paragraph > len(paragraphs) {
		return nil, fmt.Errorf("目标段落不存在")
	}
	p := paragraphs[s.Paragraph-1]
	if documentGraphicParagraphText(p) != s.ExpectedParagraph {
		return nil, fmt.Errorf("目标段落完整文字不匹配")
	}
	drawings := documentGraphicNodes(p, artifactWordNS, "drawing")
	if s.Drawing < 1 || s.Drawing > len(drawings) {
		return nil, fmt.Errorf("目标段落图形序号不存在")
	}
	return drawings[s.Drawing-1], nil
}
func documentGraphicSingleTransform(drawing *artifactNode) (*artifactNode, string, error) {
	pictures := documentGraphicNodes(drawing, documentPictureNS, "pic")
	shapes := documentGraphicNodes(drawing, documentShapeNS, "wsp")
	graphics := append(pictures, shapes...)
	if len(graphics) != 1 {
		return nil, "", fmt.Errorf("目标不是单一图片或原生形状")
	}
	transforms := documentGraphicNodes(graphics[0], documentDrawingMainNS, "xfrm")
	if len(transforms) != 1 {
		return nil, "", fmt.Errorf("目标变换不唯一")
	}
	kind := "image"
	if len(shapes) == 1 {
		kind = "shape"
	}
	return transforms[0], kind, nil
}
func documentGraphicPositionGeometry(drawing *artifactNode) (documentGraphicGeometry, error) {
	var g documentGraphicGeometry
	xfrm, kind, err := documentGraphicSingleTransform(drawing)
	if err != nil {
		return g, err
	}
	g.Kind = kind
	ext := documentGraphicChild(xfrm, "ext")
	if ext == nil {
		return g, fmt.Errorf("图形尺寸缺失")
	}
	for k, dst := range map[string]*float64{"cx": &g.WidthMM, "cy": &g.HeightMM} {
		n, err := strconv.ParseInt(ext.attr(k), 10, 64)
		if err != nil || n <= 0 {
			return g, fmt.Errorf("图形尺寸无效")
		}
		*dst = float64(n) / 36000
	}
	if r := xfrm.attr("rot"); r != "" {
		n, err := strconv.ParseInt(r, 10, 64)
		if err != nil {
			return g, err
		}
		g.RotationRadians = float64(n) / 60000 * math.Pi / 180
	}
	presets := documentGraphicNodes(drawing, documentDrawingMainNS, "prstGeom")
	if len(presets) != 1 || presets[0].attr("prst") == "" {
		return g, fmt.Errorf("位置图形目标必须有唯一标准几何类型")
	}
	g.Preset = presets[0].attr("prst")
	return g, nil
}
func documentGraphicPositionBaseline(root *artifactNode) ([]documentGraphicPositionParagraph, error) {
	paragraphs, err := documentGraphicBodyParagraphs(root)
	if err != nil {
		return nil, err
	}
	out := make([]documentGraphicPositionParagraph, 0, len(paragraphs))
	for _, p := range paragraphs {
		item := documentGraphicPositionParagraph{Text: documentGraphicParagraphText(p), Drawings: []documentGraphicGeometry{}}
		for _, d := range documentGraphicNodes(p, artifactWordNS, "drawing") {
			g, err := documentGraphicPositionGeometry(d)
			if err != nil {
				return nil, err
			}
			item.Drawings = append(item.Drawings, g)
		}
		out = append(out, item)
	}
	return out, nil
}

// Intended to run inside an async native script, before any selection/mutation.
const documentGraphicPositionRuntimeJS = `
 const positionNodes=[],positionSeen=new Set;
 if(!Array.isArray(baseline.paragraphs)||baseline.paragraphs.length!==model.children.length||typeof editor.getParagraphRange!=='function')return {ok:false,error:'位置图形正文结构与导出基线不匹配'};
 for(let pi=0;pi<baseline.paragraphs.length;pi++){
  const p=model.children[pi],want=baseline.paragraphs[pi],ds=[];
  if(typeof p.getAllDrawingObjs!=='function')return {ok:false,error:'位置图形段落接口缺失'};
  const text=(await editor.getParagraphRange(p).getText()).replace(/\r?\n$/,'');
  if(text!==want.text)return {ok:false,error:'位置图形完整正文与导出基线不一致'};
  p.getAllDrawingObjs(ds);if(ds.length!==want.drawings.length)return {ok:false,error:'位置图形数量与导出基线不一致'};
  for(let di=0;di<ds.length;di++){
   if(positionSeen.has(ds[di].graphicsObj))return {ok:false,error:'位置图形对象重复'};positionSeen.add(ds[di].graphicsObj);
   const native=ds[di].graphicsObj,actual=native?.spPr?.xfrm,geometry=want.drawings[di],target=pi===spec.paragraph-1&&di===spec.drawing-1;
   let w=geometry.widthMM,h=geometry.heightMM,r=geometry.rotationRadians;
   if(target&&positionAfter){if(spec.widthCm!==undefined){w=spec.widthCm*10;h=spec.heightCm*10}if(spec.rotationDegrees!==undefined)r=spec.rotationDegrees*Math.PI/180}
   if(!native||native.group||geometry.kind==='shape'&&native.isShape?.()!==true||geometry.kind==='image'&&native.isImage?.()!==true||native.spPr?.geometry?.preset!==geometry.preset||!actual||![actual.extX,actual.extY,actual.rot??0].every(Number.isFinite)||Math.abs(actual.extX-w)>0.01||Math.abs(actual.extY-h)>0.01||Math.abs((actual.rot??0)-r)>0.00001)return {ok:false,error:'位置图形类型或几何与导出基线不一致'};
  }
  positionNodes.push(ds);
 }
 if(!positionNodes[spec.paragraph-1]?.[spec.drawing-1]||baseline.paragraphs[spec.paragraph-1].text!==spec.expectedParagraph)return {ok:false,error:'显式位置图形目标不存在或文字不匹配'};
 matches=[positionNodes[spec.paragraph-1][spec.drawing-1]];
`
