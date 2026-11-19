package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReleaseDocumentStructuredActionsPersistAndRefuseRevertedExports(t *testing.T) {
	paragraph := func(props, text string) string {
		return `<w:p><w:pPr>` + props + `</w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
	}
	cell := func(props, text string) string {
		return `<w:tc><w:tcPr>` + props + `</w:tcPr><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:tc>`
	}
	table := func(rows string) string {
		return `<w:tbl><w:tblPr/><w:tblGrid><w:gridCol w:w="2000"/></w:tblGrid>` + rows + `</w:tbl>`
	}
	row := func(props, text string) string { return `<w:tr>` + cell(props, text) + `</w:tr>` }
	graphic := func(width string) string {
		return `<w:p><w:r><w:drawing xmlns:wp="` + documentDrawingNS + `" xmlns:a="` + documentDrawingMainNS + `" xmlns:pic="` + documentPictureNS + `"><wp:inline><wp:extent cx="` + width + `" cy="720000"/><wp:docPr id="1" name="ARROW" descr="synthetic"/><a:graphic><a:graphicData><pic:pic><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="` + width + `" cy="720000"/></a:xfrm><a:prstGeom prst="rect"/></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`
	}
	for _, tc := range []struct{ action, payload, before, after string }{
		{"document-paragraph", releaseDocumentPayloads["document-paragraph"], paragraph("", "Target"), paragraph(`<w:ind w:firstLineChars="200"/>`, "Target")},
		{"document-page", releaseDocumentPayloads["document-page"], paragraph("", "Target") + `<w:sectPr><w:cols w:num="1"/></w:sectPr>`, paragraph("", "Target") + `<w:sectPr><w:cols w:num="2"/></w:sectPr>`},
		{"document-replace", releaseDocumentPayloads["document-replace"], paragraph("", "Target"), paragraph("", "Updated")},
		{"document-mention", releaseDocumentPayloads["document-mention"], paragraph("", "Target"), paragraph("", "Target@Synthetic Tester")},
		{"document-table", releaseDocumentPayloads["document-table"], table(row("", "Target")), table(row("", "Target") + row("", ""))},
		{"document-table-style", releaseDocumentPayloads["document-table-style"], table(row("", "Target")), table(row(`<w:shd w:val="clear" w:fill="0000FF"/>`, "Target"))},
		{"document-graphic", `{"name":"ARROW","description":"synthetic","widthCm":4,"heightCm":2}`, graphic("1080000"), graphic("1440000")},
	} {
		for _, fault := range []string{"control", "reverted-reload"} {
			t.Run(tc.action+"/"+fault, func(t *testing.T) {
				before, after := releaseDocumentPackage(t, tc.before), releaseDocumentPackage(t, tc.after)
				file := filepath.Join(t.TempDir(), "before.docx")
				if err := os.WriteFile(file, before, 0600); err != nil {
					t.Fatal(err)
				}
				var p map[string]any
				if err := json.Unmarshal([]byte(tc.payload), &p); err != nil {
					t.Fatal(err)
				}
				mutation := ""
				mentionReadback := ""
				switch tc.action {
				case "document-paragraph", "document-page":
					spec, e := prepareDocumentLayout(tc.action, p)
					if e != nil {
						t.Fatal(e)
					}
					snap, e := readDocumentLayout(file)
					if e != nil {
						t.Fatal(e)
					}
					mutation = documentLayoutJS(spec, snap)
				case "document-replace":
					spec, e := prepareDocumentReplace(p)
					if e != nil {
						t.Fatal(e)
					}
					snap, e := readDocumentLayout(file)
					if e != nil {
						t.Fatal(e)
					}
					steps, e := documentReplaceSteps(snap, spec)
					if e != nil {
						t.Fatal(e)
					}
					mutation = documentReplaceJS(spec, snap, steps)
				case "document-mention":
					spec, e := prepareDocumentMention(p)
					if e != nil {
						t.Fatal(e)
					}
					mutation = documentMentionJS(spec, "42", "Synthetic Tester")
					mentionReadback = documentMentionReadbackJS(spec, "42", "Synthetic Tester")
				case "document-table":
					spec, e := prepareDocumentTable(p)
					if e != nil {
						t.Fatal(e)
					}
					snap, e := readDocumentTables(file)
					if e != nil {
						t.Fatal(e)
					}
					mutation = documentTableJS(spec, snap)
				case "document-table-style":
					spec, e := prepareDocumentTableStyle(p)
					if e != nil {
						t.Fatal(e)
					}
					snap, e := readDocumentTables(file)
					if e != nil {
						t.Fatal(e)
					}
					mutation = documentTableStyleJS(spec, snap)
				case "document-graphic":
					spec, e := prepareDocumentGraphic(p)
					if e != nil {
						t.Fatal(e)
					}
					snap, e := readDocumentGraphic(file)
					if e != nil {
						t.Fatal(e)
					}
					baseline, e := documentGraphicBaseline(snap.Root, spec)
					if e != nil {
						t.Fatal(e)
					}
					mutation = documentGraphicRuntimeJS(spec, baseline, true, "synthetic-object")
				}
				var writes, exports, reloads, opens, shots atomic.Int32
				releaseDocumentHTTP(t, func() ([]byte, int) {
					exports.Add(1)
					if writes.Load() > 0 && !(fault == "reverted-reload" && (reloads.Load() > 0 || opens.Load() > 1)) {
						return after, 200
					}
					return before, 200
				}, `{"type":"documentPro","name":"Synthetic"}`)
				old := phase3OpenDocumentEditor
				t.Cleanup(func() { phase3OpenDocumentEditor = old })
				phase3OpenDocumentEditor = func(target, readiness string) (*phase3EditorBrowser, error) {
					if target != "https://phase3.test/docx/abcdefgh" {
						return nil, errors.New("unexpected target")
					}
					opens.Add(1)
					return releaseDocumentEditor(t, func(expr string) any {
						if expr == mutation {
							writes.Add(1)
							return map[string]any{"ok": true, "submitted": true}
						}
						switch {
						case mentionReadback != "" && expr == mentionReadback:
							return map[string]any{"ok": true, "nodeId": "synthetic-mention", "userId": 42, "displayText": "@Synthetic Tester"}
						case strings.Contains(expr, "pageElapsedMs"):
							return map[string]any{"ready": true}
						case strings.Contains(expr, "haveUnsavedChange"), strings.Contains(expr, "const presence="), strings.Contains(expr, "__shimo"):
							return map[string]any{"ok": true, "objectId": "synthetic-object"}
						default:
							t.Errorf("unexpected expression %.120s", expr)
							return map[string]any{"ok": false, "error": "unexpected read"}
						}
					}, func() error { shots.Add(1); return nil }, func() { reloads.Add(1) }), nil
				}
				out := t.TempDir()
				a := newApp([]string{"phase3", "https://phase3.test/docx/abcdefgh", "--action", tc.action, "--payload", tc.payload, "--out", out, "--relogin", "--json"})
				a.Origin = "https://phase3.test"
				err := a.Run()
				success := fault == "control"
				if (err == nil) != success {
					t.Fatalf("unexpected result: %v", err)
				}
				if writes.Load() != 1 {
					t.Fatalf("expected one content write, got %d, err=%v", writes.Load(), err)
				}
				releaseDocumentEvidence(t, out, success, true)
				if exports.Load() != 3 {
					t.Fatalf("expected baseline/write/reload exports, got %d, err=%v", exports.Load(), err)
				}
				if success && shots.Load() != 3 {
					t.Fatalf("expected before/after/reload shots, got %d", shots.Load())
				}
				if !success && !strings.Contains(err.Error(), "重放") {
					t.Fatalf("missing postwrite warning: %v", err)
				}
			})
		}
	}
}
