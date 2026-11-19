package main

import (
	"strings"
	"testing"
)

func TestPhase3SlideThemePlaceholderReconciliation(t *testing.T) {
	wrap := func(shapes string) *artifactNode {
		n, e := parseArtifactXML([]byte(`<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree>` + shapes + `</p:spTree></p:cSld></p:sld>`))
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	shape := func(id, name, ph, text string) string {
		return `<p:sp><p:nvSpPr><p:cNvPr id="` + id + `" name="` + name + `"/><p:cNvSpPr/><p:nvPr>` + ph + `</p:nvPr></p:nvSpPr><p:spPr><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:pPr/>` + text + `<a:endParaRPr/></a:p></p:txBody></p:sp>`
	}
	old := shape("2", "body", `<p:ph type="body" idx="1"/>`, `<a:r><a:t>keep</a:t></a:r>`)
	newBody := strings.Replace(old, `idx="1"`, `idx="14"`, 1)
	title := shape("5", "title", `<p:ph type="title"/>`, "")
	layout := wrap(shape("7", "sourceBody", `<p:ph type="body" idx="14"/>`, "") + title)
	if _, _, err := phase3ReconcileThemePage(wrap(old), wrap(title+newBody), layout); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{newBody, title + strings.Replace(title, `id="5"`, `id="6"`, 1) + newBody, strings.Replace(title, `<a:endParaRPr/>`, `<a:r><a:t>injected</a:t></a:r><a:endParaRPr/>`, 1) + newBody, strings.Replace(title, `prst="rect"`, `prst="ellipse"`, 1) + newBody, strings.Replace(newBody, `idx="14"`, `idx="15"`, 1), title} {
		if _, _, err := phase3ReconcileThemePage(wrap(old), wrap(bad), layout); err == nil {
			t.Fatal("unproven addition/removal/identity accepted", bad)
		}
	}
}

func TestPhase3SlideThemeMultipleIndexedPlaceholders(t *testing.T) {
	wrap := func(shapes string) *artifactNode {
		n, e := parseArtifactXML([]byte(`<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:cSld><p:spTree>` + shapes + `</p:spTree></p:cSld></p:sld>`))
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	shape := func(id, kind, idx, text string) string {
		return `<p:sp><p:nvSpPr><p:cNvPr id="` + id + `" name="` + id + `"/><p:nvPr><p:ph type="` + kind + `" idx="` + idx + `"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:pPr/>` + text + `<a:endParaRPr/></a:p></p:txBody></p:sp>`
	}
	first := shape("2", "body", "14", "")
	second := shape("3", "body", "15", "")
	picture := strings.Replace(shape("4", "pic", "10", ""), `<p:spPr/>`, `<p:spPr><a:blipFill><a:blip r:embed="sourceImage"/></a:blipFill></p:spPr>`, 1)
	old := shape("1", "body", "1", `<a:r><a:t>keep</a:t></a:r>`)
	updated := strings.Replace(old, `idx="1"`, `idx="14"`, 1)
	newPicture := strings.Replace(picture, "sourceImage", "actualImage", 1)
	layout := wrap(first + second + picture)
	after := wrap(second + newPicture + updated)
	if _, _, e := phase3ReconcileThemePage(wrap(old), after, layout, map[string]string{"actualImage": "image:hash"}, map[string]string{"sourceImage": "image:hash"}); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{updated + second + newPicture, newPicture + second + updated, newPicture + updated, second + second + newPicture + updated, second + newPicture + strings.Replace(updated, `idx="14"`, `idx="15"`, 1), strings.Replace(second, `<a:endParaRPr/>`, `<a:r><a:t>injected</a:t></a:r><a:endParaRPr/>`, 1) + newPicture + updated} {
		if _, _, e := phase3ReconcileThemePage(wrap(old), wrap(bad), layout, map[string]string{"actualImage": "image:hash"}, map[string]string{"sourceImage": "image:hash"}); e == nil {
			t.Fatal("invalid indexed/source assignment accepted")
		}
	}
	if _, _, e := phase3ReconcileThemePage(wrap(old), after, layout, map[string]string{"actualImage": "image:wrong"}, map[string]string{"sourceImage": "image:hash"}); e == nil {
		t.Fatal("wrong source image accepted")
	}
}
