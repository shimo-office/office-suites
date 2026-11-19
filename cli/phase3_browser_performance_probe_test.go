package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Explicit, read-only diagnostic on a dedicated acceptance fixture. The
// independent browser session cannot issue editor actions or resume JavaScript.
func TestPhase3BrowserNativeCPUProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_NATIVE_CPU_PROBE") != "1" {
		t.Skip("opt-in dedicated Chrome renderer CPU diagnostic")
	}
	dir := os.Getenv("SHIMO_PHASE3_NATIVE_CPU_DIR")
	if dir == "" {
		t.Fatal("unique evidence directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	done := make(chan struct{})
	var timer *time.Timer
	var b *phase3EditorBrowser
	defer func() {
		if timer != nil && !timer.Stop() {
			<-done
		}
		if b != nil {
			b.Close()
		}
	}()
	var err error
	b, err = openPhase3EditorWithSetup("https://shimo.net/presentation/GWxPRXPanLszabWj?dbg=1",
		`Boolean(typeof window.presentationSDK?.API?.slides?.getSlides==='function'&&window.presentationSDK.editor.workbench.isReady())`,
		func(b *phase3EditorBrowser) error {
			profile := b.profile
			timer = time.AfterFunc(20*time.Second, func() {
				defer close(done)
				phase3SampleTaskRenderer(profile, dir)
			})
			return nil
		})
	result := map[string]any{"mutationAttempted": false, "ready": err == nil, "seconds": time.Since(start).Seconds()}
	if err != nil {
		result["error"] = err.Error()
	} else {
		if err = b.Capture(filepath.Join(dir, "ready.png")); err != nil {
			result["screenshotError"] = err.Error()
		}
	}
	if writeErr := phase3WriteEvidence(filepath.Join(dir, "result.json"), result); writeErr != nil {
		t.Fatal(writeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func phase3SampleTaskRenderer(profile, dir string) {
	record := map[string]any{"at": time.Now().UTC(), "mutationAttempted": false}
	defer func() { _ = phase3WriteEvidence(filepath.Join(dir, "native-diagnostic.json"), record) }()
	ports, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	if err != nil {
		record["error"] = err.Error()
		return
	}
	parts := strings.Fields(string(ports))
	if len(parts) < 2 || !strings.HasPrefix(parts[1], "/devtools/browser/") {
		record["error"] = "missing browser target"
		return
	}
	ws, err := dialWSWithTimeout("ws://127.0.0.1:"+parts[0]+parts[1], 3*time.Second)
	if err != nil {
		record["error"] = err.Error()
		return
	}
	defer ws.Close()
	processes, err := ws.Call(1, "SystemInfo.getProcessInfo", nil, 3*time.Second)
	if err != nil {
		record["error"] = err.Error()
		return
	}
	record["processes"] = processes
	list, _ := processes["processInfo"].([]any)
	var chosen map[string]any
	for _, raw := range list {
		p, _ := raw.(map[string]any)
		if str(p, "type") == "renderer" && (chosen == nil || num(p, "cpuTime") > num(chosen, "cpuTime")) {
			chosen = p
		}
	}
	if chosen == nil || num(chosen, "id") <= 0 {
		record["error"] = "no task renderer"
		return
	}
	record["sampledRenderer"] = chosen
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/sample", strconv.Itoa(int(num(chosen, "id"))), "3", "1", "-file", filepath.Join(dir, "renderer.sample.txt"))
	output, err := cmd.CombinedOutput()
	record["sampleLog"] = string(output)
	if err != nil {
		record["error"] = fmt.Sprintf("native sample: %v", err)
	}
}
