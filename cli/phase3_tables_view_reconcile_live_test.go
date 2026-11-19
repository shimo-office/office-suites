package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// Fixed fixture recovery: no compose call, no recreate. The original failed
// command remains separate; lazy view details come from the native GET endpoint.
func TestPhase3TableGridReadOnlyReconcile(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_GRID_RECONCILE_DIR")
	if source == "" {
		t.Skip("dedicated original evidence required")
	}
	output := os.Getenv("SHIMO_PHASE3_GRID_RECONCILE_OUT")
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
	if plan.TableGuid != "61492df5d31" {
		t.Fatal("not dedicated grid table")
	}
	var view map[string]any
	oracleScope := "original exact plan state"
	if plan.Action == "table-view-create" && len(plan.NewViews) == 1 && plan.NewViews["2acdb73f09c"] != nil {
		view = plan.NewViews["2acdb73f09c"]
	} else if plan.Action == "table-view-create" && filepath.Base(source) == "phase3-3483176541" && len(plan.NewViews) == 1 && plan.NewViews["9070e01b967"] != nil {
		view = plan.NewViews["9070e01b967"]
		// This original single CreateView never requested sort. Preserve its real
		// native Kanban default in this separate recovery oracle, not future plans.
		view["sort"] = map[string]any{"rules": []any{}, "keepSorted": false}
		if len(plan.Changes) != 1 || plan.Changes[0].Action != "CreateView" {
			t.Fatal("not original single CreateView")
		}
		var create map[string]any
		if err := json.Unmarshal([]byte(plan.Changes[0].Data), &create); err != nil {
			t.Fatal(err)
		}
		if str(create, "targetGuid") != "69d0afc1d15" || str(create, "viewGuid") != "9070e01b967" {
			t.Fatal("not original fixed Kanban anchor")
		}
		orders, newOrder, err := phase3InsertedViewOrders(before, str(create, "targetGuid"))
		if err != nil {
			t.Fatal(err)
		}
		for id, order := range orders {
			plan.Expected["views"].(map[string]any)[id].(map[string]any)["order"] = order
		}
		view["order"] = newOrder
		if err := phase3WriteEvidence(filepath.Join(output, "anchor-order-oracle.json"), map[string]any{"source": "original before snapshot and original CreateView targetGuid; native insert-after-anchor", "anchorGuid": create["targetGuid"], "existingViewOrdersAfter": orders, "newViewOrder": newOrder}); err != nil {
			t.Fatal(err)
		}
		oracleScope = "native-default-oracle.json: fixed original Kanban request with unrequested sort default keepSorted=false and insertion orders derived only from original baseline/anchor (anchor-order-oracle.json); all other state remains exact"
		if err := phase3WriteEvidence(filepath.Join(output, "native-default-oracle.json"), plan); err != nil {
			t.Fatal(err)
		}
	} else if plan.Action == "table-view-create" && filepath.Base(source) == "phase3-1038237984" && len(plan.NewViews) == 1 && plan.NewViews["9382f87cb0f"] != nil {
		view = plan.NewViews["9382f87cb0f"]
		color := view["calendar"].(map[string]any)["color"].(map[string]any)
		if str(view, "type") != "Calendar" || str(color, "type") != "custom" || color["followFieldGuid"] != nil {
			t.Fatal("not original Calendar custom color with no follow field")
		}
		color["followFieldGuid"] = ""
		oracleScope = "native-calendar-default-oracle.json: fixed original Calendar custom color with no follow field persisted as empty string; all other state remains exact"
		if err := phase3WriteEvidence(filepath.Join(output, "native-calendar-default-oracle.json"), plan); err != nil {
			t.Fatal(err)
		}
	} else if plan.Action == "table-view-update" && filepath.Base(source) == "phase3-601737983" {
		view = plan.Expected["views"].(map[string]any)["2acdb73f09c"].(map[string]any)
		view["filters"].(map[string]any)["includeSubRecord"] = false
		for _, raw := range view["groups"].([]any) {
			raw.(map[string]any)["groupBy"] = nil
		}
		oracleScope = "explicit-default-oracle.json: fixed original filter/group request with native includeSubRecord=false and groupBy=null; all other state remains exact"
		if err := phase3WriteEvidence(filepath.Join(output, "explicit-default-oracle.json"), plan); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("not dedicated grid create/filter evidence")
	}
	const target = "https://shimo.net/tables/DqdxLrxlDMcw78q4"
	app := newApp([]string{"phase3"})
	session, err := app.phase3SDK(target, "table")
	if err != nil {
		t.Fatal(err)
	}
	timezone := ""
	if str(view, "type") == "Calendar" {
		raw, err := session.get("table-api/v2/bases/" + url.PathEscape(session.guid))
		if err != nil {
			t.Fatal(err)
		}
		base, ok := raw.(map[string]any)
		if !ok || str(base, "timezone") == "" {
			t.Fatal("document timezone missing")
		}
		timezone = str(base, "timezone")
	}
	after, err := session.tableSnapshot(plan.TableGuid)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(output, "after-structure.json"), after); err != nil {
		t.Fatal(err)
	}
	if err = verifyPhase3TableEdit(before, after, plan); err != nil {
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
	browserTimezone := ""
	if str(view, "type") == "Calendar" {
		browserTimezone, err = native.BrowserTimezone()
		if err != nil {
			t.Fatal(err)
		}
	}
	spec, err := phase3ViewRecordSpec(before, view, timezone, browserTimezone)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(output, "expected-rendered-view.json"), spec); err != nil {
		t.Fatal(err)
	}
	proof, err := native.ViewProof(spec)
	if err != nil {
		if str(view, "type") == "Calendar" {
			specJSON, _ := json.Marshal(spec)
			diagnostic, diagnosticErr := native.browser.EvalReadOnly(`(` + phase3ReadNativeView + `)(` + string(specJSON) + `,".calendar-task-item[data-task-guid] .task-name")`)
			if diagnosticErr == nil {
				_ = phase3WriteEvidence(filepath.Join(output, "failed-native-view.json"), diagnostic)
			}
			_ = native.browser.Capture(filepath.Join(output, "failed-native-view.png"))
		}
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(output, "rendered-view.json"), proof); err != nil {
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
	proof, err = native.ViewProof(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(output, "reopened-view.json"), proof); err != nil {
		t.Fatal(err)
	}
	if err = native.Capture(filepath.Join(output, "reload.png")); err != nil {
		t.Fatal(err)
	}
	rev, err := session.tableRevision()
	if err != nil || rev <= 18 {
		t.Fatal("revision did not advance", err)
	}
	result := map[string]any{"ok": true, "verified": true, "reconciled": true, "mutationAttempted": false, "action": plan.Action, "tableGuid": plan.TableGuid, "createdIds": plan.CreatedIDs, "afterRev": rev, "originalCommandExitCode": 1, "originalEvidenceDir": source, "evidenceDir": output, "scope": "read-only existing view; same-revision hydrated actual views; " + oracleScope + "; native rendered named records after independent fresh profile"}
	if err = phase3WriteEvidence(filepath.Join(output, "result.json"), result); err != nil {
		t.Fatal(err)
	}
}
