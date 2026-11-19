package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPhase3DocumentLayoutBackgroundPostFailureBaseline(t *testing.T) {
	file := os.Getenv("SHIMO_PHASE3_BACKGROUND_BASELINE")
	if file == "" {
		t.Skip("explicit independent post-failure background baseline export")
	}
	base := "../tmp/phase3-live/document-layout/background-root-network/evidence/phase3-1154542018"
	before, err := readDocumentLayout(filepath.Join(base, "before/（自检）三期-主题继承-20261119.docx"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := readDocumentLayout(filepath.Join(base, "after-4/（自检）三期-主题继承-20261119.docx"))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := readDocumentLayout(file)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, baseline) {
		t.Fatal("independent baseline differs from last failed-action export")
	}
	if err := verifyDocumentLayout(before, after, documentLayoutSpec{Action: "document-page", Background: "#EAF3FF"}); err == nil || !strings.Contains(err.Error(), "非目标页面属性变化") {
		t.Fatalf("original collateral page change must remain rejected: %v", err)
	}
	if baseline.Background != "EAF3FF" || !reflect.DeepEqual(before.Text, baseline.Text) || !reflect.DeepEqual(before.Theme.Fonts, baseline.Theme.Fonts) || before.Theme.RawStyles != baseline.Theme.RawStyles || before.Theme.RawTheme != baseline.Theme.RawTheme {
		t.Fatal("background/font/text/style baseline unexpected")
	}
	if baseline.Theme.Fonts.Major.EA != "黑体" || baseline.Theme.Fonts.Minor.EA != "宋体" || len(baseline.Text.Paragraphs) != 3 {
		t.Fatal("target theme/paragraph count changed")
	}
	for _, c := range documentExpanded(baseline.Text.Paragraphs[1]) {
		fonts, err := documentThemeEffectiveFonts(c.Attrs, baseline.Theme.Fonts)
		if err != nil || fonts["eastAsia"] != "宋体" || c.Attrs["fontFamily.eastAsiaTheme"] != "minorEastAsia" {
			t.Fatal("body theme inheritance changed")
		}
	}
	for _, c := range documentExpanded(baseline.Text.Paragraphs[2]) {
		fonts, err := documentThemeEffectiveFonts(c.Attrs, baseline.Theme.Fonts)
		if err != nil || fonts["eastAsia"] != "仿宋" || c.Attrs["fontFamily.eastAsiaTheme"] != "" {
			t.Fatal("explicit font control changed")
		}
	}
	t.Log("independent baseline equals failed-action export; EAF3FF present; native page additions remain strict failure; target theme and explicit 仿宋 unchanged")
}
