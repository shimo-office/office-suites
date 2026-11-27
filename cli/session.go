package main

// Session bridging: automatic reuse is allowed only when no credential record
// exists. Expired, unreadable or origin-mismatched records require explicit login;
// explicit session harvest can select a Chrome/Chromium/Edge/Brave/Vivaldi account.
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
	Secure  bool    `json:"secure"`
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
var harvestOrigin string
var harvestErr error

func harvestDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SHIMO_NET_DISABLE_HARVEST")))
	return v == "1" || v == "true" || v == "yes"
}

// Only the absence of stored credentials permits automatic browser discovery.
// An expired, unreadable or origin-mismatched record requires explicit login.
func authCookieForOrigin(origin string) (string, error) {
	return authCookieForURL(origin, nil)
}

func authCookieForURL(origin string, u *url.URL) (string, error) {
	c, err := readCookieForURL(origin, u)
	if err == nil && c != "" {
		return c, nil
	}
	if !errors.Is(err, errNoStoredCredential) {
		return "", err
	}
	if explicitNoBrowser(&App{}) {
		return "", errors.New("未登录且当前设置禁止启动浏览器；未尝试复用浏览器会话")
	}
	// 抓取前取 revision：抓取期间若有人登录或退出，这次镜像会撞版本而不是覆盖。
	baseRev := sessionRevision(origin)
	if c, err := harvestChromeSessionForOrigin(origin, false); err == nil && c != "" {
		if err := writeCred(origin, c); err != nil {
			return "", err
		}
		warnSharedMirror(mirrorSession(origin, c, "harvest", nil, baseRev))
		return c, nil
	}
	return "", errors.New("未登录：没有已存凭据或可复用的浏览器会话；请运行 shimo-net login")
}

func authCookie() (string, error) {
	return authCookieForOrigin(originForHarvest())
}

func (a *App) requestCookie() (string, error) {
	return a.requestCookieFor(nil)
}

// requestCookieFor 解析具体请求要用的 cookie。共享会话按该地址的 Secure/Path
// 过滤；env、文件、钥匙串没有属性可判，返回整串。
func (a *App) requestCookieFor(u *url.URL) (string, error) {
	if explicitNoBrowser(a) {
		return readCookieForURL(a.Origin, u)
	}
	return authCookieForURL(a.Origin, u)
}

func resetHarvestCache() {
	harvestMu.Lock()
	defer harvestMu.Unlock()
	harvestDone = false
	harvestCookie = ""
	harvestOrigin = ""
	harvestErr = nil
}

// Results are cached only for the same origin. Forced discovery is reserved
// for explicit login/session harvest, never an automatic response to HTTP 401.
func harvestChromeSessionForOrigin(origin string, force bool) (string, error) {
	if explicitNoBrowser(&App{}) {
		return "", errors.New("当前设置禁止启动浏览器；本次未读取或保存浏览器会话")
	}
	if _, err := canonicalCredentialOrigin(origin); err != nil {
		return "", err
	}
	if harvestDisabled() {
		return "", errors.New("已通过 SHIMO_NET_DISABLE_HARVEST 关闭浏览器登录态复用")
	}
	harvestMu.Lock()
	defer harvestMu.Unlock()
	if harvestDone && !force && sameCredentialOrigin(harvestOrigin, origin) {
		return harvestCookie, harvestErr
	}
	cookie, label, err := harvestFromBrowsers(origin)
	harvestDone = true
	harvestOrigin = origin
	harvestCookie, harvestErr = cookie, err
	if err == nil && cookie != "" {
		fmt.Fprintf(os.Stderr, "已从 %s 复用登录态，无需二次登录\n", terminalText(label))
	}
	return harvestCookie, harvestErr
}

func originForHarvest() string {
	if o, _ := resolveOrigin(); o != "" {
		return o
	}
	return defaultOrigin
}

func harvestFromBrowsers(origin string) (string, string, error) {
	if explicitNoBrowser(&App{}) {
		return "", "", errors.New("当前设置禁止启动浏览器；本次未读取或保存浏览器会话")
	}
	if harvestDisabled() {
		return "", "", errors.New("已通过 SHIMO_NET_DISABLE_HARVEST 关闭浏览器登录态复用")
	}
	profiles := browserProfiles()
	if len(profiles) == 0 {
		return "", "", errors.New("没有找到可复用的 Chromium 浏览器配置")
	}
	var lastErr error
	for _, p := range profiles {
		cookie, err := cookiesFromProfile(p, origin)
		if err != nil {
			lastErr = err
			continue
		}
		if cookie == "" {
			continue
		}
		if _, err := (&App{Origin: origin}).usersMeWithCookie(cookie); err != nil {
			lastErr = err
			continue
		}
		return cookie, p.Label, nil
	}
	if lastErr == nil {
		lastErr = errors.New("浏览器里没有找到目标网址的可用登录态")
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

func browserCandidates(system, kind string) []string {
	var candidates []string
	switch system {
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
		if sub == "" {
			return nil
		}
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
	return candidates
}

func browserBinary(kind string) string {
	for _, p := range browserCandidates(runtime.GOOS, kind) {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func cookiesFromProfile(p browserProfile, origin string) (string, error) {
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

	noSandbox := browserSandboxDisabled()
	cmd, port, logPath, cleanupLog, err := startHarvestBrowser(p.Binary, tmp, noSandbox)
	defer cleanupLog()
	if err != nil {
		return "", explainBrowserFailure(err, logPath)
	}
	defer func() {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	cookies, err := readBrowserCookies(port)
	if err != nil {
		return "", explainBrowserFailure(err, logPath)
	}
	return cookieJarForOrigin(cookies, origin), nil
}

func startHarvestBrowser(binary, profileDir string, noSandbox bool) (*exec.Cmd, int, string, func(), error) {
	portFile := filepath.Join(profileDir, "DevToolsActivePort")
	_ = os.Remove(portFile)
	args := []string{
		"--headless=new",
		"--user-data-dir=" + profileDir,
		"--remote-debugging-port=0",
		"--remote-debugging-address=127.0.0.1",
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
	// 浏览器把沙箱初始化、渲染进程崩溃等原因写在 stderr 上；留档才能在失败时给出结论。
	logPath, cleanupLog := captureBrowserOutput(cmd)
	if err := cmd.Start(); err != nil {
		return nil, 0, logPath, cleanupLog, err
	}
	port, err := waitDevToolsPort(portFile, 8*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, 0, logPath, cleanupLog, err
	}
	return cmd, port, logPath, cleanupLog, nil
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
			Secure:  boolv(m, "secure"),
		})
	}
	return out, nil
}

// Browser cookie metadata has no port binding. Honor its Secure bit and use
// exact host matching; persisted credentials are additionally bound to origin.
func cookieJarForOrigin(cookies []browserCookie, origin string) string {
	canonical, err := canonicalCredentialOrigin(origin)
	if err != nil {
		return ""
	}
	filtered := make([]browserCookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Secure && !strings.HasPrefix(canonical, "https://") {
			continue
		}
		filtered = append(filtered, cookie)
	}
	return cookieJarForHost(filtered, hostOf(origin))
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
		if strings.ToLower(strings.TrimPrefix(c.Domain, ".")) != host {
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
		return errors.New("用法：shimo-net session <status|harvest|import|export|watch|logout> [--no-save] [--out 文件]")
	}
	switch a.Pos[0] {
	case "import":
		return a.cmdSessionImport()
	case "export":
		return a.cmdSessionExport()
	case "watch":
		return a.cmdSessionWatch()
	case "logout":
		return a.cmdSessionLogout()
	case "status":
		_, source, _ := credentialForOrigin(a.Origin)
		out := map[string]any{
			"origin":   a.Origin,
			"env":      os.Getenv("SHIMO_NET_COOKIE") != "",
			"keychain": source == "keychain",
			"file":     source == "file",
			"harvest":  !harvestDisabled() && !explicitNoBrowser(a),
			"profiles": len(browserProfiles()),
		}
		shared := sessionStatusMap(a.Origin)
		for k, v := range shared {
			if k != "origin" {
				out[k] = v
			}
		}
		out["bridge"] = map[string]any{
			"inbound":  "宿主已有登录态时：shimo-net session import 读 stdin（cookie 头或 cookies 数组）",
			"outbound": "宿主需要在自身浏览器打开编辑页时：shimo-net session export --out <文件>，由宿主注入同一 origin 的 cookie",
			"preview":  "宿主内置浏览器不共享 CLI 会话；不要把「内置预览显示登录页」当成 CLI 登录失效",
		}
		if a.JSON {
			return a.printJSON(out)
		}
		fmt.Printf("产品网址：%s\n", terminalText(a.Origin))
		fmt.Printf("显式会话：环境变量=%v Keychain=%v 文件=%v 共享会话=%v\n", out["env"], out["keychain"], out["file"], out["shared"])
		if boolv(out, "shared") {
			if acct, ok := out["account"].(map[string]any); ok {
				fmt.Printf("共享账号：%s <%s>（revision %v，%v 个 cookie，%v）\n", terminalText(str(acct, "name")), terminalText(str(acct, "email")), out["revision"], out["cookieCount"], out["authState"])
			}
		}
		fmt.Printf("浏览器复用：%v（发现 %v 个浏览器配置）\n", out["harvest"], out["profiles"])
		return nil
	case "harvest":
		if explicitNoBrowser(a) {
			return errors.New("当前设置禁止启动浏览器；本次未读取或保存浏览器会话")
		}
		if os.Getenv("SHIMO_NET_COOKIE") != "" && !a.has("--no-save") {
			return errors.New("SHIMO_NET_COOKIE 正在覆盖本机账号；请先取消环境变量再保存浏览器会话，或用 --no-save 仅验证")
		}
		resetHarvestCache()
		baseRev := sessionRevision(a.Origin)
		cookie, err := harvestChromeSessionForOrigin(a.Origin, true)
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
			warnSharedMirror(mirrorSession(a.Origin, cookie, "harvest", me, baseRev))
		}
		out := map[string]any{"ok": true, "source": "browser", "saved": !a.has("--no-save"),
			"storage": "none", "cookieCount": len(strings.Split(cookie, "; ")), "account": map[string]any{
				"id": str(me, "id"), "name": str(me, "name"), "email": str(me, "email")}}
		if !a.has("--no-save") {
			c := readCredFile()
			if c == nil {
				return errors.New("保存后无法读取凭据存储元数据")
			}
			out["storage"] = c.Storage
		}
		if a.JSON {
			return a.printJSON(out)
		}
		fmt.Printf("已复用浏览器登录态：%s <%s>（%d 个 cookie）\n", terminalText(str(me, "name")), terminalText(str(me, "email")), out["cookieCount"])
		if !a.has("--no-save") {
			fmt.Printf("已写入%s，后续命令可直接使用\n", credentialStorageLabel(out["storage"].(string)))
		}
		return nil
	default:
		return fmt.Errorf("未知 session 子命令：%s", a.Pos[0])
	}
}

func credentialStorageLabel(storage string) string {
	if storage == "keychain" {
		if runtime.GOOS == "windows" {
			return "Windows Credential Manager"
		}
		return "macOS Keychain"
	}
	if storage == "file" {
		return "本机凭据文件（0600）"
	}
	return "未知凭据存储"
}
