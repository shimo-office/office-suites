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

func TestReleaseFormSubmitExactlyOnceAndRejectsCorruptReadback(t *testing.T) {
	for _, mode := range []string{"success", "stopped", "notification", "permission", "stale-publication", "submit-401", "missing-submit-id", "duplicate-submit-id", "readback-401", "old-row-changed", "wrong-answer", "ui-success", "ui-click-error", "ui-confirmation-login", "ui-stale-publication", "ui-foreign-link", "ui-wrong-form", "ui-presence"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			old, oldOpen, oldFill := httpClient, phase3OpenFormEditor, phase3OpenFormFillTab
			defer func() { httpClient = old; phase3OpenFormEditor = oldOpen; phase3OpenFormFillTab = oldFill }()
			form := map[string]any{"title": "Synthetic form", "questions": []any{map[string]any{"guid": "questionA", "subject": "Synthetic question", "required": true, "type": "text", "question": map[string]any{}}}, "rules": []any{}}
			formRaw, _ := json.Marshal(form)
			publication := map[string]any{"status": "normal", "publishedRev": 1, "publishedContent": string(formRaw)}
			if mode == "stopped" {
				publication["status"] = "stopped"
			}
			if mode == "notification" {
				publication["subscribeNotification"] = "enabled"
			}
			writes, pubReads, presenceChecks, fills, closes := 0, 0, 0, 0, 0
			uiMode := strings.HasPrefix(mode, "ui-")
			phase3OpenFormFillTab = func(editor *phase3EditorBrowser, target string) (*phase3EditorBrowser, func(), error) {
				if target != "https://phase3.test/forms/abcdefgh/fill?debug=1" {
					t.Errorf("incorrect fill target %s", target)
				}
				fill := releaseFormPeer(t, func(expr string) (any, error) {
					switch {
					case strings.Contains(expr, "f.command.updateResponse"):
						fills++
						if mode == "ui-wrong-form" {
							return map[string]any{"ok": false, "error": "wrong form identity"}, nil
						}
						return map[string]any{"ok": true}, nil
					case strings.Contains(expr, "buttons[0].click()"):
						writes++
						if writes > 1 {
							return nil, errors.New("UI submit replayed")
						}
						if mode == "ui-click-error" {
							return nil, errors.New("click acknowledgement lost")
						}
						return map[string]any{"ok": true}, nil
					case strings.Contains(expr, "pageElapsedMs"):
						return map[string]any{"ready": mode != "ui-confirmation-login", "login": mode == "ui-confirmation-login"}, nil
					case strings.Contains(expr, "({pageStatus:"):
						return map[string]any{"pageStatus": float64(2), "text": "Synthetic confirmation"}, nil
					default:
						t.Errorf("unexpected fill expression %.120s", expr)
						return nil, errors.New("unexpected fill expression")
					}
				})
				return fill, func() { closes++; fill.Close() }, nil
			}
			phase3OpenFormEditor = func(target, ready string) (*phase3EditorBrowser, error) {
				return releaseFormPeer(t, func(expr string) (any, error) {
					if strings.Contains(expr, "sdkHasUnsavedUnreliable") {
						return map[string]any{"guid": "abcdefgh", "form": form, "canPublish": mode != "permission"}, nil
					}
					if expr == "window.logged.edit.query.getFillFormLink()" {
						if mode == "ui-foreign-link" {
							return "https://evil.test/fill", nil
						}
						return "https://phase3.test/forms/abcdefgh/fill", nil
					}
					if strings.Contains(expr, "const presence=") {
						presenceChecks++
						if mode == "ui-presence" && presenceChecks == 3 {
							return map[string]any{"ok": false, "error": "other user online"}, nil
						}
						return map[string]any{"ok": true}, nil
					}
					t.Errorf("unexpected expression %.80s", expr)
					return nil, nil
				}), nil
			}
			prior := map[string]any{"submitId": "existing", "content": []any{map[string]any{"guid": "questionA", "type": 0, "text": map[string]any{"content": "untouched"}}}}
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				reply := func(v any) (*http.Response, error) {
					raw, _ := json.Marshal(v)
					return phase3Response(200, string(raw)), nil
				}
				switch r.URL.Path {
				case "/api/v1/files/abcdefgh":
					return phase3Response(200, `{"type":"form"}`), nil
				case "/forms/abcdefgh":
					return phase3Response(200, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/","SDK_V2_SIGNATURE":"test","SDK_V2_TOKEN":"test"}}];`), nil
				case "/api/v1/users/me":
					return phase3Response(200, `{"id":42,"name":"Synthetic"}`), nil
				case "/sdk/forms/abcdefgh/publications":
					pubReads++
					if (pubReads > 1 && mode == "stale-publication") || (pubReads == 3 && mode == "ui-stale-publication") {
						m := phase3TableCopy(publication)
						m["publishedRev"] = 2
						return reply(m)
					}
					return reply(publication)
				case "/sdk/forms/abcdefgh/submit":
					if uiMode {
						t.Fatal("UI submit called REST endpoint directly")
					}
					if r.Method != "POST" {
						t.Fatal("incorrect submit method")
					}
					writes++
					if writes != 1 {
						t.Fatal("submit replayed")
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body["formRev"] != float64(1) || body["userFinger"] != "42" {
						t.Fatal("bad submission identity", body)
					}
					content := body["responseContent"].([]any)
					if len(content) != 1 || content[0].(map[string]any)["text"].(map[string]any)["content"] != "新答案" {
						t.Fatal(body)
					}
					if mode == "submit-401" {
						return phase3Response(401, `{}`), nil
					}
					if mode == "missing-submit-id" {
						return reply(map[string]any{})
					}
					if mode == "duplicate-submit-id" {
						return reply(map[string]any{"submitId": "existing"})
					}
					return reply(map[string]any{"submitId": "new-response"})
				case "/sdk/forms/abcdefgh/responses":
					if writes > 0 && mode == "readback-401" {
						return phase3Response(401, `{}`), nil
					}
					rows := []any{prior}
					if writes > 0 {
						text := "新答案"
						if mode == "wrong-answer" {
							text = "corrupted"
						}
						rows = append(rows, map[string]any{"submitId": "new-response", "content": []any{map[string]any{"guid": "questionA", "type": 0, "text": map[string]any{"content": text}}}})
						if mode == "old-row-changed" {
							rows[0] = map[string]any{"submitId": "existing", "content": []any{}}
						}
					}
					return reply(map[string]any{"total": len(rows), "responses": rows})
				}
				t.Errorf("unexpected HTTP %s %s", r.Method, r.URL.Path)
				return phase3Response(500, `{}`), nil
			})}
			out := t.TempDir()
			a := newApp([]string{"phase3", "https://phase3.test/forms/abcdefgh", "--action", "form-submit", "--payload", `{"answers":{"questionA":"新答案"}}`, "--out", out, "--relogin", "--json"})
			a.Origin = "https://phase3.test"
			if uiMode {
				a = newApp(append([]string{"phase3", "https://phase3.test/forms/abcdefgh", "--action", "form-submit", "--payload", `{"answers":{"questionA":"新答案"}}`, "--out", out, "--relogin", "--json"}, "--confirm-text", "Synthetic confirmation"))
				a.Origin = "https://phase3.test"
			}
			stdout, err := issueCapture(t, a.Run)
			preflight := mode == "stopped" || mode == "notification" || mode == "permission" || mode == "stale-publication" || mode == "ui-stale-publication" || mode == "ui-foreign-link" || mode == "ui-wrong-form" || mode == "ui-presence"
			want := 1
			if preflight {
				want = 0
			}
			if writes != want {
				t.Fatalf("writes=%d want%d error=%v", writes, want, err)
			}
			if mode == "success" || mode == "ui-success" {
				if err != nil {
					t.Fatal(err)
				}
				var result map[string]any
				_ = json.Unmarshal([]byte(stdout), &result)
				if result["submitId"] != "new-response" || result["total"] != float64(2) || result["actionVerified"] != true {
					t.Fatal(result)
				}
				if mode == "ui-success" {
					if result["confirmationVerified"] != true || fills != 1 || closes != 1 {
						t.Fatal("UI confirmation not verified or tab not cleaned up", result, fills, closes)
					}
				} else if result["confirmationVerified"] != nil {
					t.Fatal("REST response falsely claims UI confirmation")
				}
				return
			}
			if err == nil || strings.Contains(stdout, `"ok": true`) {
				t.Fatal("failure claimed success", stdout)
			}
			if !preflight && !strings.Contains(err.Error(), "禁止自动重放") {
				t.Fatal(err)
			}
			if mode == "stopped" || mode == "notification" {
				return
			}
			paths, _ := filepath.Glob(filepath.Join(out, "*", "failure.json"))
			if len(paths) != 1 {
				t.Fatal("missing failure evidence", paths, err)
			}
			raw, _ := os.ReadFile(paths[0])
			var f map[string]any
			_ = json.Unmarshal(raw, &f)
			if f["mayHaveApplied"] != !preflight || f["mutationAttempted"] != !preflight {
				t.Fatal(string(raw))
			}
			if _, e := os.Stat(filepath.Join(filepath.Dir(paths[0]), "result.json")); !os.IsNotExist(e) {
				t.Fatal("success artifact on failed submit")
			}
		})
	}
}

func TestReleaseFormAnswerTypesHaveMatchingRESTAndUIRepresentation(t *testing.T) {
	var form map[string]any
	_ = json.Unmarshal([]byte(`{"questions":[{"guid":"text1","subject":"Text","required":true,"type":"text","question":{}},{"guid":"multiText","subject":"Long text","required":true,"type":"multiple_text","question":{}},{"guid":"single1","subject":"Single","required":true,"type":"single_choice","question":{"options":[{"type":"normal","value":"optA","label":"A"}]}},{"guid":"dropdown1","subject":"Dropdown","required":true,"type":"dropdown_list","question":{"options":[{"type":"normal","value":"optA","label":"A"}]}},{"guid":"multiple1","subject":"Multiple","required":true,"type":"multiple_choice","question":{"options":[{"type":"normal","value":"optA","label":"A"},{"type":"normal","value":"optB","label":"B"}]}},{"guid":"rating1","subject":"Rating","required":true,"type":"rate","question":{"shape":"star","sum":5}},{"guid":"optional1","subject":"Optional","required":false,"type":"text","question":{}}],"rules":[]}`), &form)
	answers := map[string]any{"text1": "value", "multiText": "line1\nline2", "single1": "optA", "dropdown1": "optA", "multiple1": []any{"optB", "optA"}, "rating1": float64(5)}
	rest, err := phase3FormSubmissionAnswers(form, answers)
	if err != nil {
		t.Fatal(err)
	}
	ui, err := phase3FormFillResponses(form, answers)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 6 || len(ui) != 6 {
		t.Fatal("unanswered optional question was submitted", rest, ui)
	}
	expected := []int{0, 4, 1, 3, 2, 5}
	for i, typ := range expected {
		if rest[i].(map[string]any)["type"] != typ {
			t.Fatal(rest[i])
		}
		if rest[i].(map[string]any)["guid"] != ui[i].(map[string]any)["guid"] {
			t.Fatal("REST/UI question order differs")
		}
	}
	if ui[1].(map[string]any)["response"].(map[string]any)["content"] != "line1\nline2" {
		t.Fatal("multiline text lost")
	}
	choice := ui[3].(map[string]any)["response"].(map[string]any)["choice"].(map[string]any)
	if choice["value"] != "optA" || choice["type"] != "normal" {
		t.Fatal(choice)
	}
	choices := ui[4].(map[string]any)["response"].(map[string]any)["choices"].([]any)
	if len(choices) != 2 || choices[0].(map[string]any)["value"] != "optB" {
		t.Fatal("multiselect order or value lost", choices)
	}
	for _, bad := range []struct {
		id    string
		value any
	}{
		{"text1", float64(1)}, {"single1", []any{"optA"}}, {"dropdown1", "unknown"}, {"multiple1", "optA"}, {"multiple1", []any{}}, {"multiple1", []any{"optA", "optA"}}, {"multiple1", []any{false}}, {"rating1", float64(0)}, {"rating1", "5"},
	} {
		copy := phase3TableCopy(answers)
		copy[bad.id] = bad.value
		if _, err := phase3FormSubmissionAnswers(form, copy); err == nil {
			t.Fatal("invalid REST answer accepted", bad)
		}
		if _, err := phase3FormFillResponses(form, copy); err == nil {
			t.Fatal("invalid UI answer accepted", bad)
		}
	}
}
