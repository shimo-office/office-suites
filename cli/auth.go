package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const keychainService = "shimo-net-cli"

type Cred struct {
	Origin  string `json:"origin,omitempty"`
	Cookie  string `json:"cookie,omitempty"`
	SavedAt string `json:"savedAt,omitempty"`
	Storage string `json:"storage,omitempty"`
}

func credSource() string {
	if os.Getenv("SHIMO_NET_COOKIE") != "" {
		return "env"
	}
	if keychainAvailable() && keychainLoad() != "" {
		return "keychain"
	}
	if c := readCredFile(); c != nil && c.Cookie != "" {
		return "file"
	}
	return ""
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

func readCookie() (string, error) {
	if c := os.Getenv("SHIMO_NET_COOKIE"); c != "" {
		return c, nil
	}
	if keychainAvailable() {
		if c := keychainLoad(); c != "" {
			return c, nil
		}
	}
	if c := readCredFile(); c != nil && c.Cookie != "" {
		return c.Cookie, nil
	}
	return "", errors.New("未登录：请先运行 shimo-net login")
}

func writeCred(origin, cookie string) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	c := Cred{Origin: origin, SavedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z")}
	if keychainAvailable() {
		if err := keychainSave("shimo-net", cookie); err == nil {
			c.Storage = "keychain"
		}
	}
	if c.Storage == "" {
		c.Cookie = cookie
		c.Storage = "file"
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(credPath(), append(b, '\n'), 0o600); err != nil {
		return err
	}
	_ = os.Chmod(stateDir(), 0o700)
	_ = os.Chmod(credPath(), 0o600)
	return nil
}

func clearCred() {
	_ = os.Remove(credPath())
	if keychainAvailable() {
		keychainClear()
	}
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
	chrome, err := findChrome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(profilePath(), 0o700); err != nil {
		return err
	}
	portFile := filepath.Join(profilePath(), "DevToolsActivePort")
	_ = os.Remove(portFile)
	args := []string{"--no-first-run", "--no-default-browser-check",
		"--user-data-dir=" + profilePath(), "--remote-debugging-port=0",
		"--app=" + a.Origin + "/login"}
	if os.Getenv("SHIMO_NET_NO_SANDBOX") != "" {
		args = append([]string{"--no-sandbox", "--disable-gpu"}, args...)
	}
	fmt.Printf("产品网址：%s\n", a.Origin)
	fmt.Printf("正在打开 Chrome，请在窗口里用你自己的账号登录 %s…\n", a.Origin)
	cmd := exec.Command(chrome, args...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Chrome 失败：%w", err)
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
		return errors.New("Chrome 没有启动调试端口")
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
	host := hostOf(a.Origin)
	loginDeadline := time.Now().Add(10 * time.Minute)
	id := 2
	for time.Now().Before(loginDeadline) {
		res, err := ws.Call(id, "Network.getAllCookies", nil, 8*time.Second)
		id++
		if err == nil {
			cookies := cookiesForHost(res, host)
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

func waitPageTarget(port int, origin string) (string, error) {
	host := hostOf(origin)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var targets []map[string]any
		body, err := httpGet(fmt.Sprintf("http://127.0.0.1:%d/json", port))
		if err == nil {
			_ = json.Unmarshal(body, &targets)
			for _, t := range targets {
				if str(t, "type") == "page" && strings.Contains(str(t, "url"), host) {
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

func cookiesForHost(res map[string]any, host string) []string {
	var out []string
	arr, _ := res["cookies"].([]any)
	for _, item := range arr {
		c, _ := item.(map[string]any)
		domain := strings.TrimPrefix(str(c, "domain"), ".")
		if strings.HasSuffix(domain, host) {
			out = append(out, str(c, "name")+"="+str(c, "value"))
		}
	}
	return out
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
	fmt.Println("已删除本机凭据（Keychain / 文件）")
	return nil
}
