package main

import (
	"strings"
	"testing"
)

func documentSpanResourceFixture(t *testing.T, row, media, target string) (documentTablesSnapshot, error) {
	t.Helper()
	doc := `<w:document xmlns:w="` + artifactWordNS + `" xmlns:r="` + artifactOfficeRelNS + `" xmlns:a="` + artifactDrawingNS + `"><w:body><w:p><w:r><w:drawing><a:blip r:embed="rImage"/></w:drawing></w:r></w:p><w:tbl><w:tblGrid><w:gridCol w:w="1000"/><w:gridCol w:w="1000"/><w:gridCol w:w="1000"/></w:tblGrid><w:tr>` + row + `</w:tr></w:tbl></w:body></w:document>`
	rels := `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="rImage" Type="` + artifactOfficeRelNS + `/image" Target="` + target + `"/></Relationships>`
	return readDocumentTables(artifactTestZip(t, map[string]string{"word/document.xml": doc, "word/_rels/document.xml.rels": rels, "word/media/image.png": media, "word/custom/image.png": media}))
}
func TestDocumentTableSpanRejectsInvalidWidths(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf", "0", "-1", "2000.5", "2e3", "garbage"} {
		t.Run(value, func(t *testing.T) {
			row := strings.Replace(documentSpanCell(2, "A", "B"), `w:w="2000"`, `w:w="`+value+`"`, 1) + documentSpanCell(1, "C")
			if _, err := documentSpanResourceFixture(t, row, "original", "media/image.png"); err == nil {
				t.Fatal("invalid XML cell width accepted", value)
			}
		})
	}
}
func TestDocumentTableSpanPreservesMediaBytesAndBindings(t *testing.T) {
	spec := documentTableSpec{Operation: "merge", Table: 1, Row: 1, Column: 1, EndColumn: 2, ExpectedCell: "A"}
	before, err := documentSpanResourceFixture(t, documentSpanCell(1, "A")+documentSpanCell(1, "B")+documentSpanCell(1, "C"), "original", "media/image.png")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, media, target string
		reject              bool
	}{{"unchanged", "original", "media/image.png", false}, {"media-replaced", "replacement", "media/image.png", true}, {"binding-changed", "original", "custom/image.png", true}, {"custom-path-bytes", "replacement", "custom/image.png", true}} {
		t.Run(tc.name, func(t *testing.T) {
			after, err := documentSpanResourceFixture(t, documentSpanCell(2, "A", "B")+documentSpanCell(1, "C"), tc.media, tc.target)
			if err != nil {
				t.Fatal(err)
			}
			err = verifyDocumentTable(before, after, spec)
			if (err != nil) != tc.reject {
				t.Fatalf("reject=%v got %v", tc.reject, err)
			}
		})
	}
}
func TestDocumentTableSpanVerticalMarkersFollowNativeOperation(t *testing.T) {
	restart := func(c string) string {
		return strings.Replace(c, `</w:tcPr>`, `<w:vMerge w:val="restart"/></w:tcPr>`, 1)
	}
	spec := documentTableSpec{Operation: "merge", Table: 1, Row: 1, Column: 1, EndColumn: 2, ExpectedCell: "A"}
	before := documentSpanFixture(t, documentSpanCell(1, "A")+documentSpanCell(1, "B")+documentSpanCell(1, "C"), "F")
	merged := documentSpanFixture(t, restart(documentSpanCell(2, "A", "B"))+documentSpanCell(1, "C"), "F")
	if err := verifyDocumentTable(before, merged, spec); err != nil {
		t.Fatal("native horizontal merge restart rejected", err)
	}
	spec = documentTableSpec{Operation: "split", Table: 1, Row: 1, Column: 1, Columns: 2, ExpectedCell: "A\nB"}
	plain := documentSpanFixture(t, documentSpanCell(2, "A", "B")+documentSpanCell(1, "C"), "F")
	unexpected := documentSpanFixture(t, restart(documentSpanCell(1, "A", "B"))+restart(documentSpanCell(1, ""))+documentSpanCell(1, "C"), "F")
	if err := verifyDocumentTable(plain, unexpected, spec); err == nil {
		t.Fatal("split invented vertical marker")
	}
	if err := verifyDocumentTable(merged, unexpected, spec); err != nil {
		t.Fatal("native copied restart rejected", err)
	}
}

func TestDocumentTableSpanPreservesCustomPathImageBytes(t *testing.T) {
	spec := documentTableSpec{Operation: "merge", Table: 1, Row: 1, Column: 1, EndColumn: 2, ExpectedCell: "A"}
	before, err := documentSpanResourceFixture(t, documentSpanCell(1, "A")+documentSpanCell(1, "B")+documentSpanCell(1, "C"), "original", "custom/image.png")
	if err != nil {
		t.Fatal(err)
	}
	after, err := documentSpanResourceFixture(t, documentSpanCell(2, "A", "B")+documentSpanCell(1, "C"), "replacement", "custom/image.png")
	if err != nil {
		t.Fatal(err)
	}
	// The conventional media part has no binding here; isolate the custom relationship target.
	after.Assets["word/media/image.png"] = before.Assets["word/media/image.png"]
	if err = verifyDocumentTable(before, after, spec); err == nil {
		t.Fatal("custom image path replacement accepted")
	}
}
