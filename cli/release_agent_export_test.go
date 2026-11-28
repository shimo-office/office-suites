package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseSessionExportProtectsSecretsAndExistingFiles(t *testing.T) {
	sessionTestState(t)
	const origin = "https://shimo.net"
	const fixtureValue = "synthetic-export-fixture"
	if err := mirrorSession(origin, "sid="+fixtureValue, "test", nil, sessionRevision(origin)); err != nil {
		t.Fatal(err)
	}
	for _, jsonOutput := range []bool{true, false} {
		t.Run(map[bool]string{true: "json", false: "text"}[jsonOutput], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			a := newApp([]string{"session", "export", "--out", path})
			a.Origin, a.JSON = origin, jsonOutput
			output, err := issueCapture(t, a.cmdSessionExport)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output, fixtureValue) {
				t.Fatal("credential leaked to stdout")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err = json.Unmarshal(raw, &payload); err != nil || payload["origin"] != origin || payload["cookie"] != "sid="+fixtureValue {
				t.Fatal("invalid adapter payload", err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
				t.Fatalf("insecure export mode: %v", info.Mode())
			}
			if _, err = issueCapture(t, a.cmdSessionExport); err == nil {
				t.Fatal("existing export was overwritten")
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(raw) {
				t.Fatal("rejected export changed existing file")
			}
		})
	}
	a := newApp([]string{"session", "export", "--json"})
	a.Origin = origin
	output, err := issueCapture(t, a.cmdSessionExport)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err = json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(str(result, "path"), filepath.Join(stateDir(), "session-export")) || strings.Contains(output, fixtureValue) {
		t.Fatal("default export is not private")
	}
}

func TestReleaseSessionExportRejectsUnavailableCredentials(t *testing.T) {
	for _, kind := range []string{"empty", "wrong-path", "corrupt", "missing-parent"} {
		t.Run(kind, func(t *testing.T) {
			sessionTestState(t)
			const origin = "https://shimo.net"
			a := newApp([]string{"session", "export", "--out", filepath.Join(t.TempDir(), "missing", "session.json"), "--json"})
			a.Origin = origin
			switch kind {
			case "wrong-path":
				_, err := commitSession(origin, sessionRecord{Origin: origin, AuthState: sessionSignedIn, Cookies: []sessionCookie{{Name: "sid", Value: "synthetic", Domain: "shimo.net", Path: "/unrelated"}}}, -1, true)
				if err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				path, _ := sessionStorePath(origin)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-parent":
				if err := mirrorSession(origin, "sid=synthetic", "test", nil, sessionRevision(origin)); err != nil {
					t.Fatal(err)
				}
			}
			output, err := issueCapture(t, a.cmdSessionExport)
			if err == nil || output != "" {
				t.Fatalf("unavailable export claimed success: %v %s", err, output)
			}
			if kind == "empty" {
				var detail *commandError
				if !errors.As(err, &detail) || detail.Result["code"] != "SESSION_EMPTY" {
					t.Fatal("missing structured empty-session error", err)
				}
			}
		})
	}
}

func TestReleasePrivateJSONRemovesInvalidPayloadAndPreservesCollision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.json")
	if err := writePrivateJSON(path, map[string]any{"unsupported": make(chan int)}); err == nil {
		t.Fatal("unserializable payload accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed serialization left an empty credential file")
	}
	if err := writePrivateJSON(path, map[string]any{"safe": "value"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := writePrivateJSON(path, map[string]any{"replacement": true}); err == nil {
		t.Fatal("exclusive reservation was bypassed")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("existing export changed")
	}
}

func TestReleasePreviewReportsLinkWithoutLaunchingBrowser(t *testing.T) {
	sessionTestState(t)
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		return phase3Response(200, `{"guid":"abcdefgh","name":"test","type":"document"}`), nil
	})
	for _, jsonOutput := range []bool{true, false} {
		a := newApp([]string{"preview", "abcdefgh"})
		a.JSON = jsonOutput
		stdout, err := issueCapture(t, a.Run)
		if err != nil {
			t.Fatal(err)
		}
		if jsonOutput {
			var result map[string]any
			if err = json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatal(err)
			}
			view := asObj(result["view"])
			if view["browserOpened"] != false || view["transport"] != viewTransportLink || str(result, "url") != "https://shimo.net/files/abcdefgh" {
				t.Fatal("preview claimed a browser session", result)
			}
		} else if !strings.Contains(stdout, "https://shimo.net/files/abcdefgh") {
			t.Fatal("missing preview link")
		}
	}
}
