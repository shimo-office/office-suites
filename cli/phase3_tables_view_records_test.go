package main

import "testing"

func TestPhase3ViewRecordSpecUsesIndependentFilteredRecords(t *testing.T) {
	table := tableFixtureForEdit()
	view := phase3TableCopy(phase3TableMap(table, "views")["v1"])
	view["filters"] = map[string]any{"conjunction": "And", "conditions": []any{map[string]any{"fieldGuid": "f2", "operator": "GreaterThan", "value": float64(1)}}}
	spec, err := phase3ViewRecordSpec(table, view, "")
	if err != nil {
		t.Fatal(err)
	}
	expected := spec["expectedRecords"].(map[string]any)
	if len(expected) != 1 || expected["r2"] == nil {
		t.Fatalf("filter oracle mismatch %v", expected)
	}
}

func TestPhase3CalendarRecordSpecConvertsOAUsingDocumentTimezone(t *testing.T) {
	table := tableFixtureForEdit()
	view := phase3TableCopy(phase3TableMap(table, "views")["v1"])
	view["type"] = "Calendar"
	view["calendar"] = map[string]any{"startFieldGuid": "start", "endFieldGuid": "end"}
	for _, row := range phase3TableMap(table, "records") {
		v := row["valueMap"].(map[string]any)
		v["start"], v["end"] = float64(25569), float64(25570)
	}
	spec, err := phase3ViewRecordSpec(table, view, "America/Los_Angeles", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	row := spec["expectedRecords"].(map[string]any)["r1"].(map[string]any)
	if row["startDay"] != "1969-12-31" || row["endDay"] != "1970-01-01" {
		t.Fatal("calendar timezone ignored")
	}
	delete(phase3TableMap(table, "records")["r1"]["valueMap"].(map[string]any), "start")
	if _, err = phase3ViewRecordSpec(table, view, "UTC", "Asia/Shanghai"); err == nil {
		t.Fatal("missing calendar event accepted")
	}
}

func TestPhase3ViewRecordSpecRefusesNonemptyUnnamedRecord(t *testing.T) {
	for _, value := range []any{float64(0), float64(25569), false, "has-data", []any{"choice"}} {
		table := tableFixtureForEdit()
		view := phase3TableMap(table, "views")["v1"]
		values := phase3TableMap(table, "records")["r1"]["valueMap"].(map[string]any)
		values["f1"], values["f2"] = "", value
		if _, err := phase3ViewRecordSpec(table, view, ""); err == nil {
			t.Fatalf("unnamed populated record %v silently excluded", value)
		}
	}
}
