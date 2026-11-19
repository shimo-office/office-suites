package main

import "testing"

func TestDocumentLayoutThemeRequiresCompleteExplicitPayload(t *testing.T) {
	good := map[string]any{"themeFonts": map[string]any{"major": map[string]any{"latin": "Arial", "ea": "黑体", "cs": "Arial"}, "minor": map[string]any{"latin": "Arial", "ea": "宋体", "cs": "Arial"}}, "resetSupplemental": true}
	s, e := prepareDocumentLayout("document-theme", good)
	if e != nil || s.Theme == nil {
		t.Fatalf("%+v %v", s, e)
	}
	delete(good, "resetSupplemental")
	if _, e = prepareDocumentLayout("document-theme", good); e == nil {
		t.Fatal("implicit supplemental reset accepted")
	}
	good["resetSupplemental"] = true
	delete(good["themeFonts"].(map[string]any)["minor"].(map[string]any), "ea")
	if _, e = prepareDocumentLayout("document-theme", good); e == nil {
		t.Fatal("partial theme accepted")
	}
}
func TestDocumentLayoutThemeProofRejectsCollateralAndSupplemental(t *testing.T) {
	s := documentLayoutSpec{Action: "document-theme", Theme: &documentThemeFonts{Major: documentThemeFamily{Latin: "Arial", EA: "黑体", CS: "Arial"}, Minor: documentThemeFamily{Latin: "Arial", EA: "宋体", CS: "Arial"}}}
	before := documentLayoutSnapshot{Theme: &documentThemeSnapshot{Fonts: documentThemeFonts{}, NonFontDocument: "same", NonFontStyles: "same", NonFontTheme: "same"}}
	after := documentLayoutSnapshot{Theme: &documentThemeSnapshot{Fonts: *s.Theme, NonFontDocument: "same", NonFontStyles: "same", NonFontTheme: "same"}}
	if e := verifyDocumentTheme(before, after, s); e != nil {
		t.Fatal(e)
	}
	after.Theme.Supplemental = map[string]string{"minor.Hans": "wrong"}
	if e := verifyDocumentTheme(before, after, s); e == nil {
		t.Fatal("supplemental mappings silently preserved")
	}
	after.Theme.Supplemental = nil
	after.Theme.NonFontStyles = "changed spacing"
	if e := verifyDocumentTheme(before, after, s); e == nil {
		t.Fatal("nonfont change accepted")
	}
}

func TestDocumentLayoutThemeReadsRealDOCXAndResolvesReferences(t *testing.T) {
	file := artifactTestZip(t, map[string]string{
		"word/document.xml":     `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:r><w:t>正文</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`,
		"word/styles.xml":       `<w:styles xmlns:w="` + artifactWordNS + `"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:asciiTheme="minorAscii" w:eastAsiaTheme="minorEastAsia"/></w:rPr></w:rPrDefault></w:docDefaults></w:styles>`,
		"word/theme/theme1.xml": `<a:theme xmlns:a="` + artifactDrawingNS + `"><a:themeElements><a:fontScheme name="Custom"><a:majorFont><a:latin typeface="Arial"/><a:ea typeface="黑体"/><a:cs typeface="Arial"/></a:majorFont><a:minorFont><a:latin typeface="Arial"/><a:ea typeface="宋体"/><a:cs typeface="Arial"/></a:minorFont></a:fontScheme></a:themeElements></a:theme>`,
	})
	s, e := readDocumentLayout(file)
	if e != nil {
		t.Fatal(e)
	}
	if s.Theme.Fonts.Minor.EA != "宋体" {
		t.Fatal(s.Theme)
	}
	resolved, e := documentThemeEffectiveFonts(documentExpanded(s.Text.Paragraphs[0])[0].Attrs, s.Theme.Fonts)
	if e != nil || resolved["eastAsia"] != "宋体" || resolved["ascii"] != "Arial" {
		t.Fatalf("%+v %v", resolved, e)
	}
	if _, e = documentThemeEffectiveFonts(map[string]string{"fontFamily.eastAsiaTheme": "badRef"}, s.Theme.Fonts); e == nil {
		t.Fatal("bad effective theme reference accepted")
	}
}

func TestDocumentLayoutThemePreservesExplicitFontOverrides(t *testing.T) {
	s := documentLayoutSpec{Action: "document-theme", Theme: &documentThemeFonts{Major: documentThemeFamily{Latin: "Arial", EA: "黑体", CS: "Arial"}, Minor: documentThemeFamily{Latin: "Arial", EA: "宋体", CS: "Arial"}}}
	before := documentLayoutSnapshot{Theme: &documentThemeSnapshot{DirectRunFonts: []string{"explicit Calibri"}}}
	after := documentLayoutSnapshot{Theme: &documentThemeSnapshot{Fonts: *s.Theme, DirectRunFonts: []string{"wrong Arial"}}}
	if e := verifyDocumentTheme(before, after, s); e == nil {
		t.Fatal("explicit run fonts overwritten")
	}
}

func TestDocumentLayoutThemeRequiresActualChineseInheritance(t *testing.T) {
	fonts := documentThemeFonts{Major: documentThemeFamily{Latin: "Arial", EA: "黑体", CS: "Arial"}, Minor: documentThemeFamily{Latin: "Arial", EA: "宋体", CS: "Arial"}}
	before := documentLayoutSnapshot{Theme: &documentThemeSnapshot{}, Text: documentSnapshot{Paragraphs: []documentParagraph{{Text: "正文", Chars: []documentChar{{Text: "正文", Attrs: map[string]string{"fontFamily.eastAsia": "宋体"}}}}}}}
	after := documentLayoutSnapshot{Theme: &documentThemeSnapshot{Fonts: fonts}, Text: before.Text}
	spec := documentLayoutSpec{Action: "document-theme", Theme: &fonts}
	if e := verifyDocumentTheme(before, after, spec); e == nil {
		t.Fatal("theme definition without inherited Chinese text accepted")
	}
	after.Text = documentSnapshot{Paragraphs: []documentParagraph{{Text: "正文", Chars: []documentChar{{Text: "正文", Attrs: map[string]string{"fontFamily.eastAsiaTheme": "minorEastAsia"}}}}}}
	if e := verifyDocumentTheme(before, after, spec); e != nil {
		t.Fatal(e)
	}
}
