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

func TestDocumentLayoutThemeDoesNotRequireInheritedChinese(t *testing.T) {
	fonts := documentThemeFonts{Major: documentThemeFamily{Latin: "Arial", EA: "黑体", CS: "Arial"}, Minor: documentThemeFamily{Latin: "Arial", EA: "宋体", CS: "Arial"}}
	for _, tc := range []struct {
		name, text string
		attrs      map[string]string
	}{
		{"english", "English only", map[string]string{"fontFamily.asciiTheme": "minorAscii"}},
		{"explicit-chinese", "显式中文", map[string]string{"fontFamily.eastAsia": "仿宋"}},
		{"empty", "", map[string]string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := documentLayoutSnapshot{Theme: &documentThemeSnapshot{}, Text: documentSnapshot{Paragraphs: []documentParagraph{{Text: tc.text, Chars: []documentChar{{Text: tc.text, Attrs: tc.attrs}}}}}}
			after := documentLayoutSnapshot{Theme: &documentThemeSnapshot{Fonts: fonts}, Text: before.Text}
			if err := verifyDocumentTheme(before, after, documentLayoutSpec{Action: "document-theme", Theme: &fonts}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDocumentLayoutThemeCharacterPreservation(t *testing.T) {
	fonts := documentThemeFonts{Major: documentThemeFamily{EA: "黑体"}, Minor: documentThemeFamily{EA: "宋体"}}
	snapshot := func(attrs map[string]string) documentLayoutSnapshot {
		return documentLayoutSnapshot{Theme: &documentThemeSnapshot{Fonts: fonts}, Text: documentSnapshot{Paragraphs: []documentParagraph{{Text: "中文", Chars: []documentChar{{Text: "中文", Attrs: attrs}}}}}}
	}
	inherited := snapshot(map[string]string{"fontFamily.eastAsiaTheme": "minorEastAsia"})
	if err := documentThemeCharacterProof(inherited, inherited); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []documentLayoutSnapshot{snapshot(map[string]string{"fontFamily.eastAsiaTheme": "majorEastAsia"}), snapshot(map[string]string{"fontFamily.eastAsiaTheme": "badRef"})} {
		if documentThemeCharacterProof(inherited, bad) == nil {
			t.Fatal("inherited mapping corruption accepted")
		}
	}
	if documentThemeCharacterProof(snapshot(map[string]string{"fontFamily.eastAsia": "仿宋"}), snapshot(map[string]string{"fontFamily.eastAsia": "宋体"})) == nil {
		t.Fatal("explicit font corruption accepted")
	}
}
