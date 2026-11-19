package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhase3DocumentDistributionReadOnlyRecovery(t *testing.T) {
	dir := os.Getenv("SHIMO_PHASE3_DOC_DISTRIBUTION_RECOVERY_OUT")
	if dir == "" {
		t.Skip("explicit private-fixture read-only recovery")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	const original = "../tmp/phase3-live/docx-slides/acceptance-driver/runs/table-distribute-columns/20261118T192431840331Z/phase3-1286735704"
	paths, err := filepath.Glob(filepath.Join(original, "before", "*.docx"))
	if err != nil || len(paths) != 1 {
		t.Fatal("original baseline unavailable", err)
	}
	before, err := readDocumentTables(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	spec := documentTableSpec{Operation: "distribute-columns", Table: 1, Row: 1, Column: 1, ExpectedCell: "P3_A1"}
	app := newApp([]string{"phase3"})
	b, err := openPhase3EditorWhen("https://shimo.net/docx/wW0PJ2PXdrIm7mW2", documentReadyJS)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	result := map[string]any{"originalFailedRun": original, "originalBeforeArtifact": paths[0], "mutationAttempted": false, "scope": "read-only recovery of original submitted column distribution; original CLI failure preserved"}
	for _, stage := range []string{"after", "reload"} {
		if stage == "reload" {
			if _, err = b.EvalReadOnly(`(()=>{window.__shimoDistributionBeforeReload=true;return {ok:true}})()`); err != nil {
				t.Fatal(err)
			}
			if _, err = b.ws.Call(8000, "Page.reload", map[string]any{"ignoreCache": true}, 15*time.Second); err != nil {
				t.Fatal(err)
			}
			if err = b.WaitReady(`!window.__shimoDistributionBeforeReload&&(`+documentReadyJS+`)`, phase3EditorReadyTimeout); err != nil {
				t.Fatal(err)
			}
		}
		native, err := b.EvalReadOnly(`(()=>{const e=window._sdk.editor,tables=[];for(const p of e.getParagraphs()){const t=p?.getParent?.()?.getParent?.()?.Row?.Table;if(t&&!tables.includes(t))tables.push(t)}return {ok:true,tables:tables.map(t=>({rowCount:t.children.length,cellCounts:t.children.map(r=>r.children.length),layoutColumns:t.getLayoutTable?.()?.getGridCol?.()}))}})()`)
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-native.json"), native); err != nil {
			t.Fatal(err)
		}
		artifact, err := app.phase3OfficeExport("wW0PJ2PXdrIm7mW2", filepath.Join(dir, stage), "docx")
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := readDocumentTables(artifact.Path)
		if err != nil {
			t.Fatal(err)
		}
		if err = verifyDocumentTable(before, snapshot, spec); err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-readback.json"), snapshot); err != nil {
			t.Fatal(err)
		}
		if err = b.Capture(filepath.Join(dir, stage+".png")); err != nil {
			t.Fatal(err)
		}
		result[stage+"Artifact"] = artifact
	}
	result["verified"] = true
	if err = phase3WriteEvidence(filepath.Join(dir, "recovery-result.json"), result); err != nil {
		t.Fatal(err)
	}
}

func TestPhase3DocumentTableStyleReadOnlyRecovery(t *testing.T) {
	dir := os.Getenv("SHIMO_PHASE3_DOC_STYLE_RECOVERY_OUT")
	if dir == "" {
		t.Skip("explicit private-fixture read-only recovery")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	const original = "../tmp/phase3-live/docx-slides/acceptance-driver/runs/table-style/20261118T201238977793Z/phase3-605639632"
	paths, err := filepath.Glob(filepath.Join(original, "before", "*.docx"))
	if err != nil || len(paths) != 1 {
		t.Fatal("original baseline unavailable", err)
	}
	spec, err := prepareDocumentTableStyle(map[string]any{"table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "P3_A1", "fill": "#0000FF", "color": "#FFFFFF", "marginCm": 0.2, "align": "center", "border": map[string]any{"color": "#000000", "pt": float64(1)}})
	if err != nil {
		t.Fatal(err)
	}
	app := newApp([]string{"phase3"})
	b, err := openPhase3EditorWhen("https://shimo.net/docx/wvMmgVmwOXinR3vg", documentReadyJS)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	result := map[string]any{"originalFailedRun": original, "originalBeforeArtifact": paths[0], "mutationAttempted": false, "scope": "read-only recovery of original submitted table style; original CLI failure preserved"}
	for _, stage := range []string{"after", "reload"} {
		if stage == "reload" {
			if _, err = b.EvalReadOnly(`(()=>{window.__shimoStyleRecoveryBeforeReload=true;return {ok:true}})()`); err != nil {
				t.Fatal(err)
			}
			if _, err = b.ws.Call(8000, "Page.reload", map[string]any{"ignoreCache": true}, 15*time.Second); err != nil {
				t.Fatal(err)
			}
			if err = b.WaitReady(`!window.__shimoStyleRecoveryBeforeReload&&(`+documentReadyJS+`)`, phase3EditorReadyTimeout); err != nil {
				t.Fatal(err)
			}
		}
		native, err := b.EvalReadOnly(`(()=>{const e=window._sdk.editor,tables=[];for(const p of e.getParagraphs()){const t=p?.getParent?.()?.getParent?.()?.Row?.Table;if(t&&!tables.includes(t))tables.push(t)}return {ok:true,tables:tables.map(t=>({rowCount:t.children.length,cellCounts:t.children.map(r=>r.children.length),layoutColumns:t.getLayoutTable?.()?.getGridCol?.()}))}})()`)
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-native.json"), native); err != nil {
			t.Fatal(err)
		}
		artifact, err := app.phase3OfficeExport("wvMmgVmwOXinR3vg", filepath.Join(dir, stage), "docx")
		if err != nil {
			t.Fatal(err)
		}
		if err = verifyDocumentTableStyle(paths[0], artifact.Path, spec); err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-assertions.json"), map[string]any{"verified": true, "spec": spec, "originalBeforeArtifact": paths[0], "artifact": artifact}); err != nil {
			t.Fatal(err)
		}
		if err = b.Capture(filepath.Join(dir, stage+".png")); err != nil {
			t.Fatal(err)
		}
		result[stage+"Artifact"] = artifact
	}
	result["verified"] = true
	if err = phase3WriteEvidence(filepath.Join(dir, "recovery-result.json"), result); err != nil {
		t.Fatal(err)
	}
}

func TestPhase3DocumentColumnInsertReadOnlyRecovery(t *testing.T) {
	dir := os.Getenv("SHIMO_PHASE3_DOC_COLUMN_RECOVERY_OUT")
	if dir == "" {
		t.Skip("explicit private-fixture read-only recovery")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	const original = "../tmp/phase3-live/docx-slides/acceptance-driver/runs/table-column-before/20261118T212740192935Z/phase3-4233741252"
	paths, err := filepath.Glob(filepath.Join(original, "before", "*.docx"))
	if err != nil || len(paths) != 1 {
		t.Fatal("original baseline unavailable", err)
	}
	before, err := readDocumentTables(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	spec := documentTableSpec{Operation: "insert-column-before", Table: 1, Row: 1, Column: 1, ExpectedCell: "P3_A1"}
	app := newApp([]string{"phase3"})
	b, err := openPhase3EditorWhen("https://shimo.net/docx/wvMmgVmwOXinR3vg", documentReadyJS)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	result := map[string]any{"originalFailedRun": original, "originalBeforeArtifact": paths[0], "mutationAttempted": false, "scope": "read-only recovery of original submitted column insertion; original CLI failure preserved"}
	for _, stage := range []string{"after", "reload"} {
		if stage == "reload" {
			if _, err = b.EvalReadOnly(`(()=>{window.__shimoColumnRecoveryBeforeReload=true;return {ok:true}})()`); err != nil {
				t.Fatal(err)
			}
			if _, err = b.ws.Call(8000, "Page.reload", map[string]any{"ignoreCache": true}, 15*time.Second); err != nil {
				t.Fatal(err)
			}
			if err = b.WaitReady(`!window.__shimoColumnRecoveryBeforeReload&&(`+documentReadyJS+`)`, phase3EditorReadyTimeout); err != nil {
				t.Fatal(err)
			}
		}
		native, err := b.EvalReadOnly(`(()=>{const e=window._sdk.editor,tables=[];for(const p of e.getParagraphs()){const t=p?.getParent?.()?.getParent?.()?.Row?.Table;if(t&&!tables.includes(t))tables.push(t)}return {ok:true,tables:tables.map(t=>({rowCount:t.children.length,cellCounts:t.children.map(r=>r.children.length),layoutColumns:t.getLayoutTable?.()?.getGridCol?.()}))}})()`)
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-native.json"), native); err != nil {
			t.Fatal(err)
		}
		artifact, err := app.phase3OfficeExport("wvMmgVmwOXinR3vg", filepath.Join(dir, stage), "docx")
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := readDocumentTables(artifact.Path)
		if err != nil {
			t.Fatal(err)
		}
		if err = verifyDocumentTable(before, snapshot, spec); err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-readback.json"), snapshot); err != nil {
			t.Fatal(err)
		}
		if err = b.Capture(filepath.Join(dir, stage+".png")); err != nil {
			t.Fatal(err)
		}
		result[stage+"Artifact"] = artifact
	}
	result["verified"] = true
	if err = phase3WriteEvidence(filepath.Join(dir, "recovery-result.json"), result); err != nil {
		t.Fatal(err)
	}
}

func TestPhase3DocumentCompactMergeReadOnlyRecovery(t *testing.T) {
	dir := os.Getenv("SHIMO_PHASE3_DOC_COMPACT_MERGE_RECOVERY_OUT")
	if dir == "" {
		t.Skip("explicit private-fixture read-only recovery")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	const original = "../tmp/phase3-live/docx-slides/acceptance-driver/runs/table-compact-merge/20261118T234417038189Z/phase3-1038282498"
	paths, err := filepath.Glob(filepath.Join(original, "before", "*.docx"))
	if err != nil || len(paths) != 1 {
		t.Fatal("original baseline unavailable", err)
	}
	before, err := readDocumentTables(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	spec := documentTableSpec{Operation: "merge", Table: 1, Row: 1, Column: 1, EndColumn: 2, ExpectedCell: "A1"}
	app := newApp([]string{"phase3"})
	b, err := openPhase3EditorWhen("https://shimo.net/docx/j1Og3Vgwl4C9xz1z", documentReadyJS)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	result := map[string]any{"originalFailedRun": original, "originalBeforeArtifact": paths[0], "mutationAttempted": false, "scope": "read-only recovery of original submitted compact merge; original CLI failure preserved"}
	for _, stage := range []string{"after", "reload"} {
		if stage == "reload" {
			if _, err = b.EvalReadOnly(`(()=>{window.__shimoCompactMergeRecoveryBeforeReload=true;return {ok:true}})()`); err != nil {
				t.Fatal(err)
			}
			if _, err = b.ws.Call(8000, "Page.reload", map[string]any{"ignoreCache": true}, 15*time.Second); err != nil {
				t.Fatal(err)
			}
			if err = b.WaitReady(`!window.__shimoCompactMergeRecoveryBeforeReload&&(`+documentReadyJS+`)`, phase3EditorReadyTimeout); err != nil {
				t.Fatal(err)
			}
		}
		native, err := b.EvalReadOnly(`(()=>{const e=window._sdk.editor,tables=[];for(const p of e.getParagraphs()){const t=p?.getParent?.()?.getParent?.()?.Row?.Table;if(t&&!tables.includes(t))tables.push(t)}return {ok:true,tables:tables.map(t=>({rowCount:t.children.length,cellCounts:t.children.map(r=>r.children.length),layoutColumns:t.getLayoutTable?.()?.getGridCol?.()}))}})()`)
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-native.json"), native); err != nil {
			t.Fatal(err)
		}
		artifact, err := app.phase3OfficeExport("j1Og3Vgwl4C9xz1z", filepath.Join(dir, stage), "docx")
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := readDocumentTables(artifact.Path)
		if err != nil {
			t.Fatal(err)
		}
		if err = verifyDocumentTable(before, snapshot, spec); err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-readback.json"), snapshot); err != nil {
			t.Fatal(err)
		}
		if err = b.Capture(filepath.Join(dir, stage+".png")); err != nil {
			t.Fatal(err)
		}
		result[stage+"Artifact"] = artifact
	}
	result["verified"] = true
	if err = phase3WriteEvidence(filepath.Join(dir, "recovery-result.json"), result); err != nil {
		t.Fatal(err)
	}
}
