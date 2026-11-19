package main

import (
	"strings"
	"testing"
)

func TestPhase3SlideTextFormatSpec(t *testing.T) {
	s, e := preparePhase3Slides("slide-text-format", map[string]any{"slideId": "a", "text": "title", "fontFamily": "Arial", "fontSize": float64(28), "color": "#0000ff"})
	if e != nil {
		t.Fatal(e)
	}
	if s.Format == nil || s.Format.Attrs["color"] != "#0000FF" {
		t.Fatal("wrong format")
	}
	for _, p := range []map[string]any{{"slideId": "a", "text": "title"}, {"slideId": "a", "text": "title", "fontSize": float64(0)}, {"slideId": "a", "text": "title", "color": "blue"}, {"slideId": "a", "text": "title", "outline": "red"}} {
		if _, e := preparePhase3Slides("slide-text-format", p); e == nil {
			t.Fatal("invalid format accepted")
		}
	}
}
func TestPhase3SlideTextFormatMaskAndProof(t *testing.T) {
	raw := `<p:sp xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:nvSpPr><p:cNvPr id="1" hidden="0"/></p:nvSpPr><p:txBody><a:p><a:r><a:rPr sz="2800"><a:solidFill><a:srgbClr val="0000FF"/></a:solidFill><a:latin typeface="Arial"/></a:rPr><a:t>title</a:t></a:r></a:p></p:txBody></p:sp>`
	s := phase3SlideFormatSpec{Text: "title", Attrs: map[string]any{"fontSize": float64(28), "fontFamily": "Arial", "color": "#0000FF"}}
	n, e := parseArtifactXML([]byte(raw))
	if e != nil {
		t.Fatal(e)
	}
	if e = verifyPhase3SlideTextFormatNode(n, s); e != nil {
		t.Fatal(e)
	}
	for _, c := range [][2]string{{`sz="2800"`, `sz="2400"`}, {`val="0000FF"`, `val="FF0000"`}, {`typeface="Arial"`, `typeface="Calibri"`}, {`hidden="0"`, `hidden="1"`}, {`<a:srgbClr val="0000FF"/>`, `<a:srgbClr val="0000FF"><a:alpha val="0"/></a:srgbClr>`}} {
		m, _ := parseArtifactXML([]byte(strings.ReplaceAll(raw, c[0], c[1])))
		if e := verifyPhase3SlideTextFormatNode(m, s); e == nil {
			t.Fatalf("accepted wrong format %v", c)
		}
	}
}

func TestPhase3SlideTextFormatArtifactProof(t *testing.T) {
	fixture := func(size, color, bold, extra string) string {
		return artifactTestZip(t, map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId r:id="one"/></p:sldIdLst></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/one.xml"/></Relationships>`,
			"ppt/slides/one.xml":              `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="1" hidden="0"/></p:nvSpPr><p:txBody><a:p><a:r><a:rPr sz="` + size + `" b="` + bold + `"><a:solidFill><a:srgbClr val="` + color + `"/></a:solidFill></a:rPr><a:t>title</a:t></a:r></a:p></p:txBody></p:sp>` + extra + `</p:spTree></p:cSld></p:sld>`,
		})
	}
	pic := `<p:pic><p:nvPicPr><p:cNvPr id="2" name="retain me"/></p:nvPicPr></p:pic>`
	before := fixture("1200", "FF0000", "1", pic)
	after := fixture("2800", "0000FF", "1", pic)
	s := phase3SlidesSpec{Action: "slide-text-format", SlideID: "a", Format: &phase3SlideFormatSpec{Text: "title", Attrs: map[string]any{"fontSize": float64(28), "color": "#0000FF"}}}
	if e := verifyPhase3SlideTextFormatArtifacts(before, after, []string{"a"}, []string{"a"}, s); e != nil {
		t.Fatal(e)
	}
	if e := verifyPhase3SlideTextFormatArtifacts(before, fixture("2800", "0000FF", "0", pic), []string{"a"}, []string{"a"}, s); e == nil {
		t.Fatal("unrequested bold change accepted")
	}
	if e := verifyPhase3SlideTextFormatArtifacts(before, fixture("2800", "0000FF", "1", ""), []string{"a"}, []string{"a"}, s); e == nil {
		t.Fatal("image loss accepted")
	}
	if _, e := readPhase3SlideArtifactFiltered(after, []string{"a"}, phase3SlideTextFormatFilter([]string{"a"}, s, false, true)); e == nil {
		t.Fatal("already requested format not rejected before write")
	}
}

func TestPhase3SlideTextFormatRejectsUnresolvedInheritedStyle(t *testing.T) {
	fixture := func(attrs, fonts, text string) *artifactNode {
		n, e := parseArtifactXML([]byte(`<p:sp xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:nvSpPr><p:cNvPr id="1"/></p:nvSpPr><p:txBody><a:p><a:r><a:rPr ` + attrs + `>` + fonts + `</a:rPr><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp>`))
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	for _, key := range []string{"bold", "italic", "underline"} {
		if e := verifyPhase3SlideTextFormatNode(fixture("", "", "title"), phase3SlideFormatSpec{Text: "title", Attrs: map[string]any{key: false}}); e == nil {
			t.Fatalf("inherited %s falsely accepted as false", key)
		}
	}
	s := phase3SlideFormatSpec{Text: "标题", Attrs: map[string]any{"fontFamily": "Arial"}}
	if e := verifyPhase3SlideTextFormatNode(fixture("", `<a:latin typeface="Arial"/>`, "标题"), s); e == nil {
		t.Fatal("unresolved East Asian font accepted")
	}
	if e := verifyPhase3SlideTextFormatNode(fixture(`b="0" i="0" u="none"`, `<a:latin typeface="Arial"/><a:ea typeface="Arial"/><a:cs typeface="Arial"/>`, "标题"), s); e != nil {
		t.Fatal(e)
	}
}
