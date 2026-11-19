package main

import (
	"archive/zip"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func artifactTestZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "evidence.xlsx")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range entries {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(body)); e != nil {
			t.Fatal(e)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

const artifactNS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
const artifactStyles = `<styleSheet xmlns="` + artifactNS + `"><fonts count="2"><font><color rgb="FF000000"/></font><font><b/><color rgb="FFFF0000"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF00FF00"/></patternFill></fill></fills><borders count="1"><border/></borders><cellStyleXfs count="1"><xf fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="2"><xf fontId="0" fillId="0" borderId="0"/><xf fontId="1" fillId="1" borderId="0"/></cellXfs></styleSheet>`

func artifactWorkbookEntries(sheet string) map[string]string {
	return map[string]string{
		"[Content_Types].xml":        `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/></Types>`,
		"_rels/.rels":                `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="r1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<workbook xmlns="` + artifactNS + `" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="First" sheetId="1" r:id="s1"/><sheet name="Target" sheetId="2" r:id="s2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="s1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="s2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/custom.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>wrong sheet</t></is></c></row></sheetData></worksheet>`,
		"xl/worksheets/custom.xml":   sheet,
		"xl/styles.xml":              artifactStyles,
	}
}

func TestVerifyArtifactExamples(t *testing.T) {
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		ev, err := verifyArtifact("examples/接口验证."+kind, kind, nil)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if !ev.Checks["zip"] || ev.Bytes == 0 {
			t.Fatal("missing proof")
		}
	}
}

func TestArtifactUsesNamedSheetAndExactCoordinates(t *testing.T) {
	entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData><row r="2"><c r="B2" t="inlineStr"><is><t>expected</t></is></c><c r="C2"><v>10</v></c></row></sheetData></worksheet>`)
	p := artifactTestZip(t, entries)
	for _, tc := range []struct {
		rng    string
		values [][]any
		pass   bool
	}{
		{"Target!B2:C2", [][]any{{"expected", float64(10)}}, true},
		{"First!B2:C2", [][]any{{"expected", float64(10)}}, false},
		{"Missing!B2:C2", [][]any{{"expected", float64(10)}}, false},
		{"Target!A1:B1", [][]any{{"expected", float64(10)}}, false},
		{"Target!B2:C2", [][]any{{"expected", "10"}}, false},
		{"", [][]any{{"expected", float64(10)}}, false},
	} {
		_, err := verifyArtifactExpected(p, "xlsx", artifactExpectation{Range: tc.rng, Values: tc.values})
		if (err == nil) != tc.pass {
			t.Errorf("%s pass=%v err=%v", tc.rng, tc.pass, err)
		}
	}
}

func TestArtifactStylesMustApplyToEveryTargetCell(t *testing.T) {
	for _, style := range []string{"0", "1"} {
		entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1" s="` + style + `" t="inlineStr"><is><t>x</t></is></c><c r="B1" s="0" t="inlineStr"><is><t>y</t></is></c></row></sheetData></worksheet>`)
		p := artifactTestZip(t, entries)
		attrs := map[string]any{"bold": true, "color": "#FF0000", "background": "#00FF00"}
		_, err := verifyArtifactExpected(p, "xlsx", artifactExpectation{Range: "Target!A1", Attrs: attrs})
		if (err == nil) != (style == "1") {
			t.Fatalf("style %s err %v", style, err)
		}
		if _, err = verifyArtifactExpected(p, "xlsx", artifactExpectation{Range: "Target!A1:B1", Attrs: attrs}); err == nil {
			t.Fatal("partial range format accepted")
		}
	}
}

func TestArtifactMergeOnNamedSheet(t *testing.T) {
	entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData/><mergeCells count="1"><mergeCell ref="A1:C1"/></mergeCells></worksheet>`)
	p := artifactTestZip(t, entries)
	if _, err := verifyArtifactExpected(p, "xlsx", artifactExpectation{SheetName: "Target", MergeRef: "A1:C1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyArtifactExpected(p, "xlsx", artifactExpectation{SheetName: "First", MergeRef: "A1:C1"}); err == nil {
		t.Fatal("wrong sheet merge accepted")
	}
}

func TestArtifactRejectsInvalidPackageXML(t *testing.T) {
	for _, body := range []string{"", " ", "<worksheet>", "<worksheet/><worksheet/>", "hello<worksheet/>", "<notWorksheet/>"} {
		entries := artifactWorkbookEntries(body)
		if _, err := verifyArtifact(artifactTestZip(t, entries), "xlsx", nil); err == nil {
			t.Errorf("accepted %q", body)
		}
	}
}

func TestArtifactRejectsMalformedPPTXSlide(t *testing.T) {
	original, err := zip.OpenReader("examples/接口验证.pptx")
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	entries := map[string]string{}
	for _, f := range original.File {
		b, e := zipEntryBytes(f)
		if e != nil {
			t.Fatal(e)
		}
		entries[f.Name] = string(b)
	}
	entries["ppt/slides/slide1.xml"] = "<p:sld>"
	if _, err = verifyArtifact(artifactTestZip(t, entries), "pptx", nil); err == nil {
		t.Fatal("malformed slide accepted")
	}
}

func TestArtifactRangeValidation(t *testing.T) {
	for _, r := range []string{"S!A1:B2", "S!XFD1048576", "'Sheet name'!$A$1:$B$2"} {
		if _, _, _, ok := parseCellRange(r); !ok {
			t.Errorf("rejected %s", r)
		}
	}
	for _, r := range []string{"S!A1:nope", "S!B2:A1", "S!A0", "S!XFE1", "S!A1048577", "S!A1:B2:C3", "!A1", "S!A1!B2", "S!A01"} {
		if _, _, _, ok := parseCellRange(r); ok {
			t.Errorf("accepted %s", r)
		}
	}
}

func TestArtifactUnusedStylesDoNotSatisfyIndividualAttributes(t *testing.T) {
	entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1" s="0" t="inlineStr"><is><t>x</t></is></c></row></sheetData></worksheet>`)
	p := artifactTestZip(t, entries)
	for attr, value := range map[string]any{"bold": true, "color": "#FF0000", "background": "#00FF00"} {
		if _, err := verifyArtifactExpected(p, "xlsx", artifactExpectation{Range: "Target!A1", Attrs: map[string]any{attr: value}}); err == nil {
			t.Fatalf("unused %s accepted", attr)
		}
	}
}

func TestArtifactTypedSharedStringsAndBooleans(t *testing.T) {
	entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="b"><v>1</v></c><c r="C1"><v>1.5</v></c></row></sheetData></worksheet>`)
	entries["xl/sharedStrings.xml"] = `<sst xmlns="` + artifactNS + `"><si><r><t>rich</t></r><r><t> text</t></r></si></sst>`
	p := artifactTestZip(t, entries)
	if _, err := verifyArtifactExpected(p, "xlsx", artifactExpectation{Range: "Target!A1:C1", Values: [][]any{{"rich text", true, 1.5}}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := readArtifactSheet(p, "Target")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Cells["C1"].Value != "3/2" || snapshot.Cells["B1"].Type != "bool" {
		t.Fatalf("snapshot %#v", snapshot)
	}
}

func TestArtifactRejectsInvalidRelationsAndCellReferences(t *testing.T) {
	for _, mutate := range []func(map[string]string){
		func(e map[string]string) {
			e["xl/_rels/workbook.xml.rels"] = `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="s1" Type="` + artifactOfficeRelNS + `/worksheet" Target="https://example.com/sheet.xml" TargetMode="External"/></Relationships>`
		},
		func(e map[string]string) {
			e["xl/worksheets/custom.xml"] = `<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A2"><v>1</v></c></row></sheetData></worksheet>`
		},
		func(e map[string]string) {
			e["xl/worksheets/custom.xml"] = `<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1" s="99"><v>1</v></c></row></sheetData></worksheet>`
		},
		func(e map[string]string) {
			e["xl/worksheets/custom.xml"] = `<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1" t="s"><v>88</v></c></row></sheetData></worksheet>`
		},
		func(e map[string]string) {
			e["xl/worksheets/custom.xml"] = `<worksheet xmlns="` + artifactNS + `"><sheetData/><sheetData/></worksheet>`
		},
	} {
		entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData/></worksheet>`)
		mutate(entries)
		if _, err := verifyArtifact(artifactTestZip(t, entries), "xlsx", nil); err == nil {
			t.Fatal("invalid package accepted")
		}
	}
}

func TestArtifactRejectsFormulaCacheAsWrittenValue(t *testing.T) {
	entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1"><f>1+1</f><v>2</v></c></row></sheetData></worksheet>`)
	if _, err := verifyArtifactExpected(artifactTestZip(t, entries), "xlsx", artifactExpectation{Range: "Target!A1", Values: [][]any{{float64(2)}}}); err == nil {
		t.Fatal("formula cache is not a literal write")
	}
}

func TestArtifactStyleInheritanceDoesNotUseInactiveFont(t *testing.T) {
	entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1" s="1" t="inlineStr"><is><t>x</t></is></c></row></sheetData></worksheet>`)
	entries["xl/styles.xml"] = `<styleSheet xmlns="` + artifactNS + `"><fonts><font/><font><b/></font></fonts><fills><fill><patternFill patternType="none"/></fill></fills><borders><border/></borders><cellStyleXfs><xf fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs><xf fontId="0" fillId="0" borderId="0"/><xf fontId="1" fillId="0" borderId="0" xfId="0" applyFont="0"/></cellXfs></styleSheet>`
	p := artifactTestZip(t, entries)
	if _, err := verifyArtifactExpected(p, "xlsx", artifactExpectation{Range: "Target!A1", Attrs: map[string]any{"bold": true}}); err == nil {
		t.Fatal("inactive direct font accepted")
	}
	if _, err := verifyArtifactExpected(p, "xlsx", artifactExpectation{Range: "Target!A1", Attrs: map[string]any{"bold": false}}); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactRejectsBrokenFileAndPackageBoundaries(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "broken.xlsx")
	if err := os.WriteFile(p, []byte("not a zip"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{dir, p, filepath.Join(dir, "missing.xlsx")} {
		if _, err := verifyArtifact(input, "xlsx", nil); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData/></worksheet>`)
	entries["../outside.xml"] = "<x/>"
	if _, err := verifyArtifact(artifactTestZip(t, entries), "xlsx", nil); err == nil {
		t.Fatal("package traversal accepted")
	}
	delete(entries, "../outside.xml")
	delete(entries, "_rels/.rels")
	if _, err := verifyArtifact(artifactTestZip(t, entries), "xlsx", nil); err == nil {
		t.Fatal("unbound main part accepted")
	}
}

func TestArtifactSnapshotStyleFingerprintSurvivesRenumbering(t *testing.T) {
	first := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1" s="1"><v>7</v></c></row></sheetData></worksheet>`)
	second := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1" s="0"><v>7</v></c></row></sheetData></worksheet>`)
	second["xl/styles.xml"] = `<styleSheet xmlns="` + artifactNS + `"><fonts><font><b/><color rgb="FFFF0000"/></font><font><color rgb="FF000000"/></font></fonts><fills><fill><patternFill patternType="solid"><fgColor rgb="FF00FF00"/></patternFill></fill><fill><patternFill patternType="none"/></fill></fills><borders><border/></borders><cellXfs><xf fontId="0" fillId="0" borderId="0"/><xf fontId="1" fillId="1" borderId="0"/></cellXfs></styleSheet>`
	a, err := readArtifactSheet(artifactTestZip(t, first), "Target")
	if err != nil {
		t.Fatal(err)
	}
	b, err := readArtifactSheet(artifactTestZip(t, second), "Target")
	if err != nil {
		t.Fatal(err)
	}
	if a.Cells["A1"].StyleKey != b.Cells["A1"].StyleKey {
		t.Fatal("renumbered equal styles differ")
	}
}

func TestArtifactNumericTextRequiresExplicitOptIn(t *testing.T) {
	for _, text := range []string{"11", "011", "11.0", "1.1e1", " 11", "11 ", "12"} {
		entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c></row></sheetData></worksheet>`)
		entries["xl/sharedStrings.xml"] = `<sst xmlns="` + artifactNS + `"><si><t>` + text + `</t></si></sst>`
		p := artifactTestZip(t, entries)
		expected := artifactExpectation{Range: "Target!A1", Values: [][]any{{float64(11)}}}
		if _, err := verifyArtifactExpected(p, "xlsx", expected); err == nil {
			t.Fatalf("strict default accepted numeric text %q", text)
		}
		expected.NumericText = true
		ev, err := verifyArtifactExpected(p, "xlsx", expected)
		if (err == nil) != (text == "11") {
			t.Fatalf("opt-in %q error %v", text, err)
		}
		if err == nil && ev.Details["numeric-text"] == "" {
			t.Fatal("numeric compatibility must be disclosed")
		}
	}
	for _, tc := range []struct {
		value string
		want  any
	}{{"true", true}, {"1", true}, {"NaN", math.NaN()}, {"+Inf", math.Inf(1)}} {
		if artifactScalarEqualWithNumericText(xlsxCell{Type: "string", Value: tc.value}, tc.want, true) {
			t.Fatalf("converted non-finite/bool %#v", tc)
		}
	}
	if artifactScalarEqualWithNumericText(xlsxCell{Type: "string", Value: "11", Formula: "1+10"}, float64(11), true) {
		t.Fatal("formula cache accepted")
	}
}

func artifactNumberFormatFixture(id, code string) string {
	format := ""
	if code != "" {
		format = `<numFmts><numFmt numFmtId="` + id + `" formatCode="` + code + `"/></numFmts>`
	}
	return `<styleSheet xmlns="` + artifactNS + `">` + format + `<fonts><font/></fonts><fills><fill><patternFill patternType="none"/></fill></fills><borders><border/></borders><cellXfs><xf numFmtId="` + id + `" fontId="0" fillId="0" borderId="0"/></cellXfs></styleSheet>`
}

func TestArtifactNumberFormatFingerprintUsesCode(t *testing.T) {
	key := func(id, code string) string {
		root, err := parseArtifactXML([]byte(artifactNumberFormatFixture(id, code)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := artifactStyleKey(root, 0)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	decimal := key("164", "0.00")
	if decimal == key("164", "0%") {
		t.Fatal("same custom ID changing format code escaped fingerprint")
	}
	if decimal != key("165", "0.00") {
		t.Fatal("reindexed identical format changed fingerprint")
	}
	if decimal != key("2", "") {
		t.Fatal("built-in 0.00 and identical custom code differ")
	}
	root, err := parseArtifactXML([]byte(artifactNumberFormatFixture("164", "")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = artifactStyleKey(root, 0); err == nil {
		t.Fatal("undefined custom number format accepted")
	}
}

func TestArtifactNumberFormatInheritance(t *testing.T) {
	xml := `<styleSheet xmlns="` + artifactNS + `"><numFmts><numFmt numFmtId="164" formatCode="0.00"/><numFmt numFmtId="165" formatCode="0%"/></numFmts><cellStyleXfs><xf numFmtId="164"/></cellStyleXfs><cellXfs><xf xfId="0" numFmtId="165" applyNumberFormat="0"/></cellXfs></styleSheet>`
	root, err := parseArtifactXML([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	xf, err := artifactIndexedNode(root, "cellXfs", "xf", 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := artifactNumberFormatKey(root, xf)
	if err != nil {
		t.Fatal(err)
	}
	if got != "code:0.00" {
		t.Fatalf("inactive numFmtId overrides inherited format: %s", got)
	}
}
