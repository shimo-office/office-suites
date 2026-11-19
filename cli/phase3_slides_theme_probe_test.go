package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPhase3SlidesInspectFailedThemeReadOnly(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_SLIDES_INSPECT_THEME")
	if source == "" {
		t.Skip("opt-in read-only failed theme inspection")
	}
	var request struct {
		Spec phase3SlidesSpec
		URL  string
	}
	raw, err := os.ReadFile(filepath.Join(source, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &request); err != nil || request.Spec.Theme == nil {
		t.Fatal("invalid theme request", err)
	}
	raw, err = os.ReadFile(filepath.Join(source, "theme-expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, request.Spec.Theme); err != nil {
		t.Fatal(err)
	}
	b, err := openPhase3SlidesForegroundProbe(request.URL+"?dbg=1", "theme-inspection")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	dir := filepath.Join(phase3SlidesProbeRunRoot, "theme-inspection")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ids, err := phase3SlidesRead(b)
	if err != nil {
		t.Fatal(err)
	}
	native, err := b.Eval(phase3SlideThemeReloadJS())
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "reload-theme.json"), native); err != nil {
		t.Fatal(err)
	}
	design, err := b.Eval(phase3SlideDesignJS())
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "design.json"), design); err != nil {
		t.Fatal(err)
	}
	a := &App{Origin: "https://shimo.net", Cmd: "phase3", JSON: true}
	fresh, err := a.phase3OfficeExport(guidFromURL(request.URL), filepath.Join(dir, "fresh"), "pptx")
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3SlideCaptureBatch(b, dir, "reload", ids, request.Spec); err != nil {
		t.Fatal(err)
	}
	proof := map[string]any{"mutationAttempted": false, "originalFailedCommandEvidence": source, "freshArtifact": fresh, "ids": ids}
	if e := phase3VerifySlideThemeNative(native, ids, *request.Spec.Theme); e != nil {
		proof["nativeError"] = e.Error()
	} else {
		proof["nativeVerified"] = true
	}
	m, _ := native.(map[string]any)
	if e := phase3VerifyThemeDelta(m["delta"], *request.Spec.Theme); e != nil {
		proof["deltaError"] = e.Error()
	} else {
		proof["deltaVerified"] = true
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "inspection.json"), proof); err != nil {
		t.Fatal(err)
	}
}
