package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhase3SlidesResourceDiagnostic(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_SLIDES_RESOURCE_DIAGNOSTIC") != "1" {
		t.Skip("opt-in dedicated fixture read-only Chrome diagnosis")
	}
	dir := "../tmp/phase3-live/slides/resource-diagnostic"
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	safeURL := func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return ""
		}
		return u.Scheme + "://" + u.Host + u.EscapedPath()
	}
	b, err := openPhase3EditorWhen("https://shimo.net/presentation/M12Y8VYX0NiRmjWg?dbg=1", `document.readyState!=="loading"`)
	if err != nil {
		_ = phase3WriteEvidence(filepath.Join(dir, "startup-error.json"), map[string]any{"error": err.Error()})
		t.Fatal(err)
	}
	defer b.Close()
	events := []map[string]any{}
	requests := map[string]string{}
	b.ws.onEvent = func(m map[string]any) {
		if len(events) >= 600 {
			return
		}
		p, _ := m["params"].(map[string]any)
		out := map[string]any{"method": str(m, "method"), "at": time.Now().UTC()}
		switch str(m, "method") {
		case "Network.requestWillBeSent":
			r, _ := p["request"].(map[string]any)
			path := safeURL(str(r, "url"))
			requests[str(p, "requestId")] = path
			out["url"] = path
			out["type"] = p["type"]
		case "Network.responseReceived":
			r, _ := p["response"].(map[string]any)
			out["url"] = safeURL(str(r, "url"))
			out["status"] = r["status"]
			out["mimeType"] = r["mimeType"]
			out["fromDiskCache"] = r["fromDiskCache"]
		case "Network.loadingFailed":
			out["url"] = requests[str(p, "requestId")]
			out["errorText"] = p["errorText"]
			out["blockedReason"] = p["blockedReason"]
			out["canceled"] = p["canceled"]
		case "Runtime.exceptionThrown":
			e, _ := p["exceptionDetails"].(map[string]any)
			x, _ := e["exception"].(map[string]any)
			out["text"] = e["text"]
			out["className"] = x["className"]
			description := str(x, "description")
			if len(description) > 1000 {
				description = description[:1000]
			}
			out["description"] = description
			out["url"] = safeURL(str(e, "url"))
			out["lineNumber"] = e["lineNumber"]
		case "Log.entryAdded":
			e, _ := p["entry"].(map[string]any)
			out["level"] = e["level"]
			out["source"] = e["source"]
			out["url"] = safeURL(str(e, "url"))
			out["lineNumber"] = e["lineNumber"]
		default:
			return
		}
		events = append(events, out)
	}
	defer func() { _ = phase3WriteEvidence(filepath.Join(dir, "cdp-events.json"), events) }()
	for i, method := range []string{"Network.enable", "Runtime.enable", "Log.enable"} {
		if _, err = b.ws.Call(10000+i, method, nil, phase3EditorReadOnlyTimeout); err != nil {
			t.Fatal(method, err)
		}
	}
	if _, err = b.ws.Call(10010, "Page.reload", map[string]any{"ignoreCache": true}, phase3EditorReadOnlyTimeout); err != nil {
		t.Fatal(err)
	}
	expr := `(()=>{const safe=s=>{try{const u=new URL(s,location.href);return u.origin+u.pathname}catch{return ''}};const resources=performance.getEntriesByType('resource').slice(-250).map(r=>({url:safe(r.name),initiatorType:r.initiatorType,startTime:r.startTime,duration:r.duration,transferSize:r.transferSize,encodedBodySize:r.encodedBodySize,responseStatus:r.responseStatus}));const p=window.__shimoPhase3Presence?.();return {ok:true,url:safe(location.href),state:document.readyState,title:document.title,body:(document.body?.innerText||'').slice(0,2500),sdk:typeof window.presentationSDK,editor:typeof window.presentationSDK?.editor,paragraphs:typeof window.presentationSDK?.API?.slides?.getSlides,canvases:document.querySelectorAll('canvas').length,presence:p?{ready:p.ready,connected:p.connected,event:p.event,idsCount:p.ids.length}:null,scripts:[...document.scripts].map(s=>safe(s.src)).filter(Boolean),resources}})()`
	start := time.Now()
	for i := 0; i < 12; i++ {
		v, e := b.EvalReadOnly(expr)
		if e != nil {
			_ = phase3WriteEvidence(filepath.Join(dir, "read-error.json"), map[string]any{"error": e.Error(), "elapsedSeconds": time.Since(start).Seconds()})
			t.Fatal(b.captureReadinessFailure(e))
		}
		_ = phase3WriteEvidence(filepath.Join(dir, fmt.Sprintf("snapshot-%02d.json", i)), v)
		_ = phase3WriteEvidence(filepath.Join(dir, "cdp-events.json"), events)
		m, _ := v.(map[string]any)
		t.Logf("snapshot=%d elapsed=%.1fs sdk=%v editor=%v canvases=%v events=%d", i, time.Since(start).Seconds(), m["sdk"], m["editor"], m["canvases"], len(events))
		if m["paragraphs"] == "function" {
			break
		}
		time.Sleep(10 * time.Second)
	}
	if err = b.Capture(filepath.Join(dir, "page.png")); err != nil {
		t.Fatal(err)
	}
}
