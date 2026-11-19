package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPhase3DocumentLayoutLive(t *testing.T) {
	mode := os.Getenv("SHIMO_PHASE3_LAYOUT_LIVE")
	if mode == "" {
		t.Skip("opt-in dedicated private layout fixture only")
	}
	target := "https://shimo.net/docx/z18oQ2oXkZSErPW7"
	dir := "../tmp/phase3-live/document-layout"
	if mode == "seed" {
		b, e := openPhase3EditorWhen(target, documentLayoutReadyJS)
		if e != nil {
			t.Fatal(e)
		}
		defer b.Close()
		if e = b.CheckPresence(); e != nil {
			t.Fatal(e)
		}
		lines := []string{"PHASE3_LAYOUT_20261118 标题", "正文旧版测试：用于核对首行缩进、行距和段落底纹。旧版测试。", "编号测试甲", "编号测试乙", "保持原样的正文。"}
		for i := 1; i <= 65; i++ {
			lines = append(lines, fmt.Sprintf("布局测试第%02d段：用于构造多页文稿，核对页面定位和页边距，所有内容均为专用验收合成资料。", i))
		}
		lines = append(lines, "末页旧版测试结束。")
		expected := strings.Join(lines, "\n")
		payload, _ := json.Marshal(expected)
		v, e := b.Eval(`(async()=>{` + phase3PresenceGuardJS() + `const e=window._sdk.editor;if(e.config?.mode?.role!=='editor'||e.docsApi.isViewMode!==false||e.canHandleChanges===false)return {ok:false,error:'没有编辑权限'};const r=e.generateRange(e.getWholeRange());if((await r.getText()).trim()!=='')return {ok:false,error:'fixture并非空白，不覆盖'};return await e.runSelectionCallback(r,async()=>{` + phase3PresenceGuardJS() + `if(e.config?.mode?.role!=='editor'||e.docsApi.isViewMode!==false||e.canHandleChanges===false)return {ok:false,error:'编辑权限变化'};await e.pasteDataInternal(` + string(payload) + `,'text');return {ok:true,seeded:true}})})()`)
		if e != nil {
			t.Fatal(e)
		}
		t.Log(v)
		if e = b.waitDocumentSaved(); e != nil {
			t.Fatal(e)
		}
		a := newApp(nil)
		var actual documentLayoutSnapshot
		for i := 0; i < 5; i++ {
			artifact, e := a.phase3OfficeExport(guidFromURL(target), filepath.Join(dir, "seed", fmt.Sprint(i)), "docx")
			if e != nil {
				t.Fatal(e)
			}
			actual, e = readDocumentLayout(artifact.Path)
			if e != nil {
				t.Fatal(e)
			}
			if len(actual.Paragraphs) == len(lines) {
				break
			}
			time.Sleep(time.Second)
		}
		if len(actual.Paragraphs) != len(lines) {
			t.Fatalf("seed not persisted: %d", len(actual.Paragraphs))
		}
		for i, line := range lines {
			if actual.Paragraphs[i].Text != line {
				t.Fatalf("seed paragraph %d mismatch", i+1)
			}
		}
		if e = documentLayoutCapture(b, filepath.Join(dir, "seed.png")); e != nil {
			t.Fatal(e)
		}
		v, e = b.Eval(`(()=>({pages:window._sdk.editor.docsApi.getPageCount(),theme:window._sdk.editor.docsApi.getCurThemeFonts()}))()`)
		if e != nil {
			t.Fatal(e)
		}
		x, _ := json.Marshal(v)
		t.Log(string(x))
		return
	}
	action := "document-paragraph"
	var p map[string]any
	switch mode {
	case "heading2":
		p = map[string]any{"paragraph": 1, "text": "PHASE3_LAYOUT_20261118 标题", "attrs": map[string]any{"heading": 2}}
	case "heading":
		p = map[string]any{"paragraph": 1, "text": "PHASE3_LAYOUT_20261118 标题", "attrs": map[string]any{"heading": 1}}
	case "paragraph":
		p = map[string]any{"paragraph": 2, "text": "正文旧版测试：用于核对首行缩进、行距和段落底纹。旧版测试。", "attrs": map[string]any{"firstLineChars": 2, "lineMultiple": 1.5, "shading": "#EEEEEE"}}
	case "number1":
		p = map[string]any{"paragraph": 3, "text": "编号测试甲", "attrs": map[string]any{"numbering": "decimal"}}
	case "number2":
		p = map[string]any{"paragraph": 4, "text": "编号测试乙", "attrs": map[string]any{"numbering": "decimal"}}
	case "goto":
		action = "document-goto"
		p = map[string]any{"page": 2}
	case "replace":
		action = "document-replace"
		p = map[string]any{"find": "旧版测试", "replacement": "新版测试", "expectedCount": 3}
	case "theme":
		action = "document-theme"
		p = map[string]any{"themeFonts": map[string]any{"major": map[string]any{"latin": "Arial", "ea": "黑体", "cs": "Arial"}, "minor": map[string]any{"latin": "Arial", "ea": "宋体", "cs": "Arial"}}, "resetSupplemental": true}
	case "pagebaseline":
		action = "document-page"
		p = map[string]any{"columns": 1}
	case "page":
		action = "document-page"
		p = map[string]any{"marginsCm": map[string]any{"left": 1.5, "right": 1.5, "top": 2, "bottom": 2}, "columns": 2, "background": "#EAF3FF"}
	default:
		t.Fatal("unknown fixed live case")
	}
	data, _ := json.Marshal(p)
	args := []string{"phase3", target, "--action", action, "--payload", string(data), "--out", filepath.Join(dir, mode), "--json"}
	t.Log("CLI args", args)
	a := newApp(args)
	var err error
	switch action {
	case "document-goto":
		err = a.cmdPhase3DocumentGoto()
	case "document-replace":
		err = a.cmdPhase3DocumentReplace()
	default:
		err = a.cmdPhase3DocumentLayout()
	}
	if err != nil {
		t.Fatal(err)
	}
	if mode == "number2" {
		artifact, e := a.phase3OfficeExport(guidFromURL(target), filepath.Join(dir, "number-sequence"), "docx")
		if e != nil {
			t.Fatal(e)
		}
		snapshot, e := readDocumentLayout(artifact.Path)
		if e != nil {
			t.Fatal(e)
		}
		first, second := snapshot.Paragraphs[2], snapshot.Paragraphs[3]
		if first.NumberID == "" || first.NumberID != second.NumberID || first.NumberLevel != "0" || second.NumberLevel != "0" || first.NumberStart != "1" {
			t.Fatalf("numbering sequence not proven: %+v %+v", first, second)
		}
		if e = phase3WriteEvidence(filepath.Join(dir, "number-sequence/result.json"), map[string]any{"verified": true, "values": []int{1, 2}, "first": first, "second": second, "artifact": artifact}); e != nil {
			t.Fatal(e)
		}
	}
}
