package main

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestDocumentParagraphAlignment(t *testing.T) {
	for align, word := range map[string]string{"left": "left", "center": "center", "right": "right", "justify": "both"} {
		t.Run(align, func(t *testing.T) {
			s, err := prepareDocumentLayout("document-paragraph", map[string]any{"paragraph": float64(1), "text": "正文", "attrs": map[string]any{"align": align}})
			if err != nil {
				t.Fatal(err)
			}
			before := layoutFixture(t, `<w:ind w:left="600"/>`, "")
			after := layoutFixture(t, `<w:ind w:left="600"/><w:jc w:val="`+word+`"/>`, "")
			if err = verifyDocumentLayout(before, after, s); err != nil {
				t.Fatal(err)
			}
			for _, bad := range []documentLayoutSnapshot{before, layoutFixture(t, `<w:ind w:left="0"/><w:jc w:val="`+word+`"/>`, "")} {
				if err := verifyDocumentLayout(before, bad, s); err == nil {
					t.Fatal("missing alignment or unrelated indentation change accepted")
				}
			}
		})
	}
	for _, value := range []any{nil, true, float64(2), "distributed", "CENTER"} {
		if _, err := prepareDocumentLayout("document-paragraph", map[string]any{"paragraph": float64(1), "text": "正文", "attrs": map[string]any{"align": value}}); err == nil {
			t.Fatal("invalid alignment accepted")
		}
	}
}

func TestDocumentParagraphAlignmentUsesNativeSelection(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional existing Node runtime")
	}
	for align, value := range map[string]int{"left": 1, "center": 2, "right": 0, "justify": 3} {
		for _, missing := range []bool{false, true} {
			s := documentLayoutSpec{Action: "document-paragraph", Paragraph: 2, Text: "target", Attrs: map[string]any{"align": align}}
			before := documentLayoutSnapshot{Paragraphs: []documentLayoutParagraph{{Text: "keep"}, {Text: "target"}}}
			script := fmt.Sprintf(`global.window=global;let calls=[],selected=false;const ps=[{text:'keep'},{text:'target'}];const api={isViewMode:false};if(!%t)api.setParaAlign=n=>{if(!selected)throw Error('wrong paragraph');calls.push(n)};window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});window._sdk={options:{user:{id:1}},editor:{config:{mode:{role:'editor'}},docsApi:api,getParagraphs:()=>ps,getParagraphRange:p=>({getText:async()=>p.text,p}),runSelectionCallback:async(r,f)=>{selected=r.p===ps[1];return f()}}};%s.then(r=>{if(%t){if(r.ok!==false||calls.length)throw Error('missing method wrote')}else if(r.ok!==true||JSON.stringify(calls)!=='[%d]')throw Error('incorrect alignment')}).catch(e=>{console.error(e);process.exitCode=1});`, missing, documentLayoutJS(s, before), missing, value)
			if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
				t.Fatalf("%s missing=%v: %v %s", align, missing, err, out)
			}
		}
	}
}

func TestDocumentPageCombinedSettingsPreserveImplicitDefaults(t *testing.T) {
	s := documentLayoutSpec{Action: "document-page", Margins: map[string]float64{"left": 1.5, "right": 1.5, "top": 2, "bottom": 2}, Columns: 2, Background: "#EEEEEE"}
	before := layoutFixture(t, "", `<w:pgMar w:left="1440" w:right="1440" w:top="1440" w:bottom="1440" w:header="720" w:footer="720"/><w:cols w:space="425"/>`)
	makeAfter := func(gutter, sep, space, header string) documentLayoutSnapshot {
		after := layoutFixture(t, "", `<w:pgMar w:left="850" w:right="850" w:top="1134" w:bottom="1134" w:header="`+header+`" w:footer="720" w:gutter="`+gutter+`"/><w:cols w:num="2" w:equalWidth="1" w:space="`+space+`" w:sep="`+sep+`"/>`)
		after.Background = "EEEEEE"
		return after
	}
	if err := verifyDocumentLayout(before, makeAfter("0", "0", "425", "720"), s); err != nil {
		t.Fatal("requested margins+columns rejected for unchanged defaults:", err)
	}
	for _, after := range []documentLayoutSnapshot{makeAfter("1", "0", "425", "720"), makeAfter("0", "1", "425", "720"), makeAfter("0", "0", "600", "720"), makeAfter("0", "0", "425", "0")} {
		if err := verifyDocumentLayout(before, after, s); err == nil {
			t.Fatal("unrequested gutter, separator, column space or header change accepted")
		}
	}
}

func TestDocumentPageColumnsPreserveSpacing(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional existing Node runtime")
	}
	for _, space := range []string{"720", ""} {
		before := documentLayoutSnapshot{Paragraphs: []documentLayoutParagraph{{Text: "target"}}, SectionValues: []map[string]string{{"cols.space": space, "cols.sep": "1"}}}
		want := 12.7
		if space == "" {
			want = 425.0 * 25.4 / 1440
		}
		script := fmt.Sprintf(`global.window=global;let calls=[];window.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});window._sdk={options:{user:{id:1}},editor:{config:{mode:{role:'editor'}},docsApi:{isViewMode:false,setColumnsProps:p=>calls.push(p)},getParagraphs:()=>[{text:'target'}],getParagraphRange:p=>({getText:()=>p.text}),runSelectionCallback:async(r,f)=>f()}};%s.then(r=>{const p=calls[0];if(!r.ok||calls.length!==1||p.num!==2||p.equivalentWidth!==true||p.sep!==true||Math.abs(p.Space-%g)>0.00001)throw Error('columns lost spacing/separator')}).catch(e=>{console.error(e);process.exitCode=1})`, documentLayoutJS(documentLayoutSpec{Action: "document-page", Columns: 2}, before), want)
		if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
}

func TestDocumentParagraphAlignProtectsOtherParagraphFonts(t *testing.T) {
	before := layoutFixture(t, "", "")
	after := layoutFixture(t, `<w:jc w:val="center"/>`, "")
	after.Text.Paragraphs[1].Chars[0].Attrs = map[string]string{"bold": "true"}
	spec := documentLayoutSpec{Action: "document-paragraph", Paragraph: 1, Text: "正文", Attrs: map[string]any{"align": "center"}}
	if verifyDocumentLayout(before, after, spec) == nil {
		t.Fatal("unrelated character formatting change accepted")
	}
}
