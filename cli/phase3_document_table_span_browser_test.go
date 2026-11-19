package main

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestDocumentTableSpanSelectionGuards(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	for _, op := range []string{"merge", "split"} {
		t.Run(op, func(t *testing.T) {
			row := documentSpanCell(1, "A") + documentSpanCell(1, "B") + documentSpanCell(1, "C")
			spec := documentTableSpec{Operation: op, Table: 1, Row: 1, Column: 1, EndColumn: 2, ExpectedCell: "A"}
			if op == "split" {
				row = documentSpanCell(2, "A", "B") + documentSpanCell(1, "C")
				spec.EndColumn = 0
				spec.Columns = 2
				spec.ExpectedCell = "A\nB"
			}
			before := documentSpanFixture(t, row, "F")
			raw, _ := json.Marshal(before.Tables)
			script := `global.window=global;let writes=0,mode='',ids=['1'],chosen=[],splitArgs;const baseline=` + string(raw) + `;const paragraphs=[{text:'UNCHANGED'}];const tables=baseline.map(bt=>{const t={children:[],Selection:{Use:true,isTextMode:false},getSelections:()=>chosen,checkOfMerge:()=>({canMerge:true,gridStart:0,gridEnd:1})};t.children=bt.rows.map((br,ri)=>{const r={Index:ri,Table:t,children:[]};r.children=br.map((bc,ci)=>{const c={Index:ci,Row:r,getGridSpan:()=>bc.gridSpan,getVMergedRows:()=>1};bc.paragraphs.forEach((pi,j)=>paragraphs[pi-1]={text:bc.paragraphTexts[j],getParent:()=>({getParent:()=>c})});return c});return r});t.curCell=t.children[0].children[0];return t});const table=tables[0];const e={config:{mode:{role:'editor'}},docsApi:{isViewMode:false,getCurTable:()=>mode==='wrong-table'?{}:table,mergeCells:()=>writes++,splitCell:(...args)=>{writes++;splitArgs=args},selectCell:()=>{chosen=[{Row:0,Cell:mode==='wrong-cell'?1:0}]}},getParagraphs:()=>paragraphs,getParagraphRange:p=>({start:paragraphs.indexOf(p)+':start',end:paragraphs.indexOf(p)+':end',getText:()=>p.text+'\r\n'}),generateRange:r=>r,runSelectionCallback:(range,fn)=>{chosen=[{Row:0,Cell:0},{Row:0,Cell:mode==='wrong-cell'?2:1}];if(mode==='collaborator')ids=['1','2'];return fn()}};global._sdk={editor:e,options:{user:{id:1}}};global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids});(async()=>{for(const test of ['','wrong-cell','wrong-table','collaborator']){mode=test;ids=['1'];writes=0;const result=await ` + documentTableJS(spec, before) + `;if(test===''){if(!result.ok||writes!==1)throw Error(JSON.stringify(result));`
			if op == "split" {
				script += `if(JSON.stringify(splitArgs)!=='[2,1]')throw Error('split argument order');`
			}
			script += `}else if(result.ok||writes!==0)throw Error('unsafe '+test+' '+JSON.stringify(result))}})().catch(e=>{console.error(e);process.exitCode=1})`
			if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}
