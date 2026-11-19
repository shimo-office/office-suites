package main

import "testing"

func TestDocumentRunFontCascadeClearsInheritedAlternative(t *testing.T) {
	for _, pair := range [][2]string{{"ascii", "asciiTheme"}, {"hAnsi", "hAnsiTheme"}, {"eastAsia", "eastAsiaTheme"}, {"cs", "cstheme"}} {
		for _, tc := range []struct{ base, current, absent string }{{pair[1], pair[0], pair[1]}, {pair[0], pair[1], pair[0]}} {
			styles := `<w:styles xmlns:w="` + artifactWordNS + `"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:` + tc.base + `="Inherited"/></w:rPr></w:rPrDefault></w:docDefaults></w:styles>`
			doc := `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:r><w:rPr><w:rFonts w:` + tc.current + `="Current"/></w:rPr><w:t>A</w:t></w:r></w:p></w:body></w:document>`
			s, err := readDocumentSnapshot(artifactTestZip(t, map[string]string{"word/document.xml": doc, "word/styles.xml": styles}))
			if err != nil {
				t.Fatal(err)
			}
			a := documentExpanded(s.Paragraphs[0])[0].Attrs
			if a["fontFamily."+tc.current] != "Current" || a["fontFamily."+tc.absent] != "" {
				t.Fatalf("slot %s unresolved: %#v", tc.current, a)
			}
		}
	}
}

func TestDocumentRunFontSameLevelPairRetained(t *testing.T) {
	root, err := parseArtifactXML([]byte(`<w:rPr xmlns:w="` + artifactWordNS + `"><w:rFonts w:eastAsia="Literal" w:eastAsiaTheme="minorEastAsia"/></w:rPr>`))
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]string{"fontFamily.eastAsia": "Old", "fontFamily.eastAsiaTheme": "majorEastAsia", "fontFamily.ascii": "Untouched"}
	got := documentRunAttrs(base, root)
	if got["fontFamily.eastAsia"] != "Literal" || got["fontFamily.eastAsiaTheme"] != "minorEastAsia" || got["fontFamily.ascii"] != "Untouched" || base["fontFamily.eastAsia"] != "Old" {
		t.Fatalf("same-level pair or inherited source changed: %#v %#v", got, base)
	}
}
