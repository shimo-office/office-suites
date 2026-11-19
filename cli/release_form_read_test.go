package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are offline command integrations: signed SDK responses are synthetic,
// while the command, pagination, validation, CSV and evidence writes are real.
func TestReleaseFormReadCommands(t *testing.T) {
	for _, mode := range []string{"inspect", "unpublished", "results", "empty", "duplicate-question", "bad-question", "missing-questions", "bad-publication", "bad-publication-status", "bad-statistics", "count-mismatch", "missing-views", "page-failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			old := httpClient
			defer func() { httpClient = old }()
			writes := 0
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					writes++
					t.Errorf("read action wrote %s %s", r.Method, r.URL.Path)
				}
				reply := func(v any) (*http.Response, error) {
					b, _ := json.Marshal(v)
					return phase3Response(200, string(b)), nil
				}
				switch r.URL.Path {
				case "/api/v1/files/abcdefgh":
					return phase3Response(200, `{"type":"form"}`), nil
				case "/forms/abcdefgh":
					return phase3Response(200, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/","SDK_V2_SIGNATURE":"test","SDK_V2_TOKEN":"test"}}];`), nil
				case "/sdk/forms/abcdefgh/publications":
					if mode == "unpublished" {
						return reply(map[string]any{"errorCode": "20221"})
					}
					if mode == "bad-publication" {
						return reply([]any{})
					}
					qs := []any{map[string]any{"guid": "q001", "subject": "Quoted, question"}, map[string]any{"guid": "q002", "subject": "Unanswered"}}
					if mode == "duplicate-question" {
						qs = append(qs, qs[0])
					}
					if mode == "bad-question" {
						qs[0] = map[string]any{"subject": "missing identity"}
					}
					f := map[string]any{"questions": qs}
					if mode == "missing-questions" {
						delete(f, "questions")
					}
					raw, _ := json.Marshal(f)
					status := "normal"
					if mode == "bad-publication-status" {
						status = "unknown"
					}
					return reply(map[string]any{"status": status, "publishedContent": string(raw), "publishedRev": 1})
				case "/sdk/forms/abcdefgh/responses":
					if mode == "page-failure" {
						return phase3Response(403, `{}`), nil
					}
					if r.URL.Query().Get("offset") != "0" || r.URL.Query().Get("limit") != "100" {
						t.Errorf("bad pagination %s", r.URL.RawQuery)
					}
					rows := []any{map[string]any{"submitId": "submission-one", "content": []any{map[string]any{"guid": "q001", "text": map[string]any{"content": "hello,\n世界"}}}}}
					if mode == "empty" {
						rows = []any{}
					}
					return reply(map[string]any{"total": len(rows), "responses": rows})
				case "/sdk/forms/abcdefgh/statistics":
					if mode == "bad-statistics" {
						return reply([]any{})
					}
					total := 1
					if mode == "empty" {
						total = 0
					}
					if mode == "count-mismatch" {
						total = 2
					}
					m := map[string]any{"responses": total, "views": 4}
					if mode == "missing-views" {
						delete(m, "views")
					}
					return reply(m)
				}
				t.Errorf("unexpected path %s", r.URL.Path)
				return phase3Response(500, `{}`), nil
			})}
			action := "form-results"
			if mode == "inspect" || mode == "unpublished" {
				action = "form-inspect"
			}
			out := t.TempDir()
			a := newApp([]string{"phase3", "https://phase3.test/forms/abcdefgh", "--action", action, "--out", out, "--json"})
			a.Origin = "https://phase3.test"
			stdout, err := issueCapture(t, a.Run)
			success := mode == "inspect" || mode == "unpublished" || mode == "results" || mode == "empty"
			if (err == nil) != success {
				t.Fatalf("success=%t err=%v stdout=%s", success, err, stdout)
			}
			if writes != 0 {
				t.Fatal("read command mutated server")
			}
			if !success {
				if strings.Contains(stdout, `"ok": true`) {
					t.Fatal("failure emitted success")
				}
				return
			}
			var result map[string]any
			if json.Unmarshal([]byte(stdout), &result) != nil || result["ok"] != true || result["mutationAttempted"] != false {
				t.Fatal(stdout)
			}
			if mode == "unpublished" && result["published"] != false {
				t.Fatal(result)
			}
			if action == "form-results" {
				raw, e := os.ReadFile(str(result, "csv"))
				if e != nil {
					t.Fatal(e)
				}
				rows, e := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
				if e != nil {
					t.Fatal(e)
				}
				want := 2
				if mode == "empty" {
					want = 1
					if result["acceptanceNote"] == nil {
						t.Fatal("empty response set presented as acceptance")
					}
				}
				if len(rows) != want || len(rows[0]) != 5 || rows[0][3] != "q001" || rows[0][4] != "q002" {
					t.Fatal(rows)
				}
				if mode == "results" && (!strings.Contains(rows[1][3], "世界") || rows[1][4] != "") {
					t.Fatal(rows)
				}
				files, _ := filepath.Glob(filepath.Join(out, "*", "results.json"))
				if len(files) != 1 {
					t.Fatalf("missing results evidence %v", files)
				}
			}
		})
	}
}

func TestReleaseFormPublicationInputRejectsNotificationSideEffects(t *testing.T) {
	for _, field := range []string{"subscribeNotification", "reminder"} {
		b := map[string]any{"status": "unpublished", field: "enabled"}
		if _, err := phase3FormPublishPayload(b, formEditFixture(), float64(42)); err == nil {
			t.Fatalf("%s side effect accepted", field)
		}
	}
	for i, b := range []map[string]any{{"status": "invalid"}, {"status": "normal", "isQuiz": true}, {"status": "normal", "isQuiz": float64(1)}} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, err := phase3FormPublishPayload(b, formEditFixture(), float64(42)); err == nil {
				t.Fatal("unsupported publication accepted")
			}
		})
	}
}
