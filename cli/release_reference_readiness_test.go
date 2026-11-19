package main

import (
	"os/exec"
	"testing"
)

func TestReleaseReferenceMenuReadinessRetry(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	script := `let mode='delayed',reads=0,waits=0;global.setTimeout=f=>{waits++;f()};global.getComputedStyle=()=>({visibility:'visible',display:'block'});const oi={Ea:{getConfig:()=>({disableRefField:false})},getActiveTable:()=>({getGuid:()=> 't1'})};const el={getBoundingClientRect:()=>({width:10,height:10}),getAttribute:()=> 'ReferenceRecord',__reactFiber$test:{memoizedProps:{oi},return:null}};global.document={querySelectorAll:()=>{reads++;return mode==='never'||mode==='delayed'&&reads<3?[]:[mode==='unbound'?{...el,__reactFiber$test:null}:el]}};(async()=>{for(mode of ['delayed','never','unbound']){reads=0;waits=0;let result,error;try{result=await ` + phase3ReferenceCapabilityReadyRead + `}catch(e){error=e}if(mode==='delayed'){if(error||result.referenceTypeOffered!==true||reads!==3||waits!==2)throw Error('delayed menu not recovered')}else if(mode==='never'){if(!error||reads!==26||waits!==25)throw Error('retry must be bounded')}else if(!error||reads!==1||waits!==0)throw Error('binding errors must fail immediately')}})().catch(e=>{console.error(e);process.exit(1)});`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
