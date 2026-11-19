package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReleaseDocumentNavigationVerifiesContentRemainsUnchanged(t *testing.T) {
	before := releaseDocumentPackage(t, `<w:p><w:r><w:t>Read-only page navigation</w:t></w:r></w:p><w:sectPr/>`)
	changed := releaseDocumentPackage(t, `<w:p><w:r><w:t>Accidental content edit</w:t></w:r></w:p><w:sectPr/>`)
	for _, fault := range []string{"control", "content-changed", "navigation-rejected"} {
		t.Run(fault, func(t *testing.T) {
			var navigations, exports atomic.Int32
			releaseDocumentHTTP(t, func() ([]byte, int) {
				exports.Add(1)
				if fault == "content-changed" && navigations.Load() > 0 {
					return changed, 200
				}
				return before, 200
			}, `{"type":"documentPro","name":"Synthetic"}`)
			old := phase3OpenDocumentEditor
			t.Cleanup(func() { phase3OpenDocumentEditor = old })
			phase3OpenDocumentEditor = func(target, readiness string) (*phase3EditorBrowser, error) {
				return releaseDocumentEditor(t, func(expr string) any {
					switch {
					case strings.Contains(expr, "a.controller.GoToPage("):
						navigations.Add(1)
						return map[string]any{"ok": fault != "navigation-rejected", "error": "synthetic calculation changed", "page": 2, "pageCount": 2}
					case expr == documentPageCalculationStateJS(2):
						return map[string]any{"ready": true, "totalPages": 2, "calculatedPages": 2}
					case expr == documentPageNavigationStateJS(2):
						return map[string]any{"verified": true, "page": 2, "pageCount": 2, "cursorPage": 2, "documentPage": 2, "footerPage": 2}
					default:
						t.Errorf("unexpected navigation expression %.100s", expr)
						return map[string]any{"ok": false}
					}
				}, nil, nil), nil
			}
			out := t.TempDir()
			a := newApp([]string{"phase3", "https://phase3.test/docx/abcdefgh", "--action", "document-goto", "--payload", `{"page":2}`, "--out", out, "--json"})
			a.Origin = "https://phase3.test"
			err := a.Run()
			if (err == nil) != (fault == "control") {
				t.Fatalf("wrong outcome: %v", err)
			}
			if navigations.Load() != 1 {
				t.Fatalf("navigation count %d", navigations.Load())
			}
			dir := releaseDocumentEvidence(t, out, fault == "control", false)
			if fault == "control" {
				b, e := os.ReadFile(filepath.Join(dir, "result.json"))
				if e != nil {
					t.Fatal(e)
				}
				var result map[string]any
				_ = json.Unmarshal(b, &result)
				if result["mutationAttempted"] != false || result["visiblePage"] != float64(2) || result["footerPage"] != float64(2) || exports.Load() != 2 {
					t.Fatalf("missing navigation proof: %s", b)
				}
			}
		})
	}
}
