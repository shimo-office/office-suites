package main

import "testing"

func TestPhase3TableConversionRejectsLoss(t *testing.T) {
	before := tableFixtureForEdit()
	p, err := preparePhase3TableEdit("table-field-convert", map[string]any{"tableGuid": "t1", "fieldGuid": "f2", "type": "SingleLineText"}, before)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 2 {
		t.Fatal("conversion must explicitly convert values with schema")
	}
	after := phase3TableCopy(before)
	after["fields"].([]any)[1].(map[string]any)["type"] = "SingleLineText"
	for _, row := range phase3TableRows(after, "records") {
		r := row.(map[string]any)
		v := r["valueMap"].(map[string]any)
		if v["f2"] == float64(1) {
			v["f2"] = "1"
		} else {
			v["f2"] = "2"
		}
	}
	if err = verifyPhase3TableEdit(before, after, p); err != nil {
		t.Fatal(err)
	}
	after["records"].([]any)[0].(map[string]any)["valueMap"].(map[string]any)["f2"] = "001"
	if _, err = preparePhase3TableEdit("table-field-convert", map[string]any{"tableGuid": "t1", "fieldGuid": "f2", "type": "Number"}, after); err == nil {
		t.Fatal("lossy numeric conversion accepted")
	}
}
