package main

import (
	"strings"
	"testing"
)

func TestDocumentThemePreservesUnusedStyleFontSlots(t *testing.T) {
	attrs := `w:ascii="Arial" w:hAnsi="Calibri" w:eastAsia="仿宋" w:cs="Tahoma" w:asciiTheme="minorAscii" w:hAnsiTheme="minorHAnsi" w:eastAsiaTheme="minorEastAsia" w:cstheme="minorBidi"`
	snapshot := func(fonts string) documentLayoutSnapshot {
		t.Helper()
		file := artifactTestZip(t, map[string]string{
			"word/document.xml":     `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:r><w:t>Unchanged</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`,
			"word/styles.xml":       `<w:styles xmlns:w="` + artifactWordNS + `"><w:style w:type="paragraph" w:styleId="Unused"><w:name w:val="Unused"/><w:rPr><w:rFonts ` + fonts + `/></w:rPr></w:style></w:styles>`,
			"word/theme/theme1.xml": `<a:theme xmlns:a="` + artifactDrawingNS + `"><a:themeElements><a:fontScheme name="Custom"><a:majorFont><a:latin typeface="Arial"/><a:ea typeface="黑体"/><a:cs typeface="Arial"/></a:majorFont><a:minorFont><a:latin typeface="Arial"/><a:ea typeface="宋体"/><a:cs typeface="Arial"/></a:minorFont></a:fontScheme></a:themeElements></a:theme>`,
		})
		got, err := readDocumentLayout(file)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	before := snapshot(attrs)
	spec := documentLayoutSpec{Action: "document-theme", Theme: &before.Theme.Fonts}
	if err := verifyDocumentTheme(before, snapshot(strings.ReplaceAll(attrs, "minorHAnsi", "minorAscii")), spec); err != nil {
		t.Fatal("equivalent Latin theme reference rejected:", err)
	}
	for _, attr := range strings.Fields(attrs) {
		t.Run(strings.Split(attr, "=")[0], func(t *testing.T) {
			key := strings.Split(attr, "=")[0]
			changed := strings.Replace(attrs, attr, key+`="changed"`, 1)
			if err := verifyDocumentTheme(before, snapshot(changed), spec); err == nil {
				t.Fatal("unused style font change accepted", key)
			}
			if err := verifyDocumentTheme(before, snapshot(strings.Replace(attrs, attr, "", 1)), spec); err == nil {
				t.Fatal("unused style font removal accepted", key)
			}
		})
	}
}
