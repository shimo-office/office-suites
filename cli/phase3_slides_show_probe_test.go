package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPhase3SlidesInspectFailedShowReadOnly(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_SLIDES_INSPECT_SHOW")
	if source == "" {
		t.Skip("opt-in read-only failed show settings inspection")
	}
	var request struct {
		Spec phase3SlidesSpec
		URL  string
	}
	raw, err := os.ReadFile(filepath.Join(source, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &request); err != nil || request.Spec.Show == nil {
		t.Fatal("invalid show request", err)
	}
	b, err := openPhase3SlidesForegroundProbe(request.URL+"?dbg=1", "show-inspection")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	dir := filepath.Join(phase3SlidesProbeRunRoot, "show-inspection")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	state, err := b.Eval(phase3SlideShowReadJS())
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "native-settings.json"), state); err != nil {
		t.Fatal(err)
	}
	io, err := b.Eval(`(()=>{const e=window.presentationSDK.editor.workbench.editor;const operations=e._$ioAdapter$_.fromOperations(e._$collaborationManager$_._$currentPptIOData$_()).toJSON();return {ok:true,target:location.origin+location.pathname,nativeShowPr:e.presentation.showPr?.toJSON?.()??null,presentation:operations.filter(x=>x.attributes?.opType==='presentation')};})()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "loaded-native-io.json"), io); err != nil {
		t.Fatal(err)
	}
	ids, err := phase3SlidesRead(b)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{Origin: "https://shimo.net", Cmd: "phase3", JSON: true}
	fresh, err := a.phase3OfficeExport(guidFromURL(request.URL), filepath.Join(dir, "fresh"), "pptx")
	if err != nil {
		t.Fatal(err)
	}
	before, err := filepath.Glob(filepath.Join(source, "before", "*.pptx"))
	if err != nil || len(before) != 1 {
		t.Fatal("missing baseline", err)
	}
	x, err := readPhase3SlideArtifact(before[0], ids)
	if err != nil {
		t.Fatal(err)
	}
	y, err := readPhase3SlideArtifact(fresh.Path, ids)
	if err != nil {
		t.Fatal(err)
	}
	old, err := phase3ReadShowProperties(before[0])
	if err != nil {
		t.Fatal(err)
	}
	next, err := phase3ReadShowProperties(fresh.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "inspection.json"), map[string]any{"mutationAttempted": false, "originalFailedCommandEvidence": source, "freshArtifact": fresh, "canonicalPackageUnchanged": reflect.DeepEqual(x, y), "showPropertiesUnchanged": reflect.DeepEqual(old, next), "nativeSettings": state, "oldShow": old, "newShow": next}); err != nil {
		t.Fatal(err)
	}
}

// Explicit target only: inspect current design IDs and authoritative native
// settings before selecting a new action; this probe never calls any setter.
func TestPhase3SlidesInspectDesignAndSettingsReadOnly(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_SLIDES_INSPECT_DESIGN") != "1" || os.Getenv("SHIMO_PHASE3_SLIDES_TARGET") == "" {
		t.Skip("opt-in explicit target design/settings inspection")
	}
	b, err := openPhase3SlidesForegroundProbe(phase3SlidesSyntheticTarget(), "design-settings")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	design, err := b.Eval(phase3SlideDesignJS())
	if err != nil {
		t.Fatal(err)
	}
	state, err := b.Eval(phase3SlideShowReadJS())
	if err != nil {
		t.Fatal(err)
	}
	m, ok := state.(map[string]any)
	if !ok {
		t.Fatal("invalid native settings")
	}
	if _, err = phase3NativeShowProperties(m); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(phase3SlidesProbeRunRoot, "design-settings")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "inspection.json"), map[string]any{"mutationAttempted": false, "design": design, "nativeSettings": state}); err != nil {
		t.Fatal(err)
	}
	if err = phase3SlideScreenshot(b, filepath.Join(dir, "current.png")); err != nil {
		t.Fatal(err)
	}
}
