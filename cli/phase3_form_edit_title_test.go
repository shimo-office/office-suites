package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFormTitleFileNameSync(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	for _, tc := range []struct {
		name, current, after string
		wantWrites           int
		wantErr              bool
	}{
		{"sync", "before", "requested", 1, false}, {"already-synced", "requested", "requested", 0, false},
		{"concurrent-rename", "someone else", "requested", 0, true}, {"stale-readback", "before", "before", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current, writes := tc.current, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					writes++
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if body["name"] != "requested" {
						t.Errorf("wrong name %v", body)
					}
					current = tc.after
				}
				json.NewEncoder(w).Encode(map[string]any{"guid": "form12345678", "type": "form", "name": current})
			}))
			defer server.Close()
			a := &App{Origin: server.URL}
			_, err := a.phase3SyncFormTitle("form12345678", "before", "requested")
			if (err != nil) != tc.wantErr || writes != tc.wantWrites {
				t.Fatalf("err=%v writes=%d", err, writes)
			}
		})
	}
}
