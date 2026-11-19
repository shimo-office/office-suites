package main

import "testing"

func themeStyleProofFixture(t *testing.T, properties, extra string) *documentThemeStyleDefaults {
	t.Helper()
	n, e := parseArtifactXML([]byte(`<w:styles xmlns:w="` + artifactWordNS + `"><w:style w:type="paragraph" w:styleId="heading"><w:name w:val="Heading 1"/>` + properties + `</w:style>` + extra + `</w:styles>`))
	if e != nil {
		t.Fatal(e)
	}
	v, e := readDocumentThemeStyleDefaults(n)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestDocumentLayoutThemeAllowsOnlyProvenUnusedHeadingTableDefault(t *testing.T) {
	zero := `<w:tblPr><w:tblInd w:w="0"/></w:tblPr>`
	before := themeStyleProofFixture(t, "", "")
	after := themeStyleProofFixture(t, zero, "")
	if e := verifyDocumentThemeStyleDefaults(before, after); e != nil {
		t.Fatal(e)
	}
	for _, properties := range []string{`<w:tblPr><w:tblInd w:w="1"/></w:tblPr>`, `<w:tblPr><w:tblInd w:w="0" w:type="pct"/></w:tblPr>`, zero + `<w:pPr><w:spacing w:after="200"/></w:pPr>`} {
		if verifyDocumentThemeStyleDefaults(before, themeStyleProofFixture(t, properties, "")) == nil {
			t.Fatal("non-default or collateral style accepted")
		}
	}
	if verifyDocumentThemeStyleDefaults(after, before) == nil {
		t.Fatal("preexisting default removal accepted")
	}
}
func TestDocumentLayoutThemeRejectsTableInheritanceOfHeadingDefault(t *testing.T) {
	zero := `<w:tblPr><w:tblInd w:w="0"/></w:tblPr>`
	for _, extra := range []string{
		`<w:style w:type="table" w:styleId="table"><w:basedOn w:val="heading"/></w:style>`,
		`<w:style w:type="paragraph" w:styleId="middle"><w:basedOn w:val="heading"/></w:style><w:style w:type="table" w:styleId="table"><w:basedOn w:val="middle"/></w:style>`,
		`<w:style w:type="table" w:styleId="table"><w:basedOn w:val="missing"/></w:style>`,
	} {
		if verifyDocumentThemeStyleDefaults(themeStyleProofFixture(t, "", extra), themeStyleProofFixture(t, zero, extra)) == nil {
			t.Fatal("unproven table inheritance accepted")
		}
	}
}
