package main

import (
	"encoding/json"
	"testing"
)

func TestPhase3FormDraftDecode(t *testing.T) {
	var raw any
	if err := json.Unmarshal([]byte(`[{"action":"insert","attributes":{"title":"T","desc":"","descAlign":"left"},"data":{"form":[[{"action":"insert","data":{"object":{"guid":"q001","required":true,"subject":"Q","description":null,"question":{"type":"text"}}}}],null]}}]`), &raw); err != nil {
		t.Fatal(err)
	}
	got, err := phase3DecodeFormDelta(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["viewMode"] != "classic" || len(got["rules"].([]any)) != 0 || len(got["questions"].([]any)) != 1 {
		t.Fatal(got)
	}
	q := got["questions"].([]any)[0].(map[string]any)
	if q["type"] != "text" || q["guid"] != "q001" {
		t.Fatal(q)
	}
	if _, ok := q["description"]; ok {
		t.Fatal("null description should be omitted")
	}
	for _, bad := range []any{nil, []any{}, []any{map[string]any{"action": "retain"}}, []any{map[string]any{"action": "insert", "data": map[string]any{"form": []any{}}}}} {
		if _, err := phase3DecodeFormDelta(bad); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}
