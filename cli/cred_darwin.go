//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

func keychainAvailable() bool {
	_, err := exec.LookPath("security")
	return err == nil
}

func keychainSave(account, cookie string) error {
	return exec.Command("security", "add-generic-password", "-U", "-s", keychainService, "-a", account, "-w", cookie).Run()
}

func keychainLoad() string {
	out, err := exec.Command("security", "find-generic-password", "-s", keychainService, "-w").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func keychainClear() {
	_ = exec.Command("security", "delete-generic-password", "-s", keychainService).Run()
}
