package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

func loadFav() map[string]map[string]any {
	out := map[string]map[string]any{}
	b, err := os.ReadFile(favPath())
	if err != nil {
		return out
	}
	var doc struct {
		Aliases map[string]map[string]any `json:"aliases"`
	}
	if json.Unmarshal(b, &doc) == nil && doc.Aliases != nil {
		return doc.Aliases
	}
	return out
}

func saveFav(aliases map[string]map[string]any) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(map[string]any{"aliases": aliases}, "", "  ")
	return os.WriteFile(favPath(), append(b, '\n'), 0o600)
}

func ensureAlias(name string) (string, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "@")
	if name == "" {
		return "", errors.New("别名不能为空")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_\p{Han}-]{1,40}$`).MatchString(name) {
		return "", errors.New("别名只能用中英文、数字、下划线、短横线，最长 40 字")
	}
	return name, nil
}

func (a *App) cmdFav() error {
	sub := "list"
	if len(a.Pos) > 0 {
		sub = a.Pos[0]
	}
	aliases := loadFav()
	switch sub {
	case "list", "ls":
		if a.JSON {
			return a.printJSON(map[string]any{"count": len(aliases), "aliases": aliases})
		}
		if len(aliases) == 0 {
			fmt.Println("还没有常用位置。加一个：shimo-net fav add <别名> <链接> [--note 特征]")
			return nil
		}
		fmt.Printf("常用位置（%d 个，命令里用 @别名）：\n\n", len(aliases))
		for k, v := range aliases {
			fmt.Printf("@%s  %s\n     %s\n", k, str(v, "name"), str(v, "url"))
			if str(v, "note") != "" {
				fmt.Printf("     特征：%s\n", str(v, "note"))
			}
		}
		return nil
	case "add":
		if len(a.Pos) < 3 {
			return errors.New("用法：shimo-net fav add <别名> <链接|guid> [--note 特征]")
		}
		name, err := ensureAlias(a.Pos[1])
		if err != nil {
			return err
		}
		guid, err := a.resolveTarget(a.Pos[2])
		if err != nil {
			return err
		}
		info, err := a.fileInfo(guid)
		if err != nil {
			return err
		}
		kind := str(info, "type")
		if boolv(info, "isSpace") {
			kind = "space"
		} else if boolv(info, "isFolder") {
			kind = "folder"
		}
		aliases[name] = map[string]any{"name": str(info, "name"), "url": a.urlOf(withGUID(info, guid)),
			"note": a.opt("--note"), "kind": kind, "savedAt": time.Now().Format("2006-01-02T15:04:05")}
		if err := saveFav(aliases); err != nil {
			return err
		}
		if a.JSON {
			out := map[string]any{"ok": true, "alias": "@" + name}
			for k, v := range aliases[name] {
				out[k] = v
			}
			return a.printJSON(out)
		}
		fmt.Printf("已记住 @%s → %s\n", name, str(aliases[name], "url"))
		if str(aliases[name], "note") != "" {
			fmt.Printf("特征：%s\n", str(aliases[name], "note"))
		}
		fmt.Printf("下次直接：shimo-net ls @%s\n", name)
		return nil
	case "rm", "remove", "del":
		if len(a.Pos) < 2 {
			return errors.New("用法：shimo-net fav rm <别名>")
		}
		name := strings.TrimPrefix(a.Pos[1], "@")
		if _, ok := aliases[name]; !ok {
			return errors.New("没有这个别名：" + name)
		}
		delete(aliases, name)
		if err := saveFav(aliases); err != nil {
			return err
		}
		if a.JSON {
			return a.printJSON(map[string]any{"ok": true, "removed": "@" + name})
		}
		fmt.Printf("已删除 @%s\n", name)
		return nil
	case "show":
		if len(a.Pos) < 2 {
			return errors.New("用法：shimo-net fav show <别名>")
		}
		name := strings.TrimPrefix(a.Pos[1], "@")
		v, ok := aliases[name]
		if !ok {
			return errors.New("没有这个别名：" + name)
		}
		if a.JSON {
			return a.printJSON(v)
		}
		fmt.Printf("@%s  %s\n", name, str(v, "url"))
		if str(v, "name") != "" {
			fmt.Printf("名称：%s\n", str(v, "name"))
		}
		if str(v, "note") != "" {
			fmt.Printf("特征：%s\n", str(v, "note"))
		}
		return nil
	default:
		return errors.New("用法：shimo-net fav <list|add|rm|show> …")
	}
}

func (a *App) cmdOpen() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net open <链接|guid|@别名> [--default]")
	}
	raw := a.Pos[0]
	target := raw
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		guid, err := a.resolveTarget(raw)
		if err != nil {
			return err
		}
		if guid == "Desktop" {
			return errors.New("桌面没有可打开的链接")
		}
		info, err := a.fileInfo(guid)
		if err != nil {
			return err
		}
		target = a.urlOf(withGUID(info, guid))
	}
	// Resolve the session before launching anything, so opening a browser does
	// not show a login page and then reload after the cookie is injected.
	_, _ = authCookie()
	if a.has("--default") {
		if bin, label := defaultChromiumBinary(); bin != "" {
			if err := os.MkdirAll(profilePath(), 0o700); err == nil {
				portFile := filepath.Join(profilePath(), "DevToolsActivePort")
				_ = os.Remove(portFile)
				cmd := exec.Command(bin, chromeLaunchArgs("--no-first-run", "--no-default-browser-check", "--user-data-dir="+profilePath(), "--remote-debugging-port=0", target)...)
				cmd.Stdout, cmd.Stderr = nil, nil
				if err := cmd.Start(); err == nil {
					injected := injectManagedCookie(target, portFile)
					o := map[string]any{"ok": true, "url": target, "mode": "default", "browser": label,
						"launcher": bin, "profile": "managed", "sessionInjected": injected,
						"note": "系统默认浏览器 + CLI 专用配置，已注入 Keychain 会话，无需二次登录"}
					if a.JSON {
						return a.printJSON(o)
					}
					if injected {
						fmt.Printf("已用系统默认浏览器（%s，已注入登录态）打开：%s\n", label, target)
					} else {
						fmt.Printf("已用系统默认浏览器（%s）打开：%s（未注入会话，可能需要登录）\n", label, target)
					}
					return nil
				}
			}
		}
		cmd, launcher, err := defaultBrowserCommand(target)
		if err != nil {
			return err
		}
		cmd.Stdout, cmd.Stderr = nil, nil
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("启动默认浏览器失败：%w", err)
		}
		if a.JSON {
			return a.printJSON(map[string]any{
				"ok": true, "url": target, "mode": "default", "launcher": launcher, "sessionInjected": false,
				"note": "该默认浏览器不是 Chromium 内核，无法安全注入 cookie；如未登录请在页面内登录一次",
			})
		}
		fmt.Printf("已用系统默认浏览器打开：%s\n", target)
		fmt.Println("提示：该浏览器不是 Chromium 内核，无法注入登录态；如未登录请在页面内登录一次，或使用 shimo-net open（专用配置已注入）。")
		return nil
	}

	chrome, err := findChrome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(profilePath(), 0o700); err != nil {
		return err
	}
	portFile := filepath.Join(profilePath(), "DevToolsActivePort")
	_ = os.Remove(portFile)
	cmd := exec.Command(chrome, chromeLaunchArgs("--no-first-run", "--no-default-browser-check", "--user-data-dir="+profilePath(), "--remote-debugging-port=0", target)...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	injected := injectManagedCookie(target, portFile)
	o := map[string]any{"ok": true, "url": target, "profile": profilePath(), "sessionInjected": injected,
		"note": "用 CLI 专用浏览器打开；已从 Keychain 注入会话时无需重新登录"}
	if a.JSON {
		return a.printJSON(o)
	}
	fmt.Printf("已用登录态浏览器打开：%s\n", target)
	return nil
}

// injectManagedCookie upgrades the CLI-managed browser launch into an
// authenticated page when a Keychain/file session is available. The browser
// process is intentionally left running for the user or agent to operate.
// A failure to attach CDP is non-fatal: the page remains open and the caller
// can use its own browser login flow.
func injectManagedCookie(target, portFile string) bool {
	cookie, err := authCookie()
	if err != nil || !sameCookieHost(target) {
		return false
	}
	port, err := waitDevToolsPort(portFile, 10*time.Second)
	if err != nil {
		return false
	}
	wsURL, err := waitPageTarget(port, target)
	if err != nil {
		return false
	}
	ws, err := dialWS(wsURL)
	if err != nil {
		return false
	}
	defer ws.Close()
	if _, err := ws.Call(1, "Network.enable", nil, 5*time.Second); err != nil {
		return false
	}
	if _, err := ws.Call(2, "Page.enable", nil, 5*time.Second); err != nil {
		return false
	}
	if err := setCookies(ws, target, cookie); err != nil {
		return false
	}
	_, _ = ws.Call(3, "Page.reload", map[string]any{"ignoreCache": true}, 10*time.Second)
	time.Sleep(1200 * time.Millisecond)
	return true
}

// defaultChromiumBinary returns the system default browser when it is a
// Chromium-family browser that can accept a DevTools session injection.
func defaultChromiumBinary() (string, string) {
	switch runtime.GOOS {
	case "darwin":
		switch defaultBrowserBundleID() {
		case "com.google.chrome":
			return browserBinary("chrome"), "Chrome"
		case "com.microsoft.edgemac":
			return browserBinary("edge"), "Microsoft Edge"
		case "com.brave.browser":
			return browserBinary("brave"), "Brave"
		case "org.chromium.chromium":
			return browserBinary("chromium"), "Chromium"
		case "com.vivaldi.vivaldi":
			return browserBinary("vivaldi"), "Vivaldi"
		}
	case "linux":
		out, _ := exec.Command("xdg-settings", "get", "default-web-browser").Output()
		s := strings.ToLower(string(out))
		switch {
		case strings.Contains(s, "brave"):
			return browserBinary("brave"), "Brave"
		case strings.Contains(s, "edge"):
			return browserBinary("edge"), "Microsoft Edge"
		case strings.Contains(s, "chromium"):
			return browserBinary("chromium"), "Chromium"
		case strings.Contains(s, "chrome"):
			return browserBinary("chrome"), "Chrome"
		}
	}
	return "", ""
}

func defaultBrowserBundleID() string {
	out, err := exec.Command("defaults", "read", "com.apple.LaunchServices/com.apple.launchservices.secure", "LSHandlers").Output()
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`LSHandlerRoleAll\s*=\s*"?([^";]+)"?`)
	for _, block := range strings.Split(string(out), "}") {
		if !strings.Contains(block, "https") {
			continue
		}
		if m := re.FindStringSubmatch(block); len(m) == 2 {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func defaultBrowserCommand(target string) (*exec.Cmd, string, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target), "open", nil
	case "linux":
		return exec.Command("xdg-open", target), "xdg-open", nil
	case "windows":
		return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", target), "rundll32.exe", nil
	default:
		return nil, "", fmt.Errorf("当前操作系统不支持默认浏览器打开：%s", runtime.GOOS)
	}
}

func (a *App) cmdPurge() error {
	return manualOnlyError("彻底删除")
}
