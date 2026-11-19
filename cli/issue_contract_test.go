package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func issueCapture(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout-")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = old; _ = f.Close() }()
	runErr := fn()
	raw, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), runErr
}

func TestIssueGlobalJSONBeforeCommand(t *testing.T) {
	a := newApp([]string{"--json", "version"})
	if a.Cmd != "version" || !a.JSON {
		t.Fatalf("global flag misparsed: %#v", a)
	}
}

func TestIssueCLIProcess(t *testing.T) {
	if os.Getenv("SHIMO_ISSUE_CLI_HELPER") != "1" {
		return
	}
	for i, v := range os.Args {
		if v == "--" {
			os.Args = append([]string{"shimo-net"}, os.Args[i+1:]...)
			break
		}
	}
	main()
	os.Exit(0)
}

func TestIssueUnknownCommandJSON(t *testing.T) {
	c := exec.Command(os.Args[0], "-test.run=^TestIssueCLIProcess$", "--", "--json", "not-a-command")
	c.Env = append(os.Environ(), "SHIMO_ISSUE_CLI_HELPER=1")
	out, err := c.Output()
	var ex *exec.ExitError
	if !errors.As(err, &ex) || ex.ExitCode() != 2 {
		t.Fatalf("exit=%v output=%s", err, out)
	}
	var result map[string]any
	if err := json.Unmarshal(ex.Stderr, &result); err != nil || result["ok"] != false {
		t.Fatalf("not one JSON error: %s", ex.Stderr)
	}
	if len(out) != 0 {
		t.Fatalf("unexpected stdout: %s", out)
	}
}

func TestIssueInvalidConfigDoesNotPersist(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_ORIGIN", "")
	if err := saveConfig("https://previous.invalid"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(configPath())
	old := httpClient
	defer func() { httpClient = old }()
	for _, status := range []int{404, 500} {
		httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) { return phase3Response(status, `{}`), nil })}
		for _, args := range [][]string{{"init", "--origin", "https://bad.invalid"}, {"config", "set", "https://bad.invalid"}} {
			_, err := issueCapture(t, newApp(args).Run)
			if err == nil {
				t.Errorf("HTTP %d accepted: %v", status, args)
			}
			after, _ := os.ReadFile(configPath())
			if string(before) != string(after) {
				t.Errorf("failed probe saved invalid origin: %v", args)
			}
		}
	}
}

func TestIssueDoctorMissingCredentialsSingleFailure(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_COOKIE", "")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	t.Setenv("PATH", t.TempDir())
	out, err := issueCapture(t, newApp([]string{"doctor", "--json"}).Run)
	if err == nil {
		t.Fatal("missing credentials passed doctor")
	}
	if out != "" {
		t.Fatalf("failure must emit only one JSON envelope through main, stdout=%s", out)
	}
	// The structured error must include the red credentials check.
	type resultCarrier interface{ ResultDetails() map[string]any }
	var carrier resultCarrier
	if !errors.As(err, &carrier) {
		t.Fatalf("missing diagnostic details: %v", err)
	}
	checks := carrier.ResultDetails()["checks"].([]map[string]any)
	for _, check := range checks {
		if check["name"] == "credentials" {
			if check["ok"] != false || check["detail"] != "未登录" {
				t.Fatalf("false credentials success: %v", check)
			}
			return
		}
	}
	t.Fatal("credentials check missing")
}

func TestIssueAuditUnknownIsNotPrivate(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
	t.Setenv("SHIMO_NET_ORIGIN", "https://audit.invalid")
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	old := httpClient
	defer func() { httpClient = old }()
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files":
			return phase3Response(200, `{"list":[{"guid":"a","name":"A"},{"guid":"b","name":"B"},{"guid":"c","name":"C"},{"guid":"d","name":"D"}]}`), nil
		case "/api/v1/files/a":
			return phase3Response(403, `{}`), nil
		case "/api/v1/files/b":
			return phase3Response(200, `{"name":"B","shareMode":"private"}`), nil
		case "/api/v1/files/c":
			return phase3Response(200, `{"name":"C","shareMode":"readonly"}`), nil
		case "/api/v1/files/d":
			return phase3Response(200, `{"name":"D","shareMode":"enterprise_undefined"}`), nil
		}
		t.Errorf("unexpected URL %s", r.URL)
		return nil, errors.New("unexpected request")
	})}
	out, err := issueCapture(t, newApp([]string{"audit", "desktop", "--json"}).Run)
	if err == nil {
		t.Fatalf("unknown audit passed: %s", out)
	}
	type resultCarrier interface{ ResultDetails() map[string]any }
	var carrier resultCarrier
	if !errors.As(err, &carrier) {
		t.Fatalf("missing audit result: %v", err)
	}
	groups := carrier.ResultDetails()["groups"].(map[string][]map[string]any)
	if len(groups["unknown"]) != 2 || len(groups["private"]) != 1 || len(groups["public"]) != 1 {
		t.Fatalf("wrong groups: %v", groups)
	}
	if out != "" {
		t.Fatalf("duplicate output: %s", out)
	}
}

func TestIssueHelpSheetFormatPayload(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
	t.Setenv("SHIMO_NET_ORIGIN", "https://shimo.net")
	old := httpClient
	defer func() { httpClient = old }()
	calls := 0
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(raw, &p)
		if !strings.Contains(str(p, "range"), "!") {
			t.Errorf("invalid help range: %s", raw)
		}
		return phase3Response(200, `{}`), nil
	})}
	for _, line := range strings.Split(helpText(), "\n") {
		if strings.Contains(line, "--action sheet-format --payload '") {
			payload := strings.Split(line, "--payload '")[1]
			payload = strings.TrimSuffix(payload, "'")
			_, err := issueCapture(t, newApp([]string{"editor", "https://shimo.net/sheets/abcdefgh", "--action", "sheet-format", "--payload", payload, "--no-verify", "--json"}).Run)
			if err != nil {
				t.Fatal(err)
			}
			if calls == 0 {
				t.Fatal("help payload never reached API")
			}
			return
		}
	}
	t.Fatal("sheet-format example missing")
}

func TestIssueInstallerDoesNotHarvestOnInstall(t *testing.T) {
	for _, name := range []string{"install.command", "install.ps1"} {
		raw, err := os.ReadFile(filepath.Join("..", name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "\"$BIN\" doctor") || strings.Contains(string(raw), "& $dest doctor") {
			t.Errorf("%s triggers credential harvest during install", name)
		}
	}
}

func TestIssueReloginNeverReplaysMutationsOrPartialResults(t *testing.T) {
	for _, command := range []string{"import", "upload", "new", "delete", "rename", "restore", "phase3", "editor", "phase2"} {
		a := newApp([]string{command, "--relogin", "--action", "sheet-append"})
		if a.canRelogin(errors.New("HTTP 401")) {
			t.Errorf("mutation eligible for replay: %s", command)
		}
	}
	a := newApp([]string{"pull", "--relogin"})
	if a.canRelogin(&commandError{Message: "HTTP 401 after partial download", NoReplay: true}) {
		t.Fatal("partial pull replayable")
	}
	if !newApp([]string{"whoami", "--relogin"}).canRelogin(errors.New("HTTP 401")) {
		t.Fatal("explicit read-only relogin lost")
	}
}

func TestIssueDoctorCLIEmitsOneJSONError(t *testing.T) {
	c := exec.Command(os.Args[0], "-test.run=^TestIssueCLIProcess$", "--", "doctor", "--json")
	c.Env = append(os.Environ(), "SHIMO_ISSUE_CLI_HELPER=1", "SHIMO_NET_STATE_DIR="+t.TempDir(), "SHIMO_NET_COOKIE=", "SHIMO_NET_DISABLE_HARVEST=1", "SHIMO_NET_KEYCHAIN_SERVICE=shimo-net-test-no-credentials")
	out, err := c.Output()
	var ex *exec.ExitError
	if !errors.As(err, &ex) || ex.ExitCode() != 1 {
		t.Fatalf("exit=%v output=%s", err, out)
	}
	var result map[string]any
	if err := json.Unmarshal(ex.Stderr, &result); err != nil || result["ok"] != false || result["checks"] == nil {
		t.Fatalf("not a single diagnostic JSON: %s", ex.Stderr)
	}
	if len(out) != 0 {
		t.Fatalf("unexpected stdout: %s", out)
	}
}

func TestIssueNoBrowserAlsoPreventsCredentialHarvest(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "")
	t.Setenv("SHIMO_NET_ORIGIN", "https://shimo.net")
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "")
	oldClient, oldService := httpClient, keychainService
	keychainService = "invalid test service with spaces"
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) { return phase3Response(200, `{}`), nil })}
	harvestMu.Lock()
	oldDone, oldCookie, oldErr, oldOrigin := harvestDone, harvestCookie, harvestErr, harvestOrigin
	harvestDone, harvestCookie, harvestErr, harvestOrigin = true, "sid=synthetic-harvest", nil, "https://shimo.net"
	harvestMu.Unlock()
	defer func() {
		httpClient, keychainService = oldClient, oldService
		harvestMu.Lock()
		harvestDone, harvestCookie, harvestErr, harvestOrigin = oldDone, oldCookie, oldErr, oldOrigin
		harvestMu.Unlock()
	}()
	a := newApp([]string{"editor", "https://shimo.net/sheets/abcdefgh", "--action", "sheet-values", "--payload", `{"range":"Sheet1!A1"}`, "--no-browser"})
	_, err := issueCapture(t, a.Run)
	if err == nil {
		t.Fatal("no-browser silently used browser-harvested credentials")
	}
	if _, err := os.Stat(credPath()); !os.IsNotExist(err) {
		t.Fatal("no-browser saved browser-harvested credentials")
	}
}
