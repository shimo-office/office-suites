package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPhase3FormResultContextCancelsNextPage(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			time.Sleep(10 * time.Millisecond)
			json.NewEncoder(w).Encode(map[string]any{"total": 2, "responses": []any{map[string]any{"submitId": "old", "content": []any{}}}})
			return
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	s := &phase3SDKSession{app: &App{Origin: srv.URL}, prefix: "/sdk/", guid: "form12345678"}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := s.formResultsContext(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 2 || time.Since(start) > 1500*time.Millisecond {
		t.Fatal(err, calls.Load(), time.Since(start))
	}
	_, _, err = s.formResultsContext(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 2 {
		t.Fatal("expired budget dispatched another page", err, calls.Load())
	}
}
func TestPhase3FormSubmissionContextIncludesRetryPause(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"total": 0, "responses": []any{}})
	}))
	defer srv.Close()
	s := &phase3SDKSession{app: &App{Origin: srv.URL}, prefix: "/sdk/", guid: "form12345678"}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, _, err := phase3WaitFormSubmission(ctx, s.formResultsContext, nil, 0, "new", time.Second)
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 || time.Since(start) > 1500*time.Millisecond {
		t.Fatal(err, calls.Load(), time.Since(start))
	}
}
func TestPhase3FormResultLagRetainsParsedRows(t *testing.T) {
	row := map[string]any{"submitId": "new", "content": []any{}}
	rows, n, err := phase3FormPage(map[string]any{"total": float64(0), "responses": []any{row}})
	if !errors.Is(err, errPhase3FormResultTotalLag) || n != 0 || len(rows) != 1 || str(rows[0], "submitId") != "new" {
		t.Fatal(rows, n, err)
	}
	got, n, err := phase3FormResultPages(func(int) ([]map[string]any, int, error) { return rows, n, err })
	if !errors.Is(err, errPhase3FormResultTotalLag) || n != 0 || len(got) != 1 {
		t.Fatal(got, n, err)
	}
}

func TestPhase3FormSubmissionContextDoesNotRetry401(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()
	s := &phase3SDKSession{app: &App{Origin: srv.URL}, prefix: "/sdk/", guid: "form12345678"}
	_, _, _, err := phase3WaitFormSubmission(context.Background(), s.formResultsContext, nil, 0, "new", 0)
	if err == nil || calls.Load() != 1 {
		t.Fatal("permanent error retried", err, calls.Load())
	}
}
