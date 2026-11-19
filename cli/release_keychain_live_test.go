//go:build darwin

package main

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// Explicit opt-in: uses a unique synthetic item, never the CLI's saved account.
func TestReleaseNativeKeychainRoundTrip(t *testing.T) {
	if os.Getenv("SHIMO_NATIVE_KEYCHAIN_TEST") != "1" {
		t.Skip("opt-in native Keychain acceptance with synthetic data")
	}
	service := fmt.Sprintf("shimo-net-acceptance-%d-%d", os.Getpid(), time.Now().UnixNano())
	if keychainLoad(service) != "" {
		t.Fatal("test service unexpectedly exists; refusing to overwrite")
	}
	t.Cleanup(func() {
		keychainClear(service)
		if keychainLoad(service) != "" {
			t.Error("synthetic Keychain item was not removed")
		}
	})
	for _, value := range []string{"sid=synthetic-release-test; marker=first", "sid=synthetic-release-test; marker=second-\"quoted\""} {
		if err := keychainSave(service, "acceptance-only", value); err != nil {
			t.Fatalf("native save/readback failed: %v", err)
		}
		if keychainLoad(service) != value {
			t.Fatal("native readback differs from synthetic value")
		}
	}
}
