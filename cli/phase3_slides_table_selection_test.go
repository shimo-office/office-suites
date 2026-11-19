package main

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestPhase3SlideTableRequiresExactNativeRange(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional Node regression runtime")
	}
	for _, op := range []string{"merge", "split", "style"} {
		for _, mode := range []string{"ready", "wrong-table", "wrong-range", "cannot", "missing-method"} {
			t.Run(op+"/"+mode, func(t *testing.T) {
				spec := phase3SlidesSpec{SlideID: "a", Table: &phase3SlideTableSpec{Name: "Table", Operation: op, Rows: 4, Columns: 2, HeaderFillColor: "#0000FF"}}
				script := fmt.Sprintf(`const mode=%q,op=%q;let writes=0,focused=false,positions=[];const table={id:'table',rowCount:4,columnCount:2,getRange:()=>({setSpan(){}})};
const native={parent:{id:'table'},selection:{_$isUse$_:false,_$cellPositions$_:[]}};const selection={get _$selectedTable$_(){return mode==='wrong-table'?{parent:{id:'other'}}:native},get _$selectedTextSp$_(){return focused?native.parent:null}};
const editor={presentation:{_$selectionState$_:selection}},core={getSlideElements:()=>[{id:'table',name:'Table',type:'Table'}],setTextRange(){focused=false},selectElementByIds(){},setTableRangeStyle(e,r,style){if(Object.keys(style).length)throw Error('selection wrote styles');positions=[];if(focused){for(let row=r.row;row<r.row+r.rowCount;row++)for(let col=r.column;col<r.column+r.columnCount;col++)positions.push({row,_$cell$_:col});if(mode==='wrong-range')positions=[{row:3,_$cell$_:1}]}focused=true;native.selection._$isUse$_=positions.length>0;native.selection._$cellPositions$_=positions},canMergeTableCells:()=>mode!=='cannot'&&positions.length===2,canSplitTableCells:()=>mode!=='cannot'&&positions.length===1,mergeCells(){writes++},splitCell(){writes++},setCellsBackground(){writes++}};if(mode==='missing-method')delete core[op==='merge'?'mergeCells':op==='split'?'splitCell':'setCellsBackground'];
global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:[1]}),presentationSDK:{editor:{workbench:{isReady:()=>true,permissions:{canEdit:()=>true},context:{getUser:()=>({id:1})},editor,core}},API:{slides:{getSlides:()=>[{id:'a',getIndex:()=>0,getTables:async()=>[table]}],setCurrentSlideIndex(){}}}}};
(async()=>{const result=await %s;const shouldWrite=mode==='ready'||(op==='style'&&mode==='cannot');if(shouldWrite){if(result.ok!==true||writes!==1)throw Error('native action not applied exactly once: '+JSON.stringify({result,writes}))}else if(result.ok!==false||writes!==0)throw Error('unsafe range allowed write: '+JSON.stringify({result,writes}));})().catch(e=>{console.error(e);process.exitCode=1});`, mode, op, phase3SlideTableMutationJS(spec, []string{"a"}))
				if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
					t.Fatalf("%v\n%s", e, out)
				}
			})
		}
	}
}
