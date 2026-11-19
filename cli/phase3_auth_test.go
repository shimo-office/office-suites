package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type phase3AuthTransport func(*http.Request) (*http.Response, error)

func (f phase3AuthTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPhase3Mutation401DoesNotHarvestOrReplay(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	oldClient := httpClient
	harvestMu.Lock()
	oldDone, oldCookie, oldErr := harvestDone, harvestCookie, harvestErr
	harvestMu.Unlock()
	t.Cleanup(func() {
		httpClient = oldClient
		harvestMu.Lock()
		harvestDone, harvestCookie, harvestErr = oldDone, oldCookie, oldErr
		harvestMu.Unlock()
	})
	for _, pathAPI := range []bool{false, true} {
		for _, method := range []string{"POST", "PATCH", "PUT", "DELETE"} {
			t.Run(method+map[bool]string{false: "/api", true: "/path"}[pathAPI], func(t *testing.T) {
				harvestMu.Lock()
				harvestDone = false
				harvestMu.Unlock()
				calls := 0
				httpClient = &http.Client{Transport: phase3AuthTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				})}
				a := &App{Cmd: "phase3", Origin: "https://example.invalid"}
				request := a.authRequest
				if pathAPI {
					request = a.authRequestPath
				}
				_, err := request("/mutation", method, map[string]any{"value": 1}, nil, nil)
				harvestMu.Lock()
				attemptedHarvest := harvestDone
				harvestMu.Unlock()
				if err == nil || !strings.Contains(err.Error(), "禁止自动重放") || calls != 1 || attemptedHarvest {
					t.Fatalf("401 must terminate mutation: calls=%d harvest=%v err=%v", calls, attemptedHarvest, err)
				}
			})
		}
	}
}
