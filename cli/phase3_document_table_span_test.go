package main

import (
	"fmt"
	"strings"
	"testing"
)

func documentSpanFixture(t *testing.T, row, other string) documentTablesSnapshot {
	t.Helper()
	xml := `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:r><w:t>UNCHANGED</w:t></w:r></w:p><w:tbl><w:tblPr/><w:tblGrid><w:gridCol w:w="1000"/><w:gridCol w:w="1000"/><w:gridCol w:w="1000"/></w:tblGrid><w:tr>` + row + `</w:tr><w:tr>` + documentSpanCell(1, "D") + documentSpanCell(1, "E") + documentSpanCell(1, other) + `</w:tr></w:tbl></w:body></w:document>`
	s, err := readDocumentTables(artifactTestZip(t, map[string]string{"word/document.xml": xml}))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func documentSpanCell(span int, texts ...string) string {
	props := fmt.Sprintf(`<w:tcW w:w="%d" w:type="dxa"/>`, span*1000)
	if span > 1 {
		props += fmt.Sprintf(`<w:gridSpan w:val="%d"/>`, span)
	}
	out := `<w:tc><w:tcPr>` + props + `</w:tcPr>`
	for _, text := range texts {
		out += `<w:p><w:r><w:t xml:space="preserve">` + text + `</w:t></w:r></w:p>`
	}
	return out + `</w:tc>`
}
func TestDocumentTableSpanMergePreservesParagraphsAndOtherCells(t *testing.T) {
	spec, err := prepareDocumentTable(map[string]any{"operation": "merge", "table": float64(1), "row": float64(1), "column": float64(1), "endColumn": float64(2), "expectedCell": " A "})
	if err != nil {
		t.Fatal(err)
	}
	before := documentSpanFixture(t, documentSpanCell(1, " A ")+documentSpanCell(1, "B")+documentSpanCell(1, "C"), "F")
	after := documentSpanFixture(t, documentSpanCell(2, " A ", "B")+documentSpanCell(1, "C"), "F")
	if err = verifyDocumentTable(before, after, spec); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []documentTablesSnapshot{
		before,
		documentSpanFixture(t, documentSpanCell(2, "A", "B")+documentSpanCell(1, "C"), "F"),
		documentSpanFixture(t, documentSpanCell(2, " A B")+documentSpanCell(1, "C"), "F"),
		documentSpanFixture(t, documentSpanCell(2, " A ", "B")+documentSpanCell(1, "C"), "changed"),
	} {
		if err = verifyDocumentTable(before, bad, spec); err == nil {
			t.Fatal("false merge success accepted")
		}
	}
}
func TestDocumentTableSpanSplitPreservesAllMergedText(t *testing.T) {
	spec, err := prepareDocumentTable(map[string]any{"operation": "split", "table": float64(1), "row": float64(1), "column": float64(1), "columns": float64(2), "expectedCell": " A \nB"})
	if err != nil {
		t.Fatal(err)
	}
	before := documentSpanFixture(t, documentSpanCell(2, " A ", "B")+documentSpanCell(1, "C"), "F")
	after := documentSpanFixture(t, documentSpanCell(1, " A ", "B")+documentSpanCell(1, "")+documentSpanCell(1, "C"), "F")
	if err = verifyDocumentTable(before, after, spec); err != nil {
		t.Fatal(err)
	}
	bad := documentSpanFixture(t, documentSpanCell(1, " A ")+documentSpanCell(1, "")+documentSpanCell(1, "C"), "F")
	if err = verifyDocumentTable(before, bad, spec); err == nil {
		t.Fatal("split lost second paragraph")
	}
	hidden := documentSpanCell(1, "")
	hidden = strings.Replace(hidden, "</w:p>", "<w:r><w:drawing/></w:r></w:p>", 1)
	bad = documentSpanFixture(t, documentSpanCell(1, " A ", "B")+hidden+documentSpanCell(1, "C"), "F")
	if err = verifyDocumentTable(before, bad, spec); err == nil {
		t.Fatal("hidden content in new split cell accepted")
	}
	if _, err = documentTableAnchor(before, documentTableSpec{Operation: "delete-column", Table: 1, Row: 1, Column: 1, ExpectedCell: " A \nB"}); err == nil {
		t.Fatal("legacy edit accepted merged table")
	}
}
