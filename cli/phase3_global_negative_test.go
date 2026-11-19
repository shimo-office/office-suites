package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An opt-in archive contains only this test's synthetic fixtures and mock HTTP
// observations. It is never evidence of a real remote failure or CLI subprocess.
func archivePhase3Negative(t *testing.T, dir string, record map[string]any) {
	t.Helper()
	root := os.Getenv("SHIMO_PHASE3_NEGATIVE_OUT")
	if root == "" {
		return
	}
	dest := filepath.Join(root, strings.ReplaceAll(t.Name(), "/", "--"))
	if err := os.MkdirAll(dest, 0700); err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		to := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(to, 0700)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(to, b, 0600)
	}); err != nil {
		t.Fatal(err)
	}
	if record["source"] == nil {
		record["source"] = "offline-production-App.Run-with-mocked-HTTP-and-optional-fake-CDP"
	}
	record["liveFailure"] = false
	record["cliSubprocessExitCode"] = nil
	if executable, err := os.Executable(); err == nil {
		if b, e := os.ReadFile(executable); e == nil {
			record["testBinarySHA256"] = fmt.Sprintf("%x", sha256.Sum256(b))
		}
	}
	b, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dest, "offline-observation.json"), append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPhase3GlobalSheetNegativePipeline(t *testing.T) {
	for _, action := range []string{"sheet-append", "sheet-delete"} {
		for _, fault := range []string{"control", "no-op", "empty-export", "export-403", "readback-401"} {
			t.Run(action+"/"+fault, func(t *testing.T) {
				t.Setenv("SHIMO_NET_COOKIE", "sid=test")
				t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
				t.Setenv("SHIMO_NET_NO_BROWSER", "1")
				old, oldDefault := httpClient, http.DefaultTransport
				defer func() { httpClient = old; http.DefaultTransport = oldDefault }()
				beforeXML := `<row r="1"><c r="A1" t="inlineStr"><is><t>old</t></is></c></row><row r="2"><c r="A2" t="inlineStr"><is><t>keep</t></is></c></row>`
				afterXML := beforeXML + `<row r="3"><c r="A3" t="inlineStr"><is><t>new</t></is></c></row>`
				payload := `{"range":"Target!A1","resource":{"values":[["new"]]}}`
				if action == "sheet-delete" {
					afterXML = `<row r="1"><c r="A1" t="inlineStr"><is><t>keep</t></is></c></row>`
					payload = `{"sheetName":"Target","index":0,"count":1}`
				}
				before, after := phase3Workbook(t, beforeXML), phase3Workbook(t, afterXML)
				writes := 0
				trace := []map[string]any{}
				transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
					trace = append(trace, map[string]any{"method": r.Method, "path": r.URL.Path, "writesObservedBeforeRequest": writes})
					switch {
					case r.URL.Path == "/api/v1/files/abcdefgh":
						return phase3Response(200, `{"type":"spreadsheet","name":"OFFLINE_NEGATIVE_FIXTURE"}`), nil
					case r.URL.Path == "/api/v1/files/export/abcdefgh":
						if writes > 0 && fault == "export-403" {
							return phase3Response(403, `{"error":"injected export failure"}`), nil
						}
						return phase3Response(200, `{"data":{"taskId":"offline-export"}}`), nil
					case r.URL.Path == "/api/v1/files/export/progress":
						return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://phase3.test/offline.xlsx"}}`), nil
					case r.URL.Path == "/offline.xlsx":
						body := before
						if writes > 0 && fault != "no-op" {
							body = after
						}
						if writes > 0 && fault == "empty-export" {
							body = nil
						}
						return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body))}, nil
					case strings.HasPrefix(r.URL.Path, "/lizard-api/files/abcdefgh/sheets") && r.Method != "GET":
						want := "PUT"
						if action == "sheet-delete" {
							want = "DELETE"
						}
						if r.Method != want {
							t.Fatalf("wrong mutation %s", r.Method)
						}
						writes++
						return phase3Response(204, ""), nil
					case r.URL.Path == "/lizard-api/files/abcdefgh/sheets/values" && r.Method == "GET":
						if writes > 0 && fault == "readback-401" {
							return phase3Response(401, `{"error":"injected expired session"}`), nil
						}
						values := [][]string{{"old"}, {"keep"}, {""}}
						if writes > 0 && fault != "no-op" {
							values = [][]string{{"old"}, {"keep"}, {"new"}}
							if action == "sheet-delete" {
								values = [][]string{{"keep"}, {""}, {""}}
							}
						}
						b, _ := json.Marshal(map[string]any{"range": "Target!A1:A3", "values": values})
						return phase3Response(200, string(b)), nil
					default:
						t.Fatalf("unexpected mock request %s %s", r.Method, r.URL.Path)
						return nil, nil
					}
				})
				httpClient = &http.Client{Transport: transport}
				http.DefaultTransport = transport
				out := t.TempDir()
				args := []string{"phase3", "https://phase3.test/sheets/abcdefgh", "--action", action, "--payload", payload, "--read-range", "Target!A1:A3", "--out", out, "--relogin", "--json"}
				a := newApp(args)
				a.Origin = "https://phase3.test"
				err := a.Run()
				if writes != 1 {
					t.Fatalf("writes=%d want1 err=%v", writes, err)
				}
				dirs, _ := os.ReadDir(out)
				if len(dirs) != 1 {
					t.Fatal("missing evidence dir")
				}
				dir := filepath.Join(out, dirs[0].Name())
				observation := map[string]any{"action": action, "fault": fault, "argv": args, "mutationRequests": writes, "httpTrace": trace, "appRunReturnedError": err != nil}
				if fault == "control" {
					if err != nil {
						t.Fatal(err)
					}
					archivePhase3Negative(t, dir, observation)
					return
				}
				if err == nil || !strings.Contains(err.Error(), "禁止自动重放") {
					t.Fatalf("failure not disclosed: %v", err)
				}
				observation["originalReturnedError"] = err.Error()
				if _, e := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(e) {
					t.Fatal("failed mutation has success result")
				}
				b, e := os.ReadFile(filepath.Join(dir, "failure.json"))
				if e != nil {
					t.Fatal(e)
				}
				var failure map[string]any
				if e = json.Unmarshal(b, &failure); e != nil {
					t.Fatal(e)
				}
				stage := "artifact-proof"
				if fault == "export-403" || fault == "empty-export" {
					stage = "after-export"
				}
				if fault == "readback-401" {
					stage = "after-readback"
				}
				if action == "sheet-delete" && fault == "no-op" {
					stage = "structural-proof"
				}
				if failure["stage"] != stage || failure["mayHaveApplied"] != true || failure["mutationAttempted"] != true || failure["ok"] != false {
					t.Fatalf("missing precise failure metadata: %s", b)
				}
				archivePhase3Negative(t, dir, observation)
			})
		}
	}
}
