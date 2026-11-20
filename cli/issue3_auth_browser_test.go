package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIssue3LoginOriginProbeDoesNotSaveOnFailure(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("SHIMO_NET_KEYCHAIN_SERVICE", "shimo-net-cli-issue3-test")
	t.Setenv("SHIMO_NET_ORIGIN", "")
	if err := saveConfig("https://previous.invalid"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatal(err)
	}
	old := httpClient
	t.Cleanup(func() { httpClient = old })
	for _, status := range []int{404, 500} {
		calls := 0
		httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.String() != "https://bad.invalid/api/v1/auth/register/is_new" || r.Header.Get("Cookie") != "" {
				t.Fatalf("unexpected probe: %s", r.URL)
			}
			return phase3Response(status, `{}`), nil
		})}
		_, err := issueCapture(t, newApp([]string{"login", "--origin", "https://bad.invalid", "--new"}).cmdLogin)
		if err == nil || !strings.Contains(err.Error(), "HTTP") {
			t.Fatalf("probe error not returned: %v", err)
		}
		after, err := os.ReadFile(configPath())
		if err != nil || string(after) != string(before) || calls != 1 {
			t.Fatalf("failed login altered config or skipped probe: calls=%d error=%v", calls, err)
		}
	}
}

func TestIssue3BrowserCandidatesAndLabels(t *testing.T) {
	t.Setenv("ProgramFiles", t.TempDir())
	t.Setenv("ProgramFiles(x86)", "")
	t.Setenv("LOCALAPPDATA", "")
	for _, system := range []string{"darwin", "linux", "windows"} {
		for _, kind := range []string{"chrome", "chromium", "edge"} {
			paths := browserCandidates(system, kind)
			if len(paths) == 0 {
				t.Fatalf("missing %s %s", system, kind)
			}
			for _, path := range paths {
				if got := browserLabelForBinary(path); got != browserLabel(kind) {
					t.Errorf("%s label=%s want %s", path, got, browserLabel(kind))
				}
			}
		}
	}
	if got := browserCandidates("windows", "unknown"); len(got) != 0 {
		t.Fatalf("unknown browser resolves directory: %v", got)
	}
	if got := browserLabelForBinary("/unknown/launcher"); got != "Chromium-compatible" {
		t.Fatalf("unknown binary mislabelled: %s", got)
	}
}

func TestIssue3WindowsDefaultBrowserRegistration(t *testing.T) {
	for _, tc := range []struct{ progID, command, binary, label string }{
		{"ChromeHTML.123", `"C:\Program Files\Google\Chrome\Application\chrome.exe" --single-argument "%1"`, `C:\Program Files\Google\Chrome\Application\chrome.exe`, "Chrome"},
		{"MSEdgeHTM", `"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" --single-argument "%1"`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, "Microsoft Edge"},
		{"ChromiumHTM", `C:\Chromium\chrome.exe "%1"`, `C:\Chromium\chrome.exe`, "Chromium"},
		{"FirefoxURL", `"C:\Firefox\firefox.exe" "%1"`, "", ""},
		{"ChromeHTML", `cmd.exe /c "C:\Chrome\chrome.exe" "%1"`, "", ""},
		{"ChromeHTML", `"C:\Other\evil.exe" "%1"`, "", ""},
		{"ChromeHTML", `"chrome.exe" "%1"`, "", ""},
		{"ChromeHTML", `C:\Program Files\Chrome\chrome.exe "%1"`, "", ""},
	} {
		binary, label := windowsBrowserRegistration(tc.progID, tc.command)
		if binary != tc.binary || label != tc.label {
			t.Errorf("%s: got %q %q want %q %q", tc.progID, binary, label, tc.binary, tc.label)
		}
	}
	if got := registryStringValue("\r\nHKEY_CURRENT_USER\\test\r\n    ProgId    REG_SZ    ChromeHTML.123\r\n", "ProgId"); got != "ChromeHTML.123" {
		t.Fatal(got)
	}
	if got := registryStringValue("    (Default)    REG_SZ    \"C:\\Program Files\\Chrome\\chrome.exe\" --single-argument \"%1\"", ""); got != `"C:\Program Files\Chrome\chrome.exe" --single-argument "%1"` {
		t.Fatal(got)
	}
}

func TestIssue3ParentDomainLoginDiagnostic(t *testing.T) {
	future := float64(time.Now().Add(time.Hour).Unix())
	makeCookie := func(domain string) map[string]any {
		return map[string]any{"name": "sid", "value": "must-not-leak", "domain": domain, "path": "/", "expires": future, "secure": true}
	}
	for _, tc := range []struct {
		name    string
		cookies []any
		wantErr bool
		want    []string
	}{
		{"parent", []any{makeCookie(".example.test")}, true, nil},
		{"exact", []any{makeCookie("docs.example.test")}, false, []string{"sid=must-not-leak"}},
		{"exact-wins", []any{makeCookie(".example.test"), makeCookie("docs.example.test")}, false, []string{"sid=must-not-leak"}},
		{"unrelated", []any{makeCookie(".other.test")}, false, nil},
		{"suffix-attack", []any{makeCookie(".ample.test")}, false, nil},
		{"empty", nil, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loginCookiesForOrigin(map[string]any{"cookies": tc.cookies}, "https://docs.example.test")
			if (err != nil) != tc.wantErr || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got cookies=%d error=%v", len(got), err)
			}
			if err != nil && (!strings.Contains(err.Error(), "父域") || strings.Contains(err.Error(), "must-not-leak")) {
				t.Fatalf("unhelpful or leaking error: %v", err)
			}
		})
	}
	expired := makeCookie(".example.test")
	expired["expires"] = float64(time.Now().Add(-time.Hour).Unix())
	if _, err := loginCookiesForOrigin(map[string]any{"cookies": []any{expired}}, "https://docs.example.test"); err != nil {
		t.Fatal("expired parent cookie should not abort login", err)
	}
	if _, err := loginCookiesForOrigin(map[string]any{"cookies": []any{makeCookie(".example.test")}}, "http://docs.example.test"); err != nil {
		t.Fatal("secure cookie should not diagnose HTTP login", err)
	}
}

func TestIssue3LoginProbeNetworkFailureNoNewConfig(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("SHIMO_NET_KEYCHAIN_SERVICE", "shimo-net-cli-issue3-test")
	t.Setenv("SHIMO_NET_ORIGIN", "")
	old := httpClient
	t.Cleanup(func() { httpClient = old })
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("network unavailable") })}
	err := newApp([]string{"login", "--origin", "https://bad.invalid"}).cmdLogin()
	if err == nil || !strings.Contains(err.Error(), "network unavailable") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stateDir(), "config.json")); !os.IsNotExist(err) {
		t.Fatalf("config created: %v", err)
	}
}

func TestIssue3NoBrowserBlocksExplicitAuthenticationBeforeEffects(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("SHIMO_NET_KEYCHAIN_SERVICE", "shimo-net-cli-issue3-test")
	t.Setenv("SHIMO_NET_ORIGIN", "")
	// Keep the RED regression safe too: a missed guard cannot open a browser,
	// because login receives a failed origin probe and harvest is disabled.
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	if err := saveConfig("https://previous.invalid"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatal(err)
	}
	oldClient := httpClient
	t.Cleanup(func() { httpClient = oldClient })
	calls := 0
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return phase3Response(404, `{}`), nil
	})}
	harvestMu.Lock()
	oldDone, oldCookie, oldErr, oldOrigin := harvestDone, harvestCookie, harvestErr, harvestOrigin
	harvestDone, harvestCookie, harvestErr, harvestOrigin = true, "", errors.New("test sentinel"), "https://previous.invalid"
	harvestMu.Unlock()
	t.Cleanup(func() {
		harvestMu.Lock()
		defer harvestMu.Unlock()
		harvestDone, harvestCookie, harvestErr, harvestOrigin = oldDone, oldCookie, oldErr, oldOrigin
	})
	for _, mode := range []string{"flag", "env"} {
		for _, command := range [][]string{{"login", "--origin", "https://bad.invalid", "--new"}, {"session", "harvest"}, {"session", "harvest", "--no-save"}} {
			t.Run(mode+strings.Join(command, " "), func(t *testing.T) {
				t.Setenv("SHIMO_NET_NO_BROWSER", "")
				args := append([]string{}, command...)
				if mode == "flag" {
					args = append(args, "--no-browser")
				} else {
					t.Setenv("SHIMO_NET_NO_BROWSER", "true")
				}
				err := newApp(args).Run()
				if err == nil || !strings.Contains(err.Error(), "禁止启动浏览器") {
					t.Errorf("missing browser guard: %v", err)
				}
				if calls != 0 {
					t.Errorf("blocked command made %d network calls", calls)
				}
				after, err := os.ReadFile(configPath())
				if err != nil || string(before) != string(after) {
					t.Error("blocked command altered config")
				}
				if _, err := os.Stat(credPath()); !os.IsNotExist(err) {
					t.Errorf("blocked command created credential file: %v", err)
				}
				harvestMu.Lock()
				unchanged := harvestDone && harvestOrigin == "https://previous.invalid" && harvestErr != nil && harvestErr.Error() == "test sentinel"
				harvestMu.Unlock()
				if !unchanged {
					t.Error("blocked command reset harvest cache")
				}
			})
		}
	}
}

func TestIssue3NoBrowserEnvironmentBlocksDirectHarvest(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("SHIMO_NET_KEYCHAIN_SERVICE", "shimo-net-cli-issue3-test")
	t.Setenv("SHIMO_NET_NO_BROWSER", "yes")
	// This second guard prevents any real browser discovery even before the fix.
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	for _, fn := range []func() (string, error){
		func() (string, error) { return authCookieForOrigin("https://shimo.net") },
		func() (string, error) { return harvestChromeSessionForOrigin("https://shimo.net", false) },
		func() (string, error) { return harvestChromeSessionForOrigin("https://shimo.net", true) },
		func() (string, error) { c, _, err := harvestFromBrowsers("https://shimo.net"); return c, err },
	} {
		cookie, err := fn()
		if cookie != "" || err == nil || !strings.Contains(err.Error(), "禁止启动浏览器") {
			t.Errorf("direct browser path not blocked: cookie=%t error=%v", cookie != "", err)
		}
	}
	// Existing explicitly provided credentials must still support API-only work.
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
	if got, err := authCookieForOrigin("https://shimo.net"); err != nil || got != "sid=synthetic" {
		t.Fatalf("API-only credential path blocked: %v", err)
	}
}
