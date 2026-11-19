package main

import (
	"strings"
	"testing"
)

func documentStructureFixture(t *testing.T, formatted, mention bool, mutation string) documentSnapshot {
	t.Helper()
	run := `<w:r><w:t>xy</w:t></w:r>`
	if formatted {
		run = `<w:r><w:rPr><w:b/></w:rPr><w:t>x</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>y</w:t></w:r>`
	}
	if mention {
		run += `<w:hyperlink><w:r><w:rPr/><w:t>@Self</w:t></w:r></w:hyperlink>`
	}
	drawing := `<w:r><w:drawing><a:blip r:embed="image"/><a:xfrm rot="0"/></w:drawing></w:r>`
	p := `<w:p>` + run + drawing + `</w:p>`
	body := p + `<w:tbl><w:tblPr><w:jc w:val="center"/></w:tblPr><w:tr><w:tc><w:p><w:r><w:t>cell</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:sectPr><w:pgSz w:w="100"/></w:sectPr>`
	switch mutation {
	case "drawing-geometry":
		body = strings.Replace(body, `rot="0"`, `rot="90"`, 1)
	case "drawing-binding":
		body = strings.Replace(body, `r:embed="image"`, `r:embed="other"`, 1)
	case "drawing-deleted":
		body = strings.Replace(body, drawing, "", 1)
	case "drawing-position":
		body = strings.Replace(body, p, `<w:p>`+drawing+run+`</w:p>`, 1)
	case "section":
		body = strings.Replace(body, `w:w="100"`, `w:w="200"`, 1)
	case "table":
		body = strings.Replace(body, `w:val="center"`, `w:val="right"`, 1)
	case "topology":
		body = strings.ReplaceAll(strings.ReplaceAll(body, `<w:tbl>`, `<w:sdt>`), `</w:tbl>`, `</w:sdt>`)
	case "unknown":
		body = strings.Replace(body, `</w:p>`, `<x:custom x:value="new"/></w:p>`, 1)
	case "run-attribute":
		body = strings.Replace(body, `<w:r>`, `<w:r x:tracking="changed">`, 1)
	case "text-attribute":
		body = strings.Replace(body, `<w:t>`, `<w:t x:semantic="changed">`, 1)
	case "property-attribute":
		body = strings.Replace(body, `<w:r>`, `<w:r><w:rPr><w:color w:val="auto" x:semantic="changed"/></w:rPr>`, 1)
	case "empty-run":
		body = strings.Replace(body, `</w:p>`, `<w:r><w:rPr><w:b/></w:rPr></w:r></w:p>`, 1)
	case "drawing-run-format":
		body = strings.Replace(body, `<w:r><w:drawing>`, `<w:r><w:rPr><w:b/></w:rPr><w:drawing>`, 1)
	case "unknown-empty-text":
		body = strings.Replace(body, `</w:p>`, `<w:r><w:t x:semantic="changed"/></w:r></w:p>`, 1)
	case "hyperlink":
		body = strings.Replace(body, run, `<w:hyperlink r:id="link">`+run+`</w:hyperlink>`, 1)
	}
	doc := `<w:document xmlns:w="` + artifactWordNS + `" xmlns:r="` + artifactOfficeRelNS + `" xmlns:a="` + documentDrawingMainNS + `" xmlns:x="urn:custom"><w:body>` + body + `</w:body></w:document>`
	s, err := readDocumentSnapshot(artifactTestZip(t, map[string]string{"word/document.xml": doc}))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDocumentFormatAndMentionRejectNonTextStructureMutation(t *testing.T) {
	before := documentStructureFixture(t, false, false, "")
	for _, mention := range []bool{false, true} {
		spec, _ := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": "xy", "attrs": map[string]any{"bold": true}})
		check := func(after documentSnapshot) error { return verifyDocumentFormat(before, after, spec) }
		if mention {
			spec, _ = prepareDocumentMention(map[string]any{"paragraph": float64(1), "text": "xy"})
			check = func(after documentSnapshot) error { return verifyDocumentMention(before, after, spec, "Self") }
		}
		if err := check(documentStructureFixture(t, !mention, mention, "")); err != nil {
			t.Fatal("valid run split/mention rejected", err)
		}
		for _, mutation := range []string{"drawing-geometry", "drawing-binding", "drawing-deleted", "drawing-position", "section", "table", "topology", "unknown", "hyperlink", "run-attribute", "text-attribute", "property-attribute", "empty-run", "drawing-run-format", "unknown-empty-text"} {
			t.Run(mutation+map[bool]string{true: "-mention", false: "-format"}[mention], func(t *testing.T) {
				if check(documentStructureFixture(t, !mention, mention, mutation)) == nil {
					t.Fatal("non-text document mutation accepted")
				}
			})
		}
	}
}

func TestDocumentStructureRejectsAmbiguousMixedContent(t *testing.T) {
	// The shared XML tree aggregates direct character data, so mixed content
	// cannot prove the position of those bytes relative to child nodes.
	for _, content := range []string{`one<x:child/>two`, ` <x:child/>`, `onetwo<x:child/>`, `<x:child xml:space="preserve"><x:nested> <x:leaf/></x:nested></x:child>`} {
		doc := `<w:document xmlns:w="` + artifactWordNS + `" xmlns:x="urn:custom"><w:body><w:p><w:r><w:t>x</w:t></w:r><x:unknown>` + content + `</x:unknown></w:p></w:body></w:document>`
		if _, err := readDocumentSnapshot(artifactTestZip(t, map[string]string{"word/document.xml": doc})); err == nil {
			t.Fatal("accepted mixed content with unprovable positions")
		}
	}
}

func TestDocumentKnownPropertyAndSpaceGuard(t *testing.T) {
	read := func(body string) documentSnapshot {
		s, e := readDocumentSnapshot(artifactTestZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p>` + body + `</w:p></w:body></w:document>`}))
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	for _, tc := range []struct{ name, text, before, after string }{
		{"unrequested-underline-style", "xy", `<w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>xy</w:t></w:r>`, `<w:r><w:rPr><w:b/><w:u w:val="double"/></w:rPr><w:t>xy</w:t></w:r>`},
		{"text-space-preservation", " xy ", `<w:r><w:t xml:space="preserve"> xy </w:t></w:r>`, `<w:r><w:rPr><w:b/></w:rPr><w:t> xy </w:t></w:r>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, after := read(tc.before), read(tc.after)
			spec, e := prepareDocumentFormat(map[string]any{"paragraph": float64(1), "text": tc.text, "attrs": map[string]any{"bold": true}})
			if e != nil {
				t.Fatal(e)
			}
			if verifyDocumentFormat(before, after, spec) == nil {
				t.Fatal("unrequested semantic change accepted")
			}
		})
	}
}
