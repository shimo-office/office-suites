package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// This wire peer implements only the CDP replies required by the command under
// test. It cannot contact Chrome or a remote site; returned PNGs are synthetic.
func releaseDocumentEditor(t *testing.T, evaluate func(string) any, screenshot func() error, reload func()) *phase3EditorBrowser {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.Set(x, y, color.RGBA{uint8(x * y), uint8(x*x + y), uint8(y*y + x), 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
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
			case "Page.reload":
				if reload != nil {
					reload()
				}
			case "Page.captureScreenshot":
				if screenshot != nil {
					if err := screenshot(); err != nil {
						_ = peer.sendJSON(map[string]any{"id": req["id"], "error": map[string]any{"message": err.Error()}})
						continue
					}
				}
				result["data"] = base64.StdEncoding.EncodeToString(encoded.Bytes())
			case "Runtime.evaluate":
				params, _ := req["params"].(map[string]any)
				result["result"] = map[string]any{"value": evaluate(str(params, "expression"))}
			default:
				t.Errorf("unexpected CDP method: %s", str(req, "method"))
				return
			}
			if peer.sendJSON(map[string]any{"id": req["id"], "result": result}) != nil {
				return
			}
		}
	}()
	return &phase3EditorBrowser{ws: &WSConn{conn: client, rd: bufio.NewReader(client)}}
}

func releaseDocumentEvidence(t *testing.T, out string, success, attempted bool) string {
	t.Helper()
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 1 {
		t.Fatalf("missing unique evidence directory: %v %v", entries, err)
	}
	dir := filepath.Join(out, entries[0].Name())
	name := "failure.json"
	if success {
		name = "result.json"
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if json.Unmarshal(b, &result) != nil || result["ok"] != success {
		t.Fatalf("bad result: %s", b)
	}
	if !success {
		if result["mutationAttempted"] != attempted {
			t.Fatalf("wrong attempted metadata: %s", b)
		}
		if _, err := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(err) {
			t.Fatal("failed run has success evidence")
		}
	} else if result["verified"] != true {
		t.Fatalf("missing final artifact verification: %s", b)
	}
	return dir
}

func TestReleaseDocumentFormatPipelineAndPostwriteFailures(t *testing.T) {
	before := releaseDocumentPackage(t, `<w:p><w:r><w:t>Target</w:t></w:r></w:p><w:p><w:r><w:t>Untouched</w:t></w:r></w:p><w:sectPr/>`)
	after := releaseDocumentPackage(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Target</w:t></w:r></w:p><w:p><w:r><w:t>Untouched</w:t></w:r></w:p><w:sectPr/>`)
	for _, fault := range []string{"control", "open-failure", "permission", "presence", "before-screenshot", "write-rejected", "save-failure", "after-export", "no-op", "reload-reverted", "reload-screenshot"} {
		t.Run(fault, func(t *testing.T) {
			var writes, exports, reloads, shots atomic.Int32
			releaseDocumentHTTP(t, func() ([]byte, int) {
				exports.Add(1)
				if writes.Load() > 0 {
					if fault == "after-export" {
						return nil, 403
					}
					if fault != "no-op" && !(fault == "reload-reverted" && reloads.Load() > 0) {
						return after, 200
					}
				}
				return before, 200
			}, `{"type":"documentPro","name":"Synthetic"}`)
			old := phase3OpenDocumentEditor
			t.Cleanup(func() { phase3OpenDocumentEditor = old })
			var p map[string]any
			_ = json.Unmarshal([]byte(releaseDocumentPayloads["document-format"]), &p)
			spec, err := prepareDocumentFormat(p)
			if err != nil {
				t.Fatal(err)
			}
			mutation := documentFormatJS(spec)
			phase3OpenDocumentEditor = func(target, ready string) (*phase3EditorBrowser, error) {
				if target != "https://phase3.test/docx/abcdefgh" || ready != documentReadyJS {
					t.Fatalf("wrong browser target/readiness: %s %s", target, ready)
				}
				if fault == "open-failure" {
					return nil, errors.New("synthetic transport failure")
				}
				return releaseDocumentEditor(t, func(expr string) any {
					switch {
					case expr == mutation:
						writes.Add(1)
						if fault == "write-rejected" {
							return map[string]any{"ok": false, "error": "synthetic write uncertain"}
						}
						return map[string]any{"ok": true, "selectedText": "Target", "applied": []string{"bold"}}
					case strings.Contains(expr, "pageElapsedMs"):
						if fault == "permission" {
							return map[string]any{"ok": false, "error": "synthetic read-only role"}
						}
						return map[string]any{"ready": true}
					case strings.Contains(expr, "haveUnsavedChange"):
						return map[string]any{"ok": fault != "save-failure", "error": "synthetic save timeout"}
					case strings.Contains(expr, "const presence="):
						return map[string]any{"ok": fault != "presence", "error": "synthetic other account present"}
					case strings.Contains(expr, "selectedText:selected"), strings.Contains(expr, "__shimoPhase3BeforeReload"):
						return map[string]any{"ok": true}
					default:
						t.Errorf("unexpected expression %.120s", expr)
						return map[string]any{"ok": false, "error": "unexpected probe"}
					}
				}, func() error {
					shots.Add(1)
					if fault == "before-screenshot" || fault == "reload-screenshot" && reloads.Load() > 0 {
						return errors.New("synthetic screenshot failure")
					}
					return nil
				}, func() { reloads.Add(1) }), nil
			}
			out := t.TempDir()
			a := newApp([]string{"phase3", "https://phase3.test/docx/abcdefgh", "--action", "document-format", "--payload", releaseDocumentPayloads["document-format"], "--out", out, "--relogin", "--json"})
			a.Origin = "https://phase3.test"
			err = a.Run()
			success := fault == "control"
			if (err == nil) != success {
				t.Fatalf("unexpected outcome: %v", err)
			}
			attempted := fault != "open-failure" && fault != "permission" && fault != "presence" && fault != "before-screenshot"
			want := int32(0)
			if attempted {
				want = 1
			}
			if writes.Load() != want {
				t.Fatalf("writes=%d want=%d err=%v", writes.Load(), want, err)
			}
			dir := releaseDocumentEvidence(t, out, success, attempted)
			if attempted && !success && !strings.Contains(err.Error(), "重放") {
				t.Fatalf("missing no-replay warning: %v", err)
			}
			if success {
				if exports.Load() != 3 || reloads.Load() != 1 || shots.Load() != 3 {
					t.Fatalf("incomplete verification: exports=%d reloads=%d shots=%d", exports.Load(), reloads.Load(), shots.Load())
				}
				if _, err = os.Stat(filepath.Join(dir, "target-node-assertions.json")); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
