package main

import (
	"encoding/json"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestReleaseTablesEditViewValidation(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"nonobject filter", "filters", true}, {"extra filter key", "filters", map[string]any{"conjunction": "And", "extra": true}}, {"unknown conjunction", "filters", map[string]any{"conjunction": "Xor", "conditions": []any{}}}, {"empty conditions", "filters", map[string]any{"conjunction": "And", "conditions": []any{}}}, {"nonobject condition", "filters", map[string]any{"conjunction": "And", "conditions": []any{1}}},
		{"nonarray groups", "groups", true}, {"too many groups", "groups", []any{nil, nil, nil, nil}}, {"nonobject group", "groups", []any{true}}, {"unknown group field", "groups", []any{map[string]any{"fieldGuid": "missing", "order": "Asc"}}}, {"extra group key", "groups", []any{map[string]any{"fieldGuid": "f1", "order": "Asc", "extra": true}}}, {"duplicate group", "groups", []any{map[string]any{"fieldGuid": "f1", "order": "Asc"}, map[string]any{"fieldGuid": "f1", "order": "Desc"}}}, {"invalid group order", "groups", []any{map[string]any{"fieldGuid": "f1", "order": "Up"}}}, {"unsupported group type", "groups", []any{map[string]any{"fieldGuid": "date", "order": "Asc"}}},
		{"nonobject sort", "sort", true}, {"extra sort key", "sort", map[string]any{"rules": []any{}, "keepSorted": true, "extra": true}}, {"nonbool keepSorted", "sort", map[string]any{"rules": []any{}, "keepSorted": "yes"}}, {"text sort", "sort", map[string]any{"rules": []any{map[string]any{"fieldGuid": "f1", "order": "Asc"}}, "keepSorted": true}},
		{"blank view name", "name", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{"tableGuid": "t1", "viewGuid": "v1", tc.key: tc.value}
			if _, err := preparePhase3TableEdit("table-view-update", payload, releaseTablesEditFixture()); err == nil {
				t.Fatal("invalid view update accepted")
			}
		})
	}
	for _, condition := range []map[string]any{
		{"fieldGuid": "f1", "operator": "Equal", "value": "todo", "extra": true},
		{"fieldGuid": "missing", "operator": "Equal", "value": 1},
		{"fieldGuid": "f1", "operator": "Contains", "value": "todo"},
		{"fieldGuid": "f1", "operator": "GreaterThan", "value": "todo"},
		{"fieldGuid": "f1", "operator": "Equal", "value": nil},
	} {
		payload := map[string]any{"tableGuid": "t1", "viewGuid": "v1", "filters": map[string]any{"conjunction": "Or", "conditions": []any{condition}}}
		if _, err := preparePhase3TableEdit("table-view-update", payload, releaseTablesEditFixture()); err == nil {
			t.Fatal("invalid condition accepted", condition)
		}
	}
	for _, tc := range []struct {
		name string
		edit func(map[string]any, map[string]any)
	}{
		{"unknown creation key", func(p, b map[string]any) { p["extra"] = true }},
		{"missing name", func(p, b map[string]any) { delete(p, "name") }},
		{"duplicate name", func(p, b map[string]any) { p["name"] = "Original" }},
		{"invalid order", func(p, b map[string]any) { phase3TableMap(b, "views")["v1"]["order"] = -1.0 }},
		{"grid has calendar fields", func(p, b map[string]any) { p["startFieldGuid"] = "date" }},
		{"kanban has calendar fields", func(p, b map[string]any) { p["type"] = "Kanban"; p["startFieldGuid"] = "date" }},
		{"kanban wrong field type", func(p, b map[string]any) { p["type"] = "Kanban"; p["groupFieldGuid"] = "f2" }},
		{"calendar has group field", func(p, b map[string]any) { p["type"] = "Calendar"; p["groupFieldGuid"] = "f2" }},
		{"unknown type", func(p, b map[string]any) { p["type"] = "Timeline" }},
		{"missing source field order", func(p, b map[string]any) { delete(phase3TableMap(b, "views")["v1"], "fields") }},
		{"missing source row order", func(p, b map[string]any) { delete(phase3TableMap(b, "views")["v1"], "records") }},
		{"private source view", func(p, b map[string]any) {
			phase3TableMap(b, "views")["v1"]["permission"] = map[string]any{"type": "Personal"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := map[string]any{"tableGuid": "t1", "viewGuid": "v1", "name": "New", "type": "Grid"}
			b := releaseTablesEditFixture()
			tc.edit(p, b)
			if _, err := preparePhase3TableEdit("table-view-create", p, b); err == nil {
				t.Fatal("unsupported view creation accepted")
			}
		})
	}
	for _, mode := range []string{"name-duplicate", "delete-extra", "delete-last-shared", "delete-default", "delete-shared"} {
		t.Run(mode, func(t *testing.T) {
			b := releaseTablesEditFixture()
			v2 := phase3TableCopy(phase3TableMap(b, "views")["v1"])
			v2["guid"], v2["name"], v2["order"] = "v2", "Other", float64(1)
			b["views"] = append(b["views"].([]any), v2)
			p := map[string]any{"tableGuid": "t1", "viewGuid": "v1"}
			action := "table-view-delete"
			switch mode {
			case "name-duplicate":
				p["name"] = "Other"
				action = "table-view-update"
			case "delete-extra":
				p["extra"] = true
			case "delete-last-shared":
				v2["permission"] = map[string]any{"type": "Personal"}
			case "delete-default":
				b["defaultViewGuid"] = "v1"
			case "delete-shared":
				phase3TableMap(b, "views")["v1"]["shareGuid"] = "shared"
			}
			if _, err := preparePhase3TableEdit(action, p, b); err == nil {
				t.Fatal("unsafe view operation accepted")
			}
		})
	}
	for _, mode := range []string{"missing-order", "unknown-row", "tied-values"} {
		t.Run(mode, func(t *testing.T) {
			b := releaseTablesEditFixture()
			v := phase3TableMap(b, "views")["v1"]
			switch mode {
			case "missing-order":
				delete(v, "records")
			case "unknown-row":
				v["records"] = []any{"missing"}
			case "tied-values":
				phase3TableMap(b, "records")["r2"]["valueMap"].(map[string]any)["f2"] = float64(1)
			}
			got, err := phase3SortedViewRecords(b, v, []any{map[string]any{"fieldGuid": "f2", "order": "Desc"}})
			if mode == "tied-values" {
				if err != nil || !reflect.DeepEqual(got, []any{"r1", "r2"}) {
					t.Fatal("stable tie order lost", got, err)
				}
			} else if err == nil {
				t.Fatal("invalid order accepted")
			}
		})
	}
}
func TestReleaseTablesEditReadCommand(t *testing.T) {
	for _, mode := range []string{"summary", "detail", "unknown-table", "bad-base", "missing-tables", "invalid-table", "revision-error", "runtime-error", "bad-snapshot"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			old := httpClient
			defer func() { httpClient = old }()
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					t.Fatal("read command wrote data", r.Method)
				}
				reply := func(v any) (*http.Response, error) {
					b, _ := json.Marshal(v)
					return phase3Response(200, string(b)), nil
				}
				switch r.URL.Path {
				case "/api/v1/files/abcdefgh":
					return phase3Response(200, `{"type":"table"}`), nil
				case "/tables/abcdefgh":
					if mode == "runtime-error" {
						return phase3Response(403, `{}`), nil
					}
					return phase3Response(200, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/","SDK_V2_SIGNATURE":"test","SDK_V2_TOKEN":"test"}}];`), nil
				case "/sdk/table-api/v2/bases/abcdefgh":
					if mode == "bad-base" {
						return reply([]any{})
					}
					if mode == "missing-tables" {
						return reply(map[string]any{})
					}
					if mode == "invalid-table" {
						return reply(map[string]any{"tables": []any{true}})
					}
					return reply(map[string]any{"tables": []any{map[string]any{"guid": "t1", "name": "Synthetic", "tableType": 0}}})
				case "/sdk/table-api/bases/abcdefgh/rev":
					if mode == "revision-error" {
						return phase3Response(403, `{}`), nil
					}
					return reply(map[string]any{"rev": 1, "filePermission": map[string]any{"canRead": true}})
				case "/sdk/table-api/tables/t1":
					if mode == "bad-snapshot" {
						return reply(map[string]any{"guid": "wrong"})
					}
					return reply(releaseTablesEditFixture())
				default:
					t.Fatal("unexpected request", r.URL.Path)
					return nil, nil
				}
			})}
			args := []string{"phase3", "https://phase3.test/tables/abcdefgh", "--action", "table-inspect", "--json"}
			if mode == "detail" || mode == "bad-snapshot" {
				args = append(args, "--table", "t1")
			}
			if mode == "unknown-table" {
				args = append(args, "--table", "missing")
			}
			a := newApp(args)
			a.Origin = "https://phase3.test"
			out, err := issueCapture(t, a.Run)
			if mode == "summary" || mode == "detail" {
				if err != nil {
					t.Fatal(err)
				}
				var result map[string]any
				if json.Unmarshal([]byte(out), &result) != nil || result["mutationAttempted"] != false || result["action"] != "table-inspect" {
					t.Fatal("false read proof", out)
				}
				if mode == "detail" && result["table"] == nil {
					t.Fatal("detail missing")
				}
			} else if err == nil {
				t.Fatal("bad read accepted", out)
			}
		})
	}
}
func TestReleaseTablesEditBoundsBeforePlanning(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any, map[string]any)
	}{
		{"wrong table", func(p, b map[string]any) { p["tableGuid"] = "wrong" }},
		{"missing views", func(p, b map[string]any) { b["views"] = []any{} }},
		{"invalid description", func(p, b map[string]any) { p["desc"] = true }},
		{"too long name", func(p, b map[string]any) { p["name"] = strings.Repeat("x", 10001) }},
		{"unknown property", func(p, b map[string]any) { p["private"] = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := map[string]any{"tableGuid": "t1", "name": "New"}
			b := releaseTablesEditFixture()
			tc.edit(p, b)
			if _, err := preparePhase3TableEdit("table-update", p, b); err == nil {
				t.Fatal("bad edit accepted")
			}
		})
	}
	plan := phase3TableEditPlan{}
	if err := plan.add("Synthetic", map[string]any{"value": math.NaN()}); err == nil {
		t.Fatal("unencodable delta accepted")
	}
}

func TestReleaseTablesEditFieldAndRecordInputContracts(t *testing.T) {
	for _, tc := range []struct {
		name, action string
		payload      map[string]any
	}{
		{"empty fields", "table-field-add", map[string]any{"fields": []any{}}},
		{"nonobject field", "table-field-add", map[string]any{"fields": []any{false}}},
		{"unknown field property", "table-field-add", map[string]any{"fields": []any{map[string]any{"name": "new", "type": "Number", "extra": true}}}},
		{"nonstring field name", "table-field-add", map[string]any{"fields": []any{map[string]any{"name": 1, "type": "Number"}}}},
		{"duplicate field name", "table-field-add", map[string]any{"fields": []any{map[string]any{"name": "text", "type": "Number"}}}},
		{"options on number", "table-field-add", map[string]any{"fields": []any{map[string]any{"name": "new", "type": "Number", "options": []any{"x"}}}}},
		{"empty options", "table-field-add", map[string]any{"fields": []any{map[string]any{"name": "new", "type": "SingleChoice", "options": []any{}}}}},
		{"duplicate options", "table-field-add", map[string]any{"fields": []any{map[string]any{"name": "new", "type": "SingleChoice", "options": []any{"x", "x"}}}}},
		{"empty records", "table-record-add", map[string]any{"records": []any{}}},
		{"nonobject record", "table-record-add", map[string]any{"records": []any{true}}},
		{"invalid date", "table-record-add", map[string]any{"records": []any{map[string]any{"date": "not-date"}}}},
		{"no update values", "table-record-update", map[string]any{"where": map[string]any{"f1": "todo"}}},
		{"unknown condition field", "table-record-update", map[string]any{"where": map[string]any{"unknown": "todo"}, "values": map[string]any{"f2": 3}}},
		{"invalid update date", "table-record-update", map[string]any{"where": map[string]any{"f1": "todo"}, "values": map[string]any{"date": "bad"}}},
		{"wrong condition type", "table-record-update", map[string]any{"where": map[string]any{"f2": "wrong"}, "values": map[string]any{"f2": 3}}},
		{"wrong value type", "table-record-update", map[string]any{"where": map[string]any{"f1": "todo"}, "values": map[string]any{"f2": "wrong"}}},
		{"no matching records", "table-record-update", map[string]any{"where": map[string]any{"f1": "absent"}, "values": map[string]any{"f2": 3}}},
		{"empty update", "table-update", map[string]any{}},
		{"unknown action", "unknown-action", map[string]any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.payload["tableGuid"] = "t1"
			if _, err := preparePhase3TableEdit(tc.action, tc.payload, releaseTablesEditFixture()); err == nil {
				t.Fatal("invalid write plan accepted")
			}
		})
	}
	fields := phase3TableMap(releaseTablesEditFixture(), "fields")
	if err := phase3TableValues(map[string]any{}, fields); err == nil {
		t.Fatal("empty values accepted")
	}
	if err := phase3TableValues(map[string]any{"f2": math.NaN()}, fields); err == nil {
		t.Fatal("NaN accepted")
	}
	fields["f2"]["type"] = "Unknown"
	if err := phase3TableValues(map[string]any{"f2": 1}, fields); err == nil {
		t.Fatal("unknown value type accepted")
	}
}
