package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// This opt-in acceptance probe observes actual native slideshow rendering. It
// never writes editor content or changes loop/timing settings to make it pass.
func TestPhase3SlidesAnimationAndLoopPlayback(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_SLIDES_ANIMATION_PLAYBACK") != "1" {
		t.Skip("opt-in native animation and loop playback acceptance")
	}
	folder := filepath.Join(phase3SlidesProbeRunRoot, "animation-loop-playback")
	if err := os.MkdirAll(filepath.Dir(folder), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	b, err := openPhase3SlidesForegroundProbe(phase3SlidesSyntheticTarget(), "animation-loop-playback")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ids, err := phase3SlidesRead(b)
	if err != nil || len(ids) != 3 {
		t.Fatalf("expected three actual slides: %v %v", ids, err)
	}
	targets := []phase3SlideAnimationTarget{{SlideID: ids[0], Text: "三期验收"}, {SlideID: ids[1], Text: "新版测试 二"}, {SlideID: ids[2], Text: "新版测试 三"}}
	native, err := phase3VerifyReloadedAnimation(b, phase3SlidesSpec{Animation: &phase3SlideAnimationSpec{Targets: targets, FontSize: 24}})
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(folder, "native-animation.json"), native); err != nil {
		t.Fatal(err)
	}
	start, err := b.Eval(`(async()=>{const wb=window.presentationSDK.editor.workbench;if(wb.core.isLoopShow(wb.editor)!==true||wb.core.isUseTiming(wb.editor)!==false)return {ok:false,error:'loop/timing settings do not match saved request'};await wb.slideshow.enterSlideshow({fromStart:true,config:{autoFullScreen:false}},false);for(let i=0;i<40&&!wb.core.isInPPTShowMode(wb.editor);i++)await new Promise(r=>setTimeout(r,250));if(!wb.core.isInPPTShowMode(wb.editor)||wb.slideshow.getCurrentSlide()!==0)return {ok:false,error:'native slideshow failed to start at page 1'};await new Promise(r=>setTimeout(r,1000));return {ok:true,loopShow:wb.core.isLoopShow(wb.editor),useTiming:wb.core.isUseTiming(wb.editor),current:wb.slideshow.getCurrentSlide()}})()`)
	if err != nil {
		t.Fatal(err)
	}
	states := []any{start}
	sequence := []int{0}
	for i := 0; i < 3; i++ {
		number := string(rune('1' + i))
		if err = phase3SlideScreenshot(b, filepath.Join(folder, "page-"+number+"-before-click.png")); err != nil {
			t.Fatal(err)
		}
		// The first real Next must reveal the object's entrance animation without
		// changing the page. A second Next must advance, including last -> first.
		for click := 0; click < 2; click++ {
			expected := i
			if click == 1 {
				expected = (i + 1) % 3
			}
			raw, _ := json.Marshal(map[string]int{"page": i, "expected": expected, "click": click + 1})
			state, e := b.Eval(`(async()=>{const request=` + string(raw) + `,wb=window.presentationSDK.editor.workbench,show=wb.slideshow;if(!wb.core.isInPPTShowMode(wb.editor)||show.getCurrentSlide()!==request.page)return {ok:false,error:'unexpected actual slideshow page before click'};show.nextSlide();await new Promise(r=>setTimeout(r,1000));const actual=show.getCurrentSlide();if(!wb.core.isInPPTShowMode(wb.editor)||actual!==request.expected)return {ok:false,error:'native click did not produce expected animation/page transition',actual,request};return {ok:true,request,actual,coreInMode:true,demonstrating:show.isDemonstrating()}})()`)
			if e != nil {
				t.Fatal(e)
			}
			states = append(states, state)
			sequence = append(sequence, int(state.(map[string]any)["actual"].(float64)))
			if err = phase3WriteEvidence(filepath.Join(folder, "observed-clicks.json"), states); err != nil {
				t.Fatal(err)
			}
			if click == 0 {
				if err = phase3SlideScreenshot(b, filepath.Join(folder, "page-"+number+"-after-click.png")); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err = phase3SlideScreenshot(b, filepath.Join(folder, "loop-return-first-page.png")); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Eval(`(()=>{window.presentationSDK.editor.workbench.slideshow.quitSlideshow();return {ok:true}})()`); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(folder, "assertions.json"), map[string]any{"nativeAnimationAndLoopObserved": true, "actualClickPageSequence": sequence, "visualReviewRequired": true, "titleTargets": targets}); err != nil {
		t.Fatal(err)
	}
}
