package main

import (
	"strconv"
	"strings"
	"testing"
)

func TestPhase3SlideTableHeaderFillPreservesImplicitBodyStyle(t *testing.T) {
	cell := func(text, props string) *artifactNode {
		n, err := parseArtifactXML([]byte(`<a:tc xmlns:a="` + artifactDrawingNS + `"><a:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></a:txBody>` + props + `</a:tc>`))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	fill := `<a:tcPr><a:solidFill><a:srgbClr val="0000FF"/></a:solidFill></a:tcPr>`
	before := phase3SlideTableGrid{Rows: [][]*artifactNode{{cell("Header", `<a:tcPr/>`)}, {cell("Body", "")}}, Widths: []int64{100}, Heights: []int64{40, 40}}
	spec := phase3SlideTableSpec{Operation: "style", HeaderFillColor: "#0000FF"}
	for _, tc := range []struct {
		name, headerProps, bodyText, bodyProps string
		valid                                  bool
	}{
		{"untouched-implicit-body", fill, "Body", "", true},
		{"missing-requested-header-fill", "", "Body", "", false},
		{"wrong-header-fill", strings.Replace(fill, "0000FF", "FF0000", 1), "Body", "", false},
		{"changed-body-text", fill, "Changed", "", false},
		{"changed-body-fill", fill, "Body", fill, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after := phase3SlideTableGrid{Rows: [][]*artifactNode{{cell("Header", tc.headerProps)}, {cell(tc.bodyText, tc.bodyProps)}}, Widths: before.Widths, Heights: before.Heights}
			if err := verifyPhase3SlideTableGrid(before, after, spec); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestPhase3SlideTableFillCreatesOptionalStyle(t *testing.T) {
	text := `<a:txBody><a:bodyPr/><a:p><a:r><a:t>Keep</a:t></a:r></a:p></a:txBody>`
	fill := `<a:tcPr><a:solidFill><a:srgbClr val="0000FF"/></a:solidFill></a:tcPr>`
	grid := func(body string) phase3SlideTableGrid {
		n, err := parseArtifactXML([]byte(`<a:tc xmlns:a="` + artifactDrawingNS + `" xmlns:z="urn:foreign">` + body + `</a:tc>`))
		if err != nil {
			t.Fatal(err)
		}
		return phase3SlideTableGrid{Rows: [][]*artifactNode{{n}}, Widths: []int64{100}, Heights: []int64{40}}
	}
	spec := phase3SlideTableSpec{Operation: "style", HeaderFillColor: "#0000FF"}
	before := grid(text)
	if err := verifyPhase3SlideTableStyle(before, grid(text+fill), spec); err != nil {
		t.Fatalf("adding optional header style failed: %v", err)
	}
	for _, bad := range []string{
		text + fill + `<a:tcPr/>`,
		text + `<a:tcPr/>` + fill,
		fill + text,
		text + strings.Replace(fill, `<a:tcPr>`, `<a:tcPr marL="12">`, 1),
		text + strings.ReplaceAll(fill, "a:tcPr", "z:tcPr"),
	} {
		if err := verifyPhase3SlideTableStyle(before, grid(bad), spec); err == nil {
			t.Fatalf("non-target, malformed or foreign styles accepted: %s", bad)
		}
	}
}

func TestPhase3SlideTableBorderNativeUnitConversion(t *testing.T) {
	grid := func(width int64) phase3SlideTableGrid {
		borders := ""
		for _, tag := range []string{"lnL", "lnR", "lnT", "lnB"} {
			borders += `<a:` + tag + ` w="` + strconv.FormatInt(width, 10) + `"><a:solidFill><a:srgbClr val="000000"/></a:solidFill></a:` + tag + `>`
		}
		n, e := parseArtifactXML([]byte(`<a:tc xmlns:a="` + artifactDrawingNS + `"><a:txBody><a:p><a:r><a:t>Keep</a:t></a:r></a:p></a:txBody><a:tcPr>` + borders + `</a:tcPr></a:tc>`))
		if e != nil {
			t.Fatal(e)
		}
		return phase3SlideTableGrid{Rows: [][]*artifactNode{{n}}, Widths: []int64{100}, Heights: []int64{50}}
	}
	s := phase3SlideTableSpec{BorderColor: "#000000", BorderWidth: 1}
	for _, width := range []int64{12698, 12699, 12700, 12701, 25400} {
		e := verifyPhase3SlideTableStyle(grid(12700), grid(width), s)
		if (e == nil) != (width == 12699 || width == 12700) {
			t.Fatalf("width %d: %v", width, e)
		}
	}
}

func TestPhase3SlideTableBorderKeepsNonTargetProperties(t *testing.T) {
	grid := func(color, extra string) phase3SlideTableGrid {
		borders := ""
		for _, tag := range []string{"lnL", "lnR", "lnT", "lnB"} {
			borders += `<a:` + tag + ` w="12700" cap="flat"><a:solidFill><a:srgbClr val="` + color + `"/></a:solidFill>` + extra + `</a:` + tag + `>`
		}
		n, e := parseArtifactXML([]byte(`<a:tc xmlns:a="` + artifactDrawingNS + `"><a:txBody><a:p><a:r><a:t>Keep</a:t></a:r></a:p></a:txBody><a:tcPr>` + borders + `</a:tcPr></a:tc>`))
		if e != nil {
			t.Fatal(e)
		}
		return phase3SlideTableGrid{Rows: [][]*artifactNode{{n}}, Widths: []int64{100}, Heights: []int64{50}}
	}
	b := grid("FFFFFF", `<a:prstDash val="solid"/><a:round/><a:headEnd type="none"/>`)
	s := phase3SlideTableSpec{BorderColor: "#000000", BorderWidth: 1}
	if e := verifyPhase3SlideTableStyle(b, grid("000000", `<a:prstDash val="solid"/><a:round/><a:headEnd type="none"/>`), s); e != nil {
		t.Fatal(e)
	}
	for _, extra := range []string{`<a:prstDash val="dash"/><a:round/><a:headEnd type="none"/>`, `<a:prstDash val="solid"/><a:bevel/><a:headEnd type="none"/>`, `<a:prstDash val="solid"/><a:round/><a:headEnd type="triangle"/>`} {
		if e := verifyPhase3SlideTableStyle(b, grid("000000", extra), s); e == nil {
			t.Fatal("border non-target style mutation accepted")
		}
	}
}
