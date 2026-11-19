package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestEditorUnverifiedAPIOutputDoesNotClaimCompletionOrScreenshot(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	t.Setenv("SHIMO_NET_ORIGIN", "https://shimo.net")
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	previousClient := httpClient
	defer func() { httpClient = previousClient }()
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return phase3Response(200, `{}`), nil
	})}
	for _, command := range []string{"editor", "phase2"} {
		for _, jsonMode := range []bool{false, true} {
			t.Run(command+"-json="+strconv.FormatBool(jsonMode), func(t *testing.T) {
				out, err := os.CreateTemp(t.TempDir(), "output-")
				if err != nil {
					t.Fatal(err)
				}
				original := os.Stdout
				os.Stdout = out
				defer func() { os.Stdout = original; _ = out.Close() }()
				args := []string{command, "https://shimo.net/sheets/abcdefgh", "--action", "sheet-update", "--payload", `{"range":"Sheet1!A1","resource":{"values":[["value"]]}}`, "--no-verify", "--screenshot", "not-created.png"}
				if jsonMode {
					args = append(args, "--json")
				}
				if err := newApp(args).Run(); err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(out.Name())
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), "not-created.png") || strings.Contains(string(raw), "扩展动作已完成") || strings.Contains(string(raw), "WorkBuddy agent-browser") {
					t.Fatalf("misleading output: %s", raw)
				}
				if jsonMode {
					var result map[string]any
					if err := json.Unmarshal(raw, &result); err != nil {
						t.Fatal(err)
					}
					if result["verified"] != false || result["screenshot"] != nil {
						t.Fatalf("unverified response: %s", raw)
					}
				} else if !strings.Contains(string(raw), "待页面核验") {
					t.Fatalf("missing verification boundary: %s", raw)
				}
			})
		}
	}
}
