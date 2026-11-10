package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var defaultOrigin = "https://shimo.net"

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		h = "."
	}
	return h
}

func stateDir() string {
	if runtime.GOOS == "windows" {
		if d, err := os.UserConfigDir(); err == nil && d != "" {
			return filepath.Join(d, "shimo-net-cli")
		}
	}
	return filepath.Join(homeDir(), ".shimo-net-cli")
}
func configPath() string  { return filepath.Join(stateDir(), "config.json") }
func credPath() string    { return filepath.Join(stateDir(), "credentials.json") }
func favPath() string     { return filepath.Join(stateDir(), "favorites.json") }
func profilePath() string { return filepath.Join(stateDir(), "chrome-profile") }

func normalizeOrigin(value string) (string, error) {
	s := strings.TrimSpace(value)
	if s == "" {
		return "", errors.New("产品网址不能为空")
	}
	if !regexp.MustCompile(`(?i)^https?://`).MatchString(s) {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("产品网址不合法：%s", value)
	}
	path := strings.TrimRight(u.Path, "/")
	return strings.TrimRight(strings.ToLower(u.Scheme)+"://"+strings.ToLower(u.Host)+path, "/"), nil
}

func readConfig() map[string]any {
	m := map[string]any{}
	b, err := os.ReadFile(configPath())
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	return m
}

func saveConfig(origin string) error {
	o, err := normalizeOrigin(origin)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data := readConfig()
	data["origin"] = o
	data["savedAt"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	b, _ := json.MarshalIndent(data, "", "  ")
	if err := os.WriteFile(configPath(), append(b, '\n'), 0o600); err != nil {
		return err
	}
	_ = os.Chmod(stateDir(), 0o700)
	_ = os.Chmod(configPath(), 0o600)
	return nil
}

func resolveOrigin() (string, string) {
	if env := os.Getenv("SHIMO_NET_ORIGIN"); env != "" {
		if o, err := normalizeOrigin(env); err == nil {
			return o, "env"
		}
	}
	if cfg, ok := readConfig()["origin"].(string); ok && cfg != "" {
		if o, err := normalizeOrigin(cfg); err == nil {
			return o, "config"
		}
	}
	return defaultOrigin, "default"
}

func (a *App) setOrigin(value string) error {
	o, err := normalizeOrigin(value)
	if err != nil {
		return err
	}
	if err := saveConfig(o); err != nil {
		return err
	}
	a.Origin, a.OriginSource = o, "config"
	return nil
}

func (a *App) ensureOrigin() error {
	if os.Getenv("SHIMO_NET_ORIGIN") != "" || readConfig()["origin"] != nil {
		a.Origin, a.OriginSource = resolveOrigin()
		return nil
	}
	if isTTY() && !a.JSON {
		fmt.Print("首次使用，请输入产品网址（默认 https://shimo.net）：")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			line = defaultOrigin
		}
		return a.setOrigin(line)
	}
	return nil
}

func isTTY() bool {
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
