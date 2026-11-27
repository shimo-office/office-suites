package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Recheck retained exports only; never replay a write that already applied.
func TestDocumentLayoutScreenshotRecorded(t *testing.T) {
	root := os.Getenv("SHIMO_SCREENSHOT_DOCUMENT_EVIDENCE")
	if root == "" {
		t.Skip("explicit local screenshot evidence")
	}
	for _, name := range []string{"doc-para", "doc-page", "doc-theme"} {
		t.Run(name, func(t *testing.T) {
			files, err := filepath.Glob(filepath.Join(root, name, "phase3-*", "request.json"))
			if err != nil || len(files) != 1 {
				t.Fatal("unique request required", err)
			}
			dir := filepath.Dir(files[0])
			raw, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatal(err)
			}
			var request struct {
				Spec documentLayoutSpec `json:"spec"`
			}
			if err = json.Unmarshal(raw, &request); err != nil {
				t.Fatal(err)
			}
			read := func(stage string) documentLayoutSnapshot {
				t.Helper()
				paths, err := filepath.Glob(filepath.Join(dir, stage, "*.docx"))
				if err != nil || len(paths) != 1 {
					t.Fatal("unique DOCX required", err)
				}
				s, err := readDocumentLayout(paths[0])
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
			before, after := read("before"), read("after-4")
			proof := verifyDocumentLayout(before, after, request.Spec)
			report := map[string]any{"readOnlyReverification": true, "originalResultUnchanged": true, "verified": proof == nil, "diff": documentLayoutDiff(before, after)}
			if proof != nil {
				report["error"] = proof.Error()
			}
			if proof != nil {
				t.Fatal(proof)
			}
			t.Logf("%s: %v", name, proof)
			if err := phase3WriteEvidence(filepath.Join(dir, "current-readonly-proof.json"), report); err != nil {
				t.Fatal(err)
			}
		})
	}
}
