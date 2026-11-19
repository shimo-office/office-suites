package main

import (
	"os/exec"
	"testing"
)

func TestDocumentTableStyleSelectionResetBeforeWrite(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	margin := .2
	spec := documentTableStyleSpec{documentTableSpec: documentTableSpec{Operation: "format", Table: 1, Row: 1, Column: 1, ExpectedCell: "a"}, Fill: "#0000FF", MarginCm: &margin}
	before := documentTablesSnapshot{Tables: []documentTable{{Rows: [][]documentTableCell{{{Text: "a", Paragraph: 1}, {Text: "b", Paragraph: 2}}}}}}
	script := `global.window=global;let writes=0,selected=[],wrong=false,brokenAll=false,sticky=false;const table={children:[]},row={Index:0,Table:table,children:[]};table.children=[row];const cells=['a','b'].map((text,i)=>({Index:i,Row:row,getGridSpan:()=>1,getVMergedRows:()=>1}));row.children=cells;table.curCell=cells[0];const paragraphs=cells.map((cell,i)=>({getParent:()=>({getParent:()=>cell}),text:['a','b'][i]}));const e={config:{mode:{role:'editor'}},docsApi:{isViewMode:false,getCurTable:()=>table,selectCell:()=>{if(sticky)return;selected=[{Row:0,Cell:wrong?1:table.curCell.Index}]},getSelectedTableCells:()=>({data:selected}),selectTable:()=>{sticky=true;selected=brokenAll?[{Row:0,Cell:1}]:[{Row:0,Cell:0},{Row:0,Cell:1}]},setCellsBackground:()=>{if(selected.length!==1||selected[0].Cell!==0)throw Error('wrong cell fill');writes++},setTblProps:()=>writes++},getParagraphs:()=>paragraphs,getParagraphRange:p=>({getText:()=>p.text+'\r\n'}),runSelectionCallback:(range,fn)=>{table.curCell=cells[0];sticky=false;return fn()}};global._sdk={editor:e,options:{user:{id:1}}};global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});(async()=>{const pre=await ` + documentTableStylePreflightJS(spec, before) + `;if(!pre.ok||pre.mutationAttempted!==false||writes)throw Error('preflight wrote');const result=await ` + documentTableStyleJS(spec, before) + `;if(!result.ok||writes!==2)throw Error(JSON.stringify(result));brokenAll=true;writes=0;const partial=await ` + documentTableStyleJS(spec, before) + `;if(partial.ok||writes!==1||!partial.error.includes('可能已生效'))throw Error('partial write misreported');brokenAll=false;wrong=true;writes=0;const rejected=await ` + documentTableStylePreflightJS(spec, before) + `;if(rejected.ok||writes)throw Error('bad selection accepted')})().catch(e=>{console.error(e);process.exitCode=1})`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
