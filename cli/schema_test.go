package main

import (
	"strings"
	"testing"
)

func TestSchemaPhase3DoesNotClaimFullDelivery(t *testing.T) {
	p, ok := schema()["phase3"].(map[string]any)
	if !ok || p["status"] != "accepted-with-deferred-capability" || p["deferredCapability"] != "37.2" {
		t.Fatal("phase3 must distinguish accepted scope from deferred association fields")
	}
	if p["requiredCapabilities"] != 28 || p["automaticMutationReplay"] != false {
		t.Fatal("phase3 scope or replay contract missing")
	}
	if p["localArtifactScope"] != "package structure only; actionVerified=false" {
		t.Fatal("local artifact validation must not imply action acceptance")
	}
}

func TestSchemaDisclosesBrowserAndCredentialRequirements(t *testing.T) {
	s := schema()
	storage := s["auth"].(map[string]any)["storage"].(string)
	if !strings.Contains(storage, "0600") {
		t.Fatal("credential file fallback is missing")
	}
	actions := s["editor"].(map[string]any)["browserRequiredActions"].([]string)
	for _, want := range []string{"mentions-list", "comments-list", "history-show", "history-hide", "comments-show", "comments-hide"} {
		found := false
		for _, action := range actions {
			found = found || action == want
		}
		if !found {
			t.Errorf("browser requirement missing for %s", want)
		}
	}
}

func TestSchema(t *testing.T) {
	s := schema()
	if s["version"] != version {
		t.Errorf("version %v", s["version"])
	}
	cmds, _ := s["commands"].([]map[string]any)
	if len(cmds) != len(dispatch(newApp(nil)))+1 {
		t.Errorf("commands = %d", len(cmds))
	}
	if len(s["scenarios"].([]string)) != 28 {
		t.Errorf("scenarios = %d, want 28", len(s["scenarios"].([]string)))
	}
	if s["scenarioCount"] != 28 || s["capabilityCount"] != capabilityCount {
		t.Error("scenario metadata changed")
	}
	names := map[string]bool{}
	for _, c := range cmds {
		names[c["name"].(string)] = true
	}
	for _, want := range []string{"login", "logout", "phase2", "phase3", "subscriptions", "mkdir", "session"} {
		if !names[want] {
			t.Errorf("schema missing %s", want)
		}
	}
}
