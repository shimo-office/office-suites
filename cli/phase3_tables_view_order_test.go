package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPhase3ViewCreatePreservesAnchorAndPredictsOrders(t *testing.T) {
	before := tableFixtureForEdit()
	first := phase3TableMap(before, "views")["v1"]
	first["order"] = float64(0)
	second := phase3TableCopy(first)
	second["guid"] = "v2"
	second["order"] = float64(1)
	third := phase3TableCopy(first)
	third["guid"] = "v3"
	third["order"] = float64(2)
	before["views"] = []any{third, first, second}
	original := phase3TableCopy(before)
	plan, err := preparePhase3TableEdit("table-view-create", map[string]any{"tableGuid": "t1", "viewGuid": "v1", "name": "new grid", "type": "Grid"}, before)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	json.Unmarshal([]byte(plan.Changes[0].Data), &wire)
	if wire["targetGuid"] != "v1" || wire["fromViewGuid"] != "v1" {
		t.Fatal("requested anchor changed")
	}
	expected := plan.Expected["views"].(map[string]any)
	for id, want := range map[string]float64{"v1": 0, "v2": 2, "v3": 3} {
		if expected[id].(map[string]any)["order"] != want {
			t.Fatal("old order not predicted", id, expected)
		}
	}
	id := plan.CreatedIDs[0]
	if plan.NewViews[id]["order"] != float64(1) {
		t.Fatal("new order not predicted")
	}
	if !reflect.DeepEqual(before, original) {
		t.Fatal("baseline mutated")
	}
	after := phase3TableCopy(before)
	for _, v := range phase3TableMap(after, "views") {
		v["order"] = expected[str(v, "guid")].(map[string]any)["order"]
	}
	newView := phase3TableCopy(plan.NewViews[id])
	newView["fieldInfo"] = map[string]any{"f1": map[string]any{"visible": true}, "f2": map[string]any{"visible": true}}
	after["views"] = append(after["views"].([]any), newView)
	if err := verifyPhase3TableEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
	for _, badID := range []string{"v1", "v2", "v3", id} {
		bad := phase3TableCopy(after)
		phase3TableMap(bad, "views")[badID]["order"] = float64(9)
		if verifyPhase3TableEdit(before, bad, plan) == nil {
			t.Fatal("wrong order accepted", badID)
		}
	}
}

func TestPhase3ViewCreateRejectsAmbiguousOrders(t *testing.T) {
	for _, mode := range []string{"missing", "duplicate", "negative", "fraction", "string"} {
		t.Run(mode, func(t *testing.T) {
			before := tableFixtureForEdit()
			first := phase3TableMap(before, "views")["v1"]
			first["order"] = float64(0)
			second := phase3TableCopy(first)
			second["guid"] = "v2"
			second["order"] = float64(1)
			switch mode {
			case "missing":
				delete(second, "order")
			case "duplicate":
				second["order"] = float64(0)
			case "negative":
				second["order"] = float64(-1)
			case "fraction":
				second["order"] = 0.5
			case "string":
				second["order"] = "1"
			}
			before["views"] = []any{first, second}
			if _, err := preparePhase3TableEdit("table-view-create", map[string]any{"tableGuid": "t1", "viewGuid": "v1", "name": "new grid", "type": "Grid"}, before); err == nil {
				t.Fatal("ambiguous order accepted")
			}
		})
	}
}
