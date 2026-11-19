package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Investigation is read-only and limited to isolated acceptance fixtures.
func TestPhase3TableFormEditorProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_TABLE_FORM_PROBE") != "1" {
		t.Skip("opt-in fixture probe")
	}
	for _, target := range []string{"https://shimo.net/forms/aqmmL0mOBlSKOjqY?debug=1"} {
		chrome, err := findChrome()
		if err != nil {
			t.Fatal(err)
		}
		profile := t.TempDir()
		cmd := exec.Command(chrome, chromeLaunchArgs("--no-first-run", "--no-default-browser-check", "--user-data-dir="+profile, "--remote-debugging-port=0", "about:blank")...)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			port, err := waitDevToolsPort(filepath.Join(profile, "DevToolsActivePort"), 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			wsURL, err := waitPageTarget(port, "about:blank")
			if err != nil {
				t.Fatal(err)
			}
			ws, err := dialWS(wsURL)
			if err != nil {
				t.Fatal(err)
			}
			defer ws.Close()
			cookie, err := authCookie()
			if err != nil {
				t.Fatal(err)
			}
			if !sameCookieHost(target) {
				t.Fatal("host mismatch")
			}
			if err := setCookies(ws, target, cookie); err != nil {
				t.Fatal(err)
			}
			_, err = ws.Call(102, "Page.navigate", map[string]any{"url": target}, 10*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			waitPageReady(ws, 30*time.Second)
			time.Sleep(15 * time.Second)
			expr := `(()=>{const keys=o=>{if(!o)return [];const a=[];for(let p=o;p&&p!==Object.prototype;p=Object.getPrototypeOf(p))a.push(...Object.getOwnPropertyNames(p));return [...new Set(a)].sort()};const l=window.logged;const src=(o,ks)=>Object.fromEntries(ks.map(k=>[k,String(o?.[k])]));return {url:location.pathname,body:(document.body?.innerText||'').slice(0,300),logged:keys(l),loggedChildren:l?Object.fromEntries(Object.keys(l).map(k=>[k,keys(l[k])])):{},formSources:src(l?.form,keys(l?.form).filter(k=>typeof l.form[k]==='function')),editQuery:l?.edit?.query?.getForm?.(),editor:keys(l?.editor),editorChildren:l?.editor?Object.fromEntries(Object.keys(l.editor).filter(k=>l.editor[k]&&typeof l.editor[k]==='object').map(k=>[k,keys(l.editor[k])])):{}}})()`
			result, err := ws.Call(104, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true}, 45*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.MarshalIndent(result, "", "  ")
			t.Log(string(b))
		}()
	}
}

func TestPhase3TablePresenceReadOnlyProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_TABLE_PRESENCE_PROBE") != "1" {
		t.Skip("opt-in presence fixture probe")
	}
	target := "https://shimo.net/tables/DqdxLrxlDMcw78q4"
	b, err := openPhase3EditorWhen(target, `location.pathname==="/tables/DqdxLrxlDMcw78q4"&&document.readyState==="complete"`)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		v, err := b.Eval(`(()=>({presence:window.__shimoPhase3Presence?.(),body:(document.body?.innerText||'').slice(0,240),url:location.pathname}))()`)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(v)
		t.Log(string(raw))
		m, _ := v.(map[string]any)
		p, _ := m["presence"].(map[string]any)
		if p["ready"] == true {
			return
		}
		time.Sleep(10 * time.Second)
	}
	_ = capturePhase2Screenshot(b.ws, "/Users/xuyixian/Downloads/shimo-ai-solution/github/office-suites/tmp/phase3-live/forms-tables/table-presence-timeout.png")
	t.Fatal("no authoritative presence after 120s")
}

func TestPhase3TableExistingSessionProbe(t *testing.T) {
	profile := os.Getenv("SHIMO_PHASE3_TABLE_EXISTING_PROFILE")
	if profile == "" {
		t.Skip("opt-in attached read-only probe")
	}
	port, err := waitDevToolsPort(filepath.Join(profile, "DevToolsActivePort"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	wsURL, err := waitPageTarget(port, "https://shimo.net/tables/DqdxLrxlDMcw78q4")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := dialWS(wsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	v, err := ws.Call(900, "Runtime.evaluate", map[string]any{"expression": `(()=>({path:location.pathname,state:document.readyState,presence:window.__shimoPhase3Presence?.(),predicate:Boolean(window.__shimoPhase3Presence?.().ready),body:(document.body?.innerText||'').slice(0,300),iframes:[...document.querySelectorAll('iframe')].map(x=>{try{return new URL(x.src).pathname}catch{return 'unknown'}})}))()`, "returnByValue": true}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	t.Log(string(b))
}
