package main

import "testing"

func TestDocumentFormatSpec(t *testing.T) {
	good := map[string]any{"paragraph": float64(1), "text": "x2 and H2O", "match": "x2", "attrs": map[string]any{"fontSize": float64(14), "color": "#ff0000"}}
	spec, err := prepareDocumentFormat(good)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Start != 0 || spec.End != 2 || spec.Attrs["color"] != "#FF0000" {
		t.Fatalf("bad spec: %#v", spec)
	}
	for _, p := range []map[string]any{
		{"paragraph": float64(0), "text": "x", "attrs": map[string]any{"bold": true}},
		{"paragraph": float64(1), "text": "x", "attrs": map[string]any{}},
		{"paragraph": float64(1), "text": "xx", "match": "x", "attrs": map[string]any{"bold": true}},
		{"paragraph": float64(1), "text": "x", "attrs": map[string]any{"fontSize": float64(0)}},
		{"paragraph": float64(1), "text": "x", "attrs": map[string]any{"color": "red"}},
		{"paragraph": float64(1), "text": "x", "attrs": map[string]any{"typo": true}},
		{"paragraph": float64(1), "text": "x", "attrs": map[string]any{"bold": "true"}},
	} {
		if _, err := prepareDocumentFormat(p); err == nil {
			t.Fatalf("accepted invalid spec: %#v", p)
		}
	}
}

func TestDocumentFormatProof(t *testing.T) {
	spec, err := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": "x2", "match": "2", "attrs": map[string]any{"verticalAlign": "superscript"}})
	if err != nil {
		t.Fatal(err)
	}
	before := documentSnapshot{Paragraphs: []documentParagraph{{Text: "x2", Chars: []documentChar{{Text: "x", Attrs: map[string]string{}}, {Text: "2", Attrs: map[string]string{}}}}, {Text: "untouched", Chars: []documentChar{{Text: "untouched", Attrs: map[string]string{}}}}}}
	after := documentSnapshot{Paragraphs: []documentParagraph{{Text: "x2", Chars: []documentChar{{Text: "x", Attrs: map[string]string{}}, {Text: "2", Attrs: map[string]string{"verticalAlign": "superscript"}}}}, before.Paragraphs[1]}}
	if err := verifyDocumentFormat(before, after, spec); err != nil {
		t.Fatal(err)
	}
	if err := verifyDocumentFormat(before, before, spec); err == nil {
		t.Fatal("accepted unchanged artifact")
	}
	collateral := documentSnapshot{Paragraphs: append([]documentParagraph(nil), after.Paragraphs...)}
	collateral.Paragraphs[1] = documentParagraph{Text: "changed"}
	if err := verifyDocumentFormat(before, collateral, spec); err == nil {
		t.Fatal("accepted collateral edit")
	}
	wrong := documentSnapshot{Paragraphs: []documentParagraph{{Text: "x2", Chars: []documentChar{{Text: "x", Attrs: map[string]string{"verticalAlign": "superscript"}}, {Text: "2", Attrs: map[string]string{}}}}, before.Paragraphs[1]}}
	if err := verifyDocumentFormat(before, wrong, spec); err == nil {
		t.Fatal("accepted wrong character style")
	}
}

func TestDocumentFormatSubstringOffset(t *testing.T) {
	s, err := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": "字符 x2 和 H2O", "match": "x2", "offset": float64(1), "length": float64(1), "attrs": map[string]any{"verticalAlign": "superscript"}})
	if err != nil {
		t.Fatal(err)
	}
	if string([]rune(s.Text)[s.Start:s.End]) != "2" {
		t.Fatalf("wrong substring %#v", s)
	}
	for _, off := range []float64{-1, 2, 1.5} {
		_, err := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": "x2", "offset": off, "length": float64(1), "attrs": map[string]any{"bold": true}})
		if err == nil {
			t.Fatalf("accepted invalid offset %v", off)
		}
	}
}

func TestDocumentSnapshotResolvesRunAndParagraphStyles(t *testing.T) {
	name := artifactTestZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:r><w:t>x</w:t></w:r><w:r><w:rPr><w:sz w:val="28"/><w:color w:val="FF0000"/><w:vertAlign w:val="superscript"/></w:rPr><w:t>2</w:t></w:r></w:p></w:body></w:document>`, "word/styles.xml": `<w:styles xmlns:w="` + artifactWordNS + `"><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:rPr><w:sz w:val="22"/><w:i/></w:rPr></w:style></w:styles>`})
	s, err := readDocumentSnapshot(name)
	if err != nil {
		t.Fatal(err)
	}
	chars := documentExpanded(s.Paragraphs[0])
	if len(chars) != 2 || chars[0].Attrs["fontSize"] != "11" || chars[1].Attrs["fontSize"] != "14" || chars[1].Attrs["italic"] != "true" || chars[1].Attrs["verticalAlign"] != "superscript" {
		t.Fatalf("unresolved effective style: %#v", chars)
	}
}
