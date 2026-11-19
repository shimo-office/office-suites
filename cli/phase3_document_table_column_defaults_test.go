package main

import (
	"strings"
	"testing"
)

func TestDocumentTableColumnEditsUseGuardedRebuildDefaults(t *testing.T) {
	build := func(texts []string, defaults string, span, inherited bool) documentTablesSnapshot {
		grid, cells := "", ""
		for _, text := range texts {
			grid += `<w:gridCol w:w="1000"/>`
			c := documentSpanCell(1, text)
			if span {
				c = strings.Replace(c, `</w:tcPr>`, `<w:gridSpan w:val="1"/></w:tcPr>`, 1)
			}
			cells += c
		}
		doc := `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:tbl><w:tblPr/><w:tblGrid>` + grid + `</w:tblGrid><w:tr><w:trPr>` + defaults + `</w:trPr>` + cells + `</w:tr></w:tbl></w:body></w:document>`
		parts := map[string]string{"word/document.xml": doc}
		if inherited {
			parts["word/styles.xml"] = `<w:styles xmlns:w="` + artifactWordNS + `"><w:style w:type="table" w:default="1" w:styleId="T"><w:tblStylePr w:type="firstRow"><w:trPr><w:gridBefore w:val="1"/></w:trPr></w:tblStylePr></w:style></w:styles>`
		}
		result, err := readDocumentTables(artifactTestZip(t, parts))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, op := range []string{"insert-column-before", "insert-column-after", "delete-column"} {
		t.Run(op, func(t *testing.T) {
			source, next := []string{"A", "B"}, []string{"", "A", "B"}
			spec := documentTableSpec{Operation: op, Table: 1, Row: 1, Column: 1, ExpectedCell: "A"}
			if op == "insert-column-after" {
				next = []string{"A", "", "B"}
			}
			if op == "delete-column" {
				source = []string{"X", "A", "B"}
				next = []string{"A", "B"}
				spec.ExpectedCell = "X"
			}
			before := build(source, "", false, false)
			if err := verifyDocumentTable(before, build(next, documentTableZeroRowDefaults, true, false), spec); err != nil {
				t.Fatal(err)
			}
			for _, bad := range []string{strings.Replace(documentTableZeroRowDefaults, `w:val="0"`, `w:val="1"`, 1), documentTableZeroRowDefaults + `<w:cantSplit/>`, documentTableZeroRowDefaults + `<w:gridBefore w:val="0"/>`} {
				if verifyDocumentTable(before, build(next, bad, true, false), spec) == nil {
					t.Fatal("non-default row changes accepted")
				}
			}
			if verifyDocumentTable(build(source, "", false, true), build(next, documentTableZeroRowDefaults, true, true), spec) == nil {
				t.Fatal("inherited row property masked")
			}
		})
	}
}

func TestDocumentTableStructuralEditsPreserveAssets(t *testing.T) {
	row := documentSpanCell(1, "A") + documentSpanCell(1, "B") + documentSpanCell(1, "C")
	inserted := row + `</w:tr><w:tr>` + documentSpanCell(1, "") + documentSpanCell(1, "") + documentSpanCell(1, "")
	before, err := documentSpanResourceFixture(t, row, "original", "media/image.png")
	if err != nil {
		t.Fatal(err)
	}
	spec := documentTableSpec{Operation: "insert-row-after", Table: 1, Row: 1, Column: 1, ExpectedCell: "A"}
	for _, tc := range []struct {
		name, media, target string
		reject              bool
	}{
		{"same", "original", "media/image.png", false},
		{"media-changed", "replacement", "media/image.png", true},
		{"binding-changed", "original", "custom/image.png", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after, err := documentSpanResourceFixture(t, inserted, tc.media, tc.target)
			if err != nil {
				t.Fatal(err)
			}
			if err = verifyDocumentTable(before, after, spec); (err != nil) != tc.reject {
				t.Fatalf("reject=%v got %v", tc.reject, err)
			}
		})
	}
}
