package main

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestDocumentGraphicPositionInput(t *testing.T) {
	good := map[string]any{"paragraph": float64(2), "expectedParagraph": "", "drawing": float64(1), "rotationDegrees": float64(90)}
	if _, err := prepareDocumentGraphic(good); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"paragraph", "expectedParagraph", "drawing"} {
		bad := map[string]any{}
		for k, v := range good {
			if k != key {
				bad[k] = v
			}
		}
		if _, err := prepareDocumentGraphic(bad); err == nil {
			t.Fatalf("accepted incomplete selector %s", key)
		}
	}
	for _, bad := range []map[string]any{
		{"paragraph": float64(0), "expectedParagraph": "", "drawing": float64(1), "rotationDegrees": float64(90)},
		{"paragraph": float64(2), "expectedParagraph": "", "drawing": float64(1.5), "rotationDegrees": float64(90)},
		{"paragraph": float64(2), "expectedParagraph": "", "drawing": float64(1), "name": "ambiguous", "description": "mixed", "rotationDegrees": float64(90)},
	} {
		if _, err := prepareDocumentGraphic(bad); err == nil {
			t.Fatal("accepted invalid position", bad)
		}
	}
}

func TestDocumentGraphicPositionBaselineAndProof(t *testing.T) {
	drawing := `<w:drawing><wp:inline><wp:extent cx="1080000" cy="720000"/><wp:docPr name=""/><a:graphic><a:graphicData><wps:wsp><wps:spPr><a:xfrm><a:ext cx="1080000" cy="720000"/></a:xfrm><a:prstGeom prst="rect"/></wps:spPr></wps:wsp></a:graphicData></a:graphic></wp:inline></w:drawing>`
	doc := `<w:document xmlns:w="` + artifactWordNS + `" xmlns:wp="` + documentDrawingNS + `" xmlns:a="` + documentDrawingMainNS + `" xmlns:wps="` + documentShapeNS + `"><w:body><w:p><w:r><w:t>heading</w:t></w:r></w:p><w:p><w:r>` + drawing + `</w:r></w:p><w:p><w:r><w:t>control</w:t></w:r></w:p></w:body></w:document>`
	root, err := parseArtifactXML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	s, err := prepareDocumentGraphic(map[string]any{"paragraph": float64(2), "expectedParagraph": "", "drawing": float64(1), "rotationDegrees": float64(90)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := documentGraphicBaseline(root, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Paragraphs) != 3 || b.Paragraphs[0].Text != "heading" || b.Paragraphs[1].Drawings[0].Preset != "rect" {
		t.Fatalf("incorrect positional baseline %#v", b)
	}
	before := artifactTestZip(t, map[string]string{"word/document.xml": doc})
	afterXML := strings.Replace(doc, "<a:xfrm>", `<a:xfrm rot="5400000">`, 1)
	after := artifactTestZip(t, map[string]string{"word/document.xml": afterXML})
	if err = verifyDocumentGraphic(before, after, s); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(afterXML, "control", "changed", 1), strings.Replace(afterXML, `name=""`, `name="lost-other-metadata"`, 1), strings.Replace(afterXML, `prst="rect"`, `prst="rightArrow"`, 1)} {
		if verifyDocumentGraphic(before, artifactTestZip(t, map[string]string{"word/document.xml": bad}), s) == nil {
			t.Fatal("collateral change accepted")
		}
	}
	s.ExpectedParagraph = "wrong"
	if _, err = documentGraphicBaseline(root, s); err == nil {
		t.Fatal("wrong exact paragraph accepted")
	}
	s.ExpectedParagraph = ""
	s.Drawing = 2
	if _, err = documentGraphicBaseline(root, s); err == nil {
		t.Fatal("missing drawing accepted")
	}
}

func TestDocumentGraphicPositionRuntimeGuards(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	spec, _ := prepareDocumentGraphic(map[string]any{"paragraph": float64(2), "expectedParagraph": "", "drawing": float64(1), "rotationDegrees": float64(90)})
	baseline := documentGraphicNativeBaseline{Kind: "shape", WidthMM: 30, HeightMM: 20, Paragraphs: []documentGraphicPositionParagraph{{Text: "heading", Drawings: []documentGraphicGeometry{}}, {Text: "", Drawings: []documentGraphicGeometry{{Kind: "shape", Preset: "rect", WidthMM: 30, HeightMM: 20}}}}}
	for _, scenario := range []string{"normal", "text", "count", "type", "geometry", "shape", "duplicate", "permission", "selection"} {
		mode, _ := json.Marshal(scenario)
		setup := `const mode=` + string(mode) + `;let calls=0;const shape={Id:'native',group:null,spPr:{geometry:{preset:'rect'},xfrm:{extX:30,extY:20}},getNvPr:()=>null,isShape:()=>true,isImage:()=>false};const drawing={graphicsObj:shape};shape.parent=drawing;let selected=[];const ps=[{text:'heading',getAllDrawingObjs:a=>{}},{text:'',getAllDrawingObjs:a=>a.push(drawing)}];if(mode==='text')ps[0].text='changed';if(mode==='count')ps.push({text:'',getAllDrawingObjs:a=>{}});if(mode==='type')shape.isShape=()=>false;if(mode==='geometry')shape.spPr.xfrm.extX=31;if(mode==='shape')shape.spPr.geometry.preset='rightArrow';if(mode==='duplicate')ps[0].getAllDrawingObjs=a=>a.push(drawing);const drawings={drawingPages:[{inlineObjects:[shape]}],resetSelection:()=>{selected=[]},selectObj:o=>{selected.push(mode==='selection'?{...o}:o);if(mode==='permission')api.isViewMode=true},getSelectedObjs:()=>selected};const model={children:ps,drawings,setCursorPosType:()=>{}};const api={isViewMode:false,controller:{docModel:model},applyImgProperties:p=>{calls++;shape.spPr.xfrm.rot=p.rot}};const editor={docsApi:api,getParagraphRange:p=>({getText:async()=>p.text+'\r\n'}),config:{mode:{role:'editor'}},canHandleChanges:true};global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:['1']}),_sdk:{options:{user:{id:1}},editor}};`
		script := setup + `(async()=>{const result=await ` + documentGraphicRuntimeJS(spec, baseline, true, "") + `;if(mode==='normal'){if(!result.ok||calls!==1)throw Error(JSON.stringify(result));const after=await ` + documentGraphicRuntimeJS(spec, baseline, false, "native") + `;if(!after.ok)throw Error(JSON.stringify(after))}else if(result.ok||calls)throw Error('unsafe mutation reached '+JSON.stringify(result))})().catch(e=>{console.error(e);process.exitCode=1})`
		if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", scenario, err, out)
		}
	}
}
