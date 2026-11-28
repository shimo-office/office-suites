package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseSheetPDFMenuFlowRetainsArtifactAndActualSettings(t *testing.T) {
	isolatedEvidenceTemp(t)
	sessionTestState(t)
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/files/abcdefgh" {
			t.Fatal("PDF used a server export route", r.URL.Path)
		}
		return phase3Response(200, `{"guid":"abcdefgh","name":"test","type":"spreadsheet"}`), nil
	})
	screenshot := base64.StdEncoding.EncodeToString(releaseFlowPNG(t))
	var downloadDir string
	clicks := 0
	ws := releasePhase2Peer(t, func(method string, p map[string]any) (map[string]any, error) {
		switch method {
		case "Page.captureScreenshot":
			return map[string]any{"data": screenshot}, nil
		case "Browser.setDownloadBehavior":
			if str(p, "behavior") != "allow" || p["eventsEnabled"] != true {
				t.Fatal("downloads are not isolated")
			}
			downloadDir = str(p, "downloadPath")
			return map[string]any{}, nil
		case "Input.dispatchMouseEvent":
			if str(p, "type") == "mouseReleased" {
				clicks++
				if clicks == 3 {
					body := "%PDF-1.7\n" + strings.Repeat("% synthetic download envelope\n", 8) + "%%EOF\n"
					if err := os.WriteFile(filepath.Join(downloadDir, "export.pdf"), []byte(body), 0600); err != nil {
						t.Error(err)
					}
				}
			}
			return map[string]any{}, nil
		case "Runtime.evaluate":
			expression := str(p, "expression")
			value := map[string]any{"found": true, "x": 20, "y": 30, "left": 0, "right": 0, "mid": 30}
			if strings.Contains(expression, "matched:Boolean") {
				value = map[string]any{"matched": true, "text": "导出设置\n共 1 页"}
			} else if strings.Contains(expression, "text:(document.body") {
				value = map[string]any{"text": "导出设置\n纸张 A3\n共 1 页"}
			}
			return map[string]any{"result": map[string]any{"value": value}}, nil
		default:
			return map[string]any{}, nil
		}
	})
	previous := phase3StartEditor
	t.Cleanup(func() { phase3StartEditor = previous })
	phase3StartEditor = func(target, ready string) (*phase3EditorBrowser, error) {
		if target != "https://shimo.net/sheets/abcdefgh" || ready != sheetBrowserReadyJS {
			t.Fatal("wrong editor target or readiness", target)
		}
		return &phase3EditorBrowser{ws: ws, profile: t.TempDir()}, nil
	}
	out := t.TempDir()
	a := newApp([]string{"get", "abcdefgh", "--format", "pdf", "--out", out, "--json"})
	stdout, err := issueCapture(t, a.Run)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err = json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if result["ok"] != true || result["verified"] != true || result["pages"] != "共 1 页" || clicks != 3 {
		t.Fatal("incomplete export flow", result, clicks)
	}
	pdf := str(result, "pdf")
	if _, _, err = verifySheetPDF(pdf); err != nil {
		t.Fatal("delivered PDF was removed or invalid", err)
	}
	if _, err = os.Stat(downloadDir); !os.IsNotExist(err) {
		t.Fatal("temporary download directory was retained")
	}
	settings := asObj(result["settings"])
	if settings["modified"] != false || settings["source"] != "current-page-settings" || settings["paper"] != nil {
		t.Fatal("invented export settings", settings)
	}
	raw, err := os.ReadFile(str(settings, "snapshot"))
	if err != nil || !strings.Contains(string(raw), "A3") {
		t.Fatal("actual dialog setting was lost", err)
	}
	view := asObj(result["view"])
	if view["transport"] != viewTransportDownload || asObj(view["builtInBrowser"])["requiresAuth"] != false {
		t.Fatal("PDF requires a second login", view)
	}
}

func TestReleaseSheetPDFLaunchFailureIsReadOnlyAndRetainsStage(t *testing.T) {
	sessionTestState(t)
	issueFileTransport(t, func(*http.Request) (*http.Response, error) {
		return phase3Response(200, `{"guid":"abcdefgh","type":"spreadsheet"}`), nil
	})
	previous := phase3StartEditor
	t.Cleanup(func() { phase3StartEditor = previous })
	phase3StartEditor = func(string, string) (*phase3EditorBrowser, error) {
		return nil, errors.New("synthetic startup failure")
	}
	dir := t.TempDir()
	a := newApp([]string{"editor", "https://shimo.net/sheets/abcdefgh", "--action", "sheet-export-pdf", "--out", dir})
	a.Origin = "https://shimo.net"
	err := a.cmdSheetExportPDF()
	if err == nil || !strings.Contains(err.Error(), "synthetic startup failure") {
		t.Fatal("lost launch diagnosis", err)
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "phase3-*", "failure.json"))
	if len(paths) != 1 {
		t.Fatal("missing failure record", paths)
	}
	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var failure map[string]any
	if err = json.Unmarshal(raw, &failure); err != nil {
		t.Fatal(err)
	}
	if failure["stage"] != "menu" || failure["mutationAttempted"] != false || failure["mayHaveApplied"] != false || failure["noReplay"] != false {
		t.Fatal("read-only export was mislabeled as a write", failure)
	}
}
