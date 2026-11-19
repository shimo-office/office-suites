package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func releaseMediaEntries(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{}
	for _, f := range z.File {
		r, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(r)
		r.Close()
		if e != nil {
			t.Fatal(e)
		}
		entries[f.Name] = string(b)
	}
	return entries
}
func releaseMediaPic(kind, name, id, reference string) string {
	return `<p:pic><p:nvPicPr><p:cNvPr id="` + id + `" name="` + name + `"/><p:nvPr><a:` + kind + `File r:link="` + reference + `"/></p:nvPr></p:nvPicPr><p:blipFill><a:blip r:embed="poster"/></p:blipFill><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm></p:spPr></p:pic>`
}
func releaseMediaPackage(t *testing.T, base []byte, kind, name string, media []byte, timing string) []byte {
	t.Helper()
	entries := releaseMediaEntries(t, base)
	page := entries["ppt/slides/slide1.xml"]
	page = strings.Replace(page, `xmlns:a="`+artifactDrawingNS+`"`, `xmlns:a="`+artifactDrawingNS+`" xmlns:r="`+artifactOfficeRelNS+`"`, 1)
	page = strings.Replace(page, `</p:spTree>`, releaseMediaPic(kind, name, "5", "real")+`</p:spTree>`, 1)
	entries["ppt/slides/slide1.xml"] = strings.Replace(page, `</p:sld>`, timing+`</p:sld>`, 1)
	entries["ppt/slides/_rels/slide1.xml.rels"] = `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="real" Type="` + artifactOfficeRelNS + `/` + kind + `" Target="../media/clip.bin"/><Relationship Id="poster" Type="` + artifactOfficeRelNS + `/image" Target="../media/poster.png"/></Relationships>`
	entries["ppt/media/clip.bin"] = string(media)
	entries["ppt/media/poster.png"] = string(releaseFlowPNG(t))
	b, e := os.ReadFile(artifactTestZip(t, entries))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func releaseMediaWrite(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "media.pptx")
	if e := os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	return p
}

func TestReleaseSlideMediaPipeline(t *testing.T) {
	for _, tc := range []struct{ kind, ext, fault string }{{"audio", "wav", ""}, {"video", "mp4", ""}, {"audio", "wav", "media-download-mismatch"}} {
		t.Run(tc.kind+tc.fault, func(t *testing.T) {
			media := []byte("RIFF00000000WAVE synthetic offline fixture")
			file := filepath.Join(t.TempDir(), "clip."+tc.ext)
			if err := os.WriteFile(file, media, 0600); err != nil {
				t.Fatal(err)
			}
			before := releaseFlowPPTX(t, []string{releaseFlowOriginal, releaseFlowOriginal}, -1)
			after := releaseMediaPackage(t, before, tc.kind, filepath.Base(file), media, "")
			payload, _ := json.Marshal(map[string]any{"slideId": "s1", "kind": tc.kind, "file": file})
			f := &releaseSlideFlow{action: "slide-media-insert", payload: string(payload), fault: tc.fault, beforeIDs: []string{"s1", "s2"}, afterIDs: []string{"s1", "s2"}, before: before, after: after, png: releaseFlowPNG(t), mediaRaw: media}
			f.install(t)
			outDir := t.TempDir()
			out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", f.action, "--payload", f.payload, "--out", outDir, "--json", "--relogin"}).Run)
			if f.writes.Load() != 1 || f.opens != 2 {
				t.Fatalf("media replay/reopen: writes=%d opens=%d error=%v", f.writes.Load(), f.opens, err)
			}
			if tc.fault != "" {
				if err == nil || out != "" || !strings.Contains(err.Error(), "独立哈希") {
					t.Fatalf("changed download accepted: %v %s", err, out)
				}
				return
			}
			var result map[string]any
			_ = json.Unmarshal([]byte(out), &result)
			if err != nil || result["verified"] != true {
				t.Fatalf("media pipeline: %v %s", err, out)
			}
			dir := str(result, "evidenceDir")
			downloaded, e := os.ReadFile(filepath.Join(dir, "downloaded-media", filepath.Base(file)))
			if e != nil || !bytes.Equal(downloaded, media) {
				t.Fatalf("independent bytes: %v", e)
			}
			raw, e := os.ReadFile(filepath.Join(dir, "playback-evidence.json"))
			if e != nil {
				t.Fatal(e)
			}
			var playback map[string]any
			_ = json.Unmarshal(raw, &playback)
			if playback["data"] != nil || playback["loadedSourceMatched"] != true || playback["downloadPath"] == nil {
				t.Fatalf("playback evidence metadata: %s", raw)
			}
		})
	}
}

func TestReleaseSlideMediaPersistenceRejectsWrongResourcesAndTiming(t *testing.T) {
	media := []byte("synthetic audio bytes")
	baseline := releaseFlowPPTX(t, []string{releaseFlowOriginal, releaseFlowOriginal}, -1)
	valid := releaseMediaPackage(t, baseline, "audio", "clip.wav", media, `<p:timing><p:audio><p:cMediaNode><p:tgtEl><p:spTgt spid="5"/></p:tgtEl></p:cMediaNode></p:audio></p:timing>`)
	before := releaseMediaWrite(t, baseline)
	spec := phase3SlidesSpec{SlideID: "s1", Media: &phase3SlideMediaSpec{Kind: "audio", Name: "clip.wav", SHA256: fmt.Sprintf("%x", sha256.Sum256(media))}}
	if err := verifyPhase3SlideMediaArtifacts(before, releaseMediaWrite(t, valid), []string{"s1", "s2"}, []string{"s1", "s2"}, spec); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(map[string]string)
	}{
		{"media-bytes", func(e map[string]string) { e["ppt/media/clip.bin"] = "wrong bytes" }},
		{"missing-media", func(e map[string]string) { delete(e, "ppt/media/clip.bin") }},
		{"missing-reference", func(e map[string]string) { delete(e, "ppt/slides/_rels/slide1.xml.rels") }},
		{"hidden-media", func(e map[string]string) {
			e["ppt/slides/slide1.xml"] = strings.Replace(e["ppt/slides/slide1.xml"], `name="clip.wav"`, `name="clip.wav" hidden="1"`, 1)
		}},
		{"wrong-timing-target", func(e map[string]string) {
			e["ppt/slides/slide1.xml"] = strings.Replace(e["ppt/slides/slide1.xml"], `spid="5"`, `spid="other"`, 1)
		}},
		{"unexpected-animation", func(e map[string]string) {
			e["ppt/slides/slide1.xml"] = strings.Replace(e["ppt/slides/slide1.xml"], `<p:timing>`, `<p:timing><p:anim/>`, 1)
		}},
		{"wrong-media-timing-kind", func(e map[string]string) {
			e["ppt/slides/slide1.xml"] = strings.ReplaceAll(strings.ReplaceAll(e["ppt/slides/slide1.xml"], `<p:audio>`, `<p:video>`), `</p:audio>`, `</p:video>`)
		}},
		{"untargeted-timing", func(e map[string]string) {
			e["ppt/slides/slide1.xml"] = strings.Replace(e["ppt/slides/slide1.xml"], `<p:spTgt spid="5"/>`, "", 1)
		}},
		{"other-page-changed", func(e map[string]string) {
			e["ppt/slides/slide2.xml"] = strings.Replace(e["ppt/slides/slide2.xml"], ">original<", ">changed<", 1)
		}},
		{"duplicate-name", func(e map[string]string) {
			e["ppt/slides/slide1.xml"] = strings.Replace(e["ppt/slides/slide1.xml"], `</p:spTree>`, releaseMediaPic("audio", "clip.wav", "6", "real")+`</p:spTree>`, 1)
		}},
		{"media-missing-from-slide", func(e map[string]string) {
			e["ppt/slides/slide1.xml"] = strings.Replace(e["ppt/slides/slide1.xml"], releaseMediaPic("audio", "clip.wav", "5", "real"), "", 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := releaseMediaEntries(t, valid)
			tc.mutate(entries)
			if err := verifyPhase3SlideMediaArtifacts(before, artifactTestZip(t, entries), []string{"s1", "s2"}, []string{"s1", "s2"}, spec); err == nil {
				t.Fatal("invalid media evidence accepted")
			}
		})
	}
	if err := verifyPhase3SlideMediaArtifacts(releaseMediaWrite(t, valid), releaseMediaWrite(t, valid), []string{"s1", "s2"}, []string{"s1", "s2"}, spec); err == nil {
		t.Fatal("duplicate insertion accepted")
	}
	if _, err := phase3SlideMediaReferences(releaseMediaWrite(t, valid), []string{"s1"}, "s1"); err == nil {
		t.Fatal("wrong page count accepted")
	}
	if _, err := phase3SlideMediaReferences(releaseMediaWrite(t, valid), []string{"s1", "s2"}, "missing"); err == nil {
		t.Fatal("unknown target accepted")
	}
}
