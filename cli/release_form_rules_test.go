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

func releaseFormRuleDelta(t *testing.T, added bool) any {
	t.Helper()
	question := `[{"action":"insert","data":{"object":{"guid":"questionA","required":true,"subject":"Choice","question":{"type":"single_choice","options":[{"type":"normal","value":"optA","label":"A"}]}}}},{"action":"insert","data":{"object":{"guid":"questionB","required":false,"subject":"Followup","question":{"type":"text"}}}}]`
	rule := func(id string) string {
		return `{"action":"insert","data":{"object":{"guid":"` + id + `","rule":{"type":"conditional","conditions":[{"type":"select_option","question":"questionA","options":[{"type":"normal","value":"optA"}]}],"effects":[{"type":"show_question","questions":["questionB"]},{"type":"show_widget","widgets":[]}]}}}}`
	}
	rules := rule("existingRule")
	if added {
		rules += "," + rule("newRule001")
	}
	var delta any
	if err := json.Unmarshal([]byte(`[{"action":"insert","attributes":{"title":"Rules fixture"},"data":{"form":[`+question+`,[`+rules+`]]}}]`), &delta); err != nil {
		t.Fatal(err)
	}
	return delta
}

func TestReleaseFormNewRuleMustHaveUniqueServerID(t *testing.T) {
	for _, mode := range []string{"success", "duplicate-id", "invalid-id"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			old, oldDefault := httpClient, http.DefaultTransport
			defer func() { httpClient = old; http.DefaultTransport = oldDefault }()
			beforeDelta, afterDelta := releaseFormRuleDelta(t, false), releaseFormRuleDelta(t, true)
			before, _ := phase3DecodeFormDelta(beforeDelta)
			after, _ := phase3DecodeFormDelta(afterDelta)
			writes := 0
			b := releaseFormPeer(t, func(expr string) (any, error) {
				switch {
				case strings.Contains(expr, "sdkHasUnsavedUnreliable"):
					form := before
					if writes > 0 {
						form = after
					}
					return map[string]any{"guid": "abcdefgh", "form": form, "canEdit": true}, nil
				case strings.HasPrefix(expr, "new window.modoc.Delta("):
					if writes > 0 {
						return afterDelta, nil
					}
					return beforeDelta, nil
				case strings.Contains(expr, "switch(r.operation)"):
					writes++
					id := "newRule001"
					if mode == "duplicate-id" {
						id = "existingRule"
					}
					if mode == "invalid-id" {
						id = "!"
					}
					return map[string]any{"ok": true, "createdRuleGuid": id}, nil
				case strings.Contains(expr, "const presence="):
					return map[string]any{"ok": true}, nil
				default:
					return nil, errors.New("unexpected synthetic request")
				}
			})
			defer b.Close()
			transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/sdk/forms/abcdefgh/content" {
					return phase3Response(200, `{"contentUrl":"https://signed.test/draft"}`), nil
				}
				if r.URL.Host == "signed.test" {
					return phase3Response(200, "synthetic delta"), nil
				}
				return nil, errors.New("unexpected transport")
			})
			httpClient = &http.Client{Transport: transport}
			http.DefaultTransport = transport
			a := newApp(nil)
			a.Origin = "https://phase3.test"
			s := &phase3SDKSession{app: a, guid: "abcdefgh", prefix: "/sdk/", target: "https://phase3.test/forms/abcdefgh"}
			out := t.TempDir()
			result, attempted, err := phase3FormRunOne(b, s, out, map[string]any{"operation": "rule-add", "trigger": "questionA", "options": []any{"optA"}, "targets": []any{"questionB"}})
			if writes != 1 || !attempted {
				t.Fatal("rule submission absent or replayed", writes, err)
			}
			if mode == "success" {
				if err != nil || phase3VerifyForm(after, result) != nil {
					t.Fatal(result, err)
				}
				raw, _ := os.ReadFile(filepath.Join(out, "expected.json"))
				if strings.Contains(string(raw), phase3NewRuleID) || !strings.Contains(string(raw), "newRule001") {
					t.Fatal("temporary ID escaped into persistence expectation", string(raw))
				}
				return
			}
			if err == nil {
				t.Fatal("invalid server-generated ID accepted")
			}
			raw, e := os.ReadFile(filepath.Join(out, "step-failure.json"))
			if e != nil {
				t.Fatal(e)
			}
			var failure map[string]any
			_ = json.Unmarshal(raw, &failure)
			if failure["stage"] != "mutation" || failure["mayHaveApplied"] != true {
				t.Fatal(failure)
			}
		})
	}
}
