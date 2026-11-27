package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentOriginalReportRecorded(t *testing.T) {
	root, out := os.Getenv("SHIMO_DOCUMENT_REPORT_EVIDENCE"), os.Getenv("SHIMO_DOCUMENT_REPORT_OUT")
	if root == "" || out == "" {
		t.Skip("explicit original report exports")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	reports := []map[string]any{}
	for _, name := range []string{"d3-document-page", "d4-document-theme", "d5-document-replace", "d5b-document-replace", "d5c-document-replace", "d7-document-mention", "d7b-document-mention"} {
		files, _ := filepath.Glob(filepath.Join(root, name, "evi", "phase3-*", "request.json"))
		if len(files) != 1 {
			t.Fatal("unique original request required", name)
		}
		raw, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatal(err)
		}
		var req struct {
			Action string          `json:"action"`
			Spec   json.RawMessage `json:"spec"`
		}
		if err = json.Unmarshal(raw, &req); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Dir(files[0])
		report := map[string]any{"case": name, "request": files[0], "readOnlyReverification": true, "originalResultUnchanged": true}
		read := func(stage string) documentLayoutSnapshot {
			t.Helper()
			files, _ := filepath.Glob(filepath.Join(dir, stage, "*.docx"))
			if len(files) != 1 {
				t.Fatal("unique original DOCX required", name, stage)
			}
			s, e := readDocumentLayout(files[0])
			if e != nil {
				t.Fatal(e)
			}
			return s
		}
		before := read("before")
		var proof error
		switch name {
		case "d4-document-theme":
			if before.Theme != nil {
				t.Fatal("expected original fixture without theme")
			}
			report["verified"] = false
			report["reason"] = "original DOCX lacks theme; guard correctly rejected before mutation"
		case "d5b-document-replace":
			report["verified"] = false
			report["reason"] = "only before export retained; original range check rejected a paragraph with trailing mention; no original after-state proof"
		case "d3-document-page":
			var spec documentLayoutSpec
			if err = json.Unmarshal(req.Spec, &spec); err != nil {
				t.Fatal(err)
			}
			proof = verifyDocumentLayout(before, read("after-4"), spec)
			if proof == nil {
				t.Fatal("original missing header/footer and changed columns accepted")
			}
		case "d5-document-replace", "d5c-document-replace":
			var spec documentReplaceSpec
			if err = json.Unmarshal(req.Spec, &spec); err != nil {
				t.Fatal(err)
			}
			stage := "after-4"
			if name == "d5c-document-replace" {
				stage = "reload"
			}
			proof = verifyDocumentReplace(before, read(stage), spec)
			if (proof == nil) != (name == "d5c-document-replace") {
				t.Fatalf("unexpected original replace verdict: %s %v", name, proof)
			}
		default:
			var spec documentFormatSpec
			if err = json.Unmarshal(req.Spec, &spec); err != nil {
				t.Fatal(err)
			}
			after := read("after")
			label := strings.TrimPrefix(after.Paragraphs[spec.Paragraph-1].Text, spec.Text+"@")
			proof = verifyDocumentMention(before.Text, after.Text, spec, label)
			if proof != nil {
				t.Fatal(name, proof)
			}
			report["scope"] = "DOCX text/format preservation only; native user identity requires separate readback"
		}
		if _, set := report["verified"]; !set {
			report["verified"] = proof == nil
		}
		if proof != nil {
			report["error"] = proof.Error()
		}
		reports = append(reports, report)
	}
	if err := phase3WriteEvidence(filepath.Join(out, "original-report-readonly.json"), reports); err != nil {
		t.Fatal(err)
	}
}
