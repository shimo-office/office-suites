package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReleaseDocumentReadinessFailureCannotInventScreenshot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	cause := errors.New("synthetic editor unavailable")
	b := &phase3EditorBrowser{}
	err := b.captureReadinessFailure(cause)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "截图成功=false") {
		t.Fatalf("lost failure cause: %v", err)
	}
	dirs, e := os.ReadDir(root)
	if e != nil || len(dirs) != 1 {
		t.Fatalf("diagnostic directory missing: %v", e)
	}
	dir := filepath.Join(root, dirs[0].Name())
	data, e := os.ReadFile(filepath.Join(dir, "diagnostic.json"))
	if e != nil {
		t.Fatal(e)
	}
	var record map[string]any
	if json.Unmarshal(data, &record) != nil || record["screenshotCaptured"] != false || record["screenshotNonblank"] != false || record["captureError"] == "" {
		t.Fatalf("false evidence claim: %s", data)
	}
	if _, e = os.Stat(filepath.Join(dir, "page.png")); !os.IsNotExist(e) {
		t.Fatal("nonexistent screenshot materialized")
	}
	t.Setenv("TMPDIR", filepath.Join(root, "missing"))
	if e = b.captureReadinessFailure(cause); !errors.Is(e, cause) || !strings.Contains(e.Error(), "无法创建") {
		t.Fatalf("diagnostic storage error hidden: %v", e)
	}
}

func TestReleaseDocumentCaptureCarriesStartupEvidenceAndFailsClosed(t *testing.T) {
	for _, fault := range []string{"control", "missing-startup", "startup-destination-blocked"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			startup := filepath.Join(root, "source.json")
			data := []byte(`{"scope":"initial read readiness only","mutationAttempted":false,"ready":true}`)
			if fault != "missing-startup" {
				if err := os.WriteFile(startup, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			out := filepath.Join(root, "out")
			if err := os.Mkdir(out, 0700); err != nil {
				t.Fatal(err)
			}
			if fault == "startup-destination-blocked" {
				if err := os.Mkdir(filepath.Join(out, "startup-recovery.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			captures := 0
			b := releaseDocumentEditor(t, func(string) any { t.Error("capture evaluated editor JS"); return nil }, func() error { captures++; return nil }, nil)
			b.startupEvidence = startup
			err := b.Capture(filepath.Join(out, "page.png"))
			if (err == nil) != (fault == "control") {
				t.Fatalf("unexpected capture outcome: %v", err)
			}
			if fault == "control" {
				copied, e := os.ReadFile(filepath.Join(out, "startup-recovery.json"))
				if e != nil || string(copied) != string(data) || captures != 1 {
					t.Fatalf("startup provenance lost: %v %s", e, copied)
				}
			} else if captures != 0 {
				t.Fatal("screenshot taken after provenance storage failed")
			}
		})
	}
}

func TestReleaseDocumentInitialReadyUsesTypedFailureAndNoWrite(t *testing.T) {
	for _, login := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "login"}[login], func(t *testing.T) {
			calls := 0
			b := releaseDocumentEditor(t, func(expr string) any {
				calls++
				if !strings.Contains(expr, "typeof window.__shimoPhase3Presence") {
					t.Error("observer missing from initial gate")
				}
				return map[string]any{"ready": !login, "login": login}
			}, nil, nil)
			err := b.waitInitialReady(documentReadyJS, time.Second)
			if (err != nil) != login || calls != 1 {
				t.Fatalf("initial gate outcome %v calls=%d", err, calls)
			}
			if login {
				var typed *phase3ReadinessFailure
				if !errors.As(err, &typed) || typed.Unwrap() == nil || !strings.Contains(err.Error(), "未登录") {
					t.Fatalf("lost typed login failure %v", err)
				}
			}
		})
	}
}

func TestReleaseDocumentSelfMentionIdentityRejectsUnsafeIDs(t *testing.T) {
	for _, tc := range []struct{ id, name string }{{"", "Self"}, {"0", "Self"}, {"-1", "Self"}, {"1e3", "Self"}, {"9007199254740992", "Self"}, {"184467440737095516160", "Self"}, {"42", ""}, {"42", "A\nB"}} {
		if _, _, err := documentMentionIdentity(map[string]any{"id": tc.id, "name": tc.name}); err == nil {
			t.Fatalf("unsafe mention identity accepted: %#v", tc)
		}
	}
	for _, id := range []string{"1", "9007199254740991"} {
		actual, name, err := documentMentionIdentity(map[string]any{"id": id, "displayName": "Self"})
		if err != nil || actual != id || name != "Self" {
			t.Fatalf("valid identity rejected: %s %s %v", actual, name, err)
		}
	}
}
