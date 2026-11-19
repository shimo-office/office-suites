package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const releaseShapeTarget = `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Target"/></p:nvSpPr><p:spPr><a:xfrm rot="600000"><a:off x="100000" y="200000"/><a:ext cx="2000000" cy="1000000"/></a:xfrm><a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill><a:ln w="12700"><a:solidFill><a:srgbClr val="000000"/></a:solidFill></a:ln></p:spPr><p:txBody><a:p><a:r><a:t>Preserved shape text</a:t></a:r></a:p></p:txBody></p:sp>`

func TestReleaseSlideShapeFlow(t *testing.T) {
	anchor := strings.ReplaceAll(releaseFlowOriginal, "Original", "Anchor")
	changed := strings.NewReplacer(`rot="600000"`, `rot="5400000"`, `x="100000"`, `x="5096000"`, `val="FFFFFF"`, `val="0000FF"`, `val="000000"`, `val="FF0000"`).Replace(releaseShapeTarget)
	for _, fault := range []string{"", "collateral", "missing-geometry", "unusable-width", "missing-stroke", "already-behind"} {
		t.Run(fault, func(t *testing.T) {
			before := anchor + releaseShapeTarget
			after := changed + anchor
			switch fault {
			case "collateral":
				after = strings.Replace(after, "Preserved shape text", "lost content", 1)
			case "missing-geometry":
				before = strings.ReplaceAll(before, `<a:xfrm rot="600000"><a:off x="100000" y="200000"/><a:ext cx="2000000" cy="1000000"/></a:xfrm>`, "")
			case "unusable-width":
				before = strings.Replace(before, `cx="2000000"`, `cx="2000001"`, 1)
			case "missing-stroke":
				before = strings.Replace(before, `w="12700"`, `w="0"`, 1)
			case "already-behind":
				before = releaseShapeTarget + anchor
			}
			f := &releaseSlideFlow{beforeIDs: []string{"s1", "s2"}, afterIDs: []string{"s1", "s2"}, before: releaseFlowPPTX(t, []string{before, releaseFlowOriginal}, -1), after: releaseFlowPPTX(t, []string{after, releaseFlowOriginal}, -1), png: releaseFlowPNG(t)}
			f.install(t)
			out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", "slide-shape-format", "--payload", `{"slideId":"s1","shapeName":"Target","fillColor":"#0000FF","lineColor":"#FF0000","rotation":90,"horizontalCenter":true,"behindName":"Anchor"}`, "--out", t.TempDir(), "--json"}).Run)
			if fault == "" {
				if err != nil {
					t.Fatal(err)
				}
				var result map[string]any
				if json.Unmarshal([]byte(out), &result) != nil || result["verified"] != true {
					t.Fatalf("unverified: %s", out)
				}
				if f.writes.Load() != 1 || f.opens != 2 {
					t.Fatalf("writes=%d opens=%d", f.writes.Load(), f.opens)
				}
				raw, e := os.ReadFile(filepath.Join(str(result, "evidenceDir"), "shape-expected.json"))
				if e != nil {
					t.Fatal(e)
				}
				var expected phase3SlideShapeSpec
				if json.Unmarshal(raw, &expected) != nil || expected.Delta != 80 || expected.CenterX != 5096000 || expected.BackwardSteps != 1 {
					t.Fatalf("wrong derived geometry: %s", raw)
				}
			} else {
				wantWrites := int32(0)
				if fault == "collateral" {
					wantWrites = 1
				}
				if err == nil || strings.Contains(out, `"verified": true`) || f.writes.Load() != wantWrites {
					t.Fatalf("fault=%s err=%v writes=%d out=%s", fault, err, f.writes.Load(), out)
				}
			}
		})
	}
}

func releaseLayoutArtifact(t *testing.T, selected string) []byte {
	t.Helper()
	entries := releaseMediaEntries(t, releaseFlowPPTX(t, []string{releaseFlowOriginal, releaseFlowOriginal}, -1))
	relation := func(kind, target string) string {
		return `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="r1" Type="` + artifactOfficeRelNS + `/` + kind + `" Target="` + target + `"/></Relationships>`
	}
	for i, layout := range []string{selected, "old"} {
		entries[fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", i+1)] = relation("slideLayout", "../slideLayouts/"+layout+".xml")
	}
	for _, name := range []string{"old", "new"} {
		entries["ppt/slideLayouts/"+name+".xml"] = `<p:sldLayout xmlns:p="` + artifactSlideNS + `"><p:cSld name="` + name + `"><p:spTree/></p:cSld></p:sldLayout>`
		entries["ppt/slideLayouts/_rels/"+name+".xml.rels"] = relation("slideMaster", "../slideMasters/one.xml")
	}
	entries["ppt/slideMasters/one.xml"] = `<p:sldMaster xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:cSld><p:spTree/></p:cSld><p:sldLayoutIdLst><p:sldLayoutId id="1" r:id="l1"/><p:sldLayoutId id="2" r:id="l2"/></p:sldLayoutIdLst></p:sldMaster>`
	entries["ppt/slideMasters/_rels/one.xml.rels"] = `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="l1" Type="` + artifactOfficeRelNS + `/slideLayout" Target="../slideLayouts/old.xml"/><Relationship Id="l2" Type="` + artifactOfficeRelNS + `/slideLayout" Target="../slideLayouts/new.xml"/></Relationships>`
	raw, e := os.ReadFile(artifactTestZip(t, entries))
	if e != nil {
		t.Fatal(e)
	}
	return raw
}

func releaseLayoutDesign(selected string) map[string]any {
	return map[string]any{"ok": true, "slides": []any{map[string]any{"id": "s1", "masterId": "m1", "layoutId": selected}}, "masters": []any{map[string]any{"id": "m1", "layouts": []any{map[string]any{"id": "old", "index": float64(0)}, map[string]any{"id": "new", "index": float64(1)}}}}}
}

func TestReleaseSlideLayoutFlow(t *testing.T) {
	for _, fault := range []string{"", "reload-old", "reload-missing", "missing-master", "design-inspect"} {
		t.Run(fault, func(t *testing.T) {
			f := &releaseSlideFlow{beforeIDs: []string{"s1", "s2"}, afterIDs: []string{"s1", "s2"}, before: releaseLayoutArtifact(t, "old"), after: releaseLayoutArtifact(t, "new"), png: releaseFlowPNG(t)}
			f.extraEval = func(expr string, opening int) (any, bool) {
				if !strings.Contains(expr, "const masters=wb.core.getSlideMasters") {
					return nil, false
				}
				selected := "old"
				if opening > 1 && fault != "reload-old" {
					selected = "new"
				}
				v := releaseLayoutDesign(selected)
				if fault == "missing-master" || fault == "reload-missing" && opening > 1 {
					v["slides"] = []any{}
				}
				return v, true
			}
			f.install(t)
			action := "slide-layout"
			if fault == "design-inspect" {
				action = "slide-design-inspect"
			}
			out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", action, "--payload", `{"slideId":"s1","layoutId":"new"}`, "--out", t.TempDir(), "--json"}).Run)
			if fault == "design-inspect" {
				if err != nil || !strings.Contains(out, `"masterId": "m1"`) || f.writes.Load() != 0 || f.exports != 0 {
					t.Fatalf("read only failed: %v %s", err, out)
				}
				return
			}
			if fault == "" {
				var result map[string]any
				if err != nil || json.Unmarshal([]byte(out), &result) != nil || result["verified"] != true {
					t.Fatalf("result: %v %s", err, out)
				}
				if f.writes.Load() != 1 || f.opens != 2 || f.exports != 2 {
					t.Fatalf("writes=%d opens=%d exports=%d", f.writes.Load(), f.opens, f.exports)
				}
			} else {
				writes := int32(1)
				if fault == "missing-master" {
					writes = 0
				}
				if err == nil || f.writes.Load() != writes {
					t.Fatalf("fault=%s error=%v writes=%d", fault, err, f.writes.Load())
				}
			}
		})
	}
}

func TestReleaseSlideThemePreparationFlow(t *testing.T) {
	_, expected := releaseThemeXML(t)
	decoded, current := releaseThemeDecoded()
	raw, _ := json.Marshal(expected)
	hash := strings.Repeat("a", 64)
	base := map[string]any{"ok": true, "definition": map[string]any{"masters": "source masters", "layouts": "source layouts", "themes": "prefix" + string(raw)}, "decoded": decoded, "currentLayouts": current, "layoutCount": float64(2), "imageSHA256": []any{hash}, "imageKeys": map[string]any{"asset": hash}}
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"success", nil},
		{"unavailable", func(v map[string]any) { v["ok"] = false; v["error"] = "source unavailable" }},
		{"definition missing", func(v map[string]any) { delete(v, "definition") }},
		{"invalid json", func(v map[string]any) { v["definition"].(map[string]any)["themes"] = "prefix{" }},
		{"incomplete theme", func(v map[string]any) { v["definition"].(map[string]any)["themes"] = `{"name":"Only name"}` }},
		{"fractional count", func(v map[string]any) { v["layoutCount"] = float64(1.5) }},
		{"missing hashes", func(v map[string]any) { delete(v, "imageSHA256") }},
		{"invalid hash", func(v map[string]any) { v["imageSHA256"] = []any{"short"} }},
		{"missing source", func(v map[string]any) { delete(v["definition"].(map[string]any), "masters") }},
		{"missing keys", func(v map[string]any) { delete(v, "imageKeys") }},
		{"invalid key hash", func(v map[string]any) { v["imageKeys"] = map[string]any{"asset": float64(1)} }},
		{"invalid delta", func(v map[string]any) { delete(v, "decoded") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := releaseThemeClone(t, base)
			if tc.edit != nil {
				tc.edit(value)
			}
			f := &releaseSlideFlow{extraEval: func(expr string, _ int) (any, bool) {
				if strings.Contains(expr, "const decoded=Object.fromEntries") {
					return value, true
				}
				return nil, false
			}}
			browser := f.editor(t, 1)
			defer browser.Close()
			spec := phase3SlideThemeSpec{ID: "official"}
			err := phase3PrepareSlideTheme(browser, &spec)
			if tc.edit != nil {
				if err == nil {
					t.Fatal("corrupt source accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(spec.Expected, expected) || spec.LayoutCount != 2 || spec.SlideLayouts["slide-b"] != 1 || spec.ImageKeys["asset"] != hash || len(spec.Images) != 1 || spec.Geometry[0]["title:0"].X != 10 {
				t.Fatalf("preparation incomplete: %+v", spec)
			}
			if f.writes.Load() != 0 {
				t.Fatal("preparation wrote presentation")
			}
		})
	}
}
