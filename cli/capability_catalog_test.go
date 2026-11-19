package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestEditorAliasRejectsPageOnlyAndManualActions(t *testing.T) {
	for _, action := range []string{"find", "replace-all", "sheet-freeze", "sheet-sort", "sheet-filter", "purge"} {
		var messages []string
		for _, command := range []string{"editor", "phase2"} {
			a := newApp([]string{command, "not-a-real-guid", "--action", action})
			fn, ok := dispatch(a)[command]
			if !ok {
				t.Errorf("missing compatible command %s", command)
				continue
			}
			err := fn()
			if err == nil || (!strings.Contains(err.Error(), "页面") && !strings.Contains(err.Error(), "网页")) {
				t.Fatalf("%s %s must reject before target/network access: %v", command, action, err)
			}
			messages = append(messages, err.Error())
		}
		if len(messages) == 2 && messages[0] != messages[1] {
			t.Errorf("alias error differs for %s: %q", action, messages)
		}
	}
}

func TestSchemaPublicCommandContract(t *testing.T) {
	commands := map[string]map[string]any{}
	for _, command := range schema()["commands"].([]map[string]any) {
		name := command["name"].(string)
		if commands[name] != nil {
			t.Fatalf("duplicate command %s", name)
		}
		commands[name] = command
	}
	for _, name := range []string{"editor", "phase2", "purge", "version", "session", "config"} {
		if commands[name] == nil {
			t.Errorf("schema missing %s", name)
		}
	}
	for _, name := range []string{"doc", "docx", "sheet", "slide", "table", "form", "mindmap", "flowchart"} {
		if !strings.Contains(commands["new"]["usage"].(string), name) || createTypes[name] == "" {
			t.Errorf("new type %s not consistently advertised", name)
		}
	}
	if commands["phase2"]["aliasOf"] != "editor" {
		t.Error("legacy command must explicitly identify its canonical alias")
	}
}

func TestCapabilityCatalogMatchesSchemaHelpAndMatrix(t *testing.T) {
	raw, err := os.ReadFile("../docs/internal/capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Scenarios []struct {
			ID           int    `json:"id"`
			Name         string `json:"name"`
			Help         string `json:"help"`
			Capabilities []struct {
				ID   string `json:"id"`
				Mode string `json:"mode"`
			} `json:"capabilities"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	var names []string
	matrix, err := os.ReadFile("../docs/internal/capability-matrix.md")
	if err != nil {
		t.Fatal(err)
	}
	for i, scene := range catalog.Scenarios {
		if scene.ID != i+1 || scene.Name == "" || len(scene.Capabilities) == 0 {
			t.Fatalf("incomplete scenario at %d", i)
		}
		names = append(names, scene.Name)
		if !strings.Contains(helpText(), scene.Help) {
			t.Errorf("help omits source scenario %d", scene.ID)
		}
		for _, capability := range scene.Capabilities {
			if ids[capability.ID] || capability.ID == "" {
				t.Fatalf("invalid or repeated capability id %q", capability.ID)
			}
			ids[capability.ID] = true
			if capability.Mode != "API" && capability.Mode != "页面操作" && capability.Mode != "人工禁用" {
				t.Errorf("unclassified capability %s", capability.ID)
			}
			if strings.Count(string(matrix), "| "+capability.ID+" |") != 1 {
				t.Errorf("matrix must contain capability %s exactly once", capability.ID)
			}
		}
	}
	if got := len(regexp.MustCompile(`(?m)^\| [0-9]+\.[0-9]+ \|`).FindAll(matrix, -1)); got != len(ids) {
		t.Errorf("matrix contains %d rows, expected %d", got, len(ids))
	}
	s := schema()
	if len(names) != 28 || !reflect.DeepEqual(s["scenarios"], names) || s["scenarioCount"] != len(names) || s["capabilityCount"] != len(ids) {
		t.Errorf("schema drifted from source: %v scenarios, %v capabilities", s["scenarioCount"], s["capabilityCount"])
	}
	for label, text := range map[string]string{"help": helpText(), "matrix": string(matrix)} {
		if !strings.Contains(text, fmt.Sprintf("%d 个场景", len(names))) || !strings.Contains(text, fmt.Sprintf("%d 项能力", len(ids))) {
			t.Errorf("%s counts drifted", label)
		}
	}
	digest := sha256.Sum256(raw)
	if s["capabilityCatalogSHA256"] != hex.EncodeToString(digest[:]) {
		t.Fatal("generated catalog is stale; run python3 scripts/generate_capabilities.py")
	}
}

func TestEditorAliasUsesSameAPIWithoutClaimingVisualVerification(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	t.Setenv("SHIMO_NET_ORIGIN", "https://shimo.net")
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	previousClient := httpClient
	defer func() { httpClient = previousClient }()
	calls := 0
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost || r.URL.String() != "https://shimo.net/lizard-api/files/abcdefgh/sheets/values" {
			t.Errorf("wrong mutation route: %s %s", r.Method, r.URL)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["range"] != "Sheet1!A1" || !reflect.DeepEqual(payload["resource"], map[string]any{"values": []any{[]any{"value"}}}) {
			t.Errorf("alias changed payload: %s", raw)
		}
		return phase3Response(200, `{}`), nil
	})}
	var outputs []map[string]any
	for _, command := range []string{"editor", "phase2"} {
		func() {
			out, err := os.CreateTemp(t.TempDir(), "output-")
			if err != nil {
				t.Fatal(err)
			}
			originalStdout := os.Stdout
			os.Stdout = out
			defer func() { os.Stdout = originalStdout; _ = out.Close() }()
			a := newApp([]string{command, "https://shimo.net/sheets/abcdefgh", "--action", "sheet-update", "--payload", `{"range":"Sheet1!A1","resource":{"values":[["value"]]}}`, "--json"})
			if err := a.Run(); err != nil {
				t.Fatal(err)
			}
			if _, err := out.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			var output map[string]any
			if err := json.NewDecoder(out).Decode(&output); err != nil {
				t.Fatal(err)
			}
			if output["verified"] != false || output["verifyWith"] != "host-browser" {
				t.Errorf("alias falsely verified browser evidence: %v", output)
			}
			outputs = append(outputs, output)
		}()
	}
	if calls != 2 || !reflect.DeepEqual(outputs[0], outputs[1]) {
		t.Errorf("aliases differ: %d calls, %#v", calls, outputs)
	}
}
