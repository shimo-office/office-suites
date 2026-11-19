package main

import "testing"

func TestPhase3ChartStyleChangesOnlyNativePalette(t *testing.T) {
	tables := map[string]map[string]any{"t1": tableFixtureForEdit()}
	created, err := preparePhase3BaseEdit("table-chart-create", map[string]any{"tableGuid": "t1", "fieldGuid": "f1", "name": "chart", "type": "Column"}, "b1", tables)
	if err != nil {
		t.Fatal(err)
	}
	tables[created.TableGuid] = created.Dashboard
	component := created.Dashboard["components"].([]any)[0].(map[string]any)
	payload := map[string]any{"dashboardGuid": created.TableGuid, "componentGuid": component["componentGuid"], "themeColor": float64(1)}
	p, err := preparePhase3BaseEdit("table-chart-style", payload, "b1", tables)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 1 || p.Changes[0].Action != "UpdateComponent" {
		t.Fatal("not native component style update")
	}
	if err = verifyPhase3BaseEdit(tables, p.LinkedExpected, p); err != nil {
		t.Fatal(err)
	}
	if err = verifyPhase3BaseEdit(tables, tables, p); err == nil {
		t.Fatal("unchanged style accepted")
	}
	for _, value := range []any{float64(0), float64(-1), float64(11), float64(1.5), "1"} {
		payload["themeColor"] = value
		if _, err = preparePhase3BaseEdit("table-chart-style", payload, "b1", tables); err == nil {
			t.Fatalf("invalid/no-op palette %v accepted", value)
		}
	}
}

func TestPhase3ChartStyleRequiresRenderedPalette(t *testing.T) {
	spec := map[string]any{"themeColor": "1"}
	actual := map[string]any{"themeColor": "1", "timeOrigin": float64(20), "palette": []any{"#7AE6F5", "#00C9E8", "#008FA0", "#FAD2D8", "#E0504E", "#AF3A37"}}
	if err := verifyPhase3ChartIdentity(actual, spec, 10); err != nil {
		t.Fatal(err)
	}
	actual["themeColor"] = "0"
	if err := verifyPhase3ChartIdentity(actual, spec, 10); err == nil {
		t.Fatal("stale palette config accepted")
	}
	actual["themeColor"] = "1"
	delete(actual, "palette")
	if err := verifyPhase3ChartIdentity(actual, spec, 10); err == nil {
		t.Fatal("missing rendered palette accepted")
	}
}

func TestPhase3ChartStyleRejectsWrongRenderedPalette(t *testing.T) {
	for _, colors := range [][]any{
		{"#1", "#2", "#3", "#4", "#5", "#6"},
		{"#DE7F83", "#F3833A", "#3861AF", "#7AE6F5", "#9C8EC1", "#80BE7F"},
		{"#E0504E", "#E9A2A5", "#ED6B00", "#E9D45C", "#66B165", "#1E541D"},
	} {
		actual := map[string]any{"themeColor": "1", "timeOrigin": float64(20), "palette": colors}
		if err := verifyPhase3ChartIdentity(actual, map[string]any{"themeColor": "1"}, 10); err == nil {
			t.Fatal("invalid, stale, or third palette accepted")
		}
	}
}
