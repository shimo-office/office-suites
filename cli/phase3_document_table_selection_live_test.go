package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Explicitly gated, fixed-fixture selection-only diagnosis. No content methods.
func TestPhase3DocumentTableSelectionReadOnlyProbe(t *testing.T) {
	output := os.Getenv("SHIMO_PHASE3_DOC_TABLE_SELECTION_OUT")
	if output == "" {
		t.Skip("explicit dedicated selection-probe output required")
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	const target = "https://shimo.net/docx/wvMmgVmwOXinR3vg"
	app := newApp([]string{"phase3"})
	beforeArtifact, err := app.phase3OfficeExport(guidFromURL(target), filepath.Join(output, "before"), "docx")
	if err != nil {
		t.Fatal(err)
	}
	before, err := readDocumentTables(beforeArtifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	spec := documentTableSpec{Operation: "format", Table: 1, Row: 1, Column: 1, ExpectedCell: "P3_A1"}
	if len(before.Tables) == 0 {
		t.Fatal("fixture table missing")
	}
	if _, err = documentTableAnchor(before, spec); err != nil {
		t.Fatal(err)
	}
	b, err := openPhase3EditorWhen(target, documentReadyJS)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	permissions, err := b.Eval(`(()=>{const e=window._sdk?.editor;return {ok:true,isViewMode:e?.docsApi?.isViewMode,role:e?.config?.mode?.role,canHandleChanges:e?.canHandleChanges}})()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(output, "permissions-before.json"), permissions); err != nil {
		t.Fatal(err)
	}
	if err = b.WaitReady(`window._sdk?.editor?.docsApi?.isViewMode===false&&window._sdk?.editor?.config?.mode?.role==='editor'&&window._sdk?.editor?.canHandleChanges!==false`, phase3EditorReadyTimeout); err != nil {
		t.Fatal(err)
	}
	body := `const stages=[];const snapshot=name=>{const chosen=a.getSelectedTableCells?.();stages.push({name,sameTable:a.getCurTable?.()===table,curCell:table.curCell?{row:table.curCell.Row?.Index,column:table.curCell.Index}:null,targetCellIsCurrent:table.curCell===cell,selection:{use:table.Selection?.Use,textMode:table.Selection?.isTextMode,data:table.Selection?.Data},selected:chosen?.data})};snapshot('initial-range');a.selectTable();snapshot('after-selectTable');a.selectCell();snapshot('after-selectCell');return e.runSelectionCallback(range,()=>{snapshot('fresh-range');a.selectCell();snapshot('fresh-selectCell');return {ok:true,mutationAttempted:false,stages}});`
	result, err := b.Eval(documentTableOperationJS(spec, before, "selectCell", "", body))
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(output, "selection-probe.json"), result); err != nil {
		t.Fatal(err)
	}
	if err = b.Capture(filepath.Join(output, "selection.png")); err != nil {
		t.Fatal(err)
	}
	afterArtifact, err := app.phase3OfficeExport(guidFromURL(target), filepath.Join(output, "after"), "docx")
	if err != nil {
		t.Fatal(err)
	}
	after, err := readDocumentTables(afterArtifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("selection probe changed exported content")
	}
	raw, _ := json.Marshal(map[string]any{"ok": true, "verified": true, "mutationAttempted": false, "scope": "selection-only probe; before/after exported document tables, outside content and resources identical", "beforeArtifact": beforeArtifact, "afterArtifact": afterArtifact})
	if err = os.WriteFile(filepath.Join(output, "result.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
