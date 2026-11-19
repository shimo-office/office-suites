package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func releaseThemeClone[T any](t *testing.T, value T) T {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func releaseThemeOp(kind string, data map[string]any) map[string]any {
	return map[string]any{"action": "insert", "attributes": map[string]any{"opType": kind}, "data": data}
}
func releaseThemeProps(kind string, props map[string]any) map[string]any {
	return releaseThemeOp(kind, map[string]any{"object": props})
}
func releaseThemeShape(x float64) []any {
	return []any{map[string]any{"action": "insert", "attributes": map[string]any{"spType": "sp", "order": float64(0)}, "data": map[string]any{"delta": []any{
		releaseThemeProps("nvSpPr", map[string]any{"nvSpPr": map[string]any{"nvPr": map[string]any{"ph": map[string]any{"type": "title", "idx": "0"}}}}),
		releaseThemeProps("spPr", map[string]any{"spPr": map[string]any{"xfrm": map[string]any{"offX": x, "offY": float64(20), "extX": float64(300), "extY": float64(100)}}}),
	}}}}
}
func releaseThemeLayout(name string, x float64) []any {
	return []any{releaseThemeProps("attrs", map[string]any{"type": "title", "masterRef": "m-source"}), releaseThemeOp("csld", map[string]any{"delta": []any{releaseThemeProps("attrs", map[string]any{"name": name})}}), releaseThemeOp("spTree", map[string]any{"slide": map[string]any{"shape-source": releaseThemeShape(x)}})}
}
func releaseThemeDecoded() (map[string]any, []any) {
	wrap := func(v map[string]any) []any {
		return []any{map[string]any{"action": "insert", "data": map[string]any{"slide": v}}}
	}
	master := []any{releaseThemeProps("attrs", map[string]any{"themeId": "official", "themeRef": "t-source"}), releaseThemeProps("layoutRefs", map[string]any{"layoutRefs": []any{"l-title", "l-cover"}})}
	title, cover := releaseThemeLayout("Title", 10), releaseThemeLayout("Cover", 30)
	return map[string]any{"masters": wrap(map[string]any{"m-source": master}), "layouts": wrap(map[string]any{"l-title": title, "l-cover": cover}), "themes": wrap(map[string]any{"t-source": []any{releaseThemeProps("attrs", map[string]any{"name": "Synthetic"})}})}, []any{map[string]any{"id": "slide-a", "definition": title}, map[string]any{"id": "slide-b", "definition": cover}}
}
func releaseThemeGroup(t *testing.T, decoded map[string]any, key string) map[string]any {
	t.Helper()
	v, e := phase3ThemeMap(decoded[key])
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestReleaseSlidesThemeDeltaCompleteRoundTrip(t *testing.T) {
	decoded, current := releaseThemeDecoded()
	untouched := releaseThemeClone(t, decoded)
	spec := phase3SlideThemeSpec{ID: "official"}
	if err := phase3PrepareThemeDelta(&spec, decoded, current); err != nil {
		t.Fatal(err)
	}
	if spec.LayoutCount != 2 || spec.SlideLayouts["slide-a"] != 0 || spec.SlideLayouts["slide-b"] != 1 || spec.Geometry[1]["title:0"].X != 30 {
		t.Fatalf("wrong layout mapping: %+v", spec)
	}
	if !reflect.DeepEqual(decoded, untouched) {
		t.Fatal("preparation mutated source")
	}
	master := releaseThemeGroup(t, decoded, "masters")["m-source"]
	layouts := releaseThemeGroup(t, decoded, "layouts")
	reload := map[string]any{"masterRef": "m-source", "themeRef": "t-source", "layoutRefs": []any{"l-title", "l-cover"}, "master": master, "layouts": []any{layouts["l-title"], layouts["l-cover"]}}
	if err := phase3VerifyThemeDelta(reload, spec); err != nil {
		t.Fatal(err)
	}
	// Server-generated identity changes are permissible; layout semantics are not.
	raw, _ := json.Marshal(reload)
	for a, b := range map[string]string{"m-source": "m-reloaded", "t-source": "t-reloaded", "l-title": "l-reloaded-1", "l-cover": "l-reloaded-2", "shape-source": "shape-reloaded"} {
		raw = []byte(strings.ReplaceAll(string(raw), a, b))
	}
	var remapped map[string]any
	if err := json.Unmarshal(raw, &remapped); err != nil {
		t.Fatal(err)
	}
	if err := phase3VerifyThemeDelta(remapped, spec); err != nil {
		t.Fatal("valid identity remap", err)
	}
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"missing reference", func(v map[string]any) { delete(v, "layoutRefs") }},
		{"nonstr reference", func(v map[string]any) { v["layoutRefs"] = []any{float64(1), "l-cover"} }},
		{"missing layout", func(v map[string]any) { v["layouts"] = []any{layouts["l-title"]} }},
		{"reordered layouts", func(v map[string]any) { v["layouts"] = []any{layouts["l-cover"], layouts["l-title"]} }},
		{"changed master", func(v map[string]any) {
			v["master"].([]any)[0].(map[string]any)["data"].(map[string]any)["object"].(map[string]any)["new-setting"] = true
		}},
		{"invalid master operation", func(v map[string]any) { v["master"].([]any)[0].(map[string]any)["action"] = "delete" }},
		{"invalid layout operation", func(v map[string]any) { v["layouts"].([]any)[0] = []any{"bad"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := releaseThemeClone(t, reload)
			tc.edit(v)
			if err := phase3VerifyThemeDelta(v, spec); err == nil {
				t.Fatal("corrupt persisted theme accepted")
			}
		})
	}
	if err := phase3VerifyThemeDelta(nil, spec); err == nil {
		t.Fatal("missing persisted theme accepted")
	}
}
func TestReleaseSlidesThemePreparationRejectsAmbiguity(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any, []any)
	}{
		{"missing masters", func(d map[string]any, c []any) { delete(d, "masters") }},
		{"multiple masters", func(d map[string]any, c []any) {
			g := releaseThemeGroup(t, d, "masters")
			g["duplicate"] = g["m-source"]
		}},
		{"missing layouts", func(d map[string]any, c []any) { d["layouts"] = []any{} }},
		{"missing theme", func(d map[string]any, c []any) { delete(d, "themes") }},
		{"missing relation", func(d map[string]any, c []any) {
			m := releaseThemeGroup(t, d, "masters")["m-source"].([]any)
			m[1].(map[string]any)["data"].(map[string]any)["object"] = map[string]any{"layoutRefs": []any{"l-title"}}
		}},
		{"duplicate relation", func(d map[string]any, c []any) {
			m := releaseThemeGroup(t, d, "masters")["m-source"].([]any)
			m[1].(map[string]any)["data"].(map[string]any)["object"] = map[string]any{"layoutRefs": []any{"l-title", "l-title"}}
		}},
		{"missing target", func(d map[string]any, c []any) {
			m := releaseThemeGroup(t, d, "masters")["m-source"].([]any)
			m[1].(map[string]any)["data"].(map[string]any)["object"] = map[string]any{"layoutRefs": []any{"missing", "l-cover"}}
		}},
		{"wrong master identity", func(d map[string]any, c []any) {
			p, _ := phase3ThemeOpObject(releaseThemeGroup(t, d, "masters")["m-source"], "attrs")
			p["themeId"] = "another"
		}},
		{"wrong layout parent", func(d map[string]any, c []any) {
			p, _ := phase3ThemeOpObject(releaseThemeGroup(t, d, "layouts")["l-title"], "attrs")
			p["masterRef"] = "another"
		}},
		{"ambiguous layout", func(d map[string]any, c []any) {
			releaseThemeGroup(t, d, "layouts")["l-cover"] = releaseThemeLayout("Title", 30)
		}},
		{"unknown slide layout", func(d map[string]any, c []any) {
			p, _ := phase3ThemeOpObject(c[0].(map[string]any)["definition"], "attrs")
			p["type"] = "unknown"
		}},
		{"missing slide id", func(d map[string]any, c []any) { delete(c[0].(map[string]any), "id") }},
		{"malformed current layout", func(d map[string]any, c []any) { c[0].(map[string]any)["definition"] = []any{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, c := releaseThemeDecoded()
			c = releaseThemeClone(t, c)
			tc.edit(d, c)
			s := phase3SlideThemeSpec{ID: "official"}
			if err := phase3PrepareThemeDelta(&s, d, c); err == nil {
				t.Fatal("ambiguous/corrupt theme accepted")
			}
		})
	}
	for _, current := range []any{nil, []any{}} {
		d, _ := releaseThemeDecoded()
		s := phase3SlideThemeSpec{ID: "official"}
		if err := phase3PrepareThemeDelta(&s, d, current); err == nil {
			t.Fatal("missing current slides accepted")
		}
	}
	if err := phase3PrepareThemeDelta(&phase3SlideThemeSpec{}, nil, nil); err == nil {
		t.Fatal("missing decoded source accepted")
	}
}

func releaseThemeXML(t *testing.T) (string, map[string]any) {
	t.Helper()
	colors := map[string]any{}
	var xmlColors strings.Builder
	for i, key := range []string{"dk1", "lt1", "dk2", "lt2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hlink", "folHlink"} {
		val := fmt.Sprintf("%06X", i*654321)
		colors[key] = map[string]any{"color": map[string]any{"type": float64(1), "val": val}}
		fmt.Fprintf(&xmlColors, "<a:%s><a:srgbClr val=\"%s\"/></a:%s>", key, val, key)
	}
	fonts := map[string]any{}
	var xmlFonts strings.Builder
	for _, kind := range []string{"majorFont", "minorFont"} {
		fonts[kind] = map[string]any{"latin": map[string]any{"typeface": "Arial"}, "ea": map[string]any{"typeface": "宋体"}, "cs": map[string]any{"typeface": ""}}
		fmt.Fprintf(&xmlFonts, "<a:%s><a:latin typeface=\"Arial\"/><a:ea typeface=\"宋体\"/><a:cs typeface=\"\"/></a:%s>", kind, kind)
	}
	expected := map[string]any{"name": "Synthetic", "themeElements": map[string]any{"clrScheme": colors, "fontScheme": fonts}}
	return `<a:theme xmlns:a="` + artifactDrawingNS + `" name="Synthetic"><a:themeElements><a:clrScheme name="Synthetic">` + xmlColors.String() + `</a:clrScheme><a:fontScheme name="Synthetic">` + xmlFonts.String() + `</a:fontScheme></a:themeElements></a:theme>`, expected
}
func releaseThemePPTX(t *testing.T, x int) (map[string]string, phase3SlideThemeSpec) {
	t.Helper()
	rel := func(body string) string {
		return `<Relationships xmlns="` + artifactRelNS + `">` + body + `</Relationships>`
	}
	edge := func(id, kind, target string) string {
		return `<Relationship Id="` + id + `" Type="` + artifactOfficeRelNS + `/` + kind + `" Target="` + target + `"/>`
	}
	shape := func(x int, text string) string {
		return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="20"/><a:ext cx="300" cy="100"/></a:xfrm></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>`, x, text)
	}
	root := func(kind, body string) string {
		return `<p:` + kind + ` xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `" xmlns:r="` + artifactOfficeRelNS + `">` + body + `</p:` + kind + `>`
	}
	theme, expected := releaseThemeXML(t)
	image := "synthetic-image-resource"
	entries := map[string]string{
		"ppt/presentation.xml":                    root("presentation", `<p:sldIdLst><p:sldId id="256" r:id="slide"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/>`),
		"ppt/_rels/presentation.xml.rels":         rel(edge("slide", "slide", "slides/slide1.xml")),
		"ppt/slides/slide1.xml":                   root("sld", `<p:cSld><p:spTree>`+shape(x, "Preserved title")+`</p:spTree></p:cSld>`),
		"ppt/slides/_rels/slide1.xml.rels":        rel(edge("layout", "slideLayout", "../slideLayouts/layout1.xml")),
		"ppt/slideLayouts/layout1.xml":            root("sldLayout", `<p:cSld><p:spTree>`+shape(30, "")+`</p:spTree></p:cSld>`),
		"ppt/slideLayouts/_rels/layout1.xml.rels": rel(edge("master", "slideMaster", "../slideMasters/master1.xml")),
		"ppt/slideMasters/master1.xml":            root("sldMaster", `<p:cSld><p:spTree/></p:cSld><p:sldLayoutIdLst><p:sldLayoutId id="1" r:id="layout"/></p:sldLayoutIdLst>`),
		"ppt/slideMasters/_rels/master1.xml.rels": rel(edge("layout", "slideLayout", "../slideLayouts/layout1.xml") + edge("theme", "theme", "../theme/theme1.xml") + edge("image", "image", "../media/image1.png")),
		"ppt/theme/theme1.xml":                    theme, "ppt/media/image1.png": image,
	}
	s := phase3SlideThemeSpec{ID: "official", Expected: expected, LayoutCount: 1, Images: []string{fmt.Sprintf("%x", sha256.Sum256([]byte(image)))}, SlideLayouts: map[string]int{"slide-a": 0}, Geometry: []map[string]phase3ThemeTransform{{"title:0": {X: 30, Y: 20, CX: 300, CY: 100}}}}
	return entries, s
}
func TestReleaseSlidesThemeArtifactPreservation(t *testing.T) {
	beforeEntries, _ := releaseThemePPTX(t, 10)
	afterEntries, spec := releaseThemePPTX(t, 30)
	before := artifactTestZip(t, beforeEntries)
	verify := func(entries map[string]string, s phase3SlideThemeSpec) error {
		return verifyPhase3SlideThemeArtifacts(before, artifactTestZip(t, entries), []string{"slide-a"}, []string{"slide-a"}, phase3SlidesSpec{Theme: &s})
	}
	if err := verify(afterEntries, spec); err != nil {
		t.Fatal("valid theme and official geometry", err)
	}
	for _, tc := range []struct {
		name string
		edit func(map[string]string, *phase3SlideThemeSpec)
		want string
	}{
		{"changed title", func(e map[string]string, s *phase3SlideThemeSpec) {
			e["ppt/slides/slide1.xml"] = strings.ReplaceAll(e["ppt/slides/slide1.xml"], "Preserved title", "Lost title")
		}, "改变了原文字"},
		{"arbitrary movement", func(e map[string]string, s *phase3SlideThemeSpec) {
			e["ppt/slides/slide1.xml"] = strings.Replace(e["ppt/slides/slide1.xml"], `x="30"`, `x="999"`, 1)
		}, "新位置不匹配"},
		{"changed slide size", func(e map[string]string, s *phase3SlideThemeSpec) {
			e["ppt/presentation.xml"] = strings.Replace(e["ppt/presentation.xml"], `cx="12192000"`, `cx="10000000"`, 1)
		}, "改变了原文字"},
		{"changed show properties", func(e map[string]string, s *phase3SlideThemeSpec) {
			e["ppt/presProps.xml"] = `<p:presentationPr xmlns:p="` + artifactSlideNS + `"><p:showPr loop="1"/></p:presentationPr>`
		}, "演示设置"},
		{"wrong theme name", func(e map[string]string, s *phase3SlideThemeSpec) {
			e["ppt/theme/theme1.xml"] = strings.Replace(e["ppt/theme/theme1.xml"], `name="Synthetic"`, `name="Other"`, 1)
		}, "主题名"},
		{"wrong RGB", func(e map[string]string, s *phase3SlideThemeSpec) {
			e["ppt/theme/theme1.xml"] = strings.Replace(e["ppt/theme/theme1.xml"], `val="000000"`, `val="FFFFFF"`, 1)
		}, "主题颜色"},
		{"wrong font", func(e map[string]string, s *phase3SlideThemeSpec) {
			e["ppt/theme/theme1.xml"] = strings.Replace(e["ppt/theme/theme1.xml"], `typeface="Arial"`, `typeface="Wrong"`, 1)
		}, "主题字体"},
		{"unreachable image", func(e map[string]string, s *phase3SlideThemeSpec) {
			e["ppt/slideMasters/_rels/master1.xml.rels"] = strings.Replace(e["ppt/slideMasters/_rels/master1.xml.rels"], `Type="`+artifactOfficeRelNS+`/image"`, `Type="`+artifactOfficeRelNS+`/other"`, 1)
		}, "图片未嵌入"},
		{"changed image", func(e map[string]string, s *phase3SlideThemeSpec) { e["ppt/media/image1.png"] = "different-resource" }, "图片未嵌入"},
		{"missing expected slide", func(e map[string]string, s *phase3SlideThemeSpec) { s.SlideLayouts = map[string]int{} }, "缺少目标页面版式"},
		{"wrong layout count", func(e map[string]string, s *phase3SlideThemeSpec) { s.LayoutCount = 2 }, "版式数量"},
		{"wrong layout index", func(e map[string]string, s *phase3SlideThemeSpec) { s.SlideLayouts["slide-a"] = 1 }, "版式索引"},
		{"missing geometry", func(e map[string]string, s *phase3SlideThemeSpec) { s.Geometry = nil }, "几何预期缺失"},
		{"wrong template geometry", func(e map[string]string, s *phase3SlideThemeSpec) {
			e["ppt/slideLayouts/layout1.xml"] = strings.Replace(e["ppt/slideLayouts/layout1.xml"], `x="30"`, `x="31"`, 1)
		}, "几何不匹配"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := releaseThemeClone(t, afterEntries)
			s := releaseThemeClone(t, spec)
			tc.edit(entries, &s)
			err := verify(entries, s)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q rejection, got %v", tc.want, err)
			}
		})
	}
	if err := verifyPhase3SlideThemeArtifacts(before, before, []string{"slide-a"}, []string{"new-slide"}, phase3SlidesSpec{Theme: &spec}); err == nil {
		t.Fatal("changed page IDs accepted")
	}
}
