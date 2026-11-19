package main

import "testing"

func TestPhase3TableViewTypedUpdate(t *testing.T) {
	before := tableFixtureForEdit()
	payload := map[string]any{"tableGuid": "t1", "viewGuid": "v1", "name": "filtered", "filters": map[string]any{"conjunction": "And", "conditions": []any{map[string]any{"fieldGuid": "f1", "operator": "Equal", "value": "todo"}}}, "groups": []any{map[string]any{"fieldGuid": "f1", "order": "Asc"}}, "sort": map[string]any{"rules": []any{map[string]any{"fieldGuid": "f2", "order": "Desc"}}, "keepSorted": true}}
	p, err := preparePhase3TableEdit("table-view-update", payload, before)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 4 {
		t.Fatalf("expected 4 native operations, got %d", len(p.Changes))
	}
	after := phase3TableCopy(before)
	v := after["views"].([]any)[0].(map[string]any)
	for _, k := range []string{"name", "filters", "groups", "sort"} {
		v[k] = p.Expected["views"].(map[string]any)["v1"].(map[string]any)[k]
	}
	v["records"] = []any{"r2", "r1"}
	if err = verifyPhase3TableEdit(before, after, p); err != nil {
		t.Fatal(err)
	}
	v["records"] = []any{"r1", "r2"}
	if err = verifyPhase3TableEdit(before, after, p); err == nil {
		t.Fatal("wrong sort persistence accepted")
	}
}

func TestPhase3TableViewRejectsUnknownFieldsAndLastDelete(t *testing.T) {
	before := tableFixtureForEdit()
	for _, payload := range []map[string]any{
		{"tableGuid": "t1", "viewGuid": "missing", "name": "x"},
		{"tableGuid": "t1", "viewGuid": "v1", "permission": "public"},
		{"tableGuid": "t1", "viewGuid": "v1", "groups": []any{map[string]any{"fieldGuid": "missing", "order": "Asc"}}},
		{"tableGuid": "t1", "viewGuid": "v1", "filters": map[string]any{"conjunction": "And", "conditions": []any{map[string]any{"fieldGuid": "f2", "operator": "Equal", "value": "wrong type"}}}},
	} {
		if _, err := preparePhase3TableEdit("table-view-update", payload, before); err == nil {
			t.Fatalf("accepted %v", payload)
		}
	}
	if _, err := preparePhase3TableEdit("table-view-delete", map[string]any{"tableGuid": "t1", "viewGuid": "v1"}, before); err == nil {
		t.Fatal("last view deletion accepted")
	}
}

func TestPhase3TableSpecializedViewConfiguration(t *testing.T) {
	before := tableFixtureForEdit()
	before["fields"] = append(before["fields"].([]any), map[string]any{"guid": "status", "name": "status", "type": "SingleChoice"}, map[string]any{"guid": "date", "name": "date", "type": "Date"})
	for _, payload := range []map[string]any{
		{"tableGuid": "t1", "viewGuid": "v1", "name": "board", "type": "Kanban", "groupFieldGuid": "status"},
		{"tableGuid": "t1", "viewGuid": "v1", "name": "calendar", "type": "Calendar", "startFieldGuid": "date", "endFieldGuid": "date"},
	} {
		p, err := preparePhase3TableEdit("table-view-create", payload, before)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.NewViews) != 1 {
			t.Fatal("missing typed expected view")
		}
		for _, view := range p.NewViews {
			if payload["type"] == "Kanban" && view["kanban"] == nil {
				t.Fatal("missing kanban metadata")
			}
			if payload["type"] == "Calendar" && view["calendar"] == nil {
				t.Fatal("missing calendar metadata")
			}
		}
	}
	if _, err := preparePhase3TableEdit("table-view-create", map[string]any{"tableGuid": "t1", "viewGuid": "v1", "name": "bad", "type": "Calendar", "startFieldGuid": "f1", "endFieldGuid": "date"}, before); err == nil {
		t.Fatal("non-date calendar binding accepted")
	}
}

func TestPhase3TableViewDeleteRejectsForeignReference(t *testing.T) {
	a := tableFixtureForEdit()
	b := phase3TableCopy(a)
	b["guid"] = "t2"
	b["fields"].([]any)[1].(map[string]any)["type"] = "ReferenceRecord"
	b["fields"].([]any)[1].(map[string]any)["props"] = map[string]any{"tableGuid": "t1", "isRefToView": true, "refViewGuid": "v1"}
	if _, err := phase3ViewDeleteDependencies(map[string]map[string]any{"t1": a, "t2": b}, "t1", "v1"); err == nil {
		t.Fatal("referenced view deletion accepted")
	}
}

func TestPhase3NumericSortKeepsNativeBlankGroupLast(t *testing.T) {
	for _, order := range []string{"Asc", "Desc"} {
		before := tableFixtureForEdit()
		before["records"] = append(before["records"].([]any), map[string]any{"guid": "empty1", "valueMap": map[string]any{}}, map[string]any{"guid": "empty2", "valueMap": map[string]any{"f2": nil}})
		view := phase3TableMap(before, "views")["v1"]
		view["records"] = []any{"empty1", "r1", "empty2", "r2"}
		rules := []any{map[string]any{"fieldGuid": "f2", "order": order}}
		got, err := phase3SortedViewRecords(before, view, rules)
		if err != nil {
			t.Fatal(err)
		}
		first, second := "r1", "r2"
		if order == "Desc" {
			first, second = second, first
		}
		if len(got) != 4 || got[0] != first || got[1] != second || got[2] != "empty1" || got[3] != "empty2" {
			t.Fatal("native stable blank-last order not preserved", got)
		}
		phase3TableMap(before, "records")["empty1"]["valueMap"].(map[string]any)["f2"] = "invalid"
		if _, err = phase3SortedViewRecords(before, view, rules); err == nil {
			t.Fatal("invalid numeric text silently treated as blank")
		}
	}
}
