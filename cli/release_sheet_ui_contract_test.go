package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReleaseSheetMenuNeverClicksUnavailableControl(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   map[string]any
		readErr error
	}{
		{"missing", map[string]any{"found": false}, nil},
		{"disabled", map[string]any{"found": true, "disabled": true, "x": 10, "y": 20}, nil},
		{"offscreen", map[string]any{"found": true, "x": 0, "y": 20}, nil},
		{"disconnected", nil, errors.New("synthetic CDP disconnect")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clicked := false
			ws := releasePhase2Peer(t, func(method string, _ map[string]any) (map[string]any, error) {
				if method == "Input.dispatchMouseEvent" {
					clicked = true
					return nil, nil
				}
				return map[string]any{"result": map[string]any{"value": tc.value}}, tc.readErr
			})
			b := &phase3EditorBrowser{ws: ws}
			if err := b.uiClickBox("导出", "button", "下一步"); err == nil {
				t.Fatal("unavailable control was accepted")
			}
			if clicked {
				t.Fatal("unavailable control received input")
			}
		})
	}
}

func TestReleaseSheetMenuClickHoverAndStateReadback(t *testing.T) {
	var events []map[string]any
	ws := releasePhase2Peer(t, func(method string, p map[string]any) (map[string]any, error) {
		if method == "Input.dispatchMouseEvent" {
			events = append(events, p)
			return map[string]any{}, nil
		}
		expression := str(p, "expression")
		value := map[string]any{"found": true, "x": 20, "y": 30, "left": 10, "right": 22, "mid": 30}
		if strings.Contains(expression, "text:(document.body") {
			value = map[string]any{"text": "导出设置\n共 1 页"}
		}
		return map[string]any{"result": map[string]any{"value": value}}, nil
	})
	b := &phase3EditorBrowser{ws: ws}
	if err := b.uiClickBox("导出", "button", "下一步"); err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, event := range events {
		types = append(types, str(event, "type"))
		if num(event, "x") != 20 || num(event, "y") != 30 {
			t.Fatal("wrong click position", event)
		}
	}
	if !reflect.DeepEqual(types, []string{"mouseMoved", "mousePressed", "mouseReleased"}) {
		t.Fatal("incomplete click", types)
	}
	events = nil
	if err := b.uiHoverRow("menu"); err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 {
		t.Fatal("submenu hover did not traverse the row")
	}
	for i, event := range events {
		if str(event, "type") != "mouseMoved" || (i > 0 && num(event, "x") <= num(events[i-1], "x")) {
			t.Fatal("hover travel was not ordered", events)
		}
	}
	if err := b.uiWaitFound("导出", "button", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got := b.uiBodyText(); !strings.Contains(got, "共 1 页") {
		t.Fatal("lost page text", got)
	}
	if err := b.uiSwitchSheet("工作表1"); err != nil {
		t.Fatal(err)
	}
	if err := b.uiSwitchSheet(""); err != nil {
		t.Fatal(err)
	}
	if _, err := evalSheetObject(b, `({found:true})`); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseSheetUIRejectsMissingStateAndReadFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		err   error
	}{
		{"missing", map[string]any{"found": false, "error": "目标工作表不存在"}, nil},
		{"scalar", true, nil},
		{"disconnected", nil, errors.New("synthetic CDP failure")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := releasePhase2Peer(t, func(string, map[string]any) (map[string]any, error) {
				return map[string]any{"result": map[string]any{"value": tc.value}}, tc.err
			})
			b := &phase3EditorBrowser{ws: ws}
			if err := b.uiSwitchSheet("工作表1"); err == nil {
				t.Fatal("missing target was accepted")
			}
			if err := b.uiWaitFound("导出", "button", time.Nanosecond); err == nil {
				t.Fatal("missing control was accepted")
			}
			if err := b.uiHoverRow("menu"); err == nil {
				t.Fatal("missing menu was accepted")
			}
			if tc.err != nil && b.uiBodyText() != "" {
				t.Fatal("read failure returned visible text")
			}
			if tc.name != "missing" {
				if _, err := evalSheetObject(b, `({})`); err == nil {
					t.Fatal("invalid readback was accepted")
				}
			}
		})
	}
}

func TestReleaseSheetReloadVerificationProtectsIdentityAndRequestedState(t *testing.T) {
	rows := 2
	before := map[string]any{"sheetId": "sheet-a", "sheetName": "数据", "rows": float64(1), "columns": float64(0)}
	freeze := sheetBrowserSpec{Action: "sheet-freeze", Sheet: "数据", FrozenRows: &rows}
	for _, tc := range []struct {
		name  string
		spec  sheetBrowserSpec
		after map[string]any
		ok    bool
	}{
		{"frozen", freeze, map[string]any{"sheetId": "sheet-a", "sheetName": "数据", "rows": float64(2), "columns": float64(0)}, true},
		{"unchanged", freeze, before, false},
		{"wrong-sheet", freeze, map[string]any{"sheetId": "sheet-b", "sheetName": "数据", "rows": float64(2)}, false},
		{"filter", sheetBrowserSpec{Action: "sheet-filter", Sheet: "数据", RowCount: 3, ColumnCount: 2}, map[string]any{"sheetId": "sheet-a", "sheetName": "数据", "range": map[string]any{"row": float64(0), "col": float64(0), "rowCount": float64(3), "colCount": float64(2)}}, true},
		{"filter-missing", sheetBrowserSpec{Action: "sheet-filter", Sheet: "数据"}, before, false},
		{"filter-wrong-range", sheetBrowserSpec{Action: "sheet-filter", Sheet: "数据", RowCount: 3}, map[string]any{"sheetId": "sheet-a", "sheetName": "数据", "range": map[string]any{"rowCount": float64(4)}}, false},
		{"clear", sheetBrowserSpec{Action: "sheet-filter-clear", Sheet: "数据"}, before, true},
		{"clear-not-saved", sheetBrowserSpec{Action: "sheet-filter-clear", Sheet: "数据"}, map[string]any{"sheetId": "sheet-a", "sheetName": "数据", "range": map[string]any{}}, false},
		{"unknown-action", sheetBrowserSpec{Action: "unrecognized", Sheet: "数据"}, before, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifySheetBrowserState(tc.spec, before, tc.after)
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected reload verdict: %v", err)
			}
		})
	}
}
