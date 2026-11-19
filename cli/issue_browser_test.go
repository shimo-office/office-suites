package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestIssueBrowserActionsRespectNoBrowserBeforeTargetLookup(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=fake-test")
	old := httpClient
	defer func() { httpClient = old }()
	calls := 0
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected target lookup")
	})}
	for _, action := range []string{"mentions-list", "comments-list"} {
		for _, flag := range []string{"--no-browser", "env"} {
			t.Run(action+flag, func(t *testing.T) {
				t.Setenv("SHIMO_NET_NO_BROWSER", "")
				args := []string{"editor", "not-a-real-guid", "--action", action}
				if flag == "env" {
					t.Setenv("SHIMO_NET_NO_BROWSER", "true")
				} else {
					args = append(args, flag)
				}
				err := newApp(args).cmdPhase2()
				if err == nil || !strings.Contains(err.Error(), "禁止启动浏览器") {
					t.Fatalf("browser guard missing: %v", err)
				}
			})
		}
	}
	if calls != 0 {
		t.Fatalf("forbidden action performed %d target lookups", calls)
	}
}

func TestIssuePanelActionsRejectIncompleteWorkflowBeforeLookup(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=fake-test")
	old := httpClient
	defer func() { httpClient = old }()
	calls := 0
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected target lookup")
	})}
	for _, action := range []string{"save-version", "comment-add", "print"} {
		a := newApp([]string{"editor", "not-a-real-guid", "--action", action, "--verify"})
		if err := a.cmdPhase2(); err == nil || !strings.Contains(err.Error(), "未保存、未提交、未打印") {
			t.Errorf("%s misrepresents panel-only workflow: %v", action, err)
		}
	}
	if calls != 0 {
		t.Fatalf("panel rejection performed %d target lookups", calls)
	}
}

func TestIssueAvailableActionsAreExecutable(t *testing.T) {
	for _, action := range phase2ActionNames() {
		if _, ok := phase2Actions[action]; !ok {
			t.Errorf("advertised unsupported action %s", action)
		}
	}
	a := newApp([]string{"editor", "not-a-real-guid", "--action", "not-a-supported-action"})
	err := a.cmdPhase2()
	if err == nil {
		t.Fatal("unknown action accepted")
	}
	for _, page := range []string{"find", "replace-all", "page-width", "sheet-freeze", "sheet-sort", "sheet-filter", "sheet-validation", "save-version", "comment-add", "print"} {
		if strings.Contains(err.Error(), page) {
			t.Errorf("unsupported %s advertised: %v", page, err)
		}
	}
}

func TestIssueNoVerifyDoesNotMeanNoBrowser(t *testing.T) {
	t.Setenv("SHIMO_NET_NO_BROWSER", "")
	a := newApp([]string{"editor", "unused", "--no-verify", "--verify"})
	if explicitNoBrowser(a) {
		t.Fatal("no-verify must only disable screenshots")
	}
	if cliBrowserVerification(a) {
		t.Fatal("no-verify must still override screenshot request")
	}
}

func TestIssueBrowserTargetMustMatchCredentialOrigin(t *testing.T) {
	t.Setenv("SHIMO_NET_ORIGIN", "https://shimo.net")
	for _, target := range []string{"http://shimo.net/docs/abcdefgh", "https://shimo.net:444/docs/abcdefgh", "https://elsewhere.test/docs/abcdefgh"} {
		if sameCookieHost(target) {
			t.Errorf("cross-origin browser injection permitted: %s", target)
		}
		a := newApp([]string{"editor", target, "--action", "sheet-update", "--payload", `{}`})
		if err := a.cmdPhase2(); err == nil || !strings.Contains(err.Error(), "origin 不一致") {
			t.Errorf("wrong-origin command not blocked: %v", err)
		}
	}
	if !sameCookieHost("https://shimo.net:443/docs/abcdefgh") {
		t.Fatal("equivalent default port should remain supported")
	}
}

func TestIssueOpenNoBrowserRejectsBeforeTargetLookup(t *testing.T) {
	t.Setenv("SHIMO_NET_NO_BROWSER", "true")
	a := newApp([]string{"open", "not-a-real-guid"})
	if err := a.cmdOpen(); err == nil || !strings.Contains(err.Error(), "禁止启动浏览器") {
		t.Fatalf("open did not respect browser prohibition: %v", err)
	}
}

func TestIssueManagedOpenHasNoDebugPortOrSessionClaim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake browser")
	}
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_COOKIE", "sid=should-not-be-injected")
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	binary := filepath.Join(dir, "browser")
	t.Setenv("SHIMO_TEST_BROWSER_ARGS", argsPath)
	// Publish a complete argument record; cmd.Start may return before printf.
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$SHIMO_TEST_BROWSER_ARGS.tmp\"\nmv \"$SHIMO_TEST_BROWSER_ARGS.tmp\" \"$SHIMO_TEST_BROWSER_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	a := newApp([]string{"open", "https://shimo.net/docs/abcdefgh", "--json"})
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = original; _ = out.Close() }()
	if err := a.openManagedBrowser(binary, "Test Browser", "managed", a.Pos[0]); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if response["sessionInjected"] != false || response["debugPortEnabled"] != false {
		t.Fatalf("misleading security claim: %s", raw)
	}
	var args []byte
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		args, err = os.ReadFile(argsPath)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "remote-debugging") || strings.Contains(string(args), "sid=") {
		t.Fatalf("unsafe browser args: %s", args)
	}
	if !strings.Contains(string(args), "open-profile") || !strings.Contains(string(args), a.Pos[0]) {
		t.Fatalf("wrong profile/target: %s", args)
	}
}

func TestIssueManagedOpenRejectsDebugProfile(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	profile := filepath.Join(stateDir(), "open-profile")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "DevToolsActivePort"), []byte("12345\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := newApp([]string{"open", "https://shimo.net/docs/abcdefgh"})
	if err := a.openManagedBrowser("never-launch", "fake", "managed", a.Pos[0]); err == nil || !strings.Contains(err.Error(), "调试端口") {
		t.Fatalf("debug profile was not blocked before launch: %v", err)
	}
}
