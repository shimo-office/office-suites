package main

// Session bridging: when the CLI Keychain has no usable shimo.net session but
// the user is already signed in inside Chrome/Chromium/Edge/Brave, reuse that
// browser session instead of asking the user to log in a second time.
//
// The browser cookie database is encrypted with the browser's own Safe Storage
// key.  Reading that key with the `security` CLI is blocked by the browser's
// keychain ACL on current macOS versions, so the bridge copies the profile's
// cookie database into a throw-away profile and asks the browser itself (over
// the DevTools protocol) for the decrypted cookie jar.  The temporary profile
// is removed afterwards and the cookie value is never printed.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type browserCookie struct {
	Name    string  `json:"name"`
	Value   string  `json:"value"`
	Domain  string  `json:"domain"`
	Path    string  `json:"path"`
	Expires float64 `json:"expires"`
	Session bool    `json:"session"`
}

type browserProfile struct {
	Kind   string
	Label  string
	Root   string
	Dir    string
	Binary string
}

type browserRoot struct {
	Kind string
	Dir  string
}

var harvestMu sync.Mutex
var harvestDone bool
var harvestCookie string
var harvestErr error

func harvestDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SHIMO_NET_DISABLE_HARVEST")))
	return v == "1" || v == "true" || v == "yes"
}

// authCookie returns the best available session.  It first uses the explicit
// sources (env / Keychain / 0600 file) and only then bridges a browser login.
func authCookie() (string, error) {
	if c, err := readCookie(); err == nil && c != "" {
		return c, nil
	}
	if c, err := harvestChromeSession(false); err == nil && c != "" {
		_ = writeCred(originForHarvest(), c)
		return c, nil
	}
	return "", errors.New("未登录：本机 Keychain 没有会话，也没有找到已登录的 Chrome/Chromium；请先运行 shimo-net login")
}

// harvestChromeSession caches the result for the lifetime of the process.  A
// forced call is used after an HTTP 401 (the cached Keychain cookie may be
// stale even though Chrome has a fresh one).
func harvestChromeSession(force bool) (string, error) {
	harvestMu.Lock()
	defer harvestMu.Unlock()
	if harvestDone && !force {
		return harvestCookie, harvestErr
	}
	cookie, label, err := harvestFromBrowsers()
	harvestDone = true
	harvestCookie, harvestErr = cookie, err
	if err == nil && cookie != "" {
		fmt.Fprintf(os.Stderr, "已从 %s 复用登录态，无需二次登录\n", label)
	}
	return harvestCookie, harvestErr
}

func originForHarvest() string {
	if o, _ := resolveOrigin(); o != "" {
		return o
	}
	return defaultOrigin
}

func harvestFromBrowsers() (string, string, error) {
	if harvestDisabled() {
		return "", "", errors.New("已通过 SHIMO_NET_DISABLE_HARVEST 关闭浏览器登录态复用")
	}
	profiles := browserProfiles()
	if len(profiles) == 0 {
		return "", "", errors.New("没有找到可复用的 Chromium 浏览器配置")
	}
	host := hostOf(originForHarvest())
	var lastErr error
	for _, p := range profiles {
		cookie, err := cookiesFromProfile(p, host)
		if err != nil {
			lastErr = err
			continue
		}
		if cookie == "" {
			continue
		}
		if _, err := (&App{Origin: originForHarvest()}).usersMeWithCookie(cookie); err != nil {
			lastErr = err
			continue
		}
		return cookie, p.Label, nil
	}
	if lastErr == nil {
		lastErr = errors.New("浏览器里没有找到可用的 shimo.net 登录态")
	}
	return "", "", lastErr
}

func browserProfiles() []browserProfile {
	var out []browserProfile
	for _, root := range browserRoots() {
		bin := browserBinary(root.Kind)
		if bin == "" {
			continue
		}
		entries, err := os.ReadDir(root.Dir)
		if err != nil {
			continue
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "Default" || strings.HasPrefix(name, "Profile ") {
				if _, err := os.Stat(filepath.Join(root.Dir, name, "Cookies")); err == nil {
					names = append(names, name)
				}
			}
		}
		sort.Slice(names, func(i, j int) bool {
			if names[i] == "Default" {
				return true
			}
			if names[j] == "Default" {
				return false
			}
			return names[i] < names[j]
		})
		for _, name := range names {
			out = append(out, browserProfile{
				Kind:   root.Kind,
				Label:  fmt.Sprintf("%s (%s)", browserLabel(root.Kind), name),
				Root:   root.Dir,
				Dir:    filepath.Join(root.Dir, name),
				Binary: bin,
			})
		}
	}
	return out
}

func browserLabel(kind string) string {
	switch kind {
	case "chrome":
		return "Chrome"
	case "chromium":
		return "Chromium"
	case "edge":
		return "Microsoft Edge"
	case "brave":
		return "Brave"
	case "vivaldi":
		return "Vivaldi"
	}
	return kind
}

func browserRoots() []browserRoot {
	home := homeDir()
	switch runtime.GOOS {
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support")
		return []browserRoot{
			{"chrome", filepath.Join(base, "Google", "Chrome")},
			{"chromium", filepath.Join(base, "Chromium")},
			{"edge", filepath.Join(base, "Microsoft Edge")},
			{"brave", filepath.Join(base, "BraveSoftware", "Brave-Browser")},
			{"vivaldi", filepath.Join(base, "Vivaldi")},
		}
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		return []browserRoot{
			{"chrome", filepath.Join(base, "Google", "Chrome", "User Data")},
			{"chromium", filepath.Join(base, "Chromium", "User Data")},
			{"edge", filepath.Join(base, "Microsoft", "Edge", "User Data")},
			{"brave", filepath.Join(base, "BraveSoftware", "Brave-Browser", "User Data")},
			{"vivaldi", filepath.Join(base, "Vivaldi", "User Data")},
		}
	default:
		base := filepath.Join(home, ".config")
		return []browserRoot{
			{"chrome", filepath.Join(base, "google-chrome")},
			{"chromium", filepath.Join(base, "chromium")},
			{"edge", filepath.Join(base, "microsoft-edge")},
			{"brave", filepath.Join(base, "BraveSoftware", "Brave-Browser")},
			{"vivaldi", filepath.Join(base, "vivaldi")},
		}
	}
}

func browserBinary(kind string) string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = map[string][]string{
			"chrome":   {"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
			"chromium": {"/Applications/Chromium.app/Contents/MacOS/Chromium"},
			"edge":     {"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"},
			"brave":    {"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"},
			"vivaldi":  {"/Applications/Vivaldi.app/Contents/MacOS/Vivaldi"},
		}[kind]
	case "windows":
		base := os.Getenv("ProgramFiles")
		base86 := os.Getenv("ProgramFiles(x86)")
		local := os.Getenv("LOCALAPPDATA")
		sub := map[string]string{
			"chrome":   "Google\\Chrome\\Application\\chrome.exe",
			"chromium": "Chromium\\Application\\chrome.exe",
			"edge":     "Microsoft\\Edge\\Application\\msedge.exe",
			"brave":    "BraveSoftware\\Brave-Browser\\Application\\brave.exe",
			"vivaldi":  "Vivaldi\\Application\\vivaldi.exe",
		}[kind]
		for _, b := range []string{base, base86, local} {
			if b != "" {
				candidates = append(candidates, filepath.Join(b, sub))
			}
		}
	default:
		candidates = map[string][]string{
			"chrome":   {"/usr/bin/google-chrome", "/usr/bin/google-chrome-stable", "/opt/google/chrome/chrome"},
			"chromium": {"/usr/bin/chromium", "/usr/bin/chromium-browser", "/snap/bin/chromium"},
			"edge":     {"/usr/bin/microsoft-edge", "/usr/bin/microsoft-edge-stable"},
			"brave":    {"/usr/bin/brave-browser", "/usr/bin/brave"},
			"vivaldi":  {"/usr/bin/vivaldi"},
		}[kind]
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func cookiesFromProfile(p browserProfile, host string) (string, error) {
	tmp, err := os.MkdirTemp("", "shimo-net-harvest-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	_ = os.Chmod(tmp, 0o700)
	defaultDir := filepath.Join(tmp, "Default")
	if err := os.MkdirAll(defaultDir, 0o700); err != nil {
		return "", err
	}
	// Copy the cookie database and its journal/WAL sidecars.  Chrome keeps a
	// recently written cookie in the sidecar until it checkpoints, so a plain
	// copy of Cookies alone can miss the current session.
	for _, name := range []string{"Cookies", "Cookies-journal", "Cookies-wal", "Cookies-shm"} {
		if b, err := os.ReadFile(filepath.Join(p.Dir, name)); err == nil {
			_ = os.WriteFile(filepath.Join(defaultDir, name), b, 0o600)
		}
	}
	if b, err := os.ReadFile(filepath.Join(p.Root, "Local State")); err == nil {
		_ = os.WriteFile(filepath.Join(tmp, "Local State"), b, 0o600)
	}

	noSandbox := os.Getenv("SHIMO_NET_NO_SANDBOX") != ""
	cmd, port, err := startHarvestBrowser(p.Binary, tmp, noSandbox)
	if err != nil && !noSandbox {
		// Sandboxed agent environments (and some managed Macs) cannot start the
		// Chrome sandbox.  Retry once outside the sandbox; the profile copy is
		// throw-away and the command only reads cookies.
		cmd, port, err = startHarvestBrowser(p.Binary, tmp, true)
	}
	if err != nil {
		return "", err
	}
	defer func() {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	cookies, err := readBrowserCookies(port)
	if err != nil {
		return "", err
	}
	return cookieJarForHost(cookies, host), nil
}

func startHarvestBrowser(binary, profileDir string, noSandbox bool) (*exec.Cmd, int, error) {
	portFile := filepath.Join(profileDir, "DevToolsActivePort")
	_ = os.Remove(portFile)
	args := []string{
		"--headless=new",
		"--user-data-dir=" + profileDir,
		"--remote-debugging-port=0",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-gpu",
		"--disable-background-networking",
		"--disable-component-update",
		"--disable-sync",
		"--disable-extensions",
		"--metrics-recording-only",
		"--mute-audio",
		"about:blank",
	}
	if noSandbox {
		args = append([]string{"--no-sandbox"}, args...)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}
	port, err := waitDevToolsPort(portFile, 8*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, 0, err
	}
	return cmd, port, nil
}

func readBrowserCookies(port int) ([]browserCookie, error) {
	body, err := httpGet(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if err != nil {
		return nil, err
	}
	var targets []map[string]any
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, err
	}
	wsURL := ""
	for _, t := range targets {
		if str(t, "type") == "page" && str(t, "webSocketDebuggerUrl") != "" {
			wsURL = str(t, "webSocketDebuggerUrl")
			break
		}
	}
	if wsURL == "" {
		return nil, errors.New("浏览器没有可用的页面调试目标")
	}
	ws, err := dialWS(wsURL)
	if err != nil {
		return nil, err
	}
	defer ws.Close()
	if _, err := ws.Call(1, "Network.enable", nil, 10*time.Second); err != nil {
		return nil, err
	}
	res, err := ws.Call(2, "Network.getAllCookies", nil, 10*time.Second)
	if err != nil {
		return nil, err
	}
	arr, _ := res["cookies"].([]any)
	out := make([]browserCookie, 0, len(arr))
	for _, item := range arr {
		m, _ := item.(map[string]any)
		name := str(m, "name")
		if name == "" {
			continue
		}
		out = append(out, browserCookie{
			Name:    name,
			Value:   str(m, "value"),
			Domain:  strings.ToLower(strings.TrimPrefix(str(m, "domain"), ".")),
			Path:    str(m, "path"),
			Expires: num(m, "expires"),
			Session: boolv(m, "session"),
		})
	}
	return out, nil
}

func cookieJarForHost(cookies []browserCookie, host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	now := float64(time.Now().Unix())
	values := map[string]string{}
	for _, c := range cookies {
		if c.Name == "" || c.Value == "" {
			continue
		}
		if c.Domain != host && c.Domain != "."+host {
			continue
		}
		if !c.Session && c.Expires > 0 && c.Expires < now {
			continue
		}
		values[c.Name] = c.Value
	}
	if values["sid"] == "" {
		return ""
	}
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, n+"="+values[n])
	}
	return strings.Join(parts, "; ")
}

func (a *App) cmdSession() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net session <status|harvest> [--no-save]")
	}
	switch a.Pos[0] {
	case "status":
		out := map[string]any{
			"origin":   a.Origin,
			"env":      os.Getenv("SHIMO_NET_COOKIE") != "",
			"keychain": keychainAvailable() && keychainLoad() != "",
			"file":     readCredFile() != nil && readCredFile().Cookie != "",
			"harvest":  !harvestDisabled(),
			"profiles": len(browserProfiles()),
		}
		if a.JSON {
			return a.printJSON(out)
		}
		fmt.Printf("产品网址：%s\n", a.Origin)
		fmt.Printf("显式会话：环境变量=%v Keychain=%v 文件=%v\n", out["env"], out["keychain"], out["file"])
		fmt.Printf("浏览器复用：%v（发现 %v 个浏览器配置）\n", out["harvest"], out["profiles"])
		return nil
	case "harvest":
		harvestMu.Lock()
		harvestDone = false
		harvestMu.Unlock()
		cookie, err := harvestChromeSession(true)
		if err != nil {
			return err
		}
		me, err := (&App{Origin: a.Origin}).usersMeWithCookie(cookie)
		if err != nil {
			return err
		}
		if !a.has("--no-save") {
			if err := writeCred(a.Origin, cookie); err != nil {
				return err
			}
		}
		out := map[string]any{"ok": true, "source": "browser", "saved": !a.has("--no-save"),
			"cookieCount": len(strings.Split(cookie, "; ")), "account": map[string]any{
				"id": str(me, "id"), "name": str(me, "name"), "email": str(me, "email")}}
		if a.JSON {
			return a.printJSON(out)
		}
		fmt.Printf("已复用浏览器登录态：%s <%s>（%d 个 cookie）\n", str(me, "name"), str(me, "email"), out["cookieCount"])
		if !a.has("--no-save") {
			fmt.Println("已写入本机 Keychain，后续命令可直接使用")
		}
		return nil
	default:
		return fmt.Errorf("未知 session 子命令：%s", a.Pos[0])
	}
}

// setCookieDomain returns the cookie domain used for CDP injection.  A leading
// dot makes the session available on the main host and its subdomains.
func cookieDomainFor(target string) string {
	u, err := url.Parse(target)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return "." + u.Hostname()
}
