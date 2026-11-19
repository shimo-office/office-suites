package main

import (
	"os/exec"
	"testing"
)

func TestDocumentFormatExactCharacterSelection(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional JavaScript selection regression requires existing Node runtime")
	}
	spec, err := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": "汉字 x2 H2O", "match": "x2", "offset": float64(1), "length": float64(1), "attrs": map[string]any{"verticalAlign": "superscript"}})
	if err != nil {
		t.Fatal(err)
	}
	script := `global.window=global;global.btoa=s=>Buffer.from(s,"binary").toString("base64");global.atob=s=>Buffer.from(s,"base64").toString("binary");let writes=[];const text="汉字 x2 H2O",runs=["汉字 x","2 H2O"];
 const full={start:btoa("0,3,|0|0|0"),end:btoa("0,3,|0|2|1"),getText:()=>text+"\r\n"};let selection;
 const point=p=>{const a=atob(p).split('|').slice(-2).map(Number);return runs.slice(0,a[0]).reduce((n,t)=>n+Array.from(t).length,0)+a[1]};
 const editor={config:{mode:{role:"editor"}},docsApi:{isViewMode:false,setTextVertAlign:v=>writes.push({v,start:point(selection.start),end:point(selection.end)})},getParagraphs:()=>[{children:runs.map(t=>({children:Array.from(t),getText:()=>t}))}],getParagraphRange:()=>full,generateRange:r=>({...r,getText:()=>Array.from(text).slice(point(r.start),point(r.end)).join('')}),runSelectionCallback:(r,f)=>{selection=r;return f()}};
 global._sdk={editor,options:{user:{id:1}}};global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:["1"]});
 (async()=>{const result=await ` + documentFormatJS(spec) + `;if(!result.ok||result.selectedText!=="2"||writes.length!==1||writes[0].v!==1||writes[0].start!==4||writes[0].end!==5)throw Error(JSON.stringify({result,writes}));
 writes=[];global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:["1","2"]});const denied=await ` + documentFormatJS(spec) + `;if(denied.ok||writes.length)throw Error("foreign collaborator wrote content");
 global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:["1"]});editor.docsApi.isViewMode=true;const readonly=await ` + documentFormatJS(spec) + `;if(readonly.ok||writes.length)throw Error("readonly document wrote content");
 editor.docsApi.isViewMode=false;editor.config.mode.role="viewer";const viewer=await ` + documentFormatJS(spec) + `;if(viewer.ok||writes.length)throw Error("viewer role with nonview canvas wrote content");
 editor.config.mode.role="editor";let ids=["1"];global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids});full.getText=async()=>{await Promise.resolve();ids=["1","2"];return text+"\r\n"};const entered=await ` + documentFormatJS(spec) + `;if(entered.ok||writes.length)throw Error("collaborator entered during range await but content was written");
 ids=["1"];editor.config.mode.role="editor";full.getText=async()=>{await Promise.resolve();editor.config.mode.role="viewer";return text+"\r\n"};const revoked=await ` + documentFormatJS(spec) + `;if(revoked.ok||writes.length)throw Error("permission revoked during range await but content was written");

 })().catch(e=>{console.error(e);process.exitCode=1});`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("selection regression: %v\n%s", err, out)
	}
}

func TestDocumentFormatUnderlineUsesNativeValueField(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("existing Node runtime")
	}
	for _, enabled := range []bool{true, false} {
		spec, err := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": "target", "attrs": map[string]any{"underline": enabled}})
		if err != nil {
			t.Fatal(err)
		}
		want := "none"
		if enabled {
			want = "single"
		}
		script := `global.window=global;let payload;const editor={config:{mode:{role:'editor'}},docsApi:{isViewMode:false,setUnderline:v=>{payload=v}},getParagraphs:()=>[{}],getParagraphRange:()=>({getText:()=> 'target\r\n'}),runSelectionCallback:(r,f)=>f()};global._sdk={editor,options:{user:{id:1}}};global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});(async()=>{const r=await ` + documentFormatJS(spec) + `;if(!r.ok||payload.value!=='` + want + `'||'type'in payload||payload.value==='single'&&payload.color!=='auto')throw Error('incorrect native underline input '+JSON.stringify(payload))})().catch(e=>{console.error(e);process.exitCode=1});`
		if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
			t.Fatalf("%v %s", e, out)
		}
	}
}
