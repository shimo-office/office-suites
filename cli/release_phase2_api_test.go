package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestReleasePhase2RequestEncodingAndReadErrors(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	old := httpClient
	defer func() { httpClient = old }()
	calls := 0
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/custom" || r.Header.Get("Content-Type") != "application/custom" {
			t.Error(r.URL, r.Header.Get("Content-Type"))
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != "raw body" {
			t.Error(string(raw))
		}
		res := phase3Response(200, "")
		res.Body = releaseFormBrokenReader{}
		return res, nil
	})}
	a := newApp(nil)
	a.Origin = "https://phase3.test"
	if _, err := a.requestPath("custom", "POST", map[string]any{"ignored": true}, []byte("raw body"), map[string]string{"Content-Type": "application/custom"}, "sid=synthetic"); err == nil {
		t.Fatal("truncated HTTP response accepted")
	}
	if _, err := a.requestPath("custom", "POST", map[string]any{"bad": make(chan int)}, nil, nil, "sid=synthetic"); err == nil {
		t.Fatal("unencodable request accepted")
	}
	if _, err := a.requestURL(":bad-url", "GET", nil, nil, nil, "sid=synthetic"); err == nil {
		t.Fatal("invalid URL accepted")
	}
	if calls != 1 {
		t.Fatal("invalid request reached transport", calls)
	}
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("synthetic timeout") })}
	if _, err := a.authRequestPath("/custom", "GET", nil, nil, nil); err == nil {
		t.Fatal("transport error hidden")
	}
}

func TestReleasePhase2IdentityProbeAndPublicDownload(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	old, oldDefault := httpClient, http.DefaultTransport
	defer func() { httpClient = old; http.DefaultTransport = oldDefault }()
	for _, mode := range []string{"success", "unauthorized", "network"} {
		t.Run(mode, func(t *testing.T) {
			transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if mode == "network" {
					return nil, errors.New("synthetic network failure")
				}
				if mode == "unauthorized" {
					return phase3Response(401, `{}`), nil
				}
				return phase3Response(200, `{"id":42,"name":"Synthetic"}`), nil
			})
			httpClient = &http.Client{Transport: transport}
			a := newApp(nil)
			a.Origin = "https://phase3.test"
			got, err := a.usersMeWithCookie("sid=synthetic")
			if mode == "success" {
				if err != nil || got["id"] != float64(42) {
					t.Fatal(got, err)
				}
			} else if err == nil {
				t.Fatal("identity probe failure hidden")
			}
			http.DefaultTransport = transport
			raw, e := httpGet("https://public.test/download")
			if mode == "network" {
				if e == nil {
					t.Fatal("download transport failure hidden")
				}
			} else if e != nil || len(raw) == 0 {
				t.Fatal(e)
			}
		})
	}
}

func TestReleasePhase2MetadataRepresentations(t *testing.T) {
	a := newApp(nil)
	a.Origin = "https://private.test/"
	for _, tc := range []struct {
		file       map[string]any
		url, label string
	}{
		{map[string]any{"isSpace": true, "space_guid": "space001"}, "https://private.test/space/space001", "空间"},
		{map[string]any{"isSpace": true, "guid": "space002"}, "https://private.test/space/space002", "空间"},
		{map[string]any{"isFolder": true, "guid": "folder01"}, "https://private.test/folder/folder01", "文件夹"},
		{map[string]any{"url": "/files/custom01", "isCloudFile": true}, "https://private.test/files/custom01", "云文件"},
		{map[string]any{"type": "custom", "guid": "file0001"}, "https://private.test/files/file0001", "custom"},
		{map[string]any{"guid": "file0002"}, "https://private.test/files/file0002", "文件"},
	} {
		if a.urlOf(tc.file) != tc.url || typeLabel(tc.file) != tc.label {
			t.Fatal(tc, a.urlOf(tc.file), typeLabel(tc.file))
		}
	}
	for _, v := range []any{float64(3.5), true} {
		if str(map[string]any{"v": v}, "v") == "" {
			t.Fatal("metadata scalar omitted")
		}
	}
	for _, v := range []any{int(42), json.Number("42"), "42"} {
		if num(map[string]any{"v": v}, "v") != 42 {
			t.Fatal(v)
		}
	}
	if !boolv(map[string]any{"v": "true"}, "v") || !boolv(map[string]any{"v": float64(1)}, "v") {
		t.Fatal("JSON metadata truth value lost")
	}
	for _, v := range []any{nil, []any{}} {
		if fmtTime(v) != "" {
			t.Fatal(v)
		}
	}
	if fmtTime("short") != "short" || fmtTime("2026-11-19T00:00:00Z") != "2026-11-19" {
		t.Fatal("time label wrong")
	}
	stamp := time.Date(2026, 11, 19, 12, 0, 0, 0, time.Local)
	if fmtTime(float64(stamp.UnixMilli())) != stamp.Format("2006-01-02") || timeOf(float64(stamp.UnixMilli())) != stamp.UnixMilli() || timeOf(stamp.Format("2006-01-02T15:04:05")) != stamp.UnixMilli() {
		t.Fatal("timestamp unit conversion wrong")
	}
	if timeOf("invalid timestamp that is long") != 0 || timeOf(true) != 0 {
		t.Fatal("invalid time accepted")
	}
	if extFromURL("https://example.test/noextension?filename=x.pdf", "bin") != "bin" || !strings.Contains(typeLabel(map[string]any{"type": "folder"}), "文件夹") {
		t.Fatal("fallback metadata wrong")
	}
}
