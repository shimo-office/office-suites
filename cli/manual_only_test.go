package main

import (
	"strings"
	"testing"
)

func TestHighRiskOperationsAreManualOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		app  *App
	}{
		{name: "purge", app: newApp([]string{"purge", "guid"})},
		{name: "phase2 lock", app: newApp([]string{"phase2", "https://shimo.net/docs/demo", "--action", "file-lock"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			switch tc.name {
			case "purge":
				err = tc.app.cmdPurge()
			case "phase2 lock":
				err = tc.app.cmdPhase2()
			}
			if err == nil || !strings.Contains(err.Error(), "人工") {
				t.Fatalf("error = %v, want manual-operation guidance", err)
			}
		})
	}
}
