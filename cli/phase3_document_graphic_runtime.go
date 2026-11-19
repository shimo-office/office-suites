package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

type documentGraphicNativeBaseline struct {
	Paragraphs      []documentGraphicPositionParagraph `json:"paragraphs,omitempty"`
	Kind            string                             `json:"kind"`
	WidthMM         float64                            `json:"widthMM"`
	HeightMM        float64                            `json:"heightMM"`
	RotationRadians float64                            `json:"rotationRadians"`
}

func documentGraphicBaseline(root *artifactNode, s documentGraphicSpec) (documentGraphicNativeBaseline, error) {
	var out documentGraphicNativeBaseline
	drawing, xfrm, err := documentGraphicTarget(root, s)
	if err != nil {
		return out, err
	}
	if err := documentGraphicLayoutBaseline(drawing, s); err != nil {
		return out, err
	}
	out.Kind = "image"
	if len(documentGraphicNodes(drawing, documentShapeNS, "wsp")) == 1 {
		out.Kind = "shape"
	}
	ext := documentGraphicChild(xfrm, "ext")
	if ext == nil {
		return out, fmt.Errorf("目标缺少真实尺寸")
	}
	width, e1 := strconv.ParseInt(ext.attr("cx"), 10, 64)
	height, e2 := strconv.ParseInt(ext.attr("cy"), 10, 64)
	if e1 != nil || e2 != nil || width <= 0 || height <= 0 {
		return out, fmt.Errorf("目标尺寸无效")
	}
	out.WidthMM = float64(width) / 36000
	out.HeightMM = float64(height) / 36000
	if rotation := xfrm.attr("rot"); rotation != "" {
		degrees, err := strconv.ParseInt(rotation, 10, 64)
		if err != nil {
			return out, err
		}
		out.RotationRadians = float64(degrees) / 60000 * math.Pi / 180
	}
	if s.Paragraph > 0 {
		out.Paragraphs, err = documentGraphicPositionBaseline(root)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// Selection uses actual SDK object identity and the layout page containing it.
// No object ID or page index is guessed, and drawing mode 3 is module70743.wJ.
func documentGraphicRuntimeJS(s documentGraphicSpec, baseline documentGraphicNativeBaseline, mutate bool, expectedID string) string {
	return documentGraphicRuntimeScript(s, baseline, mutate, false, expectedID)
}

func documentGraphicRuntimeScript(s documentGraphicSpec, baseline documentGraphicNativeBaseline, mutate, selectOnly bool, expectedID string) string {
	request, _ := json.Marshal(s)
	original, _ := json.Marshal(baseline)
	id, _ := json.Marshal(expectedID)
	script := `(async()=>{` + phase3PresenceGuardJS() + `const spec=` + string(request) + `,baseline=` + string(original) + `,expectedID=` + string(id) + `;
 const editor=window._sdk?.editor,api=editor?.docsApi,model=api?.controller?.docModel,drawings=model?.drawings;
 if(api?.isViewMode!==false||editor.config?.mode?.role!=='editor'||editor.canHandleChanges===false)return {ok:false,error:'图形编辑权限未确认'};
 if(!Array.isArray(model?.children)||!Array.isArray(drawings?.drawingPages)||typeof drawings.getSelectedObjs!=='function'||typeof drawings.resetSelection!=='function'||typeof drawings.selectObj!=='function'||typeof model.setCursorPosType!=='function'||typeof api.applyImgProperties!=='function')return {ok:false,error:'缺少已确认的原生图形选择接口'};
 const all=[];for(const node of model.children){if(typeof node.getAllDrawingObjs!=='function')return {ok:false,error:'不支持的文稿对象结构'};node.getAllDrawingObjs(all)}
 let matches=[];const positionAfter=` + strconv.FormatBool(!mutate && expectedID != "") + `;
 if(spec.paragraph){` + documentGraphicPositionRuntimeJS + `}else{matches=all.filter(d=>{const n=d.graphicsObj?.getNvPr?.();return n?.name===spec.name&&n?.descr===spec.description});}
 if(matches.length!==1)return {ok:false,error:'真实图形名称/描述不存在或不唯一'};
 const drawing=matches[0],shape=drawing.graphicsObj;if(shape.group||!shape.Id||expectedID&&String(shape.Id)!==expectedID)return {ok:false,error:'图形分组或对象ID不匹配'};
 if(baseline.kind==='shape'&&shape.isShape?.()!==true||baseline.kind==='image'&&shape.isImage?.()!==true)return {ok:false,error:'图形真实类型与导出基线不符'};
 const xfrm=shape.spPr?.xfrm;if(!xfrm||![xfrm.extX,xfrm.extY,xfrm.rot??0].every(Number.isFinite))return {ok:false,error:'真实图形几何不可读'};
 const pages=[];drawings.drawingPages.forEach((page,index)=>{if(['inlineObjects','wrappingObjects','frontTextObjects','behindTextObjects'].some(key=>Array.isArray(page?.[key])&&page[key].includes(shape)))pages.push(index)});if(pages.length!==1)return {ok:false,error:'图形实际布局页不唯一或未就绪'};
 const snapshot=()=>({objectId:String(shape.Id),kind:baseline.kind,pageIndex:pages[0],name:shape.getNvPr?.()?.name??null,description:shape.getNvPr?.()?.descr??null,paragraph:spec.paragraph??null,drawing:spec.drawing??null,widthMM:shape.spPr.xfrm.extX,heightMM:shape.spPr.xfrm.extY,rotationRadians:shape.spPr.xfrm.rot??0});
 const props={};if(spec.widthCm!==undefined){props.w=spec.widthCm*10;props.h=spec.heightCm*10}if(spec.rotationDegrees!==undefined)props.rot=spec.rotationDegrees*Math.PI/180;
 const rgb=hex=>({type:1,r:parseInt(hex.slice(1,3),16),g:parseInt(hex.slice(3,5),16),b:parseInt(hex.slice(5,7),16),a:255});
 if(spec.fill)props.fill={type:3,fill:{color:rgb(spec.fill)}};if(spec.line)props.stroke={type:1,width:spec.line.widthPt*25.4/72,color:rgb(spec.line.color)};
 if((spec.fill||spec.line)&&baseline.kind!=='shape')return {ok:false,error:'填充/线条要求真实形状'};
 `
	script += documentGraphicLayoutPreflightJS(s)
	if s.Preset != 0 {
		script += `if(baseline.kind!=="shape")return {ok:false,error:"预设要求原生形状"};` + documentGraphicPresetRuntimeJS(s.Preset)
	}
	selection := `drawings.resetSelection();model.setCursorPosType(3);drawings.selectObj(shape,pages[0]);const chosen=drawings.getSelectedObjs();if(chosen.length!==1||chosen[0]!==shape||String(chosen[0].Id)!==String(shape.Id))return {ok:false,error:'原生图形选中回读不符'};`
	if !mutate {
		if selectOnly {
			script += selection + `if(typeof model.updateUI==='function')model.updateUI();if(typeof model.updateSelection==='function')model.updateSelection();`
		}
		if selectOnly && s.Preset != 0 {
			return script + `const preset=await waitPreset();if(!preset.ok)return preset;return {ok:true,...snapshot(),presetOption:preset.option,availablePresets:preset.available};})()`
		}
		return script + `return {ok:true,...snapshot()};})()`
	}
	if s.Preset != 0 {
		return script + `if(Math.abs(xfrm.extX-baseline.widthMM)>0.01||Math.abs(xfrm.extY-baseline.heightMM)>0.01||Math.abs((xfrm.rot??0)-baseline.rotationRadians)>0.00001)return {ok:false,error:'预设提交前几何变化'};` + selection + `if(typeof model.updateUI==='function')model.updateUI();if(typeof model.updateSelection==='function')model.updateSelection();const preset=await waitPreset();if(!preset.ok)return preset;{` + phase3PresenceGuardJS() + `if(api.isViewMode!==false||editor.config?.mode?.role!=='editor'||editor.canHandleChanges===false||drawings.getSelectedObjs().length!==1||drawings.getSelectedObjs()[0]!==shape)return {ok:false,error:'预设提交前身份/权限/选区变化'};preset.element.click();}return {ok:true,submitted:true,...snapshot(),presetOption:preset.option,availablePresets:preset.available};})()`
	}
	return script + `
 if(Math.abs(xfrm.extX-baseline.widthMM)>0.01||Math.abs(xfrm.extY-baseline.heightMM)>0.01||Math.abs((xfrm.rot??0)-baseline.rotationRadians)>0.00001)return {ok:false,error:'图形几何已改变，未写入'};
` + selection + `
 {` + phase3PresenceGuardJS() + `if(api.isViewMode!==false||editor.config?.mode?.role!=='editor'||editor.canHandleChanges===false)return {ok:false,error:'提交前图形权限变化'};if(Object.keys(props).length)api.applyImgProperties(props);` + documentGraphicLayoutMutationJS(s) + `}
 return {ok:true,submitted:true,...snapshot()};})()`
}
