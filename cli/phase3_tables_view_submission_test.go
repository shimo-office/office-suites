package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type tableViewFailureGuard struct{ tableTestGuard }

func (g *tableViewFailureGuard) ViewProof(map[string]any) (any, error) {
	return nil, errors.New("verification must fail before browser proof")
}

func TestPhase3NewViewPartialComposeNeverReplays(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	table := tableFixtureForSnapshot()
	table["baseGuid"] = "base12345678"
	posts := 0
	mutated := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/files/base12345678":
			fmt.Fprint(w, `{"guid":"base12345678","type":"table"}`)
		case "/tables/base12345678":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/v2/api","SDK_V2_SIGNATURE":"test-signature","SDK_V2_TOKEN":"test-token"}}];`)
		case "/sdk/v2/api/table-api/bases/base12345678/rev":
			rev := 1
			if mutated {
				rev = 2
			}
			json.NewEncoder(w).Encode(map[string]any{"rev": rev, "filePermission": map[string]any{"canRead": true, "canEdit": true}})
		case "/sdk/v2/api/table-api/tables/t1":
			json.NewEncoder(w).Encode(table)
		case "/sdk/v2/api/table-api/bases/base12345678/compose":
			posts++
			var body struct {
				Change []phase3TableChange `json:"change"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if len(body.Change) != 2 || body.Change[0].Action != "CreateView" || body.Change[1].Action != "UpdateViewSort" {
				t.Error("not one complete planned compose")
			}
			var create map[string]any
			json.Unmarshal([]byte(body.Change[0].Data), &create)
			// Fault injection: create persisted, following explicit sort did not.
			view := map[string]any{"guid": create["viewGuid"], "tableGuid": "t1", "order": float64(1), "name": create["name"], "type": "Grid", "desc": "", "fields": []any{"f1", "f2"}, "records": []any{"r1", "r2"}, "filters": nil, "groups": []any{}, "sort": map[string]any{"rules": []any{}, "keepSorted": false}, "permission": nil, "fieldInfo": create["fieldInfo"]}
			table["views"] = append(table["views"].([]any), view)
			mutated = true
			fmt.Fprint(w, `{"status":"normal"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := phase3OpenTableGuard
	defer func() { phase3OpenTableGuard = old }()
	phase3OpenTableGuard = func(string) (phase3TableGuard, error) { return &tableViewFailureGuard{}, nil }
	app := &App{Origin: srv.URL, JSON: true, Pos: []string{srv.URL + "/tables/base12345678"}, Flags: map[string]string{"--action": "table-view-create", "--payload": `{"tableGuid":"t1","viewGuid":"v1","name":"new grid","type":"Grid"}`, "--out": t.TempDir()}}
	if err := app.cmdPhase3TableEdit(); err == nil {
		t.Fatal("partially persisted view compose reported success")
	}
	if posts != 1 {
		t.Fatal("possibly submitted mutation replayed", posts)
	}
}
