package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReleaseFormSettingsPreservesAccessAndVerifiesDeadline(t *testing.T) {
	for _, mode := range []string{"success", "access-changed", "timezone-changed", "deadline-unchanged", "stale-publication", "mutation-403"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			old, oldDefault, oldOpen := httpClient, http.DefaultTransport, phase3OpenFormPublishEditor
			defer func() { httpClient = old; http.DefaultTransport = oldDefault; phase3OpenFormPublishEditor = oldOpen }()
			delta := releaseFormDelta(t, "Synthetic title")
			form, _ := phase3DecodeFormDelta(delta)
			publication := map[string]any{"status": "normal", "submitterCondition": "enterprise_member", "timezone": float64(0), "deadline": "2020-01-01T00:00:00Z"}
			deadline := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
			writes, reads, opens := 0, 0, 0
			phase3OpenFormPublishEditor = func(target, ready string) (*phase3EditorBrowser, error) {
				opens++
				return globalNegativeFormEditor(t, "control", form, delta), nil
			}
			transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				reply := func(v any) (*http.Response, error) {
					raw, _ := json.Marshal(v)
					return phase3Response(200, string(raw)), nil
				}
				switch r.URL.Path {
				case "/api/v1/files/abcdefgh":
					return phase3Response(200, `{"type":"form"}`), nil
				case "/forms/abcdefgh":
					return phase3Response(200, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/","SDK_V2_SIGNATURE":"test","SDK_V2_TOKEN":"test"}}];`), nil
				case "/sdk/forms/abcdefgh/content":
					return phase3Response(200, `{"contentUrl":"https://signed.test/draft"}`), nil
				case "/draft":
					return phase3Response(200, "synthetic delta"), nil
				case "/sdk/forms/abcdefgh/publications":
					if r.Method == "GET" {
						reads++
						if mode == "stale-publication" && reads == 2 {
							changed := phase3TableCopy(publication)
							changed["deadline"] = "2030-01-01T00:00:00Z"
							return reply(changed)
						}
						return reply(publication)
					}
					if r.Method != "PATCH" {
						t.Fatalf("settings used %s", r.Method)
					}
					writes++
					if writes > 1 {
						t.Fatal("settings replayed")
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if len(body) != 3 || body["deadline"] != deadline || body["timezone"] != float64(0) || body["userId"] != float64(42) {
						t.Fatal("incorrect bounded settings request", body)
					}
					if mode == "mutation-403" {
						return phase3Response(403, `{}`), nil
					}
					if mode != "deadline-unchanged" {
						publication["deadline"] = body["deadline"]
					}
					publication["timezone"] = body["timezone"]
					if mode == "access-changed" {
						publication["submitterCondition"] = "anyone"
					}
					if mode == "timezone-changed" {
						publication["timezone"] = float64(8)
					}
					return phase3Response(204, ""), nil
				}
				t.Errorf("unexpected request %s", r.URL.Path)
				return phase3Response(500, `{}`), nil
			})
			httpClient = &http.Client{Transport: transport}
			http.DefaultTransport = transport
			payload, _ := json.Marshal(map[string]any{"deadline": deadline, "timezone": "UTC"})
			out := t.TempDir()
			a := newApp([]string{"phase3", "https://phase3.test/forms/abcdefgh", "--action", "form-settings", "--payload", string(payload), "--out", out, "--relogin", "--json"})
			a.Origin = "https://phase3.test"
			stdout, err := issueCapture(t, a.Run)
			wantWrites := 1
			if mode == "stale-publication" {
				wantWrites = 0
			}
			if writes != wantWrites {
				t.Fatal(writes, err)
			}
			if mode == "success" {
				if err != nil || opens != 2 {
					t.Fatal(err, opens)
				}
				var result map[string]any
				_ = json.Unmarshal([]byte(stdout), &result)
				if result["actionVerified"] != true || result["action"] != "form-settings" {
					t.Fatal(result)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "禁止自动重放") {
				t.Fatal(err)
			}
			files, _ := filepath.Glob(filepath.Join(out, "*", "failure.json"))
			if len(files) != 1 {
				t.Fatal("missing failure evidence")
			}
			raw, _ := os.ReadFile(files[0])
			var failure map[string]any
			_ = json.Unmarshal(raw, &failure)
			stage := "readback"
			if mode == "stale-publication" {
				stage = "preflight"
			}
			if mode == "mutation-403" {
				stage = "mutation"
			}
			if failure["stage"] != stage || failure["mayHaveApplied"] != (writes > 0) {
				t.Fatal(string(raw))
			}
			if _, e := os.Stat(filepath.Join(filepath.Dir(files[0]), "result.json")); !os.IsNotExist(e) {
				t.Fatal("settings failure marked successful")
			}
		})
	}
}
