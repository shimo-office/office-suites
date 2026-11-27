package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Replays saved local artifacts only; never opens a browser or replays mutations.
func TestPhase3SlidesProductReportArtifacts(t *testing.T) {
	root := os.Getenv("SHIMO_SLIDES_PRODUCT_REPORT")
	if root == "" {
		t.Skip("opt-in local product-report evidence replay")
	}
	for _, id := range []string{"SL01", "SL02", "SM01", "SM03", "SM05", "SL05", "SN01"} {
		t.Run(id, func(t *testing.T) {
			dirs, e := filepath.Glob(filepath.Join(root, id, "phase3-*"))
			if e != nil || len(dirs) != 1 {
				t.Fatal("expected evidence directory", e)
			}
			d := dirs[0]
			read := func(name string, v any) {
				t.Helper()
				raw, e := os.ReadFile(filepath.Join(d, name))
				if e != nil {
					t.Fatal(e)
				}
				if e = json.Unmarshal(raw, v); e != nil {
					t.Fatal(e)
				}
			}
			var req struct{ Spec phase3SlidesSpec }
			read("request.json", &req)
			var b, a []phase3SlidePage
			read("before-readback.json", &b)
			var actual struct {
				IDs []string `json:"ids"`
			}
			read("after-readback.json", &actual)
			ids, next := []string{}, []string{}
			for _, p := range b {
				ids = append(ids, p.ID)
			}
			next = actual.IDs
			before, _ := filepath.Glob(filepath.Join(d, "before", "*.pptx"))
			after, _ := filepath.Glob(filepath.Join(d, "after-*", "*.pptx"))
			if len(before) != 1 || len(after) == 0 {
				t.Fatal("missing artifacts")
			}
			bp, ap := before[0], after[len(after)-1]
			switch req.Spec.Action {
			case "slide-text-format":
				e = verifyPhase3SlideTextFormatArtifacts(bp, ap, ids, next, req.Spec)
			case "slide-shape-format":
				read("shape-expected.json", &req.Spec.Shape)
				e = verifyPhase3SlideShapeArtifacts(bp, ap, ids, next, req.Spec)
			case "slide-table-edit":
				read("table-expected.json", &req.Spec.Table)
				e = verifyPhase3SlideTableArtifacts(bp, ap, ids, next, req.Spec)
			case "slide-placeholder-fill":
				e = verifyPhase3SlidePlaceholderArtifacts(bp, ap, ids, next, req.Spec)
			case "slide-theme":
				read("theme-expected.json", &req.Spec.Theme)
				e = verifyPhase3SlideThemeArtifacts(bp, ap, ids, next, req.Spec)
			default:
				b, e = readPhase3SlideArtifact(bp, ids)
				if e == nil {
					a, e = readPhase3SlideArtifact(ap, next)
				}
				if e == nil {
					e = verifyPhase3Slides(b, a, req.Spec)
				}
			}
			if e != nil {
				t.Fatal(e)
			}
		})
	}
}
