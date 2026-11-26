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

func isolatedEvidenceTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, dir)
	}
	return dir
}

func TestEvidenceLifecycleFinishesAfterDeferredWriters(t *testing.T) {
	isolatedEvidenceTemp(t)
	a := newApp([]string{"phase3", "https://shimo.net/docs/abcdefgh", "--action", "doc-replace"})
	var dir string
	out, err := issueCapture(t, func() error {
		return a.withPhase3Evidence(func() error {
			var err error
			dir, err = a.phase3EvidenceDir("")
			if err != nil {
				return err
			}
			defer func() {
				if err := phase3WriteEvidence(filepath.Join(dir, "late-log.json"), map[string]any{"finished": true}); err != nil {
					t.Error(err)
				}
			}()
			return a.printJSON(map[string]any{"ok": true, "artifact": ArtifactEvidence{Path: filepath.Join(dir, "before.docx"), Kind: "docx", Bytes: 123}, "evidenceDir": dir})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("late writer recreated directory: %v", err)
	}
	if strings.Contains(out, dir) || !strings.Contains(out, `"bytes": 123`) {
		t.Fatalf("incorrect output: %s", out)
	}
}

func TestEvidenceLifecyclePreservesRequestedFiles(t *testing.T) {
	for _, action := range []string{"slide-print-pdf", "form-results"} {
		t.Run(action, func(t *testing.T) {
			isolatedEvidenceTemp(t)
			a := newApp([]string{"phase3", "https://shimo.net/docs/abcdefgh", "--action", action})
			var dir string
			_, err := issueCapture(t, func() error {
				return a.withPhase3Evidence(func() error {
					var err error
					dir, err = a.phase3EvidenceDir("")
					if err != nil {
						return err
					}
					if err := os.WriteFile(filepath.Join(dir, "deliverable"), []byte("user file"), 0600); err != nil {
						return err
					}
					return a.printJSON(map[string]any{"ok": true, "evidenceDir": dir})
				})
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := pruneManagedEvidence(time.Now().Add(30*24*time.Hour), nil); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "deliverable")); err != nil {
				t.Fatal("requested export deleted", err)
			}
		})
	}
}

func TestEvidenceLifecycleOutputFailureRetainsCompletedEvidence(t *testing.T) {
	isolatedEvidenceTemp(t)
	a := newApp([]string{"phase3", "https://shimo.net/docs/abcdefgh", "--action", "doc-replace"})
	closed, err := os.CreateTemp(t.TempDir(), "closed-output")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = closed
	t.Cleanup(func() { os.Stdout = stdout })
	var dir string
	err = a.withPhase3Evidence(func() error {
		var err error
		dir, err = a.phase3EvidenceDir("")
		if err != nil {
			return err
		}
		return a.printJSON(map[string]any{"ok": true, "evidenceDir": dir})
	})
	if err == nil || !strings.Contains(err.Error(), "禁止自动重放") {
		t.Fatalf("unexpected output failure: %v", err)
	}
	record, err := readEvidenceRetention(dir)
	if err != nil || record.CompletedAt.IsZero() {
		t.Fatalf("output failure lost evidence: %v, %v", record, err)
	}
}

func TestEvidenceLifecycleFailureDoesNotEmitBufferedSuccess(t *testing.T) {
	isolatedEvidenceTemp(t)
	a := newApp([]string{"phase3"})
	want := errors.New("deferred verification failed")
	out, err := issueCapture(t, func() error {
		return a.withPhase3Evidence(func() (err error) {
			defer func() { err = want }()
			return a.printJSON(map[string]any{"ok": true})
		})
	})
	if !errors.Is(err, want) || out != "" {
		t.Fatalf("buffered success escaped failure: %q, %v", out, err)
	}
}

func TestEvidenceResultPreservesBusinessContentAndNumbers(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "evidence")
	path := filepath.Join(dir, "before.md")
	result := map[string]any{
		"evidenceDir": dir, "before": path, "after": map[string]any{"path": path},
		"spec":           map[string]any{"file": path, "before": path, "path": path},
		"artifact":       ArtifactEvidence{Path: path, Kind: "docx", Bytes: 9007199254740993},
		"completedSteps": []any{map[string]any{"step": 1, "evidenceDir": filepath.Join(dir, "step-01")}},
		"screenshot":     filepath.Join(dir+"-other", "image.png"),
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	out, err := evidenceResultWithoutPaths(raw, []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["before"] != nil || got["evidenceDir"] != nil || got["artifact"].(map[string]any)["path"] != nil {
		t.Fatalf("evidence path survived: %s", out)
	}
	if got["spec"].(map[string]any)["path"] != path || got["after"].(map[string]any)["path"] != path || got["screenshot"] != result["screenshot"] {
		t.Fatalf("business fields corrupted: %s", out)
	}
	if !strings.Contains(string(out), "9007199254740993") {
		t.Fatalf("integer precision lost: %s", out)
	}
	if got["completedSteps"].([]any)[0].(map[string]any)["evidenceDir"] != nil {
		t.Fatalf("nested evidence path survived: %s", out)
	}
	if result["artifact"].(ArtifactEvidence).Path != path {
		t.Fatal("source mutated")
	}
}

func TestEvidenceLifecyclePlainOutputNoDanglingPath(t *testing.T) {
	isolatedEvidenceTemp(t)
	a := newApp([]string{"phase3"})
	out, err := issueCapture(t, func() error {
		return a.withPhase3Evidence(func() error {
			dir, err := a.phase3EvidenceDir("")
			if err != nil {
				return err
			}
			return a.printPhase3Result(map[string]any{"evidenceDir": dir, "exportLimitation": "limitation"}, "done")
		})
	})
	if err != nil || out != "done\nlimitation\n" {
		t.Fatalf("unexpected text output: %q, %v", out, err)
	}
}

func TestEvidenceReplaceDefaultCleansVerifiedArtifacts(t *testing.T) {
	flow := newLightDocFlow(t, "doc-replace", `{"find":"old","replace":"new","scope":"first"}`, "https://shimo.net/docs/abcdefgh",
		func(n int) string {
			if n == 1 {
				return "old"
			}
			return "new"
		}, lightDocFlowReply)
	tmp := isolatedEvidenceTemp(t)
	delete(flow.app.Flags, "--out")
	out, err := issueCapture(t, flow.app.cmdPhase3)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result["verified"] != true || num(result, "replaceAdded") != 1 {
		t.Fatalf("verification lost: %v", result)
	}
	for _, key := range []string{"before", "after", "evidenceDir"} {
		if _, exists := result[key]; exists {
			t.Errorf("default output exposes temporary %s: %v", key, result[key])
		}
	}
	dirs, err := filepath.Glob(filepath.Join(tmp, "shimo-phase3-*"))
	if err != nil || len(dirs) != 0 {
		t.Fatalf("successful operation left evidence: %v, %v", dirs, err)
	}
}

func TestEvidenceReplaceExplicitOutKeepsArtifacts(t *testing.T) {
	flow := newLightDocFlow(t, "doc-replace", `{"find":"old","replace":"new","scope":"first"}`, "https://shimo.net/docs/abcdefgh",
		func(n int) string {
			if n == 1 {
				return "old"
			}
			return "new"
		}, lightDocFlowReply)
	isolatedEvidenceTemp(t)
	out, err := issueCapture(t, flow.app.cmdPhase3)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"before", "after", "evidenceDir"} {
		if _, err := os.Stat(str(result, key)); err != nil {
			t.Fatalf("explicit %s missing: %v", key, err)
		}
	}
}

func TestEvidenceReplaceFailureKeepsArtifacts(t *testing.T) {
	flow := newLightDocFlow(t, "doc-replace", `{"find":"missing","replace":"new"}`, "https://shimo.net/docs/abcdefgh",
		func(int) string { return "other" }, lightDocFlowReply)
	tmp := isolatedEvidenceTemp(t)
	delete(flow.app.Flags, "--out")
	out, err := issueCapture(t, flow.app.cmdPhase3)
	if err == nil || !strings.Contains(err.Error(), "找不到") || out != "" {
		t.Fatalf("unexpected failure: %s, %v", out, err)
	}
	dirs, _ := filepath.Glob(filepath.Join(tmp, "shimo-phase3-*"))
	if len(dirs) != 1 {
		t.Fatalf("failure evidence lost: %v", dirs)
	}
	if _, err := os.Stat(filepath.Join(dirs[0], "failure.json")); err != nil {
		t.Fatal(err)
	}
}
