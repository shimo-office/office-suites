package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPhase3SlideThemeNativeDefaultsAreNarrow(t *testing.T) {
	grad := func(rot any) map[string]any {
		data := map[string]any{"colors": []any{map[string]any{"pos": float64(0)}}}
		if rot != nil {
			data["rotWithShape"] = rot
		}
		return map[string]any{"fill": map[string]any{"type": float64(4), "data": data}}
	}
	theme := func(rot any) map[string]any {
		return map[string]any{"themeElements": map[string]any{"fmtScheme": map[string]any{"fillStyleLst": []any{grad(rot)}}}}
	}
	if !reflect.DeepEqual(phase3ThemeNativeDefaults(theme(nil)), phase3ThemeNativeDefaults(theme(true))) {
		t.Fatal("native gradient constructor default differs")
	}
	if reflect.DeepEqual(phase3ThemeNativeDefaults(theme(nil)), phase3ThemeNativeDefaults(theme(false))) {
		t.Fatal("explicit gradient false ignored")
	}
	if reflect.DeepEqual(phase3ThemeNativeDefaults(map[string]any{"rotWithShape": true}), phase3ThemeNativeDefaults(map[string]any{})) {
		t.Fatal("unscoped default ignored")
	}
}
func TestPhase3SlideThemeNativeFontReferences(t *testing.T) {
	expected := map[string]any{"themeElements": map[string]any{"fontScheme": map[string]any{"minorFont": map[string]any{"ea": map[string]any{"typeface": "Source Han Sans CN Light"}, "cs": map[string]any{"typeface": ""}}}}}
	value := func(font string) any {
		return map[string]any{"defaultRunPr": map[string]any{"latin": map[string]any{"typeface": font}}, "text": "+mn-ea"}
	}
	a := phase3ThemeResolvedFonts(value("+mn-ea"), expected)
	if !reflect.DeepEqual(a, phase3ThemeResolvedFonts(value("Source Han Sans CN Light"), expected)) {
		t.Fatal("source font reference not resolved")
	}
	if reflect.DeepEqual(a, phase3ThemeResolvedFonts(value("思源黑体"), expected)) {
		t.Fatal("lossy localized export name treated as native equivalent")
	}
	if reflect.DeepEqual(a, phase3ThemeResolvedFonts(value("Source Han Sans CN Medium"), expected)) {
		t.Fatal("font weight change ignored")
	}
	if a.(map[string]any)["text"] != "+mn-ea" {
		t.Fatal("ordinary text changed")
	}
}
func TestPhase3SlidesRecordedThemeProof(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_SLIDES_THEME_RECORDED")
	if source == "" {
		t.Skip("recorded theme artifact proof")
	}
	var spec phase3SlideThemeSpec
	raw, err := os.ReadFile(filepath.Join(source, "theme-expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	fresh := os.Getenv("SHIMO_PHASE3_SLIDES_THEME_FRESH")
	raw, err = os.ReadFile(filepath.Join(fresh, "reload-theme.json"))
	if err != nil {
		t.Fatal(err)
	}
	var native map[string]any
	if err = json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	ids := []string{"s0000000", "s0000001", "s0000002"}
	if err = phase3VerifySlideThemeNative(native, ids, spec); err != nil {
		t.Fatal("native", err)
	}
	if err = phase3VerifyThemeDelta(native["delta"], spec); err != nil {
		t.Fatal("delta", err)
	}
	before, err := filepath.Glob(filepath.Join(source, "before", "*.pptx"))
	if err != nil || len(before) != 1 {
		t.Fatal(err)
	}
	after, err := filepath.Glob(filepath.Join(fresh, "fresh", "*.pptx"))
	if err != nil || len(after) != 1 {
		t.Fatal(err)
	}
	if err = verifyPhase3SlideThemeArtifacts(before[0], after[0], ids, ids, phase3SlidesSpec{Theme: &spec}); err != nil {
		rb, ra, e := phase3ThemeReconciledPages(before[0], after[0])
		if e == nil {
			b, _, _ := phase3ReadThemeLocalPages(before[0], ids, nil, nil, rb)
			a, _, _ := phase3ReadThemeLocalPages(after[0], ids, &spec, nil, ra)
			masks := map[string]map[string]bool{}
			for i := range b {
				mask, _ := phase3ThemeAllowedGeometry(b[i].Transforms, a[i].Transforms, spec.Geometry[spec.SlideLayouts[ids[i]]])
				masks[ids[i]] = mask
			}
			b, _, _ = phase3ReadThemeLocalPages(before[0], ids, nil, masks, rb)
			a, _, _ = phase3ReadThemeLocalPages(after[0], ids, &spec, masks, ra)
			_ = phase3WriteEvidence("../tmp/phase3-live/slides/theme-local-diff.json", map[string]any{"before": b, "after": a})
		}
		t.Fatal("artifact", err)
	}
}
