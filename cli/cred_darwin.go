//go:build darwin

package main

import (
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"strings"
)

func keychainAvailable() bool {
	st, err := os.Stat("/usr/bin/security")
	return err == nil && !st.IsDir()
}

// security -i reads commands from stdin. The password is passed as hex data
// there, never as an argv argument or environment variable. Hex also prevents
// command/newline injection through arbitrary cookie bytes. Keep the other
// interactive tokens strictly bounded rather than relying on shell quoting.
func keychainSaveCommand(service, account, cookie string) (*exec.Cmd, error) {
	for _, token := range []string{service, account} {
		if token == "" {
			return nil, errors.New("钥匙串标识不能为空")
		}
		for _, r := range token {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:/-", r)) {
				return nil, errors.New("钥匙串标识包含不支持的字符")
			}
		}
	}
	cmd := exec.Command("/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader("add-generic-password -U -s " + service + " -a " + account + " -X " + hex.EncodeToString([]byte(cookie)) + "\n")
	return cmd, nil
}

func keychainSave(service, account, cookie string) error {
	cmd, err := keychainSaveCommand(service, account, cookie)
	if err != nil {
		return err
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	if keychainLoad(service) != cookie {
		return errors.New("钥匙串写入校验失败")
	}
	return nil
}

func keychainLoad(service string) string {
	out, err := exec.Command("/usr/bin/security", "find-generic-password", "-s", service, "-w").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func keychainClear(service string) {
	_ = exec.Command("/usr/bin/security", "delete-generic-password", "-s", service).Run()
}
