package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func tableFixtureForEdit() map[string]any {
	var x map[string]any
	_ = json.Unmarshal([]byte(`{"guid":"t1","baseGuid":"b1","name":"before","desc":"old","fields":[{"guid":"f1","name":"text","type":"SingleLineText","isPrimary":true},{"guid":"f2","name":"number","type":"Number"}],"views":[{"guid":"v1","order":0,"fields":["f1","f2"],"records":["r1","r2"]}],"records":[{"guid":"r1","valueMap":{"f1":"todo","f2":1}},{"guid":"r2","valueMap":{"f1":"done","f2":2}}]}`), &x)
	return x
}
func TestPhase3TablePlanRejectsUnsupportedBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		action  string
		payload map[string]any
	}{
		{"table-update", map[string]any{"tableGuid": "t1", "name": ""}},
		{"table-record-add", map[string]any{"tableGuid": "t1", "records": []any{map[string]any{"unknown": "text"}}}},
		{"table-record-update", map[string]any{"tableGuid": "t1", "where": map[string]any{}, "values": map[string]any{"f1": "oops"}}},
		{"table-field-add", map[string]any{"tableGuid": "t1", "fields": []any{map[string]any{"name": "bad", "type": "Formula"}}}},
	} {
		if _, err := preparePhase3TableEdit(tc.action, tc.payload, tableFixtureForEdit()); err == nil {
			t.Fatalf("accepted invalid %s", tc.action)
		}
	}
}

func TestPhase3TableDateConvertsISOToNativeOADays(t *testing.T) {
	before := tableFixtureForEdit()
	p, err := preparePhase3TableEdit("table-field-add", map[string]any{"tableGuid": "t1", "fields": []any{map[string]any{"name": "日期", "type": "Date"}}}, before)
	if err != nil {
		t.Fatal(err)
	}
	field := p.Expected["fields"].(map[string]any)[p.CreatedIDs[0]].(map[string]any)
	if field["props"].(map[string]any)["dateFormat"] != "yyyy/M/d" {
		t.Fatal("missing native date format")
	}
	fields := map[string]map[string]any{"date": {"guid": "date", "type": "Date"}}
	normalized, err := phase3NormalizeTableValues(map[string]any{"date": "1970-01-01T08:00:00+08:00"}, fields)
	if err != nil || normalized["date"] != float64(25569) {
		t.Fatalf("wrong native date conversion: %v %v", normalized, err)
	}
	if err = phase3TableValues(normalized, fields); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []any{"2026-11-18", float64(1794950400000), "not-a-date"} {
		if _, err = phase3NormalizeTableValues(map[string]any{"date": bad}, fields); err == nil {
			t.Fatalf("accepted ambiguous/invalid date %v", bad)
		}
	}
	if err = phase3TableValues(map[string]any{"date": float64(1794950400000)}, fields); err == nil {
		t.Fatal("Unix milliseconds mistaken for native OA days")
	}
}
func TestPhase3TableConditionalUpdateAndReadback(t *testing.T) {
	before := tableFixtureForEdit()
	payload := map[string]any{"tableGuid": "t1", "where": map[string]any{"f1": "todo", "f2": float64(1)}, "values": map[string]any{"f1": "doing"}}
	plan, err := preparePhase3TableEdit("table-record-update", payload, before)
	if err != nil {
		t.Fatal(err)
	}
	after := tableFixtureForEdit()
	after["records"].([]any)[0].(map[string]any)["valueMap"].(map[string]any)["f1"] = "doing"
	if err := verifyPhase3TableEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
	after["records"].([]any)[1].(map[string]any)["valueMap"].(map[string]any)["f1"] = "doing"
	if err := verifyPhase3TableEdit(before, after, plan); err == nil {
		t.Fatal("unrelated record mutation accepted")
	}
	if !reflect.DeepEqual(before, tableFixtureForEdit()) {
		t.Fatal("plan mutated baseline")
	}
}
func TestPhase3TableRenameRequiresExactPostcondition(t *testing.T) {
	before := tableFixtureForEdit()
	plan, err := preparePhase3TableEdit("table-update", map[string]any{"tableGuid": "t1", "name": "after", "desc": "new"}, before)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyPhase3TableEdit(before, tableFixtureForEdit(), plan); err == nil {
		t.Fatal("unchanged data accepted")
	}
	after := tableFixtureForEdit()
	after["name"] = "after"
	after["desc"] = "new"
	if err := verifyPhase3TableEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
}

type tableTestGuard struct{ err error }

func (g *tableTestGuard) CheckPresence() error { return g.err }
func (g *tableTestGuard) Close()               {}

type tableJoiningGuard struct{ checks int }

func (g *tableJoiningGuard) Close() {}
func (g *tableJoiningGuard) CheckPresence() error {
	g.checks++
	if g.checks >= 3 {
		return errors.New("another editor joined while revision request was in flight")
	}
	return nil
}

func TestPhase3TableComposeProtocolAndPresenceGate(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	baseline := tableFixtureForSnapshot()
	baseline["baseGuid"] = "base12345678"
	mutated := false
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/files/base12345678":
			json.NewEncoder(w).Encode(map[string]any{"guid": "base12345678", "type": "table"})
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
			out := phase3TableCopy(baseline)
			if mutated {
				out["name"] = "after"
			}
			json.NewEncoder(w).Encode(out)
		case "/sdk/v2/api/table-api/bases/base12345678/compose":
			posts++
			if r.Method != "POST" || r.Header.Get("x-weboffice-signature") != "test-signature" {
				t.Error("wrong SDK auth or verb")
			}
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("bad JSON")
			}
			if body["baseRev"] != float64(1) {
				t.Error("missing revision precondition")
			}
			changes, _ := body["change"].([]any)
			if len(changes) != 1 {
				t.Error("wrong change count")
			} else {
				m := changes[0].(map[string]any)
				var d map[string]any
				err := json.Unmarshal([]byte(str(m, "data")), &d)
				if err != nil || str(m, "action") != "UpdateTableName" || str(d, "name") != "after" {
					t.Error("wrong server delta")
				}
			}
			mutated = true
			fmt.Fprint(w, `{"status":"normal"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := phase3OpenTableGuard
	defer func() { phase3OpenTableGuard = old }()
	app := &App{Origin: srv.URL, JSON: true, Pos: []string{srv.URL + "/tables/base12345678"}, Flags: map[string]string{"--action": "table-update", "--payload": `{"tableGuid":"t1","name":"after"}`, "--out": t.TempDir()}}
	phase3OpenTableGuard = func(string) (phase3TableGuard, error) {
		return &tableTestGuard{err: errors.New("another editor online")}, nil
	}
	if err := app.cmdPhase3TableEdit(); err == nil || posts != 0 {
		t.Fatal("presence failure did not block write")
	}
	phase3OpenTableGuard = func(string) (phase3TableGuard, error) { return &tableJoiningGuard{}, nil }
	if err := app.cmdPhase3TableEdit(); err == nil || posts != 0 {
		t.Fatal("presence change during revision request did not block write")
	}
	phase3OpenTableGuard = func(string) (phase3TableGuard, error) { return &tableTestGuard{}, nil }
	if err := app.cmdPhase3TableEdit(); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatalf("compose retried: %d", posts)
	}
}

func TestPhase3TableRecordAddPreservesExistingValues(t *testing.T) {
	before := tableFixtureForEdit()
	plan, err := preparePhase3TableEdit("table-record-add", map[string]any{"tableGuid": "t1", "records": []any{map[string]any{"f1": "new", "f2": float64(3)}, map[string]any{"f1": "second", "f2": float64(4)}}}, before)
	if err != nil {
		t.Fatal(err)
	}
	after := tableFixtureForEdit()
	records := after["records"].([]any)
	for i, id := range plan.CreatedIDs {
		records = append(records, map[string]any{"guid": id, "valueMap": map[string]any{"f1": []string{"new", "second"}[i], "f2": float64(i + 3)}})
	}
	after["records"] = records
	view := after["views"].([]any)[0].(map[string]any)
	order := view["records"].([]any)
	for _, id := range plan.CreatedIDs {
		order = append(order, id)
	}
	view["records"] = order
	if err := verifyPhase3TableEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
	after["records"] = records[:len(records)-1]
	if err := verifyPhase3TableEdit(before, after, plan); err == nil {
		t.Fatal("partial batch accepted")
	}
}
func TestPhase3TableFieldAddChecksTypeAndIdentity(t *testing.T) {
	before := tableFixtureForEdit()
	plan, err := preparePhase3TableEdit("table-field-add", map[string]any{"tableGuid": "t1", "fields": []any{map[string]any{"name": "new text", "type": "SingleLineText"}, map[string]any{"name": "new number", "type": "Number"}}}, before)
	if err != nil {
		t.Fatal(err)
	}
	after := tableFixtureForEdit()
	fields := after["fields"].([]any)
	for i, id := range plan.CreatedIDs {
		fields = append(fields, map[string]any{"guid": id, "name": []string{"new text", "new number"}[i], "type": []string{"SingleLineText", "Number"}[i], "props": nil, "isPrimary": false})
	}
	after["fields"] = fields
	view := after["views"].([]any)[0].(map[string]any)
	order := view["fields"].([]any)
	for _, id := range plan.CreatedIDs {
		order = append(order, id)
	}
	view["fields"] = order
	if err := verifyPhase3TableEdit(before, after, plan); err != nil {
		t.Fatal(err)
	}
	fields[len(fields)-1].(map[string]any)["type"] = "SingleLineText"
	if err := verifyPhase3TableEdit(before, after, plan); err == nil {
		t.Fatal("wrong persisted field type accepted")
	}
}

func TestPhase3TableChoiceFieldUsesRealOptionReferences(t *testing.T) {
	plan, err := preparePhase3TableEdit("table-field-add", map[string]any{"tableGuid": "t1", "fields": []any{map[string]any{"name": "status", "type": "SingleChoice", "options": []any{"待办", "进行中"}}}}, tableFixtureForEdit())
	if err != nil {
		t.Fatal(err)
	}
	field := plan.Expected["fields"].(map[string]any)[plan.CreatedIDs[0]].(map[string]any)
	props := field["props"].(map[string]any)
	choices := props["choices"].(map[string]any)
	if len(choices) != 2 {
		t.Fatal("option refs missing")
	}
	fields := map[string]map[string]any{"status": field}
	for id := range choices {
		if err := phase3TableValues(map[string]any{"status": id}, fields); err != nil {
			t.Fatal(err)
		}
	}
	if err := phase3TableValues(map[string]any{"status": "待办"}, fields); err == nil {
		t.Fatal("label accepted as option ID")
	}
}

func tableFixtureForSnapshot() map[string]any {
	table := tableFixtureForEdit()
	for _, raw := range phase3TableRows(table, "views") {
		raw.(map[string]any)["tableGuid"] = table["guid"]
	}
	return table
}
