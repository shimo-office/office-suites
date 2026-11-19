package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type releaseBaseProofGuard struct {
	tableTestGuard
	t           *testing.T
	fault       string
	writes      *int
	fresh       bool
	navigations []string
	captures    int
	chartProofs int
}

func (g *releaseBaseProofGuard) CheckPresence() error {
	if g.fault == "after-presence" && *g.writes > 0 {
		return errors.New("synthetic second editor joined")
	}
	return nil
}
func (g *releaseBaseProofGuard) NavigateTable(id string) error {
	g.navigations = append(g.navigations, id)
	if g.fault == "after-navigation" && *g.writes > 0 {
		return errors.New("synthetic navigation failure")
	}
	return nil
}
func (g *releaseBaseProofGuard) Capture(path string) error {
	g.captures++
	if g.fault == "before-screenshot" && *g.writes == 0 || g.fault == "after-screenshot" && *g.writes > 0 {
		return errors.New("synthetic screenshot capture failure")
	}
	if g.fault == "submission-evidence" && *g.writes == 0 {
		if err := os.Mkdir(filepath.Join(filepath.Dir(path), "submission.json"), 0700); err != nil {
			return err
		}
	}
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	// Deliberately synthetic offline image; no remote visual claim is made.
	return png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2)))
}
func (g *releaseBaseProofGuard) FreshCapture(path string) error {
	g.fresh = true
	if g.fault == "fresh-failure" {
		return errors.New("synthetic fresh editor failure")
	}
	return g.Capture(path)
}
func (g *releaseBaseProofGuard) ReferenceCapability(id string) (map[string]any, error) {
	if g.fault == "capability-error" {
		return nil, errors.New("synthetic capability read failure")
	}
	if g.fault == "capability-wrong-table" {
		id = "another-table"
	}
	return map[string]any{"tableGuid": id, "fieldType": "ReferenceRecord", "menuInitialized": true, "referenceTypeOffered": true, "renderedFieldTypes": []any{"SingleLineText", "ReferenceRecord"}}, nil
}
func (g *releaseBaseProofGuard) ChartProof(spec map[string]any, expected map[string]int) (any, error) {
	g.chartProofs++
	if g.fault == "before-chart-error" {
		return nil, errors.New("synthetic baseline chart missing")
	}
	if g.fault == "before-chart-invalid" {
		return "invalid", nil
	}
	if g.fault == "fresh-chart-error" && g.fresh {
		return nil, errors.New("synthetic fresh chart mismatch")
	}
	if !reflect.DeepEqual(expected, map[string]int{"todo": 1, "done": 1}) {
		g.t.Fatalf("wrong independently known counts: %v", expected)
	}
	if g.fault == "chart-failure" && *g.writes > 0 {
		return nil, errors.New("synthetic rendered counts disagree")
	}
	palette := []any{"before-red", "before-blue"}
	if *g.writes > 0 && g.fault != "unchanged-palette" {
		palette = []any{"after-green", "after-purple"}
	}
	return map[string]any{"categories": []any{"todo", "done"}, "values": []any{float64(1), float64(1)}, "palette": palette}, nil
}

func releaseBaseTable(id, name string) map[string]any {
	x := tableFixtureForSnapshot()
	x["guid"] = id
	x["name"] = name
	x["baseGuid"] = "base12345678"
	x["tableType"] = float64(0)
	for _, v := range phase3TableMap(x, "views") {
		v["tableGuid"] = id
	}
	return x
}

func releaseBaseLink(tables map[string]map[string]any, source, field, target, reverse string) {
	for _, side := range []struct{ table, id, other, back string }{{source, field, target, reverse}, {target, reverse, source, field}} {
		x := tables[side.table]
		x["fields"] = append(x["fields"].([]any), map[string]any{"guid": side.id, "name": "linked", "type": "ReferenceRecord", "isPrimary": false, "props": map[string]any{"baseGuid": "", "tableGuid": side.other, "foreignTableColumnGuid": side.back, "isMultiple": true, "isRefToView": false, "refViewGuid": "", "error": ""}})
		for _, v := range phase3TableMap(x, "views") {
			v["fields"] = append(v["fields"].([]any), side.id)
		}
	}
}

func releaseBaseDashboard() map[string]any {
	var d map[string]any
	_ = json.Unmarshal([]byte(`{"guid":"dashboard","baseGuid":"base12345678","name":"Chart","desc":"","tableType":4,"components":[{"componentGuid":"component","baseGuid":"base12345678","dashboardGuid":"dashboard","name":"Chart","grid":{"x":0,"y":0,"w":3,"h":2},"type":1,"subType":1,"dataSource":{"tableGuid":"t1","viewGuid":""},"axis":{"xAxis":{"fieldGuid":"f1","orderBy":0,"order":0},"yAxis":{"mode":"count","series":null}},"style":{"themeColor":"0","legend":{"enable":true},"label":{"enable":true},"statNumber":{"decimalPlace":0,"formatType":0,"description":""}}}],"positions":[{"componentGuid":"component","x":0,"y":0,"w":3,"h":2}],"refreshInterval":0,"shareConfig":{"enable":false,"shareGuid":"","shareMode":""}}`), &d)
	return d
}

func TestReleaseTablesBaseProductionPipeline(t *testing.T) {
	for _, tc := range []struct{ action, payload string }{
		{"table-create", `{"name":"Created","desc":"synthetic"}`},
		{"table-duplicate", `{"tableGuid":"t1","name":"Duplicated"}`},
		{"table-delete", `{"tableGuid":"t2"}`},
		{"table-chart-create", `{"tableGuid":"t1","fieldGuid":"f1","name":"New chart","type":"Column"}`},
		{"table-chart-style", `{"dashboardGuid":"dashboard","componentGuid":"component","themeColor":1}`},
		{"table-reference-add", `{"tableGuid":"t1","referenceTableGuid":"t2","name":"linked"}`},
		{"table-reference-set", `{"tableGuid":"t1","fieldGuid":"link","recordGuid":"r1","referenceRecordGuids":["r2"]}`},
	} {
		faults := []string{"control", "stale-revision", "after-presence", "fresh-failure", "reverted-reopen"}
		if tc.action == "table-chart-style" {
			faults = append(faults, "unchanged-palette", "chart-failure", "before-chart-error", "before-chart-invalid", "fresh-chart-error")
		}
		if tc.action == "table-create" {
			faults = append(faults, "after-navigation", "before-screenshot", "after-screenshot", "submission-evidence", "ack-failure", "rev-stalled", "readback-invalid")
		}
		if tc.action == "table-reference-add" {
			faults = append(faults, "capability-error", "capability-wrong-table")
		}
		for _, fault := range faults {
			t.Run(tc.action+"/"+fault, func(t *testing.T) {
				t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic-base-fixture")
				t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
				tables := map[string]map[string]any{"t1": releaseBaseTable("t1", "Source"), "t2": releaseBaseTable("t2", "Other")}
				if tc.action == "table-chart-style" {
					tables["dashboard"] = releaseBaseDashboard()
				}
				if tc.action == "table-reference-set" {
					releaseBaseLink(tables, "t1", "link", "t2", "back")
				}
				baseline := map[string]map[string]any{}
				for id, v := range tables {
					baseline[id] = phase3TableCopy(v)
				}
				writes, revisions := 0, 0
				guard := &releaseBaseProofGuard{t: t, fault: fault, writes: &writes}
				oldClient, oldGuard := httpClient, phase3OpenTableGuard
				t.Cleanup(func() { httpClient = oldClient; phase3OpenTableGuard = oldGuard })
				phase3OpenTableGuard = func(target string) (phase3TableGuard, error) {
					if target != "https://phase3.test/tables/base12345678" {
						t.Fatalf("wrong target %s", target)
					}
					return guard, nil
				}
				respond := func(v any) (*http.Response, error) {
					b, e := json.Marshal(v)
					if e != nil {
						return nil, e
					}
					return phase3Response(200, string(b)), nil
				}
				httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
					visible := tables
					if fault == "reverted-reopen" && guard.fresh {
						visible = baseline
					}
					const prefix = "/sdk/table-api/"
					switch {
					case r.URL.Path == "/api/v1/files/base12345678":
						return phase3Response(200, `{"guid":"base12345678","type":"table"}`), nil
					case r.URL.Path == "/tables/base12345678":
						return phase3Response(200, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/","SDK_V2_SIGNATURE":"synthetic-signature","SDK_V2_TOKEN":"synthetic-token"}}];`), nil
					case r.URL.Path == prefix+"bases/base12345678/rev":
						revisions++
						rev := 1
						if writes > 0 && fault != "rev-stalled" || fault == "stale-revision" && revisions > 1 {
							rev = 2
						}
						return respond(map[string]any{"rev": rev, "filePermission": map[string]any{"canRead": true, "canEdit": true}})
					case r.URL.Path == prefix+"v2/bases/base12345678":
						if fault == "readback-invalid" && writes > 0 {
							return respond(map[string]any{"tables": nil})
						}
						list := []any{}
						ids := []string{}
						for id := range visible {
							ids = append(ids, id)
						}
						sort.Strings(ids)
						for _, id := range ids {
							x := phase3TableCopy(visible[id])
							if num(x, "tableType") == 4 {
								delete(x, "positions")
							}
							list = append(list, x)
						}
						return respond(map[string]any{"tables": list})
					case strings.HasPrefix(r.URL.Path, prefix+"tables/"):
						id := strings.TrimPrefix(r.URL.Path, prefix+"tables/")
						if visible[id] == nil {
							return phase3Response(404, `{}`), nil
						}
						return respond(visible[id])
					case strings.HasPrefix(r.URL.Path, prefix+"bases/base12345678/dashboards/"):
						id := strings.TrimPrefix(r.URL.Path, prefix+"bases/base12345678/dashboards/")
						x := phase3TableCopy(visible[id])
						x["dashboardGuid"] = id
						return respond(x)
					case r.URL.Path == prefix+"bases/base12345678/duplicateTable":
						writes++
						var body map[string]any
						if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
							t.Fatal(e)
						}
						if r.Method != "POST" || str(body, "sourceTableGuid") != "t1" || str(body, "tableName") != "Duplicated" {
							t.Fatalf("wrong duplicate request %v", body)
						}
						id := str(body, "tableGuid")
						copy := phase3TableCopy(tables["t1"])
						copy["guid"] = id
						copy["name"] = "Duplicated"
						for _, v := range phase3TableMap(copy, "views") {
							v["tableGuid"] = id
						}
						tables[id] = copy
						return phase3Response(204, ""), nil
					case r.URL.Path == prefix+"bases/base12345678/compose":
						writes++
						if writes != 1 || r.Method != "POST" || r.Header.Get("x-weboffice-signature") != "synthetic-signature" {
							t.Fatalf("replay/unsigned write: %d", writes)
						}
						var body map[string]any
						if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
							t.Fatal(e)
						}
						if body["baseRev"] != float64(1) {
							t.Fatalf("wrong base revision %v", body)
						}
						for _, raw := range body["change"].([]any) {
							change := raw.(map[string]any)
							var d map[string]any
							if e := json.Unmarshal([]byte(str(change, "data")), &d); e != nil {
								t.Fatal(e)
							}
							switch str(change, "action") {
							case "DeleteTable":
								if d["tableGuid"] != "t2" {
									t.Fatalf("wrong deleted table %v", d)
								}
								delete(tables, "t2")
							case "CreateTable":
								id, view, row := str(d, "tableGuid"), str(d, "viewGuid"), d["rowsGuid"].([]any)[0].(string)
								field := d["cols"].([]any)[0].(map[string]any)
								if d["name"] != "Created" || d["desc"] != "synthetic" || field["name"] != "标题" {
									t.Fatalf("wrong create payload %v", d)
								}
								tables[id] = map[string]any{"guid": id, "baseGuid": "base12345678", "tableType": float64(0), "name": d["name"], "desc": d["desc"], "fields": []any{field}, "views": []any{map[string]any{"guid": view, "tableGuid": id, "name": "默认视图", "type": "Grid", "fields": []any{field["guid"]}, "records": []any{row}}}, "records": []any{map[string]any{"guid": row, "valueMap": map[string]any{}}}}
							case "CreateDashboard":
								id := str(d, "dashboardGuid")
								x := phase3TableCopy(d)
								delete(x, "targetGuid")
								delete(x, "dashboardGuid")
								x["guid"] = id
								x["tableType"] = float64(4)
								tables[id] = x
							case "UpdateComponent":
								if d["componentGuid"] != "component" || d["style"].(map[string]any)["themeColor"] != "1" {
									t.Fatalf("wrong style request %v", d)
								}
								tables["dashboard"]["components"] = []any{d}
							case "CreateColumn":
								field := d["cols"].([]any)[0].(map[string]any)
								props := field["props"].(map[string]any)
								if d["tableGuid"] != "t1" || props["tableGuid"] != "t2" || field["name"] != "linked" {
									t.Fatalf("wrong reference %v", d)
								}
								releaseBaseLink(tables, "t1", str(field, "guid"), "t2", str(props, "foreignTableColumnGuid"))
							case "UpdateCell":
								rows := phase3TableMap(tables[str(d, "tableGuid")], "records")
								for id, raw := range d["rows"].(map[string]any) {
									for field, value := range raw.(map[string]any) {
										rows[id]["valueMap"].(map[string]any)[field] = value
									}
								}
							default:
								t.Fatalf("unexpected compose action %v", change)
							}
						}
						if fault == "ack-failure" {
							return phase3Response(200, `{"status":"uncertain"}`), nil
						}
						return phase3Response(200, `{"status":"normal"}`), nil
					default:
						return nil, fmt.Errorf("unexpected offline request %s %s", r.Method, r.URL.Path)
					}
				})}
				out := t.TempDir()
				a := newApp([]string{"phase3", "https://phase3.test/tables/base12345678", "--action", tc.action, "--payload", tc.payload, "--out", out, "--relogin", "--json"})
				a.Origin = "https://phase3.test"
				err := a.Run()
				success := fault == "control"
				attempted := fault != "stale-revision" && fault != "before-screenshot" && fault != "before-chart-error" && fault != "before-chart-invalid" && fault != "capability-error" && fault != "capability-wrong-table"
				if (err == nil) != success {
					t.Fatalf("wrong outcome writes=%d: %v", writes, err)
				}
				want := 0
				if attempted {
					want = 1
				}
				if writes != want {
					t.Fatalf("writes %d want %d", writes, want)
				}
				dir := releaseDocumentEvidence(t, out, success, attempted)
				if !success && attempted && !strings.Contains(err.Error(), "禁止自动重放") {
					t.Fatalf("missing no-replay warning: %v", err)
				}
				if success {
					if !guard.fresh || guard.captures < 3 {
						t.Fatal("success without fresh visual boundary")
					}
					if _, e := os.Stat(filepath.Join(dir, "reopened-structure.json")); e != nil {
						t.Fatal(e)
					}
				}
			})
		}
	}
}
