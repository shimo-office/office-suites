package main

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestPhase3SlideReplaceSpec(t *testing.T) {
	s, e := preparePhase3Slides("slide-replace-all", map[string]any{"old": "旧版测试", "new": "新版测试"})
	if e != nil {
		t.Fatal(e)
	}
	if s.Replace == nil {
		t.Fatal("missing replace")
	}
	for _, p := range []map[string]any{{"old": "", "new": "new"}, {"old": "old", "new": "old"}, {"old": "x", "new": "y", "regex": true}} {
		if _, e := preparePhase3Slides("slide-replace-all", p); e == nil {
			t.Fatal("invalid replacement accepted")
		}
	}
}
func TestPhase3SlideReplaceNodePreservesStyles(t *testing.T) {
	n, e := parseArtifactXML([]byte(`<a:p xmlns:a="` + artifactDrawingNS + `"><a:r><a:rPr b="1"/><a:t>old old</a:t></a:r></a:p>`))
	if e != nil {
		t.Fatal(e)
	}
	s := phase3SlideReplaceSpec{Old: "old", New: "new"}
	counts, e := phase3SlideReplaceCounts(n, s)
	if e != nil {
		t.Fatal(e)
	}
	if counts[0] != 2 || counts[1] != 2 {
		t.Fatal(counts)
	}
	changed := phase3SlideReplaceNode(n, s)
	if phase3SlideText(changed) != "new new" || phase3SlideText(n) != "old old" {
		t.Fatal("replacement mutated baseline")
	}
	split, _ := parseArtifactXML([]byte(`<a:p xmlns:a="` + artifactDrawingNS + `"><a:r><a:t>ol</a:t></a:r><a:r><a:t>d</a:t></a:r></a:p>`))
	if _, e := phase3SlideReplaceCounts(split, s); e == nil {
		t.Fatal("cross-run replacement should be rejected before write")
	}
}
func TestPhase3SlideReplaceSingleMutation(t *testing.T) {
	node, e := exec.LookPath("node")
	if e != nil {
		t.Skip("node only for JS regression")
	}
	for _, count := range []int{1, 2} {
		s := phase3SlidesSpec{Replace: &phase3SlideReplaceSpec{Old: "old", New: "new", Count: count, NewCount: count}}
		want := 0
		if count == 1 {
			want = 1
		}
		script := fmt.Sprintf(`let writes=0,text='old';global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:[1]}),presentationSDK:{editor:{workbench:{isReady:()=>true,permissions:{canEdit:()=>true},context:{getUser:()=>({id:1})},core:{getSlideTextSummary:()=>[{text}],replace:(ed,p)=>{writes++;text=text.replaceAll(p.text,p.replaceWith);return {replaceNum:1}}}}},API:{slides:{getSlides:()=>[{id:'a'}]}}}};const r=%s;if(writes!==%d)throw new Error('wrong write count');if(writes===1&&r.oldCount!==0)throw new Error('old remains');if(writes===0&&r.ok!==false)throw new Error('missing mismatch rejection');`, phase3SlideReplaceMutationJS(s, []string{"a"}), want)
		if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
			t.Fatalf("%v %s", e, out)
		}
	}
}

func TestPhase3SlideReplaceNativeDefaultsStayOnOriginalTargets(t *testing.T) {
	before := phase3PlaceholderBWArtifact(t, `<a:r><a:rPr b="1"/><a:t>old</a:t></a:r>`, "", "", "")
	spec := phase3SlidesSpec{Replace: &phase3SlideReplaceSpec{Old: "old", New: "new", Count: 1, NewCount: 1}}
	goodText := `<a:pPr/><a:r><a:rPr b="1"/><a:t>new</a:t></a:r><a:endParaRPr/>`
	good := phase3PlaceholderBWArtifact(t, goodText, `bwMode="auto"`, "", "")
	if e := verifyPhase3SlideReplaceArtifacts(before, good, []string{"a"}, []string{"a"}, spec); e != nil {
		t.Fatal(e)
	}
	for name, after := range map[string]string{
		"other object BW":           phase3PlaceholderBWArtifact(t, goodText, `bwMode="auto"`, `bwMode="auto"`, ""),
		"inherited BW":              phase3PlaceholderBWArtifact(t, goodText, `bwMode="auto"`, "", `bwMode="black"`),
		"font changed":              phase3PlaceholderBWArtifact(t, `<a:pPr/><a:r><a:rPr b="0"/><a:t>new</a:t></a:r><a:endParaRPr/>`, `bwMode="auto"`, "", ""),
		"nonempty end style":        phase3PlaceholderBWArtifact(t, `<a:pPr/><a:r><a:rPr b="1"/><a:t>new</a:t></a:r><a:endParaRPr sz="3000"/>`, `bwMode="auto"`, "", ""),
		"duplicate paragraph style": phase3PlaceholderBWArtifact(t, `<a:pPr/><a:pPr/><a:r><a:rPr b="1"/><a:t>new</a:t></a:r><a:endParaRPr/>`, `bwMode="auto"`, "", ""),
	} {
		if e := verifyPhase3SlideReplaceArtifacts(before, after, []string{"a"}, []string{"a"}, spec); e == nil {
			t.Fatal("collateral mutation accepted", name)
		}
	}
}

func TestPhase3SlideReplaceDoesNotTargetCrossParagraphSubstring(t *testing.T) {
	rewrite := func(file string) string {
		z, entries, e := openArtifact(file)
		if e != nil {
			t.Fatal(e)
		}
		defer z.Close()
		parts := map[string]string{}
		for name, entry := range entries {
			b, e := zipEntryBytes(entry)
			if e != nil {
				t.Fatal(e)
			}
			s := string(b)
			if name == "ppt/slides/one.xml" {
				s = strings.Replace(s, `<a:p><a:r><a:t>keep</a:t></a:r></a:p>`, `<a:p><a:r><a:t>ol</a:t></a:r></a:p><a:p><a:r><a:t>d</a:t></a:r></a:p>`, 1)
			}
			parts[name] = s
		}
		return artifactTestZip(t, parts)
	}
	before := rewrite(phase3PlaceholderBWArtifact(t, `<a:r><a:t>old</a:t></a:r>`, "", "", ""))
	after := rewrite(phase3PlaceholderBWArtifact(t, `<a:pPr/><a:r><a:t>new</a:t></a:r><a:endParaRPr/>`, `bwMode="auto"`, `bwMode="auto"`, ""))
	spec := phase3SlidesSpec{Replace: &phase3SlideReplaceSpec{Old: "old", New: "new"}}
	if e := phase3PrepareSlideReplace(before, []string{"a"}, &spec); e != nil {
		t.Fatal("valid baseline rejected", e)
	}
	if spec.Replace.Count != 1 {
		t.Fatal("unexpected count", spec.Replace.Count)
	}
	if e := verifyPhase3SlideReplaceArtifacts(before, after, []string{"a"}, []string{"a"}, spec); e == nil {
		t.Fatal("BW mutation on non-target shape with cross-paragraph old substring accepted")
	}
}
