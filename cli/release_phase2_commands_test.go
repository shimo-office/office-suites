package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReleasePhase2APICommandRequestsAndTruthfulOutput(t *testing.T) {
	for _, tc := range []struct{ action, payload, method, path, query string }{
		{"sheet-values", `{"sheetName":"工作表 A","range":"A1:B2"}`, "GET", "/lizard-api/files/abcdefgh/sheets/values", "工作表 A!A1:B2"},
		{"sheet-update", `{"sheetName":"工作表 A","range":"A1","resource":{"values":[["new"]]}}`, "POST", "/lizard-api/files/abcdefgh/sheets/values", ""},
		{"sheet-append", `{"range":"工作表 A!A1","resource":{"values":[["next"]]}}`, "PUT", "/lizard-api/files/abcdefgh/sheets/values", ""},
		{"sheet-insert", `{"sheetName":"工作表 A","index":2}`, "POST", "/lizard-api/files/abcdefgh/sheets/工作表 A/rows/2", "1"},
		{"sheet-delete", `{"sheetName":"工作表 A","index":2,"count":3}`, "DELETE", "/lizard-api/files/abcdefgh/sheets/工作表 A/rows/2", "3"},
		{"sheet-merge", `{"sheetName":"工作表 A","cell":"A1","rowspan":1,"colspan":2}`, "POST", "/lizard-api/files/abcdefgh/sheets/mergeCells", ""},
	} {
		t.Run(tc.action, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
			t.Setenv("SHIMO_NET_NO_BROWSER", "")
			t.Setenv("SHIMO_NET_VERIFY", "")
			old := httpClient
			defer func() { httpClient = old }()
			requests := 0
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.Method != tc.method || r.URL.Path != tc.path {
					t.Errorf("wrong request %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Cookie") != "sid=synthetic" {
					t.Error("missing synthetic auth")
				}
				if tc.action == "sheet-values" {
					if r.URL.Query().Get("range") != tc.query || r.Body != nil {
						t.Error("read request not encoded in query")
					}
				}
				if tc.action == "sheet-insert" || tc.action == "sheet-delete" {
					if r.URL.Query().Get("count") != tc.query || r.Body != nil {
						t.Error("row mutation used wrong count or body")
					}
				}
				if tc.action == "sheet-update" || tc.action == "sheet-append" {
					var body map[string]any
					if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
						t.Fatal(e)
					}
					if body["range"] != "工作表 A!A1" || body["resource"] == nil {
						t.Fatal(body)
					}
				}
				return phase3Response(204, ""), nil
			})}
			a := newApp([]string{"editor", "https://phase3.test/sheets/abcdefgh", "--action", tc.action, "--payload", tc.payload, "--no-verify", "--json"})
			a.Origin = "https://phase3.test"
			stdout, err := issueCapture(t, a.Run)
			if err != nil || requests != 1 {
				t.Fatal(err, requests)
			}
			var result map[string]any
			_ = json.Unmarshal([]byte(stdout), &result)
			if result["ok"] != true || result["verified"] != false || result["verifyWith"] != "host-browser" || result["screenshot"] != nil {
				t.Fatal("API response claimed visual acceptance", result)
			}
		})
	}
}

func TestReleasePhase2RejectsMalformedAPIRequestBeforeMutation(t *testing.T) {
	for _, tc := range []struct{ action, payload string }{
		{"sheet-update", `not-json`}, {"sheet-update", `[]`}, {"sheet-update", `[1]`}, {"sheet-update", `{"range":"A1","resource":{"values":[[1]]}}`}, {"sheet-update", `{"resource":{"values":[[1]]}}`}, {"sheet-update", `{"range":"Sheet1!A1","values":[[1]]}`}, {"sheet-update", `{"range":"Sheet1!A1","resource":{}}`},
		{"sheet-values", `{}`}, {"sheet-insert", `{"index":1}`}, {"sheet-delete", `{"sheetName":"Sheet1"}`}, {"sheet-format", `[]`}, {"sheet-format", `{"range":"A1","attrs":{"bold":true}}`}, {"sheet-format", `{"range":"Sheet1!A1"}`}, {"sheet-format", `{"range":"Sheet1!A1","attrs":{"fontSize":12}}`},
	} {
		t.Run(tc.action+tc.payload, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			old := httpClient
			defer func() { httpClient = old }()
			requests := 0
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				return nil, errors.New("unexpected mutation")
			})}
			a := newApp([]string{"editor", "https://phase3.test/sheets/abcdefgh", "--action", tc.action, "--payload", tc.payload, "--no-verify", "--json"})
			a.Origin = "https://phase3.test"
			stdout, err := issueCapture(t, a.Run)
			if err == nil || requests != 0 || stdout != "" {
				t.Fatalf("invalid input performed request or success: %d %v %s", requests, err, stdout)
			}
		})
	}
}

func TestReleasePhase2FormattingStopsAtFirstFailedAttribute(t *testing.T) {
	for _, mode := range []string{"success", "server-error", "transport-error"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			old := httpClient
			defer func() { httpClient = old }()
			attrs := []string{}
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["range"] != "Sheet1!A1:B1" {
					t.Fatal(body)
				}
				a := body["attrs"].(map[string]any)
				if len(a) != 1 {
					t.Fatal("attrs not split")
				}
				for k := range a {
					attrs = append(attrs, k)
				}
				if len(attrs) == 2 {
					if mode == "server-error" {
						return phase3Response(403, `denied`), nil
					}
					if mode == "transport-error" {
						return nil, errors.New("synthetic network failure")
					}
				}
				return phase3Response(200, `{"accepted":true}`), nil
			})}
			a := newApp([]string{"editor", "https://phase3.test/sheets/abcdefgh", "--action", "sheet-format", "--payload", `{"sheetName":"Sheet1","range":"A1:B1","attrs":{"italic":true,"bold":true,"background":"#FFFFFF"}}`, "--no-verify", "--json"})
			a.Origin = "https://phase3.test"
			stdout, err := issueCapture(t, a.Run)
			if mode == "success" {
				if err != nil || !reflect.DeepEqual(attrs, []string{"background", "bold", "italic"}) {
					t.Fatal(attrs, err)
				}
				return
			}
			if err == nil || !reflect.DeepEqual(attrs, []string{"background", "bold"}) || stdout != "" {
				t.Fatal("partial mutation falsely successful or continued", attrs, err, stdout)
			}
		})
	}
}

func TestReleasePhase2BrowserBoundaryClassification(t *testing.T) {
	for _, mode := range []string{"api-json", "api-text", "panel-json", "panel-text", "http-verified", "http-text", "browser-failure", "panel-no-verify"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_NO_BROWSER", "")
			t.Setenv("SHIMO_NET_VERIFY", "")
			t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
			old, oldBrowser := httpClient, phase2RunBrowser
			defer func() { httpClient = old; phase2RunBrowser = oldBrowser }()
			browserCalls, requests := 0, 0
			phase2RunBrowser = func(target, method string, args []any, shot string) (any, error) {
				browserCalls++
				if mode == "browser-failure" {
					return nil, errors.New("synthetic browser verification failed")
				}
				if strings.HasPrefix(method, "api:") && shot != "" {
					t.Error("read API requested screenshot")
				}
				if shot != "" {
					if err := os.WriteFile(shot, []byte("synthetic screenshot boundary"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				return map[string]any{"value": "synthetic"}, nil
			}
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				return phase3Response(200, `{"accepted":true}`), nil
			})}
			action := "sheet-update"
			payload := `{"range":"Sheet1!A1","resource":{"values":[[1]]}}`
			if strings.HasPrefix(mode, "api") {
				action = "mentions-list"
				payload = `{}`
			}
			if strings.HasPrefix(mode, "panel") {
				action = "history-show"
				payload = `[]`
			}
			args := []string{"editor", "https://phase3.test/sheets/abcdefgh", "--action", action, "--payload", payload, "--screenshot", filepath.Join(t.TempDir(), "proof.png")}
			if mode != "panel-no-verify" {
				args = append(args, "--verify")
			}
			if !strings.HasSuffix(mode, "text") {
				args = append(args, "--json")
			}
			a := newApp(args)
			a.Origin = "https://phase3.test"
			stdout, err := issueCapture(t, a.Run)
			if mode == "panel-no-verify" {
				if err == nil || browserCalls != 0 || requests != 0 {
					t.Fatal(err, browserCalls, requests)
				}
				return
			}
			if mode == "browser-failure" {
				if err == nil || stdout != "" || browserCalls != 1 || requests != 1 {
					t.Fatal(err, stdout, browserCalls, requests)
				}
				return
			}
			if err != nil || browserCalls != 1 {
				t.Fatal(err, browserCalls)
			}
			if strings.HasSuffix(mode, "text") {
				if strings.Contains(mode, "panel") && !strings.Contains(stdout, "前端 SDK 动作已执行") {
					t.Fatal(stdout)
				}
				if strings.Contains(mode, "api") && !strings.Contains(stdout, "只读 API 已读取") {
					t.Fatal(stdout)
				}
				if mode == "http-text" && !strings.Contains(stdout, "请核对页面结果") {
					t.Fatal(stdout)
				}
				return
			}
			var result map[string]any
			_ = json.Unmarshal([]byte(stdout), &result)
			if strings.HasPrefix(mode, "panel") && (result["actionVerified"] != false || result["scope"] != "page-sdk") {
				t.Fatal(result)
			}
			if strings.HasPrefix(mode, "api") && (result["verifyWith"] != "page-fetch" || result["screenshot"] != nil) {
				t.Fatal(result)
			}
		})
	}
}

func TestReleasePhase2HTTPErrorAndPlainBody(t *testing.T) {
	for _, mode := range []string{"transport", "denied", "plain-body"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			old := httpClient
			defer func() { httpClient = old }()
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if mode == "transport" {
					return nil, io.ErrUnexpectedEOF
				}
				if mode == "denied" {
					return phase3Response(403, "denied"), nil
				}
				return phase3Response(200, "non-json acknowledgement"), nil
			})}
			a := newApp([]string{"editor", "https://phase3.test/sheets/abcdefgh", "--action", "sheet-values", "--payload", `{"range":"Sheet1!A1"}`, "--no-verify", "--json"})
			a.Origin = "https://phase3.test"
			stdout, err := issueCapture(t, a.Run)
			if mode == "plain-body" {
				if err != nil || !strings.Contains(stdout, `"verified": false`) {
					t.Fatal(stdout, err)
				}
			} else if err == nil || stdout != "" {
				t.Fatal(stdout, err)
			}
		})
	}
}
