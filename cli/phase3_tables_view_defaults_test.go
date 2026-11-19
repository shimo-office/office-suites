package main

import (
	"encoding/json"
	"testing"
)

func TestPhase3ViewWriteDefaultsAreExplicitAndStrict(t *testing.T) {
	before := tableFixtureForEdit()
	payload := map[string]any{"tableGuid": "t1", "viewGuid": "v1", "filters": map[string]any{"conjunction": "And", "conditions": []any{map[string]any{"fieldGuid": "f2", "operator": "GreaterThanEqual", "value": float64(1)}}}, "groups": []any{map[string]any{"fieldGuid": "f1", "order": "Asc"}}}
	plan, err := preparePhase3TableEdit("table-view-update", payload, before)
	if err != nil {
		t.Fatal(err)
	}
	var filter, group bool
	for _, c := range plan.Changes {
		var data map[string]any
		json.Unmarshal([]byte(c.Data), &data)
		if c.Action == "UpdateViewFilters" {
			f := data["filters"].(map[string]any)
			v, ok := f["includeSubRecord"]
			filter = ok && v == false
		}
		if c.Action == "UpdateViewGroups" {
			g := data["groups"].([]any)[0].(map[string]any)
			v, ok := g["groupBy"]
			group = ok && v == nil
		}
	}
	if !filter || !group {
		t.Fatal("native defaults omitted from actual mutation", filter, group)
	}
	after := phase3TableCopy(before)
	view := phase3TableMap(after, "views")["v1"]
	expected := plan.Expected["views"].(map[string]any)["v1"].(map[string]any)
	view["filters"], view["groups"] = expected["filters"], expected["groups"]
	if err = verifyPhase3TableEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"subrecords", "groupBy"} {
		changed := phase3TableCopy(after)
		v := phase3TableMap(changed, "views")["v1"]
		if kind == "subrecords" {
			v["filters"].(map[string]any)["includeSubRecord"] = true
		} else {
			v["groups"].([]any)[0].(map[string]any)["groupBy"] = "changed"
		}
		if err = verifyPhase3TableEdit(before, changed, plan); err == nil {
			t.Fatal("changed default accepted", kind)
		}
	}
	if _, exists := payload["filters"].(map[string]any)["includeSubRecord"]; exists {
		t.Fatal("input payload mutated")
	}
}

func TestPhase3NewViewDeclaresSortInSameCompose(t *testing.T) {
	before := tableFixtureForEdit()
	plan, err := preparePhase3TableEdit("table-view-create", map[string]any{"tableGuid": "t1", "viewGuid": "v1", "name": "new grid", "type": "Grid"}, before)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 2 || plan.Changes[0].Action != "CreateView" || plan.Changes[1].Action != "UpdateViewSort" {
		t.Fatal("view creation omitted explicit server sort", plan.Changes)
	}
	var wire map[string]any
	if err = json.Unmarshal([]byte(plan.Changes[1].Data), &wire); err != nil {
		t.Fatal(err)
	}
	id := plan.CreatedIDs[0]
	if str(wire, "viewGuid") != id || str(wire, "tableGuid") != "t1" {
		t.Fatal("sort targets wrong view")
	}
	sort := wire["sort"].(map[string]any)
	if sort["keepSorted"] != true || len(sort["rules"].([]any)) != 0 {
		t.Fatal("sort not explicit")
	}
	rows := wire["records"].([]any)
	if len(rows) != 2 || rows[0] != "r1" || rows[1] != "r2" {
		t.Fatal("source order lost")
	}
}

func TestPhase3CalendarDeclaresEmptyFollowField(t *testing.T) {
	before := tableFixtureForEdit()
	before["fields"] = append(before["fields"].([]any), map[string]any{"guid": "start", "name": "start", "type": "Date"}, map[string]any{"guid": "end", "name": "end", "type": "Date"})
	plan, err := preparePhase3TableEdit("table-view-create", map[string]any{"tableGuid": "t1", "viewGuid": "v1", "name": "calendar", "type": "Calendar", "startFieldGuid": "start", "endFieldGuid": "end"}, before)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	json.Unmarshal([]byte(plan.Changes[0].Data), &wire)
	color := wire["calendar"].(map[string]any)["color"].(map[string]any)
	if color["followFieldGuid"] != "" {
		t.Fatal("server empty field default not explicitly requested", color)
	}
	view := plan.NewViews[plan.CreatedIDs[0]]
	if view["calendar"].(map[string]any)["color"].(map[string]any)["followFieldGuid"] != "" {
		t.Fatal("expected state differs from request")
	}
	actual := phase3TableCopy(view)
	actual["fieldInfo"] = wire["fieldInfo"]
	after := phase3TableCopy(before)
	after["views"] = append(after["views"].([]any), actual)
	if err := verifyPhase3TableEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
	actual["calendar"].(map[string]any)["color"].(map[string]any)["followFieldGuid"] = "start"
	if verifyPhase3TableEdit(before, after, plan) == nil {
		t.Fatal("changed native color field accepted")
	}
}
