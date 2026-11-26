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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func releasePhase2Peer(t *testing.T, handle func(string, map[string]any) (map[string]any, error)) *WSConn {
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
			params, _ := req["params"].(map[string]any)
			result, e := handle(str(req, "method"), params)
			reply := map[string]any{"id": req["id"], "result": result}
			if e != nil {
				delete(reply, "result")
				reply["error"] = map[string]any{"code": -32000, "message": e.Error()}
			}
			if peer.sendJSON(reply) != nil {
				return
			}
		}
	}()
	return &WSConn{conn: client, rd: bufio.NewReader(client)}
}

func TestReleasePhase2CookieAndReadinessProtocol(t *testing.T) {
	cookieCalls := 0
	pageQueries := 0
	editorQueries := 0
	ws := releasePhase2Peer(t, func(method string, p map[string]any) (map[string]any, error) {
		switch method {
		case "Network.setCookies":
			cookieCalls++
			cookies := p["cookies"].([]any)
			if len(cookies) != 2 {
				t.Error("malformed cookie included", cookies)
			}
			for _, v := range cookies {
				c := v.(map[string]any)
				if c["url"] != "https://phase3.test/docs/abcdefgh" || c["secure"] != true || c["httpOnly"] != true || c["path"] != "/" {
					t.Error(c)
				}
			}
			return map[string]any{}, nil
		case "Runtime.evaluate":
			if strings.Contains(str(p, "expression"), "document.readyState") {
				pageQueries++
			} else {
				editorQueries++
			}
			return map[string]any{"result": map[string]any{"value": true}}, nil
		default:
			return nil, errors.New("unexpected method")
		}
	})
	defer ws.Close()
	if err := setCookies(ws, "https://phase3.test/docs/abcdefgh", "invalid; =skip; sid=a=b; theme=light"); err != nil {
		t.Fatal(err)
	}
	if err := setCookies(nil, "https://phase3.test/docs/abcdefgh", "invalid; =skip"); err != nil {
		t.Fatal(err)
	}
	waitPageReady(ws, time.Second)
	if !waitEditorReady(ws, time.Second, "sdk.history.show") || cookieCalls != 1 || pageQueries != 1 || editorQueries != 1 {
		t.Fatal(cookieCalls, pageQueries, editorQueries)
	}
	timeout := releasePhase2Peer(t, func(string, map[string]any) (map[string]any, error) {
		return map[string]any{"result": map[string]any{"value": false}}, nil
	})
	defer timeout.Close()
	waitPageReady(timeout, time.Millisecond)
	if waitEditorReady(timeout, time.Millisecond, "sdk.history.show") {
		t.Fatal("unready editor accepted")
	}
}

func TestReleasePhase2ScreenshotTransportAndFilesystemFailures(t *testing.T) {
	for _, mode := range []string{"success", "rpc-error", "missing-data", "invalid-base64", "parent-file", "destination-directory"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "sub", "proof.png")
			if mode == "parent-file" {
				if e := os.WriteFile(filepath.Join(root, "sub"), []byte("x"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "destination-directory" {
				target = root
			}
			ws := releasePhase2Peer(t, func(method string, p map[string]any) (map[string]any, error) {
				if method != "Page.captureScreenshot" || p["format"] != "png" {
					t.Error(method, p)
				}
				switch mode {
				case "rpc-error":
					return nil, errors.New("synthetic screenshot failure")
				case "missing-data":
					return map[string]any{}, nil
				case "invalid-base64":
					return map[string]any{"data": "!bad"}, nil
				}
				return map[string]any{"data": base64.StdEncoding.EncodeToString([]byte("synthetic byte payload"))}, nil
			})
			defer ws.Close()
			err := capturePhase2Screenshot(ws, target)
			if mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
				raw, e := os.ReadFile(target)
				if e != nil || string(raw) != "synthetic byte payload" {
					t.Fatal(e, string(raw))
				}
				if screenshotHasContent(target) {
					t.Fatal("non-PNG accepted as real screenshot")
				}
			} else if err == nil {
				t.Fatal("capture failure hidden")
			}
		})
	}
}

func TestReleasePhase2ReadinessPortAndImageBoundaries(t *testing.T) {
	root := t.TempDir()
	port := filepath.Join(root, "DevToolsActivePort")
	if e := os.WriteFile(port, []byte("12345\n/devtools/browser/synthetic"), 0600); e != nil {
		t.Fatal(e)
	}
	if got, err := waitDevToolsPort(port, time.Second); err != nil || got != 12345 {
		t.Fatal(got, err)
	}
	if e := os.WriteFile(port, []byte("not-a-port"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, err := waitDevToolsPort(port, time.Millisecond); err == nil {
		t.Fatal("invalid port accepted")
	}
	if screenshotHasContent(filepath.Join(root, "missing.png")) {
		t.Fatal("missing screenshot accepted")
	}
	for _, size := range []int{1, 8} {
		img := image.NewRGBA(image.Rect(0, 0, size, size))
		img.Set(0, 0, color.Black)
		var raw bytes.Buffer
		_ = png.Encode(&raw, img)
		p := filepath.Join(root, "small.png")
		_ = os.WriteFile(p, raw.Bytes(), 0600)
		if !screenshotHasContent(p) {
			t.Fatal("nonblank small image caused invalid sampling")
		}
	}
	if isRetryableEditorError(nil) || isRetryableEditorError(errors.New("permission denied")) || !isRetryableEditorError(errors.New("cannot read editor instance")) {
		t.Fatal("retry classification broken")
	}
}

func TestReleasePhase2TargetAndRequestFailureBoundaries(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	old := httpClient
	defer func() { httpClient = old }()
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/files/abcdefgh" {
			return phase3Response(200, `{"guid":"abcdefgh","type":"spreadsheet"}`), nil
		}
		return phase3Response(404, `{}`), nil
	})}
	a := newApp(nil)
	a.Origin = "https://phase3.test"
	if target, err := a.phase2Target("abcdefgh"); err != nil || target != "https://phase3.test/sheets/abcdefgh" {
		t.Fatal(target, err)
	}
	for _, target := range []string{"desktop", "@missing", "ijklmnop"} {
		if _, err := a.phase2Target(target); err == nil {
			t.Fatal("invalid target accepted", target)
		}
	}
	if _, err := a.runPhase2HTTP("abcdefgh", "invalid", nil); err == nil {
		t.Fatal("invalid internal API method accepted")
	}
	for _, args := range [][]string{{"editor"}, {"editor", "https://phase3.test/desktop", "--action", "sheet-values", "--payload", `{"range":"Sheet1!A1"}`}, {"editor", "@missing", "--action", "sheet-update", "--payload", `{}`}} {
		app := newApp(args)
		app.Origin = a.Origin
		if err := app.Run(); err == nil {
			t.Fatal("invalid command accepted", args)
		}
	}
	if args, err := phase2Args(" "); err != nil || len(args) != 0 {
		t.Fatal(args, err)
	}
}
