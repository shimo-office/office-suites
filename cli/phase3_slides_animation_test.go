package main

import (
	"strings"
	"testing"
)

func TestPhase3SlideAnimationSpec(t *testing.T) {
	s, e := preparePhase3Slides("slide-title-animate", map[string]any{"targets": []any{map[string]any{"slideId": "a", "text": "Title A"}, map[string]any{"slideId": "b", "text": "Title B"}}, "fontSize": float64(24)})
	if e != nil || s.Animation == nil || s.SlideID != "a" {
		t.Fatalf("%+v %v", s, e)
	}
	if _, e = preparePhase3Slides("slide-title-animate", map[string]any{"targets": []any{map[string]any{"slideId": "a", "text": "A"}, map[string]any{"slideId": "a", "text": "B"}}, "fontSize": float64(24)}); e == nil {
		t.Fatal("duplicate page targets accepted")
	}
}
func TestPhase3SlideAnimationProof(t *testing.T) {
	raw := `<p:timing xmlns:p="` + artifactSlideNS + `"><p:tnLst><p:par><p:cTn presetClass="entr" presetID="1" nodeType="clickEffect"><p:childTnLst><p:set><p:cBhvr><p:cTn dur="1"/><p:tgtEl><p:spTgt spid="7"/></p:tgtEl><p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst></p:cBhvr><p:to><p:strVal val="visible"/></p:to></p:set></p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`
	n, _ := parseArtifactXML([]byte(raw))
	if e := verifyPhase3AppearTiming(n, "7"); e != nil {
		t.Fatal(e)
	}
	for _, v := range [][2]string{{`spid="7"`, `spid="9"`}, {`presetClass="entr"`, `presetClass="exit"`}, {`val="visible"`, `val="hidden"`}, {`style.visibility`, `style.color`}} {
		x, _ := parseArtifactXML([]byte(strings.Replace(raw, v[0], v[1], 1)))
		if e := verifyPhase3AppearTiming(x, "7"); e == nil {
			t.Fatalf("accepted %v", v)
		}
	}
}

func TestPhase3SlideAnimationArtifactProof(t *testing.T) {
	fixture := func(size, timing, other string) string {
		return artifactTestZip(t, map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId r:id="one"/></p:sldIdLst></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/one.xml"/></Relationships>`,
			"ppt/slides/one.xml":              `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="7" name="title"/></p:nvSpPr><p:txBody><a:p><a:r><a:rPr sz="` + size + `"/><a:t>Title</a:t></a:r></a:p></p:txBody></p:sp><p:sp><p:nvSpPr><p:cNvPr id="9" name="body"/></p:nvSpPr><p:txBody><a:p><a:r><a:t>` + other + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld>` + timing + `</p:sld>`})
	}
	timing := `<p:timing><p:tnLst><p:par><p:cTn presetClass="entr" presetID="1" nodeType="clickEffect"><p:childTnLst><p:set><p:cBhvr><p:cTn dur="1"/><p:tgtEl><p:spTgt spid="7"/></p:tgtEl><p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst></p:cBhvr><p:to><p:strVal val="visible"/></p:to></p:set></p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`
	s := phase3SlidesSpec{Animation: &phase3SlideAnimationSpec{Targets: []phase3SlideAnimationTarget{{SlideID: "a", Text: "Title"}}, FontSize: 24}}
	before := fixture("1200", "", "keep")
	after := fixture("2400", timing, "keep")
	if e := verifyPhase3SlideAnimationArtifacts(before, after, []string{"a"}, []string{"a"}, s); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{fixture("1200", timing, "keep"), fixture("2400", timing, "changed"), fixture("2400", "", "keep")} {
		if e := verifyPhase3SlideAnimationArtifacts(before, bad, []string{"a"}, []string{"a"}, s); e == nil {
			t.Fatal("incomplete/collateral animation batch accepted")
		}
	}
	if e := phase3PrepareSlideAnimation(after, []string{"a"}, s); e == nil {
		t.Fatal("existing animation overwritten")
	}
}
