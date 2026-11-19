package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseFileXlsxInlineAndExistingValues(t *testing.T) {
	raw := releaseFileWorkbook(t, `<row r="1"><c r="A1" t="inlineStr"><is><r><t>中文</t></r><r><t>😀</t></r></is></c><c r="B1"><v>42</v></c><c r="C1"><f>B1*2</f><v>84</v></c></row>`)
	p := filepath.Join(t.TempDir(), "inline.xlsx")
	if err := os.WriteFile(p, raw, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := extractXlsx(p)
	if err != nil || !strings.Contains(got, "中文😀\t42\t84") {
		t.Fatalf("inline/rich text/numeric/formula lost: %q %v", got, err)
	}
	p = artifactTestZip(t, releaseFileWorkbookEntries(map[string]string{"xl/sharedStrings.xml": `<sst><si><t>共享中文</t></si></sst>`, "xl/worksheets/sheet1.xml": `<worksheet><sheetData><row><c t="s"><v>0</v></c></row></sheetData></worksheet>`}, "xl/worksheets/sheet1.xml"))
	got, err = extractXlsx(p)
	if err != nil || !strings.Contains(got, "共享中文") {
		t.Fatalf("shared string changed: %q %v", got, err)
	}
}

func TestReleaseFileXlsxRejectsMalformedCellXML(t *testing.T) {
	for _, xml := range []string{`<row><c><v>123</wrong></c></row>`, `<row><c t="inlineStr"><is><t>部分中文</wrong></is></c></row>`} {
		raw := releaseFileWorkbook(t, xml)
		p := filepath.Join(t.TempDir(), "bad.xlsx")
		if err := os.WriteFile(p, raw, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := extractXlsx(p)
		if err == nil || got != "" {
			t.Errorf("malformed XML reported extracted text: %q %v", got, err)
		}
	}
}

func releaseFileWorkbook(t *testing.T, rows string) []byte {
	t.Helper()
	p := artifactTestZip(t, releaseFileWorkbookEntries(map[string]string{"xl/worksheets/sheet1.xml": `<worksheet><sheetData>` + rows + `</sheetData></worksheet>`}, "xl/worksheets/sheet1.xml"))
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReleaseFileXlsxUsesWorkbookRelationships(t *testing.T) {
	for _, mode := range []string{"custom", "wrong-type", "missing-target", "external"} {
		t.Run(mode, func(t *testing.T) {
			entries := artifactWorkbookEntries(`<worksheet xmlns="` + artifactNS + `"><sheetData><row><c t="inlineStr"><is><t>关系中文</t></is></c></row></sheetData></worksheet>`)
			entries["xl/workbook.xml"] = `<workbook xmlns="` + artifactNS + `" xmlns:r="` + artifactOfficeRelNS + `"><sheets><sheet name="Actual" sheetId="2" r:id="s2"/></sheets></workbook>`
			entries["xl/worksheets/sheet1.xml"] = `<worksheet><row><c><v>999999</v></c></row></worksheet>`
			if mode == "wrong-type" {
				entries["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(entries["xl/_rels/workbook.xml.rels"], "/worksheet", "/styles")
			}
			if mode == "missing-target" {
				delete(entries, "xl/worksheets/custom.xml")
			}
			if mode == "external" {
				entries["xl/_rels/workbook.xml.rels"] = `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="s2" Type="` + artifactOfficeRelNS + `/worksheet" Target="https://external.invalid/worksheet.xml" TargetMode="External"/></Relationships>`
			}
			got, err := extractXlsx(artifactTestZip(t, entries))
			if mode != "custom" {
				if err == nil || got != "" {
					t.Fatalf("invalid relationship read as success: %q %v", got, err)
				}
				return
			}
			if err != nil || got != "## Sheet 1\n关系中文" {
				t.Fatalf("relationship target/order ignored: %q %v", got, err)
			}
		})
	}
}

func releaseFileWorkbookEntries(content map[string]string, order ...string) map[string]string {
	out := make(map[string]string, len(content)+2)
	for name, body := range content {
		out[name] = strings.Replace(body, "<worksheet>", `<worksheet xmlns="`+artifactSheetNS+`">`, 1)
	}
	var sheets, rels strings.Builder
	for i, name := range order {
		id := fmt.Sprintf("r%d", i+1)
		fmt.Fprintf(&sheets, `<sheet name="Sheet%d" sheetId="%d" r:id="%s"/>`, i+1, i+1, id)
		fmt.Fprintf(&rels, `<Relationship Id="%s" Type="%s/worksheet" Target="%s"/>`, id, artifactOfficeRelNS, strings.TrimPrefix(name, "xl/"))
	}
	out["xl/workbook.xml"] = `<workbook xmlns="` + artifactSheetNS + `" xmlns:r="` + artifactOfficeRelNS + `"><sheets>` + sheets.String() + `</sheets></workbook>`
	out["xl/_rels/workbook.xml.rels"] = `<Relationships xmlns="` + artifactRelNS + `">` + rels.String() + `</Relationships>`
	return out
}

func TestReleaseFileXlsxStrictAndChartsheetCompatibility(t *testing.T) {
	for _, strict := range []bool{false, true} {
		for _, mode := range []string{"worksheet", "chartsheet", "chart-missing", "chart-external", "chart-wrong-root", "unknown-namespace", "unknown-relation"} {
			t.Run(fmt.Sprintf("strict=%t/%s", strict, mode), func(t *testing.T) {
				entries := releaseFileWorkbookEntries(map[string]string{"xl/worksheets/custom.xml": `<worksheet><sheetData><row><c t="inlineStr"><is><t>正文中文</t></is></c><c><v>42</v></c></row></sheetData></worksheet>`}, "xl/worksheets/custom.xml")
				if mode == "chartsheet" || strings.HasPrefix(mode, "chart-") {
					entries["xl/workbook.xml"] = strings.Replace(entries["xl/workbook.xml"], "<sheets>", `<sheets><sheet name="Chart1" sheetId="2" r:id="chart1"/>`, 1)
					rel := `<Relationship Id="chart1" Type="` + artifactOfficeRelNS + `/chartsheet" Target="chartsheets/chart.xml"/>`
					if mode == "chart-external" {
						rel = strings.Replace(rel, `Target=`, `TargetMode="External" Target=`, 1)
					}
					entries["xl/_rels/workbook.xml.rels"] = strings.Replace(entries["xl/_rels/workbook.xml.rels"], "</Relationships>", rel+"</Relationships>", 1)
					entries["xl/chartsheets/chart.xml"] = `<chartsheet xmlns="` + artifactSheetNS + `"><sheetViews><sheetView workbookViewId="0"/></sheetViews></chartsheet>`
					if mode == "chart-missing" {
						delete(entries, "xl/chartsheets/chart.xml")
					}
					if mode == "chart-wrong-root" {
						entries["xl/chartsheets/chart.xml"] = `<worksheet xmlns="` + artifactSheetNS + `"><sheetData><row><c><v>999</v></c></row></sheetData></worksheet>`
					}
				}
				if strict {
					for name, body := range entries {
						body = strings.ReplaceAll(body, artifactSheetNS, "http://purl.oclc.org/ooxml/spreadsheetml/main")
						entries[name] = strings.ReplaceAll(body, artifactOfficeRelNS, "http://purl.oclc.org/ooxml/officeDocument/relationships")
					}
				}
				if mode == "unknown-namespace" {
					entries["xl/workbook.xml"] = strings.Replace(entries["xl/workbook.xml"], `xmlns="`, `xmlns="urn:unknown:`, 1)
				}
				if mode == "unknown-relation" {
					entries["xl/_rels/workbook.xml.rels"] = strings.ReplaceAll(entries["xl/_rels/workbook.xml.rels"], "/worksheet", "/unknown")
				}
				got, err := extractXlsx(artifactTestZip(t, entries))
				if mode != "worksheet" && mode != "chartsheet" {
					if err == nil || got != "" {
						t.Fatalf("invalid workbook accepted: %q %v", got, err)
					}
					return
				}
				if err != nil || got != "## Sheet 1\n正文中文\t42" {
					t.Fatalf("valid workbook unreadable: %q %v", got, err)
				}
			})
		}
	}
}
