package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSheetWizardWaitRetainsPageDiagnostic(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("requires existing Node")
	}
	ws := releasePhase2Peer(t, func(_ string, params map[string]any) (map[string]any, error) {
		code := `global.document={body:{innerText:'正在准备导出'}};process.stdout.write(JSON.stringify(` + str(params, "expression") + `))`
		out, err := exec.Command(node, "-e", code).Output()
		if err != nil {
			return nil, err
		}
		var value any
		if err = json.Unmarshal(out, &value); err != nil {
			return nil, err
		}
		return map[string]any{"result": map[string]any{"value": value}}, nil
	})
	b := &phase3EditorBrowser{ws: ws}
	err = b.waitUIText([]string{"导出设置", "共"}, []string{"正在准备导出"}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "当前页面：正在准备导出") {
		t.Fatalf("a pending UI condition must preserve diagnostic text: %v", err)
	}
}

func TestSheetWizardWaitDoesNotHideCDPFailure(t *testing.T) {
	ws := releasePhase2Peer(t, func(string, map[string]any) (map[string]any, error) {
		return nil, errors.New("synthetic connection failure")
	})
	b := &phase3EditorBrowser{ws: ws}
	err := b.waitUIText([]string{"导出设置"}, nil, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "synthetic connection failure") {
		t.Fatalf("connection failure must not become an empty-page timeout: %v", err)
	}
}

func TestSheetBrowserActionSchemaMatchesPreflight(t *testing.T) {
	for _, action := range []string{"sheet-freeze", "sheet-filter", "sheet-filter-clear", "sheet-export-pdf"} {
		t.Run(action, func(t *testing.T) {
			result, err := schemaAction(action)
			if err != nil {
				t.Fatal(err)
			}
			contract := result["contract"].(actionContract)
			if result["command"] != "editor" || contract.Transport == "" {
				t.Fatalf("missing executable entry: %#v", result)
			}
			var payload map[string]any
			if err = json.Unmarshal([]byte(contract.Example), &payload); err != nil {
				t.Fatal(err)
			}
			if sheetBrowserAction(action) {
				if _, err = prepareSheetBrowser(action, payload); err != nil {
					t.Fatalf("published example does not match implementation: %v", err)
				}
			}
		})
	}
}

func TestGetSpreadsheetPDFRoutesToBrowserAndHonorsNoBrowser(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/files/abcdefgh" {
			t.Fatalf("PDF must not call the unsupported export API: %s", r.URL)
		}
		return phase3Response(200, `{"guid":"abcdefgh","name":"sheet","type":"spreadsheet"}`), nil
	})
	a := newApp([]string{"get", "abcdefgh", "--format", "pdf", "--no-browser", "--out", t.TempDir()})
	err := a.Run()
	var detail *commandError
	if !errors.As(err, &detail) || detail.Result["code"] != "BROWSER_REQUIRED" {
		t.Fatalf("get must route to browser PDF and report the explicit browser prohibition: %v", err)
	}
}

func TestSheetPDFDefaultOutputRetainsDeliverable(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	a := newApp([]string{"editor", "https://shimo.net/sheets/abcdefgh", "--action", "sheet-export-pdf", "--json"})
	var deliverable string
	err := a.withPhase3Evidence(func() error {
		dir, err := a.phase3EvidenceDir("")
		if err != nil {
			return err
		}
		deliverable = filepath.Join(dir, "report.pdf")
		if err = os.WriteFile(deliverable, []byte("synthetic deliverable"), 0600); err != nil {
			return err
		}
		return a.printJSON(map[string]any{"pdf": deliverable})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(deliverable); err != nil {
		t.Fatalf("successful PDF must survive temporary evidence cleanup: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(deliverable)) })
}

func TestGetCloudSpreadsheetDoesNotConvertToPDF(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/files/abcdefgh" {
			t.Fatalf("must stop before conversion: %s", r.URL)
		}
		return phase3Response(200, `{"guid":"abcdefgh","name":"original.xlsx","type":"spreadsheet","isCloudFile":true}`), nil
	})
	a := newApp([]string{"get", "abcdefgh", "--format", "pdf", "--no-browser", "--out", t.TempDir()})
	err := a.Run()
	var detail *commandError
	if err == nil || errors.As(err, &detail) {
		t.Fatalf("cloud attachments must retain original-download semantics: %v", err)
	}
}
