package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func writeTestCredential(t *testing.T, origin, cookie string) {
	t.Helper()
	raw, err := json.Marshal(Cred{Origin: origin, Cookie: cookie, Storage: "file"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credPath(), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func isolateCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_COOKIE", "")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	t.Setenv("SHIMO_NET_ORIGIN", "https://shimo.net")
}

func TestStoredCredentialCannotCrossOrigin(t *testing.T) {
	isolateCredentials(t)
	writeTestCredential(t, "https://shimo.net", "sid=synthetic-secret")
	for _, target := range []string{"https://unrelated.invalid", "http://shimo.net", "https://shimo.net:444"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("SHIMO_NET_ORIGIN", target)
			got, err := readCookie()
			if err == nil || got != "" {
				t.Fatalf("stored credential escaped binding: error=%v nonempty=%v", err, got != "")
			}
		})
	}
}

func TestUnboundCredentialRequiresExplicitLogin(t *testing.T) {
	isolateCredentials(t)
	writeTestCredential(t, "", "sid=synthetic-secret")
	got, err := readCookie()
	if err == nil || got != "" {
		t.Fatal("origin-less legacy credential must not be reused")
	}
}

func TestRequestRejectsCookieForForeignOrigin(t *testing.T) {
	isolateCredentials(t)
	old := httpClient
	defer func() { httpClient = old }()
	calls := 0
	httpClient = &http.Client{Transport: phase3AuthTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	a := &App{Origin: "https://shimo.net"}
	for _, target := range []string{"https://unrelated.invalid/file", "http://shimo.net/file", "https://shimo.net:444/file"} {
		if _, err := a.requestURL(target, "GET", nil, nil, nil, "sid=synthetic-secret"); err == nil {
			t.Errorf("accepted foreign URL %s", target)
		}
	}
	if calls != 0 {
		t.Fatalf("made %d credential-bearing foreign requests", calls)
	}
}

func TestExpiredSessionNeverAutomaticallyHarvestsOrReplays(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic-expired")
	old := httpClient
	defer func() { httpClient = old }()
	harvestMu.Lock()
	oldDone, oldCookie, oldErr := harvestDone, harvestCookie, harvestErr
	harvestMu.Unlock()
	defer func() {
		harvestMu.Lock()
		harvestDone, harvestCookie, harvestErr = oldDone, oldCookie, oldErr
		harvestMu.Unlock()
	}()
	for _, pathAPI := range []bool{false, true} {
		for _, method := range []string{"GET", "POST", "DELETE"} {
			harvestMu.Lock()
			harvestDone = false
			harvestMu.Unlock()
			calls := 0
			httpClient = &http.Client{Transport: phase3AuthTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})}
			a := &App{Origin: "https://shimo.net"}
			request := a.authRequest
			if pathAPI {
				request = a.authRequestPath
			}
			_, err := request("/synthetic", method, nil, nil, nil)
			harvestMu.Lock()
			attempted := harvestDone
			harvestMu.Unlock()
			if err == nil || calls != 1 || attempted {
				t.Errorf("method=%s pathAPI=%v calls=%d harvest=%v err=%v", method, pathAPI, calls, attempted, err)
			}
		}
	}
}

func TestLoginCookiesUseSameSafeDomainRulesAsHarvest(t *testing.T) {
	cases := []struct {
		name    string
		cookies []any
		want    string
	}{
		{"exact", []any{map[string]any{"domain": ".shimo.net", "name": "sid", "value": "s", "session": true}}, "sid=s"},
		{"suffix attack", []any{map[string]any{"domain": "evilshimo.net", "name": "sid", "value": "s", "session": true}}, ""},
		{"subdomain", []any{map[string]any{"domain": "docs.shimo.net", "name": "sid", "value": "s", "session": true}}, ""},
		{"missing sid", []any{map[string]any{"domain": "shimo.net", "name": "locale", "value": "zh", "session": true}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(cookiesForOrigin(map[string]any{"cookies": tc.cookies}, "https://shimo.net"), "; ")
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestCredentialServiceNormalizesDefaultPortsAndSeparatesAuthorities(t *testing.T) {
	baseline, err := credentialServiceFor("https://SHIMO.net")
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := credentialServiceFor("https://shimo.net:443/docs")
	if err != nil || equivalent != baseline {
		t.Fatal("equivalent origin got a different keychain service")
	}
	for _, other := range []string{"http://shimo.net", "https://shimo.net:444", "https://other.invalid"} {
		got, err := credentialServiceFor(other)
		if err != nil || got == baseline {
			t.Fatalf("origin collision %s", other)
		}
	}
	for _, bad := range []string{"", "shimo.net", "https://user:password@shimo.net", "file:///tmp/x", "https://shimo.net:99999"} {
		if _, err := credentialServiceFor(bad); err == nil {
			t.Errorf("accepted invalid origin %s", bad)
		}
	}
}

func TestAuthRequestUsesAppOriginAndDoesNotHarvestMismatchedCredentials(t *testing.T) {
	isolateCredentials(t)
	writeTestCredential(t, "https://shimo.net", "sid=synthetic")
	resetHarvestCache()
	defer resetHarvestCache()
	old := httpClient
	defer func() { httpClient = old }()
	calls := 0
	httpClient = &http.Client{Transport: phase3AuthTransport(func(r *http.Request) (*http.Response, error) { calls++; return nil, nil })}
	a := &App{Origin: "https://other.invalid"}
	if _, err := a.authRequest("/users/me", "GET", nil, nil, nil); err == nil {
		t.Fatal("mismatched stored credentials accepted")
	}
	harvestMu.Lock()
	done := harvestDone
	harvestMu.Unlock()
	if calls != 0 || done {
		t.Fatalf("origin mismatch requested network or browser: calls=%d harvest=%v", calls, done)
	}
}

func TestRequestNeverFollowsAuthenticatedRedirect(t *testing.T) {
	isolateCredentials(t)
	old := httpClient
	defer func() { httpClient = old }()
	calls := 0
	httpClient = &http.Client{Transport: phase3AuthTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://other.invalid/stolen"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	a := &App{Origin: "https://shimo.net"}
	result, err := a.requestURL("https://shimo.net/start", "GET", nil, nil, nil, "sid=synthetic")
	if err != nil || result.Status != 302 || calls != 1 {
		t.Fatalf("followed redirect: calls=%d err=%v", calls, err)
	}
}

func TestCookieHeadersCannotBypassDestinationCheck(t *testing.T) {
	isolateCredentials(t)
	old := httpClient
	defer func() { httpClient = old }()
	calls := 0
	httpClient = &http.Client{Transport: phase3AuthTransport(func(r *http.Request) (*http.Response, error) { calls++; return nil, nil })}
	a := &App{Origin: "https://shimo.net"}
	for _, header := range []string{"Cookie", "Authorization"} {
		_, err := a.requestURL("https://other.invalid/", "GET", nil, nil, map[string]string{header: "synthetic-secret"}, "")
		if err == nil {
			t.Errorf("accepted foreign %s", header)
		}
	}
	if calls != 0 {
		t.Fatalf("made %d outbound calls", calls)
	}
}

func TestFreshLoginProfileCannotReuseManagedCookies(t *testing.T) {
	isolateCredentials(t)
	if err := os.MkdirAll(profilePath(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath()+"/Cookies", []byte("old-account"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := newLoginProfile()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newLoginProfile()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == profilePath() {
		t.Fatal("login reused a persistent profile")
	}
	if _, err := os.Stat(first + "/Cookies"); !os.IsNotExist(err) {
		t.Fatal("fresh login inherited managed cookies")
	}
}

func TestCredentialStorageLabelDescribesFallback(t *testing.T) {
	if got := credentialStorageLabel("file"); !strings.Contains(got, "0600") || strings.Contains(got, "Keychain") {
		t.Fatal(got)
	}
}

func TestLoginCannotClaimAccountChangeUnderEnvironmentOverride(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("SHIMO_NET_COOKIE", "sid=override-account")
	for _, args := range [][]string{{"login"}, {"login", "--new"}} {
		a := newApp(args)
		if err := a.cmdLogin(); err == nil || !strings.Contains(err.Error(), "SHIMO_NET_COOKIE") {
			t.Fatalf("login under override was not rejected: %v", err)
		}
	}
	if _, err := os.Stat(configPath()); !os.IsNotExist(err) {
		t.Fatal("rejected login modified configuration")
	}
}

func TestSecureBrowserCookiesNeverDowngradeToHTTP(t *testing.T) {
	res := map[string]any{"cookies": []any{map[string]any{"name": "sid", "value": "synthetic", "domain": "shimo.net", "secure": true, "session": true}}}
	if got := cookiesForOrigin(res, "http://shimo.net"); len(got) != 0 {
		t.Fatal("Secure cookie sent to HTTP")
	}
	if got := cookiesForOrigin(res, "https://shimo.net"); len(got) != 1 || got[0] != "sid=synthetic" {
		t.Fatal("HTTPS cookie missing")
	}
}

func TestSandboxDisableRequiresAffirmativeOptIn(t *testing.T) {
	for _, value := range []string{"", "0", "false", "no"} {
		t.Setenv("SHIMO_NET_NO_SANDBOX", value)
		if strings.Contains(strings.Join(chromeLaunchArgs("about:blank"), " "), "--no-sandbox") {
			t.Errorf("sandbox disabled for %q", value)
		}
	}
}

func TestSessionHarvestCannotClaimSavedAccountUnderEnvOverride(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("SHIMO_NET_COOKIE", "sid=override-account")
	resetHarvestCache()
	defer resetHarvestCache()
	a := newApp([]string{"session", "harvest"})
	if err := a.cmdSession(); err == nil || !strings.Contains(err.Error(), "SHIMO_NET_COOKIE") {
		t.Fatalf("harvest under override not rejected: %v", err)
	}
	harvestMu.Lock()
	done := harvestDone
	harvestMu.Unlock()
	if done {
		t.Fatal("rejected harvest started browser discovery")
	}
}

func TestPageTargetSelectionPreservesExactBlankBootstrap(t *testing.T) {
	for _, tc := range []struct {
		target, origin string
		want           bool
	}{
		{"about:blank", "about:blank", true},
		{"https://evil.invalid/about:blank", "about:blank", false},
		{"about:blank#other", "about:blank", false},
		{"https://shimo.net/docs/id", "https://shimo.net", true},
		{"https://evil.invalid/?next=https://shimo.net", "https://shimo.net", false},
		{"http://shimo.net/docs/id", "https://shimo.net", false},
		{"https://shimo.net:444/docs/id", "https://shimo.net", false},
	} {
		if got := pageTargetMatchesOrigin(tc.target, tc.origin); got != tc.want {
			t.Errorf("target=%s origin=%s got=%v want=%v", tc.target, tc.origin, got, tc.want)
		}
	}
}
