package main

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestPhase3SlideLayoutSpec(t *testing.T) {
	s, e := preparePhase3Slides("slide-layout", map[string]any{"slideId": "s1", "layoutId": "42"})
	if e != nil {
		t.Fatal(e)
	}
	if s.Layout == nil || s.Layout.ID != "42" {
		t.Fatal("missing layout")
	}
	for _, p := range []map[string]any{{"slideId": "s1"}, {"slideId": "s1", "layoutId": 42}, {"slideId": "s1", "layoutId": ""}, {"slideId": "s1", "layoutId": "42", "extra": true}} {
		if _, e := preparePhase3Slides("slide-layout", p); e == nil {
			t.Fatal("invalid layout payload accepted")
		}
	}
}
func TestPhase3SlideLayoutPostcondition(t *testing.T) {
	b := []phase3SlidePage{{ID: "a", Content: "one", Layout: "old", FixedObjects: []string{"image-A", "chart-B"}, Texts: []string{"title", "body"}}, {ID: "b", Content: "two", Layout: "unchanged"}}
	s := phase3SlidesSpec{Action: "slide-layout", SlideID: "a", Layout: &phase3SlideLayoutSpec{ID: "new-id", Expected: "new"}}
	a := append([]phase3SlidePage(nil), b...)
	a[0].Content = "changed target style"
	a[0].Layout = "new"
	if e := verifyPhase3SlideLayout(b, a, s); e != nil {
		t.Fatal(e)
	}
	if e := verifyPhase3SlideLayout(b, b, s); e == nil {
		t.Fatal("no-op accepted")
	}
	wrong := append([]phase3SlidePage(nil), a...)
	wrong[0].Texts = []string{"title"}
	if e := verifyPhase3SlideLayout(b, wrong, s); e == nil {
		t.Fatal("text loss accepted")
	}
	wrong = append([]phase3SlidePage(nil), a...)
	wrong[1].Content = "collateral"
	if e := verifyPhase3SlideLayout(b, wrong, s); e == nil {
		t.Fatal("collateral accepted")
	}
	wrong = append([]phase3SlidePage(nil), a...)
	wrong[0].Hidden = true
	if e := verifyPhase3SlideLayout(b, wrong, s); e == nil {
		t.Fatal("hidden changed accepted")
	}
	wrong = append([]phase3SlidePage(nil), a...)
	wrong[0].FixedObjects = []string{"image-A"}
	if e := verifyPhase3SlideLayout(b, wrong, s); e == nil {
		t.Fatal("chart loss accepted")
	}
	wrong = append([]phase3SlidePage(nil), a...)
	wrong[0].FixedObjects = []string{"image-A", "different chart"}
	if e := verifyPhase3SlideLayout(b, wrong, s); e == nil {
		t.Fatal("chart mutation accepted")
	}

}

func TestPhase3SlideLayoutMapping(t *testing.T) {
	snapshot := map[string]any{"slides": []any{map[string]any{"id": "s1", "masterId": "m1", "layoutId": "old"}}, "masters": []any{map[string]any{"id": "m1", "layouts": []any{map[string]any{"id": "old", "index": float64(0)}, map[string]any{"id": "new", "index": float64(1)}}}}}
	s := phase3SlidesSpec{SlideID: "s1", Layout: &phase3SlideLayoutSpec{ID: "new"}}
	if e := phase3ResolveSlideLayoutValue(snapshot, &s); e != nil {
		t.Fatal(e)
	}
	if s.Layout.Index != 1 || s.Layout.CurrentIndex != 0 {
		t.Fatal("wrong layout mapping")
	}
	for _, id := range []string{"missing", "old"} {
		bad := phase3SlidesSpec{SlideID: "s1", Layout: &phase3SlideLayoutSpec{ID: id}}
		if e := phase3ResolveSlideLayoutValue(snapshot, &bad); e == nil {
			t.Fatal("unavailable/no-op layout accepted")
		}
	}
	entries := map[string]string{
		"ppt/presentation.xml":                `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId id="256" r:id="one"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels":     `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/slide1.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":               `<p:sld xmlns:p="` + artifactSlideNS + `"><p:cSld><p:spTree/></p:cSld></p:sld>`,
		"ppt/slides/_rels/slide1.xml.rels":    `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="layout" Type="` + artifactOfficeRelNS + `/slideLayout" Target="../slideLayouts/old.xml"/></Relationships>`,
		"ppt/slideLayouts/old.xml":            `<p:sldLayout xmlns:p="` + artifactSlideNS + `"><p:cSld name="old"/></p:sldLayout>`,
		"ppt/slideLayouts/new.xml":            `<p:sldLayout xmlns:p="` + artifactSlideNS + `"><p:cSld name="new"/></p:sldLayout>`,
		"ppt/slideLayouts/_rels/old.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="master" Type="` + artifactOfficeRelNS + `/slideMaster" Target="../slideMasters/one.xml"/></Relationships>`,
		"ppt/slideMasters/one.xml":            `<p:sldMaster xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldLayoutIdLst><p:sldLayoutId id="1" r:id="l1"/><p:sldLayoutId id="2" r:id="l2"/></p:sldLayoutIdLst></p:sldMaster>`,
		"ppt/slideMasters/_rels/one.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="l1" Type="` + artifactOfficeRelNS + `/slideLayout" Target="../slideLayouts/old.xml"/><Relationship Id="l2" Type="` + artifactOfficeRelNS + `/slideLayout" Target="../slideLayouts/new.xml"/></Relationships>`,
	}
	p := artifactTestZip(t, entries)
	got, e := phase3SlideExpectedLayout(p, []string{"s1"}, s)
	if e != nil {
		t.Fatal(e)
	}
	z, files, e := openArtifact(p)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	digest, e := phase3SlideResourceFingerprint(files, "ppt/slideLayouts/new.xml")
	if e != nil {
		t.Fatal(e)
	}
	want := artifactOfficeRelNS + "/slideLayout:" + digest
	if got != want {
		t.Fatalf("%s != %s", got, want)
	}
	s.Layout.CurrentIndex = 1
	if _, e = phase3SlideExpectedLayout(p, []string{"s1"}, s); e == nil {
		t.Fatal("SDK/export index mismatch accepted")
	}
}
func TestPhase3SlideLayoutSingleTargetMutation(t *testing.T) {
	node, e := exec.LookPath("node")
	if e != nil {
		t.Skip("node only for JS regression")
	}
	for _, edit := range []bool{true, false} {
		s := phase3SlidesSpec{Action: "slide-layout", SlideID: "s1", Layout: &phase3SlideLayoutSpec{ID: "42", Index: 1}}
		want := 0
		if edit {
			want = 1
		}
		script := fmt.Sprintf(`let writes=0,current=41,selection=[];global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:[1]}),presentationSDK:{editor:{workbench:{isReady:()=>true,permissions:{canEdit:()=>%t},context:{getUser:()=>({id:1})},core:{getSlideLayout:()=>({masterId:'m',layoutId:current}),getSlideMasters:()=>[{masterId:'m',layouts:[{layoutId:41,index:0},{layoutId:42,index:1}]}],changeLayout:(ed,id)=>{if(id!==42||JSON.stringify(selection)!=='["s1"]')throw new Error('wrong target');writes++;current=id}}}},API:{slides:{getSlides:()=>[{id:'s1',getIndex:()=>0},{id:'s2',getIndex:()=>1}],setCurrentSlideIndex:()=>{},setSelectedSlides:ids=>{selection=ids}}}}};const result=%s;if(writes!==%d)throw new Error('wrong write count');if(writes===0&&result.ok!==false)throw new Error('missing guard');`, edit, phase3SlideLayoutMutationJS(s, []string{"s1", "s2"}), want)
		if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
			t.Fatalf("%v %s", e, out)
		}
	}
}
