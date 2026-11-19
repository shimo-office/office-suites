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
)

func phase3Workbook(t *testing.T, xml string) []byte {
	t.Helper()
	p := artifactTestZip(t, artifactWorkbookEntries(`<worksheet xmlns="`+artifactNS+`"><sheetData>`+xml+`</sheetData></worksheet>`))
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPhase3HTTPPipeline(t *testing.T) {
	beforeXML := `<row r="1"><c r="A1" t="inlineStr"><is><t>old</t></is></c></row>`
	afterXML := `<row r="1"><c r="A1" t="inlineStr"><is><t>new</t></is></c></row>`
	for _, tc := range []struct {
		name              string
		noop, lateFailure bool
		unauthorized      bool
		wantSuccess       bool
		blockEvidence     string
	}{
		{name: "verified update", wantSuccess: true},
		{name: "HTTP accepted but no-op", noop: true},
		{name: "late readback failure", lateFailure: true},
		{name: "late 401 does not replay with relogin", lateFailure: true, unauthorized: true},
		{name: "submission evidence cannot be saved", blockEvidence: "submission.json"},
		{name: "result evidence cannot be saved", blockEvidence: "result.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=test")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			t.Setenv("SHIMO_NET_NO_BROWSER", "1")
			old, oldDefault := httpClient, http.DefaultTransport
			defer func() { httpClient = old; http.DefaultTransport = oldDefault }()
			before, after := phase3Workbook(t, beforeXML), phase3Workbook(t, afterXML)
			writes := 0
			out := t.TempDir()
			transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.URL.Path == "/api/v1/files/abcdefgh":
					return phase3Response(200, `{"type":"spreadsheet","name":"Evidence"}`), nil
				case r.URL.Path == "/api/v1/files/export/abcdefgh":
					return phase3Response(200, `{"data":{"taskId":"test-export"}}`), nil
				case r.URL.Path == "/api/v1/files/export/progress":
					return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://phase3.test/download.xlsx"}}`), nil
				case r.URL.Path == "/download.xlsx":
					body := before
					if writes > 0 && !tc.noop {
						body = after
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
				case r.URL.Path == "/lizard-api/files/abcdefgh/sheets/values" && r.Method == "GET":
					if writes > 0 && tc.lateFailure {
						status := 403
						if tc.unauthorized {
							status = 401
						}
						return phase3Response(status, `{"error":"forbidden"}`), nil
					}
					value := "old"
					if writes > 0 {
						value = "new"
					}
					return phase3Response(200, fmt.Sprintf(`{"range":"Target!A1","values":[[%q]]}`, value)), nil
				case r.URL.Path == "/lizard-api/files/abcdefgh/sheets/values" && r.Method == "POST":
					writes++
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if payload["range"] != "Target!A1" {
						t.Fatalf("wrong mutation payload: %#v", payload)
					}
					if tc.blockEvidence != "" {
						dirs, err := os.ReadDir(out)
						if err != nil || len(dirs) != 1 {
							t.Fatalf("evidence before accepted write: %v %v", dirs, err)
						}
						if err := os.Mkdir(filepath.Join(out, dirs[0].Name(), tc.blockEvidence), 0700); err != nil {
							t.Fatal(err)
						}
					}
					return phase3Response(204, ""), nil
				default:
					t.Fatalf("unexpected request %s %s", r.Method, r.URL)
					return nil, nil
				}
			})
			httpClient = &http.Client{Transport: transport}
			http.DefaultTransport = transport
			a := newApp([]string{"phase3", "https://phase3.test/sheets/abcdefgh", "--action", "sheet-update", "--payload", `{"range":"Target!A1","resource":{"values":[["new"]]}}`, "--read-range", "Target!A1", "--out", out})
			a.Origin = "https://phase3.test"
			a.Relogin = true
			err := a.Run()
			if (err == nil) != tc.wantSuccess {
				t.Fatalf("success=%v err=%v", tc.wantSuccess, err)
			}
			if writes != 1 {
				t.Fatalf("writes=%d want 1", writes)
			}
			if err != nil && !strings.Contains(err.Error(), "写入已提交") {
				t.Fatalf("partial write not disclosed: %v", err)
			}
			if err != nil && (!strings.Contains(err.Error(), "禁止自动重放") || !strings.Contains(err.Error(), out)) {
				t.Fatalf("post-write failure missing replay warning or evidence location: %v", err)
			}
			dirs, err := os.ReadDir(out)
			if err != nil || len(dirs) != 1 {
				t.Fatalf("evidence dirs: %v %v", dirs, err)
			}
			root := filepath.Join(out, dirs[0].Name())
			for _, file := range []string{"request.json", "before-readback.json", "submission.json", "before/Evidence.xlsx"} {
				if _, err := os.Stat(filepath.Join(root, file)); err != nil {
					t.Errorf("missing evidence %s: %v", file, err)
				}
			}
			result, readErr := os.ReadFile(filepath.Join(root, "result.json"))
			if tc.wantSuccess {
				if readErr != nil || !bytes.Contains(result, []byte(`"verified": true`)) {
					t.Fatalf("missing verified result %s %v", result, readErr)
				}
			} else if readErr == nil {
				t.Fatal("failed operation wrote success result")
			}
		})
	}
}

func TestPhase3AppendNoOpCannotUseExistingDuplicate(t *testing.T) {
	cell := xlsxCell{Ref: "A1", Type: "string", Value: "duplicate", StyleKey: "default"}
	before := artifactSheetSnapshot{Name: "Target", Cells: map[string]xlsxCell{"A1": cell}, Merges: []string{}}
	spec := phase3Spec{Action: "sheet-append", Sheet: "Target", ReadRange: "Target!A1:A2", Expected: artifactExpectation{Values: [][]any{{"duplicate"}}}}
	if err := phase3ValidateSnapshot(before, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Expected.Range != "Target!A2:A2" {
		t.Fatalf("append target=%s", spec.Expected.Range)
	}
	p := artifactTestZip(t, artifactWorkbookEntries(`<worksheet xmlns="`+artifactNS+`"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>duplicate</t></is></c></row></sheetData></worksheet>`))
	spec.Expected.SheetName = "Target"
	if _, err := verifyArtifactExpected(p, "xlsx", spec.Expected); err == nil {
		t.Fatal("pre-existing duplicate accepted as appended value")
	}
}
