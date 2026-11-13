package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// phase2Actions maps stable CLI action names to the editor SDK method exposed by
// the document/sheet page. The method is intentionally selected from a
// whitelist; callers provide only the documented method arguments as JSON.
var phase2Actions = map[string]string{
	"history-show":     "history.show",
	"history-hide":     "history.hide",
	"save-version":     "version.createRevision",
	"comments-show":    "comments.show",
	"comments-hide":    "comments.hide",
	"find":             "search.findAll",
	"replace-all":      "search.replaceAll",
	"page-width":       "settings.setPageWidth",
	"print":            "print",
	"sheet-append":     "activeSheet.appendData",
	"sheet-insert":     "activeSheet.insertRows",
	"sheet-delete":     "activeSheet.deleteRows",
	"sheet-freeze":     "activeSheet.freeze",
	"sheet-sort":       "activeSheet.sort",
	"sheet-filter":     "activeSheet.filter",
	"sheet-validation": "activeSheet.setDataValidation",
	"mentions-list":    "api:mention-at-list",
	"comments-list":    "api:comment-list",
}

func (a *App) cmdPhase2() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net phase2 <链接|guid|@别名> --action <动作> [--payload JSON] [--screenshot PATH]")
	}
	action := strings.TrimSpace(a.opt("--action"))
	method, ok := phase2Actions[action]
	if !ok {
		return fmt.Errorf("未知扩展动作 %q；可用动作：%s", action, strings.Join(phase2ActionNames(), ", "))
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	shot := a.opt("--screenshot")
	if shot == "" {
		shot = filepath.Join(stateDir(), "screenshots", fmt.Sprintf("phase2-%s.png", time.Now().Format("20060102-150405.000")))
	}
	value, err := runPhase2Browser(target, method, args, shot)
	if err != nil {
		return err
	}
	out := map[string]any{"ok": true, "action": action, "method": method, "url": target, "screenshot": shot, "result": value}
	if a.JSON {
		return a.printJSON(out)
	}
	fmt.Printf("扩展动作已完成：%s\n截图：%s\n", action, shot)
	if value != nil {
		b, _ := json.Marshal(value)
		fmt.Printf("返回：%s\n", b)
	}
	return nil
}

func phase2ActionNames() []string {
	// Keep schema/help deterministic even though the map is intentionally simple.
	return []string{"history-show", "history-hide", "save-version", "comments-show", "comments-hide", "find", "replace-all", "page-width", "print", "sheet-append", "sheet-insert", "sheet-delete", "sheet-freeze", "sheet-sort", "sheet-filter", "sheet-validation", "mentions-list", "comments-list"}
}

func phase2Args(raw string) ([]any, error) {
	if strings.TrimSpace(raw) == "" {
		return []any{}, nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("--payload 必须是合法 JSON：%w", err)
	}
	if arr, ok := v.([]any); ok {
		return arr, nil
	}
	return []any{v}, nil
}

func (a *App) phase2Target(raw string) (string, error) {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw, nil
	}
	guid, err := a.resolveTarget(raw)
	if err != nil {
		return "", err
	}
	if guid == "Desktop" {
		return "", errors.New("桌面没有可打开的文档链接")
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return "", err
	}
	return a.urlOf(withGUID(info, guid)), nil
}

func runPhase2Browser(target, method string, args []any, screenshot string) (any, error) {
	chrome, err := findChrome()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(profilePath(), 0o700); err != nil {
		return nil, err
	}
	portFile := filepath.Join(profilePath(), "DevToolsActivePort")
	_ = os.Remove(portFile)
	cmd := exec.Command(chrome, "--no-first-run", "--no-default-browser-check", "--user-data-dir="+profilePath(), "--remote-debugging-port=0", target)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动浏览器失败：%w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	port, err := waitDevToolsPort(portFile, 30*time.Second)
	if err != nil {
		return nil, err
	}
	wsURL, err := waitPageTarget(port, target)
	if err != nil {
		return nil, err
	}
	ws, err := dialWS(wsURL)
	if err != nil {
		return nil, err
	}
	defer ws.Close()
	if _, err := ws.Call(1, "Network.enable", nil, 10*time.Second); err != nil {
		return nil, err
	}
	if _, err := ws.Call(2, "Page.enable", nil, 10*time.Second); err != nil {
		return nil, err
	}
	// Reuse the CLI login cookie when the fresh profile has no session. The
	// cookie value is never included in command output or error messages.
	// Never send the stored session to a different host supplied on the
	// command line. Private deployments use their configured origin here.
	if cookie, err := readCookie(); err == nil && sameCookieHost(target) {
		if err := setCookies(ws, target, cookie); err == nil {
			_, _ = ws.Call(102, "Page.reload", map[string]any{"ignoreCache": true}, 10*time.Second)
			time.Sleep(1200 * time.Millisecond)
		}
	}
	argsJSON, _ := json.Marshal(args)
	expr := phase2Expression(method, string(argsJSON), guidFromURL(target))
	var value any
	var lastErr error
	for i := 0; i < 20; i++ {
		res, callErr := ws.Call(3+i, "Runtime.evaluate", map[string]any{"expression": expr, "awaitPromise": true, "returnByValue": true}, 10*time.Second)
		if callErr == nil {
			if _, hasException := res["exceptionDetails"]; hasException {
				lastErr = errors.New("页面执行 SDK 方法时抛出异常")
			} else if result, ok := res["result"].(map[string]any); ok {
				if desc := str(result, "description"); desc != "" && str(result, "subtype") == "error" {
					lastErr = errors.New(desc)
				} else {
					value = result["value"]
					if resultValue, ok := value.(map[string]any); ok && resultValue["ok"] == false {
						lastErr = errors.New(str(resultValue, "error"))
						// Explicit page-level failures (for example, the current
						// editor does not expose globalThis.sdk) are deterministic;
						// retrying them only delays the browser fallback.
						break
					} else {
						lastErr = nil
						break
					}
				}
			}
		} else {
			lastErr = callErr
		}
		time.Sleep(1500 * time.Millisecond)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("页面 editor SDK/API 不可用（可改用 shimo-net open --default 后按网页操作）：%w", lastErr)
	}
	if screenshot != "" {
		if err := capturePhase2Screenshot(ws, screenshot); err != nil {
			return nil, err
		}
	}
	return value, nil
}

func sameCookieHost(target string) bool {
	configured, _ := resolveOrigin()
	return hostOf(target) != "" && hostOf(target) == hostOf(configured)
}

func phase2Expression(method, argsJSON, guid string) string {
	if strings.HasPrefix(method, "api:") {
		endpoint := map[string]string{
			"api:mention-at-list": "/sdk/v2/collab-files/%s/mention-at-list",
			"api:comment-list":    "/sdk/v2/collab-files/%s/comment-list",
		}[method]
		pathJSON, _ := json.Marshal(fmt.Sprintf(endpoint, "__FILE_GUID__"))
		guidJSON, _ := json.Marshal(guid)
		return fmt.Sprintf(`(async()=>{const path=%s.replace("__FILE_GUID__",encodeURIComponent(%s));const r=await fetch(path,{credentials:"include"});const text=await r.text();let body=text;try{body=JSON.parse(text)}catch{};if(!r.ok)return {ok:false,error:"HTTP "+r.status,body};return {ok:true,value:body};})()`, string(pathJSON), string(guidJSON))
	}
	parts := strings.Split(method, ".")
	pathJSON, _ := json.Marshal(parts)
	return fmt.Sprintf(`(async()=>{const sdk=globalThis.sdk;if(!sdk)return {ok:false,error:"页面未暴露 editor SDK"};const path=%s;let owner=sdk;for(let i=0;i<path.length-1;i++){owner=owner?.[path[i]]}const fn=owner?.[path[path.length-1]];if(typeof fn!=="function")return {ok:false,error:"SDK 方法不可用："+path.join(".")};const args=%s;const value=await fn.apply(owner,args);return {ok:true,value};})()`, string(pathJSON), argsJSON)
}

func waitDevToolsPort(path string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			if len(lines) > 0 {
				var port int
				_, _ = fmt.Sscanf(strings.TrimSpace(lines[0]), "%d", &port)
				if port > 0 {
					return port, nil
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return 0, errors.New("浏览器没有启动调试端口")
}

func setCookies(ws *WSConn, target, raw string) error {
	parts := strings.Split(raw, ";")
	cookies := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 || kv[0] == "" {
			continue
		}
		cookies = append(cookies, map[string]any{"name": kv[0], "value": kv[1], "url": target})
	}
	if len(cookies) == 0 {
		return nil
	}
	_, err := ws.Call(100, "Network.setCookies", map[string]any{"cookies": cookies}, 10*time.Second)
	return err
}

func capturePhase2Screenshot(ws *WSConn, path string) error {
	res, err := ws.Call(101, "Page.captureScreenshot", map[string]any{"format": "png"}, 15*time.Second)
	if err != nil {
		return err
	}
	data := str(res, "data")
	if data == "" {
		return errors.New("浏览器没有返回截图数据")
	}
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return nil
}
