package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPhase3SlidesInspectFailedMediaReadOnly(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_SLIDES_INSPECT_MEDIA")
	if source == "" {
		t.Skip("opt-in media inspection without insertion")
	}
	var request struct {
		Spec phase3SlidesSpec
		URL  string
	}
	raw, err := os.ReadFile(filepath.Join(source, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &request); err != nil || request.Spec.Media == nil {
		t.Fatal("invalid media request", err)
	}
	raw, err = os.ReadFile(filepath.Join(source, "input-media-sha256.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, request.Spec.Media); err != nil {
		t.Fatal(err)
	}
	b, err := openPhase3SlidesForegroundProbe(request.URL+"?dbg=1", "media-inspection")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	dir := filepath.Join(phase3SlidesProbeRunRoot, "media-inspection")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(request.Spec)
	value, err := b.EvalUserGesture(`(async()=>{const r=` + string(raw) + `,sdk=window.presentationSDK,wb=sdk.editor.workbench,e=wb.editor,c=wb.core;const slide=sdk.API.slides.getSlides().find(x=>x.id===r.slideId);sdk.API.slides.setCurrentSlideIndex(slide.id);await new Promise(r=>setTimeout(r,500));const object=c.getSlideImageLikes(e,slide.getIndex()).find(x=>x.name===r.media.name),setting=wb.animation.getAudioSettings().find(x=>x.shapeId===object.id);const summary=n=>({tag:n?.tagName,constructor:n?.constructor?.name,keys:n?Object.keys(n).slice(0,20):[],time:n?.currentTime,duration:n?.duration,paused:n?.paused,readyState:n?.readyState,error:n?.error?.code??null,source:n?.src?new URL(n.src,location.href).pathname:null});const phases=[];for(const select of [false,true]){if(select){c.selectElementByIds(e,[object.id]);await new Promise(r=>setTimeout(r,400))}await wb.media.toggleAudio({url:object.audio.url,action:'start',setting,objectId:object.id});const samples=[];for(let i=0;i<25;i++){await new Promise(r=>setTimeout(r,100));const playing=wb.media.getPlayingAudio?.();samples.push({at:i*.1,dom:[...document.querySelectorAll('audio')].map(summary),playing:summary(playing),nativeState:(()=>{const state=wb.media.getAudioState?.(object.audio.url);return state?Object.fromEntries(['id','kind','currentTime','paused','hide','loopPlay','volume'].map(k=>[k,state[k]])):null})(),isPlaying:wb.media.isPlayingAudio(object.id)})}phases.push({select,samples});await wb.media.toggleAudio({url:object.audio.url,action:'pause',setting,objectId:object.id})}return {ok:true,target:location.origin+location.pathname,object:{id:object.id,name:object.name},phases};})()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "playback-inspection.json"), value); err != nil {
		t.Fatal(err)
	}
	if err = phase3SlideScreenshot(b, filepath.Join(dir, "media.png")); err != nil {
		t.Fatal(err)
	}
}

func TestPhase3SlidesRecoverMediaReadOnly(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_SLIDES_RECOVER_MEDIA")
	if source == "" {
		t.Skip("opt-in media recovery without insertion")
	}
	var request struct {
		Spec phase3SlidesSpec
		URL  string
	}
	raw, err := os.ReadFile(filepath.Join(source, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &request); err != nil || request.Spec.Media == nil {
		t.Fatal("invalid request", err)
	}
	raw, err = os.ReadFile(filepath.Join(source, "input-media-sha256.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, request.Spec.Media); err != nil {
		t.Fatal(err)
	}
	var pages []phase3SlidePage
	raw, err = os.ReadFile(filepath.Join(source, "before-readback.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &pages); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, p := range pages {
		ids = append(ids, p.ID)
	}
	b, err := openPhase3SlidesForegroundProbe(request.URL+"?dbg=1", "media-recovery")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	dir := filepath.Join(phase3SlidesProbeRunRoot, "media-recovery")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	current, err := phase3SlidesRead(b)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{Origin: "https://shimo.net", Cmd: "phase3", JSON: true}
	fresh, err := a.phase3OfficeExport(guidFromURL(request.URL), filepath.Join(dir, "fresh"), "pptx")
	if err != nil {
		t.Fatal(err)
	}
	before, err := filepath.Glob(filepath.Join(source, "before", "*.pptx"))
	if err != nil || len(before) != 1 {
		t.Fatal(err)
	}
	preservationBaseline := os.Getenv("SHIMO_PHASE3_SLIDES_MEDIA_CURRENT_BASELINE")
	if preservationBaseline != "" {
		prior, e := readPhase3SlideArtifact(preservationBaseline, ids)
		if e != nil {
			t.Fatal(e)
		}
		next, e := readPhase3SlideArtifact(fresh.Path, current)
		if e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(prior, next) {
			t.Fatal("final read-only playback baseline changed")
		}
	} else if err = verifyPhase3SlideMediaArtifacts(before[0], fresh.Path, ids, current, request.Spec); err != nil {
		t.Fatal(err)
	}
	proof, err := phase3VerifyReloadedMedia(b, request.Spec, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "playback-evidence.json"), proof); err != nil {
		t.Fatal(err)
	}
	if err = phase3SlideScreenshot(b, filepath.Join(dir, "reload.png")); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "recovered-verification.json"), map[string]any{"verified": true, "mutationAttempted": false, "originalFailedCommandEvidence": source, "preservationBaseline": preservationBaseline, "freshArtifact": fresh, "playback": proof, "visualReviewRequired": true}); err != nil {
		t.Fatal(err)
	}
}
