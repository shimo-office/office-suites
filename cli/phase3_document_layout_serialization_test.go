package main

import "testing"

func TestDocumentPageSerializationDefaults(t *testing.T) {
	before := layoutFixture(t, "", `<w:pgSz w:w="12240" w:h="15840"/><w:docGrid w:linePitch="360"/>`)
	spec := documentLayoutSpec{Action: "document-page", Background: "#EEEEEE"}
	makeAfter := func(orient, grid, sep string) documentLayoutSnapshot {
		s := layoutFixture(t, "", `<w:pgSz w:w="12240" w:h="15840" w:orient="`+orient+`"/><w:docGrid w:linePitch="360" w:type="`+grid+`"/><w:pgNumType w:chapSep="`+sep+`"/>`)
		s.Background = "EEEEEE"
		return s
	}
	if err := verifyDocumentLayout(before, makeAfter("portrait", "default", "hyphen"), spec); err != nil {
		t.Fatal(err)
	}
	for _, s := range []documentLayoutSnapshot{makeAfter("landscape", "default", "hyphen"), makeAfter("portrait", "lines", "hyphen"), makeAfter("portrait", "default", "colon"), layoutFixture(t, "", `<w:pgSz w:w="12240" w:h="15840" w:orient="portrait"/><w:docGrid w:linePitch="361" w:type="default"/><w:pgNumType w:chapSep="hyphen"/>`)} {
		s.Background = "EEEEEE"
		if verifyDocumentLayout(before, s, spec) == nil {
			t.Fatal("real page setting change accepted")
		}
	}
}

func TestDocumentThemeHexColorSerialization(t *testing.T) {
	read := func(color string) *artifactNode {
		t.Helper()
		n, err := parseArtifactXML([]byte(`<w:styles xmlns:w="` + artifactWordNS + `"><w:style w:type="paragraph" w:styleId="Title"><w:pPr><w:pBdr><w:bottom w:color="` + color + `" w:val="single"/></w:pBdr></w:pPr></w:style></w:styles>`))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	original := read("4F81BD")
	raw := layoutNodeCanonical(original)
	normalized := layoutNodeCanonical(documentThemeWithoutFonts(original, false))
	if normalized != layoutNodeCanonical(documentThemeWithoutFonts(read("4f81bd"), false)) {
		t.Fatal("equal hex colors rejected")
	}
	if normalized == layoutNodeCanonical(documentThemeWithoutFonts(read("4f81be"), false)) {
		t.Fatal("real color change accepted")
	}
	if raw != layoutNodeCanonical(original) {
		t.Fatal("normalization mutated original")
	}
}

func TestDocumentThemeLatinReferenceAliases(t *testing.T) {
	for _, reference := range []string{"majorAscii", "minorAscii", "majorBidi"} {
		before := documentLayoutSnapshot{Theme: &documentThemeSnapshot{Fonts: documentThemeFonts{Major: documentThemeFamily{Latin: "Arial"}}}, Text: documentSnapshot{Paragraphs: []documentParagraph{{Text: "Text", Chars: []documentChar{{Text: "Text", Attrs: map[string]string{"fontFamily.asciiTheme": "majorHAnsi"}}}}}}}
		after := before
		after.Text = documentSnapshot{Paragraphs: []documentParagraph{{Text: "Text", Chars: []documentChar{{Text: "Text", Attrs: map[string]string{"fontFamily.asciiTheme": reference}}}}}}
		err := documentThemeCharacterProof(before, after)
		if (err == nil) != (reference == "majorAscii") {
			t.Fatalf("%s: %v", reference, err)
		}
	}
}

func TestDocumentPageAbsentGridDefault(t *testing.T) {
	before := layoutFixture(t, "", "")
	spec := documentLayoutSpec{Action: "document-page", Background: "#EEEEEE"}
	for _, tc := range []struct {
		xml string
		ok  bool
	}{
		{`<w:docGrid w:linePitch="312" w:type="default"/>`, true},
		{`<w:docGrid w:linePitch="312" w:type="lines"/>`, false},
		{`<w:docGrid w:linePitch="360" w:type="default"/>`, false},
		{`<w:pgNumType w:chapSep="hyphen" w:start="2"/>`, false},
	} {
		after := layoutFixture(t, "", tc.xml)
		after.Background = "EEEEEE"
		err := verifyDocumentLayout(before, after, spec)
		if (err == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.xml, err)
		}
	}
}
