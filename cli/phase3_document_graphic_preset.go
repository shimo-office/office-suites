package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Values and preview colors are the actual visible first row in the editor's
// shape-style gallery (p3-a871c9e4d2, ye; Shape.onShapeStyle). No preset name is
// invented: the visible label is Abc and the native option value is recorded.
type documentGraphicPresetOption struct {
	Value  int    `json:"value"`
	Fill   string `json:"fill"`
	Stroke string `json:"stroke"`
	Font   string `json:"font"`
}

func documentGraphicPresetOptionFor(value int) (documentGraphicPresetOption, error) {
	colors := []string{"#676B6F", "#5C8BD9", "#6FBDC6", "#A7B852", "#F5BE1E", "#DC6E6E"}
	if value < 1 || value > len(colors) {
		return documentGraphicPresetOption{}, fmt.Errorf("preset仅支持已确认的原生可见选项1–6")
	}
	return documentGraphicPresetOption{Value: value, Fill: "#FFFFFF", Stroke: colors[value-1], Font: "#2C3033"}, nil
}
func documentGraphicPresetFont(root *artifactNode) (*artifactNode, error) {
	shapes := documentGraphicNodes(root, documentShapeNS, "wsp")
	if len(shapes) != 1 {
		return nil, fmt.Errorf("预设样式仅支持原生形状")
	}
	styles := shapes[0].children("style")
	if len(styles) != 1 || styles[0].Name.Space != documentShapeNS {
		return nil, fmt.Errorf("形状预设style不唯一或缺失")
	}
	refs := []*artifactNode{}
	for _, child := range styles[0].Children {
		if child.Name.Local == "fontRef" {
			refs = append(refs, child)
		}
	}
	if len(refs) != 1 || refs[0].Name.Space != documentDrawingMainNS || len(refs[0].Children) != 1 {
		return nil, fmt.Errorf("形状预设fontRef无法严格验证")
	}
	color := refs[0].Children[0]
	if color.Name.Space != documentDrawingMainNS || (color.Name.Local != "srgbClr" && color.Name.Local != "schemeClr") || len(color.Attr) != 1 || color.Attr[0].Name.Space != "" || color.Attr[0].Name.Local != "val" || !documentGraphicOpaqueAlpha(color) {
		return nil, fmt.Errorf("形状预设字体颜色结构不支持")
	}
	return refs[0], nil
}
func normalizeDocumentGraphicPreset(drawing *artifactNode) error {
	ref, err := documentGraphicPresetFont(drawing)
	if err != nil {
		return err
	}
	documentGraphicMask(ref, "idx")
	ref.Children = nil
	return nil
}
func verifyDocumentGraphicPreset(drawing *artifactNode, value int) error {
	option, err := documentGraphicPresetOptionFor(value)
	if err != nil {
		return err
	}
	props, err := documentGraphicShapeProperties(drawing)
	if err != nil {
		return err
	}
	if !documentGraphicOpaqueColor(props, option.Fill) || !documentGraphicOpaqueColor(documentGraphicChild(props, "ln"), option.Stroke) {
		return fmt.Errorf("真实预设填充/线条颜色未持久化")
	}
	font, err := documentGraphicPresetFont(drawing)
	if err != nil {
		return err
	}
	if font.attr("idx") != "minor" {
		return fmt.Errorf("真实预设fontRef类型未持久化")
	}
	c := font.Children[0]
	if c.Name.Local != "srgbClr" || strings.ToUpper(c.attr("val")) != strings.TrimPrefix(option.Font, "#") || len(c.Attr) != 1 || !documentGraphicOpaqueAlpha(c) {
		return fmt.Errorf("真实预设fontRef颜色未持久化")
	}
	return nil
}

func documentGraphicPresetRuntimeJS(value int) string {
	option, _ := documentGraphicPresetOptionFor(value)
	raw, _ := json.Marshal(option)
	return `const requestedPreset=` + string(raw) + `;
 const inspectPreset=()=>{
  const roots=[...document.querySelectorAll('[id="shape-style"]')].filter(e=>e.getClientRects().length);
  if(roots.length!==1)return {ok:false,error:'真实形状预设工具栏不存在或不唯一'};
  const root=roots[0],items=[];let gallery=null;
  const fiberFor=e=>{const k=Object.keys(e).find(k=>k.startsWith('__reactFiber'));return k?e[k]:null};
  for(const li of root.querySelectorAll('li')){
   if(!li.getClientRects().length||li.textContent!=='Abc')continue;
   let item=null;for(let f=fiberFor(li),depth=0;f&&depth<12;f=f.return,depth++){
    const p=f.memoizedProps;if(p?.name==='shape-style'&&Array.isArray(p.items)){if(p.disabled===true)return {ok:false,error:'原生预设工具栏已禁用'};gallery=p.items;break}
    if(p&&Number.isInteger(p.value)&&typeof p.fill==='string'&&typeof p.font==='string'&&typeof p.onClick==='function')item=p;
   }
   if(item)items.push({li,item});
  }
  const matches=items.filter(v=>v.item.value===requestedPreset.value);
  if(matches.length!==1||!gallery||gallery.filter(v=>v.value===requestedPreset.value).length!==1)return {ok:false,error:'真实预设可见选项不存在或不唯一'};
  const chosen=matches[0],actual=gallery.find(v=>v.value===requestedPreset.value);
  if(['fill','stroke','font'].some(k=>chosen.item[k]!==requestedPreset[k]||actual[k]!==requestedPreset[k]))return {ok:false,error:'真实预设选项颜色与已确认定义不符'};
  const box=chosen.li.querySelector('div'),style=box?getComputedStyle(box):null;
  const css=hex=>'rgb('+[1,3,5].map(i=>parseInt(hex.slice(i,i+2),16)).join(', ')+')';
  if(!style||style.backgroundColor!==css(requestedPreset.fill)||style.borderTopColor!==css(requestedPreset.stroke)||style.color!==css(requestedPreset.font))return {ok:false,error:'真实预设可见预览与定义不一致'};
  return {ok:true,element:chosen.li,option:{...requestedPreset,visibleLabel:'Abc'},available:items.map(v=>({value:v.item.value,fill:v.item.fill,stroke:v.item.stroke,font:v.item.font,visibleLabel:'Abc'}))};
 };
 const waitPreset=async()=>{let result;for(let i=0;i<20;i++){result=inspectPreset();if(result.ok)return result;await new Promise(r=>setTimeout(r,100))}return result};
 `
}

// Native solid colors may serialize the explicit opaque alpha default. No
// other transforms, duplicate alphas or unknown attributes are equivalent.
func documentGraphicOpaqueAlpha(color *artifactNode) bool {
	if strings.TrimSpace(color.Text) != "" || len(color.Children) > 1 {
		return false
	}
	if len(color.Children) == 0 {
		return true
	}
	a := color.Children[0]
	return a.Name.Space == documentDrawingMainNS && a.Name.Local == "alpha" && len(a.Attr) == 1 && a.Attr[0].Name.Space == "" && a.Attr[0].Name.Local == "val" && a.Attr[0].Value == "100000" && len(a.Children) == 0 && strings.TrimSpace(a.Text) == ""
}
