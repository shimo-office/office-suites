package main

import "testing"

func TestNormalizeOrigin(t *testing.T) {
	cases := map[string]string{
		"shimo.net":                 "https://shimo.net",
		"https://shimo.net/":        "https://shimo.net",
		"http://127.0.0.1:8877/":    "http://127.0.0.1:8877",
		"https://Example.com/base/": "https://example.com/base",
	}
	for in, want := range cases {
		got, err := normalizeOrigin(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
	}
	if _, err := normalizeOrigin(""); err == nil {
		t.Error("empty origin should fail")
	}
}

func TestParseSince(t *testing.T) {
	if _, err := parseSince("7d"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSince("2026-11-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSince("nope"); err == nil {
		t.Error("bad since should fail")
	}
}
