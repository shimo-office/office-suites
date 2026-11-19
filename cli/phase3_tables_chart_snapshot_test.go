package main

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"testing"
)

func TestPhase3DashboardRecordedEnvelopeReadback(t *testing.T) {
	const dir = "../tmp/phase3-live/forms-tables/dashboard-probe-20261118T223407Z/"
	data, err := os.ReadFile(dir + "dashboard.json")
	if os.IsNotExist(err) {
		t.Skip("historical recorded response unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	baseBytes, err := os.ReadFile(dir + "base.json")
	if err != nil {
		t.Fatal(err)
	}
	var base map[string]any
	if err = json.Unmarshal(baseBytes, &base); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	for _, raw := range base["tables"].([]any) {
		m := raw.(map[string]any)
		if str(m, "guid") == "bb7b78865a4" {
			meta = m
		}
	}
	if meta == nil {
		t.Fatal("recorded dashboard metadata missing")
	}
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	old := httpClient
	defer func() { httpClient = old }()
	calls := 0
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/sdk/table-api/bases/DqdxLrxlDMcw78q4/dashboards/bb7b78865a4" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return phase3Response(200, string(data)), nil
	})}
	s := &phase3SDKSession{app: &App{Origin: "https://phase3.test"}, guid: "DqdxLrxlDMcw78q4", prefix: "/sdk/"}
	got, err := s.dashboardSnapshot(meta)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || str(got, "guid") != "bb7b78865a4" || str(got, "baseGuid") != s.guid {
		t.Fatalf("wrong dashboard: %v", got)
	}
	components := got["components"].([]any)
	if len(components) != 1 || str(components[0].(map[string]any), "componentGuid") != "b94633dd573" {
		t.Fatal("persisted component missing")
	}
	if got["shareConfig"] == nil || got["positions"] == nil {
		t.Fatal("stable native metadata discarded")
	}
}

func TestPhase3DashboardComparableProtectsAllStableMetadata(t *testing.T) {
	before := map[string]any{"guid": "d1", "baseGuid": "b1", "name": "chart", "tableType": float64(4), "components": []any{}, "shareConfig": map[string]any{"enable": false}, "refreshInterval": float64(0), "positions": []any{}, "sequence": float64(4), "order": float64(4), "futureSetting": "keep", "updatedAt": float64(10)}
	for _, key := range []string{"shareConfig", "refreshInterval", "positions", "sequence", "order", "futureSetting"} {
		after := phase3TableCopy(before)
		after[key] = "unexpected"
		if reflect.DeepEqual(phase3DashboardComparable(before), phase3DashboardComparable(after)) {
			t.Fatalf("stable dashboard %s was ignored", key)
		}
	}
	after := phase3TableCopy(before)
	after["updatedAt"] = float64(20)
	if !reflect.DeepEqual(phase3DashboardComparable(before), phase3DashboardComparable(after)) {
		t.Fatal("update timestamp should not make content mismatch")
	}
}

func TestPhase3ChartCreationPlansNativeDefaultsExplicitly(t *testing.T) {
	plan, err := preparePhase3TableChart(map[string]any{"tableGuid": "t1", "fieldGuid": "f1", "name": "chart", "type": "Column"}, "b1", map[string]map[string]any{"t1": tableFixtureForEdit()})
	if err != nil {
		t.Fatal(err)
	}
	c := plan.Dashboard["components"].([]any)[0].(map[string]any)
	y := c["axis"].(map[string]any)["yAxis"].(map[string]any)
	if value, exists := y["series"]; !exists || value != nil {
		t.Fatal("future chart must explicitly request a null count series")
	}
	if !reflect.DeepEqual(y["countStyle"], map[string]any{"color": ""}) || !reflect.DeepEqual(y["seriesStyle"], map[string]any{"color": ""}) {
		t.Fatal("native y-axis defaults not explicit in future plan")
	}
	if !reflect.DeepEqual(c["dataRange"], map[string]any{"type": float64(0), "filter": nil}) {
		t.Fatal("all-record data range not explicit")
	}
	style := c["style"].(map[string]any)
	if style["grid"] != false {
		t.Fatal("grid false must be explicit")
	}
	label := style["label"].(map[string]any)
	for _, key := range []string{"columns", "fontBold", "fontFamily", "fontItalic", "fontSize", "position"} {
		value, exists := label[key]
		if !exists || value != nil {
			t.Fatalf("native label %s default not explicit", key)
		}
	}
	var submitted map[string]any
	if err := json.Unmarshal([]byte(plan.Changes[0].Data), &submitted); err != nil {
		t.Fatal(err)
	}
	submittedY := submitted["components"].([]any)[0].(map[string]any)["axis"].(map[string]any)["yAxis"].(map[string]any)
	if value, exists := submittedY["series"]; !exists || value != nil {
		t.Fatal("serialized new chart request must explicitly contain null series")
	}
	for _, key := range []string{"components", "positions", "refreshInterval", "shareConfig"} {
		if !reflect.DeepEqual(submitted[key], plan.Dashboard[key]) {
			t.Fatalf("expected %s was not explicitly sent", key)
		}
	}
}

func dashboardProtocolFixture(t *testing.T) (map[string]any, map[string]any) {
	t.Helper()
	p, err := preparePhase3TableChart(map[string]any{"tableGuid": "t1", "fieldGuid": "f1", "name": "chart", "type": "Column"}, "b1", map[string]map[string]any{"t1": tableFixtureForEdit()})
	if err != nil {
		t.Fatal(err)
	}
	data := phase3TableCopy(p.Dashboard)
	data["dashboardGuid"] = p.TableGuid
	delete(data, "guid")
	delete(data, "tableType")
	component := data["components"].([]any)[0].(map[string]any)
	meta := map[string]any{"guid": p.TableGuid, "baseGuid": "b1", "tableType": float64(4), "name": "chart", "desc": "", "positions": []any{map[string]any{"componentGuid": component["componentGuid"], "grid": phase3TableCopy(component["grid"].(map[string]any))}}}
	return meta, map[string]any{"code": float64(0), "data": data}
}

func readMockDashboard(t *testing.T, meta, envelope map[string]any) (map[string]any, error) {
	t.Helper()
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	old := httpClient
	defer func() { httpClient = old }()
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatal("unexpected write")
		}
		b, _ := json.Marshal(envelope)
		return phase3Response(200, string(b)), nil
	})}
	s := &phase3SDKSession{app: &App{Origin: "https://phase3.test"}, guid: "b1", prefix: "/sdk/"}
	return s.dashboardSnapshot(meta)
}

func TestPhase3DashboardRejectsAmbiguousProtocol(t *testing.T) {
	for _, kind := range []string{"business-error", "string-code", "missing-data", "wrong-dashboard", "missing-dashboard", "alias-conflict", "wrong-base", "changed-name", "foreign-component", "duplicate-component", "numeric-component-id", "missing-position", "position-grid-mismatch", "base-position-mismatch"} {
		t.Run(kind, func(t *testing.T) {
			meta, envelope := dashboardProtocolFixture(t)
			data := envelope["data"].(map[string]any)
			c := data["components"].([]any)[0].(map[string]any)
			switch kind {
			case "business-error":
				envelope["code"] = float64(400)
			case "string-code":
				envelope["code"] = "0"
			case "missing-data":
				delete(envelope, "data")
			case "wrong-dashboard":
				data["dashboardGuid"] = "other"
			case "missing-dashboard":
				delete(data, "dashboardGuid")
			case "alias-conflict":
				data["guid"] = "other"
			case "wrong-base":
				data["baseGuid"] = "other"
			case "changed-name":
				data["name"] = "other"
			case "foreign-component":
				c["dashboardGuid"] = "other"
			case "duplicate-component":
				data["components"] = append(data["components"].([]any), phase3TableCopy(c))
			case "numeric-component-id":
				c["componentGuid"] = float64(42)
				data["positions"].([]any)[0].(map[string]any)["componentGuid"] = float64(42)
				meta["positions"].([]any)[0].(map[string]any)["componentGuid"] = float64(42)
			case "missing-position":
				delete(data, "positions")
			case "position-grid-mismatch":
				data["positions"].([]any)[0].(map[string]any)["w"] = float64(4)
			case "base-position-mismatch":
				meta["positions"].([]any)[0].(map[string]any)["grid"].(map[string]any)["w"] = float64(4)
			}
			if _, err := readMockDashboard(t, meta, envelope); err == nil {
				t.Fatalf("accepted ambiguous dashboard %s", kind)
			}
		})
	}
}

func TestPhase3DashboardReadbackDoesNotNormalizeUnexpectedDefaults(t *testing.T) {
	meta, envelope := dashboardProtocolFixture(t)
	actual, err := readMockDashboard(t, meta, envelope)
	if err != nil {
		t.Fatal(err)
	}
	expected := phase3TableCopy(actual)
	for _, kind := range []string{"filter", "font", "extra-component-setting", "share", "refresh"} {
		t.Run(kind, func(t *testing.T) {
			bad := phase3TableCopy(actual)
			c := bad["components"].([]any)[0].(map[string]any)
			switch kind {
			case "filter":
				c["dataRange"].(map[string]any)["filter"] = map[string]any{"excludeBlank": true}
			case "font":
				c["style"].(map[string]any)["label"].(map[string]any)["fontBold"] = true
			case "extra-component-setting":
				c["unexpectedQuery"] = true
			case "share":
				bad["shareConfig"].(map[string]any)["enable"] = true
			case "refresh":
				bad["refreshInterval"] = float64(60)
			}
			if err := verifyPhase3Dashboard(bad, expected); err == nil {
				t.Fatalf("ignored unexpected %s", kind)
			}
		})
	}
}

func TestPhase3ChartRecordedPriorityCounts(t *testing.T) {
	b, err := os.ReadFile("../tmp/phase3-live/forms-tables/chain-evidence/phase3-1280857967/before-structure.json")
	if os.IsNotExist(err) {
		t.Skip("historical source unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	var before map[string]map[string]any
	if err = json.Unmarshal(b, &before); err != nil {
		t.Fatal(err)
	}
	counts, err := phase3ChartExpectedCounts(before["61492df5d31"], "2668e00efe3")
	if err != nil || !reflect.DeepEqual(counts, map[string]int{"P0": 2, "P1": 1, "": 1}) {
		t.Fatalf("wrong independently counted source: %v %v", counts, err)
	}
	actual := map[string]any{"categories": []any{"P0", "P1", "(空)"}, "values": []any{float64(2), float64(1), float64(1)}}
	if err = verifyPhase3ChartCounts(actual, counts); err != nil {
		t.Fatal(err)
	}
	actual["values"] = []any{float64(2), float64(1), float64(0)}
	if verifyPhase3ChartCounts(actual, counts) == nil {
		t.Fatal("blank row omission accepted")
	}
}

func TestPhase3ChartStylePreservesNullSeriesWithoutNormalizingArray(t *testing.T) {
	tables := map[string]map[string]any{"t1": tableFixtureForEdit()}
	created, err := preparePhase3TableChart(map[string]any{"tableGuid": "t1", "fieldGuid": "f1", "name": "chart", "type": "Column"}, "b1", tables)
	if err != nil {
		t.Fatal(err)
	}
	tables[created.TableGuid] = created.Dashboard
	component := created.Dashboard["components"].([]any)[0].(map[string]any)
	component["axis"].(map[string]any)["yAxis"].(map[string]any)["series"] = []any{}
	payload := map[string]any{"dashboardGuid": created.TableGuid, "componentGuid": component["componentGuid"], "themeColor": float64(1)}
	plan, err := preparePhase3ChartStyle(payload, "b1", tables)
	if err != nil {
		t.Fatal(err)
	}
	actual := map[string]map[string]any{}
	for id, table := range plan.LinkedExpected {
		actual[id] = phase3TableCopy(table)
	}
	actual[plan.TableGuid]["components"].([]any)[0].(map[string]any)["axis"].(map[string]any)["yAxis"].(map[string]any)["series"] = nil
	if err := verifyPhase3BaseEdit(tables, actual, plan); err == nil {
		t.Fatal("server coercion of explicitly submitted array to null was accepted")
	}
	// A distinct later intent uses its actual null baseline. It must submit
	// null explicitly and protect it, without changing the earlier failed plan.
	next, err := preparePhase3ChartStyle(map[string]any{"dashboardGuid": plan.TableGuid, "componentGuid": component["componentGuid"], "themeColor": float64(2)}, "b1", actual)
	if err != nil {
		t.Fatal(err)
	}
	var outgoing map[string]any
	if err := json.Unmarshal([]byte(next.Changes[0].Data), &outgoing); err != nil {
		t.Fatal(err)
	}
	yAxis := outgoing["axis"].(map[string]any)["yAxis"].(map[string]any)
	if value, exists := yAxis["series"]; !exists || value != nil {
		t.Fatal("new request must explicitly preserve null baseline")
	}
	if err := verifyPhase3BaseEdit(actual, next.LinkedExpected, next); err != nil {
		t.Fatal(err)
	}
	previous := plan.Dashboard["components"].([]any)[0].(map[string]any)["axis"].(map[string]any)["yAxis"].(map[string]any)["series"]
	if !reflect.DeepEqual(previous, []any{}) {
		t.Fatal("old expected plan was retrofitted")
	}
}
