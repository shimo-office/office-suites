package main

import "testing"

func TestPhase3SlideShowSpec(t *testing.T) {
	s, e := preparePhase3Slides("slide-show-settings", map[string]any{"loopShow": true, "useTiming": false})
	if e != nil || s.Show == nil {
		t.Fatalf("%v", e)
	}
	for _, p := range []map[string]any{{}, {"loopShow": "true"}, {"fromPage": float64(2)}} {
		if _, e := preparePhase3Slides("slide-show-settings", p); e == nil {
			t.Fatal("invalid show settings accepted")
		}
	}
}
func TestPhase3SlideShowProperties(t *testing.T) {
	fixture := func(attrs string) string {
		return artifactTestZip(t, map[string]string{"ppt/presProps.xml": `<p:presentationPr xmlns:p="` + artifactSlideNS + `"><p:showPr ` + attrs + `/></p:presentationPr>`})
	}
	before := fixture("")
	after := fixture(`loop="1" useTimings="0"`)
	loop, timing := true, false
	s := phase3SlideShowSpec{LoopShow: &loop, UseTiming: &timing}
	if e := verifyPhase3SlideShowProperties(before, after, s); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{fixture(`loop="0" useTimings="0"`), fixture(`loop="1" useTimings="1"`), fixture(`loop="1" useTimings="0" showAnimation="0"`)} {
		if e := verifyPhase3SlideShowProperties(before, bad, s); e == nil {
			t.Fatal("wrong or collateral setting accepted")
		}
	}
}
