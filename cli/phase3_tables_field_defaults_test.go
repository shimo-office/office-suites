package main

import (
	"encoding/json"
	"testing"
)

func TestPhase3TableNewFieldExplicitNativeDefaults(t *testing.T) {
	before := tableFixtureForEdit()
	plan, err := preparePhase3TableEdit("table-field-add", map[string]any{"tableGuid": "t1", "fields": []any{map[string]any{"name": "status", "type": "SingleChoice", "options": []any{"todo", "done"}}, map[string]any{"name": "date", "type": "Date"}}}, before)
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	if err = json.Unmarshal([]byte(plan.Changes[0].Data), &request); err != nil {
		t.Fatal(err)
	}
	cols := request["cols"].([]any)
	choiceProps := cols[0].(map[string]any)["props"].(map[string]any)
	for _, raw := range choiceProps["choices"].(map[string]any) {
		choice := raw.(map[string]any)
		for key, want := range map[string]any{"hidden": false, "locked": false, "statusType": float64(0), "style": nil} {
			got, exists := choice[key]
			if !exists || got != want {
				t.Fatalf("missing explicit native default %s: %#v", key, choice)
			}
		}
	}
	date := cols[1].(map[string]any)["props"].(map[string]any)
	if value, exists := date["isAutoFill"]; !exists || value != false {
		t.Fatal("date autofill default not explicit")
	}
	after := phase3TableCopy(before)
	for _, raw := range cols {
		after["fields"] = append(after["fields"].([]any), raw)
	}
	view := after["views"].([]any)[0].(map[string]any)
	for _, id := range plan.CreatedIDs {
		view["fields"] = append(view["fields"].([]any), id)
	}
	if err = verifyPhase3TableEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"hidden", "locked", "statusType", "style"} {
		changed := phase3TableCopy(after)
		choices := phase3TableMap(changed, "fields")[plan.CreatedIDs[0]]["props"].(map[string]any)["choices"].(map[string]any)
		for _, raw := range choices {
			raw.(map[string]any)[key] = "unexpected"
			break
		}
		if err = verifyPhase3TableEdit(before, changed, plan); err == nil {
			t.Fatal("changed native option property accepted", key)
		}
	}
}
