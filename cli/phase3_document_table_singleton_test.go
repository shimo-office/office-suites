package main

import (
	"strings"
	"testing"
)

func TestDocumentTableSingletonVerticalRestart(t *testing.T) {
	for _, tc := range []struct {
		name, first, second string
		reject              bool
	}{
		{"singleton-first", `<w:vMerge w:val="restart"/>`, "", false},
		{"singleton-last", "", `<w:vMerge w:val="restart"/>`, false},
		{"adjacent-restarts", `<w:vMerge w:val="restart"/>`, `<w:vMerge w:val="restart"/>`, false},
		{"real-merge", `<w:vMerge w:val="restart"/>`, `<w:vMerge/>`, true},
		{"orphan-continue", `<w:vMerge/>`, "", true},
		{"duplicate-markers", `<w:vMerge w:val="restart"/><w:vMerge w:val="continue"/>`, "", true},
		{"foreign-marker", `<x:vMerge w:val="restart"/>`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := func(marker, text string) string {
				return `<w:tr><w:tc><w:tcPr><w:tcW w:w="1000" w:type="dxa"/>` + marker + `</w:tcPr><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:tc></w:tr>`
			}
			path := artifactTestZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `" xmlns:x="urn:foreign"><w:body><w:tbl><w:tblGrid><w:gridCol w:w="1000"/></w:tblGrid>` + row(tc.first, "A") + row(tc.second, "B") + `</w:tbl></w:body></w:document>`})
			snapshot, err := readDocumentTables(path)
			if err == nil {
				_, err = documentTableAnchor(snapshot, documentTableSpec{Operation: "delete-row", Table: 1, Row: 1, Column: 1, ExpectedCell: "A"})
			}
			if (err != nil) != tc.reject {
				t.Fatalf("reject=%v got %v", tc.reject, err)
			}
			if !tc.reject {
				for _, r := range snapshot.Tables[0].Rows {
					for _, cell := range r {
						if cell.VMerge == "restart" && !strings.Contains(cell.Style, "restart") {
							t.Fatal("singleton marker must remain in exact style proof")
						}
					}
				}
			}
		})
	}
}

func TestDocumentTableInsertCannotExtendSingletonMerge(t *testing.T) {
	cell := func(text, marker string) string {
		return `<w:tr><w:tc><w:tcPr><w:tcW w:w="1000" w:type="dxa"/>` + marker + `</w:tcPr><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:tc></w:tr>`
	}
	snapshot := func(rows string) documentTablesSnapshot {
		s, e := readDocumentTables(artifactTestZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:tbl><w:tblGrid><w:gridCol w:w="1000"/></w:tblGrid>` + rows + `</w:tbl></w:body></w:document>`}))
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	first, last := cell("A", `<w:vMerge w:val="restart"/>`), cell("B", "")
	before := snapshot(first + last)
	spec := documentTableSpec{Operation: "insert-row-after", Table: 1, Row: 1, Column: 1, ExpectedCell: "A"}
	if err := verifyDocumentTable(before, snapshot(first+cell("", "")+last), spec); err != nil {
		t.Fatal(err)
	}
	if err := verifyDocumentTable(before, snapshot(first+cell("", `<w:vMerge/>`)+last), spec); err == nil {
		t.Fatal("new empty row extended an existing singleton into a vertical merge")
	}
}
