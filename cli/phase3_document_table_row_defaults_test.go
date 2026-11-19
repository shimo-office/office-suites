package main

import (
	"strings"
	"testing"
)

const documentTableZeroRowDefaults = `<w:gridBefore w:val="0"/><w:gridAfter w:val="0"/><w:wBefore w:w="0" w:type="dxa"/><w:wAfter w:w="0" w:type="dxa"/>`

func TestDocumentTableDistributionNativeZeroRowDefaults(t *testing.T) {
	build := func(widths []string, extra string) documentTablesSnapshot {
		cells := ""
		grid := ""
		for i, w := range widths {
			grid += `<w:gridCol w:w="` + w + `"/>`
			cells += `<w:tc><w:tcPr><w:tcW w:w="` + w + `" w:type="dxa"/></w:tcPr><w:p><w:r><w:t>` + []string{"A", "B"}[i] + `</w:t></w:r></w:p></w:tc>`
		}
		doc := `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:tbl><w:tblGrid>` + grid + `</w:tblGrid><w:tr><w:trPr><w:trHeight w:val="720" w:hRule="atLeast"/>` + extra + `</w:trPr>` + cells + `</w:tr></w:tbl></w:body></w:document>`
		s, err := readDocumentTables(artifactTestZip(t, map[string]string{"word/document.xml": doc}))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := build([]string{"3000", "5000"}, "")
	spec := documentTableSpec{Operation: "distribute-columns", Table: 1, Row: 1, Column: 1, ExpectedCell: "A"}
	for _, tc := range []struct {
		name, extra string
		reject      bool
	}{
		{"native-zero", documentTableZeroRowDefaults, false},
		{"nonzero-grid", strings.Replace(documentTableZeroRowDefaults, `w:val="0"`, `w:val="1"`, 1), true},
		{"nonzero-width", strings.Replace(documentTableZeroRowDefaults, `w:w="0"`, `w:w="1"`, 1), true},
		{"unknown-attribute", strings.Replace(documentTableZeroRowDefaults, `w:gridBefore w:val="0"`, `w:gridBefore w:val="0" w:unknown="1"`, 1), true},
		{"unknown-child", strings.Replace(documentTableZeroRowDefaults, `<w:gridBefore w:val="0"/>`, `<w:gridBefore w:val="0"><w:unknown/></w:gridBefore>`, 1), true},
		{"duplicate-default", documentTableZeroRowDefaults + `<w:gridBefore w:val="0"/>`, true},
		{"other-property", documentTableZeroRowDefaults + `<w:cantSplit/>`, true},
		{"height-change", documentTableZeroRowDefaults + `<w:trHeight w:val="1440" w:hRule="atLeast"/>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyDocumentTable(before, build([]string{"4000", "4000"}, tc.extra), spec)
			if (err != nil) != tc.reject {
				t.Fatalf("reject=%v got %v", tc.reject, err)
			}
		})
	}
}

func TestDocumentTableSplitNativeZeroRowDefaults(t *testing.T) {
	spec := documentTableSpec{Operation: "split", Table: 1, Row: 1, Column: 1, Columns: 2, ExpectedCell: "A\nB"}
	before := documentSpanFixture(t, documentSpanCell(2, "A", "B")+documentSpanCell(1, "C"), "F")
	for _, tc := range []struct {
		extra  string
		reject bool
	}{{documentTableZeroRowDefaults, false}, {documentTableZeroRowDefaults + `<w:cantSplit/>`, true}, {strings.Replace(documentTableZeroRowDefaults, `w:val="0"`, `w:val="1"`, 1), true}} {
		row := `<w:trPr>` + tc.extra + `</w:trPr>` + documentSpanCell(1, "A", "B") + documentSpanCell(1, "") + documentSpanCell(1, "C")
		after := documentSpanFixture(t, row, "F")
		err := verifyDocumentTable(before, after, spec)
		if (err != nil) != tc.reject {
			t.Fatalf("reject=%v got %v", tc.reject, err)
		}
	}
	// Merge has no rebuildFromPositions exception: explicit additions stay errors.
	merge := documentTableSpec{Operation: "merge", Table: 1, Row: 1, Column: 1, EndColumn: 2, ExpectedCell: "A"}
	plain := documentSpanFixture(t, documentSpanCell(1, "A")+documentSpanCell(1, "B")+documentSpanCell(1, "C"), "F")
	merged := documentSpanFixture(t, `<w:trPr>`+documentTableZeroRowDefaults+`</w:trPr>`+documentSpanCell(2, "A", "B")+documentSpanCell(1, "C"), "F")
	if err := verifyDocumentTable(plain, merged, merge); err == nil {
		t.Fatal("merge silently adopted rebuild defaults")
	}
}

func TestDocumentTableColumnRebuildDefaultSpan(t *testing.T) {
	makeCell := func(span string) *artifactNode {
		doc, err := parseArtifactXML([]byte(`<w:tc xmlns:w="` + artifactWordNS + `"><w:tcPr><w:tcW w:w="4000" w:type="dxa"/>` + span + `</w:tcPr><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc>`))
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	base := documentTableNodeKey(makeCell(""), false)
	for _, tc := range []struct {
		xml   string
		equal bool
	}{
		{`<w:gridSpan w:val="1"/>`, true},
		{`<w:gridSpan w:val="2"/>`, false},
		{`<w:gridSpan w:val="1" w:extra="x"/>`, false},
		{`<w:gridSpan w:val="1"><w:extra/></w:gridSpan>`, false},
		{`<w:gridSpan w:val="1"/><w:gridSpan w:val="1"/>`, false},
	} {
		if (documentTableRebuiltCellStyle(makeCell(tc.xml)) == base) != tc.equal {
			t.Fatalf("default span equality wrong: %s", tc.xml)
		}
	}
}

func TestDocumentTableRebuildDefaultsPreserveStyleInheritance(t *testing.T) {
	for _, name := range []string{"gridBefore", "gridAfter", "wBefore", "wAfter", "gridSpan"} {
		for _, value := range []string{"0", "1"} {
			for _, custom := range []bool{false, true} {
				t.Run(name+value, func(t *testing.T) {
					makeDoc := func(defaults string) documentTablesSnapshot {
						doc := `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:tbl><w:tblPr><w:tblStyle w:val="Custom"/></w:tblPr><w:tblGrid><w:gridCol w:w="1000"/></w:tblGrid><w:tr><w:trPr>` + defaults + `</w:trPr><w:tc><w:tcPr><w:tcW w:w="1000" w:type="dxa"/>` + func() string {
							if defaults != "" {
								return `<w:gridSpan w:val="1"/>`
							}
							return ""
						}() + `</w:tcPr><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`
						styles := `<w:styles xmlns:w="` + artifactWordNS + `"><w:style w:type="table" w:styleId="Custom"><w:tblStylePr w:type="firstRow"><w:trPr><w:` + name + ` w:val="` + value + `"/></w:trPr></w:tblStylePr></w:style></w:styles>`
						parts := map[string]string{"word/document.xml": doc, "word/styles.xml": styles}
						if custom {
							delete(parts, "word/styles.xml")
							parts["word/custom/table-styles.xml"] = styles
							parts["word/_rels/document.xml.rels"] = `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="styles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="custom/table-styles.xml"/></Relationships>`
						}
						s, err := readDocumentTables(artifactTestZip(t, parts))
						if err != nil {
							t.Fatal(err)
						}
						return s
					}
					before, after := makeDoc(""), makeDoc(documentTableZeroRowDefaults)
					if before.Tables[0].RebuiltRowStyles[0] == after.Tables[0].RebuiltRowStyles[0] {
						t.Fatal("row defaults masked possible inherited style")
					}
					if before.Tables[0].Rows[0][0].RebuiltStyle == after.Tables[0].Rows[0][0].RebuiltStyle {
						t.Fatal("cell default masked possible inherited style")
					}
				})
			}
		}
	}
}
