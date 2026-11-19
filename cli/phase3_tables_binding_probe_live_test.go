package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPhase3GridBindingReadOnlyProbe(t *testing.T) {
	out := os.Getenv("SHIMO_PHASE3_GRID_BINDING_OUT")
	if out == "" {
		t.Skip("dedicated output required")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	b, err := openPhase3TableBrowser("https://shimo.net/tables/DqdxLrxlDMcw78q4?table=61492df5d31&view=2acdb73f09c")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err = b.WaitReady(phase3TableGridReady, phase3EditorReadyTimeout); err != nil {
		t.Fatal(err)
	}
	data, err := b.EvalReadOnly(`(()=>{const rows=[];const el=document.querySelector('.sm-grid-view-container .header-field[data-field-guid]');for(let p=el;p;p=p.parentElement){const keys=Object.keys(p).filter(k=>k.startsWith('__react'));rows.push({tag:p.tagName,className:p.className,keys});for(const key of keys.filter(k=>k.startsWith('__reactFiber$')||k.startsWith('__reactInternalInstance$'))){let f=p[key];for(let i=0;f&&i<50;i++,f=f.return){const props=f.memoizedProps||{};const candidates=[];for(const [name,value] of Object.entries(props)){if(value&&typeof value==='object'){const methods=['getGuid','getType','getTable'].filter(k=>typeof value[k]==='function');if(methods.length)candidates.push({name,methods,guid:typeof value.getGuid==='function'?value.getGuid():null,type:typeof value.getType==='function'?value.getType():null,tableGuid:typeof value.getTable==='function'?value.getTable()?.getGuid?.():null})}}const editors=[];for(const [key,v] of Object.entries(f.stateNode||{})){if(v&&typeof v.getActiveRenderView==='function'&&typeof v.getActiveView==='function'){const render=v.getActiveRenderView(),view=render?.getViewModel?.();editors.push({key,table:view?.getTable?.()?.getGuid?.(),view:view?.getGuid?.(),type:view?.getType?.(),containerTag:render?.container?.tagName,containerId:render?.container?.id,containerClass:render?.container?.className,contains:render?.container?.contains?.(el)})}}rows.push({depth:i,props:Object.keys(props),candidates,editors})}}}return {ok:true,rows}})()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(out, "binding-probe.json"), data); err != nil {
		t.Fatal(err)
	}
	if err = b.Capture(filepath.Join(out, "native-grid.png")); err != nil {
		t.Fatal(err)
	}
}
