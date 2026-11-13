package main

import (
	"reflect"
	"testing"
)

func TestPhase2Args(t *testing.T) {
	got, err := phase2Args(`[{"query":"Q4"}, {"limit":10}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("args length = %d", len(got))
	}
	if _, ok := got[0].(map[string]any); !ok {
		t.Fatalf("first arg type = %T", got[0])
	}
	one, err := phase2Args(`{"width":900}`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one, []any{map[string]any{"width": float64(900)}}) {
		t.Fatalf("single object args = %#v", one)
	}
	if _, err := phase2Args("not-json"); err == nil {
		t.Fatal("invalid JSON should fail")
	}
}

func TestPhase2ExpressionWhitelistsMethodPath(t *testing.T) {
	expr := phase2Expression("history.show", "[]", "demo")
	for _, want := range []string{"globalThis.sdk", "history", "show", "await fn.apply"} {
		if !contains(expr, want) {
			t.Errorf("expression missing %q: %s", want, expr)
		}
	}
}

func TestPhase2ExpressionUsesDocumentAPIs(t *testing.T) {
	expr := phase2Expression("api:mention-at-list", "[]", "wv67RM7GKLCGNEWD")
	for _, want := range []string{"mention-at-list", "credentials", "wv67RM7GKLCGNEWD"} {
		if !contains(expr, want) {
			t.Errorf("API expression missing %q: %s", want, expr)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
