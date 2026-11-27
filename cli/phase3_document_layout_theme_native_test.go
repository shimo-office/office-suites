package main

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestDocumentThemeUsesNativeThemeTransaction(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional existing Node runtime")
	}
	for _, mode := range []string{"success", "missing-theme", "missing-start", "missing-finalize", "throw", "replaced-theme"} {
		t.Run(mode, func(t *testing.T) {
			spec := documentLayoutSpec{Action: "document-theme"}
			before := documentLayoutSnapshot{Paragraphs: []documentLayoutParagraph{{Text: "target"}}}
			script := fmt.Sprintf(`global.window=global;const mode=%q;let calls=[];const theme={setCustomThemeFont:(fonts,add)=>{calls.push('theme');if(add!==false)throw Error('unexpected add');if(mode==='throw')throw Error('native failure')}};const model={getTheme:()=>theme};const api={isViewMode:false,controller:{docModel:model},startAction:()=>calls.push('start'),finalizeAction:()=>calls.push('finalize'),setCurThemeFonts:()=>{throw Error('style rewriting setter used')}};if(mode==='missing-theme')delete theme.setCustomThemeFont;if(mode==='missing-start')delete api.startAction;if(mode==='missing-finalize')delete api.finalizeAction;window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});window._sdk={options:{user:{id:1}},editor:{config:{mode:{role:'editor'}},docsApi:api,getParagraphs:()=>[{text:'target'}],getParagraphRange:p=>({getText:()=>p.text}),runSelectionCallback:async(r,f)=>{if(mode==='replaced-theme')model.getTheme=()=>({});return f()}}};(async()=>{let r,caught;try{r=await %s}catch(e){caught=e}if(mode==='throw'){if(!caught||caught.message!=='native failure'||calls.join(',')!=='start,theme,finalize')throw Error('exception retried or not finalized')}else if(mode==='success'){if(!r?.ok||calls.join(',')!=='start,theme,finalize')throw Error('not one theme transaction')}else if(r?.ok!==false||calls.length)throw Error('missing or changed capability wrote')})().catch(e=>{console.error(e);process.exitCode=1});`, mode, documentLayoutJS(spec, before))
			if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
				t.Fatalf("%v %s", err, out)
			}
		})
	}
}
