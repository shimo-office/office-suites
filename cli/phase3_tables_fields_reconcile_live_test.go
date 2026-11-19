package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Fixed-fixture read-only recovery, never calls compose. Old request/exit remain
// unchanged. Explicit native field defaults are added only to a separate oracle.
func TestPhase3TableFieldsReadOnlyReconcile(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_FIELDS_RECONCILE_DIR")
	if source == "" {
		t.Skip("dedicated evidence required")
	}
	output := os.Getenv("SHIMO_PHASE3_FIELDS_RECONCILE_OUT")
	if output == "" {
		t.Fatal("separate recovery output required")
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	var before map[string]any
	var plan phase3TableEditPlan
	for path, target := range map[string]any{"before-structure.json": &before, "plan.json": &plan} {
		raw, err := os.ReadFile(filepath.Join(source, path))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	if plan.Action != "table-field-add" || plan.TableGuid != "61492df5d31" || len(plan.CreatedIDs) != 7 {
		t.Fatal("not dedicated failed field addition")
	}
	for _, id := range plan.CreatedIDs {
		field := plan.Expected["fields"].(map[string]any)[id].(map[string]any)
		props, _ := field["props"].(map[string]any)
		switch str(field, "type") {
		case "Date":
			props["isAutoFill"] = false
		case "SingleChoice":
			for _, raw := range props["choices"].(map[string]any) {
				choice := raw.(map[string]any)
				choice["hidden"] = false
				choice["locked"] = false
				choice["statusType"] = float64(0)
				choice["style"] = nil
			}
		}
	}
	if err := phase3WriteEvidence(filepath.Join(output, "explicit-default-oracle.json"), plan); err != nil {
		t.Fatal(err)
	}
	// First establish the old persisted artifact matches this narrow oracle.
	raw, err := os.ReadFile(filepath.Join(source, "after-structure.json"))
	if err != nil {
		t.Fatal(err)
	}
	var prior map[string]any
	if err = json.Unmarshal(raw, &prior); err != nil {
		t.Fatal(err)
	}
	if err = verifyPhase3TableEdit(before, prior, plan); err != nil {
		t.Fatal(err)
	}
	const target = "https://shimo.net/tables/DqdxLrxlDMcw78q4"
	app := newApp([]string{"phase3"})
	session, err := app.phase3SDK(target, "table")
	if err != nil {
		t.Fatal(err)
	}
	after, err := session.tableSnapshot(plan.TableGuid)
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyPhase3TableEdit(before, after, plan); err != nil {
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
	reopened, err := session.tableSnapshot(plan.TableGuid)
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyPhase3TableEdit(before, reopened, plan); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(output, "reopened-structure.json"), reopened); err != nil {
		t.Fatal(err)
	}
	rev, err := session.tableRevision()
	if err != nil || rev <= 9 {
		t.Fatal("revision did not advance", err)
	}
	result := map[string]any{"ok": true, "verified": true, "reconciled": true, "mutationAttempted": false, "action": plan.Action, "tableGuid": plan.TableGuid, "createdIds": plan.CreatedIDs, "afterRev": rev, "originalCommandExitCode": 1, "originalEvidenceDir": source, "evidenceDir": output, "scope": "read-only existing seven fields; exact IDs/order/records/views; separately explicit native option and date defaults"}
	if err = phase3WriteEvidence(filepath.Join(output, "result.json"), result); err != nil {
		t.Fatal(err)
	}
}
