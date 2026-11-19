package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Read-only re-verification of immutable exports retained from the first live
// batch. This is never reported as a successful rerun of the original command.
func TestPhase3DocumentLayoutRecordedProof(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_LAYOUT_RECORDED") != "1" {
		t.Skip("explicit local recorded evidence check")
	}
	root := "../tmp/phase3-live/document-layout"
	read := func(part string) documentLayoutSnapshot {
		t.Helper()
		x, e := readDocumentLayout(filepath.Join(root, part, "（自检）三期-文稿布局.docx"))
		if e != nil {
			t.Fatal(e)
		}
		return x
	}
	numberBefore := read("number1/phase3-1838161215/before")
	numberAfter := read("number1/phase3-1838161215/after-4")
	pageBefore := read("page/phase3-1862393371/before")
	pageAfter := read("page/phase3-1862393371/after-4")
	replacement := read("replace/phase3-913006079/reload")
	navigation := read("goto/phase3-2944066916/after")
	if e := verifyDocumentLayout(numberBefore, numberAfter, documentLayoutSpec{Action: "document-paragraph", Paragraph: 3, Text: "编号测试甲", Attrs: map[string]any{"numbering": "decimal"}}); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(numberAfter, pageBefore) {
		t.Fatal("numbering-to-page chain differs")
	}
	if e := verifyDocumentLayout(pageBefore, pageAfter, documentLayoutSpec{Action: "document-page", Margins: map[string]float64{"top": 2, "bottom": 2, "left": 1.5, "right": 1.5}, Columns: 2, Background: "#EAF3FF"}); e != nil {
		t.Fatal(e)
	}
	if e := verifyDocumentReplace(pageAfter, replacement, documentReplaceSpec{Find: "旧版测试", Replacement: "新版测试", ExpectedCount: 3}); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(replacement, navigation) {
		t.Fatal("replacement-to-navigation chain differs")
	}
	result := map[string]any{"verified": true, "readOnlyReverification": true, "originalCLIResult": "number1 and page returned verification failures; not rewritten as successful CLI runs", "chain": []string{"number1/phase3-1838161215/before -> after-4: decimal with native List Paragraph/zero first-line normalization", "number1 after-4 == page/phase3-1862393371/before", "page before -> after-4: 2cm top/bottom, 1.5cm left/right, two columns, EAF3FF background; exact standard note defaults only", "page after-4 -> replace/phase3-913006079/reload: exactly three literal replacements; all other content/layout unchanged", "replace reload == goto/phase3-2944066916/after"}, "screenshot": "goto/phase3-2944066916/after.png", "remainingNavigationAssertion": "visible page is 2 with unique paragraph 27/42 markers but footer shows 1/3; native GoToPage fix awaits new live acceptance", "firstListItemOnly": true, "multiSectionVerified": false}
	if e := phase3WriteEvidence(filepath.Join(root, "recorded-proof.json"), result); e != nil {
		t.Fatal(e)
	}
}

func TestPhase3DocumentLayoutBatch4ThemeRecordedProof(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_LAYOUT_RECORDED") != "1" {
		t.Skip("explicit local recorded evidence check")
	}
	root := "../tmp/phase3-live/document-layout/cli-batch4"
	before, e := readDocumentLayout(filepath.Join(root, "theme-20261118T182930750689Z/evidence/phase3-1643295023/before/（自检）三期-文稿布局.docx"))
	if e != nil {
		t.Fatal(e)
	}
	after, e := readDocumentLayout(filepath.Join(root, "inspect-20261118T183112001943Z/evidence/phase3-3950259678/（自检）三期-文稿布局.docx"))
	if e != nil {
		t.Fatal(e)
	}
	spec := documentLayoutSpec{Action: "document-theme", Theme: &documentThemeFonts{Major: documentThemeFamily{Latin: "Arial", EA: "黑体", CS: "Arial"}, Minor: documentThemeFamily{Latin: "Arial", EA: "宋体", CS: "Arial"}}}
	if e = verifyDocumentThemeStyleDefaults(before.Theme.StyleDefaults, after.Theme.StyleDefaults); e != nil {
		t.Fatal(e)
	}
	if e = verifyDocumentLayout(before, after, spec); e == nil || !strings.Contains(e.Error(), "未找到真实继承minorEastAsia") {
		t.Fatalf("expected real-character-inheritance rejection, got %v", e)
	}
	result := map[string]any{"verified": false, "styleDefaultNormalizationVerified": true, "readOnlyReverification": true, "originalCLIExitCode": 1, "originalCLIStatus": "retained failure; never relabeled as successful command", "source": "batch4 theme before export -> independent inspect after export", "normalization": "only absent to exact zero table-indent on native heading/linked-character styles; no table style inheritance reaches them", "remainingFailure": e.Error(), "nextAcceptance": "requires dedicated inherited-body fixture; old body has explicit fonts and must stay preserved"}
	if e = phase3WriteEvidence(filepath.Join(root, "theme-recorded-reverification.json"), result); e != nil {
		t.Fatal(e)
	}
}

func TestPhase3DocumentLayoutBatch6ThemeRecordedProof(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_LAYOUT_RECORDED") != "1" {
		t.Skip("explicit local recorded evidence check")
	}
	root := "../tmp/phase3-live/document-layout/cli-batch6/theme-20261118T193314235944Z/evidence/phase3-2186607165"
	before, err := readDocumentLayout(filepath.Join(root, "before/（自检）三期-主题继承-20261119.docx"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := readDocumentLayout(filepath.Join(root, "after-4/（自检）三期-主题继承-20261119.docx"))
	if err != nil {
		t.Fatal(err)
	}
	spec := documentLayoutSpec{Action: "document-theme", Theme: &documentThemeFonts{Major: documentThemeFamily{Latin: "Arial", EA: "黑体", CS: "Arial"}, Minor: documentThemeFamily{Latin: "Arial", EA: "宋体", CS: "Arial"}}}
	if err = documentThemeCharacterProof(before, after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Theme.DirectRunFonts, after.Theme.DirectRunFonts) || before.Theme.NonFontDocument != after.Theme.NonFontDocument || before.Theme.NonFontTheme != after.Theme.NonFontTheme || !reflect.DeepEqual(after.Theme.Fonts, *spec.Theme) {
		t.Fatal("font change/direct override/nonfont content subproof failed")
	}
	if err = verifyDocumentLayout(before, after, spec); err == nil || !strings.Contains(err.Error(), "非字体样式") {
		t.Fatalf("expected nonfont style rejection: %v", err)
	}
	result := map[string]any{"verified": false, "originalCLIExitCode": 1, "readOnlyReverification": true, "realInheritedCharacterFontChangeVerified": true, "explicitRunFontsPreserved": true, "nonFontDocumentAndThemePreserved": true, "remainingFailure": err.Error(), "styleDelta": "Heading 1 and Heading 1 Char add uiPriority=9; not silently normalized because it controls gallery ordering", "noReplay": true}
	if err = phase3WriteEvidence(filepath.Join(root, "recorded-subproof.json"), result); err != nil {
		t.Fatal(err)
	}
}
