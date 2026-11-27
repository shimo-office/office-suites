package main

import (
	"os"
	"strings"
	"testing"
)

func TestDocumentGraphicNativeStyleDefaults(t *testing.T) {
	for _, name := range []string{"rotate", "preset"} {
		t.Run(name, func(t *testing.T) {
			before, err := os.ReadFile("testdata/phase3-graphic/native-style-" + name + "-before.xml")
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile("testdata/phase3-graphic/native-style-" + name + "-after.xml")
			if err != nil {
				t.Fatal(err)
			}
			s := documentGraphicSpec{Paragraph: 2, Drawing: 1}
			if name == "rotate" {
				angle := float64(90)
				s.RotationDegrees = &angle
			} else {
				s.Preset = 2
			}
			file := func(raw string) string { return artifactTestZip(t, map[string]string{"word/document.xml": raw}) }
			if err := verifyDocumentGraphic(file(string(before)), file(string(after)), s); err != nil {
				t.Fatal(err)
			}
			for _, replacement := range [][2]string{{`idx="0"`, `idx="2"`}, {`val="B1B7BE"`, `val="123456"`}, {`val="accent1"`, `val="accent2"`}, {`l="6350"`, `l="6351"`}, {`val="000000"`, `val="123456"`}, {`val="solid"`, `val="dash"`}, {`type="none"`, `type="triangle"`}} {
				bad := strings.Replace(string(after), replacement[0], replacement[1], 1)
				if bad == string(after) {
					continue
				}
				if err := verifyDocumentGraphic(file(string(before)), file(bad), s); err == nil {
					t.Fatalf("unrequested default change accepted: %s", replacement[1])
				}
			}
			if name == "rotate" {
				withText := func(raw []byte) string {
					return strings.Replace(string(raw), "<wps:bodyPr", `<wps:txbx><w:txbxContent><w:p><w:r><w:t>Inherited font</w:t></w:r></w:p></w:txbxContent></wps:txbx><wps:bodyPr`, 1)
				}
				if err := verifyDocumentGraphic(file(withText(before)), file(withText(after)), s); err == nil {
					t.Fatal("new font style accepted for existing text")
				}
				withTheme := func(raw []byte) string {
					return artifactTestZip(t, map[string]string{"word/document.xml": string(raw), "word/theme/theme1.xml": `<a:theme xmlns:a="` + documentDrawingMainNS + `"/>`})
				}
				if err := verifyDocumentGraphic(withTheme(before), withTheme(after), s); err == nil {
					t.Fatal("unresolved theme inheritance accepted")
				}
			}
		})
	}
}
