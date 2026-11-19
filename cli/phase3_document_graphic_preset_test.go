package main

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestDocumentGraphicPresetInput(t *testing.T) {
	p := map[string]any{"paragraph": float64(2), "expectedParagraph": "", "drawing": float64(1), "preset": float64(2)}
	if _, err := prepareDocumentGraphic(p); err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{float64(0), float64(7), float64(1.5), "2"} {
		q := map[string]any{}
		for k, x := range p {
			q[k] = x
		}
		q["preset"] = v
		if _, err := prepareDocumentGraphic(q); err == nil {
			t.Fatal("unknown gallery option accepted", v)
		}
	}
	p["fill"] = "#0000FF"
	if _, err := prepareDocumentGraphic(p); err == nil {
		t.Fatal("preset combined with override hides intermediate proof")
	}
}

func TestDocumentGraphicPresetRuntimeGuards(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	spec, err := prepareDocumentGraphic(map[string]any{"name": "shape", "description": "marker", "preset": float64(2)})
	if err != nil {
		t.Fatal(err)
	}
	baseline := documentGraphicNativeBaseline{Kind: "shape", WidthMM: 30, HeightMM: 20}
	for _, scenario := range []string{"normal", "preflight", "missing", "duplicate", "preview", "disabled", "gallery", "selection", "permission", "presence"} {
		t.Run(scenario, func(t *testing.T) {
			mode, _ := json.Marshal(scenario)
			setup := `const mode=` + string(mode) + `;let clicks=0;global.setTimeout=f=>{f()};const shape={Id:'native',group:null,spPr:{xfrm:{extX:30,extY:20}},getNvPr:()=>({name:'shape',descr:'marker'}),isShape:()=>true,isImage:()=>false};const drawing={graphicsObj:shape};shape.parent=drawing;let selected=[];let active=true;const drawings={drawingPages:[{inlineObjects:[shape]}],resetSelection:()=>{selected=[]},selectObj:o=>{selected=[mode==='selection'?{...o}:o];if(mode==='permission')api.isViewMode=true;if(mode==='presence')active=false},getSelectedObjs:()=>selected};const model={children:[{getAllDrawingObjs:a=>a.push(drawing)}],drawings,setCursorPosType:()=>{}};const api={isViewMode:false,controller:{docModel:model},applyImgProperties:()=>{throw Error('preset must click visible native option')}};global.window={__shimoPhase3Presence:()=>({ready:active,connected:active,ids:['1']}),_sdk:{options:{user:{id:1}},editor:{docsApi:api,config:{mode:{role:'editor'}},canHandleChanges:{}}}};const option={value:2,fill:'#FFFFFF',stroke:'#5C8BD9',font:'#2C3033',onClick:()=>{}};const gallery={memoizedProps:{name:'shape-style',disabled:mode==='disabled',items:[mode==='gallery'?{...option,stroke:'#000000'}:option]}};const li={textContent:'Abc',getClientRects:()=>[{}],querySelector:()=>({}),click:()=>{clicks++},__reactFiber_test:{memoizedProps:option,return:gallery}};const root={getClientRects:()=>[{}],querySelectorAll:()=>mode==='missing'?[]:mode==='duplicate'?[li,li]:[li]};global.document={querySelectorAll:()=>[root]};global.getComputedStyle=()=>({backgroundColor:mode==='preview'?'rgb(0, 0, 0)':'rgb(255, 255, 255)',borderTopColor:'rgb(92, 139, 217)',color:'rgb(44, 48, 51)'});`
			script := setup + `(async()=>{const result=await ` + documentGraphicRuntimeScript(spec, baseline, scenario != "preflight", scenario == "preflight", "") + `;if(mode==='normal'){if(!result.ok||clicks!==1||result.presetOption.value!==2)throw Error(JSON.stringify({result,clicks}))}else if(mode==='preflight'){if(!result.ok||clicks!==0||result.availablePresets.length!==1)throw Error('selection-only probe failed')}else if(result.ok||clicks!==0)throw Error('unsafe click '+mode)})().catch(e=>{console.error(e);process.exitCode=1})`
			if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
				t.Fatalf("%v %s", e, out)
			}
		})
	}
}

func TestDocumentGraphicPresetArtifactProof(t *testing.T) {
	doc := `<w:document xmlns:w="` + artifactWordNS + `" xmlns:wp="` + documentDrawingNS + `" xmlns:a="` + documentDrawingMainNS + `" xmlns:wps="` + documentShapeNS + `"><w:body><w:p><w:r><w:drawing><wp:inline><wp:extent cx="1080000" cy="720000"/><wp:docPr id="1" name="shape" descr="marker"/><a:graphic><a:graphicData><wps:wsp><wps:spPr><a:xfrm><a:ext cx="1080000" cy="720000"/></a:xfrm><a:prstGeom prst="rect"/><a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill><a:ln w="12700"><a:solidFill><a:srgbClr val="5C8BD9"/></a:solidFill><a:prstDash val="solid"/></a:ln></wps:spPr><wps:style><a:lnRef idx="1"><a:schemeClr val="accent1"/></a:lnRef><a:fontRef idx="minor"><a:srgbClr val="2C3033"/></a:fontRef></wps:style><wps:bodyPr/></wps:wsp></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p><w:p><w:r><w:t>untouched</w:t></w:r></w:p></w:body></w:document>`
	zip := func(xml, asset string) string {
		return artifactTestZip(t, map[string]string{"word/document.xml": xml, "word/media/image.png": asset})
	}
	beforeXML := strings.NewReplacer(`val="FFFFFF"`, `val="FF0000"`, `val="5C8BD9"`, `val="000000"`, `<a:srgbClr val="2C3033"/>`, `<a:schemeClr val="lt1"/>`).Replace(doc)
	before := zip(beforeXML, "original")
	spec, _ := prepareDocumentGraphic(map[string]any{"name": "shape", "description": "marker", "preset": float64(2)})
	if err := verifyDocumentGraphic(before, zip(doc, "original"), spec); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range [][2]string{{`val="FFFFFF"`, `val="FF0000"`}, {`val="5C8BD9"`, `val="000000"`}, {`val="2C3033"`, `val="FF0000"`}, {`idx="minor"`, `idx="major"`}, {`w="12700"`, `w="25400"`}, {`<a:lnRef idx="1">`, `<a:lnRef idx="2">`}, {`name="shape"`, `name="other"`}, {`untouched`, `changed`}, {`<a:srgbClr val="2C3033"/>`, `<a:srgbClr val="2C3033"><a:alpha val="50000"/></a:srgbClr>`}} {
		if verifyDocumentGraphic(before, zip(strings.Replace(doc, mutation[0], mutation[1], 1), "original"), spec) == nil {
			t.Fatal("accepted mutation", mutation)
		}
	}
	if verifyDocumentGraphic(before, zip(doc, "changed"), spec) == nil {
		t.Fatal("accepted non-target asset mutation")
	}
}
