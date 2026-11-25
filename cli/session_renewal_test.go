package main

import (
	"net/http"
	"testing"
)

func TestResponseRenewalIsBoundToRequestSession(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cookie       string
		headers      map[string]string
		change       string
		wantSID      string
		wantAccount  string
		wantState    string
		wantRevision int64
	}{
		{name: "normal renewal", wantSID: "renewed", wantAccount: "A", wantState: sessionSignedIn, wantRevision: 2},
		{name: "account switch in flight", change: "switch", wantSID: "account-b", wantAccount: "B", wantState: sessionSignedIn, wantRevision: 2},
		{name: "logout in flight", change: "logout", wantState: sessionSignedOut, wantRevision: 2},
		{name: "explicit unrelated cookie", cookie: "sid=another-account", wantSID: "account-a", wantAccount: "A", wantState: sessionSignedIn, wantRevision: 1},
		{name: "header overrides cookie", headers: map[string]string{"Cookie": "sid=another-account"}, wantSID: "account-a", wantAccount: "A", wantState: sessionSignedIn, wantRevision: 1},
		{name: "authorization overrides account", headers: map[string]string{"Authorization": "Bearer another-account"}, wantSID: "account-a", wantAccount: "A", wantState: sessionSignedIn, wantRevision: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessionTestState(t)
			origin := "https://shimo.net"
			if err := mirrorSession(origin, "sid=account-a", "login", map[string]any{"id": "A"}, 0); err != nil {
				t.Fatal(err)
			}
			previousClient := httpClient
			t.Cleanup(func() { httpClient = previousClient })
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("Cookie") == "" {
					t.Fatal("test request has no credential")
				}
				var err error
				switch tc.change {
				case "switch":
					err = mirrorSession(origin, "sid=account-b", "import", map[string]any{"id": "B"}, 1)
				case "logout":
					err = markSessionSignedOut(origin, "logout")
				}
				if err != nil {
					t.Fatal(err)
				}
				response := phase3Response(http.StatusOK, `{}`)
				response.Header.Set("Set-Cookie", "sid=renewed; Path=/; Secure; HttpOnly")
				return response, nil
			})}
			app := &App{Origin: origin}
			if _, err := app.request("/users/me", http.MethodGet, nil, nil, tc.headers, tc.cookie); err != nil {
				t.Fatal(err)
			}
			record, err := readSessionRecord(origin)
			if err != nil || record == nil || record.Revision != tc.wantRevision || record.AuthState != tc.wantState || record.Account.ID != tc.wantAccount || cookieValue(record.Cookies, "sid") != tc.wantSID {
				t.Fatalf("response changed the wrong session: got %#v", record)
			}
		})
	}
}
