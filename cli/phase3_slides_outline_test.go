package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const slideTestOutline = `<a:ln w="12700"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill><a:prstDash val="solid"/></a:ln>`

func slideOutlineScopeArtifact(t *testing.T, outlined bool, location string, collateral bool) string {
	t.Helper()
	shape := func(text, outline string) string {
		return `<p:sp><p:nvSpPr><p:cNvPr id="1" name="` + text + `"/></p:nvSpPr><p:spPr/><p:txBody><a:p><a:r><a:rPr>` + outline + `</a:rPr><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp>`
	}
	targetOutline, otherOutline := "", ""
	if outlined || location == "target" {
		targetOutline = slideTestOutline
	}
	if location == "same-slide" || location == "other-slide" {
		otherOutline = slideTestOutline
	}
	otherText := "other"
	if collateral {
		otherText = "changed"
	}
	first := shape("title", targetOutline)
	second := shape(otherText, otherOutline)
	if location == "same-slide" {
		first += second
		second = shape("untouched", "")
	}
	entries := releaseMediaEntries(t, releaseFlowPPTX(t, []string{first, second}, -1))
	const textStyle = `<a:lvl1pPr><a:defRPr>` + slideTestOutline + `</a:defRPr></a:lvl1pPr>`
	relation := func(kind, target string) string {
		return `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="r1" Type="` + artifactOfficeRelNS + `/` + kind + `" Target="` + target + `"/></Relationships>`
	}
	entries["ppt/slides/_rels/slide1.xml.rels"] = relation("slideLayout", "../slideLayouts/layout.xml")
	entries["ppt/slideLayouts/_rels/layout.xml.rels"] = relation("slideMaster", "../slideMasters/master.xml")
	entries["ppt/slideMasters/_rels/master.xml.rels"] = relation("theme", "../theme/theme1.xml")
	for _, item := range []struct{ location, path, tag string }{
		{"layout", "ppt/slideLayouts/layout.xml", "sldLayout"},
		{"master", "ppt/slideMasters/master.xml", "sldMaster"},
		{"other-layout", "ppt/slideLayouts/other.xml", "sldLayout"},
		{"other-master", "ppt/slideMasters/other.xml", "sldMaster"},
	} {
		styles := ""
		if location == item.location {
			styles = `<p:txStyles><p:titleStyle>` + textStyle + `</p:titleStyle></p:txStyles>`
		}
		entries[item.path] = `<p:` + item.tag + ` xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree/></p:cSld>` + styles + `</p:` + item.tag + `>`
	}
	if location == "default" {
		entries["ppt/presentation.xml"] = strings.Replace(entries["ppt/presentation.xml"], `</p:presentation>`, `<p:defaultTextStyle xmlns:a="`+artifactDrawingNS+`">`+textStyle+`</p:defaultTextStyle></p:presentation>`, 1)
	}
	if location == "theme" {
		entries["ppt/theme/theme1.xml"] = `<a:theme xmlns:a="` + artifactDrawingNS + `"><a:themeElements/><a:objectDefaults><a:txDef><a:lstStyle>` + textStyle + `</a:lstStyle></a:txDef></a:objectDefaults></a:theme>`
	}
	return artifactTestZip(t, entries)
}

func TestPhase3SlideOutlinePreflightScopedToTarget(t *testing.T) {
	spec := phase3SlidesSpec{SlideID: "s1", Format: &phase3SlideFormatSpec{Text: "title", Attrs: map[string]any{"outlineColor": "#FF0000", "outlineWidth": float64(1)}}}
	ids := []string{"s1", "s2"}
	for _, location := range []string{"", "same-slide", "other-slide", "other-layout", "other-master", "target", "layout", "master", "default", "theme"} {
		t.Run(location, func(t *testing.T) {
			before := slideOutlineScopeArtifact(t, false, location, false)
			after := slideOutlineScopeArtifact(t, true, location, false)
			err := verifyPhase3SlideTextFormatArtifacts(before, after, ids, ids, spec)
			blocked := location == "target" || location == "layout" || location == "master" || location == "default" || location == "theme"
			if (err != nil) != blocked {
				t.Fatalf("outline at %q: error=%v, want blocked=%v", location, err, blocked)
			}
			if !blocked {
				if err := verifyPhase3SlideTextFormatArtifacts(before, slideOutlineScopeArtifact(t, true, location, true), ids, ids, spec); err == nil {
					t.Fatal("unrelated shape/text modification was accepted")
				}
			}
		})
	}
}

func TestPhase3SlideOutlineScopedCommandFlow(t *testing.T) {
	for _, location := range []string{"other-slide", "same-slide", "target", "master"} {
		t.Run(location, func(t *testing.T) {
			before, err := os.ReadFile(slideOutlineScopeArtifact(t, false, location, false))
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(slideOutlineScopeArtifact(t, true, location, false))
			if err != nil {
				t.Fatal(err)
			}
			f := &releaseSlideFlow{beforeIDs: []string{"s1", "s2"}, afterIDs: []string{"s1", "s2"}, before: before, after: after, png: releaseFlowPNG(t)}
			f.install(t)
			out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", "slide-text-format", "--payload", `{"slideId":"s1","text":"title","outlineColor":"#FF0000"}`, "--out", t.TempDir(), "--json"}).Run)
			blocked := location == "target" || location == "master"
			if blocked {
				if err == nil || f.writes.Load() != 0 {
					t.Fatalf("inherited/existing target outline wrote: %v writes=%d", err, f.writes.Load())
				}
			} else if err != nil || !strings.Contains(out, `"verified": true`) || f.writes.Load() != 1 || f.opens != 2 {
				t.Fatalf("unrelated outline blocked command: %v writes=%d opens=%d output=%s", err, f.writes.Load(), f.opens, out)
			}
		})
	}
}

func TestPhase3SlideOutlineRejectsInvalidAncestor(t *testing.T) {
	s := phase3SlidesSpec{SlideID: "s1", Format: &phase3SlideFormatSpec{Text: "title"}}
	for _, relation := range []string{
		`<Relationship Id="a" Type="` + artifactOfficeRelNS + `/slideLayout" Target="https://example.invalid/layout.xml" TargetMode="External"/>`,
		`<Relationship Id="a" Type="` + artifactOfficeRelNS + `/slideLayout" Target="missing.xml"/>`,
		`<Relationship Id="a" Type="` + artifactOfficeRelNS + `/slideLayout" Target="../slideLayouts/layout.xml"/><Relationship Id="b" Type="` + artifactOfficeRelNS + `/slideLayout" Target="../slideLayouts/layout.xml"/>`,
	} {
		raw, err := os.ReadFile(slideOutlineScopeArtifact(t, false, "", false))
		if err != nil {
			t.Fatal(err)
		}
		entries := releaseMediaEntries(t, raw)
		entries["ppt/slides/_rels/slide1.xml.rels"] = `<Relationships xmlns="` + artifactRelNS + `">` + relation + `</Relationships>`
		if err := phase3CheckNewOutlineArtifact(artifactTestZip(t, entries), []string{"s1", "s2"}, s); err == nil {
			t.Fatalf("invalid inherited style source accepted: %s", relation)
		}
	}
}

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
