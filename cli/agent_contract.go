package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The agent contract exists because a host preview browser and the CLI browser
// have different cookie jars: opening an editor link in the host's built-in
// browser lands on the login page even though the CLI session is valid. Instead
// of leaving the agent to guess, every result states how the item can be shown.

const (
	viewTransportCLIBrowser = "cli-browser"
	viewTransportArtifact   = "artifact"
	viewTransportDownload   = "download"
)

// agentView describes where the user can actually see the result.
func (a *App) agentView(transport, target string) map[string]any {
	keep := transport == viewTransportCLIBrowser
	out := map[string]any{
		"transport":      transport,
		"usesCLISession": keep,
		"builtInBrowser": map[string]any{
			"usable": false,
			"reason": "宿主内置浏览器不共享 CLI 登录态，直接打开编辑页会停在登录页；这与 CLI 会话是否有效无关",
			"workaround": []string{
				"使用本命令返回的截图/导出文件作为展示物",
				"需要交互式查看时执行 shimo-net open <链接>（独立配置，已注入 CLI 会话）",
				"确需在宿主浏览器内查看时，先用 shimo-net session export --out <文件> 导出会话，再由宿主注入同一 origin 的 cookie",
			},
		},
	}
	if target != "" {
		out["target"] = target
		out["openWith"] = "shimo-net open " + target + " --verify"
	}
	if !keep {
		out["builtInBrowser"] = map[string]any{"usable": true, "reason": "本结果不依赖编辑器页面，宿主可直接展示产物"}
	}
	return out
}

func (a *App) agentAuth() map[string]any {
	_, source, _ := credentialForOrigin(a.Origin)
	status := sessionStatusMap(a.Origin)
	return map[string]any{
		"origin": a.Origin, "cliSession": source != "none" && source != "",
		"credentialSource": source, "sharedSession": boolv(status, "shared"),
		"authState": str(status, "authState"), "revision": status["revision"],
		"loginRequired": false,
		"note":          "CLI 已登录时不要在宿主浏览器重复登录；宿主预览的登录页只代表它没有 CLI 的 cookie",
	}
}

// cmdSessionExport writes the current session for a host that must show the
// editor in its own browser. Secrets never go to stdout: only the file path and
// a short fingerprint are printed, so agent transcripts stay clean.
func (a *App) cmdSessionExport() error {
	out := strings.TrimSpace(a.opt("--out"))
	rec, err := readSessionRecord(a.Origin)
	if err != nil {
		return err
	}
	if rec == nil || len(rec.Cookies) == 0 {
		return &commandError{Message: "本机没有可导出的共享会话；先执行 shimo-net login 或 shimo-net session harvest", Result: map[string]any{"code": "SESSION_EMPTY", "origin": a.Origin}, ExitCode: 4}
	}
	header := sessionCookieHeaderFor(rec, parseURLOrNil(a.apiURL("/users/me")))
	if header == "" {
		return errors.New("当前会话 cookie 不适用于配置的产品网址，未导出")
	}
	if out == "" {
		dir := filepath.Join(stateDir(), "session-export")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		out = filepath.Join(dir, fmt.Sprintf("session-%s.json", time.Now().Format("20060102-150405")))
	}
	out = expandHome(out)
	if absolute, err := filepath.Abs(out); err == nil {
		out = absolute
	}
	if _, err := os.Lstat(out); err == nil {
		return errors.New("目标文件已存在，未覆盖：" + out)
	}
	payload := map[string]any{
		"schemaVersion": 1,
		"origin":        rec.Origin,
		"cookie":        header,
		"cookies":       rec.Cookies,
		"account":       map[string]any{"id": rec.Account.ID, "name": rec.Account.Name, "email": rec.Account.Email},
		"revision":      rec.Revision,
		"exportedAt":    time.Now().UTC().Format(time.RFC3339),
		"usage":         "宿主把 cookie 注入同一 origin 后再打开编辑页；导入回 CLI 用 shimo-net session import < 该文件",
	}
	if err := writePrivateJSON(out, payload); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "path": out, "origin": rec.Origin, "cookieCount": len(rec.Cookies), "revision": rec.Revision,
		"note": "文件含登录凭据，权限 0600；不要粘贴到聊天、日志或工单"}
	if a.JSON {
		return a.printJSON(result)
	}
	fmt.Printf("会话已导出：%s（%d 个 cookie，权限 0600）\n", terminalText(out), len(rec.Cookies))
	return nil
}

func writePrivateJSON(path string, v any) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	if _, err = f.Write(raw); err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	return f.Close()
}
