package main

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestPhase3SlideOutlineRequiresTextStroke(t *testing.T) {
	s, e := preparePhase3SlideFormat(map[string]any{"text": "title", "outlineColor": "#ff0000", "outlineWidth": float64(1)})
	if e != nil {
		t.Fatal(e)
	}
	shape := func(run, shape string) *artifactNode {
		n, e := parseArtifactXML([]byte(`<p:sp xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:nvSpPr><p:cNvPr id="1"/></p:nvSpPr><p:spPr>` + shape + `</p:spPr><p:txBody><a:p><a:r><a:rPr>` + run + `</a:rPr><a:t>title</a:t></a:r></a:p></p:txBody></p:sp>`))
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	outline := `<a:ln w="12700"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill><a:prstDash val="solid"/></a:ln>`
	if e := verifyPhase3SlideTextFormatNode(shape(outline, ""), *s); e != nil {
		t.Fatal(e)
	}
	if e := verifyPhase3SlideTextFormatNode(shape("", outline), *s); e == nil {
		t.Fatal("shape border masqueraded as text outline")
	}
	for _, bad := range []string{`<a:ln w="1"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:ln>`, `<a:ln w="12700"><a:solidFill><a:srgbClr val="FF0000"><a:alpha val="0"/></a:srgbClr></a:solidFill></a:ln>`} {
		if e := verifyPhase3SlideTextFormatNode(shape(bad, ""), *s); e == nil {
			t.Fatal("wrong/invisible outline accepted")
		}
	}
}

func TestPhase3SlideOutlineChecksActiveHistoryBeforeAnyFormatWrite(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional Node test runtime")
	}
	spec := phase3SlidesSpec{SlideID: "a", Format: &phase3SlideFormatSpec{Text: "title", Attrs: map[string]any{"fontFamily": "Arial", "fontSize": 28, "color": "#0000FF", "outlineColor": "#FF0000", "outlineWidth": float64(1)}}}
	for _, state := range []string{"missing", "wrong-presentation", "missing-constructor", "missing-parser", "active"} {
		script := fmt.Sprintf(`let writes=0,actions=0;const state=%q;class TextPr{_$fromJSON$_(v){this._$textStroke$_=v.ln}}
const presentation={_$getCalcedTextPr$_:()=>state==='missing-constructor'?Object.create(null):state==='missing-parser'?{}:new TextPr,_$applyTextPrToParagraph$_(){writes++}};
const active={presentation:%q==='wrong-presentation'?{}:presentation,_$createNewAction$_(){if(this.presentation!==presentation)throw Error('wrong history owner');actions++}};
const editor={presentation,_$collaborationManager$_:{_$changesCollector$_:{_$changesStack$_:%q==='missing'?undefined:active}}};
global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:[1]}),EditorUtil:{_$FontKit$_:{_$TextPrConstructorFn$_:TextPr},_$ChangesStack$_:{_$createNewAction$_(){throw Error('stale global history')}}},presentationSDK:{editor:{workbench:{isReady:()=>true,permissions:{canEdit:()=>true},context:{getUser:()=>({id:1})},editor,core:{setFont(){writes++},setFontSize(){writes++},setTextColor(){writes++},setTextRange(){},selectElementByIds(){},runAtomicChange(e,fn){return fn()}}}},API:{slides:{getSlides:()=>[{id:'a',getShapes:()=>[{id:'shape',textContent:'title'}]}],setCurrentSlideIndex(){},setSelectedSlides(){}}}}};
let result;try{result=%s}catch(e){throw Error('format must not use stale history: '+e.message)}
if(%q==='active'){if(result.ok!==true||writes!==4||actions!==1)throw Error('active native history not used')}
else if(result.ok!==false||writes!==0||actions!==0)throw Error('missing/wrong history allowed partial formatting');`, state, state, state, phase3SlideTextFormatMutationJS(spec, []string{"a"}), state)
		cmd := exec.Command(node, "-e", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("state=%s: %v\n%s", state, err, out)
		}
	}
}
