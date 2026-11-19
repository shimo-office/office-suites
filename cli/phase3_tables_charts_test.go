package main

import "testing"

func TestPhase3TableChartUsesDashboardNativeModel(t *testing.T) {
	before := map[string]map[string]any{"t1": tableFixtureForEdit()}
	p, err := preparePhase3BaseEdit("table-chart-create", map[string]any{"tableGuid": "t1", "fieldGuid": "f1", "name": "验收统计", "type": "Column"}, "b1", before)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 1 || p.Changes[0].Action != "CreateDashboard" {
		t.Fatal("chart must be native dashboard component")
	}
	if p.Dashboard == nil {
		t.Fatal("missing expected dashboard")
	}
	after := map[string]map[string]any{"t1": phase3TableCopy(before["t1"]), p.TableGuid: phase3TableCopy(p.Dashboard)}
	if err = verifyPhase3BaseEdit(before, after, p); err != nil {
		t.Fatal(err)
	}
	after["t1"]["name"] = "unrelated mutation"
	if err = verifyPhase3BaseEdit(before, after, p); err == nil {
		t.Fatal("chart creation changed source table")
	}
	after["t1"]["name"] = before["t1"]["name"]
	after[p.TableGuid]["components"].([]any)[0].(map[string]any)["dataSource"].(map[string]any)["tableGuid"] = "wrong-table"
	if err = verifyPhase3BaseEdit(before, after, p); err == nil {
		t.Fatal("chart with wrong data source accepted")
	}
}

func TestPhase3TableChartRejectsUnknownConfiguration(t *testing.T) {
	before := map[string]map[string]any{"t1": tableFixtureForEdit()}
	for _, payload := range []map[string]any{
		{"tableGuid": "t1", "fieldGuid": "missing", "name": "chart", "type": "Pie"},
		{"tableGuid": "t1", "fieldGuid": "f1", "name": "chart", "type": "Arbitrary"},
		{"tableGuid": "t1", "fieldGuid": "f1", "name": "chart", "type": "Column", "script": "anything"},
	} {
		if _, err := preparePhase3BaseEdit("table-chart-create", payload, "b1", before); err == nil {
			t.Fatalf("accepted %v", payload)
		}
	}
}

func TestPhase3ChartRenderedCounts(t *testing.T) {
	expected := map[string]int{"todo": 1, "done": 2, "": 3}
	actual := map[string]any{"categories": []any{"done", "todo", "(空)"}, "values": []any{float64(2), float64(1), float64(3)}}
	if err := verifyPhase3ChartCounts(actual, expected); err != nil {
		t.Fatal(err)
	}
	actual["values"] = []any{float64(2), float64(2), float64(3)}
	if err := verifyPhase3ChartCounts(actual, expected); err == nil {
		t.Fatal("wrong rendered aggregate accepted")
	}
}

func TestPhase3ChartUnknownLabelIsNotBlank(t *testing.T) {
	if err := verifyPhase3ChartCounts(map[string]any{"categories": []any{"todo", "wrong"}, "values": []any{float64(1), float64(3)}}, map[string]int{"todo": 1, "": 3}); err == nil {
		t.Fatal("unknown label treated as blank")
	}
}

func TestPhase3ChartIdentityAndFreshDocument(t *testing.T) {
	spec := map[string]any{"dashboardGuid": "d1", "componentGuid": "c1", "chartType": float64(1), "tableGuid": "t1", "fieldGuid": "f1", "nativeSeriesType": "bar", "axisDimension": "x"}
	actual := phase3TableCopy(spec)
	actual["timeOrigin"] = float64(20)
	if err := verifyPhase3ChartIdentity(actual, spec, 10); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"dashboardGuid", "componentGuid", "chartType", "tableGuid", "fieldGuid", "nativeSeriesType", "axisDimension"} {
		bad := phase3TableCopy(actual)
		bad[key] = "wrong"
		if err := verifyPhase3ChartIdentity(bad, spec, 10); err == nil {
			t.Fatalf("wrong %s accepted", key)
		}
	}
	actual["timeOrigin"] = float64(10)
	if err := verifyPhase3ChartIdentity(actual, spec, 10); err == nil {
		t.Fatal("stale document chart accepted")
	}
}
