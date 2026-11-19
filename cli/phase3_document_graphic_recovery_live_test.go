package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhase3DocumentGraphicReadOnlyRecovery(t *testing.T) {
	dir := os.Getenv("SHIMO_PHASE3_DOC_GRAPHIC_RECOVERY_OUT")
	if dir == "" {
		t.Skip("explicit private-fixture read-only recovery")
	}
	kind := os.Getenv("SHIMO_PHASE3_DOC_GRAPHIC_RECOVERY_KIND")
	original := ""
	switch kind {
	case "image":
		original = "../tmp/phase3-live/docx-slides/acceptance-driver/runs/image-position-size-rotation/20261118T203558002854Z/phase3-1276089360"
	case "direct":
		original = "../tmp/phase3-live/docx-slides/acceptance-driver/runs/native-stable-rectangle-fill-line/20261118T211755111238Z/phase3-2037369075"
	case "preset":
		original = "../tmp/phase3-live/docx-slides/acceptance-driver/runs/native-stable-preset/20261118T203838491136Z/phase3-3983194127"
	default:
		t.Fatal("only fixed original image/preset/direct failed runs may be recovered")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	requestBytes, err := os.ReadFile(filepath.Join(original, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Spec   documentGraphicSpec `json:"spec"`
		Target string              `json:"target"`
	}
	if err = json.Unmarshal(requestBytes, &request); err != nil {
		t.Fatal(err)
	}
	nativeBytes, err := os.ReadFile(filepath.Join(original, "before-native.json"))
	if err != nil {
		t.Fatal(err)
	}
	var nativeBefore struct {
		ObjectID string `json:"objectId"`
	}
	if err = json.Unmarshal(nativeBytes, &nativeBefore); err != nil || nativeBefore.ObjectID == "" {
		t.Fatal("original native ID unavailable", err)
	}
	paths, err := filepath.Glob(filepath.Join(original, "before", "*.docx"))
	if err != nil || len(paths) != 1 {
		t.Fatal("original baseline unavailable", err)
	}
	before, err := readDocumentGraphic(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := documentGraphicBaseline(before.Root, request.Spec)
	if err != nil {
		t.Fatal(err)
	}
	app := newApp([]string{"phase3"})
	b, err := openPhase3EditorWhen(request.Target, documentReadyJS)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	result := map[string]any{"originalFailedRun": original, "originalBeforeArtifact": paths[0], "mutationAttempted": false, "scope": "read-only recovery of original submitted graphic edit; original CLI failure preserved", "spec": request.Spec, "target": request.Target}
	for _, stage := range []string{"after", "reload"} {
		if stage == "reload" {
			if _, err = b.EvalReadOnly(`(()=>{window.__shimoGraphicRecoveryBeforeReload=true;return {ok:true}})()`); err != nil {
				t.Fatal(err)
			}
			if _, err = b.ws.Call(8000, "Page.reload", map[string]any{"ignoreCache": true}, 15*time.Second); err != nil {
				t.Fatal(err)
			}
			if err = b.WaitReady(`!window.__shimoGraphicRecoveryBeforeReload&&(`+documentReadyJS+`)`, phase3EditorReadyTimeout); err != nil {
				t.Fatal(err)
			}
		}
		if err = b.WaitDocumentEditable(); err != nil {
			t.Fatal(err)
		}
		if err = b.CheckPresence(); err != nil {
			t.Fatal(err)
		}
		native, err := b.EvalReadOnly(documentGraphicRuntimeJS(request.Spec, baseline, false, nativeBefore.ObjectID))
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-native.json"), native); err != nil {
			t.Fatal(err)
		}
		artifact, err := app.phase3OfficeExport(guidFromURL(request.Target), filepath.Join(dir, stage), "docx")
		if err != nil {
			t.Fatal(err)
		}
		if err = verifyDocumentGraphic(paths[0], artifact.Path, request.Spec); err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-assertions.json"), map[string]any{"verified": true, "spec": request.Spec, "originalBeforeArtifact": paths[0], "artifact": artifact, "nonTargetXMLAndAssetsPreserved": true}); err != nil {
			t.Fatal(err)
		}
		if err = b.Capture(filepath.Join(dir, stage+".png")); err != nil {
			t.Fatal(err)
		}
		result[stage+"Artifact"] = artifact
	}
	result["verified"] = true
	if err = phase3WriteEvidence(filepath.Join(dir, "recovery-result.json"), result); err != nil {
		t.Fatal(err)
	}
}
