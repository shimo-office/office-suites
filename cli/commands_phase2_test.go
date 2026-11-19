package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestPhase2ExpressionUsesWhitelistedHistoryAction(t *testing.T) {
	expr := phase2Expression(phase2Actions["history-show"], "[]", "demo")
	for _, want := range []string{"globalThis.sdk", "actions", "showViewHistory", "await fn.apply"} {
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

func TestPhase2ExpressionVerifiesAuthenticatedPage(t *testing.T) {
	expr := phase2Expression("browser.verify", "[]", "demo")
	for _, want := range []string{"location.pathname", "/login", "验证码登录", "页面未登录", "ok:true"} {
		if !contains(expr, want) {
			t.Errorf("verification expression missing %q: %s", want, expr)
		}
	}
}

func TestPhase2ObjectArg(t *testing.T) {
	payload, err := phase2ObjectArg([]any{map[string]any{"range": "A1:C1"}})
	if err != nil || payload["range"] != "A1:C1" {
		t.Fatalf("payload = %#v, err = %v", payload, err)
	}
	for _, args := range [][]any{{}, {"A1"}, {map[string]any{}, map[string]any{}}} {
		if _, err := phase2ObjectArg(args); err == nil {
			t.Fatalf("expected object validation error for %#v", args)
		}
	}
}

func TestPhase2ActionNamesIncludeSpreadsheetAPIs(t *testing.T) {
	got := phase2ActionNames()
	for _, want := range []string{"sheet-format", "sheet-merge", "sheet-values", "sheet-update"} {
		found := false
		for _, name := range got {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing action %q", want)
		}
	}
}

func TestRunPhase2HTTPFormatsCells(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/lizard-api/files/demo/sheets/attrs" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("cookie") != "sid=test" {
			t.Fatalf("cookie leaked or missing: %q", r.Header.Get("cookie"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["range"] != "工作表1!A1:C1" {
			t.Fatalf("range = %#v", body["range"])
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	a := &App{Origin: srv.URL}
	value, err := a.runPhase2HTTP("demo", phase2Actions["sheet-format"], []any{map[string]any{
		"range":     "A1:C1",
		"sheetName": "工作表1",
		"attrs":     map[string]any{"background": "#D32F2F", "bold": true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if str(value.(map[string]any), "status") != "204" {
		t.Fatalf("result = %#v", value)
	}
}

func TestPhase2RowActionDescriptor(t *testing.T) {
	if phase2Actions["sheet-insert"] != "http:POSTROW:/lizard-api/files/%s/sheets" {
		t.Fatalf("sheet-insert descriptor = %q", phase2Actions["sheet-insert"])
	}
	if phase2Actions["sheet-delete"] != "http:DELETEROW:/lizard-api/files/%s/sheets" {
		t.Fatalf("sheet-delete descriptor = %q", phase2Actions["sheet-delete"])
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

func TestCLIBrowserVerificationIsOptIn(t *testing.T) {
	base := newApp([]string{"phase2", "demo"})
	if cliBrowserVerification(base) {
		t.Fatal("local browser must be opt-in")
	}
	verify := newApp([]string{"phase2", "demo", "--verify"})
	if !cliBrowserVerification(verify) {
		t.Fatal("--verify not honored")
	}
	no := newApp([]string{"phase2", "demo", "--no-verify"})
	if cliBrowserVerification(no) {
		t.Fatal("--no-verify not honored")
	}
	t.Setenv("SHIMO_NET_VERIFY", "1")
	if !cliBrowserVerification(base) {
		t.Fatal("SHIMO_NET_VERIFY not honored")
	}
}

func TestIsRetryableEditorError(t *testing.T) {
	for _, msg := range []string{
		"页面未暴露 editor SDK",
		"SDK 方法不可用：history.show",
		"页面尚未渲染完成，不能作为视觉核验证据",
		"Cannot read properties of undefined (reading 'actions')",
	} {
		if !isRetryableEditorError(errors.New(msg)) {
			t.Fatalf("expected retryable: %s", msg)
		}
	}
	if isRetryableEditorError(errors.New("HTTP 403")) {
		t.Fatal("HTTP 403 must not be retried as an editor error")
	}
}
