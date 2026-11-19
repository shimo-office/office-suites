package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestPhase3SlideThemeDeltaPreservesNonIdentityText(t *testing.T) {
	ops := func(text string, size float64) any {
		return []any{map[string]any{"action": "insert", "attributes": map[string]any{"opType": "attrs"}, "data": map[string]any{"object": map[string]any{"themeRef": "source-theme", "name": text, "fontSize": size}}}}
	}
	base, e := phase3CanonicalThemePart(ops("source-theme", float64(20)), "master", "official", "source-theme", "master", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, candidate := range []any{ops("$theme", 20), ops("source-theme", 30)} {
		value, e := phase3CanonicalThemePart(candidate, "master", "official", "source-theme", "master", nil, nil)
		if e != nil {
			t.Fatal(e)
		}
		if reflect.DeepEqual(base, value) {
			t.Fatal("ordinary text/default font size normalized away")
		}
	}
}
func TestPhase3SlideThemeDecodedSource(t *testing.T) {
	path := os.Getenv("SHIMO_PHASE3_THEME_DECODED_SOURCE")
	if path == "" {
		t.Skip("optional offline official bundle replay")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var decoded map[string]any
	if e = json.Unmarshal(raw, &decoded); e != nil {
		t.Fatal(e)
	}
	layouts, e := phase3ThemeMap(decoded["layouts"])
	if e != nil {
		t.Fatal(e)
	}
	assets := map[string]string{"3BF5309F-9072-6A02-2C6E-738F7CF5F405": "poster-hash", "BCE7133A-8C0A-B48B-B732-8EFD750F3F2D": "background-hash"}
	s := phase3SlideThemeSpec{ID: "official", ImageKeys: assets}
	e = phase3PrepareThemeDelta(&s, decoded, []any{map[string]any{"id": "a", "definition": layouts["l0000000"]}})
	if e != nil {
		t.Fatal(e)
	}
	if s.LayoutCount != 13 || len(s.Layouts) != 13 || s.SlideLayouts["a"] != 0 {
		t.Fatalf("incorrect decoded layout source: %+v", s.SlideLayouts)
	}
}

func TestPhase3SlideThemeGeometryOnlyExactSource(t *testing.T) {
	old := phase3ThemeTransform{X: 10, Y: 20, CX: 100, CY: 50}
	want := phase3ThemeTransform{X: 50, Y: 60, CX: 200, CY: 100}
	key := "title:0"
	before := map[string]*phase3ThemeTransform{key: &old}
	expected := map[string]phase3ThemeTransform{key: want}
	for _, after := range []*phase3ThemeTransform{&want, nil} {
		allowed, e := phase3ThemeAllowedGeometry(before, map[string]*phase3ThemeTransform{key: after}, expected)
		if e != nil || !allowed[key] {
			t.Fatalf("source geometry rejected: %v", e)
		}
	}
	bad := want
	bad.X = 99999999
	if _, e := phase3ThemeAllowedGeometry(before, map[string]*phase3ThemeTransform{key: &bad}, expected); e == nil {
		t.Fatal("arbitrary off-canvas movement accepted")
	}
	bad = want
	bad.FlipH = true
	if _, e := phase3ThemeAllowedGeometry(before, map[string]*phase3ThemeTransform{key: &bad}, expected); e == nil {
		t.Fatal("unrequested flip accepted")
	}
	if _, e := phase3ThemeAllowedGeometry(before, map[string]*phase3ThemeTransform{key: nil}, map[string]phase3ThemeTransform{}); e == nil {
		t.Fatal("missing source layout geometry accepted")
	}
}
