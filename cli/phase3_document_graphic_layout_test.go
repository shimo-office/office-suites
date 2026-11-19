package main

import (
	"os/exec"
	"strings"
	"testing"
)

func graphicLayoutNode(t *testing.T, kind, attrs, layout string) *artifactNode {
	t.Helper()
	n, e := parseArtifactXML([]byte(`<w:drawing xmlns:w="` + artifactWordNS + `" xmlns:wp="` + documentDrawingNS + `" xmlns:a="` + documentDrawingMainNS + `"><wp:` + kind + ` ` + attrs + `>` + layout + `<wp:extent cx="1080000" cy="720000"/><wp:docPr id="1" name="image" descr="fixture"/><a:graphic><a:graphicData/></a:graphic></wp:` + kind + `></w:drawing>`))
	if e != nil {
		t.Fatal(e)
	}
	return n
}

const graphicAnchorAttrs = `distT="0" distB="0" distL="115200" distR="115200" simplePos="0" relativeHeight="1" behindDoc="0" locked="0" layoutInCell="1" allowOverlap="1"`
const graphicAnchorPosition = `<wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="margin"><wp:align>center</wp:align></wp:positionH><wp:positionV relativeFrom="paragraph"><wp:posOffset>0</wp:posOffset></wp:positionV>`

func TestDocumentGraphicLayoutInput(t *testing.T) {
	s := documentGraphicSpec{}
	if e := prepareDocumentGraphicLayout(map[string]any{"wrap": "square", "anchor": "paragraph", "horizontalAlign": "center", "horizontalRelativeTo": "margin"}, &s); e != nil {
		t.Fatal(e)
	}
	for _, p := range []map[string]any{{"wrap": ""}, {"wrap": "bogus"}, {"anchor": "line"}, {"horizontalAlign": "center"}, {"horizontalRelativeTo": "margin"}, {"wrap": "inline", "anchor": "page"}} {
		s := documentGraphicSpec{}
		if prepareDocumentGraphicLayout(p, &s) == nil {
			t.Fatal("invalid input accepted", p)
		}
	}
}
func TestDocumentGraphicLayoutFloatingProofProtectsUnrequestedPosition(t *testing.T) {
	before := graphicLayoutNode(t, "anchor", graphicAnchorAttrs, graphicAnchorPosition+`<wp:wrapTopAndBottom/>`)
	after := graphicLayoutNode(t, "anchor", graphicAnchorAttrs, graphicAnchorPosition+`<wp:wrapSquare wrapText="bothSides"/>`)
	s := documentGraphicSpec{Wrap: "square"}
	if e := verifyDocumentGraphicLayout(before, after, s); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []*artifactNode{
		graphicLayoutNode(t, "anchor", strings.Replace(graphicAnchorAttrs, `allowOverlap="1"`, `allowOverlap="0"`, 1), graphicAnchorPosition+`<wp:wrapSquare wrapText="bothSides"/>`),
		graphicLayoutNode(t, "anchor", graphicAnchorAttrs, strings.Replace(graphicAnchorPosition, `relativeFrom="paragraph"`, `relativeFrom="page"`, 1)+`<wp:wrapSquare wrapText="bothSides"/>`),
		graphicLayoutNode(t, "anchor", strings.Replace(graphicAnchorAttrs, `distL="115200"`, `distL="1"`, 1), graphicAnchorPosition+`<wp:wrapSquare wrapText="bothSides"/>`),
	} {
		if verifyDocumentGraphicLayout(before, bad, s) == nil {
			t.Fatal("unrequested layout mutation accepted")
		}
	}
}
func TestDocumentGraphicLayoutInlineConversionRequiresExactDefaults(t *testing.T) {
	before := graphicLayoutNode(t, "inline", `distT="0" distB="0" distL="0" distR="0"`, "")
	after := graphicLayoutNode(t, "anchor", graphicAnchorAttrs, graphicAnchorPosition+`<wp:wrapSquare wrapText="bothSides"/>`)
	s := documentGraphicSpec{Wrap: "square", Anchor: "paragraph", HorizontalAlign: "center", HorizontalRelativeTo: "margin"}
	if e := verifyDocumentGraphicLayout(before, after, s); e != nil {
		t.Fatal(e)
	}
	if e := normalizeDocumentGraphicLayout(before, s); e != nil {
		t.Fatal(e)
	}
	if e := normalizeDocumentGraphicLayout(after, s); e != nil {
		t.Fatal(e)
	}
	if layoutNodeCanonical(before) != layoutNodeCanonical(after) {
		t.Fatal("expected wrapper transition not normalized")
	}
	bad := graphicLayoutNode(t, "anchor", strings.Replace(graphicAnchorAttrs, `locked="0"`, `locked="1"`, 1), graphicAnchorPosition+`<wp:wrapSquare wrapText="bothSides"/>`)
	initial := graphicLayoutNode(t, "inline", `distT="0" distB="0" distL="0" distR="0"`, "")
	if verifyDocumentGraphicLayout(initial, bad, s) == nil {
		t.Fatal("arbitrary anchor default accepted")
	}
	if verifyDocumentGraphicLayout(initial, bad, documentGraphicSpec{Wrap: "square"}) == nil {
		t.Fatal("implicit coordinate conversion accepted")
	}
}

func TestDocumentGraphicLayoutRejectsIncorrectCoordinatesAndUnknownInlineFlags(t *testing.T) {
	initial := graphicLayoutNode(t, "inline", `distT="0" distB="0" distL="0" distR="0"`, "")
	spec := documentGraphicSpec{Wrap: "square", Anchor: "paragraph", HorizontalAlign: "center", HorizontalRelativeTo: "margin"}
	for _, layout := range []string{
		strings.Replace(graphicAnchorPosition, `<wp:align>center</wp:align>`, `<wp:posOffset>0</wp:posOffset>`, 1) + `<wp:wrapSquare wrapText="bothSides"/>`,
		strings.Replace(graphicAnchorPosition, `<wp:posOffset>0</wp:posOffset>`, `<wp:posOffset>12</wp:posOffset>`, 1) + `<wp:wrapSquare wrapText="bothSides"/>`,
		graphicAnchorPosition + `<wp:wrapSquare wrapText="left"/>`,
	} {
		if verifyDocumentGraphicLayout(initial, graphicLayoutNode(t, "anchor", graphicAnchorAttrs, layout), spec) == nil {
			t.Fatal("wrong layout accepted")
		}
	}
	unknown := graphicLayoutNode(t, "inline", `customFlag="preserve"`, "")
	after := graphicLayoutNode(t, "anchor", graphicAnchorAttrs, graphicAnchorPosition+`<wp:wrapSquare wrapText="bothSides"/>`)
	if verifyDocumentGraphicLayout(unknown, after, spec) == nil {
		t.Fatal("unknown inline attribute silently lost")
	}
}

func TestDocumentGraphicLayoutJSUsesNativePropertiesAndStopsUnsafeConversion(t *testing.T) {
	node, e := exec.LookPath("node")
	if e != nil {
		t.Skip("node unavailable")
	}
	spec := documentGraphicSpec{Wrap: "square", Anchor: "paragraph", HorizontalAlign: "center", HorizontalRelativeTo: "margin"}
	setup := `const props={},calls=[],api={setShapeAlign:(a,b)=>calls.push([a,b])},drawing={isThisInline:()=>true,getDistance:()=>({L:0,R:0,T:0,B:0}),getProps:()=>({canFlow:true})};`
	js := setup + documentGraphicLayoutPreflightJS(spec) + documentGraphicLayoutMutationJS(spec) + `if(JSON.stringify(props)!==JSON.stringify({wrappingType:1,posV:{relativeFrom:6,UseAlign:false,Value:0,Percent:false}})||JSON.stringify(calls)!=='[[4,3]]')throw Error('native payload mismatch');`
	if out, e := exec.Command(node, "-e", js).CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, out)
	}
	for _, scenario := range []struct {
		spec documentGraphicSpec
		flow string
	}{{documentGraphicSpec{Wrap: "square"}, "true"}, {spec, "false"}} {
		js := strings.Replace(setup, "canFlow:true", "canFlow:"+scenario.flow, 1) + `let stopped=false;try{` + documentGraphicLayoutPreflightJS(scenario.spec) + `}catch(e){stopped=true}if(!stopped||Object.keys(props).length||calls.length)throw Error('unsafe preflight did not stop');`
		if out, e := exec.Command(node, "-e", js).CombinedOutput(); e != nil {
			t.Fatalf("%v %s", e, out)
		}
	}
}

func TestDocumentGraphicLayoutInlineDistancesRejectedBeforeWriteAndProof(t *testing.T) {
	s := documentGraphicSpec{Wrap: "square", Anchor: "paragraph", HorizontalAlign: "center", HorizontalRelativeTo: "margin"}
	after := graphicLayoutNode(t, "anchor", graphicAnchorAttrs, graphicAnchorPosition+`<wp:wrapSquare wrapText="bothSides"/>`)
	for _, value := range []string{"360000", "-1", "NaN", "Inf", "0.5", ""} {
		before := graphicLayoutNode(t, "inline", `distT="`+value+`" distB="0" distL="0" distR="0"`, "")
		if documentGraphicLayoutBaseline(before, s) == nil {
			t.Fatal("unsafe distance accepted before write", value)
		}
		if verifyDocumentGraphicLayout(before, after, s) == nil {
			t.Fatal("unsafe distance discarded in proof", value)
		}
	}
	for _, attrs := range []string{"", `distT="0" distB="0" distL="0" distR="0"`} {
		if e := documentGraphicLayoutBaseline(graphicLayoutNode(t, "inline", attrs, ""), s); e != nil {
			t.Fatal(e)
		}
	}
}

func TestDocumentGraphicLayoutNativeDistanceGuardPrecedesProps(t *testing.T) {
	node, e := exec.LookPath("node")
	if e != nil {
		t.Skip("node unavailable")
	}
	spec := documentGraphicSpec{Wrap: "square", Anchor: "paragraph", HorizontalAlign: "center", HorizontalRelativeTo: "margin"}
	for _, value := range []string{"10", "NaN", "Infinity", "0.5"} {
		js := `const props={},api={setShapeAlign:()=>{throw Error('unexpected')}},drawing={isThisInline:()=>true,getProps:()=>({canFlow:true}),getDistance:()=>({L:0,R:0,T:` + value + `,B:0})};let stopped=false;try{` + documentGraphicLayoutPreflightJS(spec) + `}catch(e){stopped=true}if(!stopped||Object.keys(props).length)throw Error('unsafe distance not rejected');`
		if out, e := exec.Command(node, "-e", js).CombinedOutput(); e != nil {
			t.Fatalf("%v %s", e, out)
		}
	}
}
