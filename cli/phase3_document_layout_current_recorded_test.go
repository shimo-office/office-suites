package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Revalidate every successful current fixture using the current verifier. No
// editor is opened and original failed runs remain unchanged.
func TestDocumentScreenshotCurrentRecorded(t *testing.T) {
	root := os.Getenv("SHIMO_SCREENSHOT_CURRENT_DOCUMENT")
	if root == "" {
		t.Skip("explicit current document fixture exports")
	}
	files, _ := filepath.Glob(filepath.Join(root, "evidence-*", "phase3-*", "result.json"))
	if len(files) < 6 {
		t.Fatal("six successful current fixture results required")
	}
	for _, file := range files {
		t.Run(filepath.Base(filepath.Dir(filepath.Dir(file))), func(t *testing.T) {
			dir := filepath.Dir(file)
			raw, err := os.ReadFile(filepath.Join(dir, "request.json"))
			if err != nil {
				t.Fatal(err)
			}
			var request struct {
				Action string          `json:"action"`
				Spec   json.RawMessage `json:"spec"`
			}
			if err = json.Unmarshal(raw, &request); err != nil {
				t.Fatal(err)
			}
			read := func(stage string) documentLayoutSnapshot {
				t.Helper()
				paths, _ := filepath.Glob(filepath.Join(dir, stage, "*.docx"))
				if len(paths) != 1 {
					t.Fatal("unique DOCX required", stage)
				}
				s, err := readDocumentLayout(paths[0])
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
			before := read("before")
			if raw, err := os.ReadFile(filepath.Join(dir, "native-page-margins-before.json")); err == nil {
				var values map[string]any
				if err = json.Unmarshal(raw, &values); err != nil {
					t.Fatal(err)
				}
				before, err = documentWithNativeMargins(before, values)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, stage := range []string{"after-0", "reload"} {
				after := read(stage)
				if request.Action == "document-replace" {
					var spec documentReplaceSpec
					if err = json.Unmarshal(request.Spec, &spec); err != nil {
						t.Fatal(err)
					}
					err = verifyDocumentReplace(before, after, spec)
				} else {
					var spec documentLayoutSpec
					if err = json.Unmarshal(request.Spec, &spec); err != nil {
						t.Fatal(err)
					}
					err = verifyDocumentLayout(before, after, spec)
				}
				if err != nil {
					t.Fatal(stage, err)
				}
			}
		})
	}
}
