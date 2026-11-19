package main

import (
	"os"
	"strings"
	"testing"
)

func TestDocumentMergeRecordedEmptyRunPropertyMaterialization(t *testing.T) {
	beforeXML, err := os.ReadFile("testdata/phase3-table/merge-empty-rpr-before.xml")
	if err != nil {
		t.Fatal(err)
	}
	afterXML, err := os.ReadFile("testdata/phase3-table/merge-empty-rpr-after.xml")
	if err != nil {
		t.Fatal(err)
	}
	read := func(xml string) documentTablesSnapshot {
		s, e := readDocumentTables(artifactTestZip(t, map[string]string{"word/document.xml": xml}))
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	before := read(string(beforeXML))
	spec := documentTableSpec{Operation: "merge", Table: 1, Row: 1, Column: 1, EndColumn: 2, ExpectedCell: "A1"}
	if err = verifyDocumentTable(before, read(string(afterXML)), spec); err != nil {
		t.Fatal("real empty rPr materialization rejected", err)
	}
	for _, tc := range []struct{ name, replacement string }{
		{"duplicate", `<w:rPr/><w:rPr/>`}, {"attribute", `<w:rPr custom="x"/>`}, {"foreign", `<x:rPr xmlns:x="urn:unknown"/>`}, {"child", `<w:rPr><w:b/></w:rPr>`}, {"text", `<w:rPr> </w:rPr>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := strings.Replace(string(afterXML), `<w:rPr></w:rPr>`, tc.replacement, 1)
			if bad == string(afterXML) {
				t.Fatal("fixture missing empty rPr")
			}
			if verifyDocumentTable(before, read(bad), spec) == nil {
				t.Fatal("unsupported target change accepted")
			}
		})
	}
	bad := strings.Replace(string(afterXML), `<w:t>A2</w:t>`, `<w:rPr/><w:t>A2</w:t>`, 1)
	if bad == string(afterXML) {
		t.Fatal("fixture missing untouched A2")
	}
	if verifyDocumentTable(before, read(bad), spec) == nil {
		t.Fatal("non-target empty rPr insertion accepted")
	}
}
