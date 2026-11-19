package main

import "testing"

func TestPhase3TableReferenceSetUsesExistingRecords(t *testing.T) {
	a := tableFixtureForEdit()
	b := phase3TableCopy(a)
	b["guid"] = "t2"
	a["fields"] = append(a["fields"].([]any), map[string]any{"guid": "link", "type": "ReferenceRecord", "props": map[string]any{"tableGuid": "t2", "foreignTableColumnGuid": "back", "isMultiple": true}})
	b["fields"] = append(b["fields"].([]any), map[string]any{"guid": "back", "type": "ReferenceRecord", "props": map[string]any{"tableGuid": "t1", "foreignTableColumnGuid": "link", "isMultiple": true}})
	tables := map[string]map[string]any{"t1": a, "t2": b}
	p, err := preparePhase3BaseEdit("table-reference-set", map[string]any{"tableGuid": "t1", "fieldGuid": "link", "recordGuid": "r1", "referenceRecordGuids": []any{"r2"}}, "b1", tables)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 2 {
		t.Fatal("must set both reference cells explicitly")
	}
	if err = verifyPhase3BaseEdit(tables, p.LinkedExpected, p); err != nil {
		t.Fatal(err)
	}
	if err = verifyPhase3BaseEdit(tables, tables, p); err == nil {
		t.Fatal("unchanged references accepted")
	}
	if _, err = preparePhase3BaseEdit("table-reference-set", map[string]any{"tableGuid": "t1", "fieldGuid": "link", "recordGuid": "r1", "referenceRecordGuids": []any{"missing"}}, "b1", tables); err == nil {
		t.Fatal("nonexistent related record accepted")
	}
}

func TestPhase3TableReferenceSetRefusesUnsafeExistingRelations(t *testing.T) {
	for _, tc := range []string{"synced", "form", "viewLimited", "crossBaseBack", "missingMirror", "orphanMirror", "duplicateMirror"} {
		t.Run(tc, func(t *testing.T) {
			a := tableFixtureForEdit()
			b := phase3TableCopy(a)
			b["guid"] = "t2"
			props := map[string]any{"tableGuid": "t2", "foreignTableColumnGuid": "back", "isMultiple": true}
			back := map[string]any{"tableGuid": "t1", "foreignTableColumnGuid": "link", "isMultiple": true}
			a["fields"] = append(a["fields"].([]any), map[string]any{"guid": "link", "type": "ReferenceRecord", "props": props})
			b["fields"] = append(b["fields"].([]any), map[string]any{"guid": "back", "type": "ReferenceRecord", "props": back})
			av := phase3TableMap(a, "records")["r1"]["valueMap"].(map[string]any)
			bv := phase3TableMap(b, "records")["r2"]["valueMap"].(map[string]any)
			ref := map[string]any{"refRecordGuid": "r1", "refDisplayName": "one"}
			switch tc {
			case "synced":
				b["syncedConfig"] = map[string]any{}
			case "form":
				a["formGuid"] = "form"
			case "viewLimited":
				props["isRefToView"] = true
			case "crossBaseBack":
				back["baseGuid"] = "outside"
			case "missingMirror":
				av["link"] = []any{map[string]any{"refRecordGuid": "r2", "refDisplayName": "two"}}
			case "orphanMirror":
				bv["back"] = []any{ref}
			case "duplicateMirror":
				av["link"] = []any{map[string]any{"refRecordGuid": "r2", "refDisplayName": "two"}}
				bv["back"] = []any{ref, ref}
			}
			_, err := preparePhase3BaseEdit("table-reference-set", map[string]any{"tableGuid": "t1", "fieldGuid": "link", "recordGuid": "r1", "referenceRecordGuids": []any{"r2"}}, "b1", map[string]map[string]any{"t1": a, "t2": b})
			if err == nil {
				t.Fatal("unsafe existing relation accepted")
			}
		})
	}
}
