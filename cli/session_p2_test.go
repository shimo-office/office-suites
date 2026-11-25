package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrokenSharedSessionNeverFallsBack(t *testing.T) {
	for _, content := range []string{"{", "null", `{}`, `{"authState":"unknown"}`, "directory"} {
		t.Run(content, func(t *testing.T) {
			sessionTestState(t)
			origin := "https://shimo.net"
			writeTestCredential(t, origin, "sid=old-account")
			path, err := sessionStorePath(origin)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if content == "directory" {
				err = os.Mkdir(path, 0700)
			} else {
				err = os.WriteFile(path, []byte(content), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			cookie, _, err := credentialForOrigin(origin)
			if err == nil || cookie != "" {
				t.Errorf("broken shared session fell back: cookie=%q err=%v", cookie, err)
			}
			if _, err := commitSession(origin, sessionRecord{AuthState: sessionSignedIn, Cookies: parseCookieHeader("sid=replacement")}, 0, true); err == nil {
				t.Error("corrupt existing record was silently overwritten")
			}
		})
	}
}

func TestMissingSharedSessionStillUsesLocalCredential(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	writeTestCredential(t, origin, "sid=local-account")
	cookie, source, err := credentialForOrigin(origin)
	if err != nil || source != "file" || cookie != "sid=local-account" {
		t.Fatalf("missing shared record should allow local credentials: %q %q %v", cookie, source, err)
	}
}

func TestResponseCookieDefaultsToRequestDirectory(t *testing.T) {
	for _, tc := range []struct{ path, attr, want string }{
		{"/api/v1/users/me", "", "/api/v1/users"},
		{"/api/v1/users/", "", "/api/v1/users"},
		{"/api/a%2Fb/users/me", "", "/api/a%2Fb/users"},
		{"/api/v1/users/me", "; Path=relative", "/api/v1/users"},
		{"/api/v1/users/me", "; Path=/explicit", "/explicit"},
		{"/single", "", "/"},
		{"/", "", "/"},
		{"", "", "/"},
	} {
		t.Run(tc.path+tc.attr, func(t *testing.T) {
			sessionTestState(t)
			origin := "https://shimo.net"
			if err := mirrorSession(origin, "sid=account; scoped=root", "login", nil, 0); err != nil {
				t.Fatal(err)
			}
			previous := httpClient
			t.Cleanup(func() { httpClient = previous })
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(req *http.Request) (*http.Response, error) {
				r := phase3Response(http.StatusOK, `{}`)
				r.Header.Set("Set-Cookie", "scoped=narrow"+tc.attr)
				return r, nil
			})}
			a := &App{Origin: origin}
			if _, err := a.requestURL(origin+tc.path, http.MethodGet, nil, nil, nil, ""); err != nil {
				t.Fatal(err)
			}
			rec, _ := readSessionRecord(origin)
			found := false
			for _, c := range rec.Cookies {
				if c.Name == "scoped" && c.Value == "narrow" {
					found = true
					if c.Path != tc.want {
						t.Errorf("Path=%q, want %q", c.Path, tc.want)
					}
				}
			}
			if !found {
				t.Fatal("renewed cookie missing")
			}
			if tc.want != "/" && strings.Contains(sessionCookieHeaderFor(rec, parseURLOrNil(origin+"/elsewhere")), "scoped=narrow") {
				t.Error("scoped cookie leaked outside response directory")
			}
		})
	}
}

func TestResponseCookieDeletionUsesDefaultDirectory(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	if _, err := commitSession(origin, sessionRecord{AuthState: sessionSignedIn, Cookies: []sessionCookie{
		{Name: "sid", Value: "account", Path: "/"},
		{Name: "scoped", Value: "root", Path: "/"},
		{Name: "scoped", Value: "narrow", Path: "/api/v1/users"},
	}}, 0, true); err != nil {
		t.Fatal(err)
	}
	previous := httpClient
	t.Cleanup(func() { httpClient = previous })
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		r := phase3Response(http.StatusOK, `{}`)
		r.Header.Set("Set-Cookie", "scoped=; Max-Age=0")
		return r, nil
	})}
	if _, err := (&App{Origin: origin}).requestURL(origin+"/api/v1/users/me", http.MethodGet, nil, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	rec, err := readSessionRecord(origin)
	if err != nil || rec == nil {
		t.Fatalf("read renewed record: %v", err)
	}
	if len(rec.Cookies) != 2 || cookieValue(rec.Cookies, "scoped") != "root" {
		t.Fatalf("deletion must retain root cookie and remove directory cookie: %#v", rec.Cookies)
	}
}
