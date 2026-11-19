package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type releaseFormBrokenReader struct{}

func (releaseFormBrokenReader) Read([]byte) (int, error) {
	return 0, errors.New("synthetic truncation")
}
func (releaseFormBrokenReader) Close() error { return nil }

func TestReleaseFormSignedDraftTransportRejectsUnsafeAndPartialContent(t *testing.T) {
	for _, mode := range []string{"non-object", "http-url", "userinfo-url", "missing-url", "transport-error", "http-403", "truncated", "oversized", "decode-error", "invalid-delta", "redirect-http", "redirect-loop"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			old, oldDefault := httpClient, http.DefaultTransport
			defer func() { httpClient = old; http.DefaultTransport = oldDefault }()
			contentRequests := 0
			transport := phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/sdk/forms/abcdefgh/content" {
					switch mode {
					case "non-object":
						return phase3Response(200, `[]`), nil
					case "http-url":
						return phase3Response(200, `{"contentUrl":"http://signed.test/draft?secret=do-not-log"}`), nil
					case "userinfo-url":
						return phase3Response(200, `{"contentUrl":"https://user:pass@signed.test/draft"}`), nil
					case "missing-url":
						return phase3Response(200, `{}`), nil
					default:
						return phase3Response(200, `{"contentUrl":"https://signed.test/draft?secret=do-not-log"}`), nil
					}
				}
				contentRequests++
				if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Sdk-Token") != "" {
					t.Error("credentials leaked to signed content endpoint")
				}
				if mode == "transport-error" {
					return nil, errors.New("secret=do-not-log")
				}
				if mode == "http-403" {
					return phase3Response(403, `forbidden`), nil
				}
				if mode == "truncated" {
					r := phase3Response(200, "")
					r.Body = releaseFormBrokenReader{}
					return r, nil
				}
				if mode == "oversized" {
					r := phase3Response(200, "")
					r.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", 8*1024*1024+1)))
					return r, nil
				}
				if mode == "redirect-http" || mode == "redirect-loop" {
					reply := phase3Response(302, "")
					next := "https://signed.test/draft?secret=do-not-log"
					if mode == "redirect-http" {
						next = "http://signed.test/draft?secret=do-not-log"
					}
					reply.Header.Set("Location", next)
					return reply, nil
				}
				return phase3Response(200, "synthetic compressed data"), nil
			})
			httpClient = &http.Client{Transport: transport}
			http.DefaultTransport = transport
			b := releaseFormPeer(t, func(expr string) (any, error) {
				if !strings.HasPrefix(expr, "new window.modoc.Delta(") {
					return nil, errors.New("unexpected decoder request")
				}
				if mode == "decode-error" {
					return nil, errors.New("decoder rejected input")
				}
				return []any{}, nil
			})
			defer b.Close()
			a := newApp(nil)
			a.Origin = "https://phase3.test"
			s := &phase3SDKSession{app: a, guid: "abcdefgh", prefix: "/sdk/", headers: map[string]string{"X-Sdk-Token": "synthetic"}}
			_, err := s.formDraft(b)
			if err == nil || strings.Contains(err.Error(), "do-not-log") {
				t.Fatal("invalid draft accepted or secret leaked", err)
			}
			if mode == "non-object" || mode == "http-url" || mode == "userinfo-url" || mode == "missing-url" {
				if contentRequests != 0 {
					t.Fatal("invalid location was requested")
				}
			}
			if mode == "redirect-http" && contentRequests != 1 {
				t.Fatal("unsafe redirect followed", contentRequests)
			}
			if mode == "redirect-loop" && contentRequests != 4 {
				t.Fatal("redirect budget wrong", contentRequests)
			}
		})
	}
}
