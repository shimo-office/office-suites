package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPhase3SlidesReadProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_SLIDES_PROBE") != "1" {
		t.Skip("opt-in read-only live probe")
	}
	b, e := openPhase3Editor("https://shimo.net/presentation/M12Y8VYX0NiRmjWg?dbg=1")
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	v, e := b.Eval(`(()=>{const sdk=window.presentationSDK;const wb=sdk?.editor?.workbench;return {ready:wb?.isReady(),canEditType:typeof wb?.permissions?.canEdit,canEdit:wb?.permissions?.canEdit?.(),presence:window.__shimoPhase3Presence?.(),rawSlides:typeof wb.core?.getSlides==='function'?wb.core.getSlides(wb.editor):null,slideshowKeys:wb.slideshow?Object.keys(wb.slideshow):[],presentationKeys:Object.keys(sdk.API?.presentation||{}),slides:sdk.API.slides.getSlides().map(s=>({id:s.id,index:s.getIndex()})),canvas:[...document.querySelectorAll('canvas,svg')].map(n=>{const r=n.getBoundingClientRect();return {tag:n.tagName,width:r.width,height:r.height}})}})()`)
	if e != nil {
		t.Fatal(e)
	}
	p, _ := json.MarshalIndent(v, "", "  ")
	t.Log(string(p))
	t.Log("presence", b.CheckPresence())
	if e = phase3SlideScreenshot(b, "../tmp/phase3-live/slides-baseline.png"); e != nil {
		t.Fatal(e)
	}
}

// Test-only fixture preparation, never called by a normal test run. It seeds
// distinguishable content so duplicating an empty page cannot satisfy 33.2.
func TestPhase3SlidesFixtureMarkers(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_SLIDES_FIXTURE") != "1" {
		t.Skip("explicit test-file fixture mutation only")
	}
	const target = "https://shimo.net/presentation/M12Y8VYX0NiRmjWg?dbg=1"
	b, e := openPhase3Editor(target)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	for i := 0; i < 3; i++ {
		// Each marker is a separate one-shot mutation, preceded by the gate again.
		script := fmt.Sprintf(`(async()=>{%s const wb=window.presentationSDK.editor.workbench;if(wb.permissions.canEdit()!==true)return {ok:false,error:'read-only'};const api=window.presentationSDK.API.slides;const slides=api.getSlides();if(slides.length!==3)return {ok:false,error:'fixture must contain exactly three pages'};const slide=slides[%d];const before=slide.getShapes().map(s=>({id:s.id,text:s.textContent||''}));const marker='PHASE3-33.2-PAGE-%d-20261118';if(before.some(s=>s.text.includes(marker)))return {ok:false,error:'marker already present; do not replay'};const shape=await slide.insertTextBox({offset:{x:120,y:80},size:{width:650,height:100},content:[{text:marker,textStyle:{fontSize:26,bold:true}}]});if(!shape?.id||before.some(s=>s.id===shape.id))return {ok:false,error:'new shape ID missing'};if(typeof wb.content?.waitForSyncComplete==='function')await wb.content.waitForSyncComplete();const after=slide.getShapes().map(s=>({id:s.id,text:s.textContent||''}));if(after.length!==before.length+1||after.filter(s=>s.text.includes(marker)).length!==1)return {ok:false,error:'marker not confirmed; do not replay'};return {ok:true,slideId:slide.id,marker,before,after}})()`, phase3PresenceGuardJS(), i, i+1)
		v, e := b.Eval(script)
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := json.MarshalIndent(v, "", "  ")
		t.Log(string(raw))
	}
	if e = phase3SlideScreenshot(b, "../tmp/phase3-live/slides/fixture-markers.png"); e != nil {
		t.Fatal(e)
	}
}

func TestPhase3SlidesSlideshowProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_SLIDES_SHOW") != "1" {
		t.Skip("opt-in read-only slideshow verification")
	}
	b, e := openPhase3SlidesForegroundProbe("https://shimo.net/presentation/M12Y8VYX0NiRmjWg?dbg=1", "hidden-slideshow")
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	v, e := b.Eval(`(async()=>{const sdk=window.presentationSDK;const show=sdk.editor.workbench.slideshow;const wb=sdk.editor.workbench;await show.enterSlideshow({fromStart:true,config:{autoFullScreen:false}},false);for(let i=0;i<40&&!wb.core.isInPPTShowMode(wb.editor);i++)await new Promise(r=>setTimeout(r,250));if(!wb.core.isInPPTShowMode(wb.editor))return {ok:false,error:'slideshow engine never entered rendering mode'};return {current:show.getCurrentSlide(),state:show.getState(),coreInMode:wb.core.isInPPTShowMode(wb.editor),demonstrating:show.isDemonstrating(),count:show.getTotalSlideCount()}})()`)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.MarshalIndent(v, "", "  ")
	t.Log(string(raw))
	initial, ok := v.(map[string]any)
	if !ok {
		t.Fatal("invalid initial slideshow readback")
	}
	first, ok := initial["current"].(float64)
	if !ok {
		t.Fatal("missing actual first slideshow index")
	}
	sequence := []int{int(first)}
	if e = phase3SlideScreenshot(b, filepath.Join(phase3SlidesProbeRunRoot, "slideshow-page-1.png")); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		v, e = b.Eval(`(async()=>{const wb=window.presentationSDK.editor.workbench;const show=wb.slideshow;const previous=show.getCurrentSlide();if(!wb.core.isInPPTShowMode(wb.editor))return {ok:false,error:'slideshow engine not ready'};show.nextSlide({isStopAnimation:true});for(let i=0;i<20&&show.getCurrentSlide()===previous;i++)await new Promise(r=>setTimeout(r,250));return {current:show.getCurrentSlide(),state:show.getState(),coreInMode:wb.core.isInPPTShowMode(wb.editor),demonstrating:show.isDemonstrating()}})()`)
		if e != nil {
			t.Fatal(e)
		}
		raw, _ = json.MarshalIndent(v, "", "  ")
		t.Log(string(raw))
		if m, ok := v.(map[string]any); ok {
			if current, ok := m["current"].(float64); ok {
				sequence = append(sequence, int(current))
			}
		}
		if i == 0 {
			if e = phase3SlideScreenshot(b, filepath.Join(phase3SlidesProbeRunRoot, "slideshow-page-2.png")); e != nil {
				t.Fatal(e)
			}
		}
		if i == 1 {
			if e = phase3SlideScreenshot(b, filepath.Join(phase3SlidesProbeRunRoot, "slideshow.png")); e != nil {
				t.Fatal(e)
			}
		}
	}
	_, _ = b.Eval(`(()=>{window.presentationSDK.API.presentation.quit();return {ok:true}})()`)
	if len(sequence) < 3 || sequence[0] != 0 || sequence[1] != 1 || sequence[2] != 3 {
		t.Fatalf("hidden slide skip is NOT verified: observed zero-based sequence %v, expected prefix [0 1 3]", sequence)
	}
	if e = phase3WriteEvidence(filepath.Join(phase3SlidesProbeRunRoot, "slideshow-assertions.json"), map[string]any{"verified": true, "actualZeroBasedSequence": sequence, "expectedPrefix": []int{0, 1, 3}, "hiddenIndex": 2, "screenshot": "slideshow.png"}); e != nil {
		t.Fatal(e)
	}
}

func TestPhase3SlidesRecordedArtifacts(t *testing.T) {
	root := os.Getenv("SHIMO_PHASE3_SLIDES_EVIDENCE")
	if root == "" {
		t.Skip("opt-in offline replay of recorded real artifacts")
	}
	for _, name := range []string{"add", "duplicate", "hide", "delete-final"} {
		raw, e := os.ReadFile(filepath.Join(root, "33.2-"+name+".json"))
		if e != nil {
			t.Fatal(e)
		}
		var r struct {
			Action                   string
			BeforeArtifact, Artifact ArtifactEvidence
			EvidenceDir              string
			Slides                   []phase3SlidePage
		}
		if e = json.Unmarshal(raw, &r); e != nil {
			t.Fatal(e)
		}
		raw, e = os.ReadFile(filepath.Join(r.EvidenceDir, "request.json"))
		if e != nil {
			t.Fatal(e)
		}
		var req struct{ Spec phase3SlidesSpec }
		if e = json.Unmarshal(raw, &req); e != nil {
			t.Fatal(e)
		}
		raw, e = os.ReadFile(filepath.Join(r.EvidenceDir, "before-readback.json"))
		if e != nil {
			t.Fatal(e)
		}
		var recorded []phase3SlidePage
		if e = json.Unmarshal(raw, &recorded); e != nil {
			t.Fatal(e)
		}
		beforeIDs := []string{}
		afterIDs := []string{}
		for _, s := range recorded {
			beforeIDs = append(beforeIDs, s.ID)
		}
		for _, s := range r.Slides {
			afterIDs = append(afterIDs, s.ID)
		}
		before, e := readPhase3SlideArtifact(r.BeforeArtifact.Path, beforeIDs)
		if e != nil {
			t.Fatal(e)
		}
		after, e := readPhase3SlideArtifact(r.Artifact.Path, afterIDs)
		if e != nil {
			t.Fatal(e)
		}
		if e = verifyPhase3Slides(before, after, req.Spec); e != nil {
			t.Fatalf("%s: %v", name, e)
		}
	}
}

func TestPhase3SlidesCapabilitiesProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_SLIDES_CAPABILITIES") != "1" {
		t.Skip("opt-in read-only capability probe")
	}
	b, e := openPhase3SlidesForegroundProbe(phase3SlidesSyntheticTarget(), "capabilities")
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	v, e := b.Eval(`(async()=>{const sdk=window.presentationSDK,wb=sdk.editor.workbench,c=wb.core,e=wb.editor;const slides=[];for(const slide of sdk.API.slides.getSlides()){const elements=c.getSlideElements(e,slide.getIndex());const tables=await slide.getTables();slides.push({id:slide.id,index:slide.getIndex(),shapes:slide.getShapes().map(s=>({id:s.id,name:s.name,type:s.type,text:s.textContent??null})),tables:tables.map(t=>({id:t.id,name:elements.find(n=>n.id===t.id)?.name??null,rows:t.rowCount,columns:t.columnCount})),elements:elements.map(n=>({id:n.id,name:n.name,type:n.type})),animations:typeof c.getSlideAnimation==='function'?c.getSlideAnimation(e,slide.getIndex()):null})};const names=['getSlideImageLikes','addAudio','addVideo','setCellBorders','setCellsBackground','splitCell','canSplitTableCells','setTableRangeStyle','getSlideAnimation','applySlideAnimation','getSlideTransition','startPPTShow','getSlideMasters','getSlideLayout','applySlideAudios','getSlideImageLikes','getMediaByRefId'];const methods=Object.fromEntries(names.map(k=>[k,typeof c[k]==='function'?String(c[k]).slice(0,2500):null]));const themes=wb.theme.getOfficialThemes().map(t=>({id:t.id,name:t.name,colors:t.colors,bodyFont:t.bodyFont,titleFont:t.titleFont}));const official=wb.theme.getOfficialThemes()[0];const loaded=official?await official.theme():null;const template=loaded?.default;return {ok:true,target:location.origin+location.pathname,slides,methods,themes,mediaCapability:{enabled:wb.media?.queryInsertVideo?.().enabled,canInsertAudio:wb.command.canExecCommand('insert.audio'),canInsertVideo:wb.command.canExecCommand('insert.video'),acceptAudio:wb.media?.getAcceptFileType?.('audio'),acceptVideo:wb.media?.getAcceptFileType?.('video')},officialTemplate:official?{id:official.id,definition:template,images:official.images}:null,nativeThemeDelta:{modoc:typeof window.modoc?.Delta,debugParser:typeof sdk.constructor?.debug?.parse,ioData:typeof e._$collaborationManager$_?._$currentPptIOData$_,width:e.presentation.width,height:e.presentation.height,rootOps:e._$collaborationManager$_?._$currentPptIOData$_?e._$ioAdapter$_.fromOperations(e._$collaborationManager$_._$currentPptIOData$_()).toJSON().filter(x=>x.attributes?.opType==='slideMasters'||x.attributes?.opType==='layouts'):null},builtinThemes:wb.theme.getBuiltinThemes().map(t=>({id:t.id,refId:t.refId,name:t.name,colors:t.colors,titleFont:t.titleFont,bodyFont:t.bodyFont})),design:` + phase3SlideDesignJS() + `};})()`)
	if e != nil {
		t.Fatal(e)
	}
	if e = phase3WriteEvidence(filepath.Join(phase3SlidesProbeRunRoot, "capabilities-probe.json"), v); e != nil {
		t.Fatal(e)
	}
	t.Log("saved", filepath.Join(phase3SlidesProbeRunRoot, "capabilities-probe.json"))
	if _, e = b.Eval(`(()=>{window.presentationSDK.editor.workbench.slideshow.openSettings();return {ok:true}})()`); e != nil {
		t.Fatal(e)
	}
	settings, e := b.Eval(`(async()=>{await new Promise(r=>setTimeout(r,1200));return {text:document.body.innerText,dialogs:[...document.querySelectorAll('[role="dialog"],[aria-modal="true"]')].map(n=>n.innerText)}})()`)
	if e != nil {
		t.Fatal(e)
	}
	if e = phase3WriteEvidence(filepath.Join(phase3SlidesProbeRunRoot, "slideshow-settings-probe.json"), settings); e != nil {
		t.Fatal(e)
	}
	if e = phase3SlideScreenshot(b, filepath.Join(phase3SlidesProbeRunRoot, "slideshow-settings.png")); e != nil {
		t.Fatal(e)
	}
}

func TestPhase3SlidesSyntheticFixtureArtifact(t *testing.T) {
	file := os.Getenv("SHIMO_PHASE3_SLIDES_LOCAL_FIXTURE")
	if file == "" {
		t.Skip("explicit local synthetic fixture path")
	}
	ids := []string{"a", "b", "c"}
	if _, e := readPhase3SlideArtifact(file, ids); e != nil {
		t.Fatal(e)
	}
	table := phase3SlidesSpec{Action: "slide-table-edit", SlideID: "b", Table: &phase3SlideTableSpec{Name: "Phase3 Table", Operation: "insert-row", Index: 1}}
	if e := phase3PrepareSlideTable(file, ids, &table); e != nil {
		t.Fatal(e)
	}
	if table.Table.Rows != 3 || table.Table.Columns != 3 {
		t.Fatal("fixture table not 3x3")
	}
	shape := phase3SlidesSpec{Action: "slide-shape-format", SlideID: "c", Shape: &phase3SlideShapeSpec{Name: "Phase3 Rectangle", FillColor: "#0000FF", LineColor: "#FF0000", Rotation: ptrSlideNumber(45), HorizontalCenter: true, BehindName: "Phase3 Circle"}}
	if e := phase3PrepareSlideShape(file, ids, &shape); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"Phase3 Title", "Phase3 Body"} {
		s := phase3SlidesSpec{SlideID: "a", Placeholder: &phase3SlidePlaceholderSpec{Name: name, Text: "三期验收"}}
		if _, e := readPhase3SlideArtifactFiltered(file, ids, phase3PlaceholderFilter(ids, s, false)); e != nil {
			t.Fatal(e)
		}
	}
}
