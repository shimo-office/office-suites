package main

import (
	"encoding/base64"
	"encoding/xml"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPhase3DocumentCOSBrowserDiagnostic(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_COS_BROWSER_DIAGNOSTIC") != "1" {
		t.Skip("opt-in initial browser content response diagnosis")
	}
	dir := "../tmp/phase3-live/docx-slides/cos-browser-diagnostic"
	_ = os.MkdirAll(dir, 0700)
	app := newApp([]string{"phase3"})
	guid := "4WDzngzwAnCJYzqm"
	if _, err := app.phase3OfficeExport(guid, filepath.Join(dir, "before-export"), "docx"); err != nil {
		t.Fatal("baseline export failed")
	}
	requests := map[string]map[string]any{}
	failures := map[string]bool{}
	events := []map[string]any{}
	setup := func(b *phase3EditorBrowser) error {
		b.ws.onEvent = func(m map[string]any) {
			p, _ := m["params"].(map[string]any)
			id := str(p, "requestId")
			switch str(m, "method") {
			case "Network.requestWillBeSent":
				req, _ := p["request"].(map[string]any)
				u, e := url.Parse(str(req, "url"))
				if e != nil || !strings.HasSuffix(u.Host, ".cos.ap-beijing.myqcloud.com") || u.Path != "/"+guid {
					return
				}
				q := u.Query()
				entry := map[string]any{"origin": u.Scheme + "://" + u.Host, "path": u.Path, "browserWallTime": p["wallTime"], "method": req["method"]}
				if start, e := time.Parse("20060102T150405Z", q.Get("X-Amz-Date")); e == nil {
					if sec, e := strconv.Atoi(q.Get("X-Amz-Expires")); e == nil {
						entry["signatureStart"] = start
						entry["signatureEnd"] = start.Add(time.Duration(sec) * time.Second)
						entry["remainingSeconds"] = time.Until(start.Add(time.Duration(sec) * time.Second)).Seconds()
					}
				}
				requests[id] = entry
				events = append(events, entry)
			case "Network.responseReceived":
				response, _ := p["response"].(map[string]any)
				if u, e := url.Parse(str(response, "url")); e == nil && u.Host == "shimo.net" && u.Path == "/docx/"+guid {
					entry := map[string]any{"resource": "mainHTML", "status": response["status"], "fromDiskCache": response["fromDiskCache"], "fromServiceWorker": response["fromServiceWorker"]}
					h, _ := response["headers"].(map[string]any)
					for k, v := range h {
						if strings.EqualFold(k, "Date") || strings.EqualFold(k, "Age") || strings.EqualFold(k, "Cache-Control") {
							entry[strings.ToLower(k)] = v
						}
					}
					events = append(events, entry)
				}
				if entry := requests[id]; entry != nil {
					r, _ := p["response"].(map[string]any)
					entry["status"] = r["status"]
					entry["fromDiskCache"] = r["fromDiskCache"]
					h, _ := r["headers"].(map[string]any)
					for k, v := range h {
						if strings.EqualFold(k, "date") {
							entry["serverDate"] = v
						}
					}
					if num(r, "status") >= 400 {
						failures[id] = false
					}
				}
			}
		}
		_, err := b.ws.Call(19000, "Network.enable", map[string]any{"maxTotalBufferSize": 1024 * 1024, "maxResourceBufferSize": 256 * 1024}, 10*time.Second)
		return err
	}
	b, err := openPhase3EditorWithSetup("https://shimo.net/docx/"+guid, `document.readyState!=="loading"`, setup)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	defer func() { _ = phase3WriteEvidence(filepath.Join(dir, "content-events.json"), events) }()
	for i := 0; i < 15; i++ {
		v, e := b.EvalReadOnly(`(()=>({ok:true,ready:Boolean(` + documentReadyJS + `),timeOrigin:performance.timeOrigin,sdk:typeof window._sdk,canvasCount:document.querySelectorAll('canvas').length,body:(document.body?.innerText||'').slice(0,1000)}))()`)
		if e != nil {
			t.Fatal(e)
		}
		_ = phase3WriteEvidence(filepath.Join(dir, "state-"+strconv.Itoa(i)+".json"), v)
		for id, done := range failures {
			if done {
				continue
			}
			r, e := b.ws.Call(20000+i, "Network.getResponseBody", map[string]any{"requestId": id}, 10*time.Second)
			if e != nil {
				continue
			}
			body := []byte(str(r, "body"))
			if boolv(r, "base64Encoded") {
				body, _ = base64.StdEncoding.DecodeString(string(body))
			}
			var failure struct {
				Code    string `xml:"Code"`
				Message string `xml:"Message"`
			}
			if xml.Unmarshal(body, &failure) == nil {
				requests[id]["cosCode"] = failure.Code
				switch failure.Message {
				case "Request has expired", "The request signature we calculated does not match the signature you provided. Check your key and signing method.", "Access Denied":
					requests[id]["cosMessage"] = failure.Message
				}
			}
			failures[id] = true
		}
		m, _ := v.(map[string]any)
		capturedFailure := false
		for _, captured := range failures {
			if captured {
				capturedFailure = true
			}
		}
		if m["ready"] == true || capturedFailure {
			break
		}
		time.Sleep(2 * time.Second)
	}
	_ = captureBrowserScreenshotTimed(b.ws, filepath.Join(dir, "diagnostic.png"), 3*time.Second)
}
