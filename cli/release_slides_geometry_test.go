package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestReleaseSlidesThemeSourceGeometryRejectsMalformedTransforms(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func([]any)
		want string
	}{
		{"missing coordinates", func(v []any) {
			p, _ := phase3ThemeOpObject(v[0].(map[string]any)["data"].(map[string]any)["delta"], "spPr")
			delete(p["spPr"].(map[string]any)["xfrm"].(map[string]any), "extX")
		}, "几何无效"},
		{"fractional coordinate", func(v []any) {
			p, _ := phase3ThemeOpObject(v[0].(map[string]any)["data"].(map[string]any)["delta"], "spPr")
			p["spPr"].(map[string]any)["xfrm"].(map[string]any)["offX"] = 0.5
		}, "几何无效"},
		{"rotation", func(v []any) {
			p, _ := phase3ThemeOpObject(v[0].(map[string]any)["data"].(map[string]any)["delta"], "spPr")
			p["spPr"].(map[string]any)["xfrm"].(map[string]any)["rot"] = float64(90)
		}, "旋转"},
		{"missing identity", func(v []any) {
			v[0].(map[string]any)["data"].(map[string]any)["delta"] = []any{releaseThemeProps("spPr", map[string]any{})}
		}, "nvSpPr"},
		{"missing properties", func(v []any) {
			v[0].(map[string]any)["data"].(map[string]any)["delta"] = v[0].(map[string]any)["data"].(map[string]any)["delta"].([]any)[:1]
		}, "spPr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shape := releaseThemeShape(10)
			tc.edit(shape)
			_, err := phase3ThemeSourceGeometry(shape)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q got %v", tc.want, err)
			}
		})
	}
	shape := releaseThemeShape(10)
	nv, _ := phase3ThemeOpObject(shape[0].(map[string]any)["data"].(map[string]any)["delta"], "nvSpPr")
	nv["nvSpPr"].(map[string]any)["nvPr"].(map[string]any)["ph"] = map[string]any{}
	sp, _ := phase3ThemeOpObject(shape[0].(map[string]any)["data"].(map[string]any)["delta"], "spPr")
	sp["spPr"].(map[string]any)["xfrm"].(map[string]any)["flipH"] = true
	got, err := phase3ThemeSourceGeometry(shape)
	if err != nil || !got["obj:0"].FlipH || got["obj:0"].X != 10 {
		t.Fatalf("default identity and flip lost: %+v %v", got, err)
	}
	if _, err := phase3ThemeSourceGeometry([]any{shape, shape}); err == nil {
		t.Fatal("duplicate placeholder identity accepted")
	}
	delete(sp["spPr"].(map[string]any), "xfrm")
	got, err = phase3ThemeSourceGeometry(shape)
	if err != nil || len(got) != 0 {
		t.Fatalf("inherited geometry rejected: %v %v", got, err)
	}
}
func releaseTransformNode(t *testing.T, xml string) *artifactNode {
	t.Helper()
	n, e := parseArtifactXML([]byte(xml))
	if e != nil {
		t.Fatal(e)
	}
	return n
}
func TestReleaseSlidesThemeXMLGeometryTrustBoundary(t *testing.T) {
	base := `<a:xfrm xmlns:a="` + artifactDrawingNS + `" flipH="true" flipV="0"><a:off x="10" y="20"/><a:ext cx="300" cy="100"/></a:xfrm>`
	got, err := phase3ThemeXMLTransform(releaseTransformNode(t, base))
	if err != nil || got == nil || *got != (phase3ThemeTransform{X: 10, Y: 20, CX: 300, CY: 100, FlipH: true}) {
		t.Fatalf("wrong transform %+v %v", got, err)
	}
	if got, err := phase3ThemeXMLTransform(nil); err != nil || got != nil {
		t.Fatal("missing local transform should inherit")
	}
	for _, tc := range []struct{ name, from, to, want string }{
		{"rotation", `flipH="true"`, `rot="60000"`, "旋转"},
		{"unknown attribute", `flipH="true"`, `unexpected="1"`, "未知属性"},
		{"unknown child", `<a:off x="10" y="20"/>`, `<a:chOff x="10" y="20"/>`, "非预期属性"},
		{"missing extent", `<a:ext cx="300" cy="100"/>`, "", "不完整"},
		{"invalid integer", `x="10"`, `x="NaN"`, "invalid syntax"},
		{"invalid flip", `flipH="true"`, `flipH="yes"`, "翻转标记"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := phase3ThemeXMLTransform(releaseTransformNode(t, strings.Replace(base, tc.from, tc.to, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q got %v", tc.want, err)
			}
		})
	}
	shape := `<p:sp><p:nvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr>` + base + `<a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></p:spPr></p:sp>`
	wrap := func(body string) *artifactNode {
		return releaseTransformNode(t, `<p:sld xmlns:p="`+artifactSlideNS+`" xmlns:a="`+artifactDrawingNS+`"><p:cSld><p:spTree>`+body+`</p:spTree></p:cSld></p:sld>`)
	}
	root := wrap(shape)
	untouched, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := phase3ThemeXMLGeometry(root)
	if err != nil || geometry["title:0"] == nil {
		t.Fatal(err)
	}
	masked := phase3MaskThemeGeometry(root, map[string]bool{"title:0": true})
	afterMask, err := json.Marshal(root)
	if err != nil || string(afterMask) != string(untouched) {
		t.Fatal("mask mutated original evidence")
	}
	maskedGeometry, err := phase3ThemeXMLGeometry(masked)
	if err != nil || maskedGeometry["title:0"] != nil {
		t.Fatal("approved transform not masked")
	}
	tree, _ := phase3ShapeTree(masked)
	if phase3SlideChild(phase3SlideChild(tree.Children[0], artifactSlideNS, "spPr"), artifactDrawingNS, "solidFill") == nil {
		t.Fatal("mask removed local fill")
	}
	if _, err := phase3ThemeXMLGeometry(wrap(shape + shape)); err == nil {
		t.Fatal("duplicate XML placeholder accepted")
	}
	if _, err := phase3ThemeAllowedGeometry(map[string]*phase3ThemeTransform{"title:0": got}, map[string]*phase3ThemeTransform{}, nil); err == nil {
		t.Fatal("deleted placeholder accepted")
	}
	if _, err := phase3ThemeAllowedGeometry(map[string]*phase3ThemeTransform{"title:0": got}, map[string]*phase3ThemeTransform{"body:0": got}, nil); err == nil {
		t.Fatal("reidentified placeholder accepted")
	}
	if masks, err := phase3ThemeAllowedGeometry(geometry, geometry, nil); err != nil || len(masks) != 0 {
		t.Fatal("unchanged geometry should need no mask")
	}
}
func TestReleaseSlidesThemeCanonicalSourceBindings(t *testing.T) {
	source := map[string]any{"data": map[string]any{"slide": map[string]any{"first": releaseThemeShape(10), "second": releaseThemeShape(20)}}}
	source["data"].(map[string]any)["slide"].(map[string]any)["second"].([]any)[0].(map[string]any)["attributes"].(map[string]any)["order"] = float64(1)
	first, err := phase3CanonicalThemeData(source, nil)
	if err != nil {
		t.Fatal(err)
	}
	renamed := releaseThemeClone(t, source)
	group := renamed["data"].(map[string]any)["slide"].(map[string]any)
	group["other-first"] = group["first"]
	delete(group, "first")
	second, err := phase3CanonicalThemeData(renamed, nil)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("identity changes should preserve source order")
	}
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"duplicate order", func(v map[string]any) {
			v["data"].(map[string]any)["slide"].(map[string]any)["second"].([]any)[0].(map[string]any)["attributes"].(map[string]any)["order"] = float64(0)
		}},
		{"fractional order", func(v map[string]any) {
			v["data"].(map[string]any)["slide"].(map[string]any)["second"].([]any)[0].(map[string]any)["attributes"].(map[string]any)["order"] = 0.5
		}},
		{"unexplained metadata", func(v map[string]any) { v["data"].(map[string]any)["sideEffect"] = true }},
		{"multiple shape operations", func(v map[string]any) { v["data"].(map[string]any)["slide"].(map[string]any)["second"] = []any{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := releaseThemeClone(t, source)
			tc.edit(v)
			if _, err := phase3CanonicalThemeData(v, nil); err == nil {
				t.Fatal("ambiguous source shape accepted")
			}
		})
	}
	for _, value := range []any{float64(1), "missing"} {
		if _, err := phase3CanonicalThemeData(map[string]any{"nested": []any{map[string]any{"rasterImageId": value}}}, nil); err == nil {
			t.Fatal("unbound picture accepted")
		}
	}
	if v, err := phase3CanonicalThemeData(map[string]any{"rasterImageId": "source-image", "text": "source-image"}, map[string]string{"source-image": "hash"}); err != nil || v.(map[string]any)["text"] != "source-image" || v.(map[string]any)["rasterImageId"] != "hash" {
		t.Fatalf("asset binding changed ordinary text: %v %v", v, err)
	}
}
func TestReleaseSlidesThemeXMLRequiresCompleteSource(t *testing.T) {
	raw, expected := releaseThemeXML(t)
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{"missing RGB", func(v map[string]any) {
			delete(v["themeElements"].(map[string]any)["clrScheme"].(map[string]any), "accent1")
		}, "颜色accent1缺失"},
		{"unsupported color type", func(v map[string]any) {
			v["themeElements"].(map[string]any)["clrScheme"].(map[string]any)["dk1"].(map[string]any)["color"].(map[string]any)["type"] = float64(2)
		}, "并非可核验RGB"},
		{"color transform", func(v map[string]any) {
			v["themeElements"].(map[string]any)["clrScheme"].(map[string]any)["dk1"].(map[string]any)["mods"] = []any{map[string]any{"tint": 1}}
		}, "并非可核验RGB"},
		{"missing major font", func(v map[string]any) {
			delete(v["themeElements"].(map[string]any)["fontScheme"].(map[string]any), "majorFont")
		}, "字体majorFont缺失"},
		{"missing script", func(v map[string]any) {
			delete(v["themeElements"].(map[string]any)["fontScheme"].(map[string]any)["majorFont"].(map[string]any), "latin")
		}, "majorFont/latin不符"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := releaseThemeClone(t, expected)
			tc.edit(s)
			err := phase3VerifyThemeXML(releaseTransformNode(t, raw), phase3SlideThemeSpec{Expected: s})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q got %v", tc.want, err)
			}
		})
	}
	if err := phase3VerifyThemeXML(releaseTransformNode(t, `<a:theme xmlns:a="`+artifactDrawingNS+`" name="Synthetic"/>`), phase3SlideThemeSpec{Expected: expected}); err == nil {
		t.Fatal("missing XML theme structure accepted")
	}
}
