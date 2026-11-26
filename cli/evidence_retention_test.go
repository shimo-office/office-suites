package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func evidenceRetentionFixture(t *testing.T, root, name string, completed time.Time, size int) string {
	t.Helper()
	dir := filepath.Join(root, "shimo-phase3-"+name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := phase3WriteEvidence(filepath.Join(dir, evidenceMarker), evidenceRetention{Owner: "shimo-net/evidence-v1", CompletedAt: completed}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data"), make([]byte, size), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEvidenceCleanupRemainderRestoresRemovedMarker(t *testing.T) {
	isolatedEvidenceTemp(t)
	dir, err := newManagedEvidenceDir("shimo-phase3-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, evidenceMarker)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "leftover"), []byte("keep until prune"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := retainCleanupRemainder(dir); err != nil {
		t.Fatal(err)
	}
	record, err := readEvidenceRetention(dir)
	if err != nil || record.CompletedAt.IsZero() {
		t.Fatalf("remainder unmarked: %v, %v", record, err)
	}
	if err := pruneManagedEvidence(time.Now().Add(8*24*time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("remainder not collected: %v", err)
	}
}

func TestEvidenceLifecyclePartialCleanupCanBeRetried(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix permission enforcement")
	}
	isolatedEvidenceTemp(t)
	a := newApp([]string{"phase3"})
	var dir, locked string
	out, err := issueCapture(t, func() error {
		return a.withPhase3Evidence(func() error {
			var err error
			dir, err = a.phase3EvidenceDir("")
			if err != nil {
				return err
			}
			locked = filepath.Join(dir, "locked")
			if err := os.Mkdir(locked, 0700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(locked, "data"), []byte("locked"), 0600); err != nil {
				return err
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0700) })
			if err := os.Chmod(locked, 0); err != nil {
				return err
			}
			return a.printJSON(map[string]any{"ok": true, "evidenceDir": dir})
		})
	})
	if err != nil || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("cleanup changed mutation outcome: %s, %v", out, err)
	}
	record, err := readEvidenceRetention(dir)
	if err != nil || record.CompletedAt.IsZero() {
		t.Fatalf("partial cleanup not recoverable: %v, %v", record, err)
	}
	if err := os.Chmod(locked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := pruneManagedEvidence(time.Now().Add(8*24*time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("partial remainder not pruned: %v", err)
	}
}

func TestEvidenceRelativeTempRootHasNoDanglingPaths(t *testing.T) {
	base := t.TempDir()
	t.Chdir(base)
	if err := os.Mkdir("relative-temp", 0700); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, "relative-temp")
	}
	a := newApp([]string{"phase3"})
	var dir string
	out, err := issueCapture(t, func() error {
		return a.withPhase3Evidence(func() error {
			var err error
			dir, err = a.phase3EvidenceDir("")
			if err != nil {
				return err
			}
			if !filepath.IsAbs(dir) {
				t.Fatalf("relative managed dir: %s", dir)
			}
			return a.printJSON(map[string]any{"ok": true, "before": filepath.Join(dir, "before.md"), "evidenceDir": dir})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "evidenceDir") || strings.Contains(out, "before") {
		t.Fatalf("dangling relative path: %s", out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("relative temp not cleaned: %v", err)
	}
}

func TestEvidenceLocalInputsProtectedAtCommandStart(t *testing.T) {
	for _, action := range []string{"doc-image", "slide-media-insert"} {
		t.Run(action, func(t *testing.T) {
			root := isolatedEvidenceTemp(t)
			dir := evidenceRetentionFixture(t, root, "input", time.Now().Add(-8*24*time.Hour), 1)
			input := filepath.Join(dir, "data")
			payload, _ := json.Marshal(map[string]any{"file": input})
			a := newApp([]string{"phase3", "https://shimo.net/docs/abcdefgh", "--action", action, "--payload", string(payload)})
			err := a.withPhase3Evidence(func() error { _, err := os.ReadFile(input); return err })
			if err != nil {
				t.Fatalf("local input pruned: %v", err)
			}
		})
	}
}

func TestEvidenceLocalInputProtectsSymlinkPathAndTarget(t *testing.T) {
	for _, managedTarget := range []bool{false, true} {
		t.Run(fmt.Sprint(managedTarget), func(t *testing.T) {
			root := isolatedEvidenceTemp(t)
			dir := evidenceRetentionFixture(t, root, "source", time.Now().Add(-8*24*time.Hour), 1)
			target := t.TempDir()
			if managedTarget {
				target = evidenceRetentionFixture(t, root, "target", time.Now().Add(-8*24*time.Hour), 1)
			}
			if err := os.WriteFile(filepath.Join(target, "file.png"), []byte("input"), 0600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "link")
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			input := filepath.Join(link, "file.png")
			payload, _ := json.Marshal(map[string]any{"file": input})
			a := newApp([]string{"phase3", "--action", "doc-image", "--payload", string(payload)})
			err := a.withPhase3Evidence(func() error { _, err := os.ReadFile(input); return err })
			if err != nil {
				t.Fatalf("symlinked local input pruned: %v", err)
			}
			if _, err := os.Lstat(link); err != nil {
				t.Fatalf("input path symlink removed: %v", err)
			}
		})
	}
}

func TestEvidenceOutputRejectsReservedParentsBeforeAnyWork(t *testing.T) {
	for _, mode := range []string{"completed", "active", "unmarked", "missing"} {
		t.Run(mode, func(t *testing.T) {
			root := isolatedEvidenceTemp(t)
			dir := filepath.Join(root, "shimo-phase3-parent")
			if mode != "missing" {
				completed := time.Now().Add(-8 * 24 * time.Hour)
				if mode == "active" {
					completed = time.Time{}
				}
				dir = evidenceRetentionFixture(t, root, "parent", completed, 1)
				if mode == "unmarked" {
					if err := os.Remove(filepath.Join(dir, evidenceMarker)); err != nil {
						t.Fatal(err)
					}
				}
			}
			parent := filepath.Join(dir, "requested-output")
			a := newApp([]string{"phase3", "--out", parent})
			called := false
			err := a.withPhase3Evidence(func() error { called = true; _, err := a.phase3EvidenceDir(parent); return err })
			if err == nil || !strings.Contains(err.Error(), "独立目录") || called {
				t.Fatalf("reserved output accepted/reached operation: %v, called=%v", err, called)
			}
			if _, err := os.Stat(parent); !os.IsNotExist(err) {
				t.Fatalf("output path created: %v", err)
			}
			if mode != "missing" {
				if _, err := os.Stat(filepath.Join(dir, "data")); err != nil {
					t.Fatalf("existing data deleted on invalid output: %v", err)
				}
			}
			if mode == "completed" || mode == "active" {
				record, err := readEvidenceRetention(dir)
				if err != nil || record.CompletedAt.IsZero() != (mode == "active") {
					t.Fatalf("marker changed: %v, %v", record, err)
				}
			}
		})
	}
}

func TestEvidenceInputProtectionResolvesSymlinkParents(t *testing.T) {
	root := isolatedEvidenceTemp(t)
	dir := evidenceRetentionFixture(t, root, "input", time.Now().Add(-8*24*time.Hour), 1)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := pruneManagedEvidence(time.Now(), []string{filepath.Join(link, "new-child")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("symlink target pruned: %v", err)
	}
	a := newApp([]string{"phase3", "--out", filepath.Join(link, "new-child")})
	called := false
	err := a.withPhase3Evidence(func() error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "独立目录") || called {
		t.Fatalf("symlink output accepted: %v", err)
	}
	if _, err := readEvidenceRetention(dir); err != nil {
		t.Fatalf("symlink target marker changed: %v", err)
	}
}

func TestEvidenceRetentionExpiresOnlyCompletedManagedDirs(t *testing.T) {
	root := isolatedEvidenceTemp(t)
	now := time.Now()
	old := evidenceRetentionFixture(t, root, "old", now.Add(-8*24*time.Hour), 1)
	fresh := evidenceRetentionFixture(t, root, "fresh", now.Add(-time.Hour), 1)
	active := evidenceRetentionFixture(t, root, "active", time.Time{}, 1)
	legacy := filepath.Join(root, "shimo-phase3-legacy")
	if err := os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	requested := filepath.Join(root, "phase3-requested")
	if err := os.Mkdir(requested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := pruneManagedEvidence(now, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old evidence not pruned: %v", err)
	}
	for _, dir := range []string{fresh, active, legacy, requested} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("protected directory deleted %s: %v", dir, err)
		}
	}
}

func TestEvidenceRetentionCapacityOldestFirst(t *testing.T) {
	root := isolatedEvidenceTemp(t)
	now := time.Now()
	old := evidenceRetentionFixture(t, root, "old", now.Add(-2*time.Hour), 1000)
	fresh := evidenceRetentionFixture(t, root, "fresh", now.Add(-time.Hour), 1000)
	if err := pruneEvidenceAt(root, now, evidenceMaxAge, 1500, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("oldest evidence not pruned: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceRetentionNeverFollowsSymlinks(t *testing.T) {
	root := isolatedEvidenceTemp(t)
	now := time.Now()
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "precious"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	old := evidenceRetentionFixture(t, root, "old", now.Add(-8*24*time.Hour), 1)
	if err := os.Symlink(external, filepath.Join(old, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(root, "shimo-phase3-link")); err != nil {
		t.Fatal(err)
	}
	markerLink := filepath.Join(root, "shimo-phase3-marker-link")
	if err := os.Mkdir(markerLink, 0700); err != nil {
		t.Fatal(err)
	}
	if err := phase3WriteEvidence(filepath.Join(external, "marker"), evidenceRetention{Owner: "shimo-net/evidence-v1", CompletedAt: now.Add(-8 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(external, "marker"), filepath.Join(markerLink, evidenceMarker)); err != nil {
		t.Fatal(err)
	}
	if err := pruneManagedEvidence(now, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(external, "precious"), markerLink, filepath.Join(root, "shimo-phase3-link")} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("symlink boundary lost %s: %v", path, err)
		}
	}
}

func TestEvidenceRetentionCurrentFailureSurvivesUntilNextOperation(t *testing.T) {
	root := isolatedEvidenceTemp(t)
	dir, err := newManagedEvidenceDir("shimo-phase3-")
	if err != nil {
		t.Fatal(err)
	}
	if err := retainManagedEvidence(dir); err != nil {
		t.Fatal(err)
	}
	if err := pruneEvidenceAt(root, time.Now().Add(8*24*time.Hour), evidenceMaxAge, 0, []string{dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	if err := pruneEvidenceAt(root, time.Now().Add(8*24*time.Hour), evidenceMaxAge, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("unprotected expired evidence not removed: %v", err)
	}
}

func TestEvidenceRetentionPrunesOnceBeforeCommandStarts(t *testing.T) {
	root := isolatedEvidenceTemp(t)
	now := time.Now()
	old := evidenceRetentionFixture(t, root, "previous-command", now.Add(-8*24*time.Hour), 1)
	a := newApp([]string{"phase3"})
	err := a.withPhase3Evidence(func() error {
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Fatalf("command did not prune old evidence: %v", err)
		}
		// Simulate earlier closed-browser diagnostics during a long command.
		current := evidenceRetentionFixture(t, root, "current-command", now.Add(-8*24*time.Hour), 1)
		if _, err := newManagedEvidenceDir("shimo-phase3-startup-"); err != nil {
			return err
		}
		if _, err := os.Stat(current); err != nil {
			t.Fatalf("allocation pruned current diagnostics: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceOutputRejectsLexicalReservedAncestorThroughEscapingSymlink(t *testing.T) {
	root := isolatedEvidenceTemp(t)
	dir := evidenceRetentionFixture(t, root, "source", time.Now().Add(-8*24*time.Hour), 1)
	external := t.TempDir()
	link := filepath.Join(dir, "outside")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	parent := filepath.Join(link, "requested-output")
	a := newApp([]string{"phase3", "--out", parent})
	called := false
	err := a.withPhase3Evidence(func() error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "独立目录") || called {
		t.Fatalf("lexically reserved output accepted: %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("link deleted on validation failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(external, "requested-output")); !os.IsNotExist(err) {
		t.Fatalf("external output created: %v", err)
	}
}
