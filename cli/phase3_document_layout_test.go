package main

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDocumentLayoutSpecRejectsAmbiguity(t *testing.T) {
	good := map[string]any{"paragraph": float64(2), "text": "正文", "attrs": map[string]any{"firstLineChars": float64(2), "lineMultiple": 1.5, "shading": "#eeeeee"}}
	s, e := prepareDocumentLayout("document-paragraph", good)
	if e != nil || s.Paragraph != 2 || s.Attrs["shading"] != "#EEEEEE" {
		t.Fatalf("%#v %v", s, e)
	}
	for _, p := range []map[string]any{{"paragraph": float64(0), "text": "x", "attrs": map[string]any{"heading": float64(1)}}, {"paragraph": float64(1), "text": "x", "attrs": map[string]any{"lineMultiple": float64(0)}}, {"paragraph": float64(1), "text": "x", "attrs": map[string]any{"shading": "red"}}, {"paragraph": float64(1), "text": "x", "attrs": map[string]any{"typo": true}}} {
		if _, e := prepareDocumentLayout("document-paragraph", p); e == nil {
			t.Fatal("invalid accepted", p)
		}
	}
	for _, n := range []float64{0.29, 1.1, 1.15} {
		if _, e := prepareDocumentLayout("document-paragraph", map[string]any{"paragraph": float64(1), "text": "正文", "attrs": map[string]any{"firstLineChars": n}}); e != nil {
			t.Fatal("valid hundredth-character indent rejected", n, e)
		}
	}
	if _, e := prepareDocumentLayout("document-paragraph", map[string]any{"paragraph": float64(1), "text": "正文", "attrs": map[string]any{"firstLineChars": 1.111}}); e == nil {
		t.Fatal("fractional hundredth accepted")
	}

	if _, e := prepareDocumentLayout("document-page", map[string]any{"marginsCm": map[string]any{"left": 1.5}}); e == nil {
		t.Fatal("partial margins accepted")
	}
}
func layoutFixture(t *testing.T, props, sect string) documentLayoutSnapshot {
	t.Helper()
	f := artifactTestZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:pPr>` + props + `</w:pPr><w:r><w:t>正文</w:t></w:r></w:p><w:p><w:r><w:t>不改</w:t></w:r></w:p><w:sectPr>` + sect + `</w:sectPr></w:body></w:document>`})
	s, e := readDocumentLayout(f)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestDocumentLayoutProofExactTarget(t *testing.T) {
	s, _ := prepareDocumentLayout("document-paragraph", map[string]any{"paragraph": float64(1), "text": "正文", "attrs": map[string]any{"firstLineChars": float64(2), "lineMultiple": 1.5, "shading": "#EEEEEE"}})
	before := layoutFixture(t, "", "")
	after := layoutFixture(t, `<w:ind w:firstLineChars="200"/><w:spacing w:line="360" w:lineRule="auto"/><w:shd w:val="clear" w:fill="EEEEEE"/>`, "")
	if e := verifyDocumentLayout(before, after, s); e != nil {
		t.Fatal(e)
	}
	if e := verifyDocumentLayout(before, before, s); e == nil {
		t.Fatal("no-op accepted")
	}
	wrong := layoutFixture(t, `<w:ind w:firstLine="200"/><w:spacing w:line="360" w:lineRule="exact"/><w:shd w:fill="EEEEEE"/>`, "")
	if e := verifyDocumentLayout(before, wrong, s); e == nil {
		t.Fatal("units and rule mismatch accepted")
	}
	after.Paragraphs[1].Text = "误改"
	if e := verifyDocumentLayout(before, after, s); e == nil {
		t.Fatal("collateral text accepted")
	}
}
func TestDocumentPageMarginsUnits(t *testing.T) {
	s, e := prepareDocumentLayout("document-page", map[string]any{"marginsCm": map[string]any{"left": 1.5, "right": 1.5, "top": float64(2), "bottom": float64(2)}, "columns": float64(2)})
	if e != nil {
		t.Fatal(e)
	}
	before := layoutFixture(t, "", "")
	after := layoutFixture(t, "", `<w:pgMar w:left="850" w:right="850" w:top="1134" w:bottom="1134"/><w:cols w:num="2"/>`)
	if e = verifyDocumentLayout(before, after, s); e != nil {
		t.Fatal(e)
	}
	if e = verifyDocumentLayout(before, before, s); e == nil {
		t.Fatal("unchanged margins accepted")
	}
}

func TestDocumentLayoutPreservesUntargetedIndent(t *testing.T) {
	s, _ := prepareDocumentLayout("document-paragraph", map[string]any{"paragraph": float64(1), "text": "正文", "attrs": map[string]any{"firstLineChars": float64(2)}})
	before := layoutFixture(t, `<w:ind w:left="600"/>`, "")
	wrong := layoutFixture(t, `<w:ind w:left="0" w:firstLineChars="200"/>`, "")
	if e := verifyDocumentLayout(before, wrong, s); e == nil {
		t.Fatal("unrequested left indent change accepted")
	}
	if _, e := layoutColor("#+ABCDE"); e == nil {
		t.Fatal("nonhex accepted")
	}
}

func TestDocumentNumberingOverrideCannotMasqueradeAsDecimal(t *testing.T) {
	f := artifactTestZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>正文</w:t></w:r></w:p></w:body></w:document>`, "word/numbering.xml": `<w:numbering xmlns:w="` + artifactWordNS + `"><w:abstractNum w:abstractNumId="7"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="7"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="5"/><w:lvl><w:numFmt w:val="bullet"/></w:lvl></w:lvlOverride></w:num></w:numbering>`})
	x, e := readDocumentLayout(f)
	if e != nil {
		t.Fatal(e)
	}
	if x.Paragraphs[0].NumberFormat != "bullet" || x.Paragraphs[0].NumberStart != "5" || x.Paragraphs[0].NumberID != "1" {
		t.Fatal("override ignored")
	}
}

func TestDocumentLayoutBrowserChecksBeforeMutation(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("existing optional Node runtime required for SDK guard regression")
	}
	s, _ := prepareDocumentLayout("document-paragraph", map[string]any{"paragraph": float64(1), "text": "正文", "attrs": map[string]any{"lineMultiple": 1.5, "firstLineChars": float64(2), "shading": "#EEEEEE"}})
	before := documentLayoutSnapshot{Paragraphs: []documentLayoutParagraph{{Text: "正文"}}}
	setup := `global.window=global;let calls=[];window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});window._sdk={options:{user:{id:1}},editor:{config:{mode:{role:'editor'}},getParagraphs:()=>[{text:'正文'}],getParagraphRange:p=>({getText:async()=>p.text}),runSelectionCallback:async(r,f)=>f(),docsApi:{isViewMode:false,applyParagraphProperties:v=>calls.push(['indent',v]),setLineSpacing:v=>calls.push(['spacing',v]),setParagraphShd:(enabled,c)=>calls.push(['shading',enabled,c.docOfgetR(),c.docOfgetG(),c.docOfgetB()])}}};`
	for _, scenario := range []string{"good", "viewer", "other-user", "stale", "arrives-during-read", "role-changes-before-callback"} {
		change := ""
		switch scenario {
		case "viewer":
			change = `window._sdk.editor.config.mode.role='viewer';`
		case "other-user":
			change = `window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1','2']});`
		case "stale":
			change = `window._sdk.editor.getParagraphs=()=>[{text:'changed'}];`
		case "arrives-during-read":
			change = `window._sdk.editor.getParagraphRange=p=>({getText:async()=>{window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1','2']});return p.text}});`
		case "role-changes-before-callback":
			change = `window._sdk.editor.runSelectionCallback=async(r,f)=>{window._sdk.editor.config.mode.role='viewer';return f()};`
		}
		assert := `if(r.ok!==false||calls.length)throw Error('unsafe mutation allowed')`
		if scenario == "good" {
			assert = `if(!r.ok||calls.length!==3||!calls.some(c=>c[0]==='spacing'&&c[1].type===1&&c[1].value===1.5)||!calls.some(c=>c[1]?.Ind?.FirstLineChars===200)||!calls.some(c=>c[1]?.Shd?.fill==='EEEEEE'&&c[1].Shd.Value===0&&c[1].Shd.Color.docOfgetR()===238))throw Error('wrong typed mutation')`
		}
		cmd := exec.Command(node, "-e", setup+change+documentLayoutJS(s, before)+`.then(r=>{`+assert+`}).catch(e=>{console.error(e);process.exitCode=1})`)
		if output, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("%s: %v %s", scenario, e, output)
		}
	}
}

func TestDocumentReplaceRechecksEachActualWrite(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("existing optional Node runtime required")
	}
	spec := documentReplaceSpec{Find: "旧", Replacement: "新", ExpectedCount: 2}
	before := documentLayoutSnapshot{Paragraphs: []documentLayoutParagraph{{Text: "旧旧"}}}
	steps, err := documentReplaceSteps(before, spec)
	if err != nil {
		t.Fatal(err)
	}
	setup := `global.window=global;global.btoa=s=>Buffer.from(s,'binary').toString('base64');global.atob=s=>Buffer.from(s,'base64').toString('binary');let writes=0,content='旧旧',selected,mode='';let ids=['1'];window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids});const at=n=>btoa('0|0|0|'+n),offset=s=>Number(atob(s).split('|').slice(-1)[0]);const e={config:{mode:{role:'editor'}},docsApi:{isViewMode:false},getParagraphs:()=>[{text:content,children:[{getText:()=>content,children:Array.from(content)}]}],getParagraphRange:p=>({start:at(0),end:at(Array.from(p.text).length),getText:async()=>{if(mode==='read-enter')ids=['1','2'];return p.text}}),generateRange:r=>({...r,getText:async()=>Array.from(content).slice(offset(r.start),offset(r.end)).join('')}),runSelectionCallback:async(r,f)=>{selected=r;if(mode==='callback-role')e.config.mode.role='viewer';return f()},pasteDataInternal:async(t,type)=>{if(type!=='text')throw Error('wrong paste type');writes++;content=Array.from(content).slice(0,offset(selected.start)).join('')+t+Array.from(content).slice(offset(selected.end)).join('');if(mode==='after-first')ids=['1','2']}};window._sdk={options:{user:{id:1}},editor:e};`
	for _, mode := range []string{"good", "read-enter", "callback-role", "after-first"} {
		assert := `if(!r.ok||writes!==2||content!=='新新')throw Error('wrong complete result')`
		if mode == "read-enter" || mode == "callback-role" {
			assert = `if(r.ok!==false||writes!==0)throw Error('unsafe write')`
		}
		if mode == "after-first" {
			assert = `if(r.ok!==false||writes!==1||!r.error.includes('1处'))throw Error('partial failure not stopped/reported')`
		}
		cmd := exec.Command(node, "-e", setup+`mode='`+mode+`';`+documentReplaceJS(spec, before, steps)+`.then(r=>{`+assert+`}).catch(e=>{console.error(e);process.exitCode=1})`)
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("%s: %v %s", mode, e, out)
		}
	}
}

func TestDocumentReplaceLiteralCountAndPreservation(t *testing.T) {
	spec, e := prepareDocumentReplace(map[string]any{"find": "旧", "replacement": "新版", "expectedCount": float64(2)})
	if e != nil {
		t.Fatal(e)
	}
	before := layoutFixture(t, "", "")
	before.Paragraphs[0].Text = "甲旧乙旧丙"
	before.Text.Paragraphs[0] = documentParagraph{Text: "甲旧乙旧丙", Chars: []documentChar{{Text: "甲旧乙旧丙", Attrs: map[string]string{"bold": "true"}}}}
	steps, e := documentReplaceSteps(before, spec)
	if e != nil {
		t.Fatal(e)
	}
	if len(steps) != 2 || steps[0].Start != 3 || steps[1].Text != "甲旧乙新版丙" {
		t.Fatalf("wrong reverse plan %#v", steps)
	}
	after := layoutFixture(t, "", "")
	after.Paragraphs[0].Text = "甲新版乙新版丙"
	after.Text.Paragraphs[0] = documentParagraph{Text: "甲新版乙新版丙", Chars: []documentChar{{Text: "甲新版乙新版丙", Attrs: map[string]string{"bold": "true"}}}}
	if e = verifyDocumentReplace(before, after, spec); e != nil {
		t.Fatal(e)
	}
	if e = verifyDocumentReplace(before, before, spec); e == nil {
		t.Fatal("no-op replacement accepted")
	}
	after.Text.Paragraphs[0].Chars[0].Attrs = map[string]string{}
	if e = verifyDocumentReplace(before, after, spec); e == nil {
		t.Fatal("format loss accepted")
	}
	spec.ExpectedCount = 3
	if _, e = documentReplaceSteps(before, spec); e == nil {
		t.Fatal("unexpected match count accepted")
	}
}

func TestDocumentLayoutFailureReportsPostMutationUncertainty(t *testing.T) {
	cause := errors.New("fresh-profile export failed")
	err := documentLayoutFailure(cause, true, "/evidence")
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "可能已生效") || !strings.Contains(err.Error(), "禁止自动重放") || !strings.Contains(err.Error(), "/evidence") {
		t.Fatal(err)
	}
	err = documentLayoutFailure(cause, false, "/evidence")
	if strings.Contains(err.Error(), "可能已生效") || !strings.Contains(err.Error(), "未写入") {
		t.Fatal(err)
	}
}
func TestDocumentNumberingAllowsOnlyNativeListIndentNormalization(t *testing.T) {
	before := layoutFixture(t, "", "")
	after := layoutFixture(t, `<w:ind w:firstLineChars="0"/>`, "")
	after.Paragraphs[0].Props["pStyle"] = "native-list-style"
	after.Paragraphs[0].Values["pStyle.val"] = "27"
	after.Paragraphs[0].StyleName = "List Paragraph"
	after.Paragraphs[0].NumberFormat = "decimal"
	spec := documentLayoutSpec{Action: "document-paragraph", Paragraph: 1, Text: "正文", Attrs: map[string]any{"numbering": "decimal"}}
	if e := verifyDocumentLayout(before, after, spec); e != nil {
		t.Fatal(e)
	}
	after.Paragraphs[0].Values["ind.left"] = "400"
	if e := verifyDocumentLayout(before, after, spec); e == nil {
		t.Fatal("collateral indent allowed")
	}
}

func TestDocumentPageOnlyAcceptsExactImplicitNoteDefaults(t *testing.T) {
	before := layoutFixture(t, "", "")
	after := layoutFixture(t, "", `<w:cols w:num="2"/><w:footnotePr><w:pos w:val="pageBottom"/><w:numFmt w:val="decimal"/><w:numRestart w:val="continuous"/></w:footnotePr><w:endnotePr><w:pos w:val="docEnd"/><w:numFmt w:val="lowerRoman"/><w:numRestart w:val="continuous"/></w:endnotePr>`)
	spec := documentLayoutSpec{Action: "document-page", Columns: 2}
	if e := verifyDocumentLayout(before, after, spec); e != nil {
		t.Fatal(e)
	}
	after.Sections[0]["footnotePr"] = "changed note position"
	if e := verifyDocumentLayout(before, after, spec); e == nil {
		t.Fatal("nondefault note change accepted")
	}
}

func TestDocumentPageColumnsProtectsGapAndSeparator(t *testing.T) {
	before := layoutFixture(t, "", `<w:cols w:num="1" w:space="720" w:sep="0"/>`)
	good := layoutFixture(t, "", `<w:cols w:num="2" w:space="720" w:sep="0" w:equalWidth="1"/>`)
	spec := documentLayoutSpec{Action: "document-page", Columns: 2}
	if e := verifyDocumentLayout(before, good, spec); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{`<w:cols w:num="2" w:space="0" w:sep="0"/>`, `<w:cols w:num="2" w:space="720" w:sep="1"/>`, `<w:cols w:num="2" w:space="720" w:sep="0" w:equalWidth="0"/>`} {
		if e := verifyDocumentLayout(before, layoutFixture(t, "", bad), spec); e == nil {
			t.Fatal("unrequested or unequal columns accepted")
		}
	}
}

func TestDocumentPageDoesNotOverrideCustomInheritedNoteDefaults(t *testing.T) {
	before := layoutFixture(t, "", "")
	before.StandardNoteDefaults["footnotePr"] = false
	after := layoutFixture(t, "", `<w:cols w:num="2"/><w:footnotePr><w:pos w:val="pageBottom"/><w:numFmt w:val="decimal"/><w:numRestart w:val="continuous"/></w:footnotePr>`)
	if e := verifyDocumentLayout(before, after, documentLayoutSpec{Action: "document-page", Columns: 2}); e == nil {
		t.Fatal("custom inherited numbering silently overridden")
	}
	f := artifactTestZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:r><w:t>正文</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`, "word/settings.xml": `<w:settings xmlns:w="` + artifactWordNS + `"><w:footnotePr><w:numFmt w:val="lowerRoman"/></w:footnotePr></w:settings>`})
	snapshot, e := readDocumentLayout(f)
	if e != nil {
		t.Fatal(e)
	}
	if snapshot.StandardNoteDefaults["footnotePr"] {
		t.Fatal("custom global note format ignored")
	}
}
