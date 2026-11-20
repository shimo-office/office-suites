package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestIssue3Phase3NoBrowserRejectsPageActionsBeforeRequests(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("SHIMO_NET_KEYCHAIN_SERVICE", "shimo-net-cli-issue3-test")
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
	old := httpClient
	t.Cleanup(func() { httpClient = old })
	calls := 0
	browserCalls := 0
	oldDocument, oldForm, oldPublish := phase3OpenDocumentEditor, phase3OpenFormEditor, phase3OpenFormPublishEditor
	oldSlides, oldTable := phase3OpenSlidesEditor, phase3OpenTableGuard
	t.Cleanup(func() {
		phase3OpenDocumentEditor, phase3OpenFormEditor, phase3OpenFormPublishEditor = oldDocument, oldForm, oldPublish
		phase3OpenSlidesEditor, phase3OpenTableGuard = oldSlides, oldTable
	})
	stopBrowser := func(string, string) (*phase3EditorBrowser, error) {
		browserCalls++
		return nil, errors.New("synthetic browser launch blocked by test")
	}
	phase3OpenDocumentEditor, phase3OpenFormEditor, phase3OpenFormPublishEditor = stopBrowser, stopBrowser, stopBrowser
	phase3OpenSlidesEditor = func(string) (*phase3EditorBrowser, error) { return stopBrowser("", "") }
	phase3OpenTableGuard = func(string) (phase3TableGuard, error) {
		browserCalls++
		return nil, errors.New("synthetic presence launch blocked by test")
	}
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return phase3Response(404, `{}`), nil
	})}
	for _, mode := range []string{"flag", "env"} {
		for _, action := range []string{"document-format", "document-goto", "document-table", "document-layout", "table-update", "table-chart-create", "form-edit", "form-draft", "form-preview", "form-respondent", "form-submit", "slide-inspect", "slide-design-inspect", "slide-add"} {
			t.Run(mode+action, func(t *testing.T) {
				t.Setenv("SHIMO_NET_NO_BROWSER", "")
				args := []string{"phase3", "https://shimo.net/docx/abcdefgh", "--action", action, "--payload", `{}`}
				if mode == "flag" {
					args = append(args, "--no-browser")
				} else {
					t.Setenv("SHIMO_NET_NO_BROWSER", "1")
				}
				calls = 0
				err := newApp(args).Run()
				if err == nil || !strings.Contains(err.Error(), "禁止启动浏览器") {
					t.Errorf("page action not rejected early: %v", err)
				}
				if calls != 0 {
					t.Errorf("blocked page action performed %d requests", calls)
				}
				if browserCalls != 0 {
					t.Errorf("blocked action called browser constructor %d times", browserCalls)
				}
			})
		}
	}
}

func TestIssue3SharedPhase3BrowserConstructorHonorsEnvironment(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("SHIMO_NET_KEYCHAIN_SERVICE", "shimo-net-cli-issue3-test")
	t.Setenv("SHIMO_NET_NO_BROWSER", "true")
	setupCalled := false
	// Wrong origin makes the RED test safe even before the browser guard exists.
	b, err := openPhase3EditorWithSetup("https://different.invalid/docx/abcdefgh", "true", func(*phase3EditorBrowser) error { setupCalled = true; return nil })
	if b != nil || setupCalled || err == nil || !strings.Contains(err.Error(), "禁止启动浏览器") {
		t.Fatalf("constructor did not reject browser prohibition first: setup=%t error=%v", setupCalled, err)
	}
}

func TestIssue3NoBrowserKeepsAPIOnlyAndArtifactPaths(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("SHIMO_NET_KEYCHAIN_SERVICE", "shimo-net-cli-issue3-test")
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
	t.Setenv("SHIMO_NET_NO_BROWSER", "1")
	old := httpClient
	t.Cleanup(func() { httpClient = old })
	for _, action := range []string{"document-layout-inspect", "table-inspect", "form-inspect", "form-results", "slide-print-pdf", "sheet-update"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" {
					t.Errorf("failed preflight reached mutation: %s", r.Method)
				}
				return nil, errors.New("synthetic API preflight")
			})}
			args := []string{"phase3", "https://shimo.net/sheets/abcdefgh", "--action", action, "--no-browser", "--out", t.TempDir()}
			if action == "sheet-update" {
				args = append(args, "--payload", `{"range":"Sheet1!A1","resource":{"values":[["test"]]}}`, "--read-range", "Sheet1!A1")
			}
			err := newApp(args).Run()
			if err == nil || !strings.Contains(err.Error(), "synthetic API preflight") || calls != 1 {
				t.Fatalf("API-only route not preserved: calls=%d error=%v", calls, err)
			}
		})
	}
	file := "examples/接口验证.xlsx"
	out, err := issueCapture(t, newApp([]string{"phase3", "artifact", file, "--no-browser", "--json"}).Run)
	var result map[string]any
	if err != nil || json.Unmarshal([]byte(out), &result) != nil || result["ok"] != true || result["actionVerified"] != false {
		t.Fatalf("local artifact route not preserved: %v %s", err, out)
	}
}
