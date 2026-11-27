package main

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestDocumentReplaceBeforeComplexSuffix(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional existing Node runtime")
	}
	for _, crosses := range []bool{false, true} {
		spec := documentReplaceSpec{Find: "OLD", Replacement: "NEW", ExpectedCount: 1}
		if crosses {
			spec.Find = "OLD@"
		}
		before := documentLayoutSnapshot{Paragraphs: []documentLayoutParagraph{{Text: "prefix OLD@me"}}}
		steps, err := documentReplaceSteps(before, spec)
		if err != nil {
			t.Fatal(err)
		}
		setup := `global.window=global;let prefix='prefix OLD',selected,writes=0;const suffix='@me';global.btoa=s=>Buffer.from(s).toString('base64');global.atob=s=>Buffer.from(s,'base64').toString();const at=n=>btoa('0|0|0|'+n),offset=s=>Number(atob(s).split('|').pop());window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});const e={config:{mode:{role:'editor'}},docsApi:{isViewMode:false},getParagraphs:()=>[{text:prefix+suffix,children:[{getText:()=>prefix,children:Array.from(prefix)},{atType:1,getText:acc=>{if(acc)acc.Text+=suffix},children:[{}]}]}],getParagraphRange:p=>({start:at(0),end:at(p.text.length),getText:()=>p.text}),generateRange:r=>({...r,getText:()=>(prefix+suffix).slice(offset(r.start),offset(r.end))}),runSelectionCallback:async(r,f)=>{selected=r;return f()},pasteDataInternal:async value=>{writes++;prefix=prefix.slice(0,offset(selected.start))+value+prefix.slice(offset(selected.end))}};window._sdk={options:{user:{id:1}},editor:e};`
		assertion := fmt.Sprintf(`if(%t){if(r.ok!==false||writes!==0)throw Error('complex target was mutated')}else if(!r.ok||writes!==1||prefix+suffix!=='prefix NEW@me')throw Error('simple target before mention was rejected')`, crosses)
		if out, err := exec.Command(node, "-e", setup+documentReplaceJS(spec, before, steps)+`.then(r=>{`+assertion+`}).catch(e=>{console.error(e);process.exitCode=1})`).CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
}
