package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPhase3SlideMediaSpec(t *testing.T) {
	p := filepath.Join(t.TempDir(), "clip.wav")
	if e := os.WriteFile(p, []byte("RIFFfixtureWAVE"), 0600); e != nil {
		t.Fatal(e)
	}
	s, e := preparePhase3SlideMedia(map[string]any{"slideId": "a", "kind": "audio", "file": p, "triggerType": "click"})
	if e != nil || s.SHA256 == "" || s.Name != "clip.wav" {
		t.Fatalf("%+v %v", s, e)
	}
	for _, m := range []map[string]any{{"slideId": "a", "kind": "video", "file": p}, {"slideId": "a", "kind": "audio", "file": p, "triggerType": "magic"}, {"slideId": "a", "kind": "audio", "file": p, "unknown": true}} {
		if _, e := preparePhase3SlideMedia(m); e == nil {
			t.Fatal("invalid media accepted")
		}
	}
}
func TestPhase3SlideMediaNodeProof(t *testing.T) {
	node := func(extra string) *artifactNode {
		n, e := parseArtifactXML([]byte(`<p:pic xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:nvPicPr><p:cNvPr id="5" name="clip.wav"/><p:nvPr>` + extra + `</p:nvPr></p:nvPicPr><p:blipFill><a:blip r:embed="poster"/></p:blipFill><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100" cy="100"/></a:xfrm></p:spPr></p:pic>`))
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	s := phase3SlideMediaSpec{Kind: "audio", Name: "clip.wav", SHA256: "hash"}
	refs := map[string]string{"real": artifactOfficeRelNS + "/audio:hash", "poster": artifactOfficeRelNS + "/image:posterhash"}
	if _, e := phase3VerifySlideMediaNode(node(`<a:audioFile r:link="real"/>`), refs, s); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []*artifactNode{node(""), node(`<a:videoFile r:link="real"/>`), node(`<a:audioFile r:link="poster"/>`)} {
		if _, e := phase3VerifySlideMediaNode(bad, refs, s); e == nil {
			t.Fatal("poster or wrong media accepted")
		}
	}
}

func TestPhase3SlideMediaPlayerRelativeURLIdentity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional Node regression runtime")
	}
	script := `global.location={href:'https://shimo.net/presentation/fixture'};const match=` + phase3SlideMediaPlayerMatchesJS() + `;const element={currentSrc:'https://shimo.net/uploader/test.wav?g=fixture&key=one',src:'https://shimo.net/uploader/test.wav?g=fixture&key=one'};if(match({...element,currentSrc:'https://shimo.net/uploader/other.wav'},'/uploader/test.wav?g=fixture&key=one'))throw Error('loaded URL mismatch hidden by pending src');if(!match(element,'/uploader/test.wav?g=fixture&key=one'))throw Error('relative native source not matched');for(const url of ['/uploader/other.wav?g=fixture&key=one','/uploader/test.wav?g=fixture&key=two','https://other.net/uploader/test.wav?g=fixture&key=one'])if(match(element,url))throw Error('wrong media accepted');`
	if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, out)
	}
}
