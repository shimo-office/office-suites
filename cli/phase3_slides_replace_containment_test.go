package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func slideReplaceRunFixture(t *testing.T, runs []string) string {
	t.Helper()
	var content strings.Builder
	for i, run := range runs {
		fmt.Fprintf(&content, `<a:r><a:rPr sz="%d"/><a:t>%s</a:t></a:r>`, 1200+i*100, run)
	}
	return phase3PlaceholderBWArtifact(t, content.String(), "", "", "")
}

func TestPhase3SlideReplaceContainingTerms(t *testing.T) {
	for _, pair := range [][2]string{{"old", "old-new"}, {"old-new", "old"}, {"a", "aa"}} {
		if _, err := preparePhase3Slides("slide-replace-all", map[string]any{"old": pair[0], "new": pair[1]}); err != nil {
			t.Errorf("valid literal replacement rejected: %v: %v", pair, err)
		}
	}
}

func TestPhase3SlideReplaceParagraphExpectedCounts(t *testing.T) {
	cases := []struct {
		name, old, next string
		runs, after     []string
		counts          [3]int // original old, resulting new, resulting old
	}{
		{"expansion", "old", "old-new", []string{"old old"}, []string{"old-new old-new"}, [3]int{2, 2, 2}},
		{"contraction", "old-new", "old", []string{"old-new old-new"}, []string{"old old"}, [3]int{2, 2, 0}},
		{"new spans unchanged runs", "x", "ab", []string{"x", "a", "b"}, []string{"ab", "a", "b"}, [3]int{1, 2, 0}},
		{"old reformed across runs", "ab", "a", []string{"ab", "b"}, []string{"a", "b"}, [3]int{1, 1, 1}},
		{"adjacent expansions", "a", "aa", []string{"a", "a"}, []string{"aa", "aa"}, [3]int{2, 2, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, after := slideReplaceRunFixture(t, tc.runs), slideReplaceRunFixture(t, tc.after)
			spec := phase3SlidesSpec{Replace: &phase3SlideReplaceSpec{Old: tc.old, New: tc.next}}
			if err := phase3PrepareSlideReplace(before, []string{"a"}, &spec); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(spec.Replace)
			var evidence map[string]any
			if err := json.Unmarshal(raw, &evidence); err != nil {
				t.Fatal(err)
			}
			for i, key := range []string{"expectedOldCount", "expectedNewCount", "expectedRemainingOldCount"} {
				if evidence[key] != float64(tc.counts[i]) {
					t.Errorf("%s = %v, want %d", key, evidence[key], tc.counts[i])
				}
			}
			if err := verifyPhase3SlideReplaceArtifacts(before, after, []string{"a"}, []string{"a"}, spec); err != nil {
				t.Fatal("exact one-pass result rejected:", err)
			}
			for _, bad := range []string{before, slideReplaceRunFixture(t, []string{strings.Join(tc.after, "")})} {
				if bad == before || len(tc.after) > 1 {
					if err := verifyPhase3SlideReplaceArtifacts(before, bad, []string{"a"}, []string{"a"}, spec); err == nil {
						t.Fatal("unchanged result or lost run formatting accepted")
					}
				}
			}
		})
	}
}

func TestPhase3SlideReplaceRejectsCrossRunMatchWithEqualCounts(t *testing.T) {
	// Whole-paragraph search finds the first two a's, but per-run search finds
	// the last two. Equal counts do not prove that formatting will be preserved.
	n, err := parseArtifactXML([]byte(`<a:p xmlns:a="` + artifactDrawingNS + `"><a:r><a:t>a</a:t></a:r><a:r><a:t>aa</a:t></a:r></a:p>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := phase3SlideReplaceCounts(n, phase3SlideReplaceSpec{Old: "aa", New: "a"}); err == nil {
		t.Fatal("cross-run old match accepted because counts happened to agree")
	}
	if _, err := phase3SlideReplaceCounts(n, phase3SlideReplaceSpec{New: "a"}); err == nil {
		t.Fatal("empty old term accepted")
	}
}

func TestPhase3SlideReplaceContainingTermsSinglePassJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional existing Node runtime")
	}
	for _, tc := range []struct {
		old, next, before, after string
		runs                     []string
	}{
		{"old", "old-new", "old old", "old-new old-new", []string{"old old"}},
		{"old-new", "old", "old-new old-new", "old old", []string{"old-new old-new"}},
		{"x", "ab", "xab", "abab", []string{"x", "a", "b"}},
		{"ab", "a", "abb", "ab", []string{"ab", "b"}},
	} {
		for _, fault := range []bool{false, true} {
			s := phase3SlidesSpec{Replace: &phase3SlideReplaceSpec{Old: tc.old, New: tc.next}}
			if err := phase3PrepareSlideReplace(slideReplaceRunFixture(t, tc.runs), []string{"a"}, &s); err != nil {
				t.Fatal(err)
			}
			beforeJSON, _ := json.Marshal(tc.before)
			afterJSON, _ := json.Marshal(tc.after)
			script := fmt.Sprintf(`let writes=0,text=%s;global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:[1]}),presentationSDK:{editor:{workbench:{isReady:()=>true,permissions:{canEdit:()=>true},context:{getUser:()=>({id:1})},core:{getSlideTextSummary:()=>[{text}],replace:(ed,p)=>{writes++;const replaceNum=text.split(p.text).length-1;if(!%t)text=text.replaceAll(p.text,()=>p.replaceWith);return {replaceNum}}}}},API:{slides:{getSlides:()=>[{id:'a'}]}}}};const result=%s;if(writes!==1)throw Error('replacement repeated or not submitted');if(result.ok===%t)throw Error('wrong verification result '+JSON.stringify(result));if(!%t&&text!==%s)throw Error('not literal single-pass replacement');`, beforeJSON, fault, phase3SlideReplaceMutationJS(s, []string{"a"}), fault, fault, afterJSON)
			if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
				t.Fatalf("%s -> %s fault=%t: %v\n%s", tc.old, tc.next, fault, err, out)
			}
		}
	}
}

func TestPhase3SlideReplaceCountsParagraphBoundaries(t *testing.T) {
	n, err := parseArtifactXML([]byte(`<a:txBody xmlns:a="` + artifactDrawingNS + `"><a:p><a:r><a:t>x</a:t></a:r></a:p><a:p><a:r><a:t>a</a:t></a:r></a:p><a:p><a:r><a:t>b</a:t></a:r></a:p></a:txBody>`))
	if err != nil {
		t.Fatal(err)
	}
	counts, err := phase3SlideReplaceCounts(n, phase3SlideReplaceSpec{Old: "x", New: "ab"})
	if err != nil || !reflect.DeepEqual(counts, [3]int{1, 1, 0}) {
		t.Fatalf("counted new term across separate paragraphs: %v %v", counts, err)
	}
}

func TestReleaseSlideReplaceContainingTermsFlow(t *testing.T) {
	for _, next := range []string{"original-expanded", "orig"} {
		for _, fault := range []string{"", "unchanged", "font"} {
			t.Run(next+"/"+fault, func(t *testing.T) {
				after := strings.ReplaceAll(releaseFlowOriginal, "original", next)
				if fault == "unchanged" {
					after = releaseFlowOriginal
				} else if fault == "font" {
					after = strings.Replace(after, "<a:r>", `<a:r><a:rPr b="1"/>`, 1)
				}
				f := &releaseSlideFlow{beforeIDs: []string{"s1"}, afterIDs: []string{"s1"}, before: releaseFlowPPTX(t, []string{releaseFlowOriginal}, -1), after: releaseFlowPPTX(t, []string{after}, -1), png: releaseFlowPNG(t)}
				f.install(t)
				payload, _ := json.Marshal(map[string]string{"old": "original", "new": next})
				out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", "slide-replace-all", "--payload", string(payload), "--out", t.TempDir(), "--json"}).Run)
				if f.writes.Load() != 1 {
					t.Fatalf("expected one mutation, got %d: %v", f.writes.Load(), err)
				}
				if fault != "" {
					if err == nil {
						t.Fatalf("incorrect replacement accepted: %s", out)
					}
					return
				}
				var result map[string]any
				if err != nil || json.Unmarshal([]byte(out), &result) != nil || result["verified"] != true || f.opens != 2 {
					t.Fatalf("single-pass replacement not verified: %v %s", err, out)
				}
			})
		}
	}
}
