package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestPhase3SlideAnimationOnlySpec(t *testing.T) {
	targets := []any{map[string]any{"slideId": "a", "text": "Title"}}
	s, err := preparePhase3Slides("slide-title-animate", map[string]any{"targets": targets})
	if err != nil || s.Animation == nil || s.Animation.FontSize != 0 {
		t.Fatalf("animation-only request rejected: %+v %v", s, err)
	}
	raw, err := json.Marshal(s.Animation)
	if err != nil || strings.Contains(string(raw), "fontSize") {
		t.Fatalf("omitted fontSize became a font change: %s %v", raw, err)
	}
	for _, value := range []any{nil, float64(0), float64(-1), "24", float64(401)} {
		if _, err := preparePhase3Slides("slide-title-animate", map[string]any{"targets": targets, "fontSize": value}); err == nil {
			t.Fatalf("invalid explicit fontSize accepted: %v", value)
		}
	}
	if _, err := preparePhase3Slides("slide-title-animate", map[string]any{"targets": targets, "unknown": true}); err == nil {
		t.Fatal("unknown option accepted")
	}
	for _, value := range []any{nil, "", "two\nlines", strings.Repeat("x", 100001)} {
		if _, err := preparePhase3Slides("slide-title-animate", map[string]any{"targets": []any{map[string]any{"slideId": "a", "text": value}}}); err == nil {
			t.Fatal("invalid animation target text accepted")
		}
	}
}

func TestPhase3SlideAnimationOnlyPreservesTypography(t *testing.T) {
	timing := `<p:timing><p:tnLst><p:par><p:cTn presetClass="entr" presetID="1" nodeType="clickEffect"><p:childTnLst><p:set><p:cBhvr><p:cTn dur="1"/><p:tgtEl><p:spTgt spid="7"/></p:tgtEl><p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst></p:cBhvr><p:to><p:strVal val="visible"/></p:to></p:set></p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`
	fixture := func(runStyle, animation, other string) string {
		return artifactTestZip(t, map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId r:id="one"/></p:sldIdLst></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/one.xml"/></Relationships>`,
			"ppt/slides/one.xml":              `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="7" name="title"/></p:nvSpPr><p:txBody><a:p><a:r>` + runStyle + `<a:t>Title</a:t></a:r></a:p></p:txBody></p:sp><p:sp><p:nvSpPr><p:cNvPr id="9" name="body"/></p:nvSpPr><p:txBody><a:p><a:r><a:t>` + other + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld>` + animation + `</p:sld>`,
		})
	}
	s := phase3SlidesSpec{Animation: &phase3SlideAnimationSpec{Targets: []phase3SlideAnimationTarget{{SlideID: "a", Text: "Title"}}}}
	for _, style := range []string{"", `<a:rPr sz="3200" b="1"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:rPr>`} {
		before := fixture(style, "", "keep")
		if err := verifyPhase3SlideAnimationArtifacts(before, fixture(style, timing, "keep"), []string{"a"}, []string{"a"}, s); err != nil {
			t.Fatalf("animation-only changed no typography but failed: %v", err)
		}
		for _, bad := range []string{fixture(`<a:rPr sz="2400"/>`, timing, "keep"), fixture(style, timing, "changed"), fixture(style, "", "keep"), fixture(style, strings.ReplaceAll(timing, `spid="7"`, `spid="9"`), "keep")} {
			if err := verifyPhase3SlideAnimationArtifacts(before, bad, []string{"a"}, []string{"a"}, s); err == nil {
				t.Fatal("font/content change or incorrect timing accepted for animation-only request")
			}
		}
	}
}

func TestPhase3SlideAnimationOnlyNativeAction(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional existing Node runtime")
	}
	for _, tc := range []struct {
		size          float64
		missingSetter bool
	}{{0, true}, {24, false}, {24, true}} {
		s := phase3SlidesSpec{Animation: &phase3SlideAnimationSpec{Targets: []phase3SlideAnimationTarget{{SlideID: "a", Text: "Title"}}, FontSize: tc.size}}
		script := fmt.Sprintf(`let animations=[],fontWrites=[],animationWrites=0,selected=false;
const core={getSlideAnimation:()=>animations,setTextRange(){},selectElementByIds(e,ids){selected=JSON.stringify(ids)==='["title"]'},applySlideAnimation(e,value){if(!selected)throw Error('wrong target');animationWrites++;animations=value}};
if(!%t)core.setFontSize=(e,size)=>{if(!selected)throw Error('wrong font target');fontWrites.push(size)};
global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:[1]}),presentationSDK:{editor:{workbench:{isReady:()=>true,permissions:{canEdit:()=>true},context:{getUser:()=>({id:1})},editor:{},core}},API:{slides:{getSlides:()=>[{id:'a',getIndex:()=>0,getShapes:()=>[{id:'title',textContent:'Title'}]}],setCurrentSlideIndex(){}}}}};
const result=%s;
if(%t&&%v>0){if(result.ok!==false||animationWrites!==0||fontWrites.length!==0)throw Error('missing requested setter wrote data')}
else {if(result.ok!==true||animationWrites!==1)throw Error('animation not written');if(JSON.stringify(fontWrites)!==JSON.stringify(%v>0?[%v]:[]))throw Error('unexpected font mutation')}`, tc.missingSetter, phase3SlideAnimationMutationJS(s, []string{"a"}), tc.missingSetter, tc.size, tc.size, tc.size)
		if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
			t.Fatalf("size=%v missingSetter=%v: %v\n%s", tc.size, tc.missingSetter, err, out)
		}
	}
}

func TestPhase3SlideAnimationSpec(t *testing.T) {
	s, e := preparePhase3Slides("slide-title-animate", map[string]any{"targets": []any{map[string]any{"slideId": "a", "text": "Title A"}, map[string]any{"slideId": "b", "text": "Title B"}}, "fontSize": float64(24)})
	if e != nil || s.Animation == nil || s.SlideID != "a" {
		t.Fatalf("%+v %v", s, e)
	}
	if _, e = preparePhase3Slides("slide-title-animate", map[string]any{"targets": []any{map[string]any{"slideId": "a", "text": "A"}, map[string]any{"slideId": "a", "text": "B"}}, "fontSize": float64(24)}); e == nil {
		t.Fatal("duplicate page targets accepted")
	}
}
func TestPhase3SlideAnimationProof(t *testing.T) {
	raw := `<p:timing xmlns:p="` + artifactSlideNS + `"><p:tnLst><p:par><p:cTn presetClass="entr" presetID="1" nodeType="clickEffect"><p:childTnLst><p:set><p:cBhvr><p:cTn dur="1"/><p:tgtEl><p:spTgt spid="7"/></p:tgtEl><p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst></p:cBhvr><p:to><p:strVal val="visible"/></p:to></p:set></p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`
	n, _ := parseArtifactXML([]byte(raw))
	if e := verifyPhase3AppearTiming(n, "7"); e != nil {
		t.Fatal(e)
	}
	for _, v := range [][2]string{{`spid="7"`, `spid="9"`}, {`presetClass="entr"`, `presetClass="exit"`}, {`val="visible"`, `val="hidden"`}, {`style.visibility`, `style.color`}} {
		x, _ := parseArtifactXML([]byte(strings.Replace(raw, v[0], v[1], 1)))
		if e := verifyPhase3AppearTiming(x, "7"); e == nil {
			t.Fatalf("accepted %v", v)
		}
	}
}

func TestPhase3SlideAnimationArtifactProof(t *testing.T) {
	fixture := func(size, timing, other string) string {
		return artifactTestZip(t, map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId r:id="one"/></p:sldIdLst></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/one.xml"/></Relationships>`,
			"ppt/slides/one.xml":              `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="7" name="title"/></p:nvSpPr><p:txBody><a:p><a:r><a:rPr sz="` + size + `"/><a:t>Title</a:t></a:r></a:p></p:txBody></p:sp><p:sp><p:nvSpPr><p:cNvPr id="9" name="body"/></p:nvSpPr><p:txBody><a:p><a:r><a:t>` + other + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld>` + timing + `</p:sld>`})
	}
	timing := `<p:timing><p:tnLst><p:par><p:cTn presetClass="entr" presetID="1" nodeType="clickEffect"><p:childTnLst><p:set><p:cBhvr><p:cTn dur="1"/><p:tgtEl><p:spTgt spid="7"/></p:tgtEl><p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst></p:cBhvr><p:to><p:strVal val="visible"/></p:to></p:set></p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`
	s := phase3SlidesSpec{Animation: &phase3SlideAnimationSpec{Targets: []phase3SlideAnimationTarget{{SlideID: "a", Text: "Title"}}, FontSize: 24}}
	before := fixture("1200", "", "keep")
	after := fixture("2400", timing, "keep")
	if e := verifyPhase3SlideAnimationArtifacts(before, after, []string{"a"}, []string{"a"}, s); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{fixture("1200", timing, "keep"), fixture("2400", timing, "changed"), fixture("2400", "", "keep")} {
		if e := verifyPhase3SlideAnimationArtifacts(before, bad, []string{"a"}, []string{"a"}, s); e == nil {
			t.Fatal("incomplete/collateral animation batch accepted")
		}
	}
	if e := phase3PrepareSlideAnimation(after, []string{"a"}, s); e == nil {
		t.Fatal("existing animation overwritten")
	}
}
