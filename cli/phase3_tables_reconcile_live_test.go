package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Read-only recovery for this task's dedicated fixture. Never reissues the
// failed duplicate POST; retains its original failure and verifies current data.
func TestPhase3TableDuplicateReadOnlyReconcile(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_TABLE_RECONCILE_DIR")
	if source == "" {
		t.Skip("explicit dedicated fixture evidence directory required")
	}
	output := os.Getenv("SHIMO_PHASE3_TABLE_RECONCILE_OUT")
	if output == "" {
		t.Fatal("separate recovery output directory required")
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	var before map[string]map[string]any
	var plan phase3BaseEditPlan
	for path, target := range map[string]any{"before-structure.json": &before, "plan.json": &plan} {
		raw, err := os.ReadFile(filepath.Join(source, path))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	if plan.Action != "table-duplicate" || plan.SourceGuid != "61492df5d31" || plan.TableGuid != "b032efb3ccd" {
		t.Fatal("not the independently observed dedicated duplicate fixture")
	}
	const target = "https://shimo.net/tables/DqdxLrxlDMcw78q4"
	app := newApp([]string{"phase3"})
	session, err := app.phase3SDK(target, "table")
	if err != nil {
		t.Fatal(err)
	}
	after, err := session.baseSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyPhase3BaseEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(output, "after-structure.json"), after); err != nil {
		t.Fatal(err)
	}
	guard, err := phase3OpenTableGuard(target)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	native, ok := guard.(*phase3TablePresence)
	if !ok {
		t.Fatal("native browser required")
	}
	if err = native.NavigateTable(plan.TableGuid); err != nil {
		t.Fatal(err)
	}
	if err = native.Capture(filepath.Join(output, "after.png")); err != nil {
		t.Fatal(err)
	}
	if err = native.FreshCapture(filepath.Join(output, "reload.png")); err != nil {
		t.Fatal(err)
	}
	reopened, err := session.baseSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyPhase3BaseEdit(before, reopened, plan); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(output, "reopened-structure.json"), reopened); err != nil {
		t.Fatal(err)
	}
	rev, err := session.tableRevision()
	if err != nil {
		t.Fatal(err)
	}
	if rev <= 6 {
		t.Fatal("fixture revision did not advance")
	}
	result := map[string]any{"ok": true, "verified": true, "reconciled": true, "mutationAttempted": false, "action": "table-duplicate", "tableGuid": plan.TableGuid, "sourceGuid": plan.SourceGuid, "afterRev": rev, "originalCommandExitCode": 1, "originalEvidenceDir": source, "evidenceDir": output, "scope": "read-only reconciliation of already-created dedicated copy; original failed command preserved"}
	if err = phase3WriteEvidence(filepath.Join(output, "result.json"), result); err != nil {
		t.Fatal(err)
	}
}
