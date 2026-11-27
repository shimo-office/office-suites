package main

import "testing"

func TestSheetBrowserFreezePreflight(t *testing.T) {
	for _, payload := range []map[string]any{{"rows": 1.0}, {"sheetName": "数据"}, {"sheetName": "数据", "rows": -1.0}, {"sheetName": "数据", "rows": 1.5}, {"sheetName": "数据", "rows": 1.0, "script": "x"}} {
		if _, err := prepareSheetBrowser("sheet-freeze", payload); err == nil {
			t.Fatalf("accepted unsafe payload %#v", payload)
		}
	}
	spec, err := prepareSheetBrowser("sheet-freeze", map[string]any{"sheetName": "数据", "rows": 0.0, "columns": 2.0})
	if err != nil || spec.FrozenRows == nil || *spec.FrozenRows != 0 || *spec.FrozenColumns != 2 {
		t.Fatalf("lost freeze/cancel intent: %#v %v", spec, err)
	}
}

func TestSheetBrowserFilterPreflight(t *testing.T) {
	spec, err := prepareSheetBrowser("sheet-filter", map[string]any{"sheetName": "数据", "range": "数据!B2:D9"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Row != 1 || spec.Column != 1 || spec.RowCount != 8 || spec.ColumnCount != 3 {
		t.Fatalf("bad filter range: %#v", spec)
	}
	// A range without a sheet prefix is completed from sheetName, and a
	// conflicting sheet name must be rejected rather than silently trusted.
	if _, err = prepareSheetBrowser("sheet-filter", map[string]any{"sheetName": "数据", "range": "B2:D9"}); err != nil {
		t.Fatalf("prefix completion failed: %v", err)
	}
	for _, payload := range []map[string]any{
		{"sheetName": "数据"},
		{"sheetName": "数据", "range": "其它!A1:B9"},
		{"sheetName": "数据", "range": "数据!A1:B1"},
		{"sheetName": "数据", "range": "数据!A1:B2", "attrs": map[string]any{"bold": true}},
		{"sheetName": "数据", "range": "数据!A1"},
	} {
		if _, err := prepareSheetBrowser("sheet-filter", payload); err == nil {
			t.Fatalf("accepted unsafe payload %#v", payload)
		}
	}
}

func TestSheetBrowserFilterClearIgnoresRange(t *testing.T) {
	if _, err := prepareSheetBrowser("sheet-filter-clear", map[string]any{"sheetName": "数据", "range": "数据!A1:B2"}); err == nil {
		t.Fatal("filter-clear must reject an unexpected range parameter")
	}
	spec, err := prepareSheetBrowser("sheet-filter-clear", map[string]any{"sheetName": "数据"})
	if err != nil || spec.Action != "sheet-filter-clear" || spec.Sheet != "数据" {
		t.Fatalf("bad clear spec: %#v %v", spec, err)
	}
}

func TestSheetBrowserUnknownAction(t *testing.T) {
	for _, action := range []string{"sheet-style", "sheet-sort", "sheet-validation", "sheet-pivot"} {
		if _, err := prepareSheetBrowser(action, map[string]any{"sheetName": "数据"}); err == nil {
			t.Fatalf("%s must not be accepted by the browser runner", action)
		}
	}
}
