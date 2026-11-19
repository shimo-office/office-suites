package main

import "testing"

func TestPhase3FormPreviewCases(t *testing.T) {
	b := formEditFixture()
	q := b["questions"].([]any)[0].(map[string]any)
	q["type"] = "single_choice"
	q["question"] = map[string]any{"options": []any{map[string]any{"type": "normal", "value": "yes1", "label": "yes"}}}
	req := map[string]any{"trigger": "questionA", "branches": []any{map[string]any{"option": "yes1", "visible": []any{"questionB"}, "hidden": []any{}}}}
	if id, cs, e := phase3FormPreviewCases(b, req); e != nil || id != "questionA" || len(cs) != 1 {
		t.Fatal(id, cs, e)
	}
	req["trigger"] = "missing"
	if _, _, e := phase3FormPreviewCases(b, req); e == nil {
		t.Fatal("unknown trigger accepted")
	}
	req["trigger"] = "questionA"
	req["branches"] = []any{map[string]any{"option": "missing", "visible": []any{"questionB"}, "hidden": []any{}}}
	if _, _, e := phase3FormPreviewCases(b, req); e == nil {
		t.Fatal("unknown option accepted")
	}
	req["branches"] = []any{map[string]any{"option": "yes1", "visible": []any{"questionB"}, "hidden": []any{"questionB"}}}
	if _, _, e := phase3FormPreviewCases(b, req); e == nil {
		t.Fatal("contradictory expectation accepted")
	}
}

func TestPhase3FormNavigationURL(t *testing.T) {
	for _, raw := range []string{"/forms/abc/preview?t=x", "https://shimo.net/forms/abc/fill"} {
		if _, err := phase3FormNavigationURL(raw, "https://shimo.net"); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{"https://evil.test/", "//evil.test/x", "javascript:alert(1)", "https://me:secret@shimo.net/x"} {
		if _, err := phase3FormNavigationURL(raw, "https://shimo.net"); err == nil {
			t.Fatal(raw)
		}
	}
}

func TestPhase3FormLinkEvidenceRedactsSecrets(t *testing.T) {
	e := phase3FormLinkEvidence("https://shimo.net/forms/id/preview?t=secret&smParams=private#fragment")
	if e["url"] != "https://shimo.net/forms/id/preview" {
		t.Fatal(e)
	}
	keys := e["redactedQueryKeys"].([]string)
	if len(keys) != 2 || keys[0] != "smParams" || keys[1] != "t" {
		t.Fatal(e)
	}
}

func TestPhase3FormDebugURLPreservesSDKRouteOrder(t *testing.T) {
	raw := "https://shimo.net/forms/id/preview?t=signed&smParams=encoded"
	u, err := phase3FormNavigationURL(raw, "https://shimo.net")
	if err != nil {
		t.Fatal(err)
	}
	got := phase3FormDebugURL(u)
	if got != raw+"&debug=1" {
		t.Fatalf("SDK parser needs /preview?t= prefix, got %s", got)
	}
}

func TestPhase3FormPreviewSequenceValidation(t *testing.T) {
	form := formEditFixture()
	q := form["questions"].([]any)[0].(map[string]any)
	q["type"] = "single_choice"
	q["question"] = map[string]any{"options": []any{map[string]any{"type": "normal", "value": "yes1", "label": "yes"}}}
	req := map[string]any{"trigger": "questionA", "branches": []any{map[string]any{"option": "yes1", "visible": []any{"questionB"}, "hidden": []any{}, "sequence": []any{"questionA", "missing"}}}}
	if _, _, err := phase3FormPreviewCases(form, req); err == nil {
		t.Fatal("unknown sequence question accepted")
	}
	req["branches"].([]any)[0].(map[string]any)["sequence"] = []any{"questionA", "questionB"}
	if _, _, err := phase3FormPreviewCases(form, req); err != nil {
		t.Fatal(err)
	}
}
