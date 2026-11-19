package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type phase3RoundTripFunc func(*http.Request) (*http.Response, error)

func (f phase3RoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func phase3Response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestPhase3RejectsInvalidPayloadBeforeAnyRequest(t *testing.T) {
	for _, tc := range []struct{ action, payload, readRange string }{
		{"sheet-values", `{"range":"Sheet1!A1"}`, "Sheet1!A1"},
		{"sheet-update", `{"range":"Sheet1!A1","resource":{"values":[]}}`, "Sheet1!A1"},
		{"sheet-update", `{"range":"Sheet1!A1","resource":{"values":[["a"],["b","c"]]}}`, "Sheet1!A1:B2"},
		{"sheet-update", `{"range":"Sheet1!A1","resource":{"values":[[{}]]}}`, "Sheet1!A1"},
		{"sheet-update", `{"range":"Sheet1!A1","resource":{"values":[["x"]]}}`, "Sheet2!A1"},
		{"sheet-format", `{"range":"Sheet1!A1","attrs":{"bold":"yes"}}`, "Sheet1!A1"},
		{"sheet-format", `{"range":"Sheet1!A1","attrs":{"background":"not-a-color"}}`, "Sheet1!A1"},
		{"sheet-merge", `{"range":"Sheet1!B4:A1"}`, "Sheet1!A1:B4"},
		{"sheet-insert", `{"sheetName":"Sheet1","index":"x"}`, "Sheet1!A1:A9"},
		{"sheet-insert", `{"sheetName":"Sheet1","index":1.5}`, "Sheet1!A1:A9"},
		{"sheet-delete", `{"sheetName":"Sheet1","index":1,"count":-1}`, "Sheet1!A1:A9"},
	} {
		t.Run(tc.action+tc.payload, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=test")
			old := httpClient
			defer func() { httpClient = old }()
			requests := 0
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) { requests++; return phase3Response(500, `{}`), nil })}
			a := newApp([]string{"phase3", "https://shimo.net/sheets/abcdefgh", "--action", tc.action, "--payload", tc.payload, "--read-range", tc.readRange})
			a.Origin = "https://shimo.net"
			if err := a.cmdPhase3(); err == nil {
				t.Fatal("invalid payload succeeded")
			}
			if requests != 0 {
				t.Fatalf("invalid payload triggered %d network requests", requests)
			}
		})
	}
}

func TestPhase3ReadbackRequiresOrderedValues(t *testing.T) {
	payload := map[string]any{"range": "Sheet1!A1:B1", "resource": map[string]any{"values": []any{[]any{"A", float64(10)}}}}
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"range":"Sheet1!A1:B1","values":[["A",10]]}`, true},
		{`{"range":"Sheet1!A1:B1","values":[[10,"A"]]}`, false},
		{`{"range":"Sheet1!A1:B1","values":[["wrong",100]],"metadata":["A",10]}`, false},
		{`{"range":"Sheet2!A1:B1","values":[["A",10]]}`, false},
		{`{"values":["A",10]}`, false},
		{`{"status":204,"metadata":["A",10]}`, false},
	} {
		var result any
		if err := json.Unmarshal([]byte(tc.body), &result); err != nil {
			t.Fatal(err)
		}
		if got := readbackContainsExpected(result, payload); got != tc.want {
			t.Errorf("readback %s = %v, want %v", tc.body, got, tc.want)
		}
	}
}

func TestPhase3StructuralTransitions(t *testing.T) {
	cell := func(ref, value string) xlsxCell {
		return xlsxCell{Ref: ref, Type: "string", Value: value, StyleKey: "default"}
	}
	before := artifactSheetSnapshot{Name: "Target", Cells: map[string]xlsxCell{"A1": cell("A1", "header"), "A2": cell("A2", "same"), "A3": cell("A3", "same")}, Merges: []string{}}
	spec := phase3Spec{Action: "sheet-insert", Sheet: "Target", Index: 1, Count: 1}
	inserted := artifactSheetSnapshot{Name: "Target", Cells: map[string]xlsxCell{"A1": cell("A1", "header"), "A3": cell("A3", "same"), "A4": cell("A4", "same")}, Merges: []string{}}
	if err := phase3VerifyStructural(before, inserted, spec); err != nil {
		t.Fatal(err)
	}
	if err := phase3VerifyStructural(before, before, spec); err == nil {
		t.Fatal("insert no-op accepted")
	}
	wrong := artifactSheetSnapshot{Name: "Target", Cells: map[string]xlsxCell{"A2": cell("A2", "header"), "A3": cell("A3", "same"), "A4": cell("A4", "same")}, Merges: []string{}}
	if err := phase3VerifyStructural(before, wrong, spec); err == nil {
		t.Fatal("insert at wrong location accepted")
	}
	spec.Action = "sheet-delete"
	deleted := artifactSheetSnapshot{Name: "Target", Cells: map[string]xlsxCell{"A1": cell("A1", "header"), "A2": cell("A2", "same")}, Merges: []string{}}
	if err := phase3VerifyStructural(before, deleted, spec); err != nil {
		t.Fatal(err)
	}
	if err := phase3VerifyStructural(before, before, spec); err == nil {
		t.Fatal("delete no-op accepted")
	}
}

func TestPhase3SnapshotPreflightRejectsUnprovableChanges(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot artifactSheetSnapshot
		spec     phase3Spec
	}{
		{"empty row", artifactSheetSnapshot{Cells: map[string]xlsxCell{}}, phase3Spec{Action: "sheet-insert", Index: 0, Count: 1}},
		{"formula", artifactSheetSnapshot{Cells: map[string]xlsxCell{"A1": {Ref: "A1", Formula: "1+1", Value: "2"}}}, phase3Spec{Action: "sheet-delete", Index: 0, Count: 1}},
		{"merge", artifactSheetSnapshot{Merges: []string{"A1:B1"}}, phase3Spec{Action: "sheet-append"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := phase3ValidateSnapshot(tc.snapshot, &tc.spec); err == nil {
				t.Fatal("unprovable change allowed")
			}
		})
	}
}

func TestPhase3RejectsWrongOriginAndType(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	old := httpClient
	defer func() { httpClient = old }()
	requests := 0
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return phase3Response(200, `{"type":"documentPro"}`), nil
	})}
	a := newApp([]string{"phase3", "https://other.test/sheets/abcdefgh", "--action", "sheet-update", "--payload", `{"range":"Target!A1","resource":{"values":[["x"]]}}`, "--read-range", "Target!A1"})
	a.Origin = "https://shimo.net"
	if err := a.cmdPhase3(); err == nil {
		t.Fatal("cross-origin accepted")
	}
	if requests != 0 {
		t.Fatal("cross-origin request sent")
	}
	a.Pos[0] = "https://shimo.net/sheets/abcdefgh"
	if err := a.cmdPhase3(); err == nil {
		t.Fatal("wrong file type accepted")
	}
	if requests != 1 {
		t.Fatalf("wrong type requests=%d, want only metadata", requests)
	}
}

func TestPhase3ReadbackCorroboratesCompactedRows(t *testing.T) {
	snapshot := artifactSheetSnapshot{Cells: map[string]xlsxCell{
		"A1": {Ref: "A1", Type: "string", Value: "header"},
		"A3": {Ref: "A3", Type: "string", Value: "repeat"},
		"B3": {Ref: "B3", Type: "number", Value: "3/2"},
		"A4": {Ref: "A4", Type: "string", Value: "repeat"},
		"B4": {Ref: "B4", Type: "number", Value: "3/2"},
	}}
	valid := map[string]any{"values": []any{[]any{"header"}, []any{"repeat", float64(1.5)}, []any{"repeat", float64(1.5)}}}
	if err := phase3VerifyReadback(valid, snapshot, "Target!A1:B5"); err != nil {
		t.Fatal(err)
	}
	missingDuplicate := map[string]any{"values": []any{[]any{"header"}, []any{"repeat", float64(1.5)}}}
	if err := phase3VerifyReadback(missingDuplicate, snapshot, "Target!A1:B5"); err == nil {
		t.Fatal("dropped duplicate accepted")
	}
	if err := phase3VerifyReadback(map[string]any{"values": nil}, artifactSheetSnapshot{Cells: map[string]xlsxCell{}}, "Target!A1:B1"); err != nil {
		t.Fatal(err)
	}
	if err := phase3VerifyReadback(map[string]any{"metadata": "header"}, snapshot, "Target!A1:B5"); err == nil {
		t.Fatal("missing values accepted")
	}
}

func TestPhase3NumericExporterExceptionKeepsAPITypes(t *testing.T) {
	snapshot := artifactSheetSnapshot{Cells: map[string]xlsxCell{
		"A1": {Ref: "A1", Type: "string", Value: "11"},
		"C1": {Ref: "C1", Type: "string", Value: "right"},
	}}
	readback := map[string]any{"values": []any{[]any{float64(11), nil, "right"}}}
	if err := phase3VerifyReadback(readback, snapshot, "Target!A1:C1"); err != nil {
		t.Fatal(err)
	}
	spec := phase3Spec{ReadRange: "Target!A1:C1", Expected: artifactExpectation{Range: "Target!A1", Values: [][]any{{float64(11)}}}}
	if err := phase3VerifyRequestedReadback(readback, snapshot, spec); err != nil {
		t.Fatal(err)
	}
	spec.Expected.Values = [][]any{{"11"}}
	if err := phase3VerifyRequestedReadback(readback, snapshot, spec); err == nil {
		t.Fatal("numeric API value incorrectly accepted for string request")
	}
	snapshot.Cells["A1"] = xlsxCell{Ref: "A1", Type: "string", Value: "011"}
	if err := phase3VerifyReadback(readback, snapshot, "Target!A1:C1"); err == nil {
		t.Fatal("leading-zero text incorrectly accepted for numeric readback")
	}
}

func TestPhase3MergeNormalizationAndScope(t *testing.T) {
	cases := []struct {
		payload string
		want    string
	}{
		{`{"sheetName":"Target","cell":"B2","rowspan":2,"colspan":3}`, "B2:D3"},
		{`{"range":"Target!B2:D3"}`, "B2:D3"},
	}
	for _, tc := range cases {
		var payload map[string]any
		if err := json.Unmarshal([]byte(tc.payload), &payload); err != nil {
			t.Fatal(err)
		}
		spec, err := preparePhase3("sheet-merge", payload, "Target!A1:E5")
		if err != nil {
			t.Fatal(err)
		}
		if spec.Expected.MergeRef != tc.want || spec.Payload["sheetName"] != "Target" || spec.Payload["cell"] != "B2" || spec.Payload["rowspan"] != 2 || spec.Payload["colspan"] != 3 {
			t.Fatalf("bad normalization: %#v", spec)
		}
		if _, exists := spec.Payload["range"]; exists {
			t.Fatal("unsupported range field sent to merge API")
		}
	}
	for _, payload := range []map[string]any{
		{"range": "Target!A1", "cell": "A1", "rowspan": float64(1), "colspan": float64(1)},
		{"range": "Target!A1:B2", "cell": "B2", "rowspan": float64(2), "colspan": float64(2)},
		{"sheetName": "Target", "cell": "XFD1", "rowspan": float64(1), "colspan": float64(2)},
	} {
		if _, err := preparePhase3("sheet-merge", payload, "Target!A1:C3"); err == nil {
			t.Fatalf("invalid merge accepted: %#v", payload)
		}
	}
}

func TestPhase3RangeAndPayloadPreflight(t *testing.T) {
	for _, tc := range []struct{ action, payload, rng string }{
		{"sheet-update", `{"range":"Target!A1","sheetName":"Other","resource":{"values":[["x"]]}}`, "Target!A1"},
		{"sheet-update", `{"range":"Target!A1:B2","resource":{"values":[["x"]]}}`, "Target!A1:B2"},
		{"sheet-update", `{"range":"Target!A1","resource":{"values":[["x"]]},"unexpected":true}`, "Target!A1"},
		{"sheet-update", `{"range":"Target!A1","resource":{"values":[["=1+1"]]}}`, "Target!A1"},
		{"sheet-update", `{"range":"Target!A1","resource":{"values":[[null]]}}`, "Target!A1"},
		{"sheet-update", `{"range":"Target!A1","resource":{"values":[["x"]]}}`, "Target!B1"},
		{"sheet-update", `{"range":"Target!A1","resource":{"values":[["x"]]}}`, "Target!A1:XFD1048576"},
		{"sheet-append", `{"range":"Target!B1","resource":{"values":[["x"]]}}`, "Target!A1:C5"},
		{"sheet-format", `{"range":"Target!A1","attrs":{"fontSize":12}}`, "Target!A1"},
		{"sheet-insert", `{"sheetName":"Target","index":1048575,"count":2}`, "Target!A1"},
	} {
		var payload map[string]any
		if err := json.Unmarshal([]byte(tc.payload), &payload); err != nil {
			t.Fatal(err)
		}
		if _, err := preparePhase3(tc.action, payload, tc.rng); err == nil {
			t.Errorf("invalid %s accepted: %s", tc.action, tc.payload)
		}
	}
	for _, action := range []string{"sheet-update", "sheet-append", "sheet-format", "sheet-insert", "sheet-delete"} {
		raw := `{"sheetName":"Target","range":"A1","resource":{"values":[["x"]]}}`
		switch action {
		case "sheet-format":
			raw = `{"sheetName":"Target","range":"A1","attrs":{"bold":false,"color":"#123ABC"}}`
		case "sheet-insert", "sheet-delete":
			raw = `{"sheetName":"Target","index":1}`
		}
		var payload map[string]any
		_ = json.Unmarshal([]byte(raw), &payload)
		if _, err := preparePhase3(action, payload, "Target!A1:C8"); err != nil {
			t.Errorf("valid %s failed: %v", action, err)
		}
	}
}

func TestPhase3ArtifactIsOnlyStructureInspection(t *testing.T) {
	a := newApp([]string{"phase3", "artifact", "examples/接口验证.xlsx", "--json"})
	capture, err := os.Create(filepath.Join(t.TempDir(), "output.json"))
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = capture
	runErr := a.Run()
	os.Stdout = original
	_ = capture.Close()
	if runErr != nil {
		t.Fatal(runErr)
	}
	raw, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	if output["actionVerified"] != false || output["verificationScope"] != "package structure only" {
		t.Fatalf("misleading inspection output: %s", raw)
	}
	if _, exists := output["verified"]; exists {
		t.Fatal("structure-only inspection must not claim verified action")
	}
	for _, args := range [][]string{
		{"phase3"},
		{"phase3", "artifact"},
		{"phase3", "artifact", "missing.xlsx"},
		{"phase3", "https://shimo.net/sheets/abcdefgh", "--action", "file-lock"},
	} {
		if err := newApp(args).Run(); err == nil {
			t.Fatalf("invalid command succeeded: %v", args)
		}
	}
}
