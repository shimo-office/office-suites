package main

import (
	"os/exec"
	"testing"
)

func TestPhase3FormPreviewSelectionUsesCurrentReactTree(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	setup := `global.window={logged:{}};const el=id=>({id,getClientRects:()=>[{width:100,height:20}]});const input=el('trigger_no');input.checked=false;const nodes=[el('trigger'),el('rate')];global.document={getElementById:id=>id.startsWith('trigger_')?input:nodes.find(e=>e.id===id),querySelectorAll:()=>nodes,title:'Form'};global.getComputedStyle=()=>({visibility:'visible'});
 const form=option=>({title:'Form',questions:[{guid:'trigger',response:option?{choice:{value:option}}:undefined},{guid:'follow'},{guid:'rate'}]});
 const oldRoot={tag:3},currentRoot={tag:3},rootState={current:currentRoot};oldRoot.stateNode=currentRoot.stateNode=rootState;
 const oldPreview={memoizedProps:{mode:'preview',form:form('yes'),onResponseChange:()=>{}},return:oldRoot};const currentPreview={memoizedProps:{mode:'preview',form:form('no'),onResponseChange:()=>{}},return:currentRoot};
 const oldHost={stateNode:input,return:oldPreview},currentHost={stateNode:input,return:currentPreview};oldRoot.child=oldPreview;oldPreview.child=oldHost;currentRoot.child=currentPreview;currentPreview.child=currentHost;oldHost.alternate=currentHost;currentHost.alternate=oldHost;input.__reactFiber$test=oldHost;
 const spec={known:['trigger','follow','rate'],trigger:'trigger',title:'Form',guid:'form-guid',respondent:false,branch:{option:'yes',visible:['rate'],hidden:['follow'],sequence:['trigger','rate']}};
 const observe=` + phase3FormBranchObservationJS + `;const assert=(v,m)=>{if(!v)throw Error(m)};
 `
	cases := `
 assert(!observe(spec).verified,'old alternate matching expected option must not pass');
 spec.branch.option='no';const current=observe(spec);assert(current.verified&&current.selectedOption==='no'&&current.selectionVerified,'mounted current selection must pass despite input.checked false');
 currentPreview.memoizedProps.form=form(undefined);assert(!observe(spec).verified,'same layout without selection must fail');
 currentPreview.memoizedProps.form=form('no');assert(observe(spec).verified,'ready observation');currentPreview.memoizedProps.form=form('yes');assert(!observe(spec).verified,'final observation must recheck changed selection');
 currentPreview.memoizedProps.form={...form('no'),title:'Other'};assert(!observe(spec).verified,'wrong current form cannot fall back to old matching form');
 currentPreview.memoizedProps.form={...form('no'),questions:form('no').questions.slice(0,2)};assert(!observe(spec).verified,'incomplete GUID set rejected');
 currentPreview.memoizedProps.form=form('no');currentRoot.child=null;assert(!observe(spec).verified,'detached old fiber rejected');
 `
	if out, err := exec.Command(node, "-e", setup+cases).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
}
