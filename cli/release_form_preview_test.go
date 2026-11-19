package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseFormPreviewCommandRequiresFinalBranchObservation(t *testing.T) {
	for _, mode := range []string{"preview", "respondent", "foreign-link", "no-link", "not-published", "final-selection-mismatch", "invalid-branch"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			old, oldDefault, oldOpen := httpClient, http.DefaultTransport, phase3OpenFormEditor
			defer func() { httpClient = old; http.DefaultTransport = oldDefault; phase3OpenFormEditor = oldOpen }()
			var delta any
			_ = json.Unmarshal([]byte(`[{"action":"insert","attributes":{"title":"Synthetic branching form","desc":"fixture"},"data":{"form":[[{"action":"insert","data":{"object":{"guid":"questionA","required":true,"subject":"Trigger","question":{"type":"single_choice","options":[{"type":"normal","value":"optA","label":"Yes"},{"type":"normal","value":"optB","label":"No"}]}}}},{"action":"insert","data":{"object":{"guid":"questionB","required":false,"subject":"Followup","question":{"type":"text"}}}}],null]}}]`), &delta)
			form, err := phase3DecodeFormDelta(delta)
			if err != nil {
				t.Fatal(err)
			}
			formRaw, _ := json.Marshal(form)
			clicks, observed := 0, 0
			phase3OpenFormEditor = func(target, ready string) (*phase3EditorBrowser, error) {
				return releaseFormPeer(t, func(expr string) (any, error) {
					switch {
					case strings.HasPrefix(expr, "new window.modoc.Delta("):
						return delta, nil
					case expr == "window.logged.edit.query.getPreviewFormLink()" || expr == "window.logged.edit.query.getFillFormLink()":
						if mode == "no-link" {
							return nil, nil
						}
						if mode == "foreign-link" {
							return "https://untrusted.test/preview?t=fixture", nil
						}
						return "https://phase3.test/forms/abcdefgh/preview?t=fixture", nil
					case strings.Contains(expr, "pageElapsedMs"):
						return map[string]any{"ready": true}, nil
					case expr == "({title:document.title,body:document.body.innerText})":
						return map[string]any{"title": "Synthetic branching form", "body": "synthetic only"}, nil
					case strings.Contains(expr, "实际填写页与发布标题"):
						return map[string]any{"guid": "abcdefgh", "title": "Synthetic branching form", "desc": "fixture"}, nil
					case strings.Contains(expr, "input.scrollIntoView"):
						clicks++
						return map[string]any{"ok": true}, nil
					case strings.Contains(expr, "const checks="):
						observed++
						return map[string]any{"verified": mode != "final-selection-mismatch", "selectionVerified": mode != "final-selection-mismatch", "option": map[int]string{1: "optA", 2: "optB"}[observed]}, nil
					case strings.Contains(expr, "loggedKeys:"):
						return map[string]any{"path": "/forms/abcdefgh/preview", "readyState": "complete"}, nil
					default:
						t.Errorf("unexpected expression %.120s", expr)
						return nil, errors.New("unexpected synthetic request")
					}
				}), nil
			}
			transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					t.Errorf("preview submitted a mutation: %s", r.Method)
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
					status := "normal"
					if mode == "not-published" {
						status = "stopped"
					}
					raw, _ := json.Marshal(map[string]any{"status": status, "publishedContent": string(formRaw)})
					return phase3Response(200, string(raw)), nil
				}
				t.Errorf("unexpected path %s", r.URL.Path)
				return phase3Response(500, `{}`), nil
			})
			httpClient = &http.Client{Transport: transport}
			http.DefaultTransport = transport
			action := "form-preview"
			if mode == "respondent" || mode == "not-published" {
				action = "form-respondent"
			}
			payload := `{"trigger":"questionA","branches":[{"option":"optA","visible":["questionA","questionB"],"hidden":[],"sequence":["questionA","questionB"]},{"option":"optB","visible":["questionA"],"hidden":["questionB"],"sequence":["questionA"]}]}`
			if mode == "invalid-branch" {
				payload = `{"trigger":"questionA","branches":[{"option":"missing","visible":[],"hidden":[]}]}`
			}
			out := t.TempDir()
			a := newApp([]string{"phase3", "https://phase3.test/forms/abcdefgh", "--action", action, "--payload", payload, "--out", out, "--json"})
			a.Origin = "https://phase3.test"
			stdout, err := issueCapture(t, a.Run)
			success := mode == "preview" || mode == "respondent"
			if (err == nil) != success {
				t.Fatal(mode, err, stdout)
			}
			paths, _ := filepath.Glob(filepath.Join(out, "*"))
			if len(paths) != 1 {
				t.Fatal(paths)
			}
			dir := paths[0]
			if success {
				var result map[string]any
				_ = json.Unmarshal([]byte(stdout), &result)
				if clicks != 2 || observed != 2 || result["responseSubmitted"] != false || result["mutationAttempted"] != false {
					t.Fatal(clicks, observed, result)
				}
				for _, p := range []string{"result.json", "preview.png", "branch-01.json", "branch-02.png", "navigation.json"} {
					if _, e := os.Stat(filepath.Join(dir, p)); e != nil {
						t.Fatal(e)
					}
				}
				raw, _ := os.ReadFile(filepath.Join(dir, "navigation.json"))
				if strings.Contains(string(raw), "fixture") {
					t.Fatal("signed token leaked in navigation evidence")
				}
				return
			}
			var failure map[string]any
			raw, e := os.ReadFile(filepath.Join(dir, "failure.json"))
			if e != nil {
				t.Fatal(e)
			}
			_ = json.Unmarshal(raw, &failure)
			if failure["mutationAttempted"] != false || failure["responseSubmitted"] != false {
				t.Fatal(failure)
			}
			if _, e := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(e) {
				t.Fatal("invalid preview produced success")
			}
			if mode == "final-selection-mismatch" {
				if clicks != 1 || observed != 1 {
					t.Fatal("branch mutation replayed", clicks, observed)
				}
				if _, e := os.Stat(filepath.Join(dir, "branch-01-failure-state.json")); e != nil {
					t.Fatal(e)
				}
			} else if clicks != 0 {
				t.Fatal("preflight error clicked branch")
			}
		})
	}
}
