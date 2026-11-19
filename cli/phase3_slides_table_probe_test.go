package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// This opt-in diagnostic changes only selection. No merge, split, text or style
// writer is called; setTableRangeStyle with an empty object selects the range.
func TestPhase3SlidesInspectTableSelectionReadOnly(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_SLIDES_INSPECT_TABLE")
	if source == "" {
		t.Skip("opt-in selection inspection of a recorded failed table action")
	}
	var request struct {
		Spec phase3SlidesSpec
		URL  string
	}
	raw, err := os.ReadFile(filepath.Join(source, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &request); err != nil || request.Spec.Table == nil {
		t.Fatal("invalid recorded table request", err)
	}
	b, err := openPhase3SlidesForegroundProbe(request.URL+"?dbg=1", "table-selection")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	dir := filepath.Join(phase3SlidesProbeRunRoot, "table-selection")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	spec, _ := json.Marshal(request.Spec)
	state, err := b.Eval(`(async()=>{const r=` + string(spec) + `,sdk=window.presentationSDK,wb=sdk.editor.workbench,c=wb.core,e=wb.editor,p=e.presentation,api=sdk.API.slides;api.setCurrentSlideIndex(r.slideId);const slide=api.getSlides().find(s=>s.id===r.slideId),node=c.getSlideElements(e,slide.getIndex()).find(n=>n.type==='Table'&&n.name===r.table.tableName);if(!node)return {ok:false,error:'missing table'};const out={ok:true,tableId:node.id,methods:Object.fromEntries(['setTableRangeStyle','setTableRangeSpan','canMergeTableCells','mergeCells','canSplitTableCells'].map(k=>[k,typeof c[k]])),stages:[]};const capture=label=>{const s=p._$selectionState$_,t=s._$selectedTable$_,text=s._$selectedTextSp$_;out.stages.push({label,selectedTable:!!t,textShape:text?.id??null,selectedElements:s._$selectionStatusInfo$_?._$selectedElements$_?.map(x=>({id:x.id,type:x._$instanceType$_})),selection:t?{isUse:t.selection._$isUse$_,type:t.selection.type,positions:t.selection._$cellPositions$_,start:t.selection._$selectionStartInfo$_,end:t.selection._$selectionEndInfo$_}:null,canMerge:c.canMergeTableCells(e),canSplit:c.canSplitTableCells(e)});};capture('initial');c.selectElementByIds(e,[node.id]);capture('select-by-id');c.setTableRangeStyle(e,{row:0,column:0,rowCount:1,columnCount:2},{});capture('range-after-select');c.setTextRange(e,null);c.selectElementByIds(e,[node.id]);capture('reset-select');c.setTableRangeStyle(e,{row:0,column:0,rowCount:1,columnCount:2},{});capture('range-after-reset');c.setTableRangeStyle(e,{row:0,column:0,rowCount:1,columnCount:2},{});capture('range-with-text-focus');return out;})()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "selection-state.json"), state); err != nil {
		t.Fatal(err)
	}
	a := &App{Origin: "https://shimo.net", Cmd: "phase3", JSON: true}
	fresh, err := a.phase3OfficeExport(guidFromURL(request.URL), filepath.Join(dir, "fresh"), "pptx")
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3SlideScreenshot(b, filepath.Join(dir, "reload.png")); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "inspection.json"), map[string]any{"originalFailedCommandEvidence": source, "freshArtifact": fresh, "mutationAttempted": false, "status": "selection-inspection-only"}); err != nil {
		t.Fatal(err)
	}
}
