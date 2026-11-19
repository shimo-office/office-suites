package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type phase3SlideTableSpec struct {
	Row             int     `json:"row"`
	Column          int     `json:"column"`
	HeaderFillColor string  `json:"headerFillColor,omitempty"`
	BorderColor     string  `json:"borderColor,omitempty"`
	BorderWidth     float64 `json:"borderWidth,omitempty"`
	Name            string  `json:"tableName"`
	Operation       string  `json:"operation"`
	Index           int     `json:"index"`
	Rows            int     `json:"rows"`
	Columns         int     `json:"columns"`
}
type phase3SlideTableGrid struct {
	RowMetadata, ColumnMetadata []string
	Rows                        [][]*artifactNode
	Widths, Heights             []int64
}

func preparePhase3SlideTable(p map[string]any) (*phase3SlideTableSpec, error) {
	if p["operation"] != "style" && p["operation"] != "merge" && p["operation"] != "split" && len(p) != 4 {
		return nil, fmt.Errorf("表格编辑仅接受 slideId/tableName/operation/index")
	}
	s := &phase3SlideTableSpec{}
	s.Name, _ = p["tableName"].(string)
	s.Operation, _ = p["operation"].(string)
	if s.Name == "" || len(s.Name) > 250 || strings.ContainsAny(s.Name, "\r\n\x00") {
		return nil, fmt.Errorf("tableName 必须为唯一现有表格名称")
	}
	if s.Operation == "merge" || s.Operation == "split" {
		return s, preparePhase3SlideTableSpan(p, s)
	}
	if s.Operation == "style" {
		return s, preparePhase3SlideTableStyle(p, s)
	}
	switch s.Operation {
	case "insert-row", "delete-column":
	default:
		return nil, fmt.Errorf("此动作仅证实 insert-row 与 delete-column；removeSpan 不是可靠拆分接口")
	}
	n, ok := p["index"].(float64)
	if !ok || n < 1 || n > 1000 || n != float64(int(n)) {
		return nil, fmt.Errorf("index 必须为从 1 开始的整数；插入发生在该行之前")
	}
	s.Index = int(n) - 1
	return s, nil
}
func phase3TableNode(frame *artifactNode) (*artifactNode, error) {
	if frame.Name.Space != artifactSlideNS || frame.Name.Local != "graphicFrame" {
		return nil, fmt.Errorf("目标不是表格框架")
	}
	g := phase3SlideChild(frame, artifactDrawingNS, "graphic")
	d := phase3SlideChild(g, artifactDrawingNS, "graphicData")
	if d == nil || d.attr("uri") != "http://schemas.openxmlformats.org/drawingml/2006/table" {
		return nil, fmt.Errorf("目标不是标准 PPTX 文内表格")
	}
	t := phase3SlideChild(d, artifactDrawingNS, "tbl")
	if t == nil {
		return nil, fmt.Errorf("缺少文内表格节点")
	}
	return t, nil
}
func readPhase3SlideTableGrid(t *artifactNode, allowSpan bool) (phase3SlideTableGrid, error) {
	out := phase3SlideTableGrid{}
	grid := phase3SlideChild(t, artifactDrawingNS, "tblGrid")
	if grid == nil {
		return out, fmt.Errorf("缺少表格列网格")
	}
	for _, c := range grid.children("gridCol") {
		w, e := phase3ShapeInteger(c, "w")
		if e != nil || w <= 0 {
			return out, fmt.Errorf("无效列宽")
		}
		out.Widths = append(out.Widths, w)
		metadata, e := phase3TableDimensionMetadata(c, "w", "")
		if e != nil {
			return out, e
		}
		out.ColumnMetadata = append(out.ColumnMetadata, metadata)
	}
	if len(out.Widths) == 0 || len(out.Widths) > 100 {
		return out, fmt.Errorf("表格列数超出验证范围")
	}
	for _, r := range t.children("tr") {
		h, e := phase3ShapeInteger(r, "h")
		if e != nil || h <= 0 {
			return out, fmt.Errorf("无效行高")
		}
		out.Heights = append(out.Heights, h)
		metadata, e := phase3TableDimensionMetadata(r, "h", "tc")
		if e != nil {
			return out, e
		}
		out.RowMetadata = append(out.RowMetadata, metadata)
		row := r.children("tc")
		if len(row) != len(out.Widths) {
			return out, fmt.Errorf("仅支持规则未合并表格")
		}
		for _, c := range row {
			if allowSpan {
				continue
			}
			for _, k := range []string{"gridSpan", "rowSpan", "hMerge", "vMerge"} {
				v := c.attr(k)
				if v != "" && v != "0" && v != "false" && (k == "hMerge" || k == "vMerge") || v != "" && v != "1" && (k == "gridSpan" || k == "rowSpan") {
					return out, fmt.Errorf("插删操作拒绝包含合并单元格的表格")
				}
			}
		}
		out.Rows = append(out.Rows, row)
	}
	if len(out.Rows) == 0 || len(out.Rows) > 100 {
		return out, fmt.Errorf("表格行数超出验证范围")
	}
	return out, nil
}
func phase3EqualTableCell(a, b *artifactNode) bool {
	ca, e := phase3SlideCanonical(phase3TableNativeCell(a), map[string]string{})
	if e != nil {
		return false
	}
	cb, e := phase3SlideCanonical(phase3TableNativeCell(b), map[string]string{})
	return e == nil && reflect.DeepEqual(ca, cb)
}
func verifyPhase3SlideTableGrid(b, a phase3SlideTableGrid, s phase3SlideTableSpec) error {
	if e := phase3VerifyTableMetadata(b, a, s); e != nil {
		return e
	}
	switch s.Operation {
	case "merge", "split":
		return verifyPhase3SlideTableSpan(b, a, s)
	case "style":
		return verifyPhase3SlideTableStyle(b, a, s)
	case "insert-row":
		if s.Index < 0 || s.Index >= len(b.Rows) || len(a.Rows) != len(b.Rows)+1 || !reflect.DeepEqual(a.Widths, b.Widths) {
			return fmt.Errorf("插入行后的行列数或列宽不符")
		}
		for r, row := range a.Rows {
			if r == s.Index {
				if len(row) != len(b.Widths) || a.Heights[r] <= 0 {
					return fmt.Errorf("新增行结构错误")
				}
				for _, c := range row {
					if strings.TrimSpace(phase3SlideText(c)) != "" {
						return fmt.Errorf("新增行不为空")
					}
				}
				continue
			}
			old := r
			if r > s.Index {
				old--
			}
			if a.Heights[r] != b.Heights[old] || len(row) != len(b.Rows[old]) {
				return fmt.Errorf("原行高度或列数改变")
			}
			for col, c := range row {
				if !phase3EqualTableCell(c, b.Rows[old][col]) {
					return fmt.Errorf("原单元格内容/格式/位置发生变化")
				}
			}
		}
	case "delete-column":
		if s.Index < 0 || s.Index >= len(b.Widths) || len(b.Widths) < 2 || len(a.Rows) != len(b.Rows) || len(a.Widths) != len(b.Widths)-1 || !reflect.DeepEqual(a.Heights, b.Heights) {
			return fmt.Errorf("删列后的行列数或行高不符")
		}
		expected := append([]int64(nil), b.Widths[:s.Index]...)
		expected = append(expected, b.Widths[s.Index+1:]...)
		if !reflect.DeepEqual(expected, a.Widths) {
			return fmt.Errorf("保留列宽改变")
		}
		for r, row := range a.Rows {
			if len(row) != len(a.Widths) {
				return fmt.Errorf("删列后的物理单元格数不符")
			}
			for col, c := range row {
				old := col
				if col >= s.Index {
					old++
				}
				if !phase3EqualTableCell(c, b.Rows[r][old]) {
					return fmt.Errorf("删列误改了保留单元格")
				}
			}
		}
	default:
		return fmt.Errorf("未证实表格操作")
	}
	return nil
}
func phase3MapTable(slide *artifactNode, name string, fn func(*artifactNode) (*artifactNode, error)) (*artifactNode, error) {
	tree, e := phase3ShapeTree(slide)
	if e != nil {
		return nil, e
	}
	at := -1
	for i, n := range tree.Children {
		if phase3ShapeName(n) == name {
			if at >= 0 {
				return nil, fmt.Errorf("表格名称不唯一")
			}
			at = i
		}
	}
	if at < 0 {
		return nil, fmt.Errorf("表格不存在")
	}
	frame := tree.Children[at]
	if _, e = phase3TableNode(frame); e != nil {
		return nil, e
	}
	value, e := fn(frame)
	if e != nil {
		return nil, e
	}
	outTree := *tree
	outTree.Children = append([]*artifactNode(nil), tree.Children...)
	outTree.Children[at] = value
	cs := phase3SlideChild(slide, artifactSlideNS, "cSld")
	outCS := *cs
	outCS.Children = append([]*artifactNode(nil), cs.Children...)
	for i, c := range outCS.Children {
		if c == tree {
			outCS.Children[i] = &outTree
		}
	}
	out := *slide
	out.Children = append([]*artifactNode(nil), slide.Children...)
	for i, c := range out.Children {
		if c == cs {
			out.Children[i] = &outCS
		}
	}
	return &out, nil
}
func phase3MaskTable(frame *artifactNode) *artifactNode {
	return phase3MaskTableAt(frame, "")
}
func phase3MaskTableAt(frame *artifactNode, parent string) *artifactNode {
	path := phase3TableQualifiedName(frame)
	if parent != "" {
		path = parent + "/" + path
	}
	out := *frame
	out.Children = nil
	out.Attr = nil
	for _, a := range frame.Attr {
		if a.Name.Space == "" {
			if path == "p:graphicFrame/p:xfrm/a:ext" && (a.Name.Local == "cx" || a.Name.Local == "cy") {
				continue
			}
			// The current native graphic-frame writer emits these identity
			// values. They are local SDK serialization, not PML XSD defaults.
			if path == "p:graphicFrame/p:xfrm" && (a.Name.Local == "rot" && a.Value == "0" || (a.Name.Local == "flipH" || a.Name.Local == "flipV") && (a.Value == "0" || a.Value == "false")) {
				continue
			}
			if path == "p:graphicFrame/a:graphic/a:graphicData/a:tbl/a:tblPr" && (a.Value == "0" || a.Value == "false") {
				switch a.Name.Local {
				case "rtl", "firstRow", "firstCol", "lastRow", "lastCol", "bandRow", "bandCol":
					continue
				}
			}
		}
		out.Attr = append(out.Attr, a)
	}
	for _, c := range frame.Children {
		if path == "p:graphicFrame/a:graphic/a:graphicData" && c.Name.Space == artifactDrawingNS && c.Name.Local == "tbl" {
			stub := *c
			stub.Children = nil
			for _, part := range c.Children {
				if part.Name.Space == artifactDrawingNS && (part.Name.Local == "tr" || part.Name.Local == "tblGrid") {
					continue
				}
				stub.Children = append(stub.Children, phase3MaskTableAt(part, path+"/a:tbl"))
			}
			out.Children = append(out.Children, &stub)
			continue
		}
		out.Children = append(out.Children, phase3MaskTableAt(c, path))
	}
	return &out
}
func phase3SlideTableArtifact(filename string, ids []string, s phase3SlidesSpec) ([]phase3SlidePage, phase3SlideTableGrid, error) {
	var grid phase3SlideTableGrid
	found := false
	pages, e := readPhase3SlideArtifactFiltered(filename, ids, func(i int, n *artifactNode) (*artifactNode, error) {
		if ids[i] != s.SlideID {
			return n, nil
		}
		found = true
		return phase3MapTable(n, s.Table.Name, func(frame *artifactNode) (*artifactNode, error) {
			table, e := phase3TableNode(frame)
			if e != nil {
				return nil, e
			}
			grid, e = readPhase3SlideTableGrid(table, s.Table.Operation == "merge" || s.Table.Operation == "split")
			if e != nil {
				return nil, e
			}
			xf := phase3SlideChild(frame, artifactSlideNS, "xfrm")
			extent := phase3SlideChild(xf, artifactDrawingNS, "ext")
			cx, ex := phase3ShapeInteger(extent, "cx")
			cy, ey := phase3ShapeInteger(extent, "cy")
			width, height := int64(0), int64(0)
			for _, w := range grid.Widths {
				width += w
			}
			for _, h := range grid.Heights {
				height += h
			}
			if ex != nil || ey != nil || cx != width || cy != height {
				return nil, fmt.Errorf("表格框架尺寸与实际行列尺寸不一致")
			}
			return phase3MaskTable(frame), nil
		})
	})
	if e == nil && !found {
		e = fmt.Errorf("目标页面不存在")
	}
	return pages, grid, e
}
func phase3PrepareSlideTable(filename string, ids []string, s *phase3SlidesSpec) error {
	_, grid, e := phase3SlideTableArtifact(filename, ids, *s)
	if e != nil {
		return e
	}
	if e = phase3CheckTableBorderBaseline(grid, *s.Table); e != nil {
		return e
	}
	s.Table.Rows = len(grid.Rows)
	s.Table.Columns = len(grid.Widths)
	if s.Table.Operation == "merge" || s.Table.Operation == "split" {
		return validatePhase3SpanBaseline(grid, *s.Table)
	}
	if s.Table.Operation == "style" && verifyPhase3SlideTableStyle(grid, grid, *s.Table) == nil {
		return fmt.Errorf("表格已满足目标样式，未执行无效写入")
	}
	if s.Table.Operation == "insert-row" && s.Table.Index >= s.Table.Rows {
		return fmt.Errorf("插入行索引超出范围")
	}
	if s.Table.Operation == "delete-column" && (s.Table.Index >= s.Table.Columns || s.Table.Columns < 2) {
		return fmt.Errorf("删列索引超出范围或将删除最后一列")
	}
	return nil
}
func verifyPhase3SlideTableArtifacts(before, after string, ids, next []string, s phase3SlidesSpec) error {
	if s.Table == nil || !reflect.DeepEqual(ids, next) {
		return fmt.Errorf("表格编辑不得改变页面列表")
	}
	bp, b, e := phase3SlideTableArtifact(before, ids, s)
	if e != nil {
		return e
	}
	ap, a, e := phase3SlideTableArtifact(after, next, s)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(bp, ap) {
		return fmt.Errorf("表格身份、其余对象或主题资源发生变化")
	}
	return verifyPhase3SlideTableGrid(b, a, *s.Table)
}
func phase3SlideTableMutationJS(s phase3SlidesSpec, ids []string) string {
	raw, _ := json.Marshal(s)
	list, _ := json.Marshal(ids)
	// The first empty style call enters the native table text focus. Its
	// selectSelf/reset(true) clears a range when starting from object focus.
	// Select again in text focus, then verify exact native cells before writing.
	return `(async()=>{const r=` + string(raw) + `;const expected=` + string(list) + `;` + phase3PresenceGuardJS() + `
 const sdk=window.presentationSDK,wb=sdk?.editor?.workbench,api=sdk?.API?.slides;if(wb?.isReady()!==true||typeof wb.permissions?.canEdit!=='function'||wb.permissions.canEdit()!==true)return {ok:false,error:'未确认编辑权限'};if(JSON.stringify(api.getSlides().map(s=>s.id))!==JSON.stringify(expected))return {ok:false,error:'页面已变化'};const slide=api.getSlides().find(s=>s.id===r.slideId);if(!slide||typeof wb.core.getSlideElements!=='function')return {ok:false,error:'缺少表格定位接口'};const nodes=wb.core.getSlideElements(wb.editor,slide.getIndex()).filter(n=>n.type==='Table'&&n.name===r.table.tableName);if(nodes.length!==1)return {ok:false,error:'表格名不唯一'};const tables=await slide.getTables(),table=tables.find(t=>t.id===nodes[0].id),p=r.table;if(!table||table.rowCount!==p.rows||table.columnCount!==p.columns)return {ok:false,error:'表格结构已变化'};{` + phase3PresenceGuardJS() + `}if(wb.permissions.canEdit()!==true)return {ok:false,error:'编辑权限已改变'};const c=wb.core,e=wb.editor;const selectRange=(range)=>{c.setTableRangeStyle(e,range,{});const state=e.presentation?._$selectionState$_;if(state?._$selectedTextSp$_?.id!==table.id||state?._$selectedTable$_?.parent?.id!==table.id)return false;c.setTableRangeStyle(e,range,{});const native=state._$selectedTable$_,sel=native?.selection,positions=sel?._$cellPositions$_;if(native?.parent?.id!==table.id||state._$selectedTextSp$_?.id!==table.id||sel?._$isUse$_!==true||!Array.isArray(positions)||positions.length!==range.rowCount*range.columnCount)return false;const wanted=[];for(let row=range.row;row<range.row+range.rowCount;row++)for(let column=range.column;column<range.column+range.columnCount;column++)wanted.push(row+':'+column);return positions.every((v,i)=>v.row+':'+v._$cell$_===wanted[i]);};if(p.operation==='merge'||p.operation==='split'){const merging=p.operation==='merge',action=merging?'mergeCells':'splitCell',can=merging?'canMergeTableCells':'canSplitTableCells';if(['setTextRange','selectElementByIds','setTableRangeStyle',can,action].some(k=>typeof c[k]!=='function'))return {ok:false,error:'缺少真实合并/拆分接口'};api.setCurrentSlideIndex(r.slideId);c.setTextRange(e,null);c.selectElementByIds(e,[table.id]);if(!selectRange({row:p.row,column:p.column,rowCount:1,columnCount:merging?2:1})||c[can](e)!==true)return {ok:false,error:'未确认目标单元格选区可合并/拆分'};{` + phase3PresenceGuardJS() + `}if(wb.permissions.canEdit()!==true)return {ok:false,error:'编辑权限已改变'};if(merging)c.mergeCells(e);else c.splitCell(e,2,1)}else if(p.operation==='style'){const need=['setTextRange','selectElementByIds','setTableRangeStyle'];if(p.headerFillColor)need.push('setCellsBackground');if(p.borderColor)need.push('setCellBorders');if(need.some(k=>typeof c[k]!=='function'))return {ok:false,error:'缺少真实单元格样式接口'};api.setCurrentSlideIndex(r.slideId);c.setTextRange(e,null);c.selectElementByIds(e,[table.id]);if(p.headerFillColor){if(!selectRange({row:0,column:0,rowCount:1,columnCount:p.columns}))return {ok:false,error:'未确认表头精确选区'};{` + phase3PresenceGuardJS() + `}if(wb.permissions.canEdit()!==true)return {ok:false,error:'编辑权限已改变'};c.setCellsBackground(e,p.headerFillColor,0)}if(p.borderColor){if(!selectRange({row:0,column:0,rowCount:p.rows,columnCount:p.columns}))return {ok:false,error:'未确认全部单元格选区，先前样式可能已生效，请只读核对'};{` + phase3PresenceGuardJS() + `}if(wb.permissions.canEdit()!==true)return {ok:false,error:'编辑权限已改变，先前样式可能已生效'};c.setCellBorders(e,'ltrbvh',{visible:true,color:p.borderColor,transparent:0,size:p.borderWidth})}}else if(p.operation==='insert-row'){table.insertRows(p.index,1,'before')}else if(p.operation==='delete-column'){table.deleteColumns(p.index,1)}else return {ok:false,error:'未证实表格操作'};const after=(await slide.getTables()).find(t=>t.id===table.id);return {ok:true,tableId:table.id,before:{rows:table.rowCount,columns:table.columnCount},after:after?{rows:after.rowCount,columns:after.columnCount}:null};})()`
}

func phase3TableDimensionMetadata(n *artifactNode, dimension, child string) (string, error) {
	out := *n
	out.Attr = nil
	out.Children = nil
	for _, a := range n.Attr {
		if a.Name.Space == "" && a.Name.Local == dimension {
			continue
		}
		out.Attr = append(out.Attr, a)
	}
	for _, c := range n.Children {
		if c.Name.Space == artifactDrawingNS && c.Name.Local == child {
			continue
		}
		out.Children = append(out.Children, c)
	}
	v, e := phase3SlideCanonical(&out, map[string]string{})
	if e != nil {
		return "", e
	}
	raw, e := json.Marshal(v)
	return string(raw), e
}
func phase3VerifyTableMetadata(b, a phase3SlideTableGrid, s phase3SlideTableSpec) error {
	rows := b.RowMetadata
	cols := b.ColumnMetadata
	if s.Operation == "insert-row" && len(rows) > 0 {
		if s.Index < 0 || s.Index >= len(a.RowMetadata) || len(a.RowMetadata) != len(rows)+1 {
			return fmt.Errorf("行元数据数量不符")
		}
		actual := append([]string(nil), a.RowMetadata[:s.Index]...)
		actual = append(actual, a.RowMetadata[s.Index+1:]...)
		if !reflect.DeepEqual(actual, rows) {
			return fmt.Errorf("原行元数据变化")
		}
	} else if !reflect.DeepEqual(rows, a.RowMetadata) {
		return fmt.Errorf("原行元数据变化")
	}
	if s.Operation == "delete-column" && len(cols) > 0 {
		if s.Index < 0 || s.Index >= len(cols) {
			return fmt.Errorf("列元数据索引无效")
		}
		expected := append([]string(nil), cols[:s.Index]...)
		expected = append(expected, cols[s.Index+1:]...)
		if !reflect.DeepEqual(expected, a.ColumnMetadata) {
			return fmt.Errorf("保留列元数据变化")
		}
	} else if !reflect.DeepEqual(cols, a.ColumnMetadata) {
		return fmt.Errorf("列元数据变化")
	}
	return nil
}
