package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPhase3TableSnapshotHydratesEachViewAtOneRevision(t *testing.T) {
	for _, mode := range []string{"ok", "changed-revision", "wrong-view", "wrong-table", "missing-order", "foreign-record", "changed-name", "missing-name", "loaded-foreign", "loaded-duplicate", "loaded-wrong-table", "changed-nil-permission"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=test")
			gets, views, posts := 0, 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method != "GET" {
					posts++
				}
				switch r.URL.Path {
				case "/sdk/table-api/bases/b1/rev":
					gets++
					rev := 1
					if mode == "changed-revision" && gets > 1 {
						rev = 2
					}
					json.NewEncoder(w).Encode(map[string]any{"rev": rev, "filePermission": map[string]any{"canRead": true, "canEdit": false}})
				case "/sdk/table-api/tables/t1":
					table := tableFixtureForSnapshot()
					v1 := phase3TableMap(table, "views")["v1"]
					switch mode {
					case "loaded-foreign":
						v1["records"] = []any{"other"}
					case "loaded-duplicate":
						v1["fields"] = []any{"f1", "f1"}
					case "loaded-wrong-table":
						v1["tableGuid"] = "other"
					}
					table["views"] = append(table["views"].([]any), map[string]any{"guid": "v2", "tableGuid": "t1", "name": "second", "permission": nil, "records": nil})
					json.NewEncoder(w).Encode(table)
				case "/sdk/table-api/tables/t1/views/v2":
					views++
					v := map[string]any{"guid": "v2", "tableGuid": "t1", "name": "second", "permission": nil, "fields": []any{"f2", "f1"}, "records": []any{"r2", "r1"}}
					switch mode {
					case "wrong-view":
						v["guid"] = "v3"
					case "wrong-table":
						v["tableGuid"] = "t2"
					case "missing-order":
						delete(v, "fields")
					case "foreign-record":
						v["records"] = []any{"other"}
					case "changed-name":
						v["name"] = "different"
					case "missing-name":
						delete(v, "name")
					case "changed-nil-permission":
						v["permission"] = map[string]any{"type": "Personal"}
					}
					json.NewEncoder(w).Encode(v)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			app := newApp([]string{})
			app.Origin = srv.URL
			s := &phase3SDKSession{app: app, guid: "b1", prefix: "/sdk/", headers: map[string]string{}}
			result, err := s.tableSnapshot("t1")
			if mode != "ok" {
				if err == nil {
					t.Fatal("unsafe partial snapshot accepted", mode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			v := phase3TableMap(result, "views")
			if v["v1"]["records"].([]any)[0] != "r1" || v["v2"]["records"].([]any)[0] != "r2" || v["v2"]["fields"].([]any)[0] != "f2" {
				t.Fatal("view-specific ordering overwritten")
			}
			if gets != 2 || views != 1 || posts != 0 {
				t.Fatalf("wrong readonly calls rev=%d view=%d posts=%d", gets, views, posts)
			}
		})
	}
}

func TestPhase3NewViewAcceptsOnlyAbsentEmptyGroups(t *testing.T) {
	for _, value := range []any{nil, []any{}, "", map[string]any{}, []any{map[string]any{"fieldGuid": "f1"}}} {
		plan := phase3TableEditPlan{NewViews: map[string]map[string]any{"v": {"groups": []any{}, "fields": []any{}}}}
		after := map[string]any{"views": []any{map[string]any{"guid": "v", "groups": value, "fields": []any{}}}}
		err := verifyPhase3NewViews(after, plan)
		allowed := value == nil
		if a, ok := value.([]any); ok && len(a) == 0 {
			allowed = true
		}
		if (err == nil) != allowed {
			t.Fatalf("wrong empty-group semantics: %#v %v", value, err)
		}
	}
}
