package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// A local net.Pipe peer exercises the real CDP framing/Eval/WaitReady paths.
// Its replies and screenshot are synthetic; no Chrome or remote page is used.
func globalNegativeFormEditor(t *testing.T, fault string, form map[string]any, delta any) *phase3EditorBrowser {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.Set(x, y, color.RGBA{uint8(x * y), uint8(x*x + y), uint8(y*y + x), 255})
		}
	}
	if fault == "ready-white" {
		img = image.NewRGBA(image.Rect(0, 0, 1440, 1400))
		for i := range img.Pix {
			img.Pix[i] = 255
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
			var request map[string]any
			if json.Unmarshal(raw, &request) != nil {
				return
			}
			method := str(request, "method")
			result := map[string]any{}
			switch method {
			case "Browser.close":
			case "Page.captureScreenshot":
				result["data"] = base64.StdEncoding.EncodeToString(encoded.Bytes())
			case "Runtime.evaluate":
				params, _ := request["params"].(map[string]any)
				expr := str(params, "expression")
				var value any
				switch {
				case strings.Contains(expr, "pageElapsedMs"):
					state := "complete"
					if fault == "unready" {
						state = "loading"
					}
					value = map[string]any{"ready": false, "login": fault == "login", "documentState": state, "sdk": "undefined", "canvasCount": 0, "path": "/forms/abcdefgh"}
				case strings.Contains(expr, "sdkHasUnsavedUnreliable"):
					value = map[string]any{"guid": "abcdefgh", "form": form, "canEdit": fault != "permission", "canPublish": fault != "permission"}
				case strings.HasPrefix(expr, "new window.modoc.Delta("):
					value = delta
				case expr == "window.logged.edit.query.permissions.DEBUG_getContext().user.id":
					value = float64(42)
				case expr == "window.logged.edit.query.getFillFormLink()":
					value = "https://phase3.test/forms/abcdefgh/fill"
				case strings.Contains(expr, "const presence="):
					value = map[string]any{"ok": true}
				default:
					t.Errorf("unexpected fake-CDP expression: %.100s", expr)
					return
				}
				result["result"] = map[string]any{"value": value}
			default:
				t.Errorf("unexpected fake-CDP method %s", method)
				return
			}
			if err = peer.sendJSON(map[string]any{"id": request["id"], "result": result}); err != nil {
				return
			}
		}
	}()
	return &phase3EditorBrowser{ws: &WSConn{conn: client, rd: bufio.NewReader(client)}}
}

func TestPhase3GlobalPublishNegativePipeline(t *testing.T) {
	for _, fault := range []string{"control", "no-op", "readback-401", "readback-403", "mutation-401", "fresh-blank", "permission", "login", "blank", "unready", "ready-white", "fresh-white"} {
		t.Run(fault, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=test")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
			t.Setenv("SHIMO_NET_KEYCHAIN_SERVICE", "shimo-net-cli-synthetic-publish-test")
			old, oldDefault, oldOpen := httpClient, http.DefaultTransport, phase3OpenFormPublishEditor
			defer func() { httpClient = old; http.DefaultTransport = oldDefault; phase3OpenFormPublishEditor = oldOpen }()
			var delta any
			if err := json.Unmarshal([]byte(`[{"action":"insert","attributes":{"title":"OFFLINE FORM FIXTURE","desc":"synthetic only","descAlign":"left"},"data":{"form":[[{"action":"insert","data":{"object":{"guid":"q001","required":true,"subject":"Synthetic question","question":{"type":"text"}}}}],null]}}]`), &delta); err != nil {
				t.Fatal(err)
			}
			form, err := phase3DecodeFormDelta(delta)
			if err != nil {
				t.Fatal(err)
			}
			initial := map[string]any{"status": "unpublished", "submitterCondition": "enterprise_member", "isQuiz": float64(0)}
			publication := phase3TableCopy(initial)
			writes, opens := 0, 0
			trace := []map[string]any{}
			phase3OpenFormPublishEditor = func(target, readiness string) (*phase3EditorBrowser, error) {
				opens++
				mode := fault
				if fault == "fresh-blank" && opens == 1 {
					mode = "control"
				}
				if fault == "fresh-white" {
					mode = "control"
					if opens > 1 {
						mode = "ready-white"
					}
				}
				b := globalNegativeFormEditor(t, mode, form, delta)
				if mode == "login" || mode == "blank" || mode == "unready" || mode == "fresh-blank" {
					err := b.WaitReady(readiness, 5*time.Millisecond)
					if err != nil {
						b.Close()
						return nil, err
					}
				}
				return b, nil
			}
			// Exercise the publication pipeline through the synthetic CDP peer.
			// Real browser creation is replaced above; no-browser itself is tested
			// separately as a command boundary that must reject before this point.
			t.Setenv("SHIMO_NET_NO_BROWSER", "")
			transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				trace = append(trace, map[string]any{"method": r.Method, "path": r.URL.Path, "writesObservedBeforeRequest": writes})
				respond := func(v any) (*http.Response, error) {
					b, _ := json.Marshal(v)
					return phase3Response(200, string(b)), nil
				}
				switch r.URL.Path {
				case "/api/v1/files/abcdefgh":
					return phase3Response(200, `{"type":"form"}`), nil
				case "/forms/abcdefgh":
					return phase3Response(200, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/","SDK_V2_SIGNATURE":"test","SDK_V2_TOKEN":"test"}}];`), nil
				case "/sdk/forms/abcdefgh/content":
					return phase3Response(200, `{"contentUrl":"https://phase3.test/offline-form-content"}`), nil
				case "/offline-form-content":
					return phase3Response(200, `offline-compressed-delta-placeholder-decoded-by-fake-CDP`), nil
				case "/sdk/forms/abcdefgh/publications":
					if r.Method == "POST" {
						writes++
						if writes != 1 {
							t.Fatal("publication replayed")
						}
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Fatal(err)
						}
						if fault != "no-op" {
							publication = body
						}
						if fault == "mutation-401" {
							return phase3Response(401, `{}`), nil
						}
						return phase3Response(204, ""), nil
					}
					if r.Method != "GET" {
						t.Fatalf("unexpected mutation %s", r.Method)
					}
					if writes > 0 && fault == "readback-401" {
						return phase3Response(401, `{}`), nil
					}
					if writes > 0 && fault == "readback-403" {
						return phase3Response(403, `{}`), nil
					}
					return respond(publication)
				default:
					t.Fatalf("unexpected mock HTTP %s %s", r.Method, r.URL.Path)
					return nil, nil
				}
			})
			httpClient = &http.Client{Transport: transport}
			http.DefaultTransport = transport
			out := t.TempDir()
			args := []string{"phase3", "https://phase3.test/forms/abcdefgh", "--action", "form-publish", "--out", out, "--relogin", "--json"}
			a := newApp(args)
			a.Origin = "https://phase3.test"
			err = a.Run()
			preflight := fault == "permission" || fault == "login" || fault == "blank" || fault == "unready" || fault == "ready-white"
			wantWrites := 1
			if preflight {
				wantWrites = 0
			}
			if writes != wantWrites {
				t.Fatalf("writes=%d want%d err=%v", writes, wantWrites, err)
			}
			if preflight && !reflect.DeepEqual(publication, initial) {
				t.Fatal("rejected preflight changed publication")
			}
			dirs, _ := os.ReadDir(out)
			if len(dirs) != 1 {
				t.Fatal("missing evidence")
			}
			dir := filepath.Join(out, dirs[0].Name())
			observation := map[string]any{"action": "form-publish", "fault": fault, "argv": args, "mutationRequests": writes, "editorOpenCalls": opens, "httpTrace": trace, "appRunReturnedError": err != nil, "syntheticScreenshot": "Any before/reload.png is generated test data, never real browser evidence"}
			if fault == "control" {
				if err != nil {
					t.Fatal(err)
				}
				archivePhase3Negative(t, dir, observation)
				return
			}
			if err == nil || !strings.Contains(err.Error(), "禁止自动重放") {
				t.Fatalf("failure not disclosed: %v", err)
			}
			if _, e := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(e) {
				t.Fatal("failure produced success result")
			}
			b, e := os.ReadFile(filepath.Join(dir, "failure.json"))
			if e != nil {
				t.Fatal(e)
			}
			var failure map[string]any
			if e = json.Unmarshal(b, &failure); e != nil {
				t.Fatal(e)
			}
			stage := "readback"
			if preflight {
				stage = "preflight"
			}
			if fault == "mutation-401" {
				stage = "mutation"
			}
			if fault == "fresh-blank" || fault == "fresh-white" {
				stage = "fresh-profile"
			}
			if failure["stage"] != stage || failure["mayHaveApplied"] != !preflight || failure["mutationAttempted"] != !preflight || failure["ok"] != false {
				t.Fatalf("wrong failure metadata: %s", b)
			}
			observation["originalReturnedError"] = err.Error()
			archivePhase3Negative(t, dir, observation)
		})
	}
}
