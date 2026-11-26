package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBrowserDiagnosticsRetainedUntilClosedThenExpire(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, root)
	}
	b := &phase3EditorBrowser{}
	cause := errors.New("synthetic readiness failure")
	err := b.captureReadinessFailure(cause)
	if !errors.Is(err, cause) {
		t.Fatalf("lost original failure: %v", err)
	}
	dirs, err := filepath.Glob(filepath.Join(root, "shimo-phase3-browser-failure-*"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("diagnostics not saved: %v %v", dirs, err)
	}
	dir := dirs[0]
	if err := pruneEvidenceAt(root, time.Now().Add(8*24*time.Hour), evidenceMaxAge, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "diagnostic.json")); err != nil {
		t.Fatalf("active browser diagnostic was removed: %v", err)
	}
	b.Close()
	if _, err := os.Stat(filepath.Join(dir, "diagnostic.json")); err != nil {
		t.Fatalf("fresh failure must remain available: %v", err)
	}
	if err := pruneManagedEvidence(time.Now().Add(8*24*time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("closed browser diagnostic leaked: %v", err)
	}
	// Closing an already closed browser must not recreate expired evidence.
	b.Close()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("second close recreated evidence: %v", err)
	}
}

func TestBrowserClosePreservesDiagnosticFailureMessage(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, root)
	}
	profile := filepath.Join(root, "browser-profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	b := &phase3EditorBrowser{profile: profile}
	err := b.captureReadinessFailure(errors.New("synthetic timeout"))
	b.Close()
	if err == nil || !strings.Contains(err.Error(), "浏览器诊断：") {
		t.Fatalf("failure must identify retained diagnostics: %v", err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("temporary browser profile remains: %v", err)
	}
	dirs, _ := filepath.Glob(filepath.Join(root, "shimo-phase3-browser-failure-*"))
	if len(dirs) != 1 {
		t.Fatalf("diagnostic result missing: %v", dirs)
	}
	record, readErr := readEvidenceRetention(dirs[0])
	if readErr != nil || record.CompletedAt.IsZero() {
		t.Fatalf("diagnostic retention not finalized: %+v %v", record, readErr)
	}
}

func TestBrowserCloseReportsProfileCleanupFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires POSIX permissions enforced for a non-root user")
	}
	parent := t.TempDir()
	profile := filepath.Join(parent, "profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0700) })
	log, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = log
	t.Cleanup(func() { os.Stderr = previous; _ = log.Close() })
	(&phase3EditorBrowser{profile: profile}).Close()
	os.Stderr = previous
	raw, err := os.ReadFile(log.Name())
	if err != nil || !strings.Contains(string(raw), "临时浏览器配置未能清理") || !strings.Contains(string(raw), profile) {
		t.Fatalf("cleanup failure not reported: %s (%v)", raw, err)
	}
	if _, err := os.Stat(profile); err != nil {
		t.Fatalf("test did not exercise retained profile: %v", err)
	}
}
