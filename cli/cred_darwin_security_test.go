//go:build darwin

package main

import (
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
)

func TestKeychainPasswordTravelsOnlyThroughStdin(t *testing.T) {
	cookie := "sid=synthetic; quote=\"\nnot-a-command"
	cmd, err := keychainSaveCommand("shimo-net-cli:test", "shimo-net", cookie)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.Args) != 2 || cmd.Args[0] != "/usr/bin/security" || cmd.Args[1] != "-i" {
		t.Fatalf("unsafe process arguments: %#v", cmd.Args)
	}
	if len(cmd.Env) != 0 {
		t.Fatal("credential command must not set secret environment")
	}
	raw, err := io.ReadAll(cmd.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), "\n") != 1 || !strings.HasSuffix(string(raw), "-X "+hex.EncodeToString([]byte(cookie))+"\n") {
		t.Fatal("stdin must encode arbitrary cookie without command injection")
	}
	if cmd.Stdout != nil || cmd.Stderr != nil {
		t.Fatal("security interactive output must not leak commands")
	}
}

func TestKeychainRejectsInteractiveTokenInjection(t *testing.T) {
	for _, bad := range []string{"x\nhelp", "x -w", "x\"", "x\\", ""} {
		if _, err := keychainSaveCommand(bad, "shimo-net", "sid=test"); err == nil {
			t.Errorf("accepted service injection %q", bad)
		}
		if _, err := keychainSaveCommand("shimo-net-cli", bad, "sid=test"); err == nil {
			t.Errorf("accepted account injection %q", bad)
		}
	}
}

func TestUnavailableSafeKeychainWriteUsesBoundPrivateFile(t *testing.T) {
	isolateCredentials(t)
	previous := keychainService
	keychainService = "unsupported service token"
	defer func() { keychainService = previous }()
	// The unsafe token is rejected before invoking security; no real Keychain
	// item is read or written by this test.
	if err := writeCred("https://shimo.net", "sid=synthetic"); err != nil {
		t.Fatal(err)
	}
	c := readCredFile()
	if c == nil || c.Storage != "file" || c.Cookie != "sid=synthetic" || c.Origin != "https://shimo.net" {
		t.Fatalf("unexpected fallback metadata: %#v", c)
	}
	st, err := os.Stat(credPath())
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("fallback credentials lack private permissions")
	}
	if got := credentialStorageLabel(c.Storage); strings.Contains(got, "Keychain") {
		t.Fatal("fallback mislabeled as Keychain")
	}
	if _, err := readCookieForOrigin("https://unrelated.invalid"); err == nil {
		t.Fatal("fallback cookie escaped origin binding")
	}
}
