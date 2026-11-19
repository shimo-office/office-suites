package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Capture all pages after a completed layout action without invoking setters.
func TestPhase3SlidesFinalLayoutReadOnly(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_SLIDES_FINAL_LAYOUT")
	if source == "" {
		t.Skip("opt-in final layout evidence")
	}
	var request struct {
		Spec phase3SlidesSpec
		URL  string
	}
	raw, e := os.ReadFile(filepath.Join(source, "request.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &request); e != nil || request.Spec.Layout == nil {
		t.Fatal("invalid layout request", e)
	}
	raw, e = os.ReadFile(filepath.Join(source, "layout-expected.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, request.Spec.Layout); e != nil {
		t.Fatal(e)
	}
	var pages []phase3SlidePage
	raw, e = os.ReadFile(filepath.Join(source, "before-readback.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &pages); e != nil {
		t.Fatal(e)
	}
	ids := []string{}
	for _, p := range pages {
		ids = append(ids, p.ID)
	}
	b, e := openPhase3SlidesForegroundProbe(request.URL+"?dbg=1", "final-layout")
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	current, e := phase3SlidesRead(b)
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(phase3SlidesProbeRunRoot, "final-layout")
	if e = os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	design, e := b.Eval(phase3SlideDesignJS())
	if e != nil {
		t.Fatal(e)
	}
	if e = phase3WriteEvidence(filepath.Join(dir, "design.json"), design); e != nil {
		t.Fatal(e)
	}
	if e = phase3VerifyReloadedLayout(b, request.Spec); e != nil {
		t.Fatal(e)
	}
	app := &App{Origin: "https://shimo.net", Cmd: "phase3", JSON: true}
	fresh, e := app.phase3OfficeExport(guidFromURL(request.URL), filepath.Join(dir, "fresh"), "pptx")
	if e != nil {
		t.Fatal(e)
	}
	prior, e := filepath.Glob(filepath.Join(source, "before", "*.pptx"))
	if e != nil || len(prior) != 1 {
		t.Fatal(e)
	}
	before, e := readPhase3SlideArtifact(prior[0], ids)
	if e != nil {
		t.Fatal(e)
	}
	after, e := readPhase3SlideArtifact(fresh.Path, current)
	if e != nil {
		t.Fatal(e)
	}
	if e = verifyPhase3SlideLayout(before, after, request.Spec); e != nil {
		t.Fatal(e)
	}
	for i, id := range current {
		if e = phase3SlidesSelect(b, id); e != nil {
			t.Fatal(e)
		}
		if _, e = b.Eval(`new Promise(resolve=>setTimeout(()=>resolve({ok:true}),1500))`); e != nil {
			t.Fatal(e)
		}
		if e = phase3SlideScreenshot(b, filepath.Join(dir, fmt.Sprintf("page-%d.png", i+1))); e != nil {
			t.Fatal(e)
		}
	}
	if e = phase3WriteEvidence(filepath.Join(dir, "verification.json"), map[string]any{"ok": true, "mutationAttempted": false, "originalCommand": source, "before": before, "after": after, "freshArtifact": fresh, "nativeLayoutVerified": true, "nonTargetPagesUnchanged": true}); e != nil {
		t.Fatal(e)
	}
}
