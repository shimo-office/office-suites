package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
)

// Exercise the actual HTTP pagination loop, including a differently encoded
// second page. Helper-only tests cannot detect an incorrect offset or an early
// return that silently drops later responses.
func TestPhase3FormResultsHTTPPagination(t *testing.T) {
	for _, scenario := range []string{"complete", "duplicate", "changed-total", "truncated", "html", "bad-answer"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=test")
			offsets := []int{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/sdk/forms/form12345678/responses" || r.URL.Query().Get("formGuid") != "form12345678" || r.URL.Query().Get("limit") != "100" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
				if err != nil || offset < 0 || offset > 200 {
					t.Errorf("invalid offset %q", r.URL.Query().Get("offset"))
					http.Error(w, "bad offset", http.StatusBadRequest)
					return
				}
				offsets = append(offsets, offset)
				if offset == 100 && scenario == "html" {
					w.Header().Set("Content-Type", "text/html")
					fmt.Fprint(w, "<html>sign in</html>")
					return
				}
				total := 205
				rows := []map[string]any{}
				for i := offset; i < offset+100 && i < total; i++ {
					id := fmt.Sprintf("submission-%03d", i)
					if offset == 100 && i == 100 && scenario == "duplicate" {
						id = "submission-000"
					}
					var content any = []any{map[string]any{"guid": "question-1", "text": fmt.Sprintf("答案-%d", i)}}
					if offset == 100 && i == 100 && scenario == "bad-answer" {
						content = "not-json"
					}
					rows = append(rows, map[string]any{"submitId": id, "content": content})
				}
				if offset == 100 && scenario == "changed-total" {
					total++
				}
				if offset == 100 && scenario == "truncated" {
					rows = []map[string]any{}
				}
				body, err := json.Marshal(map[string]any{"total": total, "responses": rows})
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if offset == 100 {
					w.Header().Set("X-Content-Encoding", "base64")
					fmt.Fprint(w, base64.StdEncoding.EncodeToString(body))
					return
				}
				_, _ = w.Write(body)
			}))
			defer srv.Close()
			s := &phase3SDKSession{app: &App{Origin: srv.URL}, guid: "form12345678", prefix: "/sdk/"}
			rows, total, err := s.formResults()
			if scenario != "complete" {
				if err == nil || rows != nil || total != 0 {
					t.Fatalf("partial data reported as complete: rows=%d total=%d err=%v", len(rows), total, err)
				}
				if !reflect.DeepEqual(offsets, []int{0, 100}) {
					t.Fatalf("unexpected failure requests %v", offsets)
				}
				return
			}
			if err != nil || total != 205 || len(rows) != 205 || !reflect.DeepEqual(offsets, []int{0, 100, 200}) {
				t.Fatalf("incomplete pagination rows=%d total=%d offsets=%v err=%v", len(rows), total, offsets, err)
			}
			for i, row := range rows {
				if str(row, "submitId") != fmt.Sprintf("submission-%03d", i) {
					t.Fatalf("lost/reordered response at %d", i)
				}
			}
		})
	}
}
