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
)

// Protocol peer exercises production CDP framing and editor error handling.
// Form state comes from independent literal snapshots, never from Expected().
func releaseFormPeer(t *testing.T, eval func(string) (any, error)) *phase3EditorBrowser {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.Set(x, y, color.RGBA{uint8(x * y), uint8(x*x + y), uint8(y*y + x), 255})
		}
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, img); err != nil {
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
			var r map[string]any
			if json.Unmarshal(raw, &r) != nil {
				return
			}
			result := map[string]any{}
			switch str(r, "method") {
			case "Browser.close", "Page.addScriptToEvaluateOnNewDocument", "Page.navigate":
			case "Page.captureScreenshot":
				result["data"] = base64.StdEncoding.EncodeToString(imageBytes.Bytes())
			case "Runtime.evaluate":
				p, _ := r["params"].(map[string]any)
				value, e := eval(str(p, "expression"))
				if e != nil {
					result["exceptionDetails"] = map[string]any{"exception": map[string]any{"description": e.Error()}}
				} else {
					result["result"] = map[string]any{"value": value}
				}
			default:
				t.Errorf("unexpected CDP request %s", str(r, "method"))
				return
			}
			if peer.sendJSON(map[string]any{"id": r["id"], "result": result}) != nil {
				return
			}
		}
	}()
	return &phase3EditorBrowser{ws: &WSConn{conn: client, rd: bufio.NewReader(client)}}
}
func releaseFormDelta(t *testing.T, title string) any {
	t.Helper()
	var v any
	raw := `[{"action":"insert","attributes":{"title":"` + title + `","desc":"keep","descAlign":"left","viewMode":"classic"},"data":{"form":[[{"action":"insert","data":{"object":{"guid":"questionA","required":false,"subject":"A","question":{"type":"text"}}}}],null]}}]`
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestReleaseFormSingleOperationEvidence(t *testing.T) {
	for _, mode := range []string{"success", "initial-save-pending", "changed-during-sync", "permission-lost-during-sync", "identity-changed-during-sync", "wrong-guid", "permission", "stale-server", "no-op", "presence", "mutation-error", "rendered-mismatch"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			old, oldDefault := httpClient, http.DefaultTransport
			defer func() { httpClient = old; http.DefaultTransport = oldDefault }()
			beforeDelta, afterDelta := releaseFormDelta(t, "before"), releaseFormDelta(t, "after")
			before, _ := phase3DecodeFormDelta(beforeDelta)
			after, _ := phase3DecodeFormDelta(afterDelta)
			writes := 0
			downloads := 0
			snapshots := 0
			b := releaseFormPeer(t, func(expr string) (any, error) {
				switch {
				case strings.Contains(expr, "sdkHasUnsavedUnreliable"):
					snapshots++
					form := before
					if (writes > 0 && mode != "rendered-mismatch") || (mode == "changed-during-sync" && snapshots > 1) {
						form = after
					}
					guid := "abcdefgh"
					if mode == "wrong-guid" || (mode == "identity-changed-during-sync" && snapshots > 1) {
						guid = "different"
					}
					return map[string]any{"guid": guid, "form": form, "canEdit": mode != "permission" && !(mode == "permission-lost-during-sync" && snapshots > 1)}, nil
				case strings.HasPrefix(expr, "new window.modoc.Delta("):
					if writes > 0 || mode == "stale-server" || mode == "changed-during-sync" || mode == "permission-lost-during-sync" || mode == "identity-changed-during-sync" || (mode == "initial-save-pending" && downloads == 1) {
						return afterDelta, nil
					}
					return beforeDelta, nil
				case strings.Contains(expr, "switch(r.operation)"):
					writes++
					if mode == "mutation-error" {
						return nil, errors.New("synthetic mutation transport failure")
					}
					return map[string]any{"ok": true}, nil
				case strings.Contains(expr, "const presence="):
					if mode == "presence" {
						return map[string]any{"ok": false, "error": "other user online"}, nil
					}
					return map[string]any{"ok": true}, nil
				default:
					t.Errorf("unexpected expression %.180s", expr)
					return nil, errors.New("unexpected fake expression")
				}
			})
			defer b.Close()
			transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/sdk/forms/abcdefgh/content":
					return phase3Response(200, `{"contentUrl":"https://signed.test/draft?secret=fixture"}`), nil
				case "/draft":
					downloads++
					if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
						t.Error("signed content received account credentials")
					}
					return phase3Response(200, `synthetic-compressed-content`), nil
				}
				t.Errorf("unexpected URL %s", r.URL.Path)
				return phase3Response(500, `{}`), nil
			})
			httpClient = &http.Client{Transport: transport}
			http.DefaultTransport = transport
			a := newApp(nil)
			a.Origin = "https://phase3.test"
			s := &phase3SDKSession{app: a, guid: "abcdefgh", prefix: "/sdk/", target: "https://phase3.test/forms/abcdefgh"}
			title := "after"
			if mode == "no-op" {
				title = "before"
			}
			dir := t.TempDir()
			expected, attempted, err := phase3FormRunOne(b, s, dir, map[string]any{"operation": "meta", "patch": map[string]any{"title": title}})
			if mode == "success" || mode == "initial-save-pending" {
				wantDownloads := 2
				if mode == "initial-save-pending" {
					wantDownloads = 3
				}
				if err != nil || !attempted || writes != 1 || downloads != wantDownloads || phase3VerifyForm(after, expected) != nil {
					t.Fatalf("attempt=%t writes=%d downloads=%d err=%v", attempted, writes, downloads, err)
				}
				for _, name := range []string{"request.json", "before-structure.json", "server-before.json", "expected.json", "before.png", "submission.json", "server-after.json", "after-structure.json", "after.png", "step-result.json"} {
					if _, e := os.Stat(filepath.Join(dir, name)); e != nil {
						t.Fatal(name, e)
					}
				}
				return
			}
			wantAttempt := mode == "mutation-error" || mode == "rendered-mismatch"
			wantWrites := 0
			if wantAttempt {
				wantWrites = 1
			}
			if err == nil || attempted != wantAttempt || writes != wantWrites {
				t.Fatalf("attempt=%t writes=%d error=%v", attempted, writes, err)
			}
			if mode == "stale-server" && downloads != 6 {
				t.Fatalf("preflight retry bound changed: downloads=%d", downloads)
			}
			raw, e := os.ReadFile(filepath.Join(dir, "step-failure.json"))
			if e != nil {
				t.Fatal(e)
			}
			var failure map[string]any
			_ = json.Unmarshal(raw, &failure)
			if failure["mayHaveApplied"] != wantAttempt || failure["mutationAttempted"] != wantAttempt {
				t.Fatal(string(raw))
			}
			if _, e := os.Stat(filepath.Join(dir, "step-result.json")); !os.IsNotExist(e) {
				t.Fatal("failed mutation got successful evidence")
			}
		})
	}
}

func TestReleaseFormBatchCommandPreservesPartialProgress(t *testing.T) {
	for _, mode := range []string{"success", "invalid-second-step", "second-step-error", "fresh-profile-error", "fresh-guid-mismatch", "fresh-form-mismatch", "read-only", "draft"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			old, oldDefault, oldOpen := httpClient, http.DefaultTransport, phase3OpenFormEditor
			defer func() { httpClient = old; http.DefaultTransport = oldDefault; phase3OpenFormEditor = oldOpen }()
			deltas := []any{releaseFormDelta(t, "before"), releaseFormDelta(t, "middle"), releaseFormDelta(t, "after")}
			forms := []map[string]any{}
			for _, d := range deltas {
				f, _ := phase3DecodeFormDelta(d)
				forms = append(forms, f)
			}
			writes, opens := 0, 0
			phase3OpenFormEditor = func(target, readiness string) (*phase3EditorBrowser, error) {
				opens++
				if target != "https://phase3.test/forms/abcdefgh?debug=1" || readiness != phase3FormReady {
					t.Error("incorrect editor startup target")
				}
				if opens == 2 && mode == "fresh-profile-error" {
					return nil, errors.New("synthetic fresh startup failure")
				}
				fresh := opens > 1
				return releaseFormPeer(t, func(expr string) (any, error) {
					switch {
					case strings.Contains(expr, "pageElapsedMs"):
						return map[string]any{"ready": true}, nil
					case strings.Contains(expr, "changeRouterPage"):
						return map[string]any{"ok": true}, nil
					case strings.Contains(expr, "sdkHasUnsavedUnreliable"):
						guid := "abcdefgh"
						form := forms[writes]
						if fresh && mode == "fresh-guid-mismatch" {
							guid = "wrong-guid"
						}
						if fresh && mode == "fresh-form-mismatch" {
							form = forms[0]
						}
						return map[string]any{"guid": guid, "form": form, "canEdit": mode != "read-only"}, nil
					case strings.HasPrefix(expr, "new window.modoc.Delta("):
						return deltas[writes], nil
					case strings.Contains(expr, "switch(r.operation)"):
						writes++
						if writes > 2 {
							return nil, errors.New("batch was replayed")
						}
						if writes == 2 && mode == "second-step-error" {
							return nil, errors.New("synthetic acknowledgement failure after applying second mutation")
						}
						return map[string]any{"ok": true}, nil
					case strings.Contains(expr, "const presence="):
						return map[string]any{"ok": true}, nil
					default:
						t.Errorf("unexpected expression %.100s", expr)
						return nil, errors.New("unexpected fake expression")
					}
				}), nil
			}
			transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/v1/files/abcdefgh":
					return phase3Response(200, `{"type":"form"}`), nil
				case "/forms/abcdefgh":
					return phase3Response(200, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/","SDK_V2_SIGNATURE":"test","SDK_V2_TOKEN":"test"}}];`), nil
				case "/sdk/forms/abcdefgh/content":
					return phase3Response(200, `{"contentUrl":"https://signed.test/draft"}`), nil
				case "/draft":
					if r.Header.Get("Cookie") != "" {
						t.Error("signed draft got cookie")
					}
					return phase3Response(200, "compressed synthetic delta"), nil
				}
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				return phase3Response(500, `{}`), nil
			})
			httpClient = &http.Client{Transport: transport}
			http.DefaultTransport = transport
			payload := `{"operations":[{"operation":"meta","patch":{"title":"middle"}},{"operation":"meta","patch":{"title":"after"}}]}`
			if mode == "invalid-second-step" {
				payload = `{"operations":[{"operation":"meta","patch":{"title":"middle"}},{"operation":"question-delete","guid":"nonexistent"}]}`
			}
			action := "form-edit"
			if mode == "draft" {
				action = "form-draft"
			}
			out := t.TempDir()
			a := newApp([]string{"phase3", "https://phase3.test/forms/abcdefgh", "--action", action, "--payload", payload, "--out", out, "--json", "--relogin"})
			a.Origin = "https://phase3.test"
			stdout, err := issueCapture(t, a.Run)
			files, _ := filepath.Glob(filepath.Join(out, "*"))
			if len(files) != 1 {
				t.Fatal("expected one evidence dir", files, err)
			}
			dir := files[0]
			if mode == "success" || mode == "draft" {
				if err != nil {
					t.Fatal(err)
				}
				var result map[string]any
				if json.Unmarshal([]byte(stdout), &result) != nil || result["ok"] != true {
					t.Fatal(stdout)
				}
				if mode == "draft" {
					if writes != 0 || result["mutationAttempted"] != false {
						t.Fatal("draft read mutated")
					}
					for _, p := range []string{"draft.json", "draft.png"} {
						if _, e := os.Stat(filepath.Join(dir, p)); e != nil {
							t.Fatal(e)
						}
					}
					return
				}
				if writes != 2 || opens != 2 {
					t.Fatalf("writes=%d opens=%d", writes, opens)
				}
				var progress map[string]any
				raw, e := os.ReadFile(filepath.Join(dir, "progress.json"))
				if e != nil {
					t.Fatal(e)
				}
				_ = json.Unmarshal(raw, &progress)
				if progress["freshProfileVerified"] != true || len(progress["completedSteps"].([]any)) != 2 {
					t.Fatal(progress)
				}
				for _, p := range []string{"step-01/step-result.json", "step-02/step-result.json", "result.json", "reload.png"} {
					if _, e := os.Stat(filepath.Join(dir, p)); e != nil {
						t.Fatal(e)
					}
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "禁止重放整个批次") {
				t.Fatal("failed batch not disclosed", err)
			}
			var failure map[string]any
			raw, e := os.ReadFile(filepath.Join(dir, "failure.json"))
			if e != nil {
				t.Fatal(e)
			}
			_ = json.Unmarshal(raw, &failure)
			wantWrites := 2
			wantCompleted := 2
			wantStage := "fresh-profile"
			if mode == "invalid-second-step" || mode == "read-only" {
				wantWrites = 0
				wantCompleted = 0
				wantStage = "preflight"
			}
			if mode == "second-step-error" {
				wantCompleted = 1
				wantStage = "operation"
			}
			if writes != wantWrites || len(failure["completedSteps"].([]any)) != wantCompleted || failure["stage"] != wantStage || failure["mayHaveApplied"] != (wantWrites > 0) {
				t.Fatalf("writes=%d failure=%s", writes, raw)
			}
			if _, e := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(e) {
				t.Fatal("failure wrote success artifact")
			}
		})
	}
}
