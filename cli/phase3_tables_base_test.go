package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPhase3BaseLifecyclePlans(t *testing.T) {
	table := tableFixtureForEdit()
	tables := map[string]map[string]any{"t1": table}
	p, err := preparePhase3BaseEdit("table-create", map[string]any{"name": "new"}, "b1", tables)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 1 || p.Changes[0].Action != "CreateTable" {
		t.Fatalf("wrong plan: %+v", p)
	}
	var d map[string]any
	_ = json.Unmarshal([]byte(p.Changes[0].Data), &d)
	if d["baseGuid"] != "b1" || d["targetGuid"] != "t1" || len(d["cols"].([]any)) != 1 {
		t.Fatalf("wrong native payload: %+v", d)
	}
	if _, err = preparePhase3BaseEdit("table-delete", map[string]any{"tableGuid": "t1"}, "b1", tables); err == nil {
		t.Fatal("last table deletion allowed")
	}
	if _, err = preparePhase3BaseEdit("table-create", map[string]any{"name": "new", "execute": "anything"}, "b1", tables); err == nil {
		t.Fatal("unknown payload accepted")
	}
	tables["t2"] = phase3TableCopy(table)
	tables["t2"]["guid"] = "t2"
	if _, err = preparePhase3BaseEdit("table-delete", map[string]any{"tableGuid": "missing"}, "b1", tables); err == nil {
		t.Fatal("missing table deletion accepted")
	}
	p, err = preparePhase3BaseEdit("table-delete", map[string]any{"tableGuid": "t2"}, "b1", tables)
	if err != nil || p.Changes[0].Action != "DeleteTable" {
		t.Fatalf("delete: %+v %v", p, err)
	}
}

func TestPhase3BaseLifecyclePreservesOtherTables(t *testing.T) {
	a := tableFixtureForEdit()
	b := phase3TableCopy(a)
	b["guid"] = "t2"
	before := map[string]map[string]any{"t1": a, "t2": b}
	p, err := preparePhase3BaseEdit("table-delete", map[string]any{"tableGuid": "t2"}, "b1", before)
	if err != nil {
		t.Fatal(err)
	}
	after := map[string]map[string]any{"t1": phase3TableCopy(a)}
	if err = verifyPhase3BaseEdit(before, after, p); err != nil {
		t.Fatal(err)
	}
	after["t1"]["name"] = "unexpected"
	if err = verifyPhase3BaseEdit(before, after, p); err == nil {
		t.Fatal("unrelated table change accepted")
	}
}

func TestPhase3BaseDuplicateProvesValues(t *testing.T) {
	source := tableFixtureForEdit()
	target := phase3TableCopy(source)
	target["guid"] = "copy"
	target["name"] = "new name"
	if err := verifyPhase3TableDuplicate(source, target); err != nil {
		t.Fatal(err)
	}
	target["records"].([]any)[1].(map[string]any)["valueMap"].(map[string]any)["f2"] = float64(999)
	if err := verifyPhase3TableDuplicate(source, target); err == nil {
		t.Fatal("corrupted copied content accepted")
	}
}

func TestPhase3BaseDuplicateDoesNotRewriteOrdinaryText(t *testing.T) {
	source := tableFixtureForEdit()
	source["records"].([]any)[0].(map[string]any)["valueMap"].(map[string]any)["f1"] = "t1"
	target := phase3TableCopy(source)
	target["guid"] = "copy"
	target["name"] = "copied"
	target["records"].([]any)[0].(map[string]any)["valueMap"].(map[string]any)["f1"] = "copy"
	if err := verifyPhase3TableDuplicate(source, target); err == nil {
		t.Fatal("ordinary text was normalized as an identity and corruption passed")
	}
}

func TestPhase3BaseDeleteHTTPProtocol(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	a := tableFixtureForSnapshot()
	a["baseGuid"] = "base12345678"
	a["tableType"] = float64(0)
	b := phase3TableCopy(a)
	b["guid"] = "t2"
	b["views"].([]any)[0].(map[string]any)["tableGuid"] = "t2"
	posts := 0
	changed := false
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
			if changed {
				rev = 2
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"rev": rev, "filePermission": map[string]any{"canRead": true, "canEdit": true}})
		case "/sdk/v2/api/table-api/v2/bases/base12345678":
			tables := []any{a}
			if !changed {
				tables = append(tables, b)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tables": tables})
		case "/sdk/v2/api/table-api/tables/t1":
			_ = json.NewEncoder(w).Encode(a)
		case "/sdk/v2/api/table-api/tables/t2":
			_ = json.NewEncoder(w).Encode(b)
		case "/sdk/v2/api/table-api/bases/base12345678/compose":
			posts++
			if r.Method != "POST" || r.Header.Get("x-weboffice-signature") != "test-signature" {
				t.Error("wrong signed native request")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			changes, _ := body["change"].([]any)
			if body["baseRev"] != float64(1) || len(changes) != 1 {
				t.Error("missing native revision/change")
			} else {
				c := changes[0].(map[string]any)
				var data map[string]any
				_ = json.Unmarshal([]byte(str(c, "data")), &data)
				if str(c, "action") != "DeleteTable" || str(data, "tableGuid") != "t2" {
					t.Error("wrong table mutation")
				}
			}
			changed = true
			fmt.Fprint(w, `{"status":"normal"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := phase3OpenTableGuard
	defer func() { phase3OpenTableGuard = old }()
	app := &App{Origin: srv.URL, JSON: true, Pos: []string{srv.URL + "/tables/base12345678"}, Flags: map[string]string{"--action": "table-delete", "--payload": `{"tableGuid":"t2"}`, "--out": t.TempDir()}}
	phase3OpenTableGuard = func(string) (phase3TableGuard, error) { return &tableTestGuard{err: errors.New("other editor")}, nil }
	if err := app.cmdPhase3BaseEdit(); err == nil || posts != 0 {
		t.Fatal("presence did not prevent write")
	}
	phase3OpenTableGuard = func(string) (phase3TableGuard, error) { return &tableTestGuard{}, nil }
	if err := app.cmdPhase3BaseEdit(); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatalf("mutation replayed %d times", posts)
	}
	changed = false
	posts = 0
	phase3OpenTableGuard = func(string) (phase3TableGuard, error) { return &tableAfterSubmitGuard{changed: &changed}, nil }
	err := app.cmdPhase3BaseEdit()
	if err == nil || posts != 1 || !strings.Contains(err.Error(), "已提交") || !strings.Contains(err.Error(), "禁止自动重放") || !strings.Contains(err.Error(), "证据目录") {
		t.Fatalf("submitted failure not explicit: posts=%d err=%v", posts, err)
	}
}

type tableAfterSubmitGuard struct{ changed *bool }

func (g *tableAfterSubmitGuard) Close() {}
func (g *tableAfterSubmitGuard) CheckPresence() error {
	if *g.changed {
		return errors.New("拒绝写入：另一个编辑者已进入")
	}
	return nil
}
