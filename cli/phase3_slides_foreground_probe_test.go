package main

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var phase3SlidesProbeRunRoot = func() string {
	label := os.Getenv("SHIMO_PHASE3_SLIDES_RUN")
	if label == "" {
		label = time.Now().UTC().Format("20060102T150405.000000000")
	}
	if filepath.Base(label) != label || strings.ContainsAny(label, `/\\`) || label == "." || label == ".." {
		panic("invalid isolated slides probe run label")
	}
	return filepath.Join("../tmp/phase3-live/slides/runs", label)
}()

func phase3SlidesSyntheticTarget() string {
	target := os.Getenv("SHIMO_PHASE3_SLIDES_TARGET")
	if target == "" {
		target = "https://shimo.net/presentation/GWxPRXPanLszabWj"
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.Host != "shimo.net" || !strings.HasPrefix(u.Path, "/presentation/") || u.User != nil {
		panic("invalid explicit slides test target")
	}
	query := u.Query()
	query.Set("dbg", "1")
	u.RawQuery = query.Encode()
	return u.String()
}

// Test-only foreground diagnostic. It changes neither the production startup
// retry policy nor document contents. The focus action occurs before navigate.
func openPhase3SlidesForegroundProbe(target, name string) (*phase3EditorBrowser, error) {
	dir := filepath.Join(phase3SlidesProbeRunRoot, "foreground-probe", name)
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	started := time.Now()
	report := map[string]any{"target": target, "startedUTC": started.UTC(), "mutationAttempted": false}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		return nil, err
	}
	report["testBinarySHA256"] = fmt.Sprintf("%x", sha256.Sum256(binary))
	report["testArgv"] = os.Args
	events := []map[string]any{}
	safe := func(raw string) string {
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" {
			return ""
		}
		return u.Scheme + "://" + u.Host + u.EscapedPath()
	}
	save := func() error { return phase3WriteEvidence(filepath.Join(dir, "diagnostic.json"), report) }
	setup := func(b *phase3EditorBrowser) error {
		report["freshProfilePath"] = b.profile
		report["profileInitialization"] = "new empty temporary directory; authentication cookies only; no copied cache or local storage"
		previous := b.ws.onEvent
		b.ws.onEvent = func(m map[string]any) {
			if previous != nil {
				previous(m)
			}
			if len(events) >= 1200 {
				return
			}
			p, _ := m["params"].(map[string]any)
			out := map[string]any{"method": m["method"], "elapsedSeconds": time.Since(started).Seconds()}
			switch str(m, "method") {
			case "Network.requestWillBeSent":
				r, _ := p["request"].(map[string]any)
				out["url"] = safe(str(r, "url"))
				out["type"] = p["type"]
				out["requestId"] = p["requestId"]
			case "Network.responseReceived":
				r, _ := p["response"].(map[string]any)
				out["url"] = safe(str(r, "url"))
				out["status"] = r["status"]
				out["mimeType"] = r["mimeType"]
				out["requestId"] = p["requestId"]
			case "Page.javascriptDialogOpening":
				out["type"] = p["type"]
				out["url"] = safe(str(p, "url"))
				out["hasBrowserHandler"] = p["hasBrowserHandler"]
			case "Page.javascriptDialogClosed":
				out["result"] = p["result"]
			case "Inspector.targetCrashed", "Target.targetCrashed":
				out["status"] = p["status"]
				out["errorCode"] = p["errorCode"]
			case "Runtime.exceptionThrown":
				detail, _ := p["exceptionDetails"].(map[string]any)
				exception, _ := detail["exception"].(map[string]any)
				out["text"] = detail["text"]
				out["className"] = exception["className"]
				out["url"] = safe(str(detail, "url"))
				out["lineNumber"] = detail["lineNumber"]
			case "Network.loadingFailed":
				out["requestId"] = p["requestId"]
				out["errorText"] = p["errorText"]
				out["canceled"] = p["canceled"]
			default:
				return
			}
			events = append(events, out)
		}
		for _, method := range []string{"Browser.getVersion", "Page.bringToFront", "Network.enable", "Runtime.enable", "Inspector.enable", "Target.setDiscoverTargets"} {
			report["stage"] = method
			if e := save(); e != nil {
				return e
			}
			b.nextID++
			var params any
			if method == "Target.setDiscoverTargets" {
				params = map[string]any{"discover": true}
			}
			v, e := b.ws.Call(b.nextID, method, params, 15*time.Second)
			if e != nil {
				return fmt.Errorf("foreground diagnostic %s: %w", method, e)
			}
			report[method] = v
		}
		report["stage"] = "initial visibility"
		if e := save(); e != nil {
			return e
		}
		v, e := b.Eval(`(()=>({ok:true,visibilityState:document.visibilityState,hasFocus:document.hasFocus(),readyState:document.readyState,timeOrigin:performance.timeOrigin}))()`)
		if e != nil {
			return e
		}
		report["initial"] = v
		report["stage"] = "navigation and SDK readiness"
		return save()
	}
	b, e := openPhase3EditorWithSetup(target, `Boolean(typeof window.presentationSDK?.API?.slides?.getSlides==="function"&&window.presentationSDK?.editor?.workbench?.isReady())`, setup)
	report["elapsedSeconds"] = time.Since(started).Seconds()
	report["events"] = events
	if e != nil {
		report["error"] = e.Error()
		_ = save()
		return nil, e
	}
	report["stage"] = "ready visibility and resource timing"
	_ = save()
	v, e := b.EvalReadOnly(`(()=>{const safe=s=>{try{const u=new URL(s,location.href);return u.origin+u.pathname}catch{return ''}};return {ok:true,visibilityState:document.visibilityState,hasFocus:document.hasFocus(),readyState:document.readyState,timeOrigin:performance.timeOrigin,sdkReady:window.presentationSDK.editor.workbench.isReady(),resources:performance.getEntriesByType('resource').map(r=>({url:safe(r.name),initiatorType:r.initiatorType,startTime:r.startTime,duration:r.duration,transferSize:r.transferSize,encodedBodySize:r.encodedBodySize,responseStatus:r.responseStatus}))}})()`)
	report["events"] = events
	report["elapsedSeconds"] = time.Since(started).Seconds()
	if e != nil {
		report["error"] = e.Error()
		_ = save()
		b.Close()
		return nil, e
	}
	report["ready"] = v
	report["stage"] = "ready"
	report["startupEvidence"] = b.startupEvidence
	if e = save(); e != nil {
		b.Close()
		return nil, e
	}
	return b, nil
}
