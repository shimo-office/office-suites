package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const releaseFlowOriginal = `<p:sp><p:nvSpPr><p:cNvPr id="1" name="Original"/></p:nvSpPr><p:txBody><a:p><a:r><a:t>original</a:t></a:r></a:p></p:txBody></p:sp>`
const releaseFlowInserted = `<p:sp><p:nvSpPr><p:cNvPr id="4" name="Inserted"/></p:nvSpPr><p:spPr><a:xfrm><a:off x="95250" y="190500"/><a:ext cx="2857500" cy="762000"/></a:xfrm></p:spPr><p:txBody><a:p><a:r><a:rPr sz="2400" b="1"/><a:t>unique text</a:t></a:r></a:p></p:txBody></p:sp>`

// These files are generated OOXML fixtures, not browser exports. The command
// must parse and compare their real ZIP/XML bytes through its normal pipeline.
func releaseFlowPPTX(t *testing.T, pages []string, hidden int) []byte {
	t.Helper()
	entries := map[string]string{
		"[Content_Types].xml":  `<Types xmlns="` + artifactContentNS + `"><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/></Types>`,
		"_rels/.rels":          `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="doc" Type="` + artifactOfficeRelNS + `/officeDocument" Target="ppt/presentation.xml"/></Relationships>`,
		"ppt/theme/theme1.xml": `<a:theme xmlns:a="` + artifactDrawingNS + `"><a:themeElements/></a:theme>`,
	}
	var ids, rels strings.Builder
	for i, page := range pages {
		fmt.Fprintf(&ids, `<p:sldId id="%d" r:id="r%d"/>`, 256+i, i+1)
		fmt.Fprintf(&rels, `<Relationship Id="r%d" Type="%s/slide" Target="slides/slide%d.xml"/>`, i+1, artifactOfficeRelNS, i+1)
		show := ""
		if hidden == i {
			show = ` show="0"`
		}
		entries[fmt.Sprintf("ppt/slides/slide%d.xml", i+1)] = `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"` + show + `><p:cSld><p:spTree>` + page + `</p:spTree></p:cSld></p:sld>`
	}
	entries["ppt/presentation.xml"] = `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst>` + ids.String() + `</p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`
	entries["ppt/_rels/presentation.xml.rels"] = `<Relationships xmlns="` + artifactRelNS + `">` + rels.String() + `</Relationships>`
	p := artifactTestZip(t, entries)
	if _, err := verifyArtifact(p, "pptx", nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func releaseFlowPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.Set(x, y, color.RGBA{uint8(x * y), uint8(x*x + y), uint8(x + y*y), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type releaseSlideFlow struct {
	extraEval              func(string, int) (any, bool)
	mediaRaw               []byte
	action, payload, fault string
	beforeIDs, afterIDs    []string
	before, after, png     []byte
	writes                 atomic.Int32
	opens, exports         int
}

// The peer emulates protocol responses only, independently of the production
// JavaScript. Actual SDK mutation expressions have their own Node guard tests.
func (f *releaseSlideFlow) editor(t *testing.T, opening int) *phase3EditorBrowser {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	go func() {
		defer server.Close()
		peer := &WSConn{conn: server, rd: bufio.NewReader(server)}
		for {
			raw, err := peer.readMessage()
			if err != nil {
				return
			}
			var req map[string]any
			if json.Unmarshal(raw, &req) != nil {
				return
			}
			result := map[string]any{}
			switch str(req, "method") {
			case "Browser.close":
			case "Page.captureScreenshot":
				result["data"] = base64.StdEncoding.EncodeToString(f.png)
			case "Runtime.evaluate":
				expr := str(asMap(req["params"]), "expression")
				var value any = map[string]any{"ok": true}
				switch {
				case strings.Contains(expr, "const request=") || strings.Contains(expr, "insertTextBox(") || strings.Contains(expr, "wb.core.replace(wb.editor") || strings.Contains(expr, "const tables=await slide.getTables()") || strings.Contains(expr, "staged=window.__shimoPhase3SlideMedia") || strings.Contains(expr, "p=r.shape"):
					if n := f.writes.Add(1); n != 1 {
						t.Errorf("mutation expression replayed %d times", n)
					}
					if f.fault == "mutation-exception" {
						result["exceptionDetails"] = map[string]any{"exception": map[string]any{"description": "synthetic accepted write then disconnected"}}
					} else {
						value = map[string]any{"ok": true, "before": f.beforeIDs, "after": f.afterIDs, "addedId": "new", "shapeId": "new-shape"}
					}
				case strings.Contains(expr, "wb.media.checkSelectFile"):
					value = map[string]any{"ok": true, "duration": float64(1)}
				case strings.Contains(expr, "data: btoa(data)"):
					data := f.mediaRaw
					if f.fault == "media-download-mismatch" {
						data = []byte("changed")
					}
					value = map[string]any{"ok": true, "data": base64.StdEncoding.EncodeToString(data), "loadedSourceMatched": true, "duration": float64(1), "playback": []any{map[string]any{"time": float64(0)}, map[string]any{"time": float64(0.2)}}}
				case strings.Contains(expr, "return {ok:true,ids:api.getSlides()"):
					ids := f.beforeIDs
					if f.writes.Load() > 0 {
						ids = f.afterIDs
					}
					if f.fault == "reload-order" && opening > 1 {
						ids = []string{"changed", "order"}
					}
					if f.fault == "empty-ids" {
						ids = nil
					}
					if f.fault == "duplicate-ids" {
						ids = []string{"s1", "s1"}
					}
					value = map[string]any{"ok": true, "ids": ids}
				case strings.Contains(expr, "otherUsers:0"):
					if f.fault == "presence" {
						value = map[string]any{"ok": false, "error": "检测到其他用户正在协作"}
					}
				case strings.Contains(expr, "const frame="):
					if f.fault == "before-canvas" || f.fault == "after-canvas" && f.writes.Load() > 0 {
						value = map[string]any{"ok": false, "error": "未发现可见幻灯片画布"}
					}
				case strings.Contains(expr, "api.setCurrentSlideIndex(id)"):
				default:
					if f.extraEval != nil {
						if extra, handled := f.extraEval(expr, opening); handled {
							value = extra
							break
						}
					}
					t.Errorf("unhandled CDP expression: %.120s", expr)
					value = map[string]any{"ok": false, "error": "unexpected fixture expression"}
				}
				if result["exceptionDetails"] == nil {
					result["result"] = map[string]any{"value": value}
				}
			default:
				t.Errorf("unhandled CDP method: %s", str(req, "method"))
				return
			}
			if err = peer.sendJSON(map[string]any{"id": req["id"], "result": result}); err != nil {
				return
			}
		}
	}()
	return &phase3EditorBrowser{ws: &WSConn{conn: client, rd: bufio.NewReader(client)}}
}

func (f *releaseSlideFlow) install(t *testing.T) {
	t.Helper()
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic-slide-flow")
	t.Setenv("SHIMO_NET_ORIGIN", "https://slide-flow.invalid")
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	old, oldDefault, oldOpen := httpClient, http.DefaultTransport, phase3OpenSlidesEditor
	t.Cleanup(func() { httpClient = old; http.DefaultTransport = oldDefault; phase3OpenSlidesEditor = oldOpen })
	transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "slide-flow.invalid" {
			t.Errorf("unapproved fixture host: %s", r.URL)
			return nil, errors.New("no network allowed")
		}
		switch r.URL.Path {
		case "/api/v1/files/abcdefgh":
			if f.fault == "file-type" {
				return phase3Response(200, `{"type":"document","name":"synthetic"}`), nil
			}
			return phase3Response(200, `{"type":"presentation","name":"synthetic"}`), nil
		case "/api/v1/files/export/abcdefgh":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["type"] != "pptx" {
				t.Errorf("fallback/incorrect export: %v", payload)
			}
			f.exports++
			if f.fault == "export-401" && f.writes.Load() > 0 {
				return phase3Response(401, `{}`), nil
			}
			return phase3Response(200, `{"data":{"taskId":"synthetic-export"}}`), nil
		case "/api/v1/files/export/progress":
			return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://slide-flow.invalid/fixture.pptx"}}`), nil
		case "/fixture.pptx":
			data := f.before
			if f.writes.Load() > 0 && f.fault != "no-op" {
				data = f.after
			}
			if f.fault == "empty-export" && f.writes.Load() > 0 {
				data = []byte{}
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		return nil, errors.New("unexpected fixture request")
	})
	httpClient = &http.Client{Transport: transport}
	http.DefaultTransport = transport
	phase3OpenSlidesEditor = func(target string) (*phase3EditorBrowser, error) {
		f.opens++
		if !strings.Contains(target, "dbg=1") {
			t.Errorf("missing SDK debug flag: %s", target)
		}
		if f.fault == "reopen" && f.opens > 1 {
			return nil, errors.New("synthetic fresh editor unavailable")
		}
		return f.editor(t, f.opens), nil
	}
}

func TestReleaseSlideFlowSuccess(t *testing.T) {
	unchanged := strings.ReplaceAll(releaseFlowOriginal, "original", "second page unchanged")
	formatted := strings.Replace(releaseFlowOriginal, "<a:r><a:t>", `<a:r><a:rPr b="1"/><a:t>`, 1)
	for _, tc := range []struct {
		action, payload string
		afterIDs, pages []string
		hidden          int
	}{
		{"slide-add", `{"slideId":"s1"}`, []string{"s1", "new", "s2"}, []string{releaseFlowOriginal, "", unchanged}, -1},
		{"slide-duplicate", `{"slideId":"s1"}`, []string{"s1", "new", "s2"}, []string{releaseFlowOriginal, releaseFlowOriginal, unchanged}, -1},
		{"slide-delete", `{"slideId":"s1"}`, []string{"s2"}, []string{unchanged}, -1},
		{"slide-hide", `{"slideId":"s1"}`, []string{"s1", "s2"}, []string{releaseFlowOriginal, unchanged}, 0},
		{"slide-text-insert", `{"slideId":"s1","text":"unique text","x":10,"y":20,"width":300,"height":80,"fontSize":24,"bold":true}`, []string{"s1", "s2"}, []string{releaseFlowOriginal + releaseFlowInserted, unchanged}, -1},
		{"slide-text-format", `{"slideId":"s1","text":"original","bold":true}`, []string{"s1", "s2"}, []string{formatted, unchanged}, -1},
		{"slide-replace-all", `{"old":"original","new":"replacement"}`, []string{"s1", "s2"}, []string{strings.ReplaceAll(releaseFlowOriginal, "original", "replacement"), unchanged}, -1},
	} {
		t.Run(tc.action, func(t *testing.T) {
			f := &releaseSlideFlow{action: tc.action, payload: tc.payload, beforeIDs: []string{"s1", "s2"}, afterIDs: tc.afterIDs, before: releaseFlowPPTX(t, []string{releaseFlowOriginal, unchanged}, -1), after: releaseFlowPPTX(t, tc.pages, tc.hidden), png: releaseFlowPNG(t)}
			f.install(t)
			outDir := t.TempDir()
			out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", f.action, "--payload", f.payload, "--out", outDir, "--json", "--relogin"}).Run)
			if err != nil {
				t.Fatalf("command failed: %v", err)
			}
			var result map[string]any
			if err = json.Unmarshal([]byte(out), &result); err != nil || result["verified"] != true || result["action"] != f.action {
				t.Fatalf("success envelope: %s %v", out, err)
			}
			if f.writes.Load() != 1 || f.opens != 2 || f.exports != 2 {
				t.Fatalf("writes=%d opens=%d exports=%d", f.writes.Load(), f.opens, f.exports)
			}
			dir := str(result, "evidenceDir")
			for _, name := range []string{"before.png", "after.png", "reload.png", "result.json", "target-node-assertions.json", "submission.json", "command-log.json"} {
				if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}
			data, _ := os.ReadFile(filepath.Join(dir, "command-log.json"))
			var record map[string]any
			_ = json.Unmarshal(data, &record)
			if record["mutationAttempted"] != true || record["exitCode"] != float64(0) || len(str(record, "binarySHA256")) != 64 {
				t.Fatalf("command provenance: %s", data)
			}
			artifact := asMap(result["artifact"])
			actual, err := os.ReadFile(str(artifact, "path"))
			if err != nil || !bytes.Equal(actual, f.after) {
				t.Fatalf("persisted artifact differs from downloaded bytes: %v artifact=%v", err, artifact)
			}
		})
	}
}

func TestReleaseSlideFlowRejectedOrUnverifiedNeverReplays(t *testing.T) {
	for _, fault := range []string{"presence", "before-canvas", "empty-ids", "duplicate-ids", "file-type", "mutation-exception", "after-canvas", "export-401", "empty-export", "no-op", "reopen", "reload-order"} {
		t.Run(fault, func(t *testing.T) {
			before := releaseFlowPPTX(t, []string{releaseFlowOriginal, releaseFlowOriginal}, -1)
			f := &releaseSlideFlow{action: "slide-add", payload: `{"slideId":"s1"}`, fault: fault, beforeIDs: []string{"s1", "s2"}, afterIDs: []string{"s1", "new", "s2"}, before: before, after: releaseFlowPPTX(t, []string{releaseFlowOriginal, "", releaseFlowOriginal}, -1), png: releaseFlowPNG(t)}
			f.install(t)
			outDir := t.TempDir()
			out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", f.action, "--payload", f.payload, "--out", outDir, "--json", "--relogin"}).Run)
			preflight := fault == "presence" || fault == "before-canvas" || fault == "empty-ids" || fault == "duplicate-ids" || fault == "file-type"
			want := int32(1)
			if preflight {
				want = 0
			}
			if err == nil || out != "" || f.writes.Load() != want {
				t.Fatalf("fault=%s err=%v out=%s writes=%d want=%d", fault, err, out, f.writes.Load(), want)
			}
			if !preflight && (!strings.Contains(err.Error(), "禁止自动重放") || !strings.Contains(err.Error(), "可能已生效")) {
				t.Fatalf("uncertain write missing disclosure: %v", err)
			}
			dirs, _ := os.ReadDir(outDir)
			if fault == "file-type" {
				if len(dirs) != 0 {
					t.Fatal("invalid file started editor/evidence")
				}
				return
			}
			if len(dirs) != 1 {
				t.Fatalf("expected one failure directory: %v", dirs)
			}
			dir := filepath.Join(outDir, dirs[0].Name())
			if _, err := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(err) {
				t.Fatal("failed command emitted success artifact")
			}
			data, e := os.ReadFile(filepath.Join(dir, "failure.json"))
			if e != nil {
				t.Fatal(e)
			}
			var failure map[string]any
			_ = json.Unmarshal(data, &failure)
			if failure["ok"] != false || failure["mutationAttempted"] != !preflight || failure["doNotReplay"] != !preflight {
				t.Fatalf("incorrect failure evidence: %s", data)
			}
		})
	}
}

func TestReleaseSlideFlowInspectIsReadOnly(t *testing.T) {
	f := &releaseSlideFlow{beforeIDs: []string{"s1", "s2"}}
	f.install(t)
	outDir := t.TempDir()
	out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", "slide-inspect", "--out", outDir, "--json"}).Run)
	var result map[string]any
	_ = json.Unmarshal([]byte(out), &result)
	if err != nil || result["ok"] != true || len(asSlice(result["slideIds"])) != 2 || result["verified"] != nil || f.writes.Load() != 0 || f.exports != 0 || f.opens != 1 {
		t.Fatalf("read-only inspect: %v %s writes=%d exports=%d opens=%d", err, out, f.writes.Load(), f.exports, f.opens)
	}
}

func TestReleaseSlideFlowPDFExportNeverClaimsRenderingAcceptance(t *testing.T) {
	for _, mode := range []string{"opaque-pdf", "short", "html", "denied"} {
		t.Run(mode, func(t *testing.T) {
			source := releaseFlowPPTX(t, []string{releaseFlowOriginal, releaseFlowOriginal}, 1)
			// A PDF-looking payload deliberately lacks a parsed page tree. Merely
			// downloading this must never produce verified:true or rendered-page proof.
			pdf := []byte("%PDF-1.4\n" + strings.Repeat("% synthetic opaque bytes, not rendered evidence\n", 8) + "%%EOF\n")
			if mode == "short" {
				pdf = []byte("%PDF-")
			}
			if mode == "html" {
				pdf = []byte(strings.Repeat("<html>error</html>", 30))
			}
			exports := []string{}
			current := ""
			t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
			t.Setenv("SHIMO_NET_ORIGIN", "https://slide-flow.invalid")
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			old, oldDefault := httpClient, http.DefaultTransport
			t.Cleanup(func() { httpClient = old; http.DefaultTransport = oldDefault })
			transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "slide-flow.invalid" {
					return nil, errors.New("no network")
				}
				switch r.URL.Path {
				case "/api/v1/files/abcdefgh":
					return phase3Response(200, `{"type":"presentation","name":"synthetic"}`), nil
				case "/api/v1/files/export/abcdefgh":
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					current = str(body, "type")
					exports = append(exports, current)
					if current == "pdf" && mode == "denied" {
						return phase3Response(403, `{"error":"denied"}`), nil
					}
					return phase3Response(200, `{"data":{"taskId":"test"}}`), nil
				case "/api/v1/files/export/progress":
					return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://slide-flow.invalid/download"}}`), nil
				case "/download":
					data := source
					if current == "pdf" {
						data = pdf
					}
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
				}
				return nil, errors.New("unexpected request " + r.URL.String())
			})
			httpClient = &http.Client{Transport: transport}
			http.DefaultTransport = transport
			outDir := t.TempDir()
			out, err := issueCapture(t, newApp([]string{"phase3", "https://slide-flow.invalid/presentation/abcdefgh", "--action", "slide-print-pdf", "--out", outDir, "--json"}).Run)
			if strings.Join(exports, ",") != "pptx,pdf" {
				t.Fatalf("unexpected export fallback: %v", exports)
			}
			dirs, _ := os.ReadDir(outDir)
			if len(dirs) != 1 {
				t.Fatal("missing export provenance")
			}
			dir := filepath.Join(outDir, dirs[0].Name())
			if mode == "opaque-pdf" {
				var result map[string]any
				_ = json.Unmarshal([]byte(out), &result)
				if err != nil || result["verified"] != false || result["status"] != "exported-not-verified" || result["expectedVisiblePages"] != float64(1) || result["sourcePages"] != float64(2) {
					t.Fatalf("export claimed unproved acceptance: %v %s", err, out)
				}
				if hidden := asSlice(result["hiddenSlideIndices"]); len(hidden) != 1 || hidden[0] != float64(2) {
					t.Fatalf("hidden policy: %v", result)
				}
				actual, e := os.ReadFile(str(result, "pdf"))
				if e != nil || !bytes.Equal(actual, pdf) {
					t.Fatalf("download changed bytes: %v", e)
				}
			} else {
				if err == nil || out != "" {
					t.Fatalf("invalid export accepted: %v %s", err, out)
				}
				if _, e := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(e) {
					t.Fatal("invalid export wrote successful result")
				}
				if _, e := os.Stat(filepath.Join(dir, "failure.json")); e != nil {
					t.Fatal("missing failure evidence")
				}
			}
		})
	}
}
