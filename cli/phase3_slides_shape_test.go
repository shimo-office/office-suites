package main

import (
	"strings"
	"testing"
)

func TestPhase3SlideShapeSpec(t *testing.T) {
	s, e := preparePhase3Slides("slide-shape-format", map[string]any{"slideId": "a", "shapeName": "Rectangle 1", "fillColor": "#ff0000", "rotation": float64(45), "horizontalCenter": true, "behindName": "Title 1"})
	if e != nil || s.Shape == nil || s.Shape.FillColor != "#FF0000" {
		t.Fatalf("%+v %v", s, e)
	}
	for _, p := range []map[string]any{{"slideId": "a", "shapeName": "x"}, {"slideId": "a", "shapeName": "x", "rotation": float64(361)}, {"slideId": "a", "shapeName": "x", "fillColor": "red"}, {"slideId": "a", "shapeName": "x", "behindName": "x"}} {
		if _, e := preparePhase3Slides("slide-shape-format", p); e == nil {
			t.Fatalf("accepted %v", p)
		}
	}
}
func TestPhase3SlideShapeTargetProof(t *testing.T) {
	raw := `<p:sp xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:nvSpPr><p:cNvPr id="7" name="Rectangle 1"/></p:nvSpPr><p:spPr><a:xfrm rot="2700000"><a:off x="400" y="200"/><a:ext cx="200" cy="100"/></a:xfrm><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill><a:ln w="12700"><a:solidFill><a:srgbClr val="0000FF"/></a:solidFill></a:ln></p:spPr></p:sp>`
	s := phase3SlideShapeSpec{Name: "Rectangle 1", FillColor: "#FF0000", LineColor: "#0000FF", Rotation: ptrSlideNumber(45), HorizontalCenter: true, CenterX: 400}
	n, _ := parseArtifactXML([]byte(raw))
	if e := verifyPhase3SlideShapeNode(n, s); e != nil {
		t.Fatal(e)
	}
	quantized, _ := parseArtifactXML([]byte(strings.Replace(raw, `rot="2700000"`, `rot="2699999"`, 1)))
	if e := verifyPhase3SlideShapeNode(quantized, s); e != nil {
		t.Fatalf("native integer angle quantization rejected: %v", e)
	}
	for _, v := range [][2]string{{`rot="2700000"`, `rot="0"`}, {`val="FF0000"`, `val="00FF00"`}, {`x="400"`, `x="500"`}, {`id="7"`, `id="7" hidden="1"`}, {`<a:srgbClr val="0000FF"/>`, `<a:srgbClr val="0000FF"><a:alpha val="0"/></a:srgbClr>`}} {
		x, _ := parseArtifactXML([]byte(strings.Replace(raw, v[0], v[1], 1)))
		if e := verifyPhase3SlideShapeNode(x, s); e == nil {
			t.Fatalf("accepted %v", v)
		}
	}
	for _, angle := range []string{"2699998", "2700002"} {
		x, _ := parseArtifactXML([]byte(strings.Replace(raw, `rot="2700000"`, `rot="`+angle+`"`, 1)))
		if e := verifyPhase3SlideShapeNode(x, s); e == nil {
			t.Fatal("more than one DrawingML angle unit accepted")
		}
	}
}
func ptrSlideNumber(v float64) *float64 { return &v }

func TestPhase3SlideShapeNativeDefaultsStayLocal(t *testing.T) {
	fixture := func(targetAttrs, paragraph, otherAttrs, otherParagraph, master string) string {
		shape := func(name, attrs, para string) string {
			return `<p:sp><p:nvSpPr><p:cNvPr name="` + name + `"/></p:nvSpPr><p:spPr ` + attrs + `><a:xfrm><a:off x="1" y="2"/><a:ext cx="20" cy="10"/></a:xfrm><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p>` + para + `</a:p></p:txBody></p:sp>`
		}
		return artifactTestZip(t, map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId r:id="one"/></p:sldIdLst><p:sldSz cx="1000" cy="600"/></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/one.xml"/></Relationships>`,
			"ppt/slides/one.xml":              `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `" xmlns:z="urn:foreign"><p:cSld><p:spTree>` + shape("Rect", targetAttrs, paragraph) + shape("Other", otherAttrs, otherParagraph) + `</p:spTree></p:cSld></p:sld>`,
			"ppt/slideMasters/master.xml":     `<p:sldMaster xmlns:p="` + artifactSlideNS + `">` + master + `</p:sldMaster>`,
		})
	}
	s := phase3SlidesSpec{Action: "slide-shape-format", SlideID: "a", Shape: &phase3SlideShapeSpec{Name: "Rect", FillColor: "#FF0000"}}
	before := fixture("", "", "", "", "")
	after := fixture(`bwMode="auto"`, `<a:pPr/><a:endParaRPr/>`, "", "", "")
	if err := verifyPhase3SlideShapeArtifacts(before, after, []string{"a"}, []string{"a"}, s); err != nil {
		t.Fatalf("target-only native empty defaults rejected: %v", err)
	}
	for _, bad := range []string{
		fixture(`bwMode="auto"`, `<a:pPr lvl="1"/><a:endParaRPr/>`, "", "", ""),
		fixture(`bwMode="auto"`, `<a:pPr/><a:endParaRPr sz="100"/>`, "", "", ""),
		fixture(`bwMode="auto"`, `<a:pPr/><a:pPr/><a:endParaRPr/>`, "", "", ""),
		fixture(`bwMode="auto"`, `<a:pPr/><a:endParaRPr/><a:endParaRPr/>`, "", "", ""),
		fixture(`bwMode="auto"`, `<a:endParaRPr/><a:pPr/>`, "", "", ""),
		fixture(`bwMode="auto"`, `<z:pPr/><a:endParaRPr/>`, "", "", ""),
		fixture(`z:bwMode="auto"`, `<a:pPr/><a:endParaRPr/>`, "", "", ""),
		fixture(`bwMode="auto"`, `<a:pPr/><a:endParaRPr/>`, `bwMode="auto"`, "", ""),
		fixture(`bwMode="auto"`, `<a:pPr/><a:endParaRPr/>`, "", `<a:pPr/><a:endParaRPr/>`, ""),
	} {
		if err := verifyPhase3SlideShapeArtifacts(before, bad, []string{"a"}, []string{"a"}, s); err == nil {
			t.Fatal("nonempty, foreign or non-target default change accepted")
		}
	}
	for _, bad := range []string{fixture(`bwMode="black"`, "", "", "", ""), fixture("", "", "", "", `<p:spPr bwMode="black"/>`)} {
		if err := phase3PrepareSlideShape(bad, []string{"a"}, &s); err == nil {
			t.Fatal("non-auto inherited/target BW mode passed preflight")
		}
	}
}

func TestPhase3SlideShapeArtifactExactDiff(t *testing.T) {
	fixture := func(fill, rotation, x, extra string, front bool) string {
		target := `<p:sp><p:nvSpPr><p:cNvPr id="1" name="Rect"/></p:nvSpPr><p:spPr><a:xfrm rot="` + rotation + `"><a:off x="` + x + `" y="40"/><a:ext cx="200" cy="100"/></a:xfrm><a:solidFill><a:srgbClr val="` + fill + `"/></a:solidFill></p:spPr><p:txBody><a:p><a:r><a:t>preserved text</a:t></a:r></a:p></p:txBody></p:sp>`
		other := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Reference"/></p:nvSpPr><p:spPr/></p:sp>`
		objects := target + other
		if front {
			objects = other + target
		}
		return artifactTestZip(t, map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId r:id="one"/></p:sldIdLst><p:sldSz cx="1000" cy="600"/></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/one.xml"/></Relationships>`,
			"ppt/slides/one.xml":              `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree>` + objects + extra + `</p:spTree></p:cSld></p:sld>`})
	}
	s := phase3SlidesSpec{Action: "slide-shape-format", SlideID: "a", Shape: &phase3SlideShapeSpec{Name: "Rect", FillColor: "#FF0000", Rotation: ptrSlideNumber(45), HorizontalCenter: true, BehindName: "Reference"}}
	before := fixture("0000FF", "0", "20", "", true)
	if e := phase3PrepareSlideShape(before, []string{"a"}, &s); e != nil {
		t.Fatal(e)
	}
	if s.Shape.Delta != 45 || s.Shape.CenterX != 400 || s.Shape.BackwardSteps != 1 {
		t.Fatalf("bad computed target %+v", s.Shape)
	}
	after := fixture("FF0000", "2700000", "400", "", false)
	if e := verifyPhase3SlideShapeArtifacts(before, after, []string{"a"}, []string{"a"}, s); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{fixture("FF0000", "2700000", "400", "", true), fixture("FF0000", "2700000", "400", `<p:pic/>`, false), fixture("FF0000", "0", "400", "", false)} {
		if e := verifyPhase3SlideShapeArtifacts(before, bad, []string{"a"}, []string{"a"}, s); e == nil {
			t.Fatal("collateral/wrong shape artifact accepted")
		}
	}
}

func TestPhase3SlideShapeRejectsAmbiguousName(t *testing.T) {
	for _, raw := range []string{`<p:spTree xmlns:p="` + artifactSlideNS + `"><p:sp><p:nvSpPr><p:cNvPr name="other"/></p:nvSpPr></p:sp></p:spTree>`, `<p:spTree xmlns:p="` + artifactSlideNS + `"><p:sp><p:nvSpPr><p:cNvPr name="duplicate"/></p:nvSpPr></p:sp><p:sp><p:nvSpPr><p:cNvPr name="duplicate"/></p:nvSpPr></p:sp></p:spTree>`} {
		n, e := parseArtifactXML([]byte(raw))
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e = phase3ShapePositions(n, phase3SlideShapeSpec{Name: "duplicate"}); e == nil {
			t.Fatal("missing/duplicate target name accepted")
		}
	}
}
func TestPhase3SlideDesignShapeAllowlist(t *testing.T) {
	js := phase3SlideDesignJS()
	for _, token := range []string{"shapes:s.getShapes().map", "id:shape.id", "name:shape.name", "type:shape.type", "shape.textContent"} {
		if !strings.Contains(js, token) {
			t.Fatalf("inspect omits %s", token)
		}
	}
	if strings.Contains(js, "JSON.stringify(sdk)") || strings.Contains(js, "...shape") {
		t.Fatal("inspect must never serialize complete SDK shape")
	}
}
