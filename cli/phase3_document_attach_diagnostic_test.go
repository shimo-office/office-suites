package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Diagnose only the current dedicated fixture, and only reload while its SDK is
// absent. Never attach this recovery to a post-write or ready editor.
func TestPhase3DocumentAttachBootstrap(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_ATTACH_BOOTSTRAP") != "1" {
		t.Skip("opt-in dedicated initial-bootstrap diagnosis")
	}
	dirs, err := filepath.Glob(filepath.Join(os.TempDir(), "shimo-phase3-editor-*", "DevToolsActivePort"))
	if err != nil {
		t.Fatal(err)
	}
	latest := ""
	var when time.Time
	for _, p := range dirs {
		info, e := os.Stat(p)
		if e == nil && info.ModTime().After(when) {
			latest = p
			when = info.ModTime()
		}
	}
	if latest == "" || time.Since(when) > 6*time.Minute {
		t.Fatal("no recent phase3 isolated browser")
	}
	raw, err := os.ReadFile(latest)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(strings.Split(string(raw), "\n")[0])
	if err != nil {
		t.Fatal(err)
	}
	target, err := waitPageTarget(port, "https://shimo.net/docx/4WDzngzwAnCJYzqm")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := dialWS(target)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	b := &phase3EditorBrowser{ws: ws, nextID: 30000}
	v, err := b.EvalReadOnly(`(()=>{if(location.pathname!=='/docx/4WDzngzwAnCJYzqm'||typeof window._sdk!=='undefined'||document.querySelectorAll('canvas').length!==0)return {ok:false,error:'仅初始SDK缺失且0canvas允许诊断重载'};const safe=s=>{try{const u=new URL(s,location.href);return u.origin+u.pathname}catch{return ''}};const out={ok:true,mutationAttempted:false,timeOrigin:performance.timeOrigin,body:(document.body?.innerText||'').slice(0,1000),scripts:[...document.scripts].map(s=>safe(s.src)),resources:performance.getEntriesByType('resource').map(r=>({url:safe(r.name),duration:r.duration,status:r.responseStatus,initiator:r.initiatorType})),reloadScheduled:true};location.reload();return out})()`)
	dir := "../tmp/phase3-live/docx-slides/attach-bootstrap"
	_ = os.MkdirAll(dir, 0700)
	_ = phase3WriteEvidence(filepath.Join(dir, "before-reload.json"), map[string]any{"value": v, "error": func() string {
		if err != nil {
			return err.Error()
		}
		return ""
	}(), "at": time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Initial empty SDK page reloaded once; original CLI retains its budget and owns all writes.")
}
