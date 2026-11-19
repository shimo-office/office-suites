package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestPhase3SlidesMasterBackgroundClosure(t *testing.T) {
	entries := map[string]string{
		"ppt/presentation.xml":                `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId id="256" r:id="one"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels":     `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/slide1.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":               `<p:sld xmlns:p="` + artifactSlideNS + `"><p:cSld><p:spTree/></p:cSld></p:sld>`,
		"ppt/slides/_rels/slide1.xml.rels":    `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="layout" Type="` + artifactOfficeRelNS + `/slideLayout" Target="../slideLayouts/one.xml"/></Relationships>`,
		"ppt/slideLayouts/one.xml":            `<p:sldLayout xmlns:p="` + artifactSlideNS + `"><p:cSld/></p:sldLayout>`,
		"ppt/slideLayouts/_rels/one.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="master" Type="` + artifactOfficeRelNS + `/slideMaster" Target="../slideMasters/one.xml"/></Relationships>`,
		"ppt/slideMasters/one.xml":            `<p:sldMaster xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:cSld><p:bg><a:blipFill><a:blip r:embed="bg"/></a:blipFill></p:bg></p:cSld></p:sldMaster>`,
		"ppt/slideMasters/_rels/one.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="layout" Type="` + artifactOfficeRelNS + `/slideLayout" Target="../slideLayouts/one.xml"/><Relationship Id="bg" Type="` + artifactOfficeRelNS + `/image" Target="../media/bg.png"/></Relationships>`,
		"ppt/media/bg.png":                    "original PNG bytes",
	}
	read := func() []phase3SlidePage {
		t.Helper()
		s, e := readPhase3SlideArtifact(artifactTestZip(t, entries), []string{"a"})
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	before := read()
	entries["ppt/slides/slide1.xml"] = strings.Replace(entries["ppt/slides/slide1.xml"], "<p:sld ", `<p:sld show="0" `, 1)
	hidden := read()
	spec := phase3SlidesSpec{Action: "slide-hide", SlideID: "a"}
	if e := verifyPhase3Slides(before, hidden, spec); e != nil {
		t.Fatal(e)
	}
	entries["ppt/media/bg.png"] = "different PNG bytes"
	after := read()
	if e := verifyPhase3Slides(before, after, spec); e == nil {
		t.Fatal("master background image replacement missed")
	}
	if before[0].Resources == after[0].Resources || before[0].Layout == after[0].Layout {
		t.Fatal("closure hashes ignored transitive image")
	}
	delete(entries, "ppt/media/bg.png")
	if _, e := readPhase3SlideArtifact(artifactTestZip(t, entries), []string{"a"}); e == nil {
		t.Fatal("missing transitive image accepted")
	}
}
func TestPhase3SlidesNonPlaceholderObjectFingerprint(t *testing.T) {
	base := `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="1"/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>title</a:t></a:r></a:p></p:txBody></p:sp><p:pic><p:nvPicPr><p:cNvPr id="2" name="actual image"/></p:nvPicPr></p:pic></p:spTree></p:cSld></p:sld>`
	entries := map[string]string{"ppt/presentation.xml": `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId r:id="one"/></p:sldIdLst></p:presentation>`, "ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/one.xml"/></Relationships>`, "ppt/slides/one.xml": base}
	before, e := readPhase3SlideArtifact(artifactTestZip(t, entries), []string{"a"})
	if e != nil {
		t.Fatal(e)
	}
	if len(before[0].FixedObjects) != 1 {
		t.Fatal("placeholder/image classification incorrect")
	}
	entries["ppt/slides/one.xml"] = strings.Replace(base, `name="actual image"`, `name="replaced image"`, 1)
	after, e := readPhase3SlideArtifact(artifactTestZip(t, entries), []string{"a"})
	if e != nil {
		t.Fatal(e)
	}
	if reflect.DeepEqual(before[0].FixedObjects, after[0].FixedObjects) {
		t.Fatal("image node mutation missed")
	}
}
