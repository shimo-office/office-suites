package main

import (
	"strconv"
	"testing"
)

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
