package main

import (
	"fmt"
	"math"
	"strconv"
)

// DOCX may omit header/footer distances. Read their effective values before
// changing margins instead of inventing a default or accepting any export.
const documentNativePageMarginsJS = `(async()=>{const a=window._sdk?.editor?.docsApi,e=window._sdk?.editor;if(e?.config?.mode?.role!=='editor'||a?.isViewMode!==false||e.canHandleChanges===false)return {ok:false,error:'没有已确认文稿编辑权限'};const ps=e.getParagraphs?.();if(!ps?.length||typeof e.runSelectionCallback!=='function'||typeof e.getParagraphRange!=='function')return {ok:false,error:'无法定位页边距所属节，未写入'};return await e.runSelectionCallback(e.getParagraphRange(ps[0]),()=>{const s=a.getCurPosSectionPr?.();if(typeof s?.getPgMarginHeader!=='function'||typeof s?.getPgMarginFooter!=='function')return {ok:false,error:'无法读取缺省页眉/页脚距离，未写入'};const header=s.getPgMarginHeader(),footer=s.getPgMarginFooter();if(![header,footer].every(v=>typeof v==='number'&&Number.isFinite(v)&&v>=0&&v<=1000))return {ok:false,error:'页眉/页脚距离无效，未写入'};return {ok:true,header,footer}})})()`

func documentNeedsNativeMargins(spec documentLayoutSpec, before documentLayoutSnapshot) bool {
	return spec.Action == "document-page" && len(spec.Margins) > 0 && len(before.SectionValues) == 1 && (before.SectionValues[0]["pgMar.header"] == "" || before.SectionValues[0]["pgMar.footer"] == "")
}

func documentWithNativeMargins(before documentLayoutSnapshot, values map[string]any) (documentLayoutSnapshot, error) {
	if values["ok"] != true || len(before.SectionValues) != 1 {
		return before, fmt.Errorf("无法证明缺省页眉/页脚距离，未写入")
	}
	keep := map[string]float64{}
	for _, key := range []string{"header", "footer"} {
		v, ok := values[key].(float64)
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1000 {
			return before, fmt.Errorf("原生 %s 距离无效，未写入", key)
		}
		if raw := before.SectionValues[0]["pgMar."+key]; raw != "" {
			twips, err := strconv.ParseFloat(raw, 64)
			if err != nil || math.Abs(twips-v*1440/25.4) > 1 {
				return before, fmt.Errorf("原生 %s 与导出基线不一致，未写入", key)
			}
		} else {
			keep[key] = v
		}
	}
	before.NativePageMargins = keep
	return before, nil
}

func documentPageSectionValues(before documentLayoutSnapshot) map[string]string {
	out := map[string]string{}
	if len(before.SectionValues) == 0 {
		return out
	}
	for k, v := range before.SectionValues[0] {
		out[k] = v
	}
	for _, k := range []string{"header", "footer"} {
		if v, ok := before.NativePageMargins[k]; ok && out["pgMar."+k] == "" && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1000 {
			out["pgMar."+k] = strconv.FormatFloat(math.Round(v*1440/25.4), 'f', 0, 64)
		}
	}
	return out
}
