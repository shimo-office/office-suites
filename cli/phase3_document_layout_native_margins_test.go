package main

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestDocumentPagePreservesProvenImplicitMargins(t *testing.T) {
	before := layoutFixture(t, "", `<w:pgSz w:w="12240" w:h="15840"/>`)
	before.NativePageMargins = map[string]float64{"header": 12.5, "footer": 17.5}
	spec := documentLayoutSpec{Action: "document-page", Margins: map[string]float64{"left": 2, "right": 2, "top": 2, "bottom": 2}}
	after := layoutFixture(t, "", `<w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:left="1134" w:right="1134" w:top="1134" w:bottom="1134" w:header="709" w:footer="992"/>`)
	if err := verifyDocumentLayout(before, after, spec); err != nil {
		t.Fatal(err)
	}
	keep := documentPagePreserveMargins(spec, before)
	if keep["header"] != 12.5 || keep["footer"] != 17.5 {
		t.Fatal("missing native margins", keep)
	}
	for _, wrong := range []map[string]float64{nil, {"header": 12.5, "footer": 12.5}} {
		before.NativePageMargins = wrong
		if verifyDocumentLayout(before, after, spec) == nil {
			t.Fatal("unproved/changed margins accepted")
		}
	}
}

func TestDocumentPageRechecksImplicitMarginsBeforeWrite(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional existing Node runtime")
	}
	for _, mode := range []string{"success", "changed", "missing"} {
		t.Run(mode, func(t *testing.T) {
			before := documentLayoutSnapshot{Paragraphs: []documentLayoutParagraph{{Text: "target"}}, SectionValues: []map[string]string{{}}, NativePageMargins: map[string]float64{"header": 12.5, "footer": 17.5}}
			spec := documentLayoutSpec{Action: "document-page", Margins: map[string]float64{"left": 2, "right": 2, "top": 2, "bottom": 2}}
			script := fmt.Sprintf(`global.window=global;const mode=%q;let calls=[];const section={getPgMarginHeader:()=>12.5,getPgMarginFooter:()=>mode==='changed'?12.5:17.5};const api={isViewMode:false,getCurPosSectionPr:()=>section,updatePageSettings:p=>calls.push(p)};if(mode==='missing')delete section.getPgMarginFooter;window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});window._sdk={options:{user:{id:1}},editor:{config:{mode:{role:'editor'}},docsApi:api,getParagraphs:()=>[{text:'target'}],getParagraphRange:p=>({getText:()=>p.text}),runSelectionCallback:async(r,f)=>f()}};%s.then(r=>{if(mode==='success'){if(!r.ok||calls.length!==1||calls[0].margins.header!==12.5||calls[0].margins.footer!==17.5)throw Error('lost margins')}else if(r.ok!==false||calls.length)throw Error('unproven margins wrote')}).catch(e=>{console.error(e);process.exitCode=1})`, mode, documentLayoutJS(spec, before))
			if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
				t.Fatalf("%v %s", err, out)
			}
		})
	}
}

func TestDocumentPageNativeMarginEvidenceValidation(t *testing.T) {
	before := layoutFixture(t, "", `<w:pgMar w:header="709"/>`)
	spec := documentLayoutSpec{Action: "document-page", Margins: map[string]float64{"left": 2}}
	if !documentNeedsNativeMargins(spec, before) {
		t.Fatal("missing footer ignored")
	}
	for _, tc := range []struct {
		name  string
		value map[string]any
		ok    bool
	}{
		{"valid", map[string]any{"ok": true, "header": 12.5, "footer": 17.5}, true},
		{"explicit-conflict", map[string]any{"ok": true, "header": 13.5, "footer": 17.5}, false},
		{"absent", map[string]any{"ok": true, "header": 12.5}, false},
		{"string", map[string]any{"ok": true, "header": 12.5, "footer": "17.5"}, false},
		{"negative", map[string]any{"ok": true, "header": 12.5, "footer": -1.0}, false},
		{"failed", map[string]any{"ok": false, "header": 12.5, "footer": 17.5}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := documentWithNativeMargins(before, tc.value)
			if (err == nil) != tc.ok {
				t.Fatal(err)
			}
			if tc.ok {
				if len(got.NativePageMargins) != 1 || got.NativePageMargins["footer"] != 17.5 {
					t.Fatal(got.NativePageMargins)
				}
				if len(before.NativePageMargins) != 0 || before.SectionValues[0]["pgMar.footer"] != "" {
					t.Fatal("source evidence mutated")
				}
			}
		})
	}
}

func TestDocumentPageNativeMarginReadSelectsFirstSection(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional existing Node runtime")
	}
	for _, mode := range []string{"success", "missing", "permission", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			script := fmt.Sprintf(`global.window=global;let selected=false;const mode=%q,first={};const section={getPgMarginHeader:()=>12.5,getPgMarginFooter:()=>mode==='invalid'?NaN:17.5};if(mode==='missing')delete section.getPgMarginFooter;window._sdk={editor:{config:{mode:{role:'editor'}},docsApi:{isViewMode:mode==='permission',getCurPosSectionPr:()=>{if(!selected)throw Error('no selection');return section}},getParagraphs:()=>[first],getParagraphRange:p=>p,runSelectionCallback:async(r,f)=>{if(r!==first)throw Error('wrong section');selected=true;return f()}}};%s.then(r=>{if(mode==='success'){if(!r.ok||r.header!==12.5||r.footer!==17.5)throw Error('bad read')}else if(r.ok!==false)throw Error('unproven read accepted')}).catch(e=>{console.error(e);process.exitCode=1})`, mode, documentNativePageMarginsJS)
			if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
				t.Fatalf("%v %s", err, out)
			}
		})
	}
}
