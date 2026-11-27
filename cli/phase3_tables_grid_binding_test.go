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

func TestPhase3EmptyViewBindsActiveNativeRenderer(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	script := `global.window=global;global.innerWidth=1200;global.innerHeight=800;global.performance={timeOrigin:9};global.getComputedStyle=()=>({visibility:'visible',display:'block',overflowX:'visible',overflowY:'visible'});let mode='',kind='Grid';const rect=()=>({left:0,top:0,right:600,bottom:500,width:600,height:500});const root={getBoundingClientRect:rect,parentElement:null,contains:el=>el===root};const view={getGuid:()=> 'v1',getType:()=>kind,getTable:()=>({getGuid:()=> 't1',getBase:()=>({getDocumentTimezone:()=> 'UTC'})}),getRenderRecords:(filter,stats)=>{if(filter!==true||stats!==false)throw Error('wrong rendering scope');return mode==='missing-records'?{}:{recordGuids:[]}}};const render={getViewModel:()=>view};Object.defineProperty(render,'container',{get:()=>mode==='wrong-container'?{}:root});const editor={getActiveRenderView:()=>render,getActiveView:()=>mode==='wrong-active'?{}:view};root.__reactFiber$test={memoizedProps:{resolveTableInst:()=>{}},stateNode:{editor},return:null};global.document={querySelectorAll:selector=>selector==='#table-editor'?[root]:[]};const read=` + phase3ReadNativeView + `;for(const type of ['Grid','Kanban','Calendar']){kind=type;for(const scenario of ['','wrong-container','wrong-active','missing-records']){mode=scenario;const result=read({tableGuid:'t1',viewGuid:'v1',viewType:type,expectedRecords:{}},'#table-editor');if(!scenario){if(result.error||result.viewGuid!=='v1'||result.nativeRecordGuids.length!==0||Object.keys(result.renderedRecords).length!==0)throw Error(JSON.stringify(result))}else if(!result.error)throw Error('unsafe empty binding '+scenario)}};`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestPhase3EmptyGridDoesNotHideStaleRenderedCells(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	script := `global.window=global;global.innerWidth=1200;global.innerHeight=800;global.performance={timeOrigin:9};global.getComputedStyle=()=>({visibility:'visible',display:'block',overflowX:'visible',overflowY:'visible'});let foreign=false;const rect=()=>({left:0,top:0,right:600,bottom:500,width:600,height:500});const root={getBoundingClientRect:rect,parentElement:null,contains:el=>el===root||(!foreign&&el===cell)};const grid={parentElement:root,getBoundingClientRect:rect};const cell={getBoundingClientRect:rect,parentElement:grid,innerText:'Stale visible record',closest:()=>grid,getAttribute:k=>k==='data-field-guid'?'primary':'stale'};const view={getGuid:()=> 'v1',getType:()=> 'Grid',getTable:()=>({getGuid:()=> 't1'}),getRenderRecords:()=>({recordGuids:[]})};const render={getViewModel:()=>view,container:root};const editor={getActiveRenderView:()=>render,getActiveView:()=>view};root.__reactFiber$test={memoizedProps:{resolveTableInst:()=>{}},stateNode:{editor},return:null};global.document={querySelectorAll:selector=>selector==='#table-editor'?[root]:selector.includes('.cell[')?[cell]:[]};const read=` + phase3ReadNativeView + `;const spec={tableGuid:'t1',viewGuid:'v1',viewType:'Grid',primaryFieldGuid:'primary',expectedRecords:{}};let result=read(spec,'#table-editor');if(result.error||result.nativeRecordGuids.length!==0||result.renderedRecords.stale?.title!=='Stale visible record')throw Error('stale visible cell hidden by empty model');foreign=true;result=read(spec,'#table-editor');if(result.error||result.renderedRecords.stale)throw Error('foreign cell accepted');`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
