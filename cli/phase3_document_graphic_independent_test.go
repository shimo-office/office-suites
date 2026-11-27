package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Captured synthetic DOCX XML: inline-to-anchor image edit and native
// rightArrow preset. Only the selected drawing may materialize proven defaults.
func TestDocumentGraphicRecordedIndependentEdits(t *testing.T) {
	for _, kind := range []string{"floating-image", "arrow-preset"} {
		t.Run(kind, func(t *testing.T) {
			before, err := os.ReadFile("testdata/phase3-graphic/" + kind + "-before.xml")
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile("testdata/phase3-graphic/" + kind + "-after.xml")
			if err != nil {
				t.Fatal(err)
			}
			payload := map[string]any{"paragraph": float64(2), "expectedParagraph": "", "drawing": float64(1), "widthCm": float64(6), "heightCm": float64(4), "rotationDegrees": float64(90), "wrap": "square", "anchor": "paragraph", "horizontalAlign": "center", "horizontalRelativeTo": "margin"}
			if kind == "arrow-preset" {
				payload = map[string]any{"paragraph": float64(3), "expectedParagraph": "", "drawing": float64(1), "preset": float64(2)}
			}
			s, err := prepareDocumentGraphic(payload)
			if err != nil {
				t.Fatal(err)
			}
			zip := func(raw string) string { return artifactTestZip(t, map[string]string{"word/document.xml": raw}) }
			b, a := zip(string(before)), zip(string(after))
			if err := verifyDocumentGraphic(b, a, s); err != nil {
				t.Fatal(err)
			}
			for _, mutation := range [][2]string{
				{`l="3175"`, `l="3176"`}, {`l="6350"`, `l="6351"`},
				{`fmla="val 50000"`, `fmla="val 45000"`},
				{`cap="flat"`, `cap="sq"`}, {`<a:headEnd type="none"/>`, `<a:headEnd type="triangle"/>`},
				{`locked="0"`, `locked="1"`}, {`relativeFrom="margin"`, `relativeFrom="page"`},
				{`PHASE3_GRAPHICS_20261118`, `changed`}, {`NATIVE_SHAPES_UNTOUCHED_PARAGRAPH`, `changed`},
			} {
				bad := strings.Replace(string(after), mutation[0], mutation[1], 1)
				if bad == string(after) {
					continue
				}
				if verifyDocumentGraphic(b, zip(bad), s) == nil {
					t.Fatalf("accepted unrelated/default change: %v", mutation)
				}
			}
		})
	}
}

func TestDocumentGraphicDefaultArrowGeometry(t *testing.T) {
	defaultGuides := `<a:gd name="adj1" fmla="val 50000"/><a:gd name="adj2" fmla="val 50000"/>`
	for _, tc := range []struct {
		name, guides string
		valid        bool
	}{
		{"implicit", "", true}, {"explicit", defaultGuides, true},
		{"custom", strings.Replace(defaultGuides, "50000", "45000", 1), false},
		{"duplicate", strings.Replace(defaultGuides, "adj2", "adj1", 1), false},
		{"partial", `<a:gd name="adj1" fmla="val 50000"/>`, false},
		{"foreign", strings.Replace(defaultGuides, `name="adj1"`, `q:name="adj1"`, 1), false},
		{"extra", strings.Replace(defaultGuides, `name="adj1"`, `name="adj1" extra="x"`, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n, err := parseArtifactXML([]byte(`<root xmlns:a="` + documentDrawingMainNS + `" xmlns:q="urn:foreign"><a:prstGeom prst="rightArrow"><a:avLst>` + tc.guides + `</a:avLst></a:prstGeom></root>`))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := documentGraphicDefaultGeometry(n); ok != tc.valid {
				t.Fatalf("valid=%v want=%v", ok, tc.valid)
			}
		})
	}
}

// No editor or network calls: recheck a submitted run's saved files without
// repeating its write or changing the original failure record.
func TestDocumentGraphicOfflineEvidenceReplay(t *testing.T) {
	dir := os.Getenv("SHIMO_DOCUMENT_GRAPHIC_REPLAY_DIR")
	if dir == "" {
		t.Skip("opt-in offline submitted artifact replay")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Spec documentGraphicSpec `json:"spec"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	file := func(stage string) string {
		t.Helper()
		matches, err := filepath.Glob(filepath.Join(dir, stage, "*.docx"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("stage=%s files=%v error=%v", stage, matches, err)
		}
		return matches[0]
	}
	for _, stage := range []string{"after", "reload"} {
		if err := verifyDocumentGraphic(file("before"), file(stage), request.Spec); err != nil {
			t.Fatalf("%s: %v", stage, err)
		}
		t.Log("verified saved", stage, "without writes")
	}
}

func TestDocumentGraphicCommandProofRejectsCollateral(t *testing.T) {
	before, err := os.ReadFile("testdata/phase3-graphic/image-before.xml")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile("testdata/phase3-graphic/image-after.xml")
	if err != nil {
		t.Fatal(err)
	}
	s, err := prepareDocumentGraphic(map[string]any{"paragraph": float64(2), "expectedParagraph": "", "drawing": float64(1), "widthCm": float64(6), "heightCm": float64(4), "rotationDegrees": float64(90)})
	if err != nil {
		t.Fatal(err)
	}
	zip := func(raw string) string { return artifactTestZip(t, map[string]string{"word/document.xml": raw}) }
	// Inserting bold into the untouched first paragraph changes no text, but
	// must not be accepted as a generic layout-reflow warning by the command.
	bad := strings.Replace(string(after), `<w:r>`, `<w:r><w:rPr><w:b/></w:rPr>`, 1)
	if bad == string(after) {
		t.Fatal("fixture missing run")
	}
	if _, err := verifyDocumentGraphicWithWarnings(zip(string(before)), zip(bad), s); err == nil {
		t.Fatal("command accepted an unrelated formatting change as a warning")
	}
}
