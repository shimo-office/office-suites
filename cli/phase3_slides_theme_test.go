package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestPhase3SlideThemeSpec(t *testing.T) {
	s, e := preparePhase3Slides("slide-theme", map[string]any{"themeId": "official-id"})
	if e != nil || s.Theme == nil || s.Theme.ID != "official-id" {
		t.Fatalf("%+v %v", s, e)
	}
	for _, p := range []map[string]any{{}, {"themeId": ""}, {"themeId": "a", "scope": "selected"}, {"themeId": "a b"}} {
		if _, e := preparePhase3Slides("slide-theme", p); e == nil {
			t.Fatal("invalid theme accepted")
		}
	}
}
func TestPhase3SlideThemeNativeProof(t *testing.T) {
	expected := map[string]any{"name": "Stone", "themeElements": map[string]any{"clrScheme": map[string]any{"accent1": "blue"}, "fontScheme": map[string]any{"majorFont": "title"}, "fmtScheme": map[string]any{"fill": "solid"}}}
	s := phase3SlideThemeSpec{ID: "official", Expected: expected, LayoutCount: 10}
	good := map[string]any{"slides": []any{map[string]any{"id": "a", "themeId": "official", "layoutCount": float64(10), "theme": expected}}}
	if e := phase3VerifySlideThemeNative(good, []string{"a"}, s); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(map[string]any){func(m map[string]any) { m["themeId"] = "wrong" }, func(m map[string]any) { m["layoutCount"] = float64(9) }, func(m map[string]any) {
		m["theme"] = map[string]any{"name": "Stone", "themeElements": map[string]any{"clrScheme": map[string]any{"accent1": "blue"}}}
	}} {
		slide := map[string]any{"id": "a", "themeId": "official", "layoutCount": float64(10), "theme": expected}
		change(slide)
		if e := phase3VerifySlideThemeNative(map[string]any{"slides": []any{slide}}, []string{"a"}, s); e == nil {
			t.Fatal("superficial theme proof accepted")
		}
	}
}
func TestPhase3SlideThemePreservesLocalContent(t *testing.T) {
	raw := `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="1"/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr><a:xfrm><a:off x="1" y="2"/></a:xfrm><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></p:spPr><p:txBody><a:p><a:r><a:t>keep text</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
	canonical := func(s string) any {
		n, e := parseArtifactXML([]byte(s))
		if e != nil {
			t.Fatal(e)
		}
		v, e := phase3SlideCanonical(phase3MaskThemeGeometry(n, map[string]bool{}), map[string]string{})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	base := canonical(raw)
	if reflect.DeepEqual(base, canonical(strings.Replace(raw, `x="1"`, `x="9"`, 1))) {
		t.Fatal("unproven placeholder movement ignored")
	}
	for _, bad := range []string{strings.Replace(raw, "keep text", "lost text", 1), strings.Replace(raw, "FF0000", "000000", 1), strings.Replace(raw, `type="title"`, `type="body"`, 1)} {
		if reflect.DeepEqual(base, canonical(bad)) {
			t.Fatal("template destroyed local content undetected")
		}
	}
	fixed := strings.Replace(raw, `<p:ph type="title"/>`, "", 1)
	if reflect.DeepEqual(canonical(fixed), canonical(strings.Replace(fixed, `x="1"`, `x="9"`, 1))) {
		t.Fatal("nonplaceholder movement ignored")
	}
}
