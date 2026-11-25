package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// startHarvestBrowser passes Chrome switches, so the helper must run before the
// Go test flag parser. The opt-in variable is set only for these child processes.
func init() {
	mode := os.Getenv("SHIMO_TEST_HARVEST_LOG_HELPER")
	if mode == "" {
		return
	}
	fmt.Fprintln(os.Stderr, "sandbox initialization failed")
	if mode == "ready" {
		for _, arg := range os.Args[1:] {
			if strings.HasPrefix(arg, "--user-data-dir=") {
				if err := os.WriteFile(filepath.Join(strings.TrimPrefix(arg, "--user-data-dir="), "DevToolsActivePort"), []byte("12345\n"), 0600); err != nil {
					os.Exit(2)
				}
			}
		}
	}
	os.Exit(0)
}

func TestEditorCloseReleasesBrowserLog(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(fmt.Sprint(started), func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestBrowserLogHelperProcess")
			cmd.Env = append(os.Environ(), "SHIMO_BROWSER_LOG_HELPER=1")
			path, cleanup := captureBrowserOutput(cmd)
			t.Cleanup(cleanup)
			f, ok := cmd.Stdout.(*os.File)
			if !ok {
				t.Fatal("browser log was not created")
			}
			b := &phase3EditorBrowser{cmd: cmd, logPath: path, cleanupLog: cleanup, profile: t.TempDir()}
			if started {
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
			}
			b.Close()
			b.Close()
			if _, err := f.WriteString("still open"); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("log descriptor not closed: %v", err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("log not removed: %v", err)
			}
		})
	}
}

func TestHarvestBrowserLogLifecycle(t *testing.T) {
	for _, mode := range []string{"ready", "no-port", "start-failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_TEST_HARVEST_LOG_HELPER", mode)
			binary := os.Args[0]
			if mode == "start-failure" {
				binary = filepath.Join(t.TempDir(), "missing-browser")
			}
			cmd, port, path, cleanup, err := startHarvestBrowser(binary, t.TempDir(), false)
			defer cleanup()
			if mode == "ready" {
				if err != nil || port != 12345 || cmd == nil {
					t.Fatalf("helper failed to start: %d %v", port, err)
				}
				_ = cmd.Wait()
				f := cmd.Stdout.(*os.File)
				cleanup()
				if _, err := f.WriteString("still open"); !errors.Is(err, os.ErrClosed) {
					t.Fatalf("harvest log descriptor not closed: %v", err)
				}
			} else {
				if err == nil || cmd != nil {
					t.Fatal("expected failed launch")
				}
				if mode == "no-port" {
					if !strings.Contains(explainBrowserFailure(err, path).Error(), "沙箱") {
						t.Fatal("diagnostic log removed before failure explanation")
					}
				}
				cleanup()
			}
			cleanup()
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("harvest log not removed: %v", err)
			}
		})
	}
}
