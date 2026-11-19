package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestPhase3SlideTableNativeEmptyCellDefaults(t *testing.T) {
	cell := func(body string) *artifactNode {
		n, err := parseArtifactXML([]byte(`<a:tc xmlns:a="` + artifactDrawingNS + `" xmlns:z="urn:foreign"><a:txBody>` + body + `</a:txBody><a:tcPr marL="91440"/></a:tc>`))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	run := `<a:r><a:rPr lang="zh-CN" sz="2400"/><a:t>R1C1</a:t></a:r>`
	before := cell(`<a:bodyPr/><a:lstStyle/><a:p>` + run + `</a:p>`)
	after := `<a:bodyPr/><a:p><a:pPr/>` + run + `<a:endParaRPr/></a:p>`
	if !phase3EqualTableCell(before, cell(after)) {
		t.Fatal("empty native optional cell styles differ")
	}
	for _, bad := range []string{
		strings.Replace(after, `<a:pPr/>`, `<a:pPr lvl="1"/>`, 1),
		strings.Replace(after, `<a:pPr/>`, `<a:pPr/><a:pPr/>`, 1),
		strings.Replace(after, `<a:pPr/>`, `<a:endParaRPr/><a:pPr/>`, 1),
		strings.Replace(after, `<a:endParaRPr/>`, `<a:endParaRPr sz="100"/>`, 1),
		strings.Replace(after, `<a:endParaRPr/>`, `<a:endParaRPr/><a:endParaRPr/>`, 1),
		strings.Replace(after, `<a:pPr/>`, `<z:pPr/>`, 1),
		strings.Replace(after, `R1C1`, `changed`, 1),
		strings.Replace(after, `<a:bodyPr/>`, `<a:bodyPr/><a:lstStyle><a:lvl1pPr algn="r"/></a:lstStyle>`, 1),
		strings.Replace(after, `</a:p>`, `</a:p><a:p/>`, 1),
	} {
		if phase3EqualTableCell(before, cell(bad)) {
			t.Fatal("nonempty, duplicate, reordered or foreign cell change accepted", bad)
		}
	}
}

func TestPhase3SlideTableNativeFrameDefaults(t *testing.T) {
	frame := func(xf, props string) any {
		n, err := parseArtifactXML([]byte(`<p:graphicFrame xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `" xmlns:z="urn:foreign"><p:xfrm ` + xf + `><a:off x="10" y="20"/><a:ext cx="100" cy="200"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl><a:tblPr firstRow="1" bandRow="0" ` + props + `/><a:tblGrid/><a:tr h="10"/></a:tbl></a:graphicData></a:graphic></p:graphicFrame>`))
		if err != nil {
			t.Fatal(err)
		}
		canonical, err := phase3SlideCanonical(phase3MaskTable(n), map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		return canonical
	}
	before := frame("", "")
	if !reflect.DeepEqual(before, frame(`rot="0" flipH="0" flipV="0"`, `rtl="0" firstCol="0" lastRow="0" lastCol="0" bandCol="0"`)) {
		t.Fatal("native identity transform/table boolean defaults differ")
	}
	for _, bad := range []any{frame(`rot="1"`, ""), frame(`flipH="1"`, ""), frame(`z:rot="0"`, ""), frame("", `firstCol="1"`), frame("", `z:rtl="0"`)} {
		if reflect.DeepEqual(before, bad) {
			t.Fatal("nondefault or foreign table frame attribute masked")
		}
	}
}
