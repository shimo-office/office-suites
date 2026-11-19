package main

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestDocumentGraphicRuntimeRejectsWrongSelectionAndRevokedPermission(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("existing Node for browser-script guard tests")
	}
	s, _ := prepareDocumentGraphic(map[string]any{"name": "shape", "description": "marker", "rotationDegrees": float64(90)})
	baseline := documentGraphicNativeBaseline{Kind: "shape", WidthMM: 30, HeightMM: 20}
	for _, scenario := range []string{"normal", "duplicate", "revoke"} {
		mode, _ := json.Marshal(scenario)
		setup := `const mode=` + string(mode) + `;let calls=0;const shape={Id:'native-id',group:null,spPr:{xfrm:{extX:30,extY:20,rot:0}},getNvPr:()=>({name:'shape',descr:'marker'}),isShape:()=>true,isImage:()=>false};const drawing={graphicsObj:shape};shape.parent=drawing;let selected=[];const drawings={drawingPages:[{inlineObjects:[shape]}],resetSelection:()=>{selected=[]},selectObj:o=>{selected.push(o);if(mode==='revoke')api.isViewMode=true},getSelectedObjs:()=>selected};const model={children:[{getAllDrawingObjs:a=>{a.push(drawing);if(mode==='duplicate')a.push({graphicsObj:{...shape,Id:'other'}})}}],drawings,setCursorPosType:v=>{if(v!==3)throw Error('wrong native cursor mode')}};const api={isViewMode:false,controller:{docModel:model},applyImgProperties:p=>{calls++;shape.spPr.xfrm.rot=p.rot}};global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:['1']}),_sdk:{options:{user:{id:1}},editor:{docsApi:api,config:{mode:{role:'editor'}},canHandleChanges:true}}};`
		script := setup + `(async()=>{const result=await ` + documentGraphicRuntimeJS(s, baseline, true, "") + `;if(mode==='normal'){if(!result.ok||calls!==1||Math.abs(shape.spPr.xfrm.rot-Math.PI/2)>1e-9)throw Error('real selection/mutation missing')}else if(result.ok||calls!==0){throw Error('unsafe mutation reached')}})().catch(e=>{console.error(e);process.exitCode=1})`
		if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
			t.Fatalf("%s: %v %s", scenario, e, out)
		}
	}
}

func TestDocumentGraphicBaselineRejectsNonIntegralGeometry(t *testing.T) {
	const source = `<w:document xmlns:w="` + artifactWordNS + `" xmlns:wp="` + documentDrawingNS + `" xmlns:a="` + documentDrawingMainNS + `" xmlns:pic="` + documentPictureNS + `"><w:body><w:p><w:r><w:drawing><wp:inline><wp:extent cx="1080000" cy="720000"/><wp:docPr name="shape" descr="marker"/><a:graphic><a:graphicData><pic:pic><pic:spPr><a:xfrm rot="0"><a:ext cx="1080000" cy="720000"/></a:xfrm></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p></w:body></w:document>`
	spec := documentGraphicSpec{Name: "shape", Description: "marker"}
	root, err := parseArtifactXML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = documentGraphicBaseline(root, spec); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"NaN", "+Inf", "-Inf", "1.5", "1e6", "9223372036854775808"} {
		for _, attr := range []string{`cx="1080000"`, `cy="720000"`, `rot="0"`} {
			mutated := strings.ReplaceAll(source, attr, strings.Split(attr, "=")[0]+`="`+bad+`"`)
			root, err := parseArtifactXML([]byte(mutated))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = documentGraphicBaseline(root, spec); err == nil {
				t.Fatalf("accepted %s %s", attr, bad)
			}
		}
	}
}
