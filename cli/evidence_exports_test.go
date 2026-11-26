package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvidencePDFCommandRetainsDeliverableWithoutOut(t *testing.T) {
	for _, action := range []string{"slide-print-pdf", " slide-print-pdf "} {
		t.Run(action, func(t *testing.T) { evidencePDFCommand(t, action) })
	}
}

func TestEvidenceArtifactCommandDoesNotPruneItsInput(t *testing.T) {
	root := isolatedEvidenceTemp(t)
	dir := evidenceRetentionFixture(t, root, "artifact-input", time.Now().Add(-8*24*time.Hour), 1)
	path := filepath.Join(dir, "before.pptx")
	if err := os.WriteFile(path, releaseFlowPPTX(t, []string{releaseFlowOriginal}, -1), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := issueCapture(t, newApp([]string{"phase3", "artifact", path, "--json"}).Run)
	if err != nil || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("artifact input pruned: %s, %v", out, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func evidencePDFCommand(t *testing.T, action string) {
	isolatedEvidenceTemp(t)
	source := releaseFlowPPTX(t, []string{releaseFlowOriginal}, -1)
	pdf := []byte("%PDF-1.7\n" + strings.Repeat("synthetic PDF body ", 20))
	current := ""
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/abcdefgh":
			return phase3Response(200, `{"type":"presentation","name":"synthetic"}`), nil
		case "/api/v1/files/export/abcdefgh":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				return nil, err
			}
			current = str(body, "type")
			return phase3Response(200, `{"data":{"taskId":"test"}}`), nil
		case "/api/v1/files/export/progress":
			return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://shimo.net/download"}}`), nil
		case "/download":
			data := source
			if current == "pdf" {
				data = pdf
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
		}
		return nil, fmt.Errorf("unexpected offline request: %s", r.URL.Path)
	})
	out, err := issueCapture(t, newApp([]string{"phase3", "https://shimo.net/presentation/abcdefgh", "--action", action, "--json"}).Run)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result["verified"] != false {
		t.Fatalf("export incorrectly claims verified: %s", out)
	}
	if err := pruneManagedEvidence(time.Now().Add(30*24*time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(str(result, "pdf"))
	if err != nil || !bytes.Equal(got, pdf) {
		t.Fatalf("PDF lost/changed: %v", err)
	}
}

func TestEvidenceCSVCommandRetainsDeliverableWithoutOut(t *testing.T) {
	for _, action := range []string{"form-results", " form-results "} {
		t.Run(action, func(t *testing.T) { evidenceCSVCommand(t, action) })
	}
}

func evidenceCSVCommand(t *testing.T, action string) {
	isolatedEvidenceTemp(t)
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			return nil, fmt.Errorf("read action attempted %s", r.Method)
		}
		switch r.URL.Path {
		case "/api/v1/files/abcdefgh":
			return phase3Response(200, `{"type":"form"}`), nil
		case "/forms/abcdefgh":
			return phase3Response(200, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/","SDK_V2_SIGNATURE":"test","SDK_V2_TOKEN":"test"}}];`), nil
		case "/sdk/forms/abcdefgh/publications":
			return phase3Response(200, `{"status":"normal","publishedRev":1,"publishedContent":"{\"questions\":[{\"guid\":\"q001\",\"subject\":\"Question\"}]}"}`), nil
		case "/sdk/forms/abcdefgh/responses":
			return phase3Response(200, `{"total":1,"responses":[{"submitId":"one","content":[{"guid":"q001","text":{"content":"keep me"}}]}]}`), nil
		case "/sdk/forms/abcdefgh/statistics":
			return phase3Response(200, `{"responses":1,"views":1}`), nil
		}
		return nil, fmt.Errorf("unexpected offline request: %s", r.URL.Path)
	})
	out, err := issueCapture(t, newApp([]string{"phase3", "https://shimo.net/forms/abcdefgh", "--action", action, "--json"}).Run)
	// Unlike the PDF handler, form-read currently rejects a padded action before
	// creating any export. Keep that existing validation behavior unchanged.
	if action != strings.TrimSpace(action) {
		if err == nil || !strings.Contains(err.Error(), "不支持的表单读取动作") || out != "" {
			t.Fatalf("padded form action unexpectedly accepted: %q, %v", out, err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if err := pruneManagedEvidence(time.Now().Add(30*24*time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"csv", "responses"} {
		got, err := os.ReadFile(str(result, key))
		if err != nil || !strings.Contains(string(got), "keep me") {
			t.Fatalf("%s lost/changed: %v", key, err)
		}
	}
}
