package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentTableBorderExplicitZeroSpace(t *testing.T) {
	spec, err := prepareDocumentTableStyle(map[string]any{"table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "A", "border": map[string]any{"color": "#000000", "pt": float64(1)}})
	if err != nil {
		t.Fatal(err)
	}
	borders := func(extra string) string {
		v := `<w:tcBorders>`
		for _, side := range []string{"top", "right", "bottom", "left"} {
			v += `<w:` + side + ` w:val="single" w:sz="8" w:color="000000"` + extra + `/>`
		}
		return v + `</w:tcBorders>`
	}
	file := func(cell, table, styles, afterTable string) string {
		parts := map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `" xmlns:x="urn:foreign"><w:body><w:tbl><w:tblPr>` + table + `</w:tblPr><w:tblGrid><w:gridCol w:w="4000"/></w:tblGrid><w:tr><w:tc><w:tcPr>` + cell + `</w:tcPr><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` + afterTable + `</w:body></w:document>`}
		if styles != "" {
			parts["word/styles.xml"] = `<w:styles xmlns:w="` + artifactWordNS + `">` + styles + `</w:styles>`
		}
		return artifactTestZip(t, parts)
	}
	before := file("", "", "", "")
	if err = verifyDocumentTableStyle(before, file(borders(` w:space="0"`), "", "", ""), spec); err != nil {
		t.Fatalf("explicit default should preserve omitted spacing: %v", err)
	}
	for name, extra := range map[string]string{"nonzero": ` w:space="5"`, "foreign": ` x:space="0"`, "unknown": ` w:space="0" w:unknown="1"`, "lexical-other": ` w:space="00"`} {
		t.Run(name, func(t *testing.T) {
			if verifyDocumentTableStyle(before, file(borders(extra), "", "", ""), spec) == nil {
				t.Fatal("non-default accepted")
			}
		})
	}
	t.Run("existing-nonzero", func(t *testing.T) {
		if verifyDocumentTableStyle(file(borders(` w:space="5"`), "", "", ""), file(borders(` w:space="0"`), "", "", ""), spec) == nil {
			t.Fatal("existing nonzero spacing lost")
		}
	})
	t.Run("inherited-table", func(t *testing.T) {
		tp := `<w:tblBorders><w:top w:val="single" w:space="5"/></w:tblBorders>`
		if verifyDocumentTableStyle(file("", tp, "", ""), file(borders(` w:space="0"`), tp, "", ""), spec) == nil {
			t.Fatal("inherited table spacing overridden")
		}
	})
	for _, space := range []string{"0", "5"} {
		t.Run("inherited-style-"+space, func(t *testing.T) {
			style := `<w:style w:type="table" w:default="1" w:styleId="T"><w:tblStylePr w:type="firstRow"><w:tcPr><w:tcBorders><w:top w:space="` + space + `"/></w:tcBorders></w:tcPr></w:tblStylePr></w:style>`
			if verifyDocumentTableStyle(file("", "", style, ""), file(borders(` w:space="0"`), "", style, ""), spec) == nil {
				t.Fatal("style inheritance must disable zero masking")
			}
		})
	}
	t.Run("untargeted-inner-edge", func(t *testing.T) {
		if verifyDocumentTableStyle(before, file(strings.Replace(borders(` w:space="0"`), `</w:tcBorders>`, `<w:insideH w:space="0"/></w:tcBorders>`, 1), "", "", ""), spec) == nil {
			t.Fatal("inner edge changed")
		}
	})
}

func TestDocumentTableStyleRecordedZeroSpace(t *testing.T) {
	dir := os.Getenv("SHIMO_PHASE3_TABLE_STYLE_RECORDED_DIR")
	if dir == "" {
		t.Skip("opt-in historical DOCX evidence, read-only")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Spec documentTableStyleSpec `json:"spec"`
	}
	if err = json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	before, err := filepath.Glob(filepath.Join(dir, "before", "*.docx"))
	if err != nil || len(before) != 1 {
		t.Fatal("requires one before DOCX", err)
	}
	after, err := filepath.Glob(filepath.Join(dir, "after", "*.docx"))
	if err != nil || len(after) != 1 {
		t.Fatal("requires one after DOCX", err)
	}
	if err = verifyDocumentTableStyle(before[0], after[0], request.Spec); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentTableStylePreservesMediaBytes(t *testing.T) {
	file := func(fill, media string) string {
		doc := `<w:document xmlns:w="` + artifactWordNS + `" xmlns:r="` + artifactOfficeRelNS + `" xmlns:a="` + artifactDrawingNS + `"><w:body><w:p><w:r><w:drawing><a:blip r:embed="rImage"/></w:drawing></w:r></w:p><w:tbl><w:tblGrid><w:gridCol w:w="1000"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:tcW w:w="1000" w:type="dxa"/>` + fill + `</w:tcPr><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`
		rels := `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="rImage" Type="` + artifactOfficeRelNS + `/image" Target="media/image.png"/></Relationships>`
		return artifactTestZip(t, map[string]string{"word/document.xml": doc, "word/_rels/document.xml.rels": rels, "word/media/image.png": media})
	}
	spec, err := prepareDocumentTableStyle(map[string]any{"table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "A", "fill": "#0000FF"})
	if err != nil {
		t.Fatal(err)
	}
	before := file("", "original")
	fill := `<w:shd w:val="clear" w:fill="0000FF"/>`
	if err = verifyDocumentTableStyle(before, file(fill, "original"), spec); err != nil {
		t.Fatal(err)
	}
	if verifyDocumentTableStyle(before, file(fill, "changed"), spec) == nil {
		t.Fatal("table styling silently changed unrelated media bytes")
	}
}
