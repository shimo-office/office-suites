package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPhase3SlideThemeShadowRoundTripIsSourceBounded(t *testing.T) {
	part := func(scope, effectType, name string, blur, dist float64, color string) any {
		raw := fmt.Sprintf(`[{"action":"insert","attributes":{"opType":"spPr"},"data":{"object":{"%s":{"extra":{"effects":{"type":%q,"data":[{"name":%q,"blurRad":%v,"dist":%v,"color":{"color":{"type":1,"val":%q}}}]}}}}}}]`, scope, effectType, name, blur, dist, color)
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, role := range []string{"master", "layout"} {
		for _, tc := range []struct {
			name, scope, effectType, shadow string
			expected, actual, dist          float64
			color                           string
			want                            bool
		}{
			{"exact", "spPr", "effectLst", "outerShdw", 365269, 365269, 19050, "000000", true},
			{"native serialization", "spPr", "effectLst", "outerShdw", 365269, 365268, 19050, "000000", true},
			{"wrong plus one", "spPr", "effectLst", "outerShdw", 365269, 365270, 19050, "000000", false},
			{"wrong minus one", "spPr", "effectLst", "outerShdw", 365270, 365269, 19050, "000000", false},
			{"repeated loss", "spPr", "effectLst", "outerShdw", 365269, 365267, 19050, "000000", false},
			{"reverse normalization", "spPr", "effectLst", "outerShdw", 365268, 365269, 19050, "000000", false},
			{"wrong scope", "defaultRunPr", "effectLst", "outerShdw", 365269, 365268, 19050, "000000", false},
			{"wrong effect list", "spPr", "effectDag", "outerShdw", 365269, 365268, 19050, "000000", false},
			{"wrong effect", "spPr", "effectLst", "innerShdw", 365269, 365268, 19050, "000000", false},
			{"distance changed", "spPr", "effectLst", "outerShdw", 365269, 365268, 19049, "000000", false},
			{"color changed", "spPr", "effectLst", "outerShdw", 365269, 365268, 19050, "FFFFFF", false},
			{"fractional source", "spPr", "effectLst", "outerShdw", 365269.5, 365269, 19050, "000000", false},
			{"negative source", "spPr", "effectLst", "outerShdw", -1, 0, 19050, "000000", false},
			{"outside signed range", "spPr", "effectLst", "outerShdw", 2147483648, -2147483648, 19050, "000000", false},
		} {
			t.Run(role+"/"+tc.name, func(t *testing.T) {
				expected := part(tc.scope, tc.effectType, tc.shadow, tc.expected, 19050, "000000")
				actual := part(tc.scope, tc.effectType, tc.shadow, tc.actual, tc.dist, tc.color)
				spec := phase3SlideThemeSpec{ID: "theme", LayoutCount: 1, Master: []any{}, Layouts: []any{[]any{}}}
				native := map[string]any{"masterRef": "m", "themeRef": "t", "layoutRefs": []any{"l"}, "master": []any{}, "layouts": []any{[]any{}}}
				if role == "master" {
					spec.Master, native["master"] = expected, actual
				} else {
					spec.Layouts, native["layouts"] = []any{expected}, []any{actual}
				}
				expectedCopy, actualCopy := phase3ThemeCopyJSON(expected), phase3ThemeCopyJSON(actual)
				err := phase3VerifyThemeDelta(native, spec)
				if (err == nil) != tc.want {
					t.Fatalf("want accepted=%v, got %v", tc.want, err)
				}
				if !reflect.DeepEqual(expected, expectedCopy) || !reflect.DeepEqual(actual, actualCopy) {
					t.Fatal("comparison mutated source or actual tree")
				}
			})
		}
	}
}

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
	raw, err = os.ReadFile(filepath.Join(source, "before-readback.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pages []phase3SlidePage
	if err = json.Unmarshal(raw, &pages); err != nil || len(pages) == 0 {
		t.Fatalf("recorded page IDs missing: %v", err)
	}
	ids := make([]string, len(pages))
	for i, page := range pages {
		ids[i] = page.ID
	}
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
	after := []string{os.Getenv("SHIMO_PHASE3_SLIDES_THEME_OFFLINE_ARTIFACT")}
	if after[0] == "" {
		after, err = filepath.Glob(filepath.Join(fresh, "fresh", "*.pptx"))
	}
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
