package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhase3DocumentLiveFormat(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_DOCUMENT_LIVE") != "1" {
		t.Skip("opt-in dedicated phase3 test document only")
	}
	target := "https://shimo.net/docx/4WDzngzwAnCJYzqm"
	b, err := openPhase3Editor(target)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for i := 0; i < 20; i++ {
		err = b.CheckPresence()
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		t.Fatal(err)
	}
	logValue := func(v any) { x, _ := json.Marshal(v); t.Log(string(x)) }
	v, err := b.Eval(`(async()=>{` + phase3PresenceGuardJS() + `const e=window._sdk.editor;const ps=e.getParagraphs();const text=await e.generateRange(e.getWholeRange()).getText();if(text.trim()===""){await e.generateRange(e.getWholeRange()).setText("PHASE3_DOC_FORMAT_20261118 x2 H2O\nUntouched paragraph");return {ok:true,seeded:true}}if(!text.includes("PHASE3_DOC_FORMAT_20261118"))return {ok:false,error:"专用fixture内容不符，拒绝覆盖"};return {ok:true,seeded:false};})()`)
	if err != nil {
		t.Fatal(err)
	}
	logValue(v)
	time.Sleep(3 * time.Second)
	dir := "../tmp/phase3-live/docx-slides/document-format"
	a := newApp(nil)
	before, err := a.phase3OfficeExport(guidFromURL(target), filepath.Join(dir, "before"), "docx")
	if err != nil {
		t.Fatal(err)
	}
	logValue(before)
	v, err = b.Eval(`(async()=>{` + phase3PresenceGuardJS() + `const e=window._sdk.editor,p=e.getParagraphs()[0],r=e.getParagraphRange(p);const txt=(await r.getText()).replace(/\r?\n$/,'');if(txt!=="PHASE3_DOC_FORMAT_20261118 x2 H2O")return {ok:false,error:"目标段落不符: "+txt};return await e.runSelectionCallback(r,()=>{e.docsApi.setFontFamily("宋体");e.docsApi.setFontSize(14);e.docsApi.setTextColor({r:255,g:0,b:0});return {ok:true,text:txt,range:{start:atob(r.start),end:atob(r.end)},rangeSource:String(e.docsApi.getParagraphRange),underlineSource:String(e.docsApi.setUnderline),verticalSource:String(e.docsApi.setTextVertAlign)}});})()`)
	if err != nil {
		t.Fatal(err)
	}
	logValue(v)
	time.Sleep(4 * time.Second)
	after, err := a.phase3OfficeExport(guidFromURL(target), filepath.Join(dir, "after"), "docx")
	if err != nil {
		t.Fatal(err)
	}
	logValue(after)
	bs, err := readDocumentSnapshot(before.Path)
	if err != nil {
		t.Fatal(err)
	}
	as, err := readDocumentSnapshot(after.Path)
	if err != nil {
		t.Fatal(err)
	}
	logValue(as)
	spec, err := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": "PHASE3_DOC_FORMAT_20261118 x2 H2O", "attrs": map[string]any{"fontFamily": "宋体", "fontSize": float64(14), "color": "#FF0000"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyDocumentFormat(bs, as, spec); err != nil {
		t.Fatal(err)
	}
}
