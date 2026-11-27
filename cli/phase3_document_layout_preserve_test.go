package main

import (
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestDocumentPagePreserveHeaderFooterDistances(t *testing.T) {
	page := documentLayoutSpec{Action: "document-page", Margins: map[string]float64{"left": 15, "right": 15, "top": 20, "bottom": 20}}
	before := layoutFixture(t, "", `<w:pgMar w:header="720" w:footer="360"/>`)
	if got := documentPagePreserveMargins(page, before); !reflect.DeepEqual(got, map[string]float64{"header": 12.7, "footer": 6.35}) {
		t.Fatalf("header/footer conversion: %v", got)
	}
	for _, tc := range []struct {
		name string
		spec documentLayoutSpec
		snap documentLayoutSnapshot
	}{
		{"different-action", documentLayoutSpec{Action: "document-paragraph", Margins: page.Margins}, before},
		{"no-margin-write", documentLayoutSpec{Action: "document-page"}, before},
		{"missing-section", page, documentLayoutSnapshot{}},
		{"missing-distances", page, layoutFixture(t, "", "")},
		{"invalid-distances", page, layoutFixture(t, "", `<w:pgMar w:header="invalid" w:footer="invalid"/>`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := documentPagePreserveMargins(tc.spec, tc.snap); len(got) != 0 {
				t.Fatalf("unexpected preservation settings: %v", got)
			}
		})
	}
	for _, tc := range []struct {
		name  string
		after documentLayoutSnapshot
		reset bool
	}{
		{"unchanged", before, false},
		{"missing-section", documentLayoutSnapshot{}, false},
		{"header-reset", layoutFixture(t, "", `<w:pgMar w:header="0" w:footer="360"/>`), true},
		{"footer-reset", layoutFixture(t, "", `<w:pgMar w:header="720" w:footer="0"/>`), true},
		{"only-body-margins", layoutFixture(t, "", `<w:pgMar w:header="720" w:footer="360" w:left="850"/>`), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := documentPageMarginsReset(before, tc.after); got != tc.reset {
				t.Fatalf("reset = %v, want %v", got, tc.reset)
			}
		})
	}
	if documentPageMarginsReset(documentLayoutSnapshot{}, before) {
		t.Fatal("missing baseline treated as proven reset")
	}
}

func TestDocumentPagePreserveScriptRestoresOnlyHeaderFooter(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node runtime required for SDK regression")
	}
	setup := `global.window=global;let calls=[];window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});window._sdk={options:{user:{id:1}},editor:{config:{mode:{role:'editor'}},docsApi:{isViewMode:false,updatePageSettings:v=>calls.push(v)}}};`
	for _, tc := range []struct{ name, change, assertion string }{
		{"restore", "", `if(!r.ok||JSON.stringify(calls)!==JSON.stringify([{scope:1,margins:{footer:6.35,header:12.7}}])||JSON.stringify(r.restored)!==JSON.stringify({footer:6.35,header:12.7}))throw Error('wrong restoration')`},
		{"missing-api", `delete window._sdk.editor.docsApi.updatePageSettings;`, `if(r.ok!==false||!r.error.includes('页面设置')||calls.length)throw Error('missing API accepted')`},
		{"disconnected", `window.__shimoPhase3Presence=()=>({ready:true,connected:false,ids:['1']});`, `if(r.ok!==false||calls.length)throw Error('disconnected mutation')`},
		{"collaborator", `window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1','2']});`, `if(r.ok!==false||calls.length)throw Error('concurrent mutation')`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := setup + tc.change + documentPagePreserveJS(map[string]float64{"header": 12.7, "footer": 6.35}) + `.then(r=>{` + tc.assertion + `}).catch(e=>{console.error(e);process.exitCode=1})`
			if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
		})
	}
}

func TestDocumentSectionDefaultsDoNotHideUnrequestedChanges(t *testing.T) {
	implicit := map[string]string{"pgMar.left": "1440", "pgMar.header": "720", "pgSz.w": "11906", "unrelated.setting": "before"}
	explicit := map[string]string{"pgMar.left": "850", "pgMar.header": "720", "pgSz.w": "11906", "unrelated.setting": "after", "cols.num": "1", "cols.equalWidth": "1", "cols.space": "425", "cols.sep": "0", "pgMar.gutter": "0", "pgSz.orient": "portrait"}
	requested := map[string]float64{"left": 15}
	for _, pair := range [][2]map[string]string{{implicit, explicit}, {explicit, implicit}, {implicit, implicit}} {
		if err := documentSectionDefaultEffects(pair[0], pair[1], requested); err != nil {
			t.Fatalf("equivalent defaults or requested margin rejected: %v", err)
		}
	}
	for _, tc := range []struct{ key, before, after string }{
		{"pgMar.header", "720", "0"},
		{"pgMar.footer", "360", ""},
		{"cols.num", "", "2"},
		{"cols.space", "425", "600"},
		{"pgSz.orient", "", "landscape"},
		{"pgSz.w", "11906", "16838"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			err := documentSectionDefaultEffects(map[string]string{tc.key: tc.before}, map[string]string{tc.key: tc.after}, requested)
			want := fmt.Sprintf("%s 由 %q 变为 %q", tc.key, tc.before, tc.after)
			if err == nil || err.Error() != want {
				t.Fatalf("unrequested change diagnostic = %v, want %s", err, want)
			}
		})
	}
}

func TestDocumentLayoutDiffReportsSectionChanges(t *testing.T) {
	before := layoutFixture(t, "", `<w:pgMar w:header="720"/><w:cols w:num="1"/>`)
	after := layoutFixture(t, "", `<w:pgMar w:header="0" w:footer="360"/><w:cols w:num="1"/>`)
	want := []string{"第1节 pgMar 节点发生变化", "pgMar.footer （空） → 360", "pgMar.header 720 → 0"}
	if got := documentLayoutDiff(before, after); !reflect.DeepEqual(got, want) {
		t.Fatalf("section changes = %v, want %v", got, want)
	}
	if got := documentLayoutDiff(before, before); len(got) != 0 {
		t.Fatalf("identical snapshots have differences: %v", got)
	}
	before = documentLayoutSnapshot{Background: "FFFFFF"}
	after = documentLayoutSnapshot{Background: "000000", Sections: []map[string]string{{}}, Paragraphs: []documentLayoutParagraph{{Text: "added"}}}
	want = []string{`页面背景 "FFFFFF" → "000000"`, "节数 0 → 1", "段落数 0 → 1"}
	if got := documentLayoutDiff(before, after); !reflect.DeepEqual(got, want) {
		t.Fatalf("structural changes = %v, want %v", got, want)
	}
}

func TestDocumentLayoutDiffReportsParagraphAndRunChanges(t *testing.T) {
	paragraph := documentLayoutParagraph{Text: "正文"}
	plain := documentParagraph{Chars: []documentChar{{Text: "正文", Attrs: map[string]string{"bold": "true", "color": "000000"}}}}
	for _, tc := range []struct {
		name      string
		paragraph documentLayoutParagraph
		runs      []documentParagraph
		want      string
	}{
		{"text", documentLayoutParagraph{Text: "新正文"}, []documentParagraph{plain}, "第1段正文改变"},
		{"properties", documentLayoutParagraph{Text: "正文", Props: map[string]string{"jc": "center"}}, []documentParagraph{plain}, "第1段段落属性改变"},
		{"settings", documentLayoutParagraph{Text: "正文", Values: map[string]string{"spacing.line": "360", "ind.left": "850"}}, []documentParagraph{plain}, "第1段段落设置：ind.left （空）→850，spacing.line （空）→360"},
		{"missing-runs", paragraph, nil, "第1段字符数 2 → 0"},
		{"character", paragraph, []documentParagraph{{Chars: []documentChar{{Text: "正字", Attrs: plain.Chars[0].Attrs}}}}, `第1段第2字符 "文" → "字"`},
		{"formatting", paragraph, []documentParagraph{{Chars: []documentChar{{Text: "正文", Attrs: map[string]string{"color": "FF0000", "italic": "true"}}}}}, `第1段第1字符 bold "true"→""，color "000000"→"FF0000"，italic ""→"true"（该段共2字）`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := documentLayoutSnapshot{Paragraphs: []documentLayoutParagraph{paragraph}, Text: documentSnapshot{Paragraphs: []documentParagraph{plain}}}
			after := documentLayoutSnapshot{Paragraphs: []documentLayoutParagraph{tc.paragraph}, Text: documentSnapshot{Paragraphs: tc.runs}}
			if got := documentLayoutDiff(before, after); !reflect.DeepEqual(got, []string{tc.want}) {
				t.Fatalf("diagnostic = %v, want %s", got, tc.want)
			}
		})
	}
}

func TestDocumentLayoutDiffLimitsConsoleOutput(t *testing.T) {
	before, after := documentLayoutSnapshot{}, documentLayoutSnapshot{}
	for i := 0; i < 11; i++ {
		before.Paragraphs = append(before.Paragraphs, documentLayoutParagraph{Text: "原文"})
		after.Paragraphs = append(after.Paragraphs, documentLayoutParagraph{Text: "改写"})
	}
	got := documentLayoutDiff(before, after)
	if len(got) != 9 || got[0] != "第1段正文改变" || got[7] != "第8段正文改变" || got[8] != "另有3处差异，见 failure-diff.json" {
		t.Fatalf("truncated diagnostic lost positions or evidence pointer: %s", strings.Join(got, "\n"))
	}
}
