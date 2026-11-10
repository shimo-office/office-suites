package main

import "testing"

func TestSchema(t *testing.T) {
	s := schema()
	if s["version"] != version {
		t.Errorf("version %v", s["version"])
	}
	cmds, _ := s["commands"].([]map[string]any)
	if len(cmds) != 24 {
		t.Errorf("commands = %d", len(cmds))
	}
	if len(s["scenarios"].([]string)) != 15 {
		t.Error("scenarios != 15")
	}
	names := map[string]bool{}
	for _, c := range cmds {
		names[c["name"].(string)] = true
	}
	for _, want := range []string{"login", "logout"} {
		if !names[want] {
			t.Errorf("schema missing %s", want)
		}
	}
}
