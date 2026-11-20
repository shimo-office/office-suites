package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIssue3EditorVerifyFailureRetainsMutation(t *testing.T) {
	for _, action := range []string{"sheet-append", "sheet-insert", "sheet-values"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
				calls++
				return phase3Response(200, `{"accepted":true}`), nil
			})
			t.Setenv("SHIMO_NET_NO_BROWSER", "")
			old := phase2RunBrowser
			t.Cleanup(func() { phase2RunBrowser = old })
			phase2RunBrowser = func(string, string, []any, string) (any, error) { return nil, errors.New("screenshot unavailable") }
			payload := `{"range":"Sheet1!A1","resource":{"values":[["next"]]}}`
			if action == "sheet-insert" {
				payload = `{"sheetName":"Sheet1","index":1}`
			}
			a := newApp([]string{"editor", "https://shimo.net/sheets/abcdefgh", "--action", action, "--payload", payload, "--verify", "--json", "--relogin"})
			out, err := issueCapture(t, a.Run)
			if err == nil || out != "" || calls != 1 {
				t.Fatalf("calls=%d out=%q err=%v", calls, out, err)
			}
			var detail *commandError
			if action == "sheet-values" {
				if errors.As(err, &detail) && detail.NoReplay {
					t.Fatal("read failure labeled mutation")
				}
				return
			}
			if !errors.As(err, &detail) || !detail.NoReplay || detail.Result["mutationAttempted"] != true || detail.Result["stage"] != "verify" || detail.Result["guid"] != "abcdefgh" || detail.Result["result"] == nil || !strings.Contains(err.Error(), "禁止自动重放") {
				t.Fatalf("lost mutation identity/result: %#v %v", detail, err)
			}
		})
	}
}

func TestIssue3FormatFailureReportsAcceptedAttributes(t *testing.T) {
	calls := []string{}
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		for k := range p["attrs"].(map[string]any) {
			calls = append(calls, k)
		}
		if len(calls) == 2 {
			return phase3Response(503, `unavailable`), nil
		}
		return phase3Response(200, `{}`), nil
	})
	a := newApp([]string{"editor", "https://shimo.net/sheets/abcdefgh", "--action", "sheet-format", "--payload", `{"range":"Sheet1!A1","attrs":{"italic":true,"background":"#fff","bold":true}}`, "--json", "--no-verify"})
	err := a.Run()
	var detail *commandError
	if !errors.As(err, &detail) || !detail.NoReplay || !reflect.DeepEqual(detail.Result["applied"], []string{"background"}) || detail.Result["failedAttribute"] != "bold" || !reflect.DeepEqual(calls, []string{"background", "bold"}) || !strings.Contains(err.Error(), "background") {
		t.Fatalf("detail=%#v calls=%v err=%v", detail, calls, err)
	}
}

func TestIssue3CreatedFileReadbackFailureRetainsIdentity(t *testing.T) {
	for _, cmd := range []string{"new", "mkdir"} {
		t.Run(cmd, func(t *testing.T) {
			creates, reads := 0, 0
			issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, "/create/") {
					creates++
					response := phase3Response(302, "")
					kind := "docs"
					if cmd == "mkdir" {
						kind = "folder"
					}
					response.Header.Set("Location", "/"+kind+"/abcdefgh")
					return response, nil
				}
				reads++
				return phase3Response(503, `{}`), nil
			})
			a := newApp([]string{cmd, "test", "--json", "--relogin"})
			out, err := issueCapture(t, a.Run)
			var detail *commandError
			if !errors.As(err, &detail) || !detail.NoReplay || detail.Result["created"] != true || detail.Result["guid"] != "abcdefgh" || detail.Result["stage"] != "readback" || creates != 1 || reads != 1 || out != "" {
				t.Fatalf("detail=%#v calls=%d/%d out=%q err=%v", detail, creates, reads, out, err)
			}
		})
	}
}

func TestIssue3UploadRejectsMalformedSuccess(t *testing.T) {
	for _, body := range []string{`[]`, `null`, `"done"`, `{}`, `{"guid":12}`} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			issueFileTransport(t, func(r *http.Request) (*http.Response, error) { calls++; return phase3Response(200, body), nil })
			p := filepath.Join(t.TempDir(), "file.md")
			if err := os.WriteFile(p, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
			out, err := issueCapture(t, newApp([]string{"upload", p, "--to", "desktop", "--json"}).Run)
			var detail *commandError
			if !errors.As(err, &detail) || !detail.NoReplay || detail.Result["mutationAttempted"] != true || calls != 1 || out != "" {
				t.Fatalf("malformed upload success calls=%d out=%q err=%v", calls, out, err)
			}
		})
	}
}

func TestIssue3VerificationExportNeverFallsBack(t *testing.T) {
	for _, kind := range []string{"xlsx", "docx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			attempts := []string{}
			typ := map[string]string{"xlsx": "spreadsheet", "docx": "documentPro", "pptx": "presentation"}[kind]
			issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/api/v1/files/abcdefgh" {
					return phase3Response(200, `{"guid":"abcdefgh","name":"test","type":"`+typ+`"}`), nil
				}
				if strings.Contains(r.URL.Path, "/files/export/") {
					var p map[string]any
					_ = json.NewDecoder(r.Body).Decode(&p)
					attempts = append(attempts, str(p, "type"))
					return phase3Response(400, `not supported`), nil
				}
				t.Fatalf("unexpected request: %s", r.URL)
				return nil, errors.New("unexpected")
			})
			a := newApp(nil)
			var err error
			if kind == "xlsx" {
				_, err = a.phase3Export("abcdefgh", t.TempDir())
			} else {
				_, err = a.phase3OfficeExport("abcdefgh", t.TempDir(), kind)
			}
			if err == nil || !reflect.DeepEqual(attempts, []string{kind}) {
				t.Fatalf("unexpected fallback attempts=%v err=%v", attempts, err)
			}
		})
	}
}

func TestIssue3EditorWriteTransportFailureIsNotReplayable(t *testing.T) {
	calls := 0
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("connection lost after request")
	})
	a := newApp([]string{"editor", "https://shimo.net/sheets/abcdefgh", "--action", "sheet-append", "--payload", `{"range":"Sheet1!A1","resource":{"values":[[1]]}}`, "--json", "--no-verify", "--relogin"})
	err := a.Run()
	var detail *commandError
	if !errors.As(err, &detail) || !detail.NoReplay || detail.Result["stage"] != "write" || detail.Result["mayHaveApplied"] != true || calls != 1 {
		t.Fatalf("calls=%d detail=%#v err=%v", calls, detail, err)
	}
}

func TestIssue3BrowserSessionFailuresStopBeforeAction(t *testing.T) {
	for _, mode := range []string{"set-cookie-failure", "reload-failure", "success"} {
		t.Run(mode, func(t *testing.T) {
			calls := []string{}
			ws := releasePhase2Peer(t, func(method string, p map[string]any) (map[string]any, error) {
				calls = append(calls, method)
				if mode == "set-cookie-failure" && method == "Network.setCookies" {
					return nil, errors.New("synthetic-cookie-secret")
				}
				if mode == "reload-failure" && method == "Page.reload" {
					return nil, errors.New("synthetic-cookie-secret")
				}
				return map[string]any{}, nil
			})
			defer ws.Close()
			err := initializePhase2Session(ws, "https://shimo.net/sheets/abcdefgh", "sid=synthetic-cookie-secret")
			if mode == "success" {
				if err != nil || !reflect.DeepEqual(calls, []string{"Network.setCookies", "Page.reload"}) {
					t.Fatal(calls, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "会话") || strings.Contains(err.Error(), "synthetic-cookie-secret") {
				t.Fatal(err)
			}
			want := []string{"Network.setCookies"}
			if mode == "reload-failure" {
				want = append(want, "Page.reload")
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatal(calls)
			}
		})
	}
}

func TestIssue3MissingBrowserSessionFailsBeforeLaunching(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_COOKIE", "")
	t.Setenv("SHIMO_NET_ORIGIN", "https://shimo.net")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	t.Setenv("PATH", t.TempDir())
	_, err := runPhase2Browser("https://shimo.net/sheets/abcdefgh", "actions.showViewHistory", nil, "")
	if err == nil || !strings.Contains(err.Error(), "会话读取失败") || !strings.Contains(err.Error(), "未登录") {
		t.Fatalf("missing auth obscured by browser readiness: %v", err)
	}
}
