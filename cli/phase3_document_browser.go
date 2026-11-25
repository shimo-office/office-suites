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

const phase3EditorReadyTimeout = 300 * time.Second
const phase3EditorReadOnlyTimeout = 90 * time.Second

// The editor bridge executes fixed, request-specific operations only. It does
// not expose arbitrary JavaScript or an unrestricted SDK method to CLI callers.
type phase3EditorBrowser struct {
	ws               *WSConn
	cmd              *exec.Cmd
	profile          string
	logPath          string
	cleanupLog       func()
	debugURL         string
	nextID           int
	readyAttempts    int
	startupEvidence  string
	presenceObserver string
	navURL           string
	// 轻文档页面不挂 window.logged，协作名单里的当前用户只能靠 CLI 传入并记住，
	// 长操作中途的每次写入复核都要用到它。
	lightDocUserID string
}

func openPhase3Editor(target string) (*phase3EditorBrowser, error) {
	return openPhase3EditorWhen(target, `Boolean((typeof window._sdk?.editor?.getParagraphs==="function"&&typeof window._sdk?.editor?.docsApi?.setFontSize==="function")||(typeof window.presentationSDK?.API?.slides?.getSlides==="function"&&window.presentationSDK?.editor?.workbench?.isReady()))`)
}

// Offline tests replace only the process-launch boundary.
var phase3StartEditor = openPhase3EditorOnce

// readiness is fixed by the typed command, never a user-provided expression.
func openPhase3EditorWhen(target, readiness string) (*phase3EditorBrowser, error) {
	// A failed launch must not replace the user's window with a new profile.
	// Readiness can reconnect the existing target without restarting the process.
	browser, err := phase3StartEditor(target, readiness)
	if err != nil {
		return nil, fmt.Errorf("编辑器启动失败，已停止且不自动重开窗口：%w", err)
	}
	return browser, nil
}
func openPhase3EditorOnce(target, readiness string) (*phase3EditorBrowser, error) {
	return openPhase3EditorWithSetup(target, readiness, nil)
}

// setup installs read-only diagnostics before the initial navigation.
func openPhase3EditorWithSetup(target, readiness string, setup func(*phase3EditorBrowser) error) (*phase3EditorBrowser, error) {
	if explicitNoBrowser(&App{}) {
		return nil, errors.New("当前设置禁止启动浏览器；本次未创建或启动隔离编辑器")
	}
	if !sameCookieHost(target) {
		return nil, errors.New("目标 host 与登录 origin 不一致；未启动编辑器")
	}
	chrome, err := findChrome()
	if err != nil {
		return nil, err
	}
	profile, err := os.MkdirTemp("", "shimo-phase3-editor-")
	if err != nil {
		return nil, err
	}
	b := &phase3EditorBrowser{profile: profile, nextID: 200, navURL: target}
	b.cmd = phase3ChromeCommand(chrome, chromeLaunchArgs("--no-first-run", "--no-default-browser-check", "--disable-background-timer-throttling", "--disable-renderer-backgrounding", "--disable-backgrounding-occluded-windows", "--user-data-dir="+profile, "--remote-debugging-port=0", "about:blank")...)
	b.logPath, b.cleanupLog = captureBrowserOutput(b.cmd)
	if err = b.cmd.Start(); err != nil {
		b.Close()
		return nil, err
	}
	fail := func(e error) (*phase3EditorBrowser, error) { b.Close(); return nil, e }
	port, err := waitDevToolsPort(filepath.Join(profile, "DevToolsActivePort"), 45*time.Second)
	if err != nil {
		return fail(explainBrowserFailure(err, b.logPath))
	}
	wsURL, err := waitPageTarget(port, "about:blank")
	if err != nil {
		return fail(err)
	}
	b.debugURL = wsURL
	b.ws, err = dialWS(wsURL)
	if err != nil {
		return fail(explainBrowserFailure(err, b.logPath))
	}
	cookie, err := authCookie()
	if err != nil {
		return fail(err)
	}
	if err = setCookies(b.ws, target, cookie); err != nil {
		return fail(fmt.Errorf("无法设置隔离编辑器登录态：%w", explainBrowserFailure(err, b.logPath)))
	}
	observer, _ := json.Marshal(guidFromURL(target))
	b.presenceObserver = phase3PresenceObserver(string(observer))
	if err = b.installPresenceObserver(time.Now().Add(20 * time.Second)); err != nil {
		return fail(err)
	}
	if setup != nil {
		if err = setup(b); err != nil {
			return fail(err)
		}
	}
	initialDeadline := time.Now().Add(phase3EditorReadyTimeout)
	if err = b.navigateInitial(target, initialDeadline); err != nil {
		return fail(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if err = b.waitInitialReady(readiness, time.Until(initialDeadline)); err != nil {
		return fail(b.captureReadinessFailure(err))
	}
	return b, nil
}
func (b *phase3EditorBrowser) Close() {
	if b.ws != nil {
		_, _ = b.ws.Call(9000, "Browser.close", nil, 2*time.Second)
		b.ws.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		_ = b.cmd.Wait()
	}
	_ = os.RemoveAll(b.profile)
	if b.cleanupLog != nil {
		b.cleanupLog()
		b.cleanupLog = nil
	}
	b.logPath = ""
}
func (b *phase3EditorBrowser) Eval(expression string) (any, error) {
	return b.evalTimed(expression, 30*time.Second)
}

// EvalReadOnly is reserved for fixed non-mutating probes; callers must never
// route a write expression through this longer budget. It sends exactly once.
func (b *phase3EditorBrowser) EvalReadOnly(expression string) (any, error) {
	value, err := b.evalTimed(expression, phase3EditorReadOnlyTimeout)
	if err != nil {
		return nil, fmt.Errorf("编辑器只读 Runtime.evaluate（上限%s）失败：%w", phase3EditorReadOnlyTimeout, err)
	}
	return value, nil
}
func (b *phase3EditorBrowser) EvalUserGesture(expression string) (any, error) {
	return b.evalTimedGesture(expression, 30*time.Second, true)
}
func (b *phase3EditorBrowser) evalTimed(expression string, timeout time.Duration) (any, error) {
	return b.evalTimedGesture(expression, timeout, false)
}
func (b *phase3EditorBrowser) evalTimedGesture(expression string, timeout time.Duration, gesture bool) (any, error) {
	b.nextID++
	params := map[string]any{"expression": expression, "awaitPromise": true, "returnByValue": true}
	if gesture {
		params["userGesture"] = true
	}
	r, err := b.ws.Call(b.nextID, "Runtime.evaluate", params, timeout)
	if err != nil {
		return nil, err
	}
	if detail, ok := r["exceptionDetails"].(map[string]any); ok {
		exception, _ := detail["exception"].(map[string]any)
		message := strings.Split(str(exception, "description"), "\n")[0]
		if len(message) > 200 {
			message = message[:200]
		}
		return nil, fmt.Errorf("编辑器动作抛出异常（%s）；可能未完成，禁止自动重放", message)
	}
	v, ok := r["result"].(map[string]any)
	if !ok {
		return nil, errors.New("编辑器未返回可验证结果")
	}
	if m, ok := v["value"].(map[string]any); ok && m["ok"] == false {
		return nil, fmt.Errorf("编辑器拒绝执行：%s", str(m, "error"))
	}
	return v["value"], nil
}

// Observe authoritative presence messages before the app creates its socket.
// Store IDs only: cookies, session tokens and raw network payloads are excluded.
func phase3PresenceObserver(guidJSON string) string {
	return `(()=>{
if(typeof window.__shimoPhase3Presence==='function')return;
const expected=` + guidJSON + `;let state={ready:false,connected:false,ids:[],event:"pending",guid:""};let targetSource=null;let nextSocket=0,targetGeneration=0;const socketGenerations=new WeakMap();
const inspect=(o,source)=>{if(!o||typeof o!=="object")return;if(Array.isArray(o)){for(const x of o)inspect(x,source);return}if(o.type!=="COLLABROOM"||!o.data)return;const d=o.data,g=d.fileGuid||o.fileGuid||d.padId||o.padId;const type=d.type;if(type!=="RECONNECT_READY"&&type!=="USER_ENTER"&&type!=="USER_LEAVE")return;if(g&&g!==expected)return;if(!(source instanceof Native)||source.readyState!==Native.OPEN)return;if(g!==expected&&source!==targetSource)return;if(type==="RECONNECT_READY"){if(g!==expected)return;const generation=socketGenerations.get(source);if(!generation||generation<targetGeneration)return;targetSource=source;targetGeneration=generation;if(!Array.isArray(d.clients)){state={...state,ready:false,event:"malformed"};return}const ids=d.clients.map(x=>x?.id);if(ids.some(x=>typeof x!=="number"&&typeof x!=="string")){state={...state,ready:false,event:"malformed"};return}state={ready:true,connected:true,ids:ids.map(String),event:type,guid:g,transport:"websocket",binding:"incoming-fileGuid"};return}if(!state.ready||source!==targetSource)return;const id=d.userInfo?.id;if(typeof id!=="number"&&typeof id!=="string"){state={...state,ready:false,event:"malformed"};return}state={...state,event:type,ids:type==="USER_ENTER"?[...new Set([...state.ids,String(id)])]:state.ids.filter(x=>x!==String(id))}};
const receive=(raw,source)=>{if(typeof raw!=="string"||raw.length>1048576)return;for(let frame of raw.split("\x1e")){frame=frame.trim();if(!frame.startsWith("{")&&!frame.startsWith("[")){const packet=frame.match(/^\d+(?:\/[^,]+,)?([\[{].*)$/s);if(!packet)continue;frame=packet[1]}try{inspect(JSON.parse(frame),source)}catch{}}};
const Native=window.WebSocket;window.WebSocket=new Proxy(Native,{construct(Target,args){const ws=Reflect.construct(Target,args);socketGenerations.set(ws,++nextSocket);ws.addEventListener("message",e=>receive(e.data,ws));ws.addEventListener("close",()=>{if(ws!==targetSource)return;state={...state,ready:false,connected:false,event:"closed"}});ws.addEventListener("error",()=>{if(ws!==targetSource)return;state={...state,ready:false,connected:false,event:"error"}});return ws}});
Object.defineProperty(window,"__shimoPhase3Presence",{value:()=>({...state,connected:state.connected&&targetSource?.readyState===Native.OPEN,ids:[...state.ids]}),configurable:false});
})()`
}

func phase3PresenceGuardJS() string {
	return `const presence=window.__shimoPhase3Presence?.();const current=window._sdk?.options?.user?.id??window.presentationSDK?.editor?.workbench?.context?.getUser?.()?.id??window.logged?.user?.id??window.logged?.edit?.query?.permissions?.DEBUG_getContext?.()?.user?.id;if(!presence?.ready||!presence.connected||current==null)return {ok:false,error:"当前协作名单检查未通过：尚未收到服务端在线名单"};if(presence.ids.some(id=>String(id)!==String(current)))return {ok:false,error:"当前协作名单检查未通过：检测到其他用户正在协作"};`
}

// CheckPresence fails closed until the server has initialized the online list.
// Call again immediately before sending any mutation; this never mutates content.
func (b *phase3EditorBrowser) CheckPresence() error {
	_, err := b.EvalReadOnly(`(()=>{` + phase3PresenceGuardJS() + `return {ok:true,otherUsers:0,source:"COLLABROOM/RECONNECT_READY"}})()`)
	if err != nil {
		return fmt.Errorf("协作名单只读门禁失败：%w", err)
	}
	return nil
}

// WaitReady polls only read-only readiness; it never reloads or replays an action.
type phase3ReadinessFailure struct{ err error }

func (e *phase3ReadinessFailure) Error() string { return e.err.Error() }
func (e *phase3ReadinessFailure) Unwrap() error { return e.err }

func (b *phase3EditorBrowser) WaitReady(readiness string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	attempts, err := phase3PollReady(timeout, time.Second, func() (map[string]any, error) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return map[string]any{"ready": false, "budgetExpired": true}, nil
		}
		rpcTimeout := remaining
		if rpcTimeout > phase3EditorReadOnlyTimeout {
			rpcTimeout = phase3EditorReadOnlyTimeout
		}
		v, e := b.evalTimed(`(()=>{const p=window.__shimoPhase3Presence?.();return {ready:Boolean(`+readiness+`),login:location.pathname.startsWith('/login'),documentState:document.readyState,visibility:document.visibilityState,hasFocus:document.hasFocus(),pageElapsedMs:Math.round(performance.now()),path:location.pathname,sdk:typeof window._sdk,presentationSDK:typeof window.presentationSDK,canvasCount:document.querySelectorAll('canvas').length,presence:p?{ready:p.ready,connected:p.connected,event:p.event,guid:p.guid,idsCount:p.ids.length}:null}})()`, rpcTimeout)
		if e != nil {
			return nil, e
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("就绪检查返回结构无效")
		}
		return m, nil
	})
	b.readyAttempts = attempts
	if err != nil {
		return &phase3ReadinessFailure{fmt.Errorf("就绪预算 %s（单次只读RPC上限90s，不重开丢弃进度）：%w", timeout, err)}
	}
	return nil
}
func phase3PollReady(timeout, interval time.Duration, probe func() (map[string]any, error)) (int, error) {
	deadline := time.Now().Add(timeout)
	attempts := 0
	var last map[string]any
	for {
		if attempts > 0 && time.Now().After(deadline) {
			break
		}
		attempts++
		state, err := probe()
		if err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "编辑器动作抛出异常") || strings.Contains(msg, "编辑器拒绝执行") {
				detail, _ := json.Marshal(last)
				return attempts, fmt.Errorf("编辑器就绪检查异常，第 %d 次（last=%s）：%w", attempts, detail, err)
			}
			transient := strings.Contains(msg, "execution context was destroyed") || strings.Contains(msg, "cannot find context") || strings.Contains(msg, "cannot find default execution context") || strings.Contains(msg, "inspected target navigated or closed")
			if !transient {
				detail, _ := json.Marshal(last)
				return attempts, fmt.Errorf("编辑器就绪检查异常，第 %d 次（last=%s）：%w", attempts, detail, err)
			}
			last = map[string]any{"documentState": "navigation-context-unavailable"}
		} else {
			if state["budgetExpired"] == true {
				break
			}
			last = state
			if state["login"] == true {
				return attempts, errors.New("隔离编辑器未登录；本次就绪检查为只读")
			}
			if state["ready"] == true {
				return attempts, nil
			}
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		delay := interval
		if delay > remaining {
			delay = remaining
		}
		time.Sleep(delay)
	}
	detail, _ := json.Marshal(last)
	return attempts, fmt.Errorf("等待编辑器或目标方法超时（%s，%d 次，last=%s）；本次就绪检查为只读", timeout, attempts, detail)
}

func (b *phase3EditorBrowser) Capture(filename string) error {
	if b.startupEvidence != "" {
		data, err := os.ReadFile(b.startupEvidence)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(filepath.Dir(filename), "startup-recovery.json"), data, 0600); err != nil {
			return err
		}
	}
	if err := captureBrowserScreenshotTimed(b.ws, filename, phase3EditorReadOnlyTimeout); err != nil {
		return fmt.Errorf("编辑器 Page.captureScreenshot（上限%s）失败：%w", phase3EditorReadOnlyTimeout, err)
	}
	if !screenshotHasContent(filename) {
		return errors.New("编辑器截图为空白，不能作为验收物证")
	}
	return nil
}

// A timed-out WebSocket may have consumed only part of a frame. Diagnostics
// therefore use a new connection and never reuse the failed RPC stream.
func (b *phase3EditorBrowser) captureReadinessFailure(cause error) error {
	dir, err := os.MkdirTemp("", "shimo-phase3-browser-failure-")
	if err != nil {
		return fmt.Errorf("%w；无法创建浏览器诊断目录：%v", cause, err)
	}
	diagnostic := map[string]any{"cause": cause.Error(), "captureBudgetSeconds": 3, "screenshotCaptured": false, "screenshotNonblank": false}
	deadline := time.Now().Add(3 * time.Second)
	shot := filepath.Join(dir, "page.png")
	captureErr := func() error {
		if b.debugURL == "" {
			return errors.New("没有可连接的CDP目标")
		}
		ws, e := dialWSWithTimeout(b.debugURL, time.Until(deadline))
		if e != nil {
			return e
		}
		defer ws.Close()
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errors.New("截图连接已用尽3秒预算")
		}
		result, e := ws.Call(1, "Page.captureScreenshot", map[string]any{"format": "png", "captureBeyondViewport": false}, remaining)
		if e != nil {
			return e
		}
		raw, e := base64.StdEncoding.DecodeString(str(result, "data"))
		if e != nil || len(raw) == 0 {
			return errors.New("CDP没有返回有效截图")
		}
		if e = os.WriteFile(shot, raw, 0600); e != nil {
			return e
		}
		diagnostic["screenshotCaptured"] = true
		diagnostic["screenshotNonblank"] = screenshotHasContent(shot)
		diagnostic["screenshot"] = shot
		return nil
	}()
	if captureErr != nil {
		diagnostic["captureError"] = captureErr.Error()
	}
	if e := phase3WriteEvidence(filepath.Join(dir, "diagnostic.json"), diagnostic); e != nil {
		return fmt.Errorf("%w；浏览器诊断保存失败：%v", cause, e)
	}
	return fmt.Errorf("%w；浏览器诊断：%s（截图成功=%v）", cause, dir, diagnostic["screenshotCaptured"])
}
