package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func releaseTableCell(text, props, attrs string) string {
	body := `<a:p/>`
	if text != "" {
		body = `<a:p><a:r><a:t>` + text + `</a:t></a:r></a:p>`
	}
	return `<a:tc` + attrs + `><a:txBody><a:bodyPr/><a:lstStyle/>` + body + `</a:txBody><a:tcPr>` + props + `</a:tcPr></a:tc>`
}
func releaseTableFrame(rows [][]string) string {
	columns := len(rows[0])
	grid := strings.Repeat(`<a:gridCol w="1000000"/>`, columns)
	body := ""
	for _, row := range rows {
		body += `<a:tr h="400000">` + strings.Join(row, "") + `</a:tr>`
	}
	return `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="8" name="Table 1"/></p:nvGraphicFramePr><p:xfrm><a:off x="0" y="0"/><a:ext cx="` + fmt.Sprint(columns*1000000) + `" cy="` + fmt.Sprint(len(rows)*400000) + `"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl><a:tblPr/><a:tblGrid>` + grid + `</a:tblGrid>` + body + `</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
}
func releaseTableBaseline() [][]string {
	return [][]string{{releaseTableCell("A", "", ""), releaseTableCell("B", "", "")}, {releaseTableCell("C", "", ""), releaseTableCell("D", "", "")}}
}

func TestReleaseSlideTableFlow(t *testing.T) {
	base := releaseTableBaseline()
	insert := [][]string{base[0], {releaseTableCell("", "", ""), releaseTableCell("", "", "")}, base[1]}
	deleted := [][]string{{base[0][0]}, {base[1][0]}}
	mergedCell := strings.Replace(base[0][0], `</a:p></a:txBody>`, `</a:p><a:p><a:r><a:t>B</a:t></a:r></a:p></a:txBody>`, 1)
	mergedCell = strings.Replace(mergedCell, `<a:tc>`, `<a:tc gridSpan="2">`, 1)
	merged := [][]string{{mergedCell, `<a:tc hMerge="1"/>`}, base[1]}
	split := [][]string{{strings.Replace(mergedCell, ` gridSpan="2"`, "", 1), releaseTableCell("", "", "")}, base[1]}
	borders := ""
	for _, edge := range []string{"lnL", "lnR", "lnT", "lnB"} {
		borders += `<a:` + edge + ` w="12700"><a:solidFill><a:srgbClr val="000000"/></a:solidFill></a:` + edge + `>`
	}
	styled := [][]string{{releaseTableCell("A", `<a:solidFill><a:srgbClr val="0000FF"/></a:solidFill>`+borders, ""), releaseTableCell("B", `<a:solidFill><a:srgbClr val="0000FF"/></a:solidFill>`+borders, "")}, {releaseTableCell("C", borders, ""), releaseTableCell("D", borders, "")}}
	for _, tc := range []struct {
		name, payload string
		before, after [][]string
	}{
		{"insert-row", `{"slideId":"s1","tableName":"Table 1","operation":"insert-row","index":2}`, base, insert},
		{"delete-column", `{"slideId":"s1","tableName":"Table 1","operation":"delete-column","index":2}`, base, deleted},
		{"style", `{"slideId":"s1","tableName":"Table 1","operation":"style","headerFillColor":"#0000FF","borderColor":"#000000","borderWidth":1}`, base, styled},
		{"merge", `{"slideId":"s1","tableName":"Table 1","operation":"merge","row":1,"column":1}`, base, merged},
		{"split", `{"slideId":"s1","tableName":"Table 1","operation":"split","row":1,"column":1}`, merged, split},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &releaseSlideFlow{action: "slide-table-edit", payload: tc.payload, beforeIDs: []string{"s1", "s2"}, afterIDs: []string{"s1", "s2"}, before: releaseFlowPPTX(t, []string{releaseTableFrame(tc.before), releaseFlowOriginal}, -1), after: releaseFlowPPTX(t, []string{releaseTableFrame(tc.after), releaseFlowOriginal}, -1), png: releaseFlowPNG(t)}
			f.install(t)
			outDir := t.TempDir()
			out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", f.action, "--payload", f.payload, "--out", outDir, "--json"}).Run)
			var result map[string]any
			_ = json.Unmarshal([]byte(out), &result)
			if err != nil || result["verified"] != true || f.writes.Load() != 1 || f.opens != 2 {
				t.Fatalf("table flow %s: %v %s writes=%d opens=%d", tc.name, err, out, f.writes.Load(), f.opens)
			}
			raw, e := os.ReadFile(filepath.Join(str(result, "evidenceDir"), "table-expected.json"))
			if e != nil {
				t.Fatal(e)
			}
			var expected map[string]any
			_ = json.Unmarshal(raw, &expected)
			if expected["rows"] != float64(len(tc.before)) || expected["columns"] != float64(len(tc.before[0])) {
				t.Fatalf("baseline size not recorded: %s", raw)
			}
		})
	}
}

func TestReleaseSlideTableHeaderFillOnly(t *testing.T) {
	// A default-styled body cell need not have tcPr. Header fills must neither
	// require its serialization nor overwrite a distinct existing border style.
	border := `<a:lnL w="12700"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill><a:prstDash val="dash"/></a:lnL>`
	fill := `<a:solidFill><a:srgbClr val="0000FF"/></a:solidFill>`
	body := strings.Replace(releaseTableCell("Body", "", ""), `<a:tcPr></a:tcPr>`, "", 1)
	before := [][]string{{releaseTableCell("Header", border, "")}, {body}}
	for _, fault := range []string{"", "body-text", "body-fill", "header-border", "missing-fill"} {
		t.Run(fault, func(t *testing.T) {
			header := releaseTableCell("Header", fill+border, "")
			afterBody := body
			switch fault {
			case "body-text":
				afterBody = strings.Replace(body, ">Body<", ">Changed<", 1)
			case "body-fill":
				afterBody = releaseTableCell("Body", fill, "")
			case "header-border":
				header = strings.Replace(header, `val="dash"`, `val="solid"`, 1)
			case "missing-fill":
				header = before[0][0]
			}
			after := [][]string{{header}, {afterBody}}
			f := &releaseSlideFlow{beforeIDs: []string{"s1", "s2"}, afterIDs: []string{"s1", "s2"}, before: releaseFlowPPTX(t, []string{releaseTableFrame(before), releaseFlowOriginal}, -1), after: releaseFlowPPTX(t, []string{releaseTableFrame(after), releaseFlowOriginal}, -1), png: releaseFlowPNG(t)}
			f.install(t)
			out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", "slide-table-edit", "--payload", `{"slideId":"s1","tableName":"Table 1","operation":"style","headerFillColor":"#0000FF"}`, "--out", t.TempDir(), "--json"}).Run)
			var result map[string]any
			_ = json.Unmarshal([]byte(out), &result)
			if fault == "" {
				if err != nil || result["verified"] != true || f.opens != 2 {
					t.Fatalf("header-only fill failed: %v %s opens=%d", err, out, f.opens)
				}
			} else if err == nil || result["verified"] == true {
				t.Fatalf("accepted fault %s: %v %s", fault, err, out)
			}
			if f.writes.Load() != 1 {
				t.Fatalf("writes=%d", f.writes.Load())
			}
		})
	}
}

func TestReleaseSlideTableArtifactsRejectCollateralDamage(t *testing.T) {
	base := releaseTableBaseline()
	inserted := [][]string{base[0], {releaseTableCell("", "", ""), releaseTableCell("", "", "")}, base[1]}
	makeFile := func(frame, other string) string {
		p := filepath.Join(t.TempDir(), "fixture.pptx")
		if e := os.WriteFile(p, releaseFlowPPTX(t, []string{frame, other}, -1), 0600); e != nil {
			t.Fatal(e)
		}
		return p
	}
	beforeFrame := releaseTableFrame(base)
	afterFrame := releaseTableFrame(inserted)
	before := makeFile(beforeFrame, releaseFlowOriginal)
	spec := phase3SlidesSpec{Action: "slide-table-edit", SlideID: "s1", Table: &phase3SlideTableSpec{Name: "Table 1", Operation: "insert-row", Index: 1}}
	if e := phase3PrepareSlideTable(before, []string{"s1", "s2"}, &spec); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ name, frame, other string }{
		{"outside-slide", afterFrame, strings.Replace(releaseFlowOriginal, "original", "changed", 1)},
		{"frame-identity", strings.Replace(afterFrame, `name="Table 1"`, `name="different"`, 1), releaseFlowOriginal},
		{"frame-offset", strings.Replace(afterFrame, `<a:off x="0"`, `<a:off x="12"`, 1), releaseFlowOriginal},
		{"wrong-extent", strings.Replace(afterFrame, `cy="1200000"`, `cy="800000"`, 1), releaseFlowOriginal},
		{"reordered-content", strings.Replace(afterFrame, ">A<", ">changed<", 1), releaseFlowOriginal},
		{"duplicate-table-name", afterFrame + afterFrame, releaseFlowOriginal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if e := verifyPhase3SlideTableArtifacts(before, makeFile(tc.frame, tc.other), []string{"s1", "s2"}, []string{"s1", "s2"}, spec); e == nil {
				t.Fatal("collateral edit accepted")
			}
		})
	}
	for _, tc := range []struct {
		name  string
		table *phase3SlideTableSpec
		frame string
	}{
		{"missing-table", &phase3SlideTableSpec{Name: "unknown", Operation: "insert-row", Index: 0}, beforeFrame},
		{"missing-slide", &phase3SlideTableSpec{Name: "Table 1", Operation: "insert-row", Index: 0}, beforeFrame},
		{"insert-overrun", &phase3SlideTableSpec{Name: "Table 1", Operation: "insert-row", Index: 9}, beforeFrame},
		{"delete-overrun", &phase3SlideTableSpec{Name: "Table 1", Operation: "delete-column", Index: 9}, beforeFrame},
		{"last-column", &phase3SlideTableSpec{Name: "Table 1", Operation: "delete-column", Index: 0}, releaseTableFrame([][]string{{base[0][0]}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := phase3SlidesSpec{SlideID: "s1", Table: tc.table}
			if tc.name == "missing-slide" {
				s.SlideID = "missing"
			}
			if e := phase3PrepareSlideTable(makeFile(tc.frame, releaseFlowOriginal), []string{"s1", "s2"}, &s); e == nil {
				t.Fatal("unsafe baseline accepted")
			}
		})
	}
}
