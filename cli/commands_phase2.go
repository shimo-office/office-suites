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
	"history-show":  "sdk.history.show",
	"history-hide":  "sdk.history.hide",
	"comments-show": "sdk.comments.show",
	"comments-hide": "sdk.comments.hide",
	"save-version":  "sdk.version.createRevision",
	"print":         "sdk.print",
	"cell-history":  "sdk.cellHistory",
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
	"comment-add":      "提交评论",
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

// Offline command tests replace only the browser boundary.
// Production continues to use the isolated browser and screenshot checks.
var phase2RunBrowser = runPhase2Browser

// 展示型动作（打开侧栏、打印预览）的目的就是让用户看到结果。CLI 启动的
// 浏览器窗口默认保留在桌面上，由用户自己关闭；批量验收可用 --close 关掉。
var phase2KeepBrowserOpen bool
var phase2KeptProfile string

func phase2ViewAction(action string) bool {
	switch action {
	case "history-show", "comments-show", "print", "cell-history", "comment-add":
		return true
	}
	return false
}

func (a *App) cmdPhase2() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net editor <链接|guid|@别名> --action <动作> [--payload JSON] [--screenshot PATH] [--verify|--no-verify]")
	}
	action := strings.TrimSpace(a.opt("--action"))
	if operation, manualOnly := phase2ManualOnlyActions[action]; manualOnly {
		return manualOnlyError(operation)
	}
	if label, pageOnly := phase2PageOnlyActions[action]; pageOnly {
		return fmt.Errorf("%s 需要页面操作（当前 editor SDK 未暴露该能力）；浏览器选择见 docs/scenarios.md#浏览器优先级，本次未执行", label)
	}
	method, ok := phase2Actions[action]
	if !ok {
		return fmt.Errorf("未知扩展动作 %q；可用动作：%s", action, strings.Join(phase2ActionNames(), ", "))
	}
	if strings.HasPrefix(method, "api:") {
		return a.cmdEditorRead(action, method)
	}
	phase2KeepBrowserOpen = phase2ViewAction(action)
	if a.has("--close") {
		phase2KeepBrowserOpen = false
	}
	if a.has("--keep-open") {
		phase2KeepBrowserOpen = true
	}
	phase2KeptProfile = ""
	if explicitNoBrowser(a) && !strings.HasPrefix(method, "http:") {
		return errors.New("当前设置禁止启动浏览器；该动作依赖页面，请改用宿主浏览器。本次未执行")
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !sameCredentialOrigin(target, a.Origin) {
		return errors.New("编辑器目标与当前配置的 origin 不一致；本次未执行")
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
	if strings.HasPrefix(method, "http:") {
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
			_, err = phase2RunBrowser(target, "browser.verify", nil, shot)
			if err != nil && action != "sheet-values" {
				return phase2MutationFailure(err, guid, "verify", map[string]any{"action": action, "result": value, "verified": false})
			}
		}
	} else {
		// Page actions cannot be proven without a browser and a non-blank
		// screenshot. Refuse to return a success envelope when no CLI browser
		// verification was requested; the host (for example WorkBuddy's
		// present_files panel) must perform the page action instead.
		if !cliBrowserVerification(a) {
			return errors.New("页面动作需要 --verify（CLI 本地浏览器）或宿主内置浏览器；本次未执行，不返回成功")
		}
		value, err = phase2RunBrowser(target, method, args, shot)
	}
	if err != nil {
		return err
	}
	out := map[string]any{"ok": true, "action": action, "method": method, "url": target, "result": value}
	if strings.HasPrefix(method, "sdk.") {
		out["scope"] = "page-sdk"
		actionVerified := false
		panelVisible := false
		if m, ok := value.(map[string]any); ok {
			actionVerified = boolv(m, "ok")
			panelVisible = boolv(m, "panelVisible")
		}
		if strings.HasPrefix(method, "sdk.history.") || strings.HasPrefix(method, "sdk.comments.") || method == "sdk.cellHistory" {
			// 面板类动作必须看到真实面板，只调用成功不算完成。
			out["actionVerified"] = actionVerified && panelVisible
			out["panelVisible"] = panelVisible
			out["note"] = "通过页面右键菜单/前端 SDK 打开面板，等待渲染后检测面板可见性；截图需自行核对"
		} else {
			out["actionVerified"] = actionVerified
			out["note"] = "直接调用套件前端 SDK；结果以接口返回为准"
		}
	}
	if !cliBrowserVerification(a) {
		out["verified"] = false
		out["verifyWith"] = "host-browser"
		out["note"] = "默认不启动 CLI 本地浏览器；结果待页面核验，浏览器选择见 docs/scenarios.md#浏览器优先级；需要 CLI 截图时加 --verify"
	}
	if shot != "" && cliBrowserVerification(a) {
		out["screenshot"] = shot
	}
	if phase2KeptProfile != "" {
		// 面板类结果留给人看：浏览器保持打开，CLI 只负责执行和截图。
		out["browserKept"] = true
		out["browserProfile"] = phase2KeptProfile
	}
	if a.JSON {
		return a.printJSON(out)
	}
	if screenshot, ok := out["screenshot"].(string); ok {
		if strings.HasPrefix(method, "sdk.") {
			fmt.Printf("前端 SDK 动作已执行：%s\n截图：%s\n", action, terminalText(screenshot))
		} else {
			fmt.Printf("动作已执行并截图，请核对页面结果：%s\n截图：%s\n", action, terminalText(screenshot))
		}
	} else {
		fmt.Printf("API 请求已完成，结果待页面核验：%s\n", action)
	}
	if value != nil {
		b, _ := json.Marshal(value)
		fmt.Printf("返回：%s\n", b)
	}
	if phase2KeptProfile != "" {
		fmt.Printf("浏览器窗口已保留在桌面（面板可直接查看），关闭窗口即可结束；配置文件：%s\n", terminalText(phase2KeptProfile))
	}
	return nil
}

func explicitNoBrowser(a *App) bool {
	if a.has("--no-browser") {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SHIMO_NET_NO_BROWSER")))
	return v == "1" || v == "true" || v == "yes"
}

// cliBrowserVerification is opt-in: API writes do not launch a local Chrome
// unless the caller explicitly asks for CLI screenshot verification.
func cliBrowserVerification(a *App) bool {
	if a.has("--no-verify") || explicitNoBrowser(a) {
		return false
	}
	if a.has("--verify") {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SHIMO_NET_VERIFY")))
	return v == "1" || v == "true" || v == "yes"
}

func phase2ActionNames() []string {
	return keys(phase2Actions)
}

func (a *App) runPhase2HTTP(guid, method string, args []any) (any, error) {
	parts := strings.SplitN(method, ":", 3)
	if len(parts) != 3 || parts[0] != "http" {
		return nil, fmt.Errorf("无效的 editor API 动作：%s", method)
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
	var requestBody any
	if payload != nil {
		requestBody = payload
	}
	r, err := a.authRequestPath(path, httpMethod, requestBody, nil, nil)
	if err == nil && (r.Status < 200 || r.Status >= 300) {
		err = fmt.Errorf("表格 API 失败：HTTP %d %s", r.Status, snippet(r.Raw))
	}
	if err != nil {
		if httpMethod != "GET" {
			return nil, phase2MutationFailure(err, guid, "write", nil)
		}
		return nil, err
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
		if err == nil && (r.Status < 200 || r.Status >= 300) {
			err = fmt.Errorf("sheet-format 设置 %s 失败：HTTP %d %s", key, r.Status, snippet(r.Raw))
		}
		if err != nil {
			accepted := "无"
			if len(applied) > 0 {
				accepted = strings.Join(applied, ", ")
			}
			err = fmt.Errorf("设置属性 %s 失败；此前 API 已受理的属性：%s（仍需读回核对）：%w", key, accepted, err)
			return nil, phase2MutationFailure(err, guid, "write", map[string]any{
				"action": "sheet-format", "range": rangeValue, "applied": applied, "failedAttribute": key,
			})
		}
		applied = append(applied, key)
	}
	return map[string]any{"range": rangeValue, "applied": applied}, nil
}

// A failed write response or later verification cannot prove the mutation did
// not happen. Preserve accepted results and stop whole-command retries.
func phase2MutationFailure(err error, guid, stage string, detail map[string]any) error {
	result := map[string]any{"guid": guid, "stage": stage, "mutationAttempted": true,
		"mayHaveApplied": true, "noReplay": true, "originalError": err.Error()}
	for key, value := range detail {
		result[key] = value
	}
	return &commandError{Message: "写入已提交或可能已提交；请先只读核对，禁止自动重放：" + err.Error(), Result: result, NoReplay: true}
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
	for _, needle := range []string{"页面未暴露 editor sdk", "editor sdk", "sdk 方法不可用", "页面尚未渲染", "cannot read", "undefined is not", "没有版本接口", "没有可用的历史面板", "没有可用的评论面板", "没有打印接口", "尚未就绪"} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func runPhase2Browser(target, method string, args []any, screenshot string) (any, error) {
	if !sameCookieHost(target) {
		return nil, errors.New("浏览器编辑目标与配置的 origin 不一致；本次未执行")
	}
	cookie, err := authCookie()
	if err != nil {
		return nil, fmt.Errorf("浏览器会话读取失败，未执行页面动作：%w", err)
	}
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
	if phase2KeepBrowserOpen {
		// 窗口要留给用户看，profile 不能在退出时删掉，否则运行中的浏览器
		// 会失去它的用户目录。
		phase2KeptProfile = profile
	} else {
		defer os.RemoveAll(profile)
	}
	portFile := filepath.Join(profile, "DevToolsActivePort")
	cmd := exec.Command(chrome, chromeLaunchArgs("--no-first-run", "--no-default-browser-check", "--user-data-dir="+profile, "--remote-debugging-port=0", target)...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动浏览器失败：%w", err)
	}
	defer func() {
		if phase2KeepBrowserOpen {
			return
		}
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
	if err := initializePhase2Session(ws, target, cookie); err != nil {
		return nil, err
	}
	// A screenshot taken before the editor mounts is a blank white image and
	// must not be accepted as visual proof.  Wait for a real document body
	// before running the action or capturing the result.
	waitPageReady(ws, 30*time.Second)
	if (strings.HasPrefix(method, "actions.") || strings.HasPrefix(method, "sdk.")) && !waitEditorReady(ws, 35*time.Second, method) {
		return nil, errors.New("页面已打开但编辑器未加载完成（等待 35 秒）；未执行页面动作")
	}
	// 就绪判定命中后布局仍可能在做最后一帧调整；给一帧稳定时间再调用，避免
	// showSidebar 在 UI 层尚未挂载时被吞掉。
	if strings.HasPrefix(method, "sdk.") {
		time.Sleep(1 * time.Second)
	}
	// 面板类动作会被页面的 focus/visibility 逻辑影响；无头或后台窗口里侧栏
	// 可能因此不挂载。显式置前并开启焦点模拟，让页面按真实前台行为渲染。
	if strings.HasPrefix(method, "sdk.history.") || strings.HasPrefix(method, "sdk.comments.") || method == "sdk.print" {
		_, _ = ws.Call(1400, "Page.bringToFront", nil, 5*time.Second)
		_, _ = ws.Call(1401, "Emulation.setFocusEmulationEnabled", map[string]any{"enabled": true}, 5*time.Second)
	}
	argsJSON, _ := json.Marshal(args)
	// 单元格历史没有对外 SDK 方法，必须走真实右键菜单。合成 MouseEvent 不会
	// 触发产品菜单（它监听的是原生输入），因此这条路径单独用 CDP 输入事件。
	if method == "sdk.cellHistory" {
		return runCellHistory(ws, screenshot)
	}
	expr := phase2Expression(method, string(argsJSON), guidFromURL(target))
	// The editor mounts asynchronously after the document shell is ready. A
	// missing SDK in the first second usually means "not initialized yet", not
	// "this page will never support the action". Retry a small, bounded number
	// of times so a slow editor does not turn into a hard failure.
	editorAttempts := 5
	editorRetryDelay := 2 * time.Second
	if strings.HasPrefix(method, "sdk.") {
		// 根级 SDK（version/history/print）比编辑器外壳晚初始化，给足冷启动时间。
		editorAttempts, editorRetryDelay = 9, 3*time.Second
	}
	var value any
	var lastErr error
	attempts := 0
	for i := 0; i < editorAttempts; i++ {
		attempts++
		res, callErr := ws.Call(3+i, "Runtime.evaluate", map[string]any{"expression": expr, "awaitPromise": true, "returnByValue": true}, 10*time.Second)
		if callErr == nil {
			value, lastErr = phase2BrowserResult(res)
			if lastErr == nil {
				break
			}
			if !isRetryableEditorError(lastErr) {
				break
			}
		} else {
			lastErr = callErr
		}
		if i < editorAttempts-1 {
			time.Sleep(editorRetryDelay)
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("页面 editor SDK/API 不可用（已尝试 %d 次；请用宿主页面操作，浏览器选择见 docs/scenarios.md#浏览器优先级）：%w", attempts, lastErr)
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

func phase2BrowserResult(res map[string]any) (any, error) {
	if _, exists := res["exceptionDetails"]; exists {
		return nil, errors.New("页面执行 SDK 方法时抛出异常")
	}
	result, ok := res["result"].(map[string]any)
	if !ok {
		return nil, errors.New("页面动作响应缺少 Runtime.result 对象，不能确认执行结果")
	}
	if str(result, "subtype") == "error" {
		return nil, errors.New(str(result, "description"))
	}
	value, ok := result["value"].(map[string]any)
	if !ok {
		return nil, errors.New("页面动作未返回结果对象，不能确认执行结果")
	}
	if value["ok"] == false {
		return nil, errors.New(str(value, "error"))
	}
	if value["ok"] != true {
		return nil, errors.New("页面动作结果缺少明确成功标记，不能确认执行结果")
	}
	return value, nil
}

// cellHistoryRectJS 定位一个可右键的数据单元格。表格是 canvas 渲染，取画布
// 内的固定偏移（跳过行号列与列头）即可落在 A1 附近的数据区。
const cellHistoryRectJS = `(()=>{const c=document.querySelector('canvas');if(!c)return {ok:false,error:'当前页面没有表格画布，不像是表格页'};const b=c.getBoundingClientRect();return {ok:true,x:Math.round(b.left+110),y:Math.round(b.top+70)}})()`

// 产品页的右键菜单在 shadow DOM 内，需要深度遍历。返回菜单项中心坐标，改用
// 真实鼠标点击（合成 click 对产品菜单无效）。
const cellHistoryMenuItemJS = `(()=>{const deep=[];const walk=r=>{for(const el of r.querySelectorAll('*')){deep.push(el);if(el.shadowRoot)walk(el.shadowRoot)}};walk(document);
for(const el of deep){const t=(el.textContent||'').trim();if(t.includes('查看单元格历史')&&t.length<30){const b=el.getBoundingClientRect();if(b.width>0&&b.height>0)return {ok:true,x:Math.round(b.left+b.width/2),y:Math.round(b.top+b.height/2)}}}
return {ok:false}})()`

// 评论提交按钮的文案可能是"确定/确认"，且可能被图标或空格包裹，用正则匹配
// 并取面积最小的可见节点。

// 评论卡片以"按 Enter 发送"作为标识；输入框必须从卡片内部取，否则会命中
// 单元格自己的编辑器（页面里更靠前），把评论写进单元格。

// 评论输入框的提示在 shadow DOM 内，body.innerText 读不到，必须深度遍历。

// 表格用隐藏的 2×2 contenteditable 代理键盘输入；真正的评论输入框有可见尺寸。
// 只用尺寸判断，避免把单元格编辑态当成评论框。

// 直接在页面里点击包含指定文案的最小可见节点。产品菜单项是 React 节点，
// 合成 click 能被它的 onClick 接住，比按坐标点击更不容易落偏到单元格上。

const cellHistoryPanelJS = `(()=>{const deep=[];const walk=r=>{for(const el of r.querySelectorAll('*')){deep.push(el);if(el.shadowRoot)walk(el.shadowRoot)}};walk(document);
const panels=deep.filter(el=>{const b=el.getBoundingClientRect();return b.width>160&&b.height>50&&/单元格历史/.test(el.innerText||'')}).map(el=>({cls:String(el.className).slice(0,80),text:(el.innerText||'').trim().slice(0,200)}));
return {ok:panels.length>0,panelVisible:panels.length>0,panels:panels.slice(0,3)}})()`

// 菜单项/按钮定位：同名节点可能层层包裹，取面积最小的可见节点才是可点目标。

const commentInputFocusJS = `(()=>{const deep=[];const walk=r=>{for(const el of r.querySelectorAll('*')){deep.push(el);if(el.shadowRoot)walk(el.shadowRoot)}};walk(document);
for(const el of deep){const ok=el.isContentEditable||el.tagName==='TEXTAREA'||(el.tagName==='INPUT'&&String(el.type||'text').toLowerCase()==='text');if(!ok)continue;const b=el.getBoundingClientRect();if(b.width>80&&b.height>18&&b.top>0){el.focus();return {ok:true,tag:el.tagName}}}
return {ok:false}})()`

func phase2EvalJS(ws *WSConn, id int, expr string) (map[string]any, error) {
	res, err := ws.Call(id, "Runtime.evaluate", map[string]any{"expression": expr, "awaitPromise": true, "returnByValue": true}, 15*time.Second)
	if err != nil {
		return nil, err
	}
	result, ok := res["result"].(map[string]any)
	if !ok {
		return nil, errors.New("页面没有返回结果对象")
	}
	value, ok := result["value"].(map[string]any)
	if !ok {
		return nil, errors.New("页面返回格式异常，不能确认执行结果")
	}
	return value, nil
}

func phase2Coord(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func runCellHistory(ws *WSConn, screenshot string) (any, error) {
	point, err := phase2EvalJS(ws, 2000, cellHistoryRectJS)
	if err != nil {
		return nil, err
	}
	if point["ok"] != true {
		return nil, errors.New(str(point, "error"))
	}
	x, y := phase2Coord(point["x"]), phase2Coord(point["y"])
	raw := func(id int, kind, button string, buttons int) {
		_, _ = ws.Call(id, "Input.dispatchMouseEvent", map[string]any{"type": kind, "x": x, "y": y, "button": button, "buttons": buttons, "clickCount": 1}, 5*time.Second)
	}
	_, _ = ws.Call(1999, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": x, "y": y}, 5*time.Second)
	raw(2001, "mousePressed", "right", 2)
	raw(2002, "mouseReleased", "right", 0)
	var item map[string]any
	id := 2010
	for i := 0; i < 12; i++ {
		time.Sleep(300 * time.Millisecond)
		v, e := phase2EvalJS(ws, id, cellHistoryMenuItemJS)
		id++
		if e == nil && v["ok"] == true {
			item = v
			break
		}
	}
	if item == nil {
		return nil, errors.New("右键菜单没有出现“查看单元格历史”，该位置可能不是数据单元格")
	}
	ix, iy := phase2Coord(item["x"]), phase2Coord(item["y"])
	for _, step := range []struct {
		id   int
		kind string
		btn  int
	}{{2100, "mouseMoved", 0}, {2101, "mousePressed", 1}, {2102, "mouseReleased", 0}} {
		_, _ = ws.Call(step.id, "Input.dispatchMouseEvent", map[string]any{"type": step.kind, "x": ix, "y": iy, "button": "left", "buttons": step.btn, "clickCount": 1}, 5*time.Second)
	}
	var panel map[string]any
	id = 2110
	for i := 0; i < 14; i++ {
		time.Sleep(300 * time.Millisecond)
		v, e := phase2EvalJS(ws, id, cellHistoryPanelJS)
		id++
		if e == nil && v["ok"] == true && v["panelVisible"] == true {
			panel = v
			break
		}
	}
	if panel == nil {
		return nil, errors.New("已点击“查看单元格历史”，但没有检测到面板渲染")
	}
	if screenshot != "" {
		if err := capturePhase2Screenshot(ws, screenshot); err != nil {
			return nil, err
		}
	}
	return panel, nil
}

func phase2PollJS(ws *WSConn, id int, expr string, tries int, delay time.Duration) map[string]any {
	for i := 0; i < tries; i++ {
		time.Sleep(delay)
		v, err := phase2EvalJS(ws, id+i, expr)
		if err == nil && v["ok"] == true {
			return v
		}
	}
	return nil
}

func phase2ClickPoint(ws *WSConn, baseID int, x, y float64) {
	for _, s := range []struct {
		id   int
		kind string
		btn  int
	}{{baseID, "mouseMoved", 0}, {baseID + 1, "mousePressed", 1}, {baseID + 2, "mouseReleased", 0}} {
		_, _ = ws.Call(s.id, "Input.dispatchMouseEvent", map[string]any{"type": s.kind, "x": x, "y": y, "button": "left", "buttons": s.btn, "clickCount": 1}, 5*time.Second)
	}
}

func sameCookieHost(target string) bool {
	configured, _ := resolveOrigin()
	return sameCredentialOrigin(target, configured)
}

func phase2Expression(method, argsJSON, guid string) string {
	switch method {
	case "sdk.history.show", "sdk.history.hide", "sdk.comments.show", "sdk.comments.hide", "sdk.version.createRevision", "sdk.print":
		var args any
		_ = json.Unmarshal([]byte(argsJSON), &args)
		kind := map[string]string{
			"sdk.history.show": "history-show", "sdk.history.hide": "history-hide",
			"sdk.comments.show": "comments-show", "sdk.comments.hide": "comments-hide",
			"sdk.version.createRevision": "save-version", "sdk.print": "print",
		}[method]
		return phase2SDKCallExpression(kind, args)
	}
	if method == "sdk.cellHistory" {
		return cellHistoryJS
	}
	if method == "browser.verify" {
		return `(async()=>{const path=location.pathname||"";const text=(document.body?.innerText||"").trim();if(path==="/login"||text.includes("验证码登录")||text.includes("欢迎使用石墨办公"))return {ok:false,error:"页面未登录，无法完成视觉核验"};if(document.readyState!=="complete")return {ok:false,error:"页面尚未渲染完成，不能作为视觉核验证据"};if(!document.querySelector("canvas")&&text.length<20)return {ok:false,error:"页面尚未渲染完成，不能作为视觉核验证据"};return {ok:true,textLength:text.length,canvas:document.querySelectorAll("canvas").length};})()`
	}
	parts := strings.Split(method, ".")
	pathJSON, _ := json.Marshal(parts)
	return fmt.Sprintf(`(async()=>{const sdk=window.__SM__?.editor||window.smsdk?.__DRIVE_SDK__?.editor||globalThis.sdk;if(!sdk)return {ok:false,error:"页面未暴露 editor SDK"};const path=%s;let owner=sdk;for(let i=0;i<path.length-1;i++){owner=owner?.[path[i]]}const fn=owner?.[path[path.length-1]];if(typeof fn!=="function")return {ok:false,error:"SDK 方法不可用："+path.join(".")};const args=%s;const value=await fn.apply(owner,args);return {ok:true,value};})()`, string(pathJSON), argsJSON)
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

func initializePhase2Session(ws *WSConn, target, cookie string) error {
	if err := setCookies(ws, target, cookie); err != nil {
		// CDP errors may echo cookie parameters; never print their raw message.
		return errors.New("浏览器会话注入失败；未执行页面动作，请检查登录状态和浏览器连接")
	}
	if _, err := ws.Call(102, "Page.reload", map[string]any{"ignoreCache": true}, 10*time.Second); err != nil {
		return errors.New("浏览器会话已注入，但刷新页面失败；未执行页面动作，请检查浏览器连接")
	}
	return nil
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

// phase2ReadyExpression 针对具体动作判断"它要调的能力"是否已挂载。产品页的
// __SM__ 会分阶段挂 editor / sdk，不同套件暴露的能力也不同（文稿有 print 但
// 不一定有 history）。按动作精确等待，避免"有别的能力"就提前放行。
//
// 注意：history.show 这类 facade 在 editor 构造时就已存在，远早于界面渲染
// 完成。若此时调用，showSidebar 会指向尚未挂载的 UI 层而静默失效。因此除了
// 方法存在，还必须确认 editor.initialized、sdk._rendered 与正文已渲染。
func phase2ReadyExpression(method string) string {
	var probe string
	switch {
	case strings.HasPrefix(method, "sdk.history."):
		probe = `!!(sm.editor?.history?.show||sm.sdk?.history?.show)`
	case strings.HasPrefix(method, "sdk.comments."):
		probe = `!!(sm.editor?.comments?.show||sm.sdk?.comments?.show)`
	case method == "sdk.print":
		probe = `!!(sm.editor?.print||sm.sdk?.print)`
	case strings.HasPrefix(method, "sdk.version."):
		probe = `!!(sm.editor?.version?.createRevision||sm.editor?.editor?.createRevision||sm.editor?.createRevision||sm.sdk?.version?.createRevision)`
	default:
		probe = `true`
	}
	return `(()=>{const sm=window.__SM__;
if(!sm)return !!(window.smsdk?.__DRIVE_SDK__?.editor)||!!globalThis.sdk;
const ed=sm.editor;
if(ed?.initialized===false)return false;
if(sm.sdk?._rendered===false)return false;
// 不同套件的"渲染完成"信号不同：文档/文稿是 editor.initialized，
// 表格是 canvas 首帧。任一成立且页面已有可见文本即视为就绪。
const rendered=(ed?.initialized===true)||(sm.sdk?._rendered===true)||!!document.querySelector('canvas');
if(!rendered)return false;
if(!(document.body?.innerText||'').trim())return false;
return ` + probe + `})()`
}

func waitEditorReady(ws *WSConn, timeout time.Duration, method string) bool {
	deadline := time.Now().Add(timeout)
	id := 1200
	expr := phase2ReadyExpression(method)
	for time.Now().Before(deadline) {
		res, err := ws.Call(id, "Runtime.evaluate", map[string]any{
			"expression":    expr,
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
	return captureBrowserScreenshotTimed(ws, path, 15*time.Second)
}

func captureBrowserScreenshotTimed(ws *WSConn, path string, timeout time.Duration) error {
	res, err := ws.Call(101, "Page.captureScreenshot", map[string]any{"format": "png"}, timeout)
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
