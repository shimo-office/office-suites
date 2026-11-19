package main

import (
	"encoding/base64"
	"encoding/csv"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestPhase3FormCSVHeaderEscapesFormulaAndKeepsUnanswered(t *testing.T) {
	data, _, err := phase3FormCSV(nil, "=HYPERLINK(\"evil\")", "normal-unanswered")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]) != 5 || rows[0][3] != "'=HYPERLINK(\"evil\")" || rows[0][4] != "normal-unanswered" {
		t.Fatalf("unsafe/missing header: %v", rows)
	}
}

func TestPhase3FormResponseEncoding(t *testing.T) {
	raw := `{"total":0,"responses":[]}`
	r := &Response{Status: 200, Raw: []byte(base64.StdEncoding.EncodeToString([]byte(raw))), Header: http.Header{"X-Content-Encoding": []string{"base64"}}}
	m, err := phase3FormResponseJSON(r)
	if err != nil || num(m, "total") != 0 {
		t.Fatalf("decode %v", err)
	}
	r.Raw = []byte("bad encoding")
	if _, err := phase3FormResponseJSON(r); err == nil {
		t.Fatal("invalid base64 accepted")
	}
}

func TestPhase3FormPageRejectsPartialOrDuplicates(t *testing.T) {
	for _, m := range []map[string]any{
		{"responses": []any{}},
		{"total": float64(1), "responses": []any{map[string]any{"submitId": "a"}}},
		{"total": float64(2), "responses": []any{map[string]any{"submitId": "a", "content": []any{}}, map[string]any{"submitId": "a", "content": []any{}}}},
	} {
		if _, _, err := phase3FormPage(m); err == nil {
			t.Fatal("invalid response page accepted")
		}
	}
}

func TestPhase3FormCSVIsCompleteAndRoundTrips(t *testing.T) {
	rows := []map[string]any{
		{"submitId": "s1", "content": []any{map[string]any{"guid": "q1", "text": map[string]any{"content": "测试,\"值\"\n第二行"}}, map[string]any{"guid": "q2", "choice": "o1"}}},
		{"submitId": "s2", "content": `[{"guid":"q2","choice":"o2"}]`},
	}
	b, counts, err := phase3FormCSV(rows)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(string(b))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || len(records[0]) != 5 {
		t.Fatalf("incomplete csv %#v", records)
	}
	if counts["q2"][`{"choice":"o1"}`] != 1 || counts["q2"][`{"choice":"o2"}`] != 1 {
		t.Fatalf("counts %#v", counts)
	}
	if !strings.Contains(records[1][3], "第二行") {
		t.Fatal("unicode/newline lost")
	}
}

func TestPhase3FormInitializationIsNotPublication(t *testing.T) {
	for _, status := range []string{"unpublished", "normal", "stopped"} {
		m := map[string]any{"status": status, "publishedRev": float64(1), "publishedContent": `{"questions":[],"rules":[]}`}
		result, err := phase3FormPublication(m)
		if err != nil {
			t.Fatal(err)
		}
		if result["published"] != (status != "unpublished") {
			t.Fatalf("wrong published flag for %s", status)
		}
	}
	if _, err := phase3FormPublication(map[string]any{"publishedContent": `{"questions":[]}`}); err == nil {
		t.Fatal("missing status accepted")
	}
}

func TestPhase3FormCSVIncludesUnansweredQuestions(t *testing.T) {
	b, _, err := phase3FormCSV([]map[string]any{}, "q1", "q2")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(b))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]) != 5 {
		t.Fatal("unanswered questions lost from schema")
	}
}

func TestPhase3FormCountLagIsDistinctFromInvalidRows(t *testing.T) {
	row := map[string]any{"submitId": "a", "content": []any{}}
	if _, _, err := phase3FormPage(map[string]any{"total": float64(0), "responses": []any{row}}); !errors.Is(err, errPhase3FormResultTotalLag) {
		t.Fatalf("missing count-lag sentinel: %v", err)
	}
	if _, _, err := phase3FormPage(map[string]any{"total": float64(0), "responses": []any{row, row}}); err == nil || errors.Is(err, errPhase3FormResultTotalLag) {
		t.Fatalf("duplicate row misclassified as retryable lag: %v", err)
	}
}
