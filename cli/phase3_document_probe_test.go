package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPhase3EditorReadOnlyProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_EDITOR_PROBE") != "1" {
		t.Skip("opt-in live read-only inspection")
	}
	for _, target := range []string{"https://shimo.net/docx/4WDzngzwAnCJYzqm"} {
		b, e := openPhase3EditorWhen(target, `document.readyState==="complete"`)
		if e != nil {
			t.Fatal(e)
		}
		v, e := b.Eval(`(async()=>{await new Promise(r=>setTimeout(r,15000));if(!window._sdk?.editor?.docsApi)return {url:location.pathname,title:document.title,body:(document.body?.innerText||"").slice(0,400),sdk:typeof window._sdk,scriptCount:document.scripts.length};const e=window._sdk.editor,p=e.getParagraphs()[0],r=e.getParagraphRange(p);const keys=o=>Object.getOwnPropertyNames(Object.getPrototypeOf(o));return {range:{start:atob(r.start),end:atob(r.end)},text:r.getText(),paraIndex:p.index??p.Index,children:p.children.map((c,i)=>({i,instanceType:c.instanceType,type:c.type,text:typeof c.getText==='function'?c.getText():typeof c.GetText==='function'?c.GetText():null,length:c.children?.length,chars:c.children?.slice(0,3).map(g=>({keys:Object.keys(g),type:g.type,value:g.value,text:g.text})),methods:keys(c).filter(k=>/text|pos/i.test(k))}))};})()`)
		if e == nil {
			t.Log("presence guard:", b.CheckPresence())
		}
		b.Close()
		if e != nil {
			t.Fatal(e)
		}
		out, _ := json.MarshalIndent(v, "", "  ")
		t.Log(string(out))
	}
}
