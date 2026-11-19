package main

import (
	"archive/zip"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func releaseArtifactExample(t *testing.T, kind string) map[string]string {
	t.Helper()
	z, err := zip.OpenReader("examples/接口验证." + kind)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	out := map[string]string{}
	for _, f := range z.File {
		raw, err := zipEntryBytes(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(raw)
	}
	return out
}
func releaseArtifactWorksheet(body string) string {
	return `<worksheet xmlns="` + artifactSheetNS + `">` + body + `</worksheet>`
}
func TestReleaseArtifactWorkbookRejectsMalformedRecords(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"zero row", `<sheetData><row r="0"/></sheetData>`, "行号"},
		{"duplicate rows", `<sheetData><row r="1"/><row r="1"/></sheetData>`, "行号"},
		{"duplicate cells", `<sheetData><row r="1"><c r="A1"/><c r="A1"/></row></sheetData>`, "重复单元格"},
		{"negative style", `<sheetData><row r="1"><c r="A1" s="-1"/></row></sheetData>`, "样式索引"},
		{"malformed style", `<sheetData><row r="1"><c r="A1" s="bold"/></row></sheetData>`, "样式索引"},
		{"shared formula without body", `<sheetData><row r="1"><c r="A1"><f t="shared" si="0"/><v>12</v></c></row></sheetData>`, "共享公式"},
		{"missing inline value", `<sheetData><row r="1"><c r="A1" t="inlineStr"/></row></sheetData>`, "缺少 is"},
		{"invalid boolean", `<sheetData><row r="1"><c r="A1" t="b"><v>true</v></c></row></sheetData>`, "布尔"},
		{"invalid number", `<sheetData><row r="1"><c r="A1"><v>NaN</v></c></row></sheetData>`, "数字"},
		{"unsupported type", `<sheetData><row r="1"><c r="A1" t="date"><v>today</v></c></row></sheetData>`, "不支持的单元格类型"},
		{"single-cell merge", `<sheetData/><mergeCells><mergeCell ref="A1"/></mergeCells>`, "合并范围"},
		{"cross-sheet merge", `<sheetData/><mergeCells><mergeCell ref="Other!A1:B1"/></mergeCells>`, "合并范围"},
		{"duplicate merge", `<sheetData/><mergeCells><mergeCell ref="A1:B1"/><mergeCell ref="A1:B1"/></mergeCells>`, "重复合并"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := artifactWorkbookEntries(releaseArtifactWorksheet(tc.body))
			_, err := verifyArtifact(artifactTestZip(t, entries), "xlsx", nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s rejection got %v", tc.want, err)
			}
		})
	}
	for _, tc := range []struct {
		name string
		edit func(map[string]string)
		want string
	}{
		{"missing workbook", func(e map[string]string) { delete(e, "xl/workbook.xml") }, "workbook.xml"},
		{"missing relationships", func(e map[string]string) { delete(e, "xl/_rels/workbook.xml.rels") }, "workbook.xml.rels"},
		{"missing styles", func(e map[string]string) { delete(e, "xl/styles.xml") }, "styles.xml"},
		{"corrupt shared strings", func(e map[string]string) { e["xl/sharedStrings.xml"] = "<broken>" }, "sharedStrings.xml"},
		{"empty workbook", func(e map[string]string) {
			e["xl/workbook.xml"] = `<workbook xmlns="` + artifactSheetNS + `"><sheets/></workbook>`
		}, "没有工作表"},
		{"unnamed sheet", func(e map[string]string) {
			e["xl/workbook.xml"] = strings.Replace(e["xl/workbook.xml"], `name="First"`, `name=""`, 1)
		}, "没有名称"},
		{"duplicate sheet", func(e map[string]string) {
			e["xl/workbook.xml"] = strings.Replace(e["xl/workbook.xml"], `name="First"`, `name="Target"`, 1)
		}, "重复工作表"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := artifactWorkbookEntries(releaseArtifactWorksheet(`<sheetData/>`))
			tc.edit(entries)
			p := artifactTestZip(t, entries)
			_, err := readArtifactSheet(p, "Target")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s rejection got %v", tc.want, err)
			}
		})
	}
	p := artifactTestZip(t, artifactWorkbookEntries(releaseArtifactWorksheet(`<sheetData><row r="1"><c r="A1" t="str"><v>plain</v></c><c r="B1" t="b"><v>0</v></c><c r="C1" t="e"><v>#DIV/0!</v></c></row></sheetData>`)))
	s, err := readArtifactSheet(p, "Target")
	if err != nil {
		t.Fatal(err)
	}
	if s.Cells["A1"].Value != "plain" || s.Cells["B1"].Value != "false" || s.Cells["C1"].Type != "error" {
		t.Fatalf("lost typed values: %+v", s.Cells)
	}
	if _, err := readArtifactSheet(p, "Missing"); err == nil {
		t.Fatal("nonexistent target accepted")
	}
	if _, err := readArtifactSheet(filepath.Join(t.TempDir(), "missing.xlsx"), "Target"); err == nil {
		t.Fatal("missing file accepted")
	}
}
func TestReleaseArtifactWorkbookAssertionBoundaries(t *testing.T) {
	entries := artifactWorkbookEntries(releaseArtifactWorksheet(`<sheetData><row r="1"><c r="A1"><v>12</v></c><c r="B1" t="b"><v>0</v></c></row></sheetData>`))
	p := artifactTestZip(t, entries)
	for _, tc := range []struct {
		name      string
		want      artifactExpectation
		errorText string
	}{
		{"conflicting sheet", artifactExpectation{Range: "Target!A1", SheetName: "First", Values: [][]any{{12}}}, "名称与 Range"},
		{"missing value range", artifactExpectation{SheetName: "Target", Values: [][]any{{12}}}, "有效 Range"},
		{"too many rows", artifactExpectation{Range: "Target!A1", Values: [][]any{{12}, {13}}}, "请求行数"},
		{"too many columns", artifactExpectation{Range: "Target!A1", Values: [][]any{{12, 13}}}, "请求列数"},
		{"empty row", artifactExpectation{Range: "Target!A1", Values: [][]any{{}}}, "请求列数"},
		{"missing style range", artifactExpectation{SheetName: "Target", Attrs: map[string]any{"bold": true}}, "有效 Range"},
		{"enormous style range", artifactExpectation{Range: "Target!A1:XFD1048576", Attrs: map[string]any{"bold": true}}, "100000"},
		{"missing styled cell", artifactExpectation{Range: "Target!C1", Attrs: map[string]any{"bold": false}}, "不存在"},
		{"unsupported attribute", artifactExpectation{Range: "Target!A1", Attrs: map[string]any{"rotation": 90}}, "不支持校验"},
		{"wrong attribute type", artifactExpectation{Range: "Target!A1", Attrs: map[string]any{"bold": "false"}}, "必须是 bool"},
		{"invalid merge range", artifactExpectation{SheetName: "Target", MergeRef: "A1"}, "合并校验范围"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := verifyArtifactExpected(p, "xlsx", tc.want)
			if err == nil || !strings.Contains(err.Error(), tc.errorText) {
				t.Fatalf("want %s rejection got %v", tc.errorText, err)
			}
		})
	}
	for _, value := range []any{float32(12), int64(12), json.Number("12"), 12.0, 12} {
		if _, err := verifyArtifactExpected(p, "xlsx", artifactExpectation{Range: "Target!A1", Values: [][]any{{value}}}); err != nil {
			t.Fatalf("literal numeric type %T rejected: %v", value, err)
		}
	}
	if _, err := verifyArtifactExpected(p, "xlsx", artifactExpectation{Range: "Target!B1:C1", Values: [][]any{{false, nil}}}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{map[string]any{"x": 12}, "12", true} {
		if artifactScalarEqual(xlsxCell{Type: "number", Value: "12"}, value) {
			t.Fatal("unrelated scalar coerced", value)
		}
	}
	for _, value := range []any{float32(12), int64(12), int(12)} {
		if !artifactScalarEqualWithNumericText(xlsxCell{Type: "string", Value: "12"}, value, true) {
			t.Fatal("explicit exporter compatibility lost", value)
		}
	}
	if artifactScalarEqualWithNumericText(xlsxCell{Type: "string", Value: "NaN"}, float32(math.NaN()), true) {
		t.Fatal("nonfinite numeric text accepted")
	}
	for _, v := range []struct {
		value any
		want  string
	}{{nil, ""}, {"x", "x"}, {json.Number("1.20"), "1.20"}, {float32(1.5), "1.5"}, {int64(12), "12"}, {12, "12"}, {false, "false"}, {[]int{1, 2}, "[1 2]"}} {
		if got := canonicalScalar(v.value); got != v.want {
			t.Fatalf("%T got %q want %q", v.value, got, v.want)
		}
	}
	for _, bad := range []string{"'Broken!A1", "Broken'!A1", "''!A1"} {
		if _, _, _, ok := parseCellRange(bad); ok {
			t.Fatal("malformed sheet quote accepted")
		}
	}
	if colName(0) != "" || colName(16385) != "" {
		t.Fatal("out-of-range column accepted")
	}
}
func TestReleaseArtifactStylesResolveEffectiveReferences(t *testing.T) {
	baseline := artifactStyles
	for _, tc := range []struct{ name, old, new, want string }{
		{"missing fonts", `<fonts count="2"><font><color rgb="FF000000"/></font><font><b/><color rgb="FFFF0000"/></font></fonts>`, "", "缺少 fonts"},
		{"bad font reference", `fontId="0"`, `fontId="bad"`, "样式索引"},
		{"bad fill reference", `fillId="0"`, `fillId="99"`, "fills 索引"},
		{"bad border reference", `borderId="0"`, `borderId="99"`, "borders 索引"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.ReplaceAll(baseline, tc.old, tc.new)
			n, err := parseArtifactXML([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			_, err = artifactStyleKey(n, 0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q got %v", tc.want, err)
			}
		})
	}
	for _, apply := range []string{"applyFont", "applyFill", "applyNumberFormat"} {
		for _, base := range []string{"bad", "99"} {
			t.Run(apply+base, func(t *testing.T) {
				raw := strings.Replace(baseline, `<cellXfs count="2"><xf `, `<cellXfs count="2"><xf `+apply+`="0" xfId="`+base+`" `, 1)
				n, err := parseArtifactXML([]byte(raw))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := artifactStyleKey(n, 0); err == nil {
					t.Fatal("invalid inherited style accepted")
				}
			})
		}
	}
	for _, raw := range []string{
		strings.Replace(artifactNumberFormatFixture("164", "0.0"), `numFmtId="164" formatCode="0.0"`, `numFmtId="bad" formatCode="0.0"`, 1),
		strings.Replace(artifactNumberFormatFixture("164", "0.0"), `</numFmts>`, `<numFmt numFmtId="164" formatCode="0%"/></numFmts>`, 1),
		strings.Replace(artifactNumberFormatFixture("164", "0.0"), `<xf numFmtId="164"`, `<xf numFmtId="bad"`, 1),
	} {
		n, err := parseArtifactXML([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := artifactStyleKey(n, 0); err == nil {
			t.Fatal("malformed number formats accepted")
		}
	}
	for _, tc := range []struct {
		raw   string
		value any
	}{
		{`<color rgb="FF000000" theme="1"/>`, "#000000"}, {`<color rgb="FF000000" indexed="1"/>`, "#000000"}, {`<color rgb="FF000000" tint="0.5"/>`, "#000000"}, {`<color rgb="FF000000"/>`, 12}, {`<color rgb="FF000000"/>`, "#000"}, {`<color rgb="FF000000"/>`, "#ZZZZZZ"},
	} {
		n, err := parseArtifactXML([]byte(tc.raw))
		if err != nil {
			t.Fatal(err)
		}
		if artifactColorMatches(n, tc.value) {
			t.Fatal("ambiguous color accepted")
		}
	}
	n, err := parseArtifactXML([]byte(artifactNumberFormatFixture("14", "")))
	if err != nil {
		t.Fatal(err)
	}
	xf, _ := artifactIndexedNode(n, "cellXfs", "xf", 0)
	if got, err := artifactNumberFormatKey(n, xf); err != nil || got != "builtin:14" {
		t.Fatalf("locale builtin guessed: %q %v", got, err)
	}
	if _, err := artifactCellAttr(n, 99, "bold", false); err == nil {
		t.Fatal("out-of-range cell style accepted")
	}
	for _, attr := range []string{"background", "color"} {
		broken, _ := parseArtifactXML([]byte(`<styleSheet xmlns="` + artifactSheetNS + `"><cellXfs><xf fillId="99" fontId="99"/></cellXfs></styleSheet>`))
		if _, err := artifactCellAttr(broken, 0, attr, "#000000"); err == nil {
			t.Fatal("missing component accepted")
		}
	}
	// Local alignment remains in the fingerprint even when all component IDs match.
	aligned := strings.Replace(artifactNumberFormatFixture("14", ""), `borderId="0"/>`, `borderId="0"><alignment horizontal="center"/></xf>`, 1)
	a, err := parseArtifactXML([]byte(aligned))
	if err != nil {
		t.Fatal(err)
	}
	ak, err := artifactStyleKey(a, 0)
	if err != nil {
		t.Fatal(err)
	}
	nk, err := artifactStyleKey(n, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ak == nk {
		t.Fatal("alignment disappeared from fingerprint")
	}
}
func TestReleaseArtifactPackageStructureRejectsBrokenLinks(t *testing.T) {
	for _, kind := range []string{"docx", "pptx"} {
		source := releaseArtifactExample(t, kind)
		for _, tc := range []struct {
			name string
			edit func(map[string]string)
			want string
		}{
			{"wrong content type", func(e map[string]string) { e["[Content_Types].xml"] = `<Types xmlns="` + artifactContentNS + `"/>` }, "缺少主文档声明"},
			{"no main relation", func(e map[string]string) { e["_rels/.rels"] = `<Relationships xmlns="` + artifactRelNS + `"/>` }, "包关系没有指向"},
			{"missing id", func(e map[string]string) {
				e["_rels/.rels"] = `<Relationships xmlns="` + artifactRelNS + `"><Relationship Target="foo"/></Relationships>`
			}, "缺少 Id"},
			{"duplicate id", func(e map[string]string) {
				e["_rels/.rels"] = `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="r1" Target="foo"/><Relationship Id="r1" Target="bar"/></Relationships>`
			}, "重复关系"},
			{"query path", func(e map[string]string) {
				e["_rels/.rels"] = `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="r1" Target="foo?token=1"/></Relationships>`
			}, "无效关系路径"},
			{"escaping path", func(e map[string]string) {
				e["_rels/.rels"] = `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="r1" Target="../foo"/></Relationships>`
			}, "包外"},
		} {
			t.Run(kind+tc.name, func(t *testing.T) {
				e := map[string]string{}
				for k, v := range source {
					e[k] = v
				}
				tc.edit(e)
				_, err := verifyArtifact(artifactTestZip(t, e), kind, nil)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("want %q got %v", tc.want, err)
				}
			})
		}
	}
	for _, tc := range []struct{ kind, part, body, want string }{
		{"docx", "word/document.xml", `<w:document xmlns:w="` + artifactWordNS + `"/>`, "缺少正文"},
		{"docx", "word/styles.xml", `<bad/>`, "根节点必须"},
		{"docx", "word/document.xml", `<bad/>`, "根节点必须"},
		{"pptx", "ppt/presentation.xml", `<p:presentation xmlns:p="` + artifactSlideNS + `"/>`, "没有幻灯片"},
		{"pptx", "ppt/presentation.xml", `<bad/>`, "根节点必须"},
		{"pptx", "ppt/_rels/presentation.xml.rels", `<bad/>`, "根节点必须"},
		{"pptx", "ppt/slides/slide1.xml", `<p:sld xmlns:p="` + artifactSlideNS + `"/>`, "缺少 cSld"},
		{"pptx", "ppt/theme/theme1.xml", `<a:theme xmlns:a="` + artifactDrawingNS + `"/>`, "缺少 themeElements"},
		{"pptx", "ppt/theme/theme1.xml", `<bad/>`, "根节点必须"},
	} {
		t.Run(tc.kind+tc.part+tc.want, func(t *testing.T) {
			e := releaseArtifactExample(t, tc.kind)
			e[tc.part] = tc.body
			_, err := verifyArtifact(artifactTestZip(t, e), tc.kind, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q got %v", tc.want, err)
			}
		})
	}
	ppt := releaseArtifactExample(t, "pptx")
	for k := range ppt {
		if strings.HasPrefix(k, "ppt/theme/") {
			delete(ppt, k)
		}
	}
	if _, err := verifyArtifact(artifactTestZip(t, ppt), "pptx", nil); err == nil {
		t.Fatal("missing theme accepted")
	}
	ppt = releaseArtifactExample(t, "pptx")
	ppt["ppt/_rels/presentation.xml.rels"] = `<Relationships xmlns="` + artifactRelNS + `"/>`
	if _, err := verifyArtifact(artifactTestZip(t, ppt), "pptx", nil); err == nil {
		t.Fatal("unbound slide accepted")
	}
	for _, kind := range []string{"doc", "sheet", "slide"} {
		ext := map[string]string{"doc": "docx", "sheet": "xlsx", "slide": "pptx"}[kind]
		ev, err := verifyArtifact("examples/接口验证."+ext, kind, nil)
		if err != nil || ev.Kind != ext {
			t.Fatalf("alias %s failed %v", kind, err)
		}
	}
	if _, err := verifyArtifact("examples/接口验证.docx", "csv", nil); err == nil {
		t.Fatal("unsupported kind accepted")
	}
	if _, err := verifyArtifactExpected("examples/接口验证.docx", "docx", artifactExpectation{Range: "A1"}); err == nil {
		t.Fatal("spreadsheet assertion applied to docx")
	}
}
func TestReleaseArtifactXMLAndZipSafetyLimits(t *testing.T) {
	for _, raw := range []string{`<a x="1" x="2"/>`, `<!DOCTYPE a><a/>`, strings.Repeat("<a>", 258) + strings.Repeat("</a>", 258)} {
		if _, err := parseArtifactXML([]byte(raw)); err == nil {
			t.Fatal("unsafe XML accepted")
		}
	}
	for _, tc := range []struct {
		name    string
		headers []zip.FileHeader
		want    string
	}{
		{"duplicate entries", []zip.FileHeader{{Name: "data.xml"}, {Name: "data.xml"}}, "重复压缩包"},
		{"oversized XML metadata", []zip.FileHeader{{Name: "data.xml", UncompressedSize64: artifactMaxPart + 1}}, "XML 条目过大"},
		{"oversized package metadata", []zip.FileHeader{{Name: "data.bin", UncompressedSize64: 257 << 20}}, "256 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "hostile.zip")
			f, err := os.Create(p)
			if err != nil {
				t.Fatal(err)
			}
			w := zip.NewWriter(f)
			for _, h := range tc.headers {
				if _, err := w.CreateRaw(&h); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			z, _, err := openArtifact(p)
			if z != nil {
				z.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q got %v", tc.want, err)
			}
		})
	}
	p := artifactTestZip(t, map[string]string{"directory/": "", "data.xml": "<a/>"})
	z, e, err := openArtifact(p)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(e) != 1 {
		t.Fatal("directory counted as evidence")
	}
	oversized := &zip.File{FileHeader: zip.FileHeader{UncompressedSize64: artifactMaxPart + 1}}
	if _, err := zipEntryBytes(oversized); err == nil {
		t.Fatal("oversized entry read")
	}
}
