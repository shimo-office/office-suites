package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func releasePhase2ZIP(t *testing.T, entries map[string]string, unsupported string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.zip")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	w := zip.NewWriter(f)
	for name, text := range entries {
		var e error
		if name == unsupported {
			_, e = w.CreateRaw(&zip.FileHeader{Name: name, Method: 99})
		} else {
			var out interface{ Write([]byte) (int, error) }
			out, e = w.Create(name)
			if e == nil {
				_, e = out.Write([]byte(text))
			}
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	return path
}

func TestReleasePhase2OfficeExtractionPreservesOrderAndSharedText(t *testing.T) {
	ppt := releasePhase2ZIP(t, map[string]string{"ppt/slides/slide10.xml": "<slide><t>Ten</t></slide>", "ppt/slides/slide2.xml": "<slide><t>Two</t><t>二</t></slide>", "ppt/slides/_rels/slide2.xml.rels": "<unrelated>ignore</unrelated>"}, "")
	got, err := extractPptx(ppt)
	if err != nil || got != "## 幻灯片 1\nTwo 二\n\n## 幻灯片 2\nTen" {
		t.Fatal(got, err)
	}
	xlsx := releasePhase2ZIP(t, releaseFileWorkbookEntries(map[string]string{
		"xl/sharedStrings.xml":      "<sst><si><t>Hello </t><r><t>世界</t></r></si><si><t>Second</t></si></sst>",
		"xl/worksheets/sheet10.xml": "<worksheet><row><c t=\"s\"><v>1</v></c></row></worksheet>",
		"xl/worksheets/sheet2.xml":  "<worksheet><row><c t=\"s\"><v>0</v></c><c><v>42.5</v></c></row><row><c><v> </v></c></row></worksheet>",
	}, "xl/worksheets/sheet2.xml", "xl/worksheets/sheet10.xml"), "")
	got, err = extractXlsx(xlsx)
	if err != nil || got != "## Sheet 1\nHello 世界\t42.5\n\n## Sheet 2\nSecond" {
		t.Fatal(got, err)
	}
}

func TestReleasePhase2OfficeExtractionRejectsCorruptContainers(t *testing.T) {
	for _, fn := range []func(string) (string, error){extractDocx, extractXlsx, extractPptx} {
		if _, err := fn(filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("missing archive accepted")
		}
	}
	if _, err := extractDocx(releasePhase2ZIP(t, map[string]string{"word/styles.xml": "<styles/>"}, "")); err == nil {
		t.Fatal("missing document part accepted")
	}
	for _, tc := range []struct {
		name  string
		fn    func(string) (string, error)
		entry string
	}{
		{"docx", extractDocx, "word/document.xml"}, {"pptx", extractPptx, "ppt/slides/slide1.xml"}, {"xlsx-values", extractXlsx, "xl/worksheets/sheet1.xml"}, {"xlsx-shared", extractXlsx, "xl/sharedStrings.xml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.fn(releasePhase2ZIP(t, map[string]string{tc.entry: "<broken>"}, "")); err == nil {
				t.Fatal("malformed XML accepted")
			}
			if _, err := tc.fn(releasePhase2ZIP(t, map[string]string{tc.entry: ""}, tc.entry)); err == nil {
				t.Fatal("unreadable compression accepted")
			}
		})
	}
}

func TestReleasePhase2WorkbookEmptyRowsAndUnknownStringIndex(t *testing.T) {
	path := releasePhase2ZIP(t, releaseFileWorkbookEntries(map[string]string{"xl/worksheets/sheet1.xml": `<worksheet><row/><row><c t="s"><v>99</v></c><c t="s"><v>not-number</v></c></row></worksheet>`}, "xl/worksheets/sheet1.xml"), "")
	got, err := extractXlsx(path)
	if err != nil || !strings.Contains(got, "99\tnot-number") {
		t.Fatal("unresolvable index discarded", got, err)
	}
}
