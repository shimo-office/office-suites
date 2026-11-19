package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhase3SlideMediaPreservesClickAudioTiming(t *testing.T) {
	raw, e := os.ReadFile("testdata/phase3-click-audio-timing.xml")
	if e != nil {
		t.Fatal(e)
	}
	page := `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree><p:pic><p:nvPicPr><p:cNvPr id="7" name="old.wav"/><p:nvPr><a:audioFile/></p:nvPr></p:nvPicPr></p:pic></p:spTree></p:cSld>` + string(raw) + `</p:sld>`
	parse := func(s string) *artifactNode {
		n, e := parseArtifactXML([]byte(s))
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	spec := phase3SlidesSpec{SlideID: "one", Media: &phase3SlideMediaSpec{Kind: "video", Name: "new.mp4"}}
	if _, e := phase3MediaTargetFilter([]string{"one"}, spec, nil, false)(0, parse(page)); e != nil {
		t.Fatalf("valid existing click audio rejected: %v", e)
	}
	for _, bad := range []string{
		strings.Replace(page, `spid="7"`, `spid="8"`, 1),
		strings.Replace(page, `nodeType="clickEffect"`, `nodeType="withEffect"`, 1),
		strings.Replace(page, `cmd="playFrom(0.0)"`, `cmd="stop"`, 1),
		strings.ReplaceAll(strings.ReplaceAll(page, `<ns0:audio>`, `<ns0:video>`), `</ns0:audio>`, `</ns0:video>`),
		strings.Replace(page, `<a:audioFile/>`, `<a:audioFile/><a:audioFile/>`, 1),
		strings.Replace(page, `<ns0:tnLst>`, `<ns0:anim/><ns0:tnLst>`, 1),
	} {
		if _, e := phase3MediaTargetFilter([]string{"one"}, spec, nil, false)(0, parse(bad)); e == nil {
			t.Fatal("invalid existing timing accepted")
		}
	}
}

func TestPhase3SlideMediaRecordedSamePageZIP(t *testing.T) {
	dir := os.Getenv("SHIMO_PHASE3_SAME_PAGE_ZIP")
	if dir == "" {
		t.Skip("opt-in actual exported ZIP regression")
	}
	raw, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var files struct{ Before, After string }
	if e = json.Unmarshal(raw, &files); e != nil {
		t.Fatal(e)
	}
	ids := []string{"s0000000", "s3283c53", "s1cd3169", "sdfdadaa"}
	spec := phase3SlidesSpec{SlideID: ids[0], Media: &phase3SlideMediaSpec{Kind: "video", Name: "phase3-flower.mp4", SHA256: "0cd83d944a6ca7822b4a8306cecc60a36e859b041f6702c6a1ad9ead78924451"}}
	if e = verifyPhase3SlideMediaArtifacts(files.Before, files.After, ids, ids, spec); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"timing-drift", "unrelated-target", "multiple-audio"} {
		if e = verifyPhase3SlideMediaArtifacts(files.Before, filepath.Join(dir, name+".pptx"), ids, ids, spec); e == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}
