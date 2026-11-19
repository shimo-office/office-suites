package main

import (
	"testing"
)

func TestDocumentResourcesDetectCustomThemeAndBindings(t *testing.T) {
	base := func(target, font string) map[string]string {
		return map[string]string{
			"word/document.xml":            `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:r><w:t>A</w:t></w:r></w:p></w:body></w:document>`,
			"word/_rels/document.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="rT" Type="` + artifactOfficeRelNS + `/theme" Target="` + target + `"/></Relationships>`,
			"custom/a.xml":                 `<a:theme xmlns:a="urn:test"><a:font typeface="` + font + `"/></a:theme>`,
			"custom/b.xml":                 `<a:theme xmlns:a="urn:test"><a:font typeface="Courier"/></a:theme>`,
		}
	}
	read := func(parts map[string]string) documentSnapshot {
		t.Helper()
		got, err := readDocumentSnapshot(artifactTestZip(t, parts))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	before := read(base("../custom/a.xml", "Arial"))
	if err := verifyDocumentResources(before.StyleResources, read(base("../custom/a.xml", "Arial")).StyleResources); err != nil {
		t.Fatal(err)
	}
	for _, after := range []documentSnapshot{read(base("../custom/a.xml", "Courier")), read(base("../custom/b.xml", "Arial"))} {
		if verifyDocumentResources(before.StyleResources, after.StyleResources) == nil {
			t.Fatal("changed custom resource or binding accepted")
		}
	}
}

func TestDocumentResourcesRejectMalformedBinding(t *testing.T) {
	parts := map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `"><w:body/></w:document>`, "word/_rels/document.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="rT" Type="` + artifactOfficeRelNS + `/theme" Target="missing.xml"/></Relationships>`}
	if _, err := readDocumentSnapshot(artifactTestZip(t, parts)); err == nil {
		t.Fatal("missing bound resource accepted")
	}
}
