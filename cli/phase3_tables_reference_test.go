package main

import (
	"encoding/json"
	"testing"
)

func TestPhase3TableReferenceUsesKnownReverseField(t *testing.T) {
	a := tableFixtureForEdit()
	b := phase3TableCopy(a)
	b["guid"] = "t2"
	b["name"] = "target"
	p, err := preparePhase3BaseEdit("table-reference-add", map[string]any{"tableGuid": "t1", "referenceTableGuid": "t2", "name": "linked"}, "b1", map[string]map[string]any{"t1": a, "t2": b})
	if err != nil {
		t.Fatal(err)
	}
	if p.ReferenceTableGuid != "t2" || p.ReferenceFieldGuid == "" || p.FieldGuid == "" {
		t.Fatal("missing reverse identity")
	}
	var d map[string]any
	_ = json.Unmarshal([]byte(p.Changes[0].Data), &d)
	cols := d["cols"].([]any)
	props := cols[0].(map[string]any)["props"].(map[string]any)
	if props["tableGuid"] != "t2" || props["foreignTableColumnGuid"] != p.ReferenceFieldGuid {
		t.Fatal("native reference payload mismatch")
	}
	before := map[string]map[string]any{"t1": a, "t2": b}
	after := map[string]map[string]any{"t1": phase3TableCopy(a), "t2": phase3TableCopy(b)}
	for _, side := range []struct{ table, field, other, reverse string }{{"t1", p.FieldGuid, "t2", p.ReferenceFieldGuid}, {"t2", p.ReferenceFieldGuid, "t1", p.FieldGuid}} {
		table := after[side.table]
		field := map[string]any{"guid": side.field, "name": "linked", "type": "ReferenceRecord", "isPrimary": false, "props": map[string]any{"baseGuid": "", "tableGuid": side.other, "isMultiple": true, "isRefToView": false, "refViewGuid": "", "error": "", "foreignTableColumnGuid": side.reverse}}
		table["fields"] = append(table["fields"].([]any), field)
		for _, v := range phase3TableMap(table, "views") {
			v["fields"] = append(v["fields"].([]any), side.field)
		}
	}
	if err = verifyPhase3BaseEdit(before, after, p); err != nil {
		t.Fatal(err)
	}
	after["t2"]["fields"] = after["t2"]["fields"].([]any)[:2]
	if err = verifyPhase3BaseEdit(before, after, p); err == nil {
		t.Fatal("missing reverse field accepted")
	}
	if _, err = preparePhase3BaseEdit("table-reference-add", map[string]any{"tableGuid": "t1", "referenceTableGuid": "outside", "name": "linked"}, "b1", map[string]map[string]any{"t1": a, "t2": b}); err == nil {
		t.Fatal("unread outside base reference accepted")
	}
}
