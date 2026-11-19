package main

import (
	"os/exec"
	"testing"
)

func TestPhase3GridProofBindsActualRenderContainer(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	script := `global.window=global;global.innerWidth=1200;global.innerHeight=800;global.performance={timeOrigin:9};global.getComputedStyle=()=>({visibility:'visible',display:'block',overflowX:'visible',overflowY:'visible'});let mode='';const rect=()=>({left:0,top:0,right:600,bottom:500,width:600,height:500});const root={getBoundingClientRect:rect,parentElement:null,contains:el=>el===header||el===cell};const header={getBoundingClientRect:rect,parentElement:root,closest:()=>root};const cell={getBoundingClientRect:rect,parentElement:root,innerText:'P3-A',closest:()=>mode==='wrong-record-container'?{}:root,getAttribute:k=>k==='data-field-guid'?'primary':'A'};const view={getGuid:()=> 'grid1',getType:()=> 'Grid',getTable:()=>({getGuid:()=>mode==='wrong-table'?'other':'table1'})};const render={getViewModel:()=>view};Object.defineProperty(render,'container',{get:()=>mode==='wrong-container'?{}:root});const editor={getActiveRenderView:()=>render,getActiveView:()=>mode==='wrong-active'?{}:view};root.__reactFiber$test={memoizedProps:{id:'table-editor'},return:{memoizedProps:{resolveTableInst:()=>{},requestAdaptor:{}},stateNode:{Fi:editor},return:null}};global.document={querySelectorAll:selector=>selector.includes('.cell[')?[cell]:[header]};const spec={tableGuid:'table1',viewGuid:'grid1',viewType:'Grid',primaryFieldGuid:'primary'};const read=` + phase3ReadNativeView + `;for(const scenario of ['','wrong-container','wrong-active','wrong-table','wrong-record-container']){mode=scenario;const result=read(spec,'headers');if(!scenario){if(!result.ok&&result.error||result.viewGuid!=='grid1'||result.renderedRecords.A.title!=='P3-A')throw Error(JSON.stringify(result))}else if(scenario==='wrong-record-container'){if(result.renderedRecords.A)throw Error('foreign grid record accepted')}else if(!result.error)throw Error('unsafe binding '+scenario)};console.log('native container binding checked')`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
