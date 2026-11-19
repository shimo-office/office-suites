package main

import (
	"testing"
)

func TestReleasePhase2BrowserResultRequiresExplicitEnvelope(t *testing.T) {
	for _, res := range []map[string]any{
		nil, {}, {"result": nil}, {"result": false}, {"result": map[string]any{"type": "undefined"}},
		{"result": map[string]any{"value": nil}}, {"result": map[string]any{"value": true}},
		{"result": map[string]any{"value": map[string]any{"value": "not an acknowledgement"}}},
		{"result": map[string]any{"value": map[string]any{"ok": "true"}}},
	} {
		if _, err := phase2BrowserResult(res); err == nil {
			t.Errorf("missing/malformed CDP result was accepted: %#v", res)
		}
	}
}
func TestReleasePhase2BrowserResultAllowsVoidPanelAndNullAPIValue(t *testing.T) {
	for _, value := range []map[string]any{{"ok": true}, {"ok": true, "value": nil}, {"ok": true, "value": []any{}}} {
		got, err := phase2BrowserResult(map[string]any{"result": map[string]any{"value": value}})
		if err != nil || got.(map[string]any)["ok"] != true {
			t.Fatal(got, err)
		}
	}
	for _, res := range []map[string]any{{"exceptionDetails": map[string]any{}}, {"result": map[string]any{"subtype": "error", "description": "failure"}}, {"result": map[string]any{"value": map[string]any{"ok": false, "error": "not ready"}}}} {
		if _, err := phase2BrowserResult(res); err == nil {
			t.Fatal("failure accepted", res)
		}
	}
}
