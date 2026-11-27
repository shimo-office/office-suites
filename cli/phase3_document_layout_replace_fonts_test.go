package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"
)

func TestDocumentReplaceKeepsScriptFontSlots(t *testing.T) {
	attrs := map[string]string{"fontFamily.ascii": "Arial", "fontFamily.hAnsi": "Calibri", "fontFamily.eastAsia": "仿宋", "fontFamily.cs": "Tahoma", "fontFamily.eastAsiaTheme": "minorEastAsia", "fontFamily.asciiTheme": "minorAscii", "fontFamily.hAnsiTheme": "minorHAnsi", "fontFamily.cstheme": "minorBidi"}
	raw, _ := json.Marshal(documentReplaceFormat(attrs))
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	fonts, ok := got["fonts"].(map[string]any)
	if !ok || len(fonts) != 8 {
		t.Fatalf("script fonts collapsed: %s", raw)
	}
	for _, key := range []string{"ascii", "hAnsi", "eastAsia", "cs", "asciiTheme", "hAnsiTheme", "eastAsiaTheme", "cstheme"} {
		if fonts[key] != attrs["fontFamily."+key] {
			t.Fatal("lost font slot", key)
		}
	}
}

func TestDocumentReplaceRestoresNativeFontSlots(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional existing Node runtime")
	}
	attrs := map[string]string{"fontFamily.ascii": "Arial", "fontFamily.eastAsia": "仿宋", "fontFamily.hAnsi": "Arial", "fontFamily.cs": "Arial", "fontFamily.asciiTheme": "minorAscii", "fontFamily.hAnsiTheme": "minorHAnsi", "fontFamily.eastAsiaTheme": "minorEastAsia", "fontFamily.cstheme": "minorBidi"}
	before := documentLayoutSnapshot{Paragraphs: []documentLayoutParagraph{{Text: "OLD"}}, Text: documentSnapshot{Paragraphs: []documentParagraph{{Text: "OLD", Chars: []documentChar{{Text: "OLD", Attrs: attrs}}}}}}
	spec := documentReplaceSpec{Find: "OLD", Replacement: "NEW", ExpectedCount: 1}
	steps, err := documentReplaceSteps(before, spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []bool{false, true} {
		setup := fmt.Sprintf(`global.window=global;let content='OLD',selected,writes=0,fonts;global.btoa=s=>Buffer.from(s).toString('base64');global.atob=s=>Buffer.from(s,'base64').toString();const at=n=>btoa('0|0|0|'+n),offset=s=>Number(atob(s).split('|').pop());window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});const api={isViewMode:false,setFontFamily:v=>{fonts={ascii:v,eastAsia:v}}};if(!%t)api.updateDocxSelectTextPr=p=>{fonts=p.RFonts};const e={config:{mode:{role:'editor'}},docsApi:api,getParagraphs:()=>[{text:content,children:[{getText:()=>content,children:Array.from(content)}]}],getParagraphRange:p=>({start:at(0),end:at(content.length),getText:()=>p.text}),generateRange:r=>({...r,getText:()=>content.slice(offset(r.start),offset(r.end))}),runSelectionCallback:async(r,f)=>{selected=r;return f()},pasteDataInternal:async value=>{writes++;content=content.slice(0,offset(selected.start))+value+content.slice(offset(selected.end))}};window._sdk={options:{user:{id:1}},editor:e};`, missing)
		assertion := `if(!r.ok||writes!==1||fonts?.ascii!=='Arial'||fonts?.eastAsia!=='仿宋'||fonts?.cs!=='Arial'||fonts?.hAnsi!=='Arial'||fonts?.asciiTheme!=='minorAscii'||fonts?.hAnsiTheme!=='minorHAnsi'||fonts?.eastAsiaTheme!=='minorEastAsia'||fonts?.cstheme!=='minorBidi'||Object.keys(fonts).length!==8)throw Error('font slots not restored')`
		if missing {
			assertion = `if(r.ok!==false||writes!==0)throw Error('missing font API mutated text')`
		}
		script := setup + documentReplaceJS(spec, before, steps) + `.then(r=>{` + assertion + `}).catch(e=>{console.error(e);process.exitCode=1})`
		if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
}
