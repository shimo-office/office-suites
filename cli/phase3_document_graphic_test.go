package main

import (
	"strings"
	"testing"
)

func TestDocumentGraphicSpecRequiresPreciseTargetAndChange(t *testing.T) {
	good := map[string]any{"name": "PHASE3_ARROW", "description": "PHASE3_ARROW_20261118", "widthCm": float64(4), "heightCm": float64(3), "rotationDegrees": float64(90)}
	s, err := prepareDocumentGraphic(good)
	if err != nil || s.Name != "PHASE3_ARROW" {
		t.Fatal(s, err)
	}
	for _, bad := range []map[string]any{
		{"name": "x", "description": "d"},
		{"name": "x", "description": "d", "widthCm": float64(3)},
		{"name": "x", "description": "d", "widthCm": float64(1e-9), "heightCm": float64(1e-9)},
		{"name": "x", "description": "d", "rotationDegrees": float64(360)},
		{"name": "x", "description": "d", "rotationDegrees": float64(-1)},
		{"name": "x", "description": "d", "rotationDegrees": float64(90), "javascript": "x"},
	} {
		if _, err := prepareDocumentGraphic(bad); err == nil {
			t.Fatal("unsafe graphic request accepted")
		}
	}
}

func TestDocumentGraphicProofPreservesImageAndUnrelatedXML(t *testing.T) {
	fixture := func(rot, width, height, extra, other, media string) string {
		doc := `<w:document xmlns:w="` + artifactWordNS + `" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body><w:p><w:r><w:drawing><wp:inline><wp:extent cx="` + width + `" cy="` + height + `"/><wp:docPr id="1" name="PHASE3_ARROW" descr="PHASE3_ARROW_20261118"/><a:graphic><a:graphicData><pic:pic><pic:spPr><a:xfrm` + rot + extra + `><a:off x="0" y="0"/><a:ext cx="` + width + `" cy="` + height + `"/></a:xfrm><a:prstGeom prst="rect"/></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p><w:p><w:r><w:t>` + other + `</w:t></w:r></w:p></w:body></w:document>`
		return artifactTestZip(t, map[string]string{"word/document.xml": doc, "word/media/image.png": media})
	}
	before := fixture("", "1080000", "720000", "", "untouched", "original-png")
	after := fixture(` rot="5400000"`, "1440000", "1080000", "", "untouched", "original-png")
	s, err := prepareDocumentGraphic(map[string]any{"name": "PHASE3_ARROW", "description": "PHASE3_ARROW_20261118", "widthCm": float64(4), "heightCm": float64(3), "rotationDegrees": float64(90)})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyDocumentGraphic(before, after, s); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{before, fixture(` rot="5400000"`, "1440000", "1080000", ` flipH="1"`, "untouched", "original-png"), fixture(` rot="5400000"`, "1440000", "1080000", "", "changed", "original-png"), fixture(` rot="5400000"`, "1440000", "1080000", "", "untouched", "replacement-png")} {
		if verifyDocumentGraphic(before, bad, s) == nil {
			t.Fatal("incorrect graphic proof accepted")
		}
	}
	s.Name = "wrong"
	if err := verifyDocumentGraphic(before, after, s); err == nil || !strings.Contains(err.Error(), "目标") {
		t.Fatal("missing target accepted")
	}
}

func TestDocumentNativeShapeFillAndLineProof(t *testing.T) {
	fixture := func(color, width, lineColor, dash string) string {
		doc := `<w:document xmlns:w="` + artifactWordNS + `" xmlns:wp="` + documentDrawingNS + `" xmlns:a="` + documentDrawingMainNS + `" xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><w:body><w:p><w:r><w:drawing><wp:inline><wp:extent cx="1080000" cy="720000"/><wp:docPr id="1" name="SHAPE" descr="native"/><a:graphic><a:graphicData><wps:wsp><wps:cNvSpPr/><wps:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="1080000" cy="720000"/></a:xfrm><a:prstGeom prst="rightArrow"/><a:solidFill><a:srgbClr val="` + color + `"/></a:solidFill><a:ln w="` + width + `" cap="rnd"><a:solidFill><a:srgbClr val="` + lineColor + `"/></a:solidFill><a:prstDash val="` + dash + `"/></a:ln></wps:spPr><wps:bodyPr/></wps:wsp></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p></w:body></w:document>`
		return artifactTestZip(t, map[string]string{"word/document.xml": doc})
	}
	before := fixture("FF0000", "12700", "000000", "solid")
	after := fixture("0000FF", "25400", "00FF00", "solid")
	s, err := prepareDocumentGraphic(map[string]any{"name": "SHAPE", "description": "native", "fill": "#0000FF", "line": map[string]any{"color": "#00FF00", "widthPt": float64(2)}})
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyDocumentGraphic(before, after, s); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{before, fixture("0000FF", "25400", "00FF00", "dash"), fixture("0000FF", "12700", "00FF00", "solid")} {
		if verifyDocumentGraphic(before, bad, s) == nil {
			t.Fatal("incorrect shape styling accepted")
		}
	}
}

func TestDocumentGraphicInvalidWidthsAndCompetingFill(t *testing.T) {
	fixture := func(width, line, fillExtra, chart string) string {
		doc := `<w:document xmlns:w="` + artifactWordNS + `" xmlns:r="` + artifactOfficeRelNS + `" xmlns:wp="` + documentDrawingNS + `" xmlns:a="` + documentDrawingMainNS + `" xmlns:wps="` + documentShapeNS + `"><w:body><w:p><w:r><w:drawing><wp:inline><wp:extent cx="` + width + `" cy="720000"/><wp:docPr id="1" name="SHAPE" descr="native"/><a:graphic><a:graphicData><wps:wsp><wps:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="` + width + `" cy="720000"/></a:xfrm><a:prstGeom prst="rect"/><a:solidFill><a:srgbClr val="0000FF"/></a:solidFill>` + fillExtra + `<a:ln w="` + line + `"><a:solidFill><a:srgbClr val="00FF00"/></a:solidFill></a:ln></wps:spPr></wps:wsp></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p><w:p><w:r><w:drawing><a:graphic><a:graphicData><c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" r:id="rChart"/></a:graphicData></a:graphic></w:drawing></w:r></w:p></w:body></w:document>`
		rels := `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="rChart" Type="` + artifactOfficeRelNS + `/chart" Target="charts/chart1.xml"/></Relationships>`
		return artifactTestZip(t, map[string]string{"word/document.xml": doc, "word/_rels/document.xml.rels": rels, "word/charts/chart1.xml": chart})
	}
	before := fixture("1080000", "12700", "", "<chart>original</chart>")
	goodSpec, _ := prepareDocumentGraphic(map[string]any{"name": "SHAPE", "description": "native", "widthCm": float64(4), "heightCm": float64(2), "fill": "#0000FF", "line": map[string]any{"color": "#00FF00", "widthPt": float64(2)}})
	if err := verifyDocumentGraphic(before, fixture("1440000", "25400", "", "<chart>original</chart>"), goodSpec); err != nil {
		t.Fatal("valid control failed", err)
	}
	if err := verifyDocumentGraphic(fixture("1080000", "12700", "<a:noFill/>", "<chart>original</chart>"), fixture("1440000", "25400", "", "<chart>original</chart>"), goodSpec); err == nil {
		t.Fatal("ambiguous baseline fill accepted")
	}
	for _, kind := range []string{"nan-size", "nan-line", "competing-fill", "unrelated-chart"} {
		t.Run(kind, func(t *testing.T) {
			width, line, fill, chart := "1440000", "25400", "", "<chart>original</chart>"
			switch kind {
			case "nan-size":
				width = "NaN"
			case "nan-line":
				line = "NaN"
			case "competing-fill":
				fill = "<a:noFill/>"
			case "unrelated-chart":
				chart = "<chart>changed</chart>"
			}
			s, e := prepareDocumentGraphic(map[string]any{"name": "SHAPE", "description": "native", "widthCm": float64(4), "heightCm": float64(2), "fill": "#0000FF", "line": map[string]any{"color": "#00FF00", "widthPt": float64(2)}})
			if e != nil {
				t.Fatal(e)
			}
			if e = verifyDocumentGraphic(before, fixture(width, line, fill, chart), s); e == nil {
				t.Fatal("accepted", kind)
			}
		})
	}
}
