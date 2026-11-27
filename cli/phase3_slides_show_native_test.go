package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPhase3SlideShowNativePersistenceAndCollateral(t *testing.T) {
	loop, timing := true, false
	s := phase3SlideShowSpec{LoopShow: &loop, UseTiming: &timing}
	state := func(loop, timing bool, pr map[string]any) map[string]any {
		return map[string]any{"target": "https://shimo.net/presentation/test", "loopShow": loop, "useTiming": timing, "nativePresentationPr": pr, "persistenceSource": "collaboration.PresentationPr"}
	}
	b := state(false, true, map[string]any{"sectionLst": "keep"})
	a := state(true, false, map[string]any{"sectionLst": "keep", "showPr": map[string]any{"present": false, "loop": true, "useTimings": false}})
	if e := phase3VerifyNativeShowState(b, a, s); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []map[string]any{
		b,
		state(true, false, map[string]any{"sectionLst": "changed", "showPr": map[string]any{"loop": true, "useTimings": false}}),
		state(true, false, map[string]any{"sectionLst": "keep", "showPr": map[string]any{"loop": true, "useTimings": false, "showAnimation": false}}),
		state(true, false, map[string]any{"sectionLst": "keep"}),
	} {
		if e := phase3VerifyNativeShowState(b, bad, s); e == nil {
			t.Fatal("unpersisted/collateral state accepted", bad)
		}
	}
	partial := phase3SlideShowSpec{LoopShow: &loop}
	if e := phase3VerifyNativeShowState(b, a, partial); e == nil {
		t.Fatal("unrequested timing change accepted")
	}
}

func TestPhase3SlideShowArtifactProofAllowsUnchangedExportOnly(t *testing.T) {
	b := phase3PlaceholderBWArtifact(t, `<a:r><a:t>keep</a:t></a:r>`, "", "", "")
	loop, timing := true, false
	s := phase3SlidesSpec{Show: &phase3SlideShowSpec{LoopShow: &loop, UseTiming: &timing}}
	if e := verifyPhase3SlideShowArtifacts(b, b, []string{"a"}, []string{"a"}, s); e != nil {
		t.Fatal("unchanged exporter is collateral proof, not native success", e)
	}
	bad := phase3PlaceholderBWArtifact(t, `<a:r><a:t>changed</a:t></a:r>`, "", "", "")
	if e := verifyPhase3SlideShowArtifacts(b, bad, []string{"a"}, []string{"a"}, s); e == nil {
		t.Fatal("collateral text change accepted")
	}
}

func TestPhase3SlideShowRejectsWrongTarget(t *testing.T) {
	loop := true
	s := phase3SlideShowSpec{LoopShow: &loop}
	before := map[string]any{"target": "https://shimo.net/presentation/test", "loopShow": false, "useTiming": true, "nativePresentationPr": map[string]any{}, "persistenceSource": "collaboration.PresentationPr"}
	for _, target := range []string{"https://shimo.net/presentation/other", "https://other.net/presentation/test", ""} {
		after := map[string]any{"target": target, "loopShow": true, "useTiming": true, "nativePresentationPr": map[string]any{"showPr": map[string]any{"loop": true}}, "persistenceSource": "collaboration.PresentationPr"}
		if err := phase3VerifyNativeShowState(before, after, s); err == nil {
			t.Fatal("wrong target accepted", target)
		}
		if err := phase3BindNativeShow(before, target, &s); err == nil {
			t.Fatal("unbound before accepted", target)
		}
	}
}
func TestPhase3SlideShowMutationRequiresUnchangedBaseline(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional Node regression runtime")
	}
	for _, mode := range []string{"ready", "missing", "changed", "metadata", "target", "noop"} {
		t.Run(mode, func(t *testing.T) {
			loop := true
			before := map[string]any{"target": "https://shimo.net/presentation/test", "loopShow": false, "useTiming": true, "nativePresentationPr": map[string]any{}}
			if mode == "missing" {
				before = nil
			}
			if mode == "noop" {
				before["loopShow"] = true
				before["nativePresentationPr"] = map[string]any{"showPr": map[string]any{"loop": true}}
			}
			s := phase3SlidesSpec{Show: &phase3SlideShowSpec{LoopShow: &loop, NativeBefore: before}}
			script := fmt.Sprintf(`const mode=%q;let writes=0,loop=mode==='changed'||mode==='noop';const properties=mode==='metadata'?{sectionLst:'changed'}:mode==='noop'?{showPr:{loop:true}}:{};global.location={origin:'https://shimo.net',pathname:mode==='target'?'/presentation/other':'/presentation/test'};const editor={_$collaborationManager$_:{_$currentPptIOData$_:()=>[]},_$ioAdapter$_:{fromOperations:()=>({toJSON:()=>[{attributes:{opType:'presentation'},data:{delta:[{attributes:{opType:'PresentationPr'},data:{object:properties}}]}}]})}};const core={isLoopShow:()=>loop,isUseTiming:()=>true,setLoopShow(e,v){writes++;loop=v},setUseTiming(){writes++}};global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:[1]}),presentationSDK:{editor:{workbench:{isReady:()=>true,permissions:{canEdit:()=>true},context:{getUser:()=>({id:1})},editor,core}},API:{slides:{getSlides:()=>[{id:'a'}]}}}};const r=%s;if(mode==='ready'){if(r.ok!==true||writes!==1)throw Error(JSON.stringify({r,writes}))}else if(r.ok!==false||writes!==0)throw Error(JSON.stringify({r,writes}));`, mode, phase3SlideShowMutationJS(s, []string{"a"}))
			if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
				t.Fatalf("%v\n%s", e, out)
			}
		})
	}
}

func TestPhase3SlideShowIndependentSetters(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional Node regression runtime")
	}
	for _, property := range []string{"loopShow", "useTiming"} {
		for _, missing := range []string{"unrequested-setter", "requested-setter", "reader"} {
			t.Run(property+"/"+missing, func(t *testing.T) {
				s, err := preparePhase3SlideShow(map[string]any{property: property == "loopShow"})
				if err != nil {
					t.Fatal(err)
				}
				s.NativeBefore = map[string]any{"target": "https://shimo.net/presentation/test", "loopShow": false, "useTiming": true, "nativePresentationPr": map[string]any{}}
				script := fmt.Sprintf(`
let writes=0,loop=false,timing=true;
global.location={origin:'https://shimo.net',pathname:'/presentation/test'};
const editor={_$collaborationManager$_:{_$currentPptIOData$_:()=>[]},_$ioAdapter$_:{fromOperations:()=>({toJSON:()=>[{attributes:{opType:'presentation'},data:{delta:[]}}]})}};
const core={isLoopShow:()=>loop,isUseTiming:()=>timing,setLoopShow(e,v){writes++;loop=v},setUseTiming(e,v){writes++;timing=v}};
const property=%q,missing=%q,requested=property==='loopShow'?'setLoopShow':'setUseTiming',other=property==='loopShow'?'setUseTiming':'setLoopShow';
delete core[missing==='unrequested-setter'?other:missing==='requested-setter'?requested:'isUseTiming'];
global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:[1]}),presentationSDK:{editor:{workbench:{isReady:()=>true,permissions:{canEdit:()=>true},context:{getUser:()=>({id:1})},editor,core}},API:{slides:{getSlides:()=>[{id:'a'}]}}}};
const result=%s;
if(missing==='unrequested-setter'){
 if(result.ok!==true||writes!==1||loop!==(property==='loopShow')||timing!==(property==='loopShow'))throw Error(JSON.stringify({result,writes,loop,timing}));
}else if(result.ok!==false||writes!==0)throw Error(JSON.stringify({result,writes}));`, property, missing, phase3SlideShowMutationJS(phase3SlidesSpec{Show: s}, []string{"a"}))
				if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
					t.Fatalf("%v\n%s", err, out)
				}
			})
		}
	}
}

func TestPhase3SlideShowNativeResetUsesSavedState(t *testing.T) {
	loop, timing := false, true
	state := map[string]any{"target": "https://shimo.net/presentation/test", "loopShow": true, "useTiming": false, "persistenceSource": "collaboration.PresentationPr", "nativePresentationPr": map[string]any{"showPr": map[string]any{"loop": true, "useTimings": false}}}
	s := phase3SlideShowSpec{LoopShow: &loop, UseTiming: &timing}
	if err := phase3BindNativeShow(state, "https://shimo.net/files/test?dbg=1", &s); err != nil {
		t.Fatal("native reset must not be blocked by stale exporter defaults", err)
	}
	if s.NativeBefore == nil {
		t.Fatal("baseline not captured")
	}
	if _, err := phase3NativeShowProperties(state); err != nil {
		t.Fatal(err)
	}
	if _, ok := state["nativePresentationPr"].(map[string]any)["showPr"].(map[string]any)["present"]; ok {
		t.Fatal("input mutated")
	}
}

func TestPhase3SlidesRecordedShowIOReadback(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_SLIDES_SHOW_IO")
	if source == "" {
		t.Skip("opt-in recorded server IO regression")
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`const saved=%s;const url=new URL(saved.target);global.location={origin:url.origin,pathname:url.pathname};global.window={presentationSDK:{editor:{workbench:{isReady:()=>true,core:{isLoopShow:()=>saved.nativeShowPr.loop,isUseTiming:()=>saved.nativeShowPr.useTimings},editor:{_$collaborationManager$_:{_$currentPptIOData$_:()=>[]},_$ioAdapter$_:{fromOperations:()=>({toJSON:()=>saved.presentation})}}}}}};process.stdout.write(JSON.stringify(%s));`, raw, phase3SlideShowReadJS())
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("%v %s", err, out)
	}
	var state map[string]any
	if err = json.Unmarshal(out, &state); err != nil {
		t.Fatal(err)
	}
	if state["ok"] != true || state["loopShow"] != true || state["useTiming"] != false {
		t.Fatal("saved target settings missing", state)
	}
	if _, err = phase3NativeShowProperties(state); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(phase3SlidesProbeRunRoot, "recorded-show-readback")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "production-parser-result.json"), map[string]any{"source": source, "scope": "offline regression against actual server-loaded IO; no fabricated pre-write native baseline; no browser or mutation", "nativeState": state}); err != nil {
		t.Fatal(err)
	}
}
