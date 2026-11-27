package main

import (
	"archive/zip"
	"fmt"
	"math"
	"path"
)

func phase3PrepareOutline(p map[string]any) (map[string]any, error) {
	color, hasColor := p["outlineColor"]
	width, hasWidth := p["outlineWidth"]
	if !hasColor && !hasWidth {
		return nil, nil
	}
	if !hasColor {
		return nil, fmt.Errorf("outlineWidth 必须与 outlineColor 一起提供")
	}
	checked, e := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": p["text"], "attrs": map[string]any{"color": color}})
	if e != nil {
		return nil, e
	}
	points := float64(1)
	if hasWidth {
		n, ok := width.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < .25 || n > 20 || n*12700 != math.Trunc(n*12700) {
			return nil, fmt.Errorf("outlineWidth 为0.25–20磅")
		}
		points = n
	}
	return map[string]any{"outlineColor": checked.Attrs["color"], "outlineWidth": points}, nil
}
func phase3VerifyOutline(rp *artifactNode, attrs map[string]any) error {
	color, ok := attrs["outlineColor"].(string)
	if !ok {
		return nil
	}
	line := phase3SlideChild(rp, artifactDrawingNS, "ln")
	if line == nil {
		return fmt.Errorf("文本run缺少真实文字轮廓a:ln")
	}
	width, e := phase3ShapeInteger(line, "w")
	if e != nil || width != int64(attrs["outlineWidth"].(float64)*12700) {
		return fmt.Errorf("文字轮廓宽度不符")
	}
	if e = phase3ShapeColor(line, color); e != nil {
		return e
	}
	for _, n := range line.Children {
		switch n.Name.Local {
		case "prstDash":
			if n.Name.Space != artifactDrawingNS || n.attr("val") != "solid" {
				return fmt.Errorf("文字轮廓并非实线")
			}
		case "solidFill":
		case "headEnd", "tailEnd":
			if n.attr("type") != "none" {
				return fmt.Errorf("文字轮廓意外出现箭头")
			}
		default:
			return fmt.Errorf("文字轮廓含未请求线型属性")
		}
	}
	return nil
}

// The source-backed setter creates a new complete stroke. Until preserving an
// existing stroke is verified live, refuse a target's explicit/inherited text
// outline rather than silently replacing its dash/cap/join/arrow properties.
// Unrelated shapes and slides do not contribute to the target's text style.
func phase3CheckNewOutlineArtifact(filename string, ids []string, s phase3SlidesSpec) error {
	z, entries, e := openArtifact(filename)
	if e != nil {
		return e
	}
	defer z.Close()
	root, e := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if e != nil {
		return e
	}
	if e = phase3CheckOutlineNode(phase3SlideChild(root, artifactSlideNS, "defaultTextStyle")); e != nil {
		return e
	}
	list := phase3SlideChild(root, artifactSlideNS, "sldIdLst")
	if list == nil || len(list.children("sldId")) != len(ids) || s.Format == nil {
		return fmt.Errorf("文字轮廓目标与页面列表不符")
	}
	rels, e := artifactRelations(entries, "ppt/_rels/presentation.xml.rels", "ppt")
	if e != nil {
		return e
	}
	part := ""
	for i, id := range ids {
		if id != s.SlideID {
			continue
		}
		rel, ok := rels[relationID(list.children("sldId")[i])]
		if part != "" || !ok || rel.Kind != artifactOfficeRelNS+"/slide" {
			return fmt.Errorf("文字轮廓目标页关系无效或重复")
		}
		part = rel.Target
	}
	if part == "" {
		return fmt.Errorf("文字轮廓目标页不存在")
	}
	slide, e := artifactPart(entries, part, artifactSlideNS, "sld")
	if e != nil {
		return e
	}
	if _, e = phase3MapSlideShape(slide, func(n *artifactNode) bool { return phase3SlideText(n) == s.Format.Text }, func(n *artifactNode) (*artifactNode, error) {
		return n, phase3CheckOutlineNode(n)
	}); e != nil {
		return e
	}
	// Check only actual ancestors, never the sibling layouts reachable from a
	// master. Ancestor styles remain conservative until run inheritance is resolved.
	for _, ancestor := range []struct{ kind, namespace, tag string }{
		{"slideLayout", artifactSlideNS, "sldLayout"},
		{"slideMaster", artifactSlideNS, "sldMaster"},
		{"theme", artifactDrawingNS, "theme"},
	} {
		part, e = phase3OutlineAncestor(entries, part, ancestor.kind)
		if e != nil || part == "" {
			return e
		}
		n, e := artifactPart(entries, part, ancestor.namespace, ancestor.tag)
		if e != nil {
			return e
		}
		if e = phase3CheckOutlineNode(n); e != nil {
			return e
		}
	}
	return nil
}

func phase3CheckOutlineNode(root *artifactNode) error {
	if root == nil {
		return nil
	}
	found := false
	phase3WalkSlideNode(root, func(n *artifactNode) {
		if n.Name.Space == artifactDrawingNS && (n.Name.Local == "rPr" || n.Name.Local == "defRPr" || n.Name.Local == "endParaRPr") && phase3SlideChild(n, artifactDrawingNS, "ln") != nil {
			found = true
		}
	})
	if found {
		return fmt.Errorf("目标文字或其版式/母版/默认样式已有文字轮廓；当前动作拒绝覆盖已有轮廓样式")
	}
	return nil
}

func phase3OutlineAncestor(entries map[string]*zip.File, part, kind string) (string, error) {
	name := path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
	if entries[name] == nil {
		return "", nil
	}
	rels, e := artifactRelations(entries, name, path.Dir(part))
	if e != nil {
		return "", e
	}
	root, e := artifactPart(entries, name, artifactRelNS, "Relationships")
	if e != nil {
		return "", e
	}
	target := ""
	for _, r := range root.children("Relationship") {
		if r.attr("Type") != artifactOfficeRelNS+"/"+kind {
			continue
		}
		rel, ok := rels[r.attr("Id")]
		if target != "" || !ok {
			return "", fmt.Errorf("文字轮廓继承关系%s无效或重复", kind)
		}
		target = rel.Target
	}
	return target, nil
}
func phase3SlideOutlinePreflightJS() string {
	return `let outline,outlineHistory;if(p.outlineColor!==undefined){const presentation=wb.editor?.presentation;outlineHistory=wb.editor?._$collaborationManager$_?._$changesCollector$_?._$changesStack$_;if(!presentation||outlineHistory?.presentation!==presentation||typeof outlineHistory?._$createNewAction$_!=='function'||typeof presentation._$getCalcedTextPr$_!=='function'||typeof presentation._$applyTextPrToParagraph$_!=='function'||typeof wb.core.runAtomicChange!=='function')return {ok:false,error:'缺少当前编辑器绑定的原生文字轮廓接口，未写入'};const OutlinePr=presentation._$getCalcedTextPr$_()?.constructor;if(typeof OutlinePr!=='function')return {ok:false,error:'缺少当前编辑器文字样式构造器，未写入'};const parsed=new OutlinePr;if(typeof parsed._$fromJSON$_!=='function')return {ok:false,error:'缺少原生文字样式解析器'};parsed._$fromJSON$_({ln:{w:Math.round(p.outlineWidth*12700),prstDash:'solid',fill:{fill:{type:3,data:{color:{color:{type:1,val:p.outlineColor.slice(1)}}}}}}});outline=parsed._$textStroke$_;if(!outline)return {ok:false,error:'原生文字轮廓未解析，未写入'}};`
}
func phase3SlideOutlineApplyJS() string {
	return `if(outline){wb.core.runAtomicChange(wb.editor,()=>{outlineHistory._$createNewAction$_(214);wb.editor.presentation._$applyTextPrToParagraph$_({_$textStroke$_:outline},false)})};`
}
