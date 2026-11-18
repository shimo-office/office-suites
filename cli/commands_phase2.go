package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// phase2Actions maps stable CLI action names to the editor SDK method exposed by
// the document/sheet page. The method is intentionally selected from a
// whitelist; callers provide only the documented method arguments as JSON.
var phase2Actions = map[string]string{
	"history-show":  "actions.showViewHistory",
	"history-hide":  "actions.hideViewHistory",
	"save-version":  "actions.showSaveVersion",
	"comments-show": "actions.showViewCommentList",
	"comments-hide": "actions.hideViewCommentList",
	"comment-add":   "actions.showAddComment",
	"print":         "actions.showPrint",
	"sheet-append":  "http:PUT:/lizard-api/files/%s/sheets/values",
	"sheet-insert":  "http:POSTROW:/lizard-api/files/%s/sheets",
	"sheet-delete":  "http:DELETEROW:/lizard-api/files/%s/sheets",
	// Stable same-origin spreadsheet APIs. These do not depend on a
	// page-global editor SDK, so agents can call them directly from the CLI.
	"sheet-format":  "http:POST:/lizard-api/files/%s/sheets/attrs",
	"sheet-merge":   "http:POST:/lizard-api/files/%s/sheets/mergeCells",
	"sheet-values":  "http:GET:/lizard-api/files/%s/sheets/values",
	"sheet-update":  "http:POST:/lizard-api/files/%s/sheets/values",
	"mentions-list": "api:mention-at-list",
	"comments-list": "api:comment-list",
}

var phase2PageOnlyActions = map[string]string{
	"find":             "查找/替换",
	"replace-all":      "查找/替换",
	"page-width":       "页面宽度/重排",
	"sheet-freeze":     "冻结行列",
	"sheet-sort":       "排序",
	"sheet-filter":     "筛选",
	"sheet-validation": "数据验证",
}

var phase2ManualOnlyActions = map[string]string{
	"lock":                 "文件锁定",
	"file-lock":            "文件锁定",
	"restrict-editing":     "限制编辑",
	"advanced-permissions": "高级权限",
	"member-authorization": "成员级授权",
	"permanent-delete":     "彻底删除",
	"purge":                "彻底删除",
}

func (a *App) cmdPhase2() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net phase2 <链接|guid|@别名> --action <动作> [--payload JSON] [--screenshot PATH] [--verify|--no-verify]")
	}
	action := strings.TrimSpace(a.opt("--action"))
	if operation, manualOnly := phase2ManualOnlyActions[action]; manualOnly {
		return manualOnlyError(operation)
	}
	if label, pageOnly := phase2PageOnlyActions[action]; pageOnly {
		return fmt.Errorf("%s 需要页面操作（当前 editor SDK 未暴露该能力）；请用 shimo-net open <链接> 后在页面操作", label)
	}
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
	} else if absolute, absErr := filepath.Abs(shot); absErr == nil {
		shot = absolute
	}
	var value any
	if strings.HasPrefix(method, "api:") {
		// These endpoints need the editor's same-origin signature headers; run
		// them through the page fetch path. They are read-only and do not need a
		// screenshot, so --no-verify must not block them.
		value, err = runPhase2Browser(target, method, args, "")
	} else if strings.HasPrefix(method, "http:") {
		// An API response proves only that the server accepted the mutation. The
		// user-facing result also requires a visual check in the real editor.
		// Open the target page in the isolated browser, inject the CLI session
		// when available, and capture the rendered result before reporting success.
		guid := guidFromURL(target)
		if guid == "" {
			return errors.New("表格 API 动作需要包含文件 guid 的石墨链接")
		}
		if action == "sheet-format" {
			// The attrs endpoint silently drops some attributes when several are
			// sent together (for example background + color). Apply one attribute
			// at a time so every requested property is actually persisted.
			value, err = a.runSheetFormat(guid, args)
		} else {
			value, err = a.runPhase2HTTP(guid, method, args)
		}
		if err == nil && shot != "" && cliBrowserVerification(a) {
			_, err = runPhase2Browser(target, "browser.verify", nil, shot)
		}
	} else {
		// Page actions cannot be proven without a browser and a non-blank
		// screenshot. Refuse to return a success envelope when no CLI browser
		// verification was requested; the host (for example WorkBuddy's
		// present_files panel) must perform the page action instead.
		if !cliBrowserVerification(a) {
			return errors.New("页面动作需要 --verify（CLI 本地浏览器）或宿主内置浏览器；本次未执行，不返回成功")
		}
		value, err = runPhase2Browser(target, method, args, shot)
	}
	if err != nil {
		return err
	}
	out := map[string]any{"ok": true, "action": action, "method": method, "url": target, "result": value}
	if strings.HasPrefix(method, "api:") {
		out["verified"] = true
		out["verifyWith"] = "page-fetch"
		out["note"] = "只读 API 通过页面同源 fetch 执行；无需截图"
	} else if !cliBrowserVerification(a) {
		out["verified"] = false
		out["verifyWith"] = "host-browser"
		out["note"] = "默认不启动 CLI 本地浏览器；请用宿主内置浏览器（WorkBuddy agent-browser）打开 url 截图；需要 CLI 截图时加 --verify"
	}
	if shot != "" && cliBrowserVerification(a) {
		out["screenshot"] = shot
	}
	if a.JSON {
		return a.printJSON(out)
	}
	fmt.Printf("扩展动作已完成：%s\n", action)
	if shot != "" {
		fmt.Printf("截图：%s\n", shot)
	}
	if value != nil {
		b, _ := json.Marshal(value)
		fmt.Printf("返回：%s\n", b)
	}
	return nil
}

func explicitNoBrowser(a *App) bool {
	if a.has("--no-verify") || a.has("--no-browser") {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SHIMO_NET_NO_BROWSER")))
	return v == "1" || v == "true" || v == "yes"
}

// cliBrowserVerification is opt-in: API writes do not launch a local Chrome
// unless the caller explicitly asks for CLI screenshot verification.
func cliBrowserVerification(a *App) bool {
	if explicitNoBrowser(a) {
		return false
	}
	if a.has("--verify") {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SHIMO_NET_VERIFY")))
	return v == "1" || v == "true" || v == "yes"
}

func phase2ActionNames() []string {
	// Keep schema/help deterministic even though the map is intentionally simple.
	return []string{"history-show", "history-hide", "save-version", "comments-show", "comments-hide", "comment-add", "find", "replace-all", "page-width", "print", "sheet-append", "sheet-insert", "sheet-delete", "sheet-freeze", "sheet-sort", "sheet-filter", "sheet-validation", "sheet-format", "sheet-merge", "sheet-values", "sheet-update", "mentions-list", "comments-list"}
}

func (a *App) runPhase2HTTP(guid, method string, args []any) (any, error) {
	parts := strings.SplitN(method, ":", 3)
	if len(parts) != 3 || parts[0] != "http" {
		return nil, fmt.Errorf("无效的 phase2 API 动作：%s", method)
	}
	httpMethod, pathTemplate := parts[1], parts[2]
	path := fmt.Sprintf(pathTemplate, url.PathEscape(guid))
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return nil, err
	}
	// The spreadsheet API addresses cells as "<工作表名>!A1:C1".  Accept the
	// friendlier {"sheetName":"工作表1","range":"A1:C1"} form and normalize it;
	// a bare "A1:C1" is rejected by the server with a generic 400.
	if strings.Contains(path, "/sheets/") {
		if name := strings.TrimSpace(str(payload, "sheetName")); name != "" {
			if rng := strings.TrimSpace(str(payload, "range")); rng != "" && !strings.Contains(rng, "!") {
				payload["range"] = name + "!" + rng
			}
		}
		if rng := strings.TrimSpace(str(payload, "range")); rng != "" && !strings.Contains(rng, "!") {
			return nil, errors.New("range 必须带工作表名，例如 \"工作表1!A1:C1\"，或在 payload 中提供 sheetName")
		}
	}
	if (httpMethod == "POST" || httpMethod == "PUT") && strings.HasSuffix(path, "/sheets/values") {
		if strings.TrimSpace(str(payload, "range")) == "" {
			return nil, errors.New("sheet-update/sheet-append 必须提供 range，例如 \"工作表1!A1:D4\"")
		}
		resource, ok := payload["resource"].(map[string]any)
		if !ok {
			return nil, errors.New("sheet-update/sheet-append 必须提供 resource.values；裸 values 会被服务端静默忽略")
		}
		if _, ok := resource["values"]; !ok {
			return nil, errors.New("sheet-update/sheet-append 的 resource 必须包含 values")
		}
	}
	if httpMethod == "POSTROW" || httpMethod == "DELETEROW" {
		sheetName := strings.TrimSpace(str(payload, "sheetName"))
		index := strings.TrimSpace(str(payload, "index"))
		if sheetName == "" || index == "" {
			return nil, errors.New("sheet-insert/sheet-delete 必须提供 sheetName 和 index")
		}
		count := strings.TrimSpace(str(payload, "count"))
		if count == "" {
			count = "1"
		}
		path += "/" + url.PathEscape(sheetName) + "/rows/" + url.PathEscape(index) + "?count=" + url.QueryEscape(count)
		if httpMethod == "POSTROW" {
			httpMethod = "POST"
		} else {
			httpMethod = "DELETE"
		}
		payload = nil
	}
	if httpMethod == "GET" {
		rangeValue := strings.TrimSpace(str(payload, "range"))
		if rangeValue == "" {
			return nil, errors.New("sheet-values 必须在 payload 中提供 range")
		}
		path += "?range=" + url.QueryEscape(rangeValue)
		payload = nil
	}
	r, err := a.authRequestPath(path, httpMethod, payload, nil, nil)
	if err != nil {
		return nil, err
	}
	if r.Status < 200 || r.Status >= 300 {
		return nil, fmt.Errorf("表格 API 失败：HTTP %d %s", r.Status, snippet(r.Raw))
	}
	if len(r.Raw) == 0 || r.Status == 204 {
		return map[string]any{"status": r.Status}, nil
	}
	if r.Data != nil {
		return r.Data, nil
	}
	return map[string]any{"status": r.Status}, nil
}

func (a *App) runSheetFormat(guid string, args []any) (any, error) {
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return nil, err
	}
	rangeValue := strings.TrimSpace(str(payload, "range"))
	sheetName := strings.TrimSpace(str(payload, "sheetName"))
	if sheetName != "" && rangeValue != "" && !strings.Contains(rangeValue, "!") {
		rangeValue = sheetName + "!" + rangeValue
	}
	if rangeValue == "" || !strings.Contains(rangeValue, "!") {
		return nil, errors.New("sheet-format 的 range 必须带工作表名，例如 \"工作表1!A1:C1\"，或在 payload 中提供 sheetName")
	}
	attrs, ok := payload["attrs"].(map[string]any)
	if !ok || len(attrs) == 0 {
		return nil, errors.New("sheet-format 必须在 attrs 中提供至少一个属性，例如 {\"background\":\"#D32F2F\"}")
	}
	// Only attributes verified to land in the exported xlsx are accepted.
	// fontSize/fontFamily/align/valign were returned as "applied" by the server
	// but did not change the exported cell style, so they are rejected instead
	// of producing another trusted-looking false success.
	supported := map[string]bool{"background": true, "color": true, "bold": true, "italic": true, "underline": true}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		if !supported[k] {
			return nil, fmt.Errorf("sheet-format 不支持属性 %s；已验证可落地的只有 background/color/bold/italic/underline", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	applied := make([]string, 0, len(keys))
	for _, key := range keys {
		body := map[string]any{"range": rangeValue, "attrs": map[string]any{key: attrs[key]}}
		r, err := a.authRequestPath("/lizard-api/files/"+url.PathEscape(guid)+"/sheets/attrs", "POST", body, nil, nil)
		if err != nil {
			return nil, err
		}
		if r.Status < 200 || r.Status >= 300 {
			return nil, fmt.Errorf("sheet-format 设置 %s 失败：HTTP %d %s", key, r.Status, snippet(r.Raw))
		}
		applied = append(applied, key)
	}
	return map[string]any{"range": rangeValue, "applied": applied}, nil
}

func phase2ObjectArg(args []any) (map[string]any, error) {
	if len(args) != 1 {
		return nil, errors.New("表格 API 动作的 --payload 必须是一个 JSON 对象")
	}
	payload, ok := args[0].(map[string]any)
	if !ok {
		return nil, errors.New("表格 API 动作的 --payload 必须是一个 JSON 对象")
	}
	return payload, nil
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

func isRetryableEditorError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{"页面未暴露 editor sdk", "editor sdk", "sdk 方法不可用", "页面尚未渲染", "cannot read", "undefined is not"} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func runPhase2Browser(target, method string, args []any, screenshot string) (any, error) {
	chrome, err := findChrome()
	if err != nil {
		return nil, err
	}
	// Use a throw-away profile for visual verification: the sheet editor keeps
	// a rendered snapshot in the persistent CLI profile, so reusing it can
	// produce a screenshot of the pre-write state even after the API succeeded.
	profile, err := os.MkdirTemp("", "shimo-net-verify-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(profile)
	portFile := filepath.Join(profile, "DevToolsActivePort")
	cmd := exec.Command(chrome, chromeLaunchArgs("--no-first-run", "--no-default-browser-check", "--user-data-dir="+profile, "--remote-debugging-port=0", target)...)
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
	if cookie, err := authCookie(); err == nil && sameCookieHost(target) {
		if err := setCookies(ws, target, cookie); err == nil {
			_, _ = ws.Call(102, "Page.reload", map[string]any{"ignoreCache": true}, 10*time.Second)
		}
	}
	// A screenshot taken before the editor mounts is a blank white image and
	// must not be accepted as visual proof.  Wait for a real document body
	// before running the action or capturing the result.
	waitPageReady(ws, 30*time.Second)
	if strings.HasPrefix(method, "actions.") && !waitEditorReady(ws, 10*time.Second) {
		return nil, errors.New("页面已打开但编辑器未加载完成（等待 10 秒）；未执行页面动作")
	}
	argsJSON, _ := json.Marshal(args)
	expr := phase2Expression(method, string(argsJSON), guidFromURL(target))
	// The editor mounts asynchronously after the document shell is ready. A
	// missing SDK in the first second usually means "not initialized yet", not
	// "this page will never support the action". Retry a small, bounded number
	// of times so a slow editor does not turn into a hard failure.
	const editorAttempts = 5
	const editorRetryDelay = 2 * time.Second
	var value any
	var lastErr error
	for i := 0; i < editorAttempts; i++ {
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
						if !isRetryableEditorError(lastErr) {
							break
						}
					} else {
						lastErr = nil
						break
					}
				}
			}
		} else {
			lastErr = callErr
		}
		if i < editorAttempts-1 {
			time.Sleep(editorRetryDelay)
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("页面 editor SDK/API 不可用（已重试 %d 次；可改用 shimo-net open 后由 computer-use 按网页操作）：%w", editorAttempts, lastErr)
	}
	if screenshot != "" {
		for attempt := 0; attempt < 3; attempt++ {
			if err := capturePhase2Screenshot(ws, screenshot); err != nil {
				return nil, err
			}
			if screenshotHasContent(screenshot) {
				return value, nil
			}
			if attempt < 2 {
				time.Sleep(1200 * time.Millisecond)
			}
		}
		return nil, errors.New("截图内容为空白，页面未渲染完成；不返回成功")
	}
	return value, nil
}

func sameCookieHost(target string) bool {
	configured, _ := resolveOrigin()
	return hostOf(target) != "" && hostOf(target) == hostOf(configured)
}

func phase2Expression(method, argsJSON, guid string) string {
	if method == "browser.verify" {
		return `(async()=>{const path=location.pathname||"";const text=(document.body?.innerText||"").trim();if(path==="/login"||text.includes("验证码登录")||text.includes("欢迎使用石墨办公"))return {ok:false,error:"页面未登录，无法完成视觉核验"};if(document.readyState!=="complete")return {ok:false,error:"页面尚未渲染完成，不能作为视觉核验证据"};if(!document.querySelector("canvas")&&text.length<20)return {ok:false,error:"页面尚未渲染完成，不能作为视觉核验证据"};return {ok:true,textLength:text.length,canvas:document.querySelectorAll("canvas").length};})()`
	}
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
	return fmt.Sprintf(`(async()=>{const sdk=window.smsdk?.__DRIVE_SDK__?.editor||globalThis.sdk;if(!sdk)return {ok:false,error:"页面未暴露 editor SDK"};const path=%s;let owner=sdk;for(let i=0;i<path.length-1;i++){owner=owner?.[path[i]]}const fn=owner?.[path[path.length-1]];if(typeof fn!=="function")return {ok:false,error:"SDK 方法不可用："+path.join(".")};const args=%s;const value=await fn.apply(owner,args);return {ok:true,value};})()`, string(pathJSON), argsJSON)
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
		cookies = append(cookies, map[string]any{
			"name": kv[0], "value": kv[1], "url": target,
			"path": "/", "httpOnly": true, "secure": strings.HasPrefix(target, "https://"),
		})
	}
	if len(cookies) == 0 {
		return nil
	}
	_, err := ws.Call(100, "Network.setCookies", map[string]any{"cookies": cookies}, 10*time.Second)
	return err
}

func waitPageReady(ws *WSConn, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	id := 900
	for time.Now().Before(deadline) {
		res, err := ws.Call(id, "Runtime.evaluate", map[string]any{
			"expression":    `document.readyState==="complete" && (document.querySelector("canvas")!=null || (document.body?.innerText||"").trim().length>=20)`,
			"returnByValue": true,
		}, 5*time.Second)
		id++
		if err == nil {
			if result, ok := res["result"].(map[string]any); ok && boolv(result, "value") {
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func waitEditorReady(ws *WSConn, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	id := 1200
	for time.Now().Before(deadline) {
		res, err := ws.Call(id, "Runtime.evaluate", map[string]any{
			"expression":    `!!(window.smsdk?.__DRIVE_SDK__?.editor) || !!globalThis.sdk`,
			"returnByValue": true,
		}, 5*time.Second)
		id++
		if err == nil {
			if result, ok := res["result"].(map[string]any); ok && boolv(result, "value") {
				return true
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func screenshotHasContent(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return false
	}
	b := img.Bounds()
	stepX, stepY := b.Dx()/40, b.Dy()/40
	if stepX < 1 {
		stepX = 1
	}
	if stepY < 1 {
		stepY = 1
	}
	total, colored := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			r, g, bl, _ := img.At(x, y).RGBA()
			total++
			if r>>8 < 245 || g>>8 < 245 || bl>>8 < 245 {
				colored++
			}
		}
	}
	return total > 0 && colored*100 >= total
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
