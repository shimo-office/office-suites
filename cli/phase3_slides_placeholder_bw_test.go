package main

import "testing"

func phase3PlaceholderBWArtifact(t *testing.T, content, target, other, master string) string {
	t.Helper()
	return artifactTestZip(t, map[string]string{
		"ppt/presentation.xml":            `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId r:id="one"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/one.xml"/></Relationships>`,
		"ppt/slides/one.xml":              `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `" xmlns:z="urn:foreign"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="7" name="Title"/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr ` + target + `/><p:txBody><a:bodyPr/><a:lstStyle/><a:p>` + content + `</a:p></p:txBody></p:sp><p:sp><p:nvSpPr><p:cNvPr id="8" name="Other"/></p:nvSpPr><p:spPr ` + other + `/><p:txBody><a:p><a:r><a:t>keep</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`,
		"ppt/slideMasters/master.xml":     `<p:sldMaster xmlns:p="` + artifactSlideNS + `"><p:cSld><p:spTree><p:sp><p:spPr ` + master + `/></p:sp></p:spTree></p:cSld></p:sldMaster>`,
	})
}
func TestPhase3PlaceholderBWIsLocalAndPreflighted(t *testing.T) {
	before := phase3PlaceholderBWArtifact(t, "", "", "", "")
	spec := phase3SlidesSpec{Action: "slide-placeholder-fill", SlideID: "a", Placeholder: &phase3SlidePlaceholderSpec{Name: "Title", Text: "filled"}}
	text := `<a:r><a:t>filled</a:t></a:r>`
	good := phase3PlaceholderBWArtifact(t, text, `bwMode="auto"`, "", "")
	if e := verifyPhase3SlidePlaceholderArtifacts(before, good, []string{"a"}, []string{"a"}, spec); e != nil {
		t.Fatal(e)
	}
	for name, after := range map[string]string{
		"target changes mode":         phase3PlaceholderBWArtifact(t, text, `bwMode="black"`, "", ""),
		"foreign mode not normalized": phase3PlaceholderBWArtifact(t, text, `z:bwMode="auto"`, "", ""),
		"other shape gains auto":      phase3PlaceholderBWArtifact(t, text, `bwMode="auto"`, `bwMode="auto"`, ""),
		"master changes mode":         phase3PlaceholderBWArtifact(t, text, `bwMode="auto"`, "", `bwMode="black"`),
	} {
		t.Run(name, func(t *testing.T) {
			if e := verifyPhase3SlidePlaceholderArtifacts(before, after, []string{"a"}, []string{"a"}, spec); e == nil {
				t.Fatal("collateral BW change accepted")
			}
		})
	}
	inherited := phase3PlaceholderBWArtifact(t, "", "", "", `bwMode="black"`)
	if e := phase3PlaceholderBWPreflight(inherited); e == nil {
		t.Fatal("non-auto existing master must be rejected before write")
	}
}
