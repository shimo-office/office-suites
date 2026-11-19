package main

import "testing"

func TestPhase3SlideTableGraphicFrameUsesStandardTableURI(t *testing.T) {
	for _, uri := range []string{"http://schemas.openxmlformats.org/drawingml/2006/table", artifactDrawingNS + "/table", artifactDrawingNS + "/chart", "urn:foreign"} {
		n, err := parseArtifactXML([]byte(`<p:graphicFrame xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><a:graphic><a:graphicData uri="` + uri + `"><a:tbl/></a:graphicData></a:graphic></p:graphicFrame>`))
		if err != nil {
			t.Fatal(err)
		}
		_, err = phase3TableNode(n)
		if (err == nil) != (uri == "http://schemas.openxmlformats.org/drawingml/2006/table") {
			t.Fatalf("incorrect table URI recognition: %s: %v", uri, err)
		}
	}
}

func TestPhase3SlideTableSpec(t *testing.T) {
	s, e := preparePhase3Slides("slide-table-edit", map[string]any{"slideId": "a", "tableName": "Table 1", "operation": "insert-row", "index": float64(2)})
	if e != nil || s.Table == nil || s.Table.Index != 1 {
		t.Fatalf("%+v %v", s, e)
	}
	for _, op := range []string{"removeSpan", "split", "style", "unknown"} {
		if _, e := preparePhase3Slides("slide-table-edit", map[string]any{"slideId": "a", "tableName": "Table 1", "operation": op, "index": float64(1)}); e == nil {
			t.Fatalf("unproven operation %s accepted", op)
		}
	}
}
func TestPhase3SlideTableStructureProof(t *testing.T) {
	cell := func(v string) *artifactNode {
		n, e := parseArtifactXML([]byte(`<a:tc xmlns:a="` + artifactDrawingNS + `"><a:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>` + v + `</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc>`))
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	b := phase3SlideTableGrid{Rows: [][]*artifactNode{{cell("A"), cell("B")}, {cell("C"), cell("D")}}, Widths: []int64{100, 100}, Heights: []int64{40, 40}}
	s := phase3SlideTableSpec{Operation: "insert-row", Index: 1}
	a := phase3SlideTableGrid{Rows: [][]*artifactNode{b.Rows[0], {cell(""), cell("")}, b.Rows[1]}, Widths: b.Widths, Heights: []int64{40, 40, 40}}
	if e := verifyPhase3SlideTableGrid(b, a, s); e != nil {
		t.Fatal(e)
	}
	bad := a
	bad.Rows = [][]*artifactNode{b.Rows[0], {cell(""), cell("lost")}, b.Rows[1]}
	if e := verifyPhase3SlideTableGrid(b, bad, s); e == nil {
		t.Fatal("new row text accepted")
	}
	bad = a
	bad.Rows = [][]*artifactNode{b.Rows[1], {cell(""), cell("")}, b.Rows[0]}
	if e := verifyPhase3SlideTableGrid(b, bad, s); e == nil {
		t.Fatal("existing text reordering accepted")
	}
	s = phase3SlideTableSpec{Operation: "delete-column", Index: 1}
	a = phase3SlideTableGrid{Rows: [][]*artifactNode{{b.Rows[0][0]}, {b.Rows[1][0]}}, Widths: []int64{100}, Heights: b.Heights}
	if e := verifyPhase3SlideTableGrid(b, a, s); e != nil {
		t.Fatal(e)
	}
}

func TestPhase3SlideTableStyleProof(t *testing.T) {
	cell := func(fill, border string) *artifactNode {
		color := ""
		if fill != "" {
			color = `<a:solidFill><a:srgbClr val="` + fill + `"/></a:solidFill>`
		}
		lines := ""
		if border != "" {
			for _, tag := range []string{"lnL", "lnR", "lnT", "lnB"} {
				lines += `<a:` + tag + ` w="12700"><a:solidFill><a:srgbClr val="` + border + `"/></a:solidFill></a:` + tag + `>`
			}
		}
		n, e := parseArtifactXML([]byte(`<a:tc xmlns:a="` + artifactDrawingNS + `"><a:txBody><a:p><a:r><a:t>keep</a:t></a:r></a:p></a:txBody><a:tcPr>` + color + lines + `</a:tcPr></a:tc>`))
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	b := phase3SlideTableGrid{Rows: [][]*artifactNode{{cell("", "")}, {cell("", "")}}, Widths: []int64{100}, Heights: []int64{40, 40}}
	a := phase3SlideTableGrid{Rows: [][]*artifactNode{{cell("0000FF", "000000")}, {cell("", "000000")}}, Widths: b.Widths, Heights: b.Heights}
	s := phase3SlideTableSpec{Operation: "style", HeaderFillColor: "#0000FF", BorderColor: "#000000", BorderWidth: 1}
	if e := verifyPhase3SlideTableGrid(b, a, s); e != nil {
		t.Fatal(e)
	}
	a.Rows[1][0] = cell("0000FF", "000000")
	if e := verifyPhase3SlideTableGrid(b, a, s); e == nil {
		t.Fatal("nonheader fill accepted")
	}
}
