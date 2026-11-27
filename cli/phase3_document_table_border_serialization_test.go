package main

import (
	"strings"
	"testing"
)

func TestDocumentTableStylePreservesImplicitTableBorderSpacing(t *testing.T) {
	borderXML := func(extra string) string {
		out := `<w:tblBorders>`
		for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
			out += `<w:` + side + ` w:val="single" w:sz="8" w:color="000000"` + extra + `/>`
		}
		return out + `</w:tblBorders>`
	}
	tableXML := func(border, fill string) string {
		return `<w:tbl><w:tblPr>` + border + `</w:tblPr><w:tblGrid><w:gridCol w:w="2000"/><w:gridCol w:w="2000"/></w:tblGrid><w:tr><w:tc><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc><w:tc><w:tcPr>` + fill + `</w:tcPr><w:p><w:r><w:t>B</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	}
	file := func(border, fill, otherTable, styles string) string {
		parts := map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `" xmlns:x="urn:foreign"><w:body>` + tableXML(border, fill) + otherTable + `</w:body></w:document>`}
		if styles != "" {
			parts["word/styles.xml"] = `<w:styles xmlns:w="` + artifactWordNS + `">` + styles + `</w:styles>`
		}
		return artifactTestZip(t, parts)
	}
	spec, err := prepareDocumentTableStyle(map[string]any{"table": float64(1), "row": float64(1), "column": float64(2), "expectedCell": "B", "fill": "#00AA00"})
	if err != nil {
		t.Fatal(err)
	}
	beforeBorders, afterBorders := borderXML(""), borderXML(` w:space="0"`)
	fill := `<w:shd w:val="clear" w:fill="00AA00"/>`
	before := file(beforeBorders, "", "", "")
	if err = verifyDocumentTableStyle(before, file(afterBorders, fill, "", ""), spec); err != nil {
		t.Fatalf("unchanged table borders may serialize explicit zero spacing: %v", err)
	}
	for name, borders := range map[string]string{
		"nonzero":         borderXML(` w:space="1"`),
		"lexical":         borderXML(` w:space="00"`),
		"foreign":         borderXML(` x:space="0"`),
		"unknown":         borderXML(` w:space="0" w:unknown="1"`),
		"duplicate-space": borderXML(` w:space="0" w:space="0"`),
		"changed-inner":   strings.Replace(afterBorders, `<w:insideH w:val="single"`, `<w:insideH w:val="double"`, 1),
		"changed-color":   strings.Replace(afterBorders, `w:color="000000"`, `w:color="FF0000"`, 1),
		"new-border-edge": strings.Replace(afterBorders, `</w:tblBorders>`, `<w:tl2br w:space="0"/></w:tblBorders>`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if verifyDocumentTableStyle(before, file(borders, fill, "", ""), spec) == nil {
				t.Fatal("real border change accepted")
			}
		})
	}
	t.Run("other-table", func(t *testing.T) {
		if verifyDocumentTableStyle(file(beforeBorders, "", tableXML(beforeBorders, ""), ""), file(afterBorders, fill, tableXML(afterBorders, ""), ""), spec) == nil {
			t.Fatal("other table serialization change masked")
		}
	})
	t.Run("previous-nonzero", func(t *testing.T) {
		if verifyDocumentTableStyle(file(borderXML(` w:space="5"`), "", "", ""), file(afterBorders, fill, "", ""), spec) == nil {
			t.Fatal("nonzero spacing replaced by zero")
		}
	})
	t.Run("new-table-borders", func(t *testing.T) {
		if verifyDocumentTableStyle(file("", "", "", ""), file(afterBorders, fill, "", ""), spec) == nil {
			t.Fatal("new borders accepted as default serialization")
		}
	})
	t.Run("style-inheritance", func(t *testing.T) {
		style := `<w:style w:type="table" w:default="1" w:styleId="T"><w:tblPr><w:tblBorders><w:insideH w:space="5"/></w:tblBorders></w:tblPr></w:style>`
		if verifyDocumentTableStyle(file(beforeBorders, "", "", style), file(afterBorders, fill, "", style), spec) == nil {
			t.Fatal("inherited spacing override accepted")
		}
	})
}
