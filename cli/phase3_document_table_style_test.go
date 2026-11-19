package main

import (
	"strings"
	"testing"
)

func TestDocumentTableStyleSpec(t *testing.T) {
	p := map[string]any{"table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "A", "fill": "#0000ff", "color": "#ffffff", "marginCm": float64(.2), "align": "center", "border": map[string]any{"color": "#000000", "pt": float64(1)}}
	s, err := prepareDocumentTableStyle(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Fill != "#0000FF" || s.MarginCm == nil || *s.MarginCm != .2 {
		t.Fatal(s)
	}
	for _, bad := range []map[string]any{{"table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "A"}, {"table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "A", "border": map[string]any{"pt": float64(-1), "color": "#000000"}}} {
		if _, err := prepareDocumentTableStyle(bad); err == nil {
			t.Fatal("invalid style accepted")
		}
	}
}

func TestDocumentTableStyleArtifactProof(t *testing.T) {
	source := func(tp, cp, rp, outside string) string {
		return artifactTestZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:tbl><w:tblPr>` + tp + `</w:tblPr><w:tblGrid><w:gridCol w:w="4000"/></w:tblGrid><w:tr><w:tc><w:tcPr>` + cp + `</w:tcPr><w:p><w:r>` + rp + `<w:t>A</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p><w:r><w:t>` + outside + `</w:t></w:r></w:p></w:body></w:document>`})
	}
	before := source("", "", "", "untouched")
	margins := `<w:tcMar>`
	borders := `<w:tcBorders>`
	for _, side := range []string{"top", "right", "bottom", "left"} {
		margins += `<w:` + side + ` w:w="113" w:type="dxa"/>`
		borders += `<w:` + side + ` w:val="single" w:sz="8" w:color="000000"/>`
	}
	margins += `</w:tcMar>`
	borders += `</w:tcBorders>`
	cp := `<w:shd w:val="clear" w:fill="0000FF"/>` + margins + borders
	rp := `<w:rPr><w:color w:val="FFFFFF"/></w:rPr>`
	tp := `<w:jc w:val="center"/>`
	after := source(tp, cp, rp, "untouched")
	spec, err := prepareDocumentTableStyle(map[string]any{"table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "A", "fill": "#0000FF", "color": "#FFFFFF", "marginCm": float64(.2), "align": "center", "border": map[string]any{"color": "#000000", "pt": float64(1)}})
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyDocumentTableStyle(before, after, spec); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{before, source(tp, strings.ReplaceAll(cp, `w:sz="8"`, `w:sz="8" w:space="5"`), rp, "untouched"), source(tp, strings.ReplaceAll(cp, `w:sz="8"`, `w:sz="16"`), rp, "untouched"), source(tp, cp, rp, "changed"), source(tp, cp, `<w:rPr><w:color w:val="000000"/></w:rPr>`, "untouched")} {
		if err = verifyDocumentTableStyle(before, bad, spec); err == nil {
			t.Fatal("incorrect table style proof accepted")
		}
	}
}

func TestDocumentTableStyleRejectsColorWithoutCharacters(t *testing.T) {
	_, err := prepareDocumentTableStyle(map[string]any{"table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "", "fill": "#0000FF", "color": "#FFFFFF"})
	if err == nil {
		t.Fatal("empty-cell text color cannot be asserted from zero characters")
	}
}

func TestDocumentTableStylePreservesInheritedStyleResources(t *testing.T) {
	doc := func(fill string) string {
		return `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:tbl><w:tr><w:tc><w:tcPr>` + fill + `</w:tcPr><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p><w:r><w:t>outside</w:t></w:r></w:p></w:body></w:document>`
	}
	styles := func(line string) string {
		return `<w:styles xmlns:w="` + artifactWordNS + `"><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:pPr><w:spacing w:line="` + line + `"/></w:pPr></w:style></w:styles>`
	}
	before := artifactTestZip(t, map[string]string{"word/document.xml": doc(""), "word/styles.xml": styles("240")})
	after := artifactTestZip(t, map[string]string{"word/document.xml": doc(`<w:shd w:val="clear" w:fill="0000FF"/>`), "word/styles.xml": styles("480")})
	spec, _ := prepareDocumentTableStyle(map[string]any{"table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "A", "fill": "#0000FF"})
	if err := verifyDocumentTableStyle(before, after, spec); err == nil {
		t.Fatal("unrelated inherited paragraph spacing changed without detection")
	}
}
