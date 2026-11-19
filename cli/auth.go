package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const defaultKeychainService = "shimo-net-cli"

var keychainService = func() string {
	if v := strings.TrimSpace(os.Getenv("SHIMO_NET_KEYCHAIN_SERVICE")); v != "" {
		return v
	}
	return defaultKeychainService
}()

type Cred struct {
	Origin          string `json:"origin,omitempty"`
	Cookie          string `json:"cookie,omitempty"`
	SavedAt         string `json:"savedAt,omitempty"`
	Storage         string `json:"storage,omitempty"`
	KeychainService string `json:"keychainService,omitempty"`
}

var errNoStoredCredential = errors.New("未登录：请先运行 shimo-net login")

// Bind credentials to the web origin, including scheme and effective port.
// URL paths are irrelevant to the authority, but userinfo is never accepted.
func canonicalCredentialOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("凭据网址不合法；请检查配置后重新登录")
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("凭据网址端口不合法")
		}
		port = strconv.Itoa(n)
	}
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return strings.ToLower(u.Scheme) + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port), nil
}

func sameCredentialOrigin(left, right string) bool {
	a, err := canonicalCredentialOrigin(left)
	if err != nil {
		return false
	}
	b, err := canonicalCredentialOrigin(right)
	return err == nil && a == b
}

func credentialServiceFor(origin string) (string, error) {
	canonical, err := canonicalCredentialOrigin(origin)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%x", keychainService, sha256.Sum256([]byte(canonical))), nil
}

func credSource() string {
	origin, _ := resolveOrigin()
	_, source, _ := credentialForOrigin(origin)
	return source
}

func readCredFile() *Cred {
	b, err := os.ReadFile(credPath())
	if err != nil {
		return nil
	}
	var c Cred
	if json.Unmarshal(b, &c) != nil {
		return nil
	}
	return &c
}

func credentialForOrigin(origin string) (string, string, error) {
	if _, err := canonicalCredentialOrigin(origin); err != nil {
		return "", "", err
	}
	// This override is explicitly supplied for the current command/origin.
	if c := os.Getenv("SHIMO_NET_COOKIE"); c != "" {
		return c, "env", nil
	}
	c := readCredFile()
	if c == nil {
		if _, err := os.Stat(credPath()); err == nil || !errors.Is(err, os.ErrNotExist) {
			return "", "", errors.New("本机凭据记录不可读；请显式重新登录，不自动切换浏览器账号")
		}
		// Unscoped legacy Keychain entries without origin metadata are not safe.
		return "", "", errNoStoredCredential
	}
	if !sameCredentialOrigin(c.Origin, origin) {
		return "", "", errors.New("已存凭据与当前网址不匹配；拒绝发送，请为当前网址运行 shimo-net login")
	}
	if c.Cookie != "" {
		return c.Cookie, "file", nil
	}
	service, err := credentialServiceFor(origin)
	if err != nil {
		return "", "", err
	}
	if c.KeychainService == "" {
		service = keychainService
	} else if c.KeychainService != service {
		return "", "", errors.New("凭据存储与当前网址不匹配，请重新登录")
	}
	if c.Storage == "keychain" && keychainAvailable() {
		if cookie := keychainLoad(service); cookie != "" {
			return cookie, "keychain", nil
		}
	}
	return "", "", errors.New("已存会话不可用；请显式重新登录，不自动切换浏览器账号")
}

func readCookieForOrigin(origin string) (string, error) {
	cookie, _, err := credentialForOrigin(origin)
	return cookie, err
}

func readCookie() (string, error) {
	origin, _ := resolveOrigin()
	return readCookieForOrigin(origin)
}

func writeCred(origin, cookie string) error {
	if strings.TrimSpace(cookie) == "" {
		return errors.New("不能保存空会话")
	}
	service, err := credentialServiceFor(origin)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	c := Cred{Origin: origin, SavedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z")}
	if keychainAvailable() {
		if err := keychainSave(service, "shimo-net", cookie); err == nil {
			c.Storage = "keychain"
			c.KeychainService = service
		}
	}
	if c.Storage == "" {
		c.Cookie = cookie
		c.Storage = "file"
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.Chmod(stateDir(), 0o700); err != nil {
		return err
	}
	// Replace atomically so an existing permissive file or symlink cannot expose
	// a newly written cookie before chmod, or leave a partial credential record.
	f, err := os.CreateTemp(stateDir(), ".credentials-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), credPath())
}

func clearCred() {
	c := readCredFile()
	if c != nil && c.Storage == "keychain" && keychainAvailable() {
		if service, err := credentialServiceFor(c.Origin); err == nil {
			keychainClear(service)
			if c.KeychainService == "" {
				keychainClear(keychainService)
			}
		}
	}
	_ = os.Remove(credPath())
}

func newLoginProfile() (string, error) {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return "", err
	}
	return os.MkdirTemp(stateDir(), "login-")
}

func browserSandboxDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SHIMO_NET_NO_SANDBOX")))
	return v == "1" || v == "true" || v == "yes"
}

func chromeLaunchArgs(extra ...string) []string {
	args := []string{}
	if browserSandboxDisabled() {
		args = append(args, "--no-sandbox", "--disable-gpu")
	}
	return append(args, extra...)
}

func findChrome() (string, error) {
	paths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"/usr/bin/google-chrome", "/usr/bin/google-chrome-stable",
		"/usr/bin/chromium", "/usr/bin/chromium-browser", "/snap/bin/chromium",
	}
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
			if base == "" {
				continue
			}
			paths = append(paths,
				filepath.Join(base, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`))
		}
	}
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("没找到 Chrome/Chromium/Edge，请先安装 Google Chrome")
}

func (a *App) cmdLogin() error {
	if os.Getenv("SHIMO_NET_COOKIE") != "" {
		return errors.New("SHIMO_NET_COOKIE 正在覆盖本机账号；请先取消该环境变量再登录/切换账号，或用 whoami 核验环境会话")
	}
	if v := a.opt("--origin"); v != "" {
		if err := a.setOrigin(v); err != nil {
			return err
		}
	}
	if err := a.ensureOrigin(); err != nil {
		return err
	}
	if err := saveConfig(a.Origin); err != nil {
		return err
	}
	// If the user is already signed in inside Chrome, reuse that session
	// instead of opening a second login window.  `login --new` deliberately
	// skips the bridge so the user can switch accounts.
	if !a.has("--new") && readCredFile() == nil {
		if cookie, err := harvestChromeSessionForOrigin(a.Origin, true); err == nil && cookie != "" {
			if me, err := a.usersMeWithCookie(cookie); err == nil {
				if err := writeCred(a.Origin, cookie); err != nil {
					return err
				}
				fmt.Printf("已复用本机浏览器登录态：%s <%s>\n", str(me, "name"), str(me, "email"))
				fmt.Println("如需切换账号，请先运行 shimo-net logout，再运行 shimo-net login --new")
				return nil
			}
		}
	}
	chrome, err := findChrome()
	if err != nil {
		return err
	}
	loginProfile, err := newLoginProfile()
	if err != nil {
		return err
	}
	defer os.RemoveAll(loginProfile)
	portFile := filepath.Join(loginProfile, "DevToolsActivePort")
	_ = os.Remove(portFile)
	args := chromeLaunchArgs("--no-first-run", "--no-default-browser-check",
		"--user-data-dir="+loginProfile, "--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1",
		"--app="+a.Origin+"/login")
	fmt.Printf("产品网址：%s\n", a.Origin)
	fmt.Printf("正在打开 Chromium 浏览器，请在窗口里用你自己的账号登录 %s…\n", a.Origin)
	cmd := exec.Command(chrome, args...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动浏览器失败：%w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	port := 0
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(portFile)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			if len(lines) > 0 {
				fmt.Sscanf(strings.TrimSpace(lines[0]), "%d", &port)
				if port > 0 {
					break
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	if port == 0 {
		return errors.New("浏览器没有启动调试端口")
	}
	target, err := waitPageTarget(port, a.Origin)
	if err != nil {
		return err
	}
	ws, err := dialWS(target)
	if err != nil {
		return err
	}
	defer ws.Close()
	if _, err := ws.Call(1, "Network.enable", nil, 10*time.Second); err != nil {
		return err
	}
	loginDeadline := time.Now().Add(10 * time.Minute)
	id := 2
	for time.Now().Before(loginDeadline) {
		res, err := ws.Call(id, "Network.getAllCookies", nil, 8*time.Second)
		id++
		if err == nil {
			cookies := cookiesForOrigin(res, a.Origin)
			if len(cookies) > 0 {
				cookie := strings.Join(cookies, "; ")
				if me, err := a.usersMeWithCookie(cookie); err == nil {
					if err := writeCred(a.Origin, cookie); err != nil {
						return err
					}
					fmt.Printf("登录成功：%s <%s>\n", str(me, "name"), str(me, "email"))
					return nil
				}
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}
	return errors.New("等待登录超时（10 分钟）")
}

func pageTargetMatchesOrigin(target, origin string) bool {
	// The phase3 bootstrap connects to its own empty page before navigation.
	// It carries no credentials and must match exactly, never by substring.
	if origin == "about:blank" {
		return target == "about:blank"
	}
	return sameCredentialOrigin(target, origin)
}

func waitPageTarget(port int, origin string) (string, error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var targets []map[string]any
		body, err := httpGet(fmt.Sprintf("http://127.0.0.1:%d/json", port))
		if err == nil {
			_ = json.Unmarshal(body, &targets)
			for _, t := range targets {
				if str(t, "type") == "page" && pageTargetMatchesOrigin(str(t, "url"), origin) {
					if u := str(t, "webSocketDebuggerUrl"); u != "" {
						return u, nil
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", errors.New("没找到登录页面（可能窗口被关闭）")
}

func cookiesForOrigin(res map[string]any, origin string) []string {
	arr, _ := res["cookies"].([]any)
	cookies := make([]browserCookie, 0, len(arr))
	for _, item := range arr {
		c, _ := item.(map[string]any)
		cookies = append(cookies, browserCookie{Name: str(c, "name"), Value: str(c, "value"), Domain: str(c, "domain"), Path: str(c, "path"), Expires: num(c, "expires"), Session: boolv(c, "session"), Secure: boolv(c, "secure")})
	}
	cookie := cookieJarForOrigin(cookies, origin)
	if cookie == "" {
		return nil
	}
	return strings.Split(cookie, "; ")
}

func hostOf(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func (a *App) cmdLogout() error {
	clearCred()
	if err := os.RemoveAll(profilePath()); err != nil {
		return fmt.Errorf("凭据已删除，但专用浏览器配置清理失败：%w", err)
	}
	resetHarvestCache()
	fmt.Println("已删除本机凭据及旧版 CLI 登录浏览器配置；已打开的浏览器账号不变")
	return nil
}
