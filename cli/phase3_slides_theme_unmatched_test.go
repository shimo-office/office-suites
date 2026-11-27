package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestPhase3SlideThemeMatchedPlaceholderRequiresOfficialGeometry(t *testing.T) {
	beforeEntries, _ := releaseThemePPTX(t, 10)
	oldTransform := `<a:xfrm><a:off x="10" y="20"/><a:ext cx="300" cy="100"/></a:xfrm>`
	beforeEntries["ppt/slides/slide1.xml"] = strings.Replace(beforeEntries["ppt/slides/slide1.xml"], oldTransform, "", 1)
	beforeEntries["ppt/slideLayouts/layout1.xml"] = strings.Replace(beforeEntries["ppt/slideLayouts/layout1.xml"], `x="30"`, `x="10"`, 1)
	before := artifactTestZip(t, beforeEntries)
	for _, x := range []int{10, 30, 99} {
		t.Run(fmt.Sprint(x), func(t *testing.T) {
			afterEntries, spec := releaseThemePPTX(t, x)
			err := verifyPhase3SlideThemeArtifacts(before, artifactTestZip(t, afterEntries), []string{"slide-a"}, []string{"slide-a"}, phase3SlidesSpec{Theme: &spec})
			if (err == nil) != (x == 30) {
				t.Fatalf("matched title x=%d, want official x=30: %v", x, err)
			}
		})
	}
}

func TestPhase3SlideThemeRetainsUnmatchedContent(t *testing.T) {
	shape := func(id, kind, props, text string) string {
		return `<p:sp><p:nvSpPr><p:cNvPr id="` + id + `" name="Shape ` + id + `"/><p:nvPr><p:ph type="` + kind + `"/></p:nvPr></p:nvSpPr><p:spPr>` + props + `</p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:pPr/>` + text + `<a:endParaRPr/></a:p></p:txBody></p:sp>`
	}
	page := func(kind, shapes string) string {
		return `<p:` + kind + ` xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree>` + shapes + `</p:spTree></p:cSld></p:` + kind + `>`
	}
	oldGeometry := `<a:xfrm><a:off x="100" y="200"/><a:ext cx="300" cy="100"/></a:xfrm>`
	newGeometry := strings.Replace(oldGeometry, `x="100" y="200"`, `x="30" y="20"`, 1)
	emptyTitle := shape("4", "ctrTitle", "", "")
	text := `<a:r><a:t>Preserved subtitle</a:t></a:r>`
	subtitle := shape("5", "subTitle", "", text)
	newTitle := shape("2", "title", "", "")
	for _, fault := range []string{"", "subtitle-text", "subtitle-geometry", "subtitle-type", "missing-subtitle", "injected-title", "deleted-filled-title", "deleted-styled-title", "deleted-linked-title", "deleted-image-title", "deleted-positioned-title"} {
		t.Run(fault, func(t *testing.T) {
			beforeEntries, _ := releaseThemePPTX(t, 10)
			afterEntries, spec := releaseThemePPTX(t, 30)
			oldTitle := emptyTitle
			afterSubtitle := shape("5", "subTitle", oldGeometry, text)
			afterTitle := newTitle
			switch fault {
			case "subtitle-text":
				afterSubtitle = strings.Replace(afterSubtitle, "Preserved subtitle", "Changed", 1)
			case "subtitle-geometry":
				afterSubtitle = strings.Replace(afterSubtitle, `x="100"`, `x="101"`, 1)
			case "subtitle-type":
				afterSubtitle = strings.Replace(afterSubtitle, `type="subTitle"`, `type="body"`, 1)
			case "missing-subtitle":
				afterSubtitle = ""
			case "injected-title":
				afterTitle = strings.Replace(afterTitle, `<a:pPr/>`, `<a:pPr/>`+text, 1)
			case "deleted-filled-title":
				oldTitle = strings.Replace(oldTitle, `<a:pPr/>`, `<a:pPr/>`+text, 1)
			case "deleted-styled-title":
				oldTitle = shape("4", "ctrTitle", `<a:solidFill><a:srgbClr val="FF0000"/></a:solidFill>`, "")
			case "deleted-linked-title":
				oldTitle = strings.Replace(oldTitle, `<a:endParaRPr/>`, `<a:endParaRPr><a:hlinkClick/></a:endParaRPr>`, 1)
			case "deleted-image-title":
				oldTitle = shape("4", "ctrTitle", `<a:blipFill><a:blip/></a:blipFill>`, "")
			case "deleted-positioned-title":
				oldTitle = shape("4", "ctrTitle", oldGeometry, "")
			}
			beforeEntries["ppt/slides/slide1.xml"] = page("sld", oldTitle+subtitle)
			beforeEntries["ppt/slideLayouts/layout1.xml"] = page("sldLayout", shape("4", "ctrTitle", oldGeometry, "")+shape("5", "subTitle", oldGeometry, ""))
			afterEntries["ppt/slides/slide1.xml"] = page("sld", afterTitle+afterSubtitle)
			afterEntries["ppt/slideLayouts/layout1.xml"] = page("sldLayout", shape("2", "title", newGeometry, ""))
			err := verifyPhase3SlideThemeArtifacts(artifactTestZip(t, beforeEntries), artifactTestZip(t, afterEntries), []string{"slide-a"}, []string{"slide-a"}, phase3SlidesSpec{Theme: &spec})
			if (err == nil) != (fault == "") {
				t.Fatalf("fault=%q: %v", fault, err)
			}
		})
	}
}
