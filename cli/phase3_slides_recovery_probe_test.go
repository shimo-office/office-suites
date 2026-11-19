package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// After an uncertain/failed postcondition, this opt-in test only reads the
// original artifacts, reopens the file, and exports fresh server state. It never
// calls a mutation or converts the original command's failed exit into success.
func TestPhase3SlidesRecoverPlaceholderReadOnly(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_SLIDES_RECOVER_PLACEHOLDER")
	if source == "" {
		t.Skip("opt-in read-only recovery of an already submitted placeholder")
	}
	var request struct {
		Spec phase3SlidesSpec
		URL  string
	}
	raw, err := os.ReadFile(filepath.Join(source, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	if request.Spec.Placeholder == nil || request.Spec.Action != "slide-placeholder-fill" {
		t.Fatal("recovery only supports recorded typed placeholder fills")
	}
	var pages []phase3SlidePage
	raw, err = os.ReadFile(filepath.Join(source, "before-readback.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &pages); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, p := range pages {
		ids = append(ids, p.ID)
	}
	before, err := filepath.Glob(filepath.Join(source, "before", "*.pptx"))
	if err != nil || len(before) != 1 {
		t.Fatal("missing unique original baseline", err)
	}
	b, err := openPhase3SlidesForegroundProbe(request.URL+"?dbg=1", "placeholder-recovery")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	dir := filepath.Join(phase3SlidesProbeRunRoot, "placeholder-recovery")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	current, err := phase3SlidesRead(b)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(request.Spec)
	state, err := b.Eval(`(()=>{const r=` + string(raw) + `,api=window.presentationSDK.API.slides;const page=api.getSlides().find(x=>x.id===r.slideId);const shapes=page?.getShapes().filter(x=>x.name===r.placeholder.shapeName);if(shapes?.length!==1||shapes[0].textContent!==r.placeholder.text)return {ok:false,error:'reopened placeholder content is not the requested exact text'};return {ok:true,slideId:page.id,shapeId:shapes[0].id,name:shapes[0].name,text:shapes[0].textContent}})()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "reload-readback.json"), state); err != nil {
		t.Fatal(err)
	}
	a := &App{Origin: "https://shimo.net", Cmd: "phase3", JSON: true}
	fresh, err := a.phase3OfficeExport(guidFromURL(request.URL), filepath.Join(dir, "fresh"), "pptx")
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyPhase3SlidePlaceholderArtifacts(before[0], fresh.Path, ids, current, request.Spec); err != nil {
		t.Fatal(err)
	}
	if err = phase3SlidesSelect(b, request.Spec.SlideID); err != nil {
		t.Fatal(err)
	}
	if err = phase3SlideScreenshot(b, filepath.Join(dir, "reload.png")); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "recovered-verification.json"), map[string]any{"verified": true, "mutationAttempted": false, "originalFailedCommandEvidence": source, "originalBeforeArtifact": before[0], "freshArtifact": fresh, "request": request, "reopened": state, "visualReviewRequired": true}); err != nil {
		t.Fatal(err)
	}
	t.Log("read-only recovered verification saved", dir)
}

func TestPhase3SlidesInspectFailedFormatReadOnly(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_SLIDES_INSPECT_FORMAT")
	if source == "" {
		t.Skip("opt-in read-only inspection of a failed format command")
	}
	var request struct {
		Spec phase3SlidesSpec
		URL  string
	}
	raw, err := os.ReadFile(filepath.Join(source, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &request); err != nil || request.Spec.Format == nil {
		t.Fatal("invalid recorded format request", err)
	}
	b, err := openPhase3SlidesForegroundProbe(request.URL+"?dbg=1", "format-inspection")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	dir := filepath.Join(phase3SlidesProbeRunRoot, "format-inspection")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	state, err := b.Eval(`(()=>{const wb=window.presentationSDK.editor.workbench,e=wb.editor,u=window.EditorUtil,active=e._$collaborationManager$_?._$changesCollector$_?._$changesStack$_;return {ok:true,globalHistoryHasPresentation:!!u?._$ChangesStack$_?.presentation,globalHistoryMatchesActive:u?._$ChangesStack$_===active,activeHistoryMatchesPresentation:active?.presentation===e.presentation,activeHistoryCreateType:typeof active?._$createNewAction$_,textPrConstructorType:typeof e.presentation._$getCalcedTextPr$_?.()?.constructor,slides:window.presentationSDK.API.slides.getSlides().map(s=>({id:s.id,shapes:s.getShapes().map(x=>({id:x.id,name:x.name,text:x.textContent}))}))}})()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "native-state.json"), state); err != nil {
		t.Fatal(err)
	}
	a := &App{Origin: "https://shimo.net", Cmd: "phase3", JSON: true}
	fresh, err := a.phase3OfficeExport(guidFromURL(request.URL), filepath.Join(dir, "fresh"), "pptx")
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3SlidesSelect(b, request.Spec.SlideID); err != nil {
		t.Fatal(err)
	}
	if err = phase3SlideScreenshot(b, filepath.Join(dir, "reload.png")); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "inspection.json"), map[string]any{"originalFailedCommandEvidence": source, "freshArtifact": fresh, "mutationAttempted": false, "status": "inspection-only", "request": request}); err != nil {
		t.Fatal(err)
	}
}

func TestPhase3SlidesRecoverShapeReadOnly(t *testing.T) {
	phase3RecoverRecordedSlideAction(t, "SHAPE", "shape-recovery")
}

func TestPhase3SlidesRecoverTableReadOnly(t *testing.T) {
	phase3RecoverRecordedSlideAction(t, "TABLE", "table-recovery")
}

func TestPhase3SlidesRecoverReplaceReadOnly(t *testing.T) {
	phase3RecoverRecordedSlideAction(t, "REPLACE", "replace-recovery")
}

func phase3RecoverRecordedSlideAction(t *testing.T, kind, label string) {
	t.Helper()
	source := os.Getenv("SHIMO_PHASE3_SLIDES_RECOVER_" + kind)
	if source == "" {
		t.Skip("opt-in recovery of an already applied shape edit")
	}
	var request struct {
		Spec phase3SlidesSpec
		URL  string
	}
	raw, err := os.ReadFile(filepath.Join(source, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &request); err != nil || (kind == "SHAPE" && request.Spec.Shape == nil) || (kind == "TABLE" && request.Spec.Table == nil) || (kind == "REPLACE" && request.Spec.Replace == nil) {
		t.Fatal("invalid recorded shape request", err)
	}
	var pages []phase3SlidePage
	raw, err = os.ReadFile(filepath.Join(source, "before-readback.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &pages); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, p := range pages {
		ids = append(ids, p.ID)
	}
	before, err := filepath.Glob(filepath.Join(source, "before", "*.pptx"))
	if err != nil || len(before) != 1 {
		t.Fatal("missing unique original baseline", err)
	}
	prepare := phase3PrepareSlideShape
	verify := verifyPhase3SlideShapeArtifacts
	if kind == "TABLE" {
		prepare = phase3PrepareSlideTable
		verify = verifyPhase3SlideTableArtifacts
	} else if kind == "REPLACE" {
		prepare = phase3PrepareSlideReplace
		verify = verifyPhase3SlideReplaceArtifacts
	}
	if err = prepare(before[0], ids, &request.Spec); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SHIMO_PHASE3_SLIDES_"+kind+"_OFFLINE") == "1" {
		after, err := filepath.Glob(filepath.Join(source, "after-3", "*.pptx"))
		if exact := os.Getenv("SHIMO_PHASE3_SLIDES_" + kind + "_OFFLINE_ARTIFACT"); exact != "" {
			after = []string{exact}
			err = nil
		}
		if err != nil || len(after) != 1 {
			t.Fatal("missing recorded after artifact", err)
		}
		if err = verify(before[0], after[0], ids, ids, request.Spec); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := openPhase3SlidesForegroundProbe(request.URL+"?dbg=1", label)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	dir := filepath.Join(phase3SlidesProbeRunRoot, label)
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	current, err := phase3SlidesRead(b)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{Origin: "https://shimo.net", Cmd: "phase3", JSON: true}
	fresh, err := a.phase3OfficeExport(guidFromURL(request.URL), filepath.Join(dir, "fresh"), "pptx")
	if err != nil {
		t.Fatal(err)
	}
	slideID := request.Spec.SlideID
	if kind == "REPLACE" && len(current) > 0 {
		slideID = current[0]
	}
	if err = phase3SlidesSelect(b, slideID); err != nil {
		t.Fatal(err)
	}
	if err = phase3SlideScreenshot(b, filepath.Join(dir, "reload.png")); err != nil {
		t.Fatal(err)
	}
	if err = verify(before[0], fresh.Path, ids, current, request.Spec); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "recovered-verification.json"), map[string]any{"verified": true, "mutationAttempted": false, "originalFailedCommandEvidence": source, "originalBeforeArtifact": before[0], "freshArtifact": fresh, "request": request, "reopenedSlideIds": current, "actionKind": kind, "visualReviewRequired": true}); err != nil {
		t.Fatal(err)
	}
}
