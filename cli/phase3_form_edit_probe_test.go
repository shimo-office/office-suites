package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPhase3FormEditProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_FORM_EDIT_PROBE") != "1" {
		t.Skip("opt-in isolated form probe")
	}
	app := newApp([]string{"phase3"})
	sdk, err := app.phase3SDK("https://shimo.net/forms/aqmmL0mOBlSKOjqY", "form")
	if err != nil {
		t.Fatal(err)
	}
	content, err := sdk.get("forms/" + sdk.guid + "/content")
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := content.(map[string]any); ok {
		for k, v := range m {
			t.Logf("content key %s type %T", k, v)
		}
	} else {
		t.Logf("content type %T", content)
	}
	contentMap := content.(map[string]any)
	rawPath := filepath.Join(t.TempDir(), "draft.ot")
	if _, err = fetchToFile(str(contentMap, "contentUrl"), rawPath); err != nil {
		t.Fatal("draft download failed")
	}
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatal(err)
	}
	b, err := openPhase3EditorWhen("https://shimo.net/forms/aqmmL0mOBlSKOjqY?debug=1", `Boolean(window.logged?.edit?.query?.getForm)`)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	encoded, _ := json.Marshal(string(raw))
	decoded, err := b.Eval(`new window.modoc.Delta(` + string(encoded) + `).toJSON()`)
	if err != nil {
		t.Fatal(err)
	}
	rawJSON, _ := json.MarshalIndent(decoded, "", "  ")
	t.Log("decoded " + string(rawJSON))
	v, err := b.Eval(`(()=>{const e=window.logged.edit;return {form:e.query.getForm(),rev:e.query.getRev(),canEdit:e.query.permissions.canEditForm(),queryKeys:Object.keys(e.query),commandKeys:Object.keys(e.command),presence:window.__shimoPhase3Presence?.(),user:window.logged.user?.id,contextUser:e.query.permissions.DEBUG_getContext()?.user?.id,publication:e.query.getPublishStatus(),body:document.body.innerText.slice(0,1000)}})()`)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(v, "", "  ")
	t.Log(string(data))
}

func TestPhase3FormFillSource(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_FORM_FILL_SOURCE") != "1" {
		t.Skip("opt-in static source read")
	}
	app := newApp([]string{"phase3"})
	r, err := app.authRequestPath("/forms/aqmmL0mOBlSKOjqY/fill?debug=1", "GET", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`<script[^>]+src="([^"]+)"`).FindAllSubmatch(r.Raw, -1)
	for _, m := range matches {
		u, e := url.Parse(string(m[1]))
		if e == nil && u.RawQuery == "" {
			t.Log(u.String())
		}
	}
}

func TestPhase3FormReadinessDiagnosis(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_FORM_READY_DIAG") != "1" {
		t.Skip("opt-in readiness diagnostic")
	}
	b, err := openPhase3EditorWhen("https://shimo.net/forms/aqmmL0mOBlSKOjqY?debug=1", `document.readyState==='complete'`)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for i := 0; i < 24; i++ {
		v, e := b.Eval(`(()=>{const q=window.logged?.edit?.query;const form=q?.getForm();return {keys:Object.keys(window.logged||{}),query:!!q,route:q?.getRouterPage?.(),firstSubject:form?.questions?.[0]?.subject,titleInBody:document.body.innerText.includes(form?.title),inputValues:[...document.querySelectorAll("input,textarea")].map(e=>e.value).filter(Boolean),modoc:!!window.modoc?.Delta,title:form?.title,text:document.body.innerText.slice(0,700),length:document.body.innerText.length,ready:` + phase3FormReady + `,presence:window.__shimoPhase3Presence?.()}})()`)
		if e != nil {
			t.Fatal(e)
		}
		data, _ := json.Marshal(v)
		t.Log(string(data))
		if os.Getenv("SHIMO_PHASE3_FORM_READY_QUESTIONS") == "1" {
			if str(v.(map[string]any), "route") == "result" {
				if _, err = b.Eval(`window.logged.edit.command.changeRouterPage('questions')`); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if str(v.(map[string]any), "route") == "questions" && len(str(v.(map[string]any), "text")) > 100 {
				_ = phase3FormScreenshot(b, "../tmp/phase3-live/forms-tables/questions-ready-diagnostic.png")
				return
			}
		} else if v.(map[string]any)["ready"] == true || str(v.(map[string]any), "route") == "result" {
			return
		}
		time.Sleep(5 * time.Second)
	}
	_ = phase3FormScreenshot(b, "../tmp/phase3-live/forms-tables/readiness-failure.png")
	t.Fatal("form readiness did not complete")
}

func TestPhase3FormPublicationProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_FORM_PUBLICATION_PROBE") != "1" {
		t.Skip("opt-in read-only publication probe")
	}
	app := newApp([]string{"phase3"})
	sdk, err := app.phase3SDK("https://shimo.net/forms/aqmmL0mOBlSKOjqY", "form")
	if err != nil {
		t.Fatal(err)
	}
	publication, err := sdk.formPublication()
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence("../tmp/phase3-live/forms-tables/publication-after-deadline-error.json", publication); err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%s deadline=%s timezone=%v", str(publication, "status"), str(publication, "deadline"), publication["timezone"])
}

func TestPhase3FormEditorNetworkProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_FORM_NETWORK_PROBE") != "1" {
		t.Skip("opt-in read-only editor network probe")
	}
	events := []any{}
	defer func() { _ = phase3WriteEvidence("../tmp/phase3-live/forms-tables/editor-network-probe.json", events) }()
	b, err := openPhase3EditorWithSetup("https://shimo.net/forms/aqmmL0mOBlSKOjqY?debug=1", phase3FormReady, func(b *phase3EditorBrowser) error {
		b.ws.onEvent = func(event map[string]any) {
			params, _ := event["params"].(map[string]any)
			if str(event, "method") == "Network.responseReceived" {
				response, _ := params["response"].(map[string]any)
				u, e := url.Parse(str(response, "url"))
				if e != nil {
					return
				}
				events = append(events, map[string]any{"origin": u.Scheme + "://" + u.Host, "path": u.EscapedPath(), "httpStatus": response["status"], "type": params["type"]})
			}
			if str(event, "method") == "Network.loadingFailed" {
				events = append(events, map[string]any{"event": "loadingFailed", "error": params["errorText"], "type": params["type"]})
			}
		}
		_, e := b.ws.Call(110, "Network.enable", nil, 10*time.Second)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err = phase3FormShowQuestions(b); err != nil {
		t.Fatal(err)
	}
	snap, err := phase3FormSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence("../tmp/phase3-live/forms-tables/editor-network-probe-snapshot.json", snap); err != nil {
		t.Fatal(err)
	}
	if err = phase3FormScreenshot(b, "../tmp/phase3-live/forms-tables/editor-network-probe.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("rendered questions; network responses=%d", len(events))
}

func TestPhase3FormActiveBrowserProbe(t *testing.T) {
	file := os.Getenv("SHIMO_PHASE3_FORM_ACTIVE_PORT_FILE")
	if file == "" {
		t.Skip("opt-in readonly active-browser attachment")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(strings.Split(string(raw), "\n")[0])
	if err != nil {
		t.Fatal(err)
	}
	target, err := waitPageTarget(port, "https://shimo.net/forms/aqmmL0mOBlSKOjqY")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := dialWS(target)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	b := &phase3EditorBrowser{ws: ws, nextID: 15000}
	v, err := b.Eval(`(()=>{if(!location.pathname.includes('aqmmL0mOBlSKOjqY'))return {ok:false,error:'wrong target'};const q=window.logged?.edit?.query,f=q?.getForm();return {path:location.pathname,route:q?.getRouterPage?.(),keys:Object.keys(window.logged||{}),modoc:!!window.modoc?.Delta,title:f?.title,titleInBody:document.body.innerText.includes(f?.title),ready:` + phase3FormReady + `,body:document.body.innerText.slice(0,3000),inputs:[...document.querySelectorAll('input,textarea')].map(e=>({value:e.value,rects:[...e.getClientRects()].map(r=>({width:r.width,height:r.height}))})),resources:performance.getEntriesByType('resource').filter(e=>e.responseStatus>=400).map(e=>({origin:new URL(e.name).origin,path:new URL(e.name).pathname,status:e.responseStatus}))}})()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence("../tmp/phase3-live/forms-tables/active-form-browser-probe.json", v); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(v)
	t.Log(string(data))
}

func TestPhase3FormActiveForegroundProbe(t *testing.T) {
	file := os.Getenv("SHIMO_PHASE3_FORM_ACTIVE_PORT_FILE")
	if file == "" {
		t.Skip("opt-in existing-browser foreground diagnostic")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(strings.Split(string(raw), "\n")[0])
	if err != nil {
		t.Fatal(err)
	}
	target, err := waitPageTarget(port, "https://shimo.net/forms/aqmmL0mOBlSKOjqY")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := dialWS(target)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	b := &phase3EditorBrowser{ws: ws, nextID: 20000}
	pending := map[string]any{}
	responses := []any{}
	ws.onEvent = func(event map[string]any) {
		p, _ := event["params"].(map[string]any)
		id := str(p, "requestId")
		switch str(event, "method") {
		case "Network.requestWillBeSent":
			r, _ := p["request"].(map[string]any)
			u, e := url.Parse(str(r, "url"))
			if e == nil {
				pending[id] = map[string]any{"origin": u.Scheme + "://" + u.Host, "path": u.EscapedPath(), "type": p["type"]}
			}
		case "Network.responseReceived":
			r, _ := p["response"].(map[string]any)
			u, e := url.Parse(str(r, "url"))
			if e == nil {
				responses = append(responses, map[string]any{"origin": u.Scheme + "://" + u.Host, "path": u.EscapedPath(), "status": r["status"], "type": p["type"]})
			}
		case "Network.loadingFinished", "Network.loadingFailed":
			delete(pending, id)
		}
	}
	if _, err = ws.Call(19000, "Network.enable", nil, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	expression := `(()=>{if(!location.pathname.includes('aqmmL0mOBlSKOjqY'))return {ok:false,error:'wrong target'};return {path:location.pathname,visibility:document.visibilityState,hasFocus:document.hasFocus(),ready:` + phase3FormReady + `,keys:Object.keys(window.logged||{}),route:window.logged?.edit?.query?.getRouterPage?.(),bodyLength:document.body.innerText.length,presence:window.__shimoPhase3Presence?.()}})()`
	before, err := b.Eval(expression)
	if err != nil {
		t.Fatal(err)
	}
	report := map[string]any{"before": before, "broughtToFront": false, "networkObservation": "requests starting after attachment only; earlier pending requests may not be observable"}
	defer func() {
		report["pending"] = pending
		report["responses"] = responses
		output := os.Getenv("SHIMO_PHASE3_FORM_FOREGROUND_EVIDENCE")
		if output == "" {
			output = "../tmp/phase3-live/forms-tables/active-form-foreground-probe.json"
		}
		_ = phase3WriteEvidence(output, report)
	}()
	if before.(map[string]any)["visibility"] == "hidden" || os.Getenv("SHIMO_PHASE3_FORM_FORCE_FOREGROUND") == "1" {
		if _, err = ws.Call(19001, "Page.bringToFront", nil, 10*time.Second); err != nil {
			t.Fatal(err)
		}
		report["broughtToFront"] = true
	}
	states := []any{}
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Second)
		v, e := b.Eval(expression)
		if e != nil {
			t.Fatal(e)
		}
		states = append(states, v)
		if v.(map[string]any)["ready"] == true {
			break
		}
	}
	report["after"] = states
	t.Logf("foreground diagnostic; broughtToFront=%v afterCount=%d", report["broughtToFront"], len(states))
}
