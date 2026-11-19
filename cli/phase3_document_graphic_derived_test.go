package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestDocumentGraphicRecordedDerivedDefaults(t *testing.T) {
	for _, kind := range []string{"image", "preset", "direct"} {
		t.Run(kind, func(t *testing.T) {
			before, e := os.ReadFile("testdata/phase3-graphic/" + kind + "-before.xml")
			if e != nil {
				t.Fatal(e)
			}
			after, e := os.ReadFile("testdata/phase3-graphic/" + kind + "-after.xml")
			if e != nil {
				t.Fatal(e)
			}
			payload := map[string]any{"paragraph": float64(2), "expectedParagraph": "", "drawing": float64(1)}
			if kind == "image" {
				payload["widthCm"] = float64(6)
				payload["heightCm"] = float64(4)
				payload["rotationDegrees"] = float64(90)
			} else if kind == "direct" {
				payload["fill"] = "#0000FF"
				payload["line"] = map[string]any{"color": "#FF0000", "widthPt": float64(2)}
			} else {
				payload["preset"] = float64(2)
			}
			spec, e := prepareDocumentGraphic(payload)
			if e != nil {
				t.Fatal(e)
			}
			zip := func(doc string, theme bool) string {
				parts := map[string]string{"word/document.xml": doc}
				if theme {
					parts["word/theme/theme1.xml"] = `<a:theme xmlns:a="` + documentDrawingMainNS + `"/>`
				}
				return artifactTestZip(t, parts)
			}
			if e = verifyDocumentGraphic(zip(string(before), false), zip(string(after), false), spec); e != nil {
				t.Fatal(e)
			}
			effect := regexp.MustCompile(`<wp:effectExtent[^>]+/>`).FindString(string(after))
			if effect == "" {
				t.Fatal("recorded effect absent")
			}
			nested := strings.Replace(string(after), effect, "", 1)
			nested = strings.Replace(nested, "</a:graphicData>", effect+"</a:graphicData>", 1)
			if verifyDocumentGraphic(zip(string(before), false), zip(nested, false), spec) == nil {
				t.Fatal("effect under wrong parent accepted")
			}
			if e = verifyDocumentGraphic(zip(string(before), true), zip(string(after), true), spec); e == nil {
				t.Fatal("derived defaults accepted with inherited theme")
			}
			for _, mutation := range [][2]string{{`<wp:effectExtent `, `<wp:effectExtent unexpected="1" `}, {`<wp:effectExtent `, `<wp:effectExtent xmlns:q="urn:wrong" q:extra="1" `}, {`l="3175"`, `l="3174"`}, {`l="6350"`, `l="6348"`}, {`cap="flat"`, `cap="sq"`}, {`cmpd="sng"`, `cmpd="dbl"`}, {`algn="ctr"`, `algn="in"`}, {`<a:headEnd type="none"/>`, `<a:headEnd type="triangle"/>`}, {`<a:tailEnd type="none"/>`, `<a:tailEnd type="none" w="sm"/>`}, {`<a:alpha val="100000"/>`, `<a:alpha val="50000"/>`}} {
				changed := strings.Replace(string(after), mutation[0], mutation[1], 1)
				if changed == string(after) {
					continue
				}
				if verifyDocumentGraphic(zip(string(before), false), zip(changed, false), spec) == nil {
					t.Fatal("unsafe derived normalization", mutation)
				}
			}
		})
	}
}

func TestDocumentGraphicOpaqueAlphaRejectsOtherTransforms(t *testing.T) {
	for _, xml := range []string{`<a:srgbClr xmlns:a="` + documentDrawingMainNS + `" val="2C3033"><a:alpha val="100000" extra="x"/></a:srgbClr>`, `<a:srgbClr xmlns:a="` + documentDrawingMainNS + `" val="2C3033"><a:alpha val="100000"/><a:alpha val="100000"/></a:srgbClr>`, `<a:srgbClr xmlns:a="` + documentDrawingMainNS + `" val="2C3033"><a:alpha val="100000">unexpected</a:alpha></a:srgbClr>`, `<a:srgbClr xmlns:a="` + documentDrawingMainNS + `" val="2C3033"><alpha val="100000"/></a:srgbClr>`} {
		node, err := parseArtifactXML([]byte(xml))
		if err != nil {
			t.Fatal(err)
		}
		if documentGraphicOpaqueAlpha(node) {
			t.Fatal("unknown alpha representation accepted", xml)
		}
	}
}
func TestDocumentGraphicDerivedRejectsCustomThemeBinding(t *testing.T) {
	resources := map[string]string{"binding:word/_rels/document.xml.rels:rCustom": `Type="` + artifactOfficeRelNS + `/theme" Target="custom/look.xml"`}
	if documentGraphicNoTheme(resources) {
		t.Fatal("custom theme binding ignored")
	}
}

func TestDocumentGraphicDerivedRejectsNegativeExtent(t *testing.T) {
	root, err := parseArtifactXML([]byte(`<w:drawing xmlns:w="` + artifactWordNS + `" xmlns:wp="` + documentDrawingNS + `"><wp:inline><wp:effectExtent l="0" t="0" r="-1" b="0"/></wp:inline></w:drawing>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := documentGraphicExactEffect(root, 0); ok {
		t.Fatal("negative margin accepted")
	}
}
