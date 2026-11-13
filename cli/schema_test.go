package main

import "testing"

func TestSchema(t *testing.T) {
	s := schema()
	if s["version"] != version {
		t.Errorf("version %v", s["version"])
	}
	cmds, _ := s["commands"].([]map[string]any)
	if len(cmds) != 26 {
		t.Errorf("commands = %d", len(cmds))
	}
	if len(s["scenarios"].([]string)) != 28 {
		t.Errorf("scenarios = %d, want 28", len(s["scenarios"].([]string)))
	}
	if s["scenarioCount"] != 28 || s["capabilityCount"] != 45 {
		t.Error("scenario metadata changed")
	}
	names := map[string]bool{}
	for _, c := range cmds {
		names[c["name"].(string)] = true
	}
	for _, want := range []string{"login", "logout", "phase2", "subscriptions"} {
		if !names[want] {
			t.Errorf("schema missing %s", want)
		}
	}
}
