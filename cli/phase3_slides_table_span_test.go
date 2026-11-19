package main

import "testing"

func TestPhase3SlideTableMergeSplitProof(t *testing.T) {
	cell := func(attrs, text string) *artifactNode {
		n, e := parseArtifactXML([]byte(`<a:tc xmlns:a="` + artifactDrawingNS + `" ` + attrs + `><a:txBody><a:bodyPr/><a:lstStyle/>` + text + `</a:txBody><a:tcPr/></a:tc>`))
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	p := func(s string) string { return `<a:p><a:r><a:t>` + s + `</a:t></a:r></a:p>` }
	b := phase3SlideTableGrid{Rows: [][]*artifactNode{{cell("", p("A")), cell("", p("B"))}, {cell("", p("C")), cell("", p("D"))}}, Widths: []int64{100, 100}, Heights: []int64{60, 60}}
	a := phase3SlideTableGrid{Rows: [][]*artifactNode{{cell(`gridSpan="2"`, p("A")+p("B")), cell(`hMerge="1"`, `<a:p/>`)}, b.Rows[1]}, Widths: b.Widths, Heights: b.Heights}
	s := phase3SlideTableSpec{Operation: "merge", Row: 0, Column: 0}
	if e := verifyPhase3SlideTableGrid(b, a, s); e != nil {
		t.Fatal(e)
	}
	broken := a
	broken.Rows = [][]*artifactNode{{cell(`gridSpan="2"`, p("A")), cell(`hMerge="1"`, `<a:p/>`)}, b.Rows[1]}
	if e := verifyPhase3SlideTableGrid(b, broken, s); e == nil {
		t.Fatal("merged text loss accepted")
	}
	s.Operation = "split"
	split := phase3SlideTableGrid{Rows: [][]*artifactNode{{cell("", p("A")+p("B")), cell("", `<a:p/>`)}, b.Rows[1]}, Widths: b.Widths, Heights: b.Heights}
	if e := verifyPhase3SlideTableGrid(a, split, s); e != nil {
		t.Fatal(e)
	}
	if e := verifyPhase3SlideTableGrid(a, a, s); e == nil {
		t.Fatal("merge masquerading as split accepted")
	}
}

func TestPhase3SlideTableMergeRejectsWhitespaceLoss(t *testing.T) {
	cell := func(content string) *artifactNode {
		n, e := parseArtifactXML([]byte(`<a:tc xmlns:a="` + artifactDrawingNS + `"><a:txBody>` + content + `</a:txBody><a:tcPr/></a:tc>`))
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	full := cell(`<a:p><a:r><a:t>A</a:t></a:r></a:p>`)
	for _, raw := range []string{`<a:p><a:r><a:t> </a:t></a:r></a:p>`, `<a:p/><a:p><a:r><a:t>B</a:t></a:r></a:p>`, `<a:p><a:r><a:t>\n</a:t></a:r></a:p>`} {
		if raw == `<a:p><a:r><a:t>\n</a:t></a:r></a:p>` {
			raw = "<a:p><a:r><a:t>\n</a:t></a:r></a:p>"
		}
		g := phase3SlideTableGrid{Rows: [][]*artifactNode{{full, cell(raw)}}, Widths: []int64{100, 100}, Heights: []int64{60}}
		if e := validatePhase3SpanBaseline(g, phase3SlideTableSpec{Operation: "merge"}); e == nil {
			t.Fatalf("blank paragraph/whitespace accepted: %q", raw)
		}
	}
	if phase3EmptyCellPlaceholder(cell(`<a:p><a:r><a:t> </a:t></a:r></a:p>`)) {
		t.Fatal("whitespace run treated as empty placeholder")
	}
}

func TestPhase3SlideTableMergeNativeEmptyContinuation(t *testing.T) {
	cell := func(raw string) *artifactNode {
		n, e := parseArtifactXML([]byte(`<a:tbl xmlns:a="` + artifactDrawingNS + `"><a:tc ` + raw + `</a:tc></a:tbl>`))
		if e != nil {
			t.Fatal(e)
		}
		return n.Children[0]
	}
	body := func(text string) string {
		return `<a:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></a:txBody><a:tcPr marL="91440"/>`
	}
	b := phase3SlideTableGrid{Rows: [][]*artifactNode{{cell(`>` + body("A")), cell(`>` + body("B"))}}, Widths: []int64{100, 100}, Heights: []int64{60}}
	merged := cell(`gridSpan="2"><a:txBody><a:bodyPr/><a:p><a:r><a:t>A</a:t></a:r></a:p><a:p><a:r><a:t>B</a:t></a:r></a:p></a:txBody><a:tcPr marL="91440"/>`)
	a := phase3SlideTableGrid{Rows: [][]*artifactNode{{merged, cell(`hMerge="1">`)}}, Widths: b.Widths, Heights: b.Heights}
	s := phase3SlideTableSpec{Operation: "merge"}
	if e := verifyPhase3SlideTableGrid(b, a, s); e != nil {
		t.Fatal("native empty continuation rejected", e)
	}
	bad := b
	bad.Rows = [][]*artifactNode{{b.Rows[0][0], cell(`>` + body("B") + `<a:extLst/>`)}}
	if e := validatePhase3SpanBaseline(bad, s); e == nil {
		t.Fatal("differing cell properties accepted before native destructive merge")
	}
	for _, raw := range []string{`hMerge="1" surprise="1">`, `hMerge="1"><a:extLst/>`, `hMerge="1"> `} {
		bad = a
		bad.Rows = [][]*artifactNode{{merged, cell(raw)}}
		if e := verifyPhase3SlideTableGrid(b, bad, s); e == nil {
			t.Fatal("nonempty continuation metadata ignored", raw)
		}
	}
	s.Operation = "split"
	left := cell(`><a:txBody><a:bodyPr/><a:p><a:r><a:t>A</a:t></a:r></a:p><a:p><a:r><a:t>B</a:t></a:r></a:p></a:txBody><a:tcPr marL="91440"/>`)
	right := cell(`><a:txBody><a:bodyPr/><a:p/></a:txBody><a:tcPr marL="91440"/>`)
	split := phase3SlideTableGrid{Rows: [][]*artifactNode{{left, right}}, Widths: b.Widths, Heights: b.Heights}
	if e := verifyPhase3SlideTableGrid(a, split, s); e != nil {
		t.Fatal("split from native continuation rejected", e)
	}
}
