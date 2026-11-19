package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type releaseTablesGuard struct {
	t                                *testing.T
	mode                             string
	writes                           *int
	fresh                            bool
	checks, captures, proofs, closed int
}

func (g *releaseTablesGuard) Close() { g.closed++ }
func (g *releaseTablesGuard) CheckPresence() error {
	g.checks++
	if g.mode == "presence" || g.mode == "joined-before-submit" && g.checks >= 3 || g.mode == "joined-after-submit" && *g.writes > 0 {
		return errors.New("another synthetic editor is present")
	}
	return nil
}
func (g *releaseTablesGuard) NavigateTable(table string) error {
	if table != "t1" {
		g.t.Fatal("wrong navigation target", table)
	}
	if g.mode == "navigate" {
		return errors.New("navigation failed")
	}
	return nil
}
func (g *releaseTablesGuard) Capture(path string) error {
	g.captures++
	if g.mode == "before-capture" && g.captures == 1 || g.mode == "after-capture" && g.captures == 2 || g.mode == "reopened-capture" && g.captures == 3 {
		return errors.New("synthetic screenshot failure")
	}
	return os.WriteFile(path, releaseFlowPNG(g.t), 0600)
}
func (g *releaseTablesGuard) FreshCapture(path string) error {
	g.fresh = true
	if g.mode == "fresh-capture" {
		return errors.New("fresh editor failed")
	}
	return os.WriteFile(path, releaseFlowPNG(g.t), 0600)
}
func (g *releaseTablesGuard) ViewProof(spec map[string]any) (any, error) {
	g.proofs++
	if g.mode == "view-proof" || g.mode == "reopened-view" && g.proofs == 2 {
		return nil, errors.New("rendered records mismatch")
	}
	if str(spec, "viewGuid") == "" || len(spec["expectedRecords"].(map[string]any)) != 2 {
		g.t.Fatal("view proof scope missing", spec)
	}
	return map[string]any{"syntheticProof": true, "viewGuid": spec["viewGuid"], "records": spec["expectedRecords"]}, nil
}
func (g *releaseTablesGuard) BrowserTimezone() (string, error) {
	if g.mode == "timezone" {
		return "", errors.New("timezone unavailable")
	}
	return "Asia/Shanghai", nil
}

func releaseTablesEditFixture() map[string]any {
	table := tableFixtureForSnapshot()
	table["baseGuid"] = "abcdefgh"
	table["tableType"] = float64(0)
	view := table["views"].([]any)[0].(map[string]any)
	view["name"], view["type"], view["desc"] = "Original", "Grid", ""
	view["filters"], view["groups"], view["sort"], view["permission"] = nil, []any{}, map[string]any{"rules": []any{}, "keepSorted": true}, nil
	view["fieldInfo"] = map[string]any{"f1": map[string]any{"visible": true}, "f2": map[string]any{"visible": true}, "date": map[string]any{"visible": true}}
	table["fields"] = append(table["fields"].([]any), map[string]any{"guid": "date", "name": "Date", "type": "Date"})
	view["fields"] = append(view["fields"].([]any), "date")
	for _, raw := range table["records"].([]any) {
		raw.(map[string]any)["valueMap"].(map[string]any)["date"] = float64(45000)
	}
	return table
}
func releaseTablesApplyCompose(t *testing.T, table map[string]any, changes []phase3TableChange) {
	t.Helper()
	for _, change := range changes {
		var d map[string]any
		if err := json.Unmarshal([]byte(change.Data), &d); err != nil {
			t.Fatal(err)
		}
		switch change.Action {
		case "UpdateTableName":
			table["name"] = d["name"]
		case "UpdateTableDesc":
			table["desc"] = d["desc"]
		case "UpdateColumn":
			field := phase3TableMap(table, "fields")[str(d, "colGuid")]
			for _, key := range []string{"name", "type", "props"} {
				if value, ok := d[key]; ok {
					field[key] = value
				}
			}
		case "UpdateCell":
			for id, raw := range d["rows"].(map[string]any) {
				row := phase3TableMap(table, "records")[id]["valueMap"].(map[string]any)
				for field, value := range raw.(map[string]any) {
					row[field] = value
				}
			}
		case "DeleteView":
			if len(d["views"].([]any)) != 1 || str(d["views"].([]any)[0].(map[string]any), "viewGuid") != "v2" {
				t.Fatal("unexpected deletion target")
			}
			table["views"] = table["views"].([]any)[:1]
		case "UpdateViewName", "UpdateViewFilters", "UpdateViewGroups", "UpdateViewSort":
			key := map[string]string{"UpdateViewName": "name", "UpdateViewFilters": "filters", "UpdateViewGroups": "groups", "UpdateViewSort": "sort"}[change.Action]
			view := phase3TableMap(table, "views")[str(d, "viewGuid")]
			if view == nil {
				t.Fatal("compose targets missing view")
			}
			view[key] = d[key]
			if d["records"] != nil {
				view["records"] = d["records"]
			}
		case "CreateView":
			if str(d, "fromViewGuid") != "v1" || str(d, "targetGuid") != "v1" {
				t.Fatal("create view lost source anchor")
			}
			v := map[string]any{"guid": d["viewGuid"], "tableGuid": "t1", "name": d["name"], "type": d["type"], "desc": "", "order": float64(1), "fields": d["colsOrder"], "records": []any{"r1", "r2"}, "fieldInfo": d["fieldInfo"], "filters": nil, "groups": d["groups"], "permission": nil, "sort": map[string]any{"rules": []any{}, "keepSorted": false}}
			if d["calendar"] != nil {
				v["calendar"] = d["calendar"]
			}
			if d["kanban"] != nil {
				v["kanban"] = d["kanban"]
			}
			table["views"] = append(table["views"].([]any), v)
		case "DeleteColumn":
			ids := map[string]bool{}
			for _, id := range d["colGuids"].([]any) {
				ids[id.(string)] = true
			}
			fields := []any{}
			for _, raw := range table["fields"].([]any) {
				if !ids[str(raw.(map[string]any), "guid")] {
					fields = append(fields, raw)
				}
			}
			table["fields"] = fields
			for _, raw := range table["records"].([]any) {
				for id := range ids {
					delete(raw.(map[string]any)["valueMap"].(map[string]any), id)
				}
			}
			for _, raw := range table["views"].([]any) {
				v := raw.(map[string]any)
				order := []any{}
				for _, id := range v["fields"].([]any) {
					if !ids[id.(string)] {
						order = append(order, id)
					}
				}
				v["fields"] = order
				for id := range ids {
					delete(v["fieldInfo"].(map[string]any), id)
				}
			}
		default:
			t.Fatalf("unexpected compose action %s", change.Action)
		}
	}
}
func TestReleaseTablesEditCommandLifecycle(t *testing.T) {
	for _, scenario := range []struct{ name, action, payload string }{
		{"rename", "table-update", `{"tableGuid":"t1","name":"Renamed","desc":"Changed description"}`},
		{"grid-create", "table-view-create", `{"tableGuid":"t1","viewGuid":"v1","name":"New Grid","type":"Grid"}`},
		{"calendar-create", "table-view-create", `{"tableGuid":"t1","viewGuid":"v1","name":"New Calendar","type":"Calendar","startFieldGuid":"date","endFieldGuid":"date"}`},
		{"view-update", "table-view-update", `{"tableGuid":"t1","viewGuid":"v1","name":"Sorted","sort":{"rules":[{"fieldGuid":"f2","order":"Desc"}],"keepSorted":true},"groups":[],"filters":null}`},
		{"field-delete", "table-field-delete", `{"tableGuid":"t1","fieldGuids":["f2"]}`},
		{"field-convert", "table-field-convert", `{"tableGuid":"t1","fieldGuid":"f2","type":"SingleLineText"}`},
		{"field-rename", "table-field-update", `{"tableGuid":"t1","fieldGuid":"f2","name":"Renamed column"}`},
		{"record-update", "table-record-update", `{"tableGuid":"t1","where":{"f1":"todo"},"values":{"f2":12}}`},
		{"view-delete", "table-view-delete", `{"tableGuid":"t1","viewGuid":"v2"}`},
	} {
		t.Run(scenario.name, func(t *testing.T) { runReleaseTablesEdit(t, scenario.action, scenario.payload, "success", true) })
	}
	for _, mode := range []string{"open", "presence", "permission", "missing-table-guid", "invalid-payload", "navigate", "before-capture", "revision-changed", "joined-before-submit", "mutation-403", "mutation-ambiguous", "joined-after-submit", "no-op", "readback-401", "revision-not-advanced", "after-capture", "fresh-capture", "fresh-content-changed", "fresh-readback-401"} {
		t.Run(mode, func(t *testing.T) {
			runReleaseTablesEdit(t, "table-update", `{"tableGuid":"t1","name":"Renamed","desc":"Changed description"}`, mode, false)
		})
	}
	for _, mode := range []string{"view-proof", "reopened-view", "reopened-capture", "missing-view-bridge", "missing-view-guid"} {
		t.Run(mode, func(t *testing.T) {
			runReleaseTablesEdit(t, "table-view-create", `{"tableGuid":"t1","viewGuid":"v1","name":"New Grid","type":"Grid"}`, mode, false)
		})
	}
	for _, mode := range []string{"timezone", "bad-calendar-base", "missing-timezone-bridge"} {
		t.Run(mode, func(t *testing.T) {
			runReleaseTablesEdit(t, "table-view-create", `{"tableGuid":"t1","viewGuid":"v1","name":"New Calendar","type":"Calendar","startFieldGuid":"date","endFieldGuid":"date"}`, mode, false)
		})
	}
}

type releaseTablesNoTimezone struct{ g *releaseTablesGuard }

func (g *releaseTablesNoTimezone) CheckPresence() error                    { return g.g.CheckPresence() }
func (g *releaseTablesNoTimezone) Close()                                  { g.g.Close() }
func (g *releaseTablesNoTimezone) ViewProof(s map[string]any) (any, error) { return g.g.ViewProof(s) }
func runReleaseTablesEdit(t *testing.T, action, payload, mode string, wantSuccess bool) {
	t.Helper()
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	oldClient, oldGuard := httpClient, phase3OpenTableGuard
	defer func() { httpClient = oldClient; phase3OpenTableGuard = oldGuard }()
	table := releaseTablesEditFixture()
	if action == "table-view-delete" {
		v := phase3TableCopy(phase3TableMap(table, "views")["v1"])
		v["guid"], v["name"], v["order"] = "v2", "Disposable view", float64(1)
		table["views"] = append(table["views"].([]any), v)
	}
	writes, revReads := 0, 0
	g := &releaseTablesGuard{t: t, mode: mode, writes: &writes}
	phase3OpenTableGuard = func(target string) (phase3TableGuard, error) {
		if target != "https://phase3.test/tables/abcdefgh" {
			t.Fatal("wrong target", target)
		}
		if mode == "open" {
			return nil, errors.New("editor failed")
		}
		if mode == "missing-view-bridge" {
			return &tableTestGuard{}, nil
		}
		if mode == "missing-timezone-bridge" {
			return &releaseTablesNoTimezone{g: g}, nil
		}
		return g, nil
	}
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		reply := func(value any) (*http.Response, error) {
			b, e := json.Marshal(value)
			if e != nil {
				t.Fatal(e)
			}
			return phase3Response(200, string(b)), nil
		}
		switch r.URL.Path {
		case "/api/v1/files/abcdefgh":
			return phase3Response(200, `{"type":"table"}`), nil
		case "/tables/abcdefgh":
			return phase3Response(200, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/","SDK_V2_SIGNATURE":"test","SDK_V2_TOKEN":"test"}}];`), nil
		case "/sdk/table-api/bases/abcdefgh/rev":
			revReads++
			rev := 1
			if writes > 0 && mode != "revision-not-advanced" {
				rev = 2
			}
			if mode == "revision-changed" && revReads >= 4 {
				rev = 2
			}
			return reply(map[string]any{"rev": rev, "filePermission": map[string]any{"canRead": true, "canEdit": mode != "permission"}})
		case "/sdk/table-api/tables/t1":
			if mode == "readback-401" && writes > 0 || mode == "fresh-readback-401" && g.fresh {
				return phase3Response(401, `{}`), nil
			}
			value := phase3TableCopy(table)
			if mode == "fresh-content-changed" && g.fresh {
				value["name"] = "unexpected drift"
			}
			return reply(value)
		case "/sdk/table-api/v2/bases/abcdefgh":
			if mode == "bad-calendar-base" {
				return reply([]any{})
			}
			return reply(map[string]any{"tables": []any{map[string]any{"guid": "t1", "tableType": float64(0)}}, "timezone": "Asia/Shanghai"})
		case "/sdk/table-api/bases/abcdefgh/compose":
			writes++
			if writes != 1 || r.Method != "POST" {
				t.Fatal("mutation replayed or wrong method")
			}
			var body struct {
				BaseRev int                 `json:"baseRev"`
				Change  []phase3TableChange `json:"change"`
			}
			if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
				t.Fatal(e)
			}
			if body.BaseRev != 1 || len(body.Change) == 0 {
				t.Fatal("unbounded mutation", body)
			}
			if mode == "mutation-403" {
				return phase3Response(403, `{}`), nil
			}
			if mode != "no-op" {
				releaseTablesApplyCompose(t, table, body.Change)
			}
			if mode == "mutation-ambiguous" {
				return reply(map[string]any{"status": "pending"})
			}
			return reply(map[string]any{"status": "normal"})
		default:
			t.Errorf("unexpected offline route %s", r.URL.Path)
			return phase3Response(500, `{}`), nil
		}
	})}
	if mode == "missing-table-guid" {
		payload = `{"name":"Renamed"}`
	}
	if mode == "invalid-payload" {
		payload = `{"tableGuid":"t1","name":5}`
	}
	if mode == "missing-view-guid" {
		payload = `{"tableGuid":"t1","name":"New Grid","type":"Grid"}`
	}
	out := t.TempDir()
	a := newApp([]string{"phase3", "https://phase3.test/tables/abcdefgh", "--action", action, "--payload", payload, "--out", out, "--relogin", "--json"})
	a.Origin = "https://phase3.test"
	stdout, err := issueCapture(t, a.Run)
	results, _ := filepath.Glob(filepath.Join(out, "*", "result.json"))
	failures, _ := filepath.Glob(filepath.Join(out, "*", "failure.json"))
	if wantSuccess {
		if err != nil || writes != 1 || len(results) != 1 || !g.fresh {
			t.Fatalf("success failed writes=%d result=%d fresh=%v err=%v", writes, len(results), g.fresh, err)
		}
		var result map[string]any
		if json.Unmarshal([]byte(stdout), &result) != nil || result["verified"] != true || result["afterRev"] != float64(2) {
			t.Fatal("invalid success", stdout)
		}
		if action == "table-view-create" || action == "table-view-update" {
			if g.proofs != 2 || g.captures != 3 {
				t.Fatal("missing current/fresh view proof", g.proofs, g.captures)
			}
		}
		if len(failures) != 0 {
			t.Fatal("success left failure evidence")
		}
	} else {
		if err == nil || len(results) != 0 {
			t.Fatalf("failure became success: %s err=%v", stdout, err)
		}
		post := mode == "mutation-403" || mode == "mutation-ambiguous" || mode == "joined-after-submit" || mode == "no-op" || mode == "readback-401" || mode == "revision-not-advanced" || mode == "after-capture" || mode == "fresh-capture" || mode == "fresh-content-changed" || mode == "fresh-readback-401" || mode == "view-proof" || mode == "reopened-view" || mode == "reopened-capture"
		expectedWrites := 0
		if post {
			expectedWrites = 1
		}
		if writes != expectedWrites {
			t.Fatalf("unexpected write count %d want %d: %v", writes, expectedWrites, err)
		}
		if post {
			if len(failures) != 1 || !strings.Contains(err.Error(), "禁止自动重放") {
				t.Fatalf("uncertain write not disclosed: %v", err)
			}
			raw, e := os.ReadFile(failures[0])
			if e != nil {
				t.Fatal(e)
			}
			var failure map[string]any
			_ = json.Unmarshal(raw, &failure)
			if failure["mutationAttempted"] != true || failure["mayHaveApplied"] != true {
				t.Fatal("wrong failure evidence", string(raw))
			}
		}
	}
	if mode != "open" && mode != "missing-view-bridge" && g.closed == 0 {
		t.Fatal("editor not closed")
	}
}
