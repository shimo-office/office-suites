package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchAndURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search/files" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("cookie") != "sid=test" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"source":{"guid":"doc12345678","name":"Q4","type":"document","url":"/docs/doc12345678","ancestors":[{"name":"团队"}]}}]}`))
	}))
	defer srv.Close()
	a := &App{Origin: srv.URL, JSON: true}
	r, err := a.request("/search/files", "POST", map[string]any{"keyword": "Q4"}, nil, nil, "sid=test")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != 200 {
		t.Fatalf("status %d", r.Status)
	}
	if got := a.urlOf(map[string]any{"guid": "doc12345678", "type": "document"}); got != srv.URL+"/docs/doc12345678" {
		t.Errorf("urlOf = %s", got)
	}
}

func TestRequestPathKeepsLizardAPIPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lizard-api/files/demo/sheets/attrs" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodPost || r.Header.Get("cookie") != "sid=test" {
			t.Fatalf("method/cookie = %s/%q", r.Method, r.Header.Get("cookie"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	a := &App{Origin: srv.URL}
	r, err := a.requestPath("/lizard-api/files/demo/sheets/attrs", http.MethodPost,
		map[string]any{"range": "A1", "attrs": map[string]any{"bold": true}}, nil, nil, "sid=test")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != http.StatusNoContent {
		t.Fatalf("status = %d", r.Status)
	}
}
