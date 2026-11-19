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

func TestPhase3SlidesStartupFailureHasEvidence(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	previous := httpClient
	defer func() { httpClient = previous }()
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/files/abcdefgh" {
			t.Fatalf("unexpected API call %s %s", r.Method, r.URL.Path)
		}
		return phase3Response(200, `{"type":"presentation","name":"startup fixture"}`), nil
	})}
	out := t.TempDir()
	a := newApp([]string{"phase3", "https://phase3.test/presentation/abcdefgh", "--action", "slide-placeholder-fill", "--payload", `{"slideId":"s1","shapeName":"Title","text":"test"}`, "--out", out, "--json"})
	a.Origin = "https://phase3.test"
	sentinel := errors.New("initial editor unavailable")
	opens := 0
	var dir string
	err := a.cmdPhase3SlidesWithOpen(func(string) (*phase3EditorBrowser, error) {
		opens++
		dirs, e := os.ReadDir(out)
		if e != nil || len(dirs) != 1 {
			t.Fatalf("startup evidence must precede Chrome: %v %v", dirs, e)
		}
		dir = filepath.Join(out, dirs[0].Name())
		for _, name := range []string{"request.json", "command-log.json"} {
			if _, e = os.Stat(filepath.Join(dir, name)); e != nil {
				t.Fatal(e)
			}
		}
		return nil, sentinel
	})
	if !errors.Is(err, sentinel) || opens != 1 || !strings.Contains(err.Error(), dir) {
		t.Fatalf("startup outcome %v, opens=%d", err, opens)
	}
	read := func(name string) map[string]any {
		raw, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if e = json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	command := read("command-log.json")
	failure := read("failure.json")
	if command["exitCode"] != float64(1) || command["targetGuid"] != "abcdefgh" || len(str(command, "binarySHA256")) != 64 || str(command, "finishedUTC") == "" {
		t.Fatalf("incomplete startup command %v", command)
	}
	if failure["mutationAttempted"] != false || failure["doNotReplay"] != false || !strings.Contains(str(failure, "error"), sentinel.Error()) {
		t.Fatalf("incorrect startup failure %v", failure)
	}
	if _, e := os.Stat(filepath.Join(dir, "submission.json")); !os.IsNotExist(e) {
		t.Fatal("startup failure must not record submission")
	}
}
