package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPhase3SlideGeneratedJavaScriptSyntax(t *testing.T) {
	node, e := exec.LookPath("node")
	if e != nil {
		t.Skip("optional existing Node parser; CLI has no Node dependency")
	}
	cases := map[string]string{
		"outline-write": phase3SlideTextFormatMutationJS(phase3SlidesSpec{SlideID: "a", Format: &phase3SlideFormatSpec{Text: "title", Attrs: map[string]any{"outlineColor": "#FF0000", "outlineWidth": float64(1)}}}, []string{"a"}),
		"theme-read":    phase3SlideThemeNativeJS(), "theme-reload": phase3SlideThemeReloadJS(),
		"theme-write": phase3SlideThemeMutationJS(phase3SlidesSpec{Theme: &phase3SlideThemeSpec{ID: "fixture"}}, []string{"a"}),
		"media-write": phase3SlideMediaMutationJS(phase3SlidesSpec{SlideID: "a", Media: &phase3SlideMediaSpec{Kind: "audio", Name: "file.wav"}}, []string{"a"}),
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(node, "--check")
			cmd.Stdin = strings.NewReader(script)
			if output, e := cmd.CombinedOutput(); e != nil {
				t.Fatalf("generated browser JavaScript is invalid: %v\n%s", e, output)
			}
		})
	}
}
