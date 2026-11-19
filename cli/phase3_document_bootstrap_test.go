package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhase3DocumentBootstrapComparison(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_BOOTSTRAP_COMPARISON") != "1" {
		t.Skip("opt-in read-only initial navigation versus same-profile reload")
	}
	dir := "../tmp/phase3-live/docx-slides/bootstrap-comparison-confirm"
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	b, err := openPhase3EditorWhen("https://shimo.net/docx/4WDzngzwAnCJYzqm", `document.readyState!=="loading"`)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	expr := `(()=>({ok:true,timeOrigin:performance.timeOrigin,scripts:[...document.scripts].map(s=>{try{const u=new URL(s.src);return u.origin+u.pathname}catch{return ''}}).filter(Boolean),resources:performance.getEntriesByType('resource').slice(-250).map(r=>{const u=new URL(r.name,location.href);return {url:u.origin+u.pathname,duration:r.duration,initiator:r.initiatorType,status:r.responseStatus}}),ready:Boolean(!window.__bootstrapBeforeReload&&(` + documentReadyJS + `)),state:document.readyState,sdk:typeof window._sdk,canvases:document.querySelectorAll('canvas').length,body:(document.body?.innerText||'').slice(0,500)}))()`
	start := time.Now()
	probe := func(stage string, count int) bool {
		for i := 0; i < count; i++ {
			v, e := b.EvalReadOnly(expr)
			if e != nil {
				t.Fatal(e)
			}
			m, _ := v.(map[string]any)
			_ = phase3WriteEvidence(filepath.Join(dir, stage+"-"+time.Now().UTC().Format("150405")+".json"), map[string]any{"elapsed": time.Since(start).Seconds(), "state": v})
			t.Log(stage, i, m["ready"], m["sdk"], m["canvases"])
			if m["ready"] == true {
				return true
			}
			time.Sleep(2 * time.Second)
		}
		return false
	}
	initial := probe("initial", 15)
	_ = b.Capture(filepath.Join(dir, "initial.png"))
	if _, err = b.EvalReadOnly(`(()=>{window.__bootstrapBeforeReload=true;return {ok:true}})()`); err != nil {
		t.Fatal(err)
	}
	if _, err = b.ws.Call(11000, "Page.reload", map[string]any{"ignoreCache": true}, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	reloaded := probe("reloaded", 30)
	_ = b.Capture(filepath.Join(dir, "reloaded.png"))
	_ = phase3WriteEvidence(filepath.Join(dir, "result.json"), map[string]any{"initialReady": initial, "reloadedReady": reloaded, "seconds": time.Since(start).Seconds(), "mutationAttempted": false})
	if !reloaded {
		t.Fatal("reload did not make editor ready")
	}
}
