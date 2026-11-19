package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPhase3SlideExplicitDefaultsAreEquivalent(t *testing.T) {
	cases := []struct{ ns, element, attribute, defaultValue, changed string }{
		{artifactSlideNS, "cNvPr", "hidden", "0", "1"},
		{artifactSlideNS, "nvPr", "isPhoto", "0", "1"},
		{artifactSlideNS, "nvPr", "userDrawn", "false", "true"},
		{artifactSlideNS, "ph", "hasCustomPrompt", "0", "1"},
		{artifactDrawingNS, "xfrm", "rot", "0", "60000"},
		{artifactDrawingNS, "xfrm", "flipH", "false", "true"},
		{artifactDrawingNS, "xfrm", "flipV", "0", "1"},
	}
	for _, c := range cases {
		t.Run(c.element+"."+c.attribute, func(t *testing.T) {
			canonical := func(ns, attribute, value string) any {
				raw := `<x:` + c.element + ` xmlns:x="` + ns + `"`
				if attribute != "" {
					raw += ` ` + attribute + `="` + value + `"`
				}
				raw += `/>`
				n, e := parseArtifactXML([]byte(raw))
				if e != nil {
					t.Fatal(e)
				}
				v, e := phase3SlideCanonical(n, nil)
				if e != nil {
					t.Fatal(e)
				}
				return v
			}
			plain := canonical(c.ns, "", "")
			if !reflect.DeepEqual(plain, canonical(c.ns, c.attribute, c.defaultValue)) {
				t.Fatalf("explicit schema default must match omission: %s", c.defaultValue)
			}
			if reflect.DeepEqual(plain, canonical(c.ns, c.attribute, c.changed)) {
				t.Fatal("changed semantic property was masked")
			}
			if reflect.DeepEqual(canonical("urn:foreign", "", ""), canonical("urn:foreign", c.attribute, c.defaultValue)) {
				t.Fatal("foreign namespace attribute was masked")
			}
		})
	}
}

func TestPhase3SlidesRecordedPlaceholderDefaults(t *testing.T) {
	dir := os.Getenv("SHIMO_PHASE3_SLIDES_RECORDED_PLACEHOLDER")
	if dir == "" {
		t.Skip("opt-in read-only recorded real PPTX replay")
	}
	var request struct{ Spec phase3SlidesSpec }
	raw, e := os.ReadFile(filepath.Join(dir, "request.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &request); e != nil {
		t.Fatal(e)
	}
	var pages []phase3SlidePage
	raw, e = os.ReadFile(filepath.Join(dir, "before-readback.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &pages); e != nil {
		t.Fatal(e)
	}
	ids := []string{}
	for _, p := range pages {
		ids = append(ids, p.ID)
	}
	before, e := filepath.Glob(filepath.Join(dir, "before", "*.pptx"))
	if e != nil || len(before) != 1 {
		t.Fatal("missing unique before artifact", e)
	}
	after, e := filepath.Glob(filepath.Join(dir, "after-*", "*.pptx"))
	if e != nil || len(after) == 0 {
		t.Fatal("missing actual after artifact", e)
	}
	if e = verifyPhase3SlidePlaceholderArtifacts(before[0], after[len(after)-1], ids, ids, request.Spec); e != nil {
		t.Fatal(e)
	}
}
