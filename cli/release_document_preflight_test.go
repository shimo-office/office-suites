package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are synthetic OOXML packages served by an in-memory transport. They
// exercise the production command/export/evidence path, not a remote editor.
func releaseDocumentPackage(t *testing.T, body string) []byte {
	t.Helper()
	p := artifactTestZip(t, map[string]string{
		"[Content_Types].xml": `<Types xmlns="` + artifactContentNS + `"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="main" Type="` + artifactOfficeRelNS + `/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="` + artifactWordNS + `"><w:body>` + body + `</w:body></w:document>`,
		"word/styles.xml":     `<w:styles xmlns:w="` + artifactWordNS + `"/>`,
	})
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func releaseDocumentHTTP(t *testing.T, exported func() ([]byte, int), info string) *[]string {
	t.Helper()
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic-document-test")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	old, oldDefault := httpClient, http.DefaultTransport
	t.Cleanup(func() { httpClient = old; http.DefaultTransport = oldDefault })
	oldOpen := phase3OpenDocumentEditor
	phase3OpenDocumentEditor = func(string, string) (*phase3EditorBrowser, error) {
		t.Error("preflight unexpectedly attempted to open a browser")
		return nil, errors.New("test blocks all actual browser launch")
	}
	t.Cleanup(func() { phase3OpenDocumentEditor = oldOpen })
	trace := []string{}
	transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		trace = append(trace, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/users/me":
			return phase3Response(200, `{"id":42,"name":"Synthetic Tester"}`), nil
		case "/api/v1/files/abcdefgh":
			return phase3Response(200, info), nil
		case "/api/v1/files/export/abcdefgh":
			if r.Method != "POST" {
				t.Fatalf("unexpected export method: %s", r.Method)
			}
			return phase3Response(200, `{"data":{"taskId":"synthetic-docx"}}`), nil
		case "/api/v1/files/export/progress":
			return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://phase3.test/synthetic.docx"}}`), nil
		case "/synthetic.docx":
			body, status := exported()
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
		default:
			return nil, fmt.Errorf("unexpected request in document test: %s %s", r.Method, r.URL.Path)
		}
	})
	httpClient = &http.Client{Transport: transport}
	http.DefaultTransport = transport
	return &trace
}

var releaseDocumentPayloads = map[string]string{
	"document-format":      `{"paragraph":1,"text":"Target","attrs":{"bold":true}}`,
	"document-paragraph":   `{"paragraph":1,"text":"Target","attrs":{"firstLineChars":2}}`,
	"document-page":        `{"columns":2}`,
	"document-theme":       `{"resetSupplemental":true,"themeFonts":{"major":{"latin":"Arial","ea":"宋体","cs":"Arial"},"minor":{"latin":"Arial","ea":"宋体","cs":"Arial"}}}`,
	"document-table":       `{"operation":"insert-row-after","table":1,"row":1,"column":1,"expectedCell":"Target"}`,
	"document-table-style": `{"table":1,"row":1,"column":1,"expectedCell":"Target","fill":"#0000FF"}`,
	"document-graphic":     `{"name":"MISSING","description":"synthetic","widthCm":4,"heightCm":2}`,
	"document-mention":     `{"paragraph":1,"text":"Target"}`,
	"document-replace":     `{"find":"Target","replacement":"Updated","expectedCount":1}`,
	"document-goto":        `{"page":2}`,
}

func TestReleaseDocumentCommandsRejectInvalidPayloadBeforeHTTP(t *testing.T) {
	for action := range releaseDocumentPayloads {
		for _, payload := range []string{`{`, `[]`, `{"unknown":true}`} {
			t.Run(action+"/"+payload, func(t *testing.T) {
				trace := releaseDocumentHTTP(t, func() ([]byte, int) { t.Fatal("download on invalid request"); return nil, 500 }, `{}`)
				a := newApp([]string{"phase3", "https://phase3.test/docx/abcdefgh", "--action", action, "--payload", payload, "--json"})
				a.Origin = "https://phase3.test"
				if err := a.Run(); err == nil {
					t.Fatal("invalid request succeeded")
				}
				if len(*trace) != 0 {
					t.Fatalf("invalid input reached HTTP: %v", *trace)
				}
			})
		}
	}
}

func TestReleaseDocumentExportFailureIsPrewriteAndRetainsEvidence(t *testing.T) {
	for action, payload := range releaseDocumentPayloads {
		for _, fault := range []string{"wrong-type", "empty-package", "download-403"} {
			t.Run(action+"/"+fault, func(t *testing.T) {
				info := `{"type":"documentPro","name":"Synthetic"}`
				if fault == "wrong-type" {
					info = `{"type":"presentation","name":"Synthetic"}`
				}
				trace := releaseDocumentHTTP(t, func() ([]byte, int) {
					if fault == "download-403" {
						return []byte("forbidden"), 403
					}
					return []byte{}, 200
				}, info)
				out := t.TempDir()
				a := newApp([]string{"phase3", "https://phase3.test/docx/abcdefgh", "--action", action, "--payload", payload, "--out", out, "--json"})
				a.Origin = "https://phase3.test"
				err := a.Run()
				if err == nil {
					t.Fatal("failed export accepted")
				}
				entries, _ := os.ReadDir(out)
				if len(entries) != 1 {
					t.Fatalf("missing evidence for %s: %v", action, err)
				}
				dir := filepath.Join(out, entries[0].Name())
				b, readErr := os.ReadFile(filepath.Join(dir, "failure.json"))
				if readErr != nil {
					t.Fatal(readErr)
				}
				var failure map[string]any
				if json.Unmarshal(b, &failure) != nil || failure["ok"] != false || failure["mutationAttempted"] != false {
					t.Fatalf("false success/attempt metadata: %s", b)
				}
				if _, err := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(err) {
					t.Fatal("failure emitted success evidence")
				}
				for _, request := range *trace {
					if strings.Contains(request, "POST") && request != "POST /api/v1/files/export/abcdefgh" && request != "POST /api/v1/files/export/progress" {
						t.Fatalf("unexpected write %s", request)
					}
				}
			})
		}
	}
}

func TestReleaseDocumentBaselineMismatchRejectsBeforeBrowser(t *testing.T) {
	base := releaseDocumentPackage(t, `<w:p><w:r><w:t>Different</w:t></w:r></w:p><w:sectPr/>`)
	for _, action := range []string{"document-format", "document-paragraph", "document-theme", "document-table", "document-table-style", "document-graphic", "document-mention", "document-replace"} {
		t.Run(action, func(t *testing.T) {
			releaseDocumentHTTP(t, func() ([]byte, int) { return base, 200 }, `{"type":"documentPro","name":"Synthetic"}`)
			out := t.TempDir()
			a := newApp([]string{"phase3", "https://phase3.test/docx/abcdefgh", "--action", action, "--payload", releaseDocumentPayloads[action], "--out", out, "--json"})
			a.Origin = "https://phase3.test"
			if err := a.Run(); err == nil {
				t.Fatal("baseline mismatch accepted")
			}
			entries, _ := os.ReadDir(out)
			if len(entries) != 1 {
				t.Fatal("missing evidence")
			}
			b, err := os.ReadFile(filepath.Join(out, entries[0].Name(), "failure.json"))
			if err != nil {
				t.Fatal(err)
			}
			var failure map[string]any
			if json.Unmarshal(b, &failure) != nil || failure["mutationAttempted"] != false {
				t.Fatalf("mutation incorrectly attempted: %s", b)
			}
		})
	}
}

func TestReleaseDocumentLayoutInspectExportsWithoutMutation(t *testing.T) {
	base := releaseDocumentPackage(t, `<w:p><w:r><w:t>Target</w:t></w:r></w:p><w:sectPr><w:cols w:num="2"/></w:sectPr>`)
	trace := releaseDocumentHTTP(t, func() ([]byte, int) { return base, 200 }, `{"type":"documentPro","name":"Synthetic"}`)
	out := t.TempDir()
	a := newApp([]string{"phase3", "https://phase3.test/docx/abcdefgh", "--action", "document-layout-inspect", "--out", out, "--json"})
	a.Origin = "https://phase3.test"
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(out)
	b, err := os.ReadFile(filepath.Join(out, entries[0].Name(), "inspection.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if json.Unmarshal(b, &result) != nil || result["ok"] != true || result["mutationAttempted"] != false {
		t.Fatalf("invalid inspection metadata: %s", b)
	}
	if strings.Contains(strings.Join(*trace, "\n"), "PUT ") {
		t.Fatal("read-only inspection wrote data")
	}
}
