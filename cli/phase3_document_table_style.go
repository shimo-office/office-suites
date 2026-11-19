package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

type documentTableBorder struct {
	Color string  `json:"color"`
	Pt    float64 `json:"pt"`
}
type documentTableStyleSpec struct {
	documentTableSpec
	Fill     string               `json:"fill,omitempty"`
	Color    string               `json:"color,omitempty"`
	MarginCm *float64             `json:"marginCm,omitempty"`
	Align    string               `json:"align,omitempty"`
	Border   *documentTableBorder `json:"border,omitempty"`
}

func prepareDocumentTableStyle(p map[string]any) (documentTableStyleSpec, error) {
	s := documentTableStyleSpec{}
	for k := range p {
		if k != "table" && k != "row" && k != "column" && k != "expectedCell" && k != "fill" && k != "color" && k != "marginCm" && k != "align" && k != "border" {
			return s, fmt.Errorf("表格样式未知参数%s", k)
		}
	}
	base, err := prepareDocumentTable(map[string]any{"operation": "insert-row-after", "table": p["table"], "row": p["row"], "column": p["column"], "expectedCell": p["expectedCell"]})
	if err != nil {
		return s, err
	}
	base.Operation = "format"
	s.documentTableSpec = base
	for key, dst := range map[string]*string{"fill": &s.Fill, "color": &s.Color} {
		if v, ok := p[key]; ok {
			str, ok := v.(string)
			if !ok || !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(str) {
				return s, fmt.Errorf("%s需要#RRGGBB", key)
			}
			*dst = strings.ToUpper(str)
		}
	}
	if v, ok := p["marginCm"]; ok {
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || n > 2 {
			return s, fmt.Errorf("marginCm需要大于0且不超过2厘米")
		}
		s.MarginCm = &n
	}
	if v, ok := p["align"]; ok {
		str, ok := v.(string)
		if !ok || (str != "left" && str != "center" && str != "right") {
			return s, fmt.Errorf("align需要left/center/right")
		}
		s.Align = str
	}
	if v, ok := p["border"]; ok {
		b, ok := v.(map[string]any)
		if !ok || len(b) != 2 {
			return s, fmt.Errorf("border需要color/pt")
		}
		c, cok := b["color"].(string)
		pt, pok := b["pt"].(float64)
		if !cok || !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(c) || !pok || math.IsNaN(pt) || math.IsInf(pt, 0) || pt < .25 || pt > 12 || math.Trunc(pt*8) != pt*8 {
			return s, fmt.Errorf("border颜色或0.25–12磅宽度无效")
		}
		s.Border = &documentTableBorder{Color: strings.ToUpper(c), Pt: pt}
	}
	if s.Color != "" && s.ExpectedCell == "" {
		return s, fmt.Errorf("空单元格没有可验证字符，拒绝请求文字颜色；可单独设置fill")
	}
	if s.Align != "" && s.MarginCm == nil {
		return s, fmt.Errorf("表格对齐需要同时明确marginCm，避免编辑器重置未指定的单元格内边距")
	}
	if s.Fill == "" && s.Color == "" && s.MarginCm == nil && s.Align == "" && s.Border == nil {
		return s, fmt.Errorf("表格样式请求不能为空")
	}
	return s, nil
}

func documentTableStyleSetupJS(s documentTableStyleSpec) string {
	raw, _ := json.Marshal(s)
	preflight := `const style=` + string(raw) + `;const required=['selectCell','getSelectedTableCells','getCurTable'];if(style.fill)required.push('setCellsBackground');if(style.color)required.push('setTextColor');if(style.marginCm||style.align||style.border)required.push('selectTable','getSelectedTableCells');if(style.marginCm||style.align)required.push('setTblProps');if(style.border)required.push('setSelectedCellBorders');if(required.some(k=>typeof a[k]!=='function'))return {ok:false,error:'所需表格样式方法不可用'};let borderColor;if(style.border){let create;for(const r of p.children||[]){const c=r.getCompiledPr?.()?.Color;if(typeof c?.constructor?.create==='function'){create=c.constructor.create;break}}if(!create)return {ok:false,error:'真实DocColor构造器不可用，不猜颜色对象'};borderColor=create(style.border.color);if(borderColor?.toHex?.().toUpperCase()!==style.border.color.slice(1))return {ok:false,error:'边框颜色构造读回不一致'}};`
	return preflight
}

const documentTableStyleSelectCellJS = `a.selectCell();const chosen=a.getSelectedTableCells()?.data;if(a.getCurTable()!==table||!Array.isArray(chosen)||chosen.length!==1||chosen[0].Row!==request.row-1||chosen[0].Cell!==request.column-1)return {ok:false,error:'目标单元格选区不匹配，未写入'};`
const documentTableStyleSelectAllJS = `a.selectTable();const selectedCells=a.getSelectedTableCells()?.data;const rows=table.children.length,cols=table.children[0].children.length;if(a.getCurTable()!==table||!Array.isArray(selectedCells)||selectedCells.length!==rows*cols||new Set(selectedCells.map(c=>c.Row+':'+c.Cell)).size!==rows*cols||selectedCells.some(c=>c.Row<0||c.Row>=rows||c.Cell<0||c.Cell>=cols))return {ok:false,error:'目标表格选区不完整，停止后续操作'};`

// Selection-only preflight happens before the runner marks a content mutation.
// The real write evaluates a fresh native range, so selectTable cannot redirect
// the following cell edit: native selectCell keeps an active whole-table selection.
func documentTableStylePreflightJS(s documentTableStyleSpec, before documentTablesSnapshot) string {
	checks := documentTableStyleSelectCellJS + `if(style.marginCm||style.align||style.border){` + documentTableStyleSelectAllJS + `};return {ok:true,selectionVerified:true,mutationAttempted:false};`
	return documentTableOperationJS(s.documentTableSpec, before, "selectCell", documentTableStyleSetupJS(s), checks)
}
func documentTableStyleJS(s documentTableStyleSpec, before documentTablesSnapshot) string {
	mutation := documentTableStyleSelectCellJS + `if(style.fill)a.setCellsBackground(style.fill);if(style.color)a.setTextColor({r:parseInt(style.color.slice(1,3),16),g:parseInt(style.color.slice(3,5),16),b:parseInt(style.color.slice(5,7),16)});if(style.marginCm||style.align||style.border){a.selectTable();const selectedCells=a.getSelectedTableCells()?.data;const rows=table.children.length,cols=table.children[0].children.length;if(a.getCurTable()!==table||!Array.isArray(selectedCells)||selectedCells.length!==rows*cols||new Set(selectedCells.map(c=>c.Row+':'+c.Cell)).size!==rows*cols||selectedCells.some(c=>c.Row<0||c.Row>=rows||c.Cell<0||c.Cell>=cols))return {ok:false,error:'后续整表选区检查失败；先前单元格样式可能已生效，禁止重放'};if(style.marginCm||style.align){const props={};if(style.marginCm){const mm=style.marginCm*10;props.cellMargin={top:mm,right:mm,bottom:mm,left:mm}}if(style.align)props.jc={left:1,center:2,right:0}[style.align];a.setTblProps(props)}if(style.border)a.setSelectedCellBorders({outside:true},{value:1,size:style.border.pt*25.4/72,color:borderColor})}`
	return documentTableOperationJS(s.documentTableSpec, before, "selectCell", documentTableStyleSetupJS(s), mutation)
}
