package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPhase3TableViewReadOnlyProtocol(t *testing.T) {
	out := os.Getenv("SHIMO_PHASE3_VIEW_PROTOCOL_OUT")
	if out == "" {
		t.Skip("explicit fixed fixture output required")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	app := newApp([]string{"phase3"})
	s, err := app.phase3SDK("https://shimo.net/tables/DqdxLrxlDMcw78q4", "table")
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.get("table-api/tables/61492df5d31/views/2acdb73f09c")
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(out, "view.json"), data); err != nil {
		t.Fatal(err)
	}
}
