package main

import "testing"

func TestPhase3TableFieldDeleteCrossTableDependencies(t *testing.T) {
	a := tableFixtureForEdit()
	b := phase3TableCopy(a)
	b["guid"] = "t2"
	b["fields"].([]any)[1].(map[string]any)["props"] = map[string]any{"refFieldGuid": "f2"}
	if _, err := phase3FieldDeleteDependencies(map[string]map[string]any{"t1": a, "t2": b}, "t1", map[string]any{"fieldGuids": []any{"f2"}}); err == nil {
		t.Fatal("cross-table reference ignored")
	}
	a["fields"].([]any)[1].(map[string]any)["type"] = "ReferenceRecord"
	if _, err := preparePhase3TableEdit("table-field-delete", map[string]any{"tableGuid": "t1", "fieldGuids": []any{"f2"}}, a); err == nil {
		t.Fatal("relation field delete allowed without reverse verification")
	}
}

func TestPhase3TableFieldRenameAndDeleteAreTargeted(t *testing.T) {
	before := tableFixtureForEdit()
	plan, err := preparePhase3TableEdit("table-field-update", map[string]any{"tableGuid": "t1", "fieldGuid": "f2", "name": "工时"}, before)
	if err != nil {
		t.Fatal(err)
	}
	after := tableFixtureForEdit()
	after["fields"].([]any)[1].(map[string]any)["name"] = "工时"
	if err := verifyPhase3TableEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := preparePhase3TableEdit("table-field-delete", map[string]any{"tableGuid": "t1", "fieldGuids": []any{"f1"}}, before); err == nil {
		t.Fatal("primary field deletion accepted")
	}
	plan, err = preparePhase3TableEdit("table-field-delete", map[string]any{"tableGuid": "t1", "fieldGuids": []any{"f2"}}, before)
	if err != nil {
		t.Fatal(err)
	}
	after = tableFixtureForEdit()
	after["fields"] = after["fields"].([]any)[:1]
	for _, r := range after["records"].([]any) {
		delete(r.(map[string]any)["valueMap"].(map[string]any), "f2")
	}
	after["views"].([]any)[0].(map[string]any)["fields"] = []any{"f1"}
	if err := verifyPhase3TableEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
}
