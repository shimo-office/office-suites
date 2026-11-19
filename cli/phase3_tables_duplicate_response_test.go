package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPhase3DuplicateEmptyAckRequiresIndependentVerification(t *testing.T) {
	for _, status := range []int{200, 204} {
		ack, err := phase3DuplicateResponse(&Response{Status: status, Raw: []byte(" \n")})
		if err != nil {
			t.Fatal(err)
		}
		value := ack.(map[string]any)
		if value["requiresReadback"] != true || value["verified"] != nil || value["ok"] != nil {
			t.Fatal("empty ack masqueraded as verified", value)
		}
	}
	for _, r := range []*Response{{Status: 200, Raw: []byte("<html>login</html>")}, {Status: 400}, {Status: 200, Raw: []byte("null")}, {Status: 200, Data: map[string]any{"code": float64(1)}}, {Status: 200, Data: map[string]any{}}} {
		if _, err := phase3DuplicateResponse(r); err == nil {
			t.Fatal("invalid acknowledgement accepted", r.Status)
		}
	}
	if _, err := phase3DuplicateResponse(&Response{Status: 200, Raw: []byte(`{"code":0}`), Data: map[string]any{"code": float64(0)}}); err != nil {
		t.Fatal(err)
	}
}

func TestPhase3DuplicateEmptyAckStillChecksPersistence(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	for _, persist := range []bool{true, false} {
		t.Run(fmt.Sprint(persist), func(t *testing.T) {
			source := tableFixtureForSnapshot()
			source["baseGuid"] = "base12345678"
			source["tableType"] = float64(0)
			var copied map[string]any
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/files/base12345678":
					fmt.Fprint(w, `{"guid":"base12345678","type":"table"}`)
				case "/tables/base12345678":
					fmt.Fprint(w, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/v2/api","SDK_V2_SIGNATURE":"test-signature","SDK_V2_TOKEN":"test-token"}}];`)
				case "/sdk/v2/api/table-api/bases/base12345678/rev":
					rev := 1
					if copied != nil {
						rev = 2
					}
					json.NewEncoder(w).Encode(map[string]any{"rev": rev, "filePermission": map[string]any{"canRead": true, "canEdit": true}})
				case "/sdk/v2/api/table-api/v2/bases/base12345678":
					tables := []any{source}
					if copied != nil {
						tables = append(tables, copied)
					}
					json.NewEncoder(w).Encode(map[string]any{"tables": tables})
				case "/sdk/v2/api/table-api/tables/t1":
					json.NewEncoder(w).Encode(source)
				case "/sdk/v2/api/table-api/bases/base12345678/duplicateTable":
					posts++
					if r.Method != "POST" {
						t.Error("wrong verb")
					}
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if persist {
						copied = phase3TableCopy(source)
						copied["guid"] = body["tableGuid"]
						copied["views"].([]any)[0].(map[string]any)["tableGuid"] = body["tableGuid"]
						copied["name"] = body["tableName"]
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					if copied != nil && r.URL.Path == "/sdk/v2/api/table-api/tables/"+str(copied, "guid") {
						json.NewEncoder(w).Encode(copied)
					} else {
						http.NotFound(w, r)
					}
				}
			}))
			defer server.Close()
			original := phase3OpenTableGuard
			defer func() { phase3OpenTableGuard = original }()
			phase3OpenTableGuard = func(string) (phase3TableGuard, error) { return &tableTestGuard{}, nil }
			app := &App{Origin: server.URL, JSON: true, Pos: []string{server.URL + "/tables/base12345678"}, Flags: map[string]string{"--action": "table-duplicate", "--payload": `{"tableGuid":"t1","name":"copied"}`, "--out": t.TempDir()}}
			err := app.cmdPhase3BaseEdit()
			if persist && err != nil {
				t.Fatal(err)
			}
			if !persist && err == nil {
				t.Fatal("empty acknowledgement accepted without persisted copy")
			}
			if posts != 1 {
				t.Fatal("duplicate replayed", posts)
			}
		})
	}
}
