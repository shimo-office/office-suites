package main

import "testing"

func TestSchema(t *testing.T) {
	s := schema()
	if s["version"] != version {
		t.Errorf("version %v", s["version"])
	}
	cmds, _ := s["commands"].([]map[string]any)
	if len(cmds) != 22 {
		t.Errorf("commands = %d", len(cmds))
	}
	if len(s["scenarios"].([]string)) != 15 {
		t.Error("scenarios != 15")
	}
}
