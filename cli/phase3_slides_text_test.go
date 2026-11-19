package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestPhase3SlideTextPayload(t *testing.T) {
	p := map[string]any{"slideId": "s1", "text": "unique text", "x": float64(10), "y": float64(20), "width": float64(300), "height": float64(80), "fontSize": float64(24), "bold": true}
	s, e := preparePhase3Slides("slide-text-insert", p)
	if e != nil {
		t.Fatal(e)
	}
	if s.Text == nil || s.Text.FontSize != 24 || !s.Text.Bold {
		t.Fatal("incorrect spec")
	}
	for _, key := range []string{"text", "x", "width", "fontSize", "bold"} {
		q := map[string]any{}
		for k, v := range p {
			q[k] = v
		}
		q[key] = nil
		if _, e := preparePhase3Slides("slide-text-insert", q); e == nil {
			t.Errorf("accepted invalid %s", key)
		}
	}
	for _, v := range []string{"", "with\nnewline", "bad\x00"} {
		q := map[string]any{}
		for k, w := range p {
			q[k] = w
		}
		q["text"] = v
		if _, e := preparePhase3Slides("slide-text-insert", q); e == nil {
			t.Errorf("accepted text %q", v)
		}
	}
}
func TestPhase3SlideTextNodeProof(t *testing.T) {
	raw := `<p:sp xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:nvSpPr><p:cNvPr id="4" name=""/></p:nvSpPr><p:spPr><a:xfrm><a:off x="95250" y="190500"/><a:ext cx="2857500" cy="762000"/></a:xfrm></p:spPr><p:txBody><a:p><a:r><a:rPr sz="2400" b="1"/><a:t>unique text</a:t></a:r></a:p></p:txBody></p:sp>`
	s := phase3SlideTextSpec{Text: "unique text", X: 10, Y: 20, Width: 300, Height: 80, FontSize: 24, Bold: true}
	n, e := parseArtifactXML([]byte(raw))
	if e != nil {
		t.Fatal(e)
	}
	if e = verifyPhase3SlideTextNode(n, s); e != nil {
		t.Fatal(e)
	}
	for _, change := range [][2]string{{`sz="2400"`, `sz="1200"`}, {`b="1"`, `b="0"`}, {`cx="2857500"`, `cx="10"`}, {`unique text`, `wrong text`}, {`id="4" name=""`, `id="4" name="" hidden="1"`}} {
		n, e = parseArtifactXML([]byte(strings.ReplaceAll(raw, change[0], change[1])))
		if e != nil {
			t.Fatal(e)
		}
		if e = verifyPhase3SlideTextNode(n, s); e == nil {
			t.Fatalf("accepted wrong node %v", change)
		}
	}
}

func TestPhase3SlideTextArtifactDifference(t *testing.T) {
	original := `<p:sp><p:nvSpPr><p:cNvPr id="1" name="old"/></p:nvSpPr><p:txBody><a:p><a:r><a:t>original</a:t></a:r></a:p></p:txBody></p:sp>`
	inserted := `<p:sp><p:nvSpPr><p:cNvPr id="4" name=""/></p:nvSpPr><p:spPr><a:xfrm><a:off x="95250" y="190500"/><a:ext cx="2857500" cy="762000"/></a:xfrm></p:spPr><p:txBody><a:p><a:r><a:rPr sz="2400" b="1"/><a:t>unique text</a:t></a:r></a:p></p:txBody></p:sp>`
	fixture := func(shapes string) string {
		return artifactTestZip(t, map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId id="256" r:id="one"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/slide1.xml"/></Relationships>`,
			"ppt/slides/slide1.xml":           `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree>` + shapes + `</p:spTree></p:cSld></p:sld>`,
		})
	}
	spec := phase3SlidesSpec{Action: "slide-text-insert", SlideID: "s1", Text: &phase3SlideTextSpec{Text: "unique text", X: 10, Y: 20, Width: 300, Height: 80, FontSize: 24, Bold: true}}
	before := fixture(original)
	if e := phase3SlideTextBounds(before, *spec.Text); e != nil {
		t.Fatal(e)
	}
	badBounds := *spec.Text
	badBounds.X = 1500
	if e := phase3SlideTextBounds(before, badBounds); e == nil {
		t.Fatal("out of bounds accepted")
	}
	for _, c := range []struct {
		body string
		pass bool
	}{{original + inserted, true}, {original, false}, {original + inserted + inserted, false}, {strings.ReplaceAll(original, "original", "changed") + inserted, false}, {original + strings.ReplaceAll(inserted, `b="1"`, `b="0"`), false}} {
		e := verifyPhase3SlideTextArtifacts(before, fixture(c.body), []string{"s1"}, []string{"s1"}, spec)
		if (e == nil) != c.pass {
			t.Errorf("pass=%t err=%v", c.pass, e)
		}
	}
	if e := verifyPhase3SlideTextArtifacts(fixture(original+inserted), fixture(original+inserted), []string{"s1"}, []string{"s1"}, spec); e == nil {
		t.Fatal("existing text claimed as insertion")
	}
	bad := spec
	bad.SlideID = "missing"
	if e := verifyPhase3SlideTextArtifacts(before, before, []string{"s1"}, []string{"s1"}, bad); e == nil {
		t.Fatal("missing target accepted")
	}
}
func TestPhase3SlideTextMutationOneShot(t *testing.T) {
	node, e := exec.LookPath("node")
	if e != nil {
		t.Skip("node only for JS regression tests")
	}
	s := phase3SlidesSpec{Action: "slide-text-insert", SlideID: "s1", Text: &phase3SlideTextSpec{Text: "unique' text", X: 10, Y: 20, Width: 300, Height: 80, FontSize: 24}}
	text, _ := json.Marshal(s.Text.Text)
	for _, edit := range []bool{false, true} {
		expected := 0
		if edit {
			expected = 1
		}
		script := fmt.Sprintf(`let calls=0;const shapes=[{id:'old',textContent:'before'}];const slide={id:'s1',getShapes:()=>shapes,insertTextBox:async p=>{calls++;const shape={id:'new',textContent:p.content[0].text};shapes.push(shape);return shape}};global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:[1]}),presentationSDK:{editor:{workbench:{isReady:()=>true,permissions:{canEdit:()=>%t},context:{getUser:()=>({id:1})}}},API:{slides:{getSlides:()=>[slide]}}}};(async()=>{const result=await %s;if(calls!==%d)throw new Error('write count');if(calls===1&&(result.shapeId!=='new'||shapes[1].textContent!==%s))throw new Error('bad inserted shape');if(calls===0&&result.ok!==false)throw new Error('guard failure')})().catch(e=>{console.error(e);process.exit(1)})`, edit, phase3SlideTextMutationJS(s, []string{"s1"}), expected, string(text))
		if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
			t.Fatalf("%v %s", e, out)
		}
	}
}
