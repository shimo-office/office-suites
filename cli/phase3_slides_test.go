package main

import (
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestPhase3SlidesPayload(t *testing.T) {
	for _, a := range []string{"slide-add", "slide-duplicate", "slide-hide", "slide-delete"} {
		if _, e := preparePhase3Slides(a, map[string]any{"slideId": "s-1"}); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range []map[string]any{nil, {}, {"slideId": ""}, {"slideId": 12}, {"slideId": "x", "index": 1}, {"slideId": "x\n"}} {
		if _, e := preparePhase3Slides("slide-add", p); e == nil {
			t.Fatalf("accepted %#v", p)
		}
	}
	if _, e := preparePhase3Slides("slide-magic", map[string]any{"slideId": "x"}); e == nil {
		t.Fatal("unknown action")
	}
}
func TestPhase3SlidesPostconditions(t *testing.T) {
	b := []phase3SlidePage{{ID: "a", Content: "A"}, {ID: "b", Content: "B"}, {ID: "c", Content: "C"}}
	for _, c := range []struct {
		a    string
		p    []phase3SlidePage
		pass bool
	}{
		{"slide-add", []phase3SlidePage{b[0], b[1], {ID: "new", Content: "empty"}, b[2]}, true},
		{"slide-add", b, false},
		{"slide-duplicate", []phase3SlidePage{b[0], b[1], {ID: "new", Content: "B"}, b[2]}, true},
		{"slide-duplicate", []phase3SlidePage{b[0], b[1], {ID: "new", Content: "wrong"}, b[2]}, false},
		{"slide-hide", []phase3SlidePage{b[0], {ID: "b", Content: "B", Hidden: true}, b[2]}, true},
		{"slide-hide", []phase3SlidePage{b[0], b[1], {ID: "c", Content: "C", Hidden: true}}, false},
		{"slide-delete", []phase3SlidePage{b[0], b[2]}, true},
		{"slide-delete", []phase3SlidePage{b[1], b[2]}, false},
		{"slide-add", []phase3SlidePage{b[0], b[1], {ID: "new", Content: "empty"}, {ID: "c", Content: "changed"}}, false},
		{"slide-add", []phase3SlidePage{b[0], {ID: "new", Content: "empty"}, b[1], b[2]}, false},
		{"slide-duplicate", []phase3SlidePage{b[0], b[1], {ID: "b", Content: "B"}, b[2]}, false},
	} {
		if e := verifyPhase3Slides(b, c.p, phase3SlidesSpec{Action: c.a, SlideID: "b"}); (e == nil) != c.pass {
			t.Errorf("%s pass=%t err=%v", c.a, c.pass, e)
		}
	}
}
func TestPhase3SlideArtifactHiddenAndContent(t *testing.T) {
	e := map[string]string{
		"ppt/presentation.xml":            `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId id="256" r:id="one"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/slide1.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":           `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `" show="0"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="1" name="title"/></p:nvSpPr><p:txBody><a:p><a:r><a:t>hello</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`,
	}
	one, err := readPhase3SlideArtifact(artifactTestZip(t, e), []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if !one[0].Hidden {
		t.Fatal("lost hidden state")
	}
	e["ppt/slides/slide1.xml"] = strings.ReplaceAll(strings.ReplaceAll(e["ppt/slides/slide1.xml"], `show="0"`, `show="1"`), `id="1"`, `id="99"`)
	two, err := readPhase3SlideArtifact(artifactTestZip(t, e), []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if two[0].Hidden || two[0].Content != one[0].Content {
		t.Fatal("identity normalization changed content")
	}
	e["ppt/slides/slide1.xml"] = strings.ReplaceAll(e["ppt/slides/slide1.xml"], "hello", "changed")
	three, err := readPhase3SlideArtifact(artifactTestZip(t, e), []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if three[0].Content == one[0].Content {
		t.Fatal("missed text edit")
	}
	if _, err := readPhase3SlideArtifact(artifactTestZip(t, e), []string{"a", "b"}); err == nil {
		t.Fatal("accepted count mismatch")
	}
}

func TestPhase3SlidesRejectGuardAndExecuteOnce(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node only used for JavaScript regression tests")
	}
	for _, tc := range []struct {
		name                                string
		ready, edit, presence, other, stale bool
		calls                               int
	}{
		{"allowed", true, true, true, false, false, 1}, {"not ready", false, true, true, false, false, 0}, {"read only", true, false, true, false, false, 0}, {"unknown presence", true, true, false, false, false, 0}, {"other collaborator", true, true, true, true, false, 0}, {"stale IDs", true, true, true, false, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := fmt.Sprintf(`let calls=0;const ids=%s;global.window={__shimoPhase3Presence:()=>({ready:%t,connected:true,ids:%s}),logged:{user:{id:1}},presentationSDK:{editor:{workbench:{isReady:()=>%t,permissions:{canEdit:()=>%t},context:{getUser:()=>({id:1})}}},API:{slides:{getSlides:()=>ids.map(id=>({id})),setCurrentSlideIndex:()=>{},setSelectedSlides:()=>{},addSlide:()=>{calls++;ids.push('b');return {id:'b'}}}}}};const result=%s;if(calls!==%d)throw new Error('unexpected writes: '+calls);if(calls===0&&result.ok!==false)throw new Error('guard did not reject');`, map[bool]string{false: `["a"]`, true: `["changed"]`}[tc.stale], tc.presence, map[bool]string{false: `[1]`, true: `[1,2]`}[tc.other], tc.ready, tc.edit, phase3SlidesMutationJS(phase3SlidesSpec{Action: "slide-add", SlideID: "a"}, []string{"a"}), tc.calls)
			if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
				t.Fatalf("%v %s", e, out)
			}
		})
	}
}

func TestPhase3SlideRelationshipProof(t *testing.T) {
	entries := map[string]string{
		"ppt/slides/_rels/slide1.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="img" Type="` + artifactOfficeRelNS + `/image" Target="../media/image.png"/><Relationship Id="link" Type="` + artifactOfficeRelNS + `/hyperlink" Target="https://example.com/a" TargetMode="External"/><Relationship Id="layout" Type="` + artifactOfficeRelNS + `/slideLayout" Target="../slideLayouts/layout.xml"/></Relationships>`,
		"ppt/media/image.png":              "image1", "ppt/slideLayouts/layout.xml": "layout1",
	}
	read := func() map[string]string {
		t.Helper()
		z, files, e := openArtifact(artifactTestZip(t, entries))
		if e != nil {
			t.Fatal(e)
		}
		defer z.Close()
		r, e := phase3SlideRelationValues(files, "ppt/slides/slide1.xml")
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	one := read()
	entries["ppt/media/image.png"] = "image2"
	two := read()
	if one["img"] == two["img"] || one["__layout"] == "" || one["link"] == "" {
		t.Fatal("relationship content not proved")
	}
	n, e := parseArtifactXML([]byte(`<a:blip xmlns:a="` + artifactDrawingNS + `" xmlns:r="` + artifactOfficeRelNS + `" r:embed="img"/>`))
	if e != nil {
		t.Fatal(e)
	}
	a, e := phase3SlideCanonical(n, one)
	if e != nil {
		t.Fatal(e)
	}
	b, e := phase3SlideCanonical(n, two)
	if e != nil {
		t.Fatal(e)
	}
	if reflect.DeepEqual(a, b) {
		t.Fatal("referenced media replacement missed")
	}
	if _, e := phase3SlideCanonical(n, map[string]string{}); e == nil {
		t.Fatal("missing relationship accepted")
	}
}
func TestPhase3SlideInvalidStructure(t *testing.T) {
	b := []phase3SlidePage{{ID: "only", Content: "x"}}
	for _, s := range []phase3SlidesSpec{{Action: "slide-delete", SlideID: "only"}, {Action: "slide-add", SlideID: "missing"}, {Action: "unknown", SlideID: "only"}} {
		if e := verifyPhase3Slides(b, b, s); e == nil {
			t.Fatalf("invalid accepted %+v", s)
		}
	}
	hidden := []phase3SlidePage{{ID: "only", Content: "x", Hidden: true}}
	if e := verifyPhase3Slides(hidden, hidden, phase3SlidesSpec{Action: "slide-hide", SlideID: "only"}); e == nil {
		t.Fatal("idempotent hide claimed mutation")
	}
	if e := verifyPhase3Slides([]phase3SlidePage{{ID: ""}}, b, phase3SlidesSpec{Action: "slide-add", SlideID: "only"}); e == nil {
		t.Fatal("invalid before ids")
	}
}

func TestPhase3SlideMutationFailureKeepsReplayWarning(t *testing.T) {
	original := fmt.Errorf("fresh reopen timed out")
	if got := phase3SlideMutationError(original, false, "/evidence"); got != original {
		t.Fatal("prewrite failure changed")
	}
	got := phase3SlideMutationError(original, true, "/evidence")
	for _, part := range []string{"可能已生效", "禁止自动重放", "/evidence", "fresh reopen timed out"} {
		if !strings.Contains(got.Error(), part) {
			t.Fatalf("postwrite failure missing %s: %v", part, got)
		}
	}
	if phase3SlideMutationError(nil, true, "/evidence") != nil {
		t.Fatal("success turned into failure")
	}
}
