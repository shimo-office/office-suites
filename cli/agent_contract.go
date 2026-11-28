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
// can have different cookie jars. CLI credential presence cannot establish the
// host's login state; only a host adapter can verify it in that browser.

const (
	viewTransportCLIBrowser = "cli-browser"
	viewTransportArtifact   = "artifact"
	viewTransportDownload   = "download"
	viewTransportLink       = "link"
)

// agentView describes where the user can actually see the result.
func (a *App) agentView(transport, target string) map[string]any {
	cliBrowser := transport == viewTransportCLIBrowser
	artifact := transport == viewTransportArtifact || transport == viewTransportDownload
	out := map[string]any{
		"transport":      transport,
		"usesCLISession": cliBrowser,
		"browserOpened":  cliBrowser,
		"builtInBrowser": map[string]any{
			"usable": nil, "sessionShared": nil, "authState": "unknown",
			"reason": "CLI 未检查宿主浏览器的登录态；直接打开链接可能需要该浏览器自己的会话，不能据此判定 CLI 会话失效",
			"workaround": []string{
				"使用本命令返回的截图/导出文件作为展示物",
				"宿主支持安全会话导入时，由适配程序读取 session export 文件并导入同一 origin，再在宿主中验证账号；不把凭据读入聊天",
				"宿主只支持打开链接时，CLI 无法自动共享会话；由 Agent 明确展示通道状态",
			},
		},
	}
	if target != "" {
		out["target"] = target
		if !artifact {
			out["openWith"] = "shimo-net open '" + strings.ReplaceAll(target, "'", "'\"'\"'") + "'"
			out["openSessionShared"] = false
			out["openNote"] = "open 不注入 CLI 会话；目标浏览器使用自身登录状态"
		}
	}
	if artifact {
		out["builtInBrowser"] = map[string]any{"usable": true, "requiresAuth": false, "reason": "本地产物不依赖石墨页面登录，宿主可直接展示"}
	}
	return out
}

func (a *App) agentAuth() map[string]any {
	credential, source, err := credentialForOrigin(a.Origin)
	status := sessionStatusMap(a.Origin)
	return map[string]any{
		"origin": a.Origin, "cliSession": err == nil && credential != "",
		"credentialSource": source, "sharedSession": boolv(status, "shared"),
		"authState": str(status, "authState"), "revision": status["revision"],
		"loginRequired": nil, "verified": false,
		"note": "这里只报告本地凭据来源，未调用 whoami 验证账号；宿主登录态另行验证，不因宿主预览登录页要求 CLI 重登",
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
