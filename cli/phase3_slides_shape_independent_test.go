package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// A title placeholder may inherit its entire transform from its layout. Color
// and stacking edits must not need or materialize a local transform.
const inheritedShapeTarget = `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title 1"/><p:nvPr><p:ph type="ctrTitle"/></p:nvPr></p:nvSpPr><p:spPr><a:ln w="12700"><a:solidFill><a:srgbClr val="000000"/></a:solidFill></a:ln></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>Keep title</a:t></a:r></a:p></p:txBody></p:sp>`

func TestPhase3SlideShapeIndependentProperties(t *testing.T) {
	anchor := strings.ReplaceAll(releaseFlowOriginal, "Original", "Anchor")
	for _, tc := range []struct {
		name, property, after string
	}{
		{"fill", `"fillColor":"#0000FF"`, anchor + strings.Replace(inheritedShapeTarget, `<p:spPr>`, `<p:spPr><a:solidFill><a:srgbClr val="0000FF"/></a:solidFill>`, 1)},
		{"line", `"lineColor":"#0000FF"`, anchor + strings.Replace(inheritedShapeTarget, `val="000000"`, `val="0000FF"`, 1)},
		{"layer", `"behindName":"Anchor"`, inheritedShapeTarget + anchor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, fault := range []string{"", "wrong-result", "changed-text", "changed-geometry", "changed-other-page"} {
				t.Run(fault, func(t *testing.T) {
					before := anchor + inheritedShapeTarget
					after, other := tc.after, releaseFlowOriginal
					switch fault {
					case "wrong-result":
						after = before
					case "changed-text":
						after = strings.Replace(after, "Keep title", "Lost title", 1)
					case "changed-geometry":
						after = strings.Replace(after, `<p:spPr>`, `<p:spPr><a:xfrm><a:off x="1" y="2"/><a:ext cx="100" cy="100"/></a:xfrm>`, 1)
					case "changed-other-page":
						other = strings.Replace(other, "original", "changed", 1)
					}
					f := &releaseSlideFlow{beforeIDs: []string{"s1", "s2"}, afterIDs: []string{"s1", "s2"}, before: releaseFlowPPTX(t, []string{before, releaseFlowOriginal}, -1), after: releaseFlowPPTX(t, []string{after, other}, -1), png: releaseFlowPNG(t)}
					f.install(t)
					out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", "slide-shape-format", "--payload", `{"slideId":"s1","shapeName":"Title 1",` + tc.property + `}`, "--out", t.TempDir(), "--json"}).Run)
					if f.writes.Load() != 1 {
						t.Fatalf("independent %s should submit once, writes=%d: %v", tc.name, f.writes.Load(), err)
					}
					if fault != "" {
						if err == nil || strings.Contains(out, `"verified": true`) {
							t.Fatalf("accepted %s: %s", fault, out)
						}
						return
					}
					var result map[string]any
					if err != nil || json.Unmarshal([]byte(out), &result) != nil || result["verified"] != true || f.opens != 2 {
						t.Fatalf("independent property failed: err=%v opens=%d result=%s", err, f.opens, out)
					}
				})
			}
		})
	}
}

func TestPhase3SlideShapeOnlyCenterNeedsPageWidth(t *testing.T) {
	entries := releaseMediaEntries(t, releaseFlowPPTX(t, []string{releaseShapeTarget}, -1))
	entries["ppt/presentation.xml"] = strings.Replace(entries["ppt/presentation.xml"], `<p:sldSz cx="12192000" cy="6858000"/>`, "", 1)
	filename := artifactTestZip(t, entries)
	for _, tc := range []struct {
		name      string
		shape     phase3SlideShapeSpec
		wantError bool
	}{
		{"fill", phase3SlideShapeSpec{Name: "Target", FillColor: "#0000FF"}, false},
		{"rotation", phase3SlideShapeSpec{Name: "Target", Rotation: ptrSlideNumber(45)}, false},
		{"center", phase3SlideShapeSpec{Name: "Target", HorizontalCenter: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := phase3SlidesSpec{SlideID: "s1", Shape: &tc.shape}
			if err := phase3PrepareSlideShape(filename, []string{"s1"}, &s); (err != nil) != tc.wantError {
				t.Fatalf("error=%v wantError=%v", err, tc.wantError)
			}
		})
	}
}
