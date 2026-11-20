package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func partialRecordPlan(t *testing.T, update bool) (map[string]any, phase3TableEditPlan) {
	t.Helper()
	before := tableFixtureForSnapshot()
	action := "table-record-add"
	payload := map[string]any{"tableGuid": "t1", "records": []any{map[string]any{"f1": "A"}, map[string]any{"f1": "B"}, map[string]any{"f1": "C"}}}
	if update {
		action = "table-record-update"
		before["records"].([]any)[1].(map[string]any)["valueMap"].(map[string]any)["f1"] = "todo"
		payload = map[string]any{"tableGuid": "t1", "where": map[string]any{"f1": "todo"}, "values": map[string]any{"f1": "changed", "f2": float64(7)}}
	}
	plan, err := preparePhase3TableEdit(action, payload, before)
	if err != nil {
		t.Fatal(err)
	}
	return before, plan
}

func TestPhase3TablePartialRecordClassification(t *testing.T) {
	before, plan := partialRecordPlan(t, false)
	after := phase3TableCopy(before)
	ids := plan.CreatedIDs
	after["records"] = append(after["records"].([]any),
		map[string]any{"guid": ids[0], "valueMap": plan.Expected["records"].(map[string]any)[ids[0]]},
		map[string]any{"guid": ids[2], "valueMap": map[string]any{"f1": "wrong"}})
	report := phase3TableRecordOutcomes(before, plan, after, 2, 1, nil, errors.New("original write failure"))
	if got := []string{report.Records[0].Status, report.Records[1].Status, report.Records[2].Status}; !reflect.DeepEqual(got, []string{"applied", "not_applied", "unknown"}) {
		t.Fatalf("wrong per-ID report: %+v", report)
	}
	if report.OK || report.Verified || report.AutomaticReplayAllowed || report.OriginalError != "original write failure" {
		t.Fatalf("partial readback converted failure into success: %+v", report)
	}
	for _, row := range report.Records {
		if row.Source != "stable_readback" {
			t.Fatalf("missing stable source: %+v", row)
		}
	}
	for _, tc := range []struct {
		name string
		rev  float64
		err  error
	}{
		{"read failed despite stale data", 2, errors.New("HTTP 401")},
		{"revision not advanced", 1, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := phase3TableRecordOutcomes(before, plan, after, tc.rev, 1, tc.err, errors.New("original"))
			for _, row := range r.Records {
				if row.Status != "unknown" || row.Source != "unknown" {
					t.Fatalf("uncertain read treated as known: %+v", r)
				}
			}
		})
	}
}

func TestPhase3TablePartialUpdateRequiresExactWholeRow(t *testing.T) {
	before, plan := partialRecordPlan(t, true)
	after := phase3TableCopy(before)
	rows := phase3TableMap(after, "records")
	rows["r1"]["valueMap"] = plan.Expected["records"].(map[string]any)["r1"]
	r := phase3TableRecordOutcomes(before, plan, after, 2, 1, nil, errors.New("original"))
	if r.Records[0].Status != "applied" || r.Records[1].Status != "not_applied" {
		t.Fatalf("wrong update outcomes: %+v", r)
	}
	rows["r2"]["valueMap"].(map[string]any)["f1"] = "changed"
	r = phase3TableRecordOutcomes(before, plan, after, 2, 1, nil, errors.New("original"))
	if r.Records[1].Status != "unknown" {
		t.Fatalf("partially changed row accepted: %+v", r)
	}
	after["records"] = after["records"].([]any)[:1]
	r = phase3TableRecordOutcomes(before, plan, after, 2, 1, nil, errors.New("original"))
	if r.Records[1].Status != "unknown" {
		t.Fatalf("missing update row treated as unapplied: %+v", r)
	}
}

func TestPhase3TablePartialReportDoesNotHideCollateralDamage(t *testing.T) {
	before, plan := partialRecordPlan(t, true)
	after := phase3TableCopy(before)
	for id, row := range phase3TableMap(after, "records") {
		row["valueMap"] = plan.Expected["records"].(map[string]any)[id]
	}
	after["name"] = "unexpected rename"
	r := phase3TableRecordOutcomes(before, plan, after, 2, 1, nil, errors.New("original failure"))
	if r.Verified || r.OK || r.WholeTableError == "" {
		t.Fatalf("collateral damage accepted: %+v", r)
	}
	for _, row := range r.Records {
		if row.Status != "applied" {
			t.Fatalf("exact row not recorded: %+v", r)
		}
	}
}

func TestPhase3TablePartialReadbackBudget(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	old := httpClient
	defer func() { httpClient = old }()
	calls := 0
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	s := &phase3SDKSession{app: &App{Origin: "https://phase3.test"}, guid: "b1", prefix: "/sdk/"}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _, err := s.phase3TablePartialSnapshot(ctx, "t1")
	if err == nil || calls != 1 {
		t.Fatalf("timeout continued reads: calls=%d err=%v", calls, err)
	}
}

func TestPhase3TablePartialReadbackRejectsInconsistentSnapshot(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	for _, mode := range []string{"revision-change", "duplicate-id", "wrong-table", "missing-records"} {
		t.Run(mode, func(t *testing.T) {
			old := httpClient
			defer func() { httpClient = old }()
			revisions := 0
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					t.Fatal("diagnostic wrote data")
				}
				var value any
				if strings.HasSuffix(r.URL.Path, "/rev") {
					revisions++
					rev := 2
					if mode == "revision-change" {
						rev += revisions
					}
					value = map[string]any{"rev": rev, "filePermission": map[string]any{"canRead": true}}
				} else {
					table := tableFixtureForSnapshot()
					switch mode {
					case "duplicate-id":
						table["records"] = append(table["records"].([]any), table["records"].([]any)[0])
					case "wrong-table":
						table["guid"] = "other"
					case "missing-records":
						delete(table, "records")
					}
					value = table
				}
				b, _ := json.Marshal(value)
				return phase3Response(200, string(b)), nil
			})}
			s := &phase3SDKSession{app: &App{Origin: "https://phase3.test"}, guid: "b1", prefix: "/sdk/"}
			after, _, err := s.phase3TablePartialSnapshot(context.Background(), "t1")
			if err == nil || after != nil {
				t.Fatalf("inconsistent read returned usable snapshot: %v %v", after, err)
			}
		})
	}
}

func TestPhase3TablePartialPipelineSingleCompose(t *testing.T) {
	for _, mode := range []string{"partial", "read401", "compose401", "collateral", "evidence-failure", "read-failure", "accepted-partial", "update-partial"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=test")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
			t.Setenv("SHIMO_NET_KEYCHAIN_SERVICE", "shimo-net-cli-synthetic-table-test")
			harvestMu.Lock()
			oldHarvestDone, oldHarvestCookie, oldHarvestErr := harvestDone, harvestCookie, harvestErr
			harvestDone = false
			harvestMu.Unlock()
			defer func() {
				harvestMu.Lock()
				harvestDone, harvestCookie, harvestErr = oldHarvestDone, oldHarvestCookie, oldHarvestErr
				harvestMu.Unlock()
			}()
			oldClient, oldGuard := httpClient, phase3OpenTableGuard
			defer func() { httpClient = oldClient; phase3OpenTableGuard = oldGuard }()
			phase3OpenTableGuard = func(string) (phase3TableGuard, error) { return &tableTestGuard{}, nil }
			// The presence guard is entirely synthetic. Allow dispatch to reach
			// the partial-write assertions; no real browser constructor is used.
			t.Setenv("SHIMO_NET_NO_BROWSER", "")
			before := tableFixtureForSnapshot()
			before["baseGuid"] = "base12345678"
			if mode == "update-partial" {
				before["records"].([]any)[1].(map[string]any)["valueMap"] = map[string]any{"f1": "todo", "f2": float64(1)}
			}
			after := phase3TableCopy(before)
			posts := 0
			var ids []string
			out := t.TempDir()
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				response := func(value any) (*http.Response, error) {
					b, _ := json.Marshal(value)
					return phase3Response(200, string(b)), nil
				}
				switch r.URL.Path {
				case "/api/v1/files/base12345678":
					return phase3Response(200, `{"type":"table"}`), nil
				case "/tables/base12345678":
					return phase3Response(200, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/","SDK_V2_SIGNATURE":"test","SDK_V2_TOKEN":"test"}}];`), nil
				case "/sdk/table-api/bases/base12345678/rev":
					if posts > 0 && mode == "read401" {
						return phase3Response(401, `{}`), nil
					}
					if posts > 0 && mode == "read-failure" {
						return nil, errors.New("simulated network failure")
					}
					return response(map[string]any{"rev": posts + 1, "filePermission": map[string]any{"canRead": true, "canEdit": true}})
				case "/sdk/table-api/tables/t1":
					if posts == 0 {
						return response(before)
					}
					return response(after)
				case "/sdk/table-api/bases/base12345678/compose":
					posts++
					if r.Method != "POST" || posts != 1 {
						t.Fatalf("mutation replayed %d", posts)
					}
					var body struct {
						Change []phase3TableChange `json:"change"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					wantedAction := "CreateRow"
					if mode == "update-partial" {
						wantedAction = "UpdateCell"
					}
					if len(body.Change) != 1 || body.Change[0].Action != wantedAction {
						t.Fatal("unexpected compose")
					}
					if mode == "update-partial" {
						var update struct {
							Rows map[string]map[string]any `json:"rows"`
						}
						if err := json.Unmarshal([]byte(body.Change[0].Data), &update); err != nil {
							t.Fatal(err)
						}
						if len(update.Rows) != 2 {
							t.Fatalf("multi-condition update matched %d", len(update.Rows))
						}
						ids = []string{"r1", "r2"}
						phase3TableMap(after, "records")["r1"]["valueMap"] = update.Rows["r1"]
						return phase3Response(500, `{"error":"injected uncertain acknowledgement"}`), nil
					}
					var change struct {
						Rows []map[string]any `json:"rows"`
					}
					if err := json.Unmarshal([]byte(body.Change[0].Data), &change); err != nil {
						t.Fatal(err)
					}
					for i, row := range change.Rows {
						ids = append(ids, str(row, "guid"))
						if i == 0 || mode == "collateral" {
							after["records"] = append(after["records"].([]any), row)
							v := after["views"].([]any)[0].(map[string]any)
							v["records"] = append(v["records"].([]any), row["guid"])
						}
					}
					if mode == "collateral" {
						after["name"] = "unexpected"
					}
					if mode == "compose401" {
						return phase3Response(401, `{}`), nil
					}
					if mode == "evidence-failure" {
						dirs, _ := os.ReadDir(out)
						if len(dirs) != 1 {
							t.Fatal("missing pre-write evidence")
						}
						if err := os.Mkdir(filepath.Join(out, dirs[0].Name(), "record-outcomes.json"), 0700); err != nil {
							t.Fatal(err)
						}
					}
					if mode == "accepted-partial" {
						return phase3Response(200, `{"status":"normal"}`), nil
					}
					return phase3Response(500, `{"error":"injected uncertain acknowledgement"}`), nil
				default:
					t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
					return nil, nil
				}
			})}
			action, payload := "table-record-add", `{"tableGuid":"t1","records":[{"f1":"A"},{"f1":"B"},{"f1":"C"}]}`
			if mode == "update-partial" {
				action, payload = "table-record-update", `{"tableGuid":"t1","where":{"f1":"todo","f2":1},"values":{"f1":"changed","f2":7}}`
			}
			a := newApp([]string{"phase3", "https://phase3.test/tables/base12345678", "--action", action, "--payload", payload, "--out", out, "--relogin", "--json"})
			a.Origin = "https://phase3.test"
			err := a.Run()
			harvestMu.Lock()
			attemptedHarvest := harvestDone
			harvestMu.Unlock()
			if attemptedHarvest {
				t.Fatal("failure readback attempted browser credential refresh")
			}
			if err == nil || posts != 1 || !strings.Contains(err.Error(), "禁止自动重放") {
				t.Fatalf("failure replay/success: posts=%d err=%v", posts, err)
			}
			dirs, _ := os.ReadDir(out)
			if len(dirs) != 1 {
				t.Fatalf("evidence dirs=%d", len(dirs))
			}
			dir := filepath.Join(out, dirs[0].Name())
			if mode == "evidence-failure" {
				b, e := os.ReadFile(filepath.Join(dir, "failure.json"))
				if e != nil || !strings.Contains(err.Error(), "逐条失败报告无法保存") || !strings.Contains(string(b), "originalError") {
					t.Fatalf("lost original/evidence failure: %v %s", err, b)
				}
				return
			}
			var report phase3TablePartialReport
			b, e := os.ReadFile(filepath.Join(dir, "record-outcomes.json"))
			if e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(b, &report); e != nil {
				t.Fatal(e)
			}
			if len(report.Records) != len(ids) || report.OK || report.Verified {
				t.Fatalf("invalid report: %+v", report)
			}
			for i, row := range report.Records {
				want := "not_applied"
				if i == 0 || mode == "collateral" {
					want = "applied"
				}
				if mode == "read401" || mode == "read-failure" {
					want = "unknown"
				}
				if row.ID != ids[i] || row.Status != want {
					t.Fatalf("row %d %+v want %s", i, row, want)
				}
			}
			if mode == "collateral" && report.WholeTableError == "" {
				t.Fatal("lost collateral error")
			}
			if _, e = os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(e) {
				t.Fatal("failure wrote successful result")
			}
			b, e = os.ReadFile(filepath.Join(dir, "failure.json"))
			if e != nil || !strings.Contains(string(b), "record-outcomes.json") {
				t.Fatalf("missing failure link: %s %v", b, e)
			}
		})
	}
}
