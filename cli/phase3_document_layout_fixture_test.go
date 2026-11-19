package main

import (
	"os"
	"reflect"
	"testing"
)

func TestPhase3DocumentLayoutThemeFixtureInheritance(t *testing.T) {
	file := os.Getenv("SHIMO_PHASE3_THEME_FIXTURE")
	if file == "" {
		t.Skip("explicit local fixture inspection")
	}
	s, e := readDocumentLayout(file)
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Text.Paragraphs) != 3 || s.Theme == nil {
		t.Fatal("fixture paragraph/theme shape incorrect")
	}
	body := documentExpanded(s.Text.Paragraphs[1])
	control := documentExpanded(s.Text.Paragraphs[2])
	if len(body) == 0 || len(control) == 0 {
		t.Fatal("missing body/control")
	}
	for _, c := range body {
		if c.Attrs["fontFamily.eastAsiaTheme"] != "minorEastAsia" {
			t.Fatalf("body not inherited: %#v", c.Attrs)
		}
	}
	for _, c := range control {
		if c.Attrs["fontFamily.eastAsia"] != "仿宋" || c.Attrs["fontFamily.eastAsiaTheme"] != "" {
			t.Fatalf("control not explicit: %#v", c.Attrs)
		}
	}
	if s.Theme.Fonts.Minor.EA != "黑体" || s.Theme.Fonts.Major.EA != "宋体" {
		t.Fatal("fixture does not establish a genuine theme change")
	}
	t.Log("verified 3 paragraphs: body inherits minorEastAsia=黑体; explicit control remains 仿宋; heading majorEastAsia=宋体")
}

func TestPhase3DocumentLayoutThemePostFailureBaseline(t *testing.T) {
	file := os.Getenv("SHIMO_PHASE3_THEME_BASELINE")
	if file == "" {
		t.Skip("explicit independent post-failure baseline export")
	}
	baseline, err := readDocumentLayout(file)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := readDocumentLayout("../tmp/phase3-live/document-layout/cli-batch6/theme-20261118T193314235944Z/evidence/phase3-2186607165/after-4/（自检）三期-主题继承-20261119.docx")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(previous, baseline) {
		t.Fatal("independent baseline differs from last post-failure export")
	}
	if baseline.Theme.Fonts.Major.EA != "黑体" || baseline.Theme.Fonts.Minor.EA != "宋体" || len(baseline.Text.Paragraphs) != 3 {
		t.Fatal("unexpected applied state")
	}
	for _, c := range documentExpanded(baseline.Text.Paragraphs[1]) {
		resolved, err := documentThemeEffectiveFonts(c.Attrs, baseline.Theme.Fonts)
		if err != nil || c.Attrs["fontFamily.eastAsiaTheme"] != "minorEastAsia" || resolved["eastAsia"] != "宋体" {
			t.Fatal("body no longer actually inherits 宋体", c.Attrs, err)
		}
	}
	for _, c := range documentExpanded(baseline.Text.Paragraphs[2]) {
		resolved, err := documentThemeEffectiveFonts(c.Attrs, baseline.Theme.Fonts)
		if err != nil || c.Attrs["fontFamily.eastAsiaTheme"] != "" || resolved["eastAsia"] != "仿宋" {
			t.Fatal("explicit 仿宋 control differs", c.Attrs, err)
		}
	}
	t.Log("independent full snapshot equals post-failure state, including uiPriority=9; body inherits 宋体, explicit 仿宋 control preserved; ready for explicit reverse request")
}
