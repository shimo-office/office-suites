package main

import (
	"os"
	"strings"
	"testing"
)

func TestDocumentMergeRejectsPreferredWidthDriftBeforeWrite(t *testing.T) {
	raw, err := os.ReadFile("testdata/phase3-table/merge-preferred-drift-before.xml")
	if err != nil {
		t.Fatal(err)
	}
	before, err := readDocumentTables(artifactTestZip(t, map[string]string{"word/document.xml": string(raw)}))
	if err != nil {
		t.Fatal(err)
	}
	spec := documentTableSpec{Operation: "merge", Table: 1, Row: 1, Column: 1, EndColumn: 2, ExpectedCell: "KEEP_A1"}
	if _, err = documentTableAnchor(before, spec); err == nil {
		t.Fatal("real native full-table preferred-width rewrite trigger allowed before write")
	}
}

func TestDocumentMergePreferredWidthsRequirePredictableTable(t *testing.T) {
	spec := documentTableSpec{Operation: "merge", Table: 1, Row: 1, Column: 1, EndColumn: 2, ExpectedCell: "A"}
	for _, tc := range []struct {
		name, row string
		reject    bool
	}{
		{"aligned", documentSpanCell(1, "A") + documentSpanCell(1, "B") + documentSpanCell(1, "C"), false},
		{"target-drift", strings.Replace(documentSpanCell(1, "A"), `w:w="1000"`, `w:w="500"`, 1) + documentSpanCell(1, "B") + documentSpanCell(1, "C"), true},
		{"non-target-drift", documentSpanCell(1, "A") + documentSpanCell(1, "B") + strings.Replace(documentSpanCell(1, "C"), `w:w="1000"`, `w:w="500"`, 1), true},
		{"mixed-width-type", documentSpanCell(1, "A") + documentSpanCell(1, "B") + strings.Replace(documentSpanCell(1, "C"), `w:type="dxa"`, `w:type="pct"`, 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := documentSpanFixture(t, tc.row, "F")
			_, err := documentTableAnchor(before, spec)
			if (err != nil) != tc.reject {
				t.Fatalf("reject=%v got %v", tc.reject, err)
			}
		})
	}
}
