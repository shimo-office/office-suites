package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"
)

// Table APIs already validate the authenticated file type. The browser is used
// only for the server's live presence feed; identity comes independently from
// the authenticated /users/me endpoint, not a guessed page-global SDK object.
type phase3TablePresence struct {
	browser   *phase3EditorBrowser
	userID    string
	target    string
	readiness string
}

func (g *phase3TablePresence) Close() { g.browser.Close() }
func (g *phase3TablePresence) CheckPresence() error {
	id, _ := json.Marshal(g.userID)
	_, err := g.browser.EvalReadOnly(`(()=>{const p=window.__shimoPhase3Presence?.();const id=` + string(id) + `;if(!p?.ready||!p.connected||!id)return {ok:false,error:"服务端在线协作名单尚未初始化，拒绝写入"};if(p.ids.some(x=>String(x)!==id))return {ok:false,error:"检测到其他用户在线协作，拒绝写入"};return {ok:true,source:"COLLABROOM/RECONNECT_READY",otherUsers:0}})()`)
	if err != nil {
		return fmt.Errorf("表格协作名单只读检查失败：%w", err)
	}
	return err
}
func init() {
	phase3OpenTableGuard = func(target string) (phase3TableGuard, error) {
		a := newApp([]string{"phase3"})
		if !phase3SameOrigin(target, a.Origin) {
			return nil, errors.New("协作门禁目标与登录 origin 不一致")
		}
		browser, err := openPhase3TableBrowser(target)
		if err != nil {
			return nil, err
		}
		me, err := a.usersMe()
		if err != nil {
			browser.Close()
			return nil, err
		}
		id := str(me, "id")
		if id == "" {
			browser.Close()
			return nil, errors.New("当前用户身份未知，拒绝写入")
		}
		guard := &phase3TablePresence{browser: browser, userID: id, target: target}
		if err = guard.CheckPresence(); err != nil {
			guard.Close()
			return nil, err
		}
		return guard, nil
	}
}

func (g *phase3TablePresence) Capture(path string) error {
	readiness := g.readiness
	if readiness == "" {
		readiness = phase3TableGridReady
	}
	if err := g.browser.WaitReady(readiness, phase3EditorReadyTimeout); err != nil {
		return fmt.Errorf("表格截图前页面就绪检查失败：%w", err)
	}
	if err := g.CheckPresence(); err != nil {
		return err
	}
	if err := g.browser.Capture(path); err != nil {
		return fmt.Errorf("表格 Page.captureScreenshot 失败：%w", err)
	}
	return nil
}
func (g *phase3TablePresence) FreshCapture(path string) error {
	// The mutation and its API proof are complete; close the old editor before
	// opening the independent profile so verification does not double memory.
	g.browser.Close()
	b, err := openPhase3TableBrowser(g.target)
	if err != nil {
		return err
	}
	g.browser = b
	next := &phase3TablePresence{browser: b, userID: g.userID, target: g.target, readiness: g.readiness}
	if err = next.CheckPresence(); err != nil {
		return err
	}
	if err := next.Capture(path); err != nil {
		return fmt.Errorf("表格新配置 Page.captureScreenshot 失败：%w", err)
	}
	return nil
}

// Use the shared single-profile readiness budget, including the authoritative
// collaboration feed. A timeout never falls back to an empty online list.
func openPhase3TableBrowser(target string) (*phase3EditorBrowser, error) {
	return openPhase3EditorWhen(target, `document.readyState==="complete"&&location.href!=="about:blank"&&Boolean(window.__shimoPhase3Presence?.().ready&&window.__shimoPhase3Presence?.().connected)`)
}

// These are native grid selectors from the loaded grid-view SDK, not text or
// generic canvas heuristics. Presence alone can precede the first grid paint.
const phase3TableGridReady = `(()=>{const p=window.__shimoPhase3Presence?.();if(!p?.ready||!p.connected)return false;const visible=e=>{if(!e)return false;const r=e.getBoundingClientRect(),s=getComputedStyle(e);return r.width>0&&r.height>0&&s.visibility!=="hidden"&&s.display!=="none"};return [...document.querySelectorAll('.sm-grid-view-container')].some(grid=>visible(grid)&&!grid.classList.contains('first-paint-pending')&&[...grid.querySelectorAll('.header-field[data-field-guid]')].some(visible))})()`

func phase3TableTargetURL(target, tableGuid string) (string, error) {
	u, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if tableGuid != "" {
		q := u.Query()
		q.Set("table", tableGuid)
		q.Del("view")
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}
func (g *phase3TablePresence) NavigateTable(tableGuid string) error {
	target, err := phase3TableTargetURL(g.target, tableGuid)
	if err != nil {
		return err
	}
	previous, err := g.browser.EvalReadOnly(`performance.timeOrigin`)
	if err != nil {
		return err
	}
	origin, ok := previous.(float64)
	if !ok {
		return errors.New("表格导航generation缺失")
	}
	if _, err = g.browser.ws.Call(707, "Page.navigate", map[string]any{"url": target}, 15*time.Second); err != nil {
		return err
	}
	raw, _ := json.Marshal(origin)
	ready := `performance.timeOrigin>` + string(raw) + `&&new URLSearchParams(location.search).get('table')===` + jsonString(tableGuid) + `&&(` + phase3TableGridReady + `)`
	if err = g.browser.WaitReady(ready, phase3EditorReadyTimeout); err != nil {
		return err
	}
	g.target = target
	g.readiness = `new URLSearchParams(location.search).get('table')===` + jsonString(tableGuid) + `&&(` + phase3TableGridReady + `)`
	return g.CheckPresence()
}
