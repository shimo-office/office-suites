package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPhase3DashboardReadOnlyProbe(t *testing.T) {
	out := os.Getenv("SHIMO_PHASE3_DASHBOARD_PROBE_OUT")
	if out == "" {
		t.Skip("fixed fixture only")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	app := newApp([]string{"phase3"})
	s, err := app.phase3SDK("https://shimo.net/tables/DqdxLrxlDMcw78q4", "table")
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"dashboard": "table-api/bases/DqdxLrxlDMcw78q4/dashboards/bb7b78865a4", "base": "table-api/v2/bases/DqdxLrxlDMcw78q4"} {
		raw, err := s.get(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(out, name+".json"), raw); err != nil {
			t.Fatal(err)
		}
	}
}
