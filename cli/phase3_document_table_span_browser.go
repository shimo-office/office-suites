package main

import "encoding/json"

// Select via the native document range API; never fabricate Table.Selection data.
func documentTableSpanJS(s documentTableSpec, before documentTablesSnapshot) string {
	anchor, _ := documentTableAnchor(before, s)
	request, _ := json.Marshal(s)
	baseline, _ := json.Marshal(before.Tables)
	return `(async()=>{` + phase3PresenceGuardJS() + documentRangeJS(anchor) + `const request=` + string(request) + `,baseline=` + string(baseline) + `;
 const cellFor=p=>p?.getParent?.()?.getParent?.(),cell=cellFor(p),row=cell?.Row,table=row?.Table,tables=[];
 for(const paragraph of paragraphs){const t=cellFor(paragraph)?.Row?.Table;if(t&&!tables.includes(t))tables.push(t)}
 if(tables.length!==baseline.length||tables[request.table-1]!==table||row?.Index!==request.row-1||cell?.Index!==request.column-1)return {ok:false,error:'目标表格或单元格模型身份与导出不符'};
 const validate=()=>tables.every((t,ti)=>Array.isArray(t.children)&&t.children.length===baseline[ti].rows.length&&t.children.every((r,ri)=>Array.isArray(r.children)&&r.children.length===baseline[ti].rows[ri].length&&r.children.every((c,ci)=>{const bc=baseline[ti].rows[ri][ci];return c.Row===r&&c.Index===ci&&c.getGridSpan?.()===bc.gridSpan&&c.getVMergedRows?.()===1&&bc.paragraphs.every(pi=>cellFor(paragraphs[pi-1])===c)})));
 if(!validate()||typeof a.getCurTable!=='function'||typeof table.getSelections!=='function'||typeof a[request.operation==='merge'?'mergeCells':'splitCell']!=='function')return {ok:false,error:'合并拆分模型或方法未就绪'};
 for(const bt of baseline)for(const r of bt.rows)for(const c of r)for(let i=0;i<c.paragraphs.length;i++){if((await e.getParagraphRange(paragraphs[c.paragraphs[i]-1]).getText()).replace(/\r?\n$/,'')!==c.paragraphTexts[i])return {ok:false,error:'表格段落与导出基线不符，未写入'}}
 const target=baseline[request.table-1].rows[request.row-1][request.column-1];
 if(request.operation==='merge'){if(typeof table.checkOfMerge!=='function')return {ok:false,error:'缺少原生合并可行性检查'};const end=baseline[request.table-1].rows[request.row-1][request.endColumn-1],last=e.getParagraphRange(paragraphs[end.paragraphs[end.paragraphs.length-1]-1]);if(typeof range.start!=='string'||typeof last.end!=='string')return {ok:false,error:'无法构造完整单元格选区'};range=e.generateRange({start:range.start,end:last.end})}
 else if(typeof a.selectCell!=='function')return {ok:false,error:'原生单元格选择方法未就绪'};
 return await e.runSelectionCallback(range,()=>{` + phase3PresenceGuardJS() + `
 if(a.isViewMode!==false||e.config?.mode?.role!=='editor'||e.canHandleChanges===false||!validate()||a.getCurTable()!==table)return {ok:false,error:'写入前权限或表格身份变化'};
 if(request.operation==='split'){if(table.curCell!==cell)return {ok:false,error:'当前单元格不是拆分目标'};a.selectCell()}
 const selectedCells=table.getSelections(),count=request.operation==='merge'?request.endColumn-request.column+1:1;
 if(table.Selection?.Use!==true||table.Selection?.isTextMode!==false||!Array.isArray(selectedCells)||selectedCells.length!==count||selectedCells.some((c,i)=>c.Row!==request.row-1||c.Cell!==request.column-1+i||table.children[c.Row]?.children[c.Cell]!==row.children[request.column-1+i]))return {ok:false,error:'真实表格选区与请求范围不符，未写入'};
 if(request.operation==='merge'){const check=table.checkOfMerge();if(check?.canMerge!==true||check.gridStart!==(target.startGridCol||0)||check.gridEnd!==(target.startGridCol||0)+count-1)return {ok:false,error:'原生合并检查与逻辑网格范围不符'};a.mergeCells()}
 else {if(cell.getGridSpan()!==request.columns)return {ok:false,error:'拆分目标跨度已改变'};a.splitCell(request.columns,1)}
 return {ok:true,submitted:true,operation:request.operation,table:request.table,row:request.row,column:request.column,selectedCells};});})()`
}
