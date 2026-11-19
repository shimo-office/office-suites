package main

import "testing"

func documentAssetsFixture(t *testing.T, formatted, mentioned bool, mutation string) documentSnapshot {
	t.Helper()
	attrs, text := "", "x"
	if formatted {
		attrs = "<w:rPr><w:b/></w:rPr>"
	}
	if mentioned {
		text += "@Self"
	}
	doc := `<w:document xmlns:w="` + artifactWordNS + `" xmlns:r="` + artifactOfficeRelNS + `" xmlns:a="` + documentDrawingMainNS + `"><w:body><w:p><w:r>` + attrs + `<w:t>` + text + `</w:t></w:r></w:p><w:p><w:r><w:drawing><a:blip r:embed="image"/></w:drawing></w:r></w:p><w:sectPr><w:headerReference r:id="header"/></w:sectPr></w:body></w:document>`
	rels := `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="image" Type="` + artifactOfficeRelNS + `/image" Target="media/image.png"/><Relationship Id="header" Type="` + artifactOfficeRelNS + `/header" Target="header1.xml"/></Relationships>`
	parts := map[string]string{"word/document.xml": doc, "word/_rels/document.xml.rels": rels, "word/media/image.png": "original image bytes", "word/header1.xml": `<w:hdr xmlns:w="` + artifactWordNS + `"><w:p><w:r><w:t>HEADER</w:t></w:r></w:p></w:hdr>`, "word/charts/chart1.xml": "<chart>original data</chart>", "word/embeddings/data.xlsx": "original embedded sheet"}
	switch mutation {
	case "image":
		parts["word/media/image.png"] = "different image bytes"
	case "header":
		parts["word/header1.xml"] = `<w:hdr xmlns:w="` + artifactWordNS + `"><w:p><w:r><w:t>CHANGED</w:t></w:r></w:p></w:hdr>`
	case "chart":
		parts["word/charts/chart1.xml"] = "<chart>changed</chart>"
	case "embedding":
		parts["word/embeddings/data.xlsx"] = "changed embedded sheet"
	case "deleted":
		delete(parts, "word/embeddings/data.xlsx")
	case "added":
		parts["word/embeddings/extra.xlsx"] = "new embedded sheet"
	case "rebound":
		parts["word/header2.xml"] = parts["word/header1.xml"]
		parts["word/_rels/document.xml.rels"] = `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="image" Type="` + artifactOfficeRelNS + `/image" Target="media/image.png"/><Relationship Id="header" Type="` + artifactOfficeRelNS + `/header" Target="header2.xml"/></Relationships>`
	}
	snapshot, err := readDocumentSnapshot(artifactTestZip(t, parts))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestDocumentFormatRejectsNonTargetPackageAssets(t *testing.T) {
	before := documentAssetsFixture(t, false, false, "")
	spec, _ := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": "x", "attrs": map[string]any{"bold": true}})
	if err := verifyDocumentFormat(before, documentAssetsFixture(t, true, false, ""), spec); err != nil {
		t.Fatal("valid formatting rejected", err)
	}
	for _, mutation := range []string{"image", "header", "chart", "embedding", "deleted", "added", "rebound"} {
		t.Run(mutation, func(t *testing.T) {
			if verifyDocumentFormat(before, documentAssetsFixture(t, true, false, mutation), spec) == nil {
				t.Fatal("non-target package mutation accepted")
			}
		})
	}
}

func TestDocumentMentionRejectsNonTargetPackageAssets(t *testing.T) {
	before := documentAssetsFixture(t, false, false, "")
	spec, _ := prepareDocumentMention(map[string]any{"paragraph": float64(1), "text": "x"})
	if err := verifyDocumentMention(before, documentAssetsFixture(t, false, true, ""), spec, "Self"); err != nil {
		t.Fatal("valid mention export rejected", err)
	}
	for _, mutation := range []string{"image", "header", "chart", "embedding", "deleted", "added", "rebound"} {
		t.Run(mutation, func(t *testing.T) {
			if verifyDocumentMention(before, documentAssetsFixture(t, false, true, mutation), spec, "Self") == nil {
				t.Fatal("mention hid non-target package mutation")
			}
		})
	}
}
