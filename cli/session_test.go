package main

import (
	"strings"
	"testing"
	"time"
)

func TestCookieJarForHostFiltersAndSorts(t *testing.T) {
	now := float64(time.Now().Unix())
	cookies := []browserCookie{
		{Name: "sid", Value: "s1", Domain: "shimo.net", Path: "/", Session: true},
		{Name: "deviceId", Value: "d1", Domain: "shimo.net", Path: "/", Session: true},
		{Name: "old", Value: "x", Domain: "shimo.net", Path: "/", Expires: now - 10},
		{Name: "docs", Value: "x", Domain: "docs.shimo.net", Path: "/", Session: true},
		{Name: "other", Value: "x", Domain: "example.com", Path: "/", Session: true},
		{Name: "locale", Value: "zh-CN", Domain: "shimo.net", Path: "/", Session: true},
	}
	got := cookieJarForHost(cookies, "shimo.net")
	if !strings.Contains(got, "sid=s1") || !strings.Contains(got, "deviceId=d1") || !strings.Contains(got, "locale=zh-CN") {
		t.Fatalf("missing expected cookie: %s", got)
	}
	if strings.Contains(got, "old=") || strings.Contains(got, "docs=") || strings.Contains(got, "other=") {
		t.Fatalf("leaked unexpected cookie: %s", got)
	}
	if got != "deviceId=d1; locale=zh-CN; sid=s1" {
		t.Fatalf("unexpected order: %s", got)
	}
}

func TestCookieJarRequiresSid(t *testing.T) {
	if got := cookieJarForHost([]browserCookie{{Name: "locale", Value: "zh-CN", Domain: "shimo.net"}}, "shimo.net"); got != "" {
		t.Fatalf("expected empty jar without sid, got %q", got)
	}
}

func TestRepairMojibake(t *testing.T) {
	cases := map[string]string{
		"ä¼ä¸šé«˜çº§ç‰ˆ": "企业高级版",
		"café":            "café",
		"plain ascii":     "plain ascii",
		"徐依娴":             "徐依娴",
	}
	for in, want := range cases {
		if got := repairMojibake(in); got != want {
			t.Fatalf("repairMojibake(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChromeLaunchArgsNoSandbox(t *testing.T) {
	t.Setenv("SHIMO_NET_NO_SANDBOX", "1")
	args := chromeLaunchArgs("--no-first-run")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--no-sandbox") || !strings.Contains(joined, "--no-first-run") {
		t.Fatalf("unexpected args: %s", joined)
	}
}

func TestSessionOverrideStateDir(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", "/tmp/shimo-state-test")
	if got := stateDir(); got != "/tmp/shimo-state-test" {
		t.Fatalf("stateDir = %q", got)
	}
}
