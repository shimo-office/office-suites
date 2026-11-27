package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in end-to-end probe of the UI PDF export, capturing the real download.
func TestSheetPDFExportEndToEndProbe(t *testing.T) {
	target := os.Getenv("SHIMO_SHEET_PROBE_TARGET")
	outPath := os.Getenv("SHIMO_SHEET_PROBE_OUT")
	if os.Getenv("SHIMO_SHEET_PROBE") != "1" || target == "" || outPath == "" {
		t.Skip("opt-in sheet façade probe")
	}
	b, err := openPhase3EditorWhen(target, `Boolean(window.__SM__?.editor?.workbook||document.querySelector('canvas'))`)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	report := map[string]any{}
	record := func(stage string, v any) {
		report[stage] = v
		raw, _ := json.MarshalIndent(report, "", "  ")
		_ = os.WriteFile(outPath, raw, 0o600)
	}
	dir := t.TempDir()
	b.nextID++
	if _, err := b.ws.Call(b.nextID, "Browser.setDownloadBehavior", map[string]any{"behavior": "allow", "downloadPath": dir, "eventsEnabled": true}, 10*time.Second); err != nil {
		record("downloadBehavior", err.Error())
	}
	moveTo := func(x, y float64) {
		b.nextID++
		_, _ = b.ws.Call(b.nextID, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": x, "y": y, "button": "none", "buttons": 0}, 8*time.Second)
	}
	clickText := func(text string) bool {
		v, err := b.EvalReadOnly(`(()=>{const vis=e=>e.getClientRects().length>0;
const hits=[...document.querySelectorAll('button,[role="button"],.item___HtCza')].filter(e=>vis(e)&&(e.innerText||'').trim()===` + jsString(text) + `);
if(!hits.length)return {found:false};const e=hits[hits.length-1];const r=e.getBoundingClientRect();return {found:true,x:r.x+r.width/2,y:r.y+r.height/2,disabled:e.disabled===true}})()`)
		if err != nil {
			return false
		}
		m, _ := v.(map[string]any)
		record("box_"+text, m)
		if m["found"] != true {
			return false
		}
		_ = b.clickPoint(num(m, "x"), num(m, "y"))
		return true
	}
	for i := 0; i < 40; i++ {
		v, err := b.EvalReadOnly(`(()=>{const found=Boolean(document.querySelector('[data-testid="header-more-btn"]'));return {found}})()`)
		if err == nil {
			if m, ok := v.(map[string]any); ok && m["found"] == true {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(time.Second)
	if v, err := b.EvalReadOnly(`(()=>{const e=document.querySelector('[data-testid="header-more-btn"]');const r=e.getBoundingClientRect();return {found:true,x:r.x+r.width/2,y:r.y+r.height/2}})()`); err == nil {
		m, _ := v.(map[string]any)
		_ = b.clickPoint(num(m, "x"), num(m, "y"))
	}
	time.Sleep(1000)
	v, err := b.EvalReadOnly(`(()=>{const row=document.querySelector('[data-testid="menu-item-download"]');if(!row)return {found:false};
for(const t of ['pointerover','pointerenter','mouseover','mouseenter']){row.dispatchEvent(new MouseEvent(t,{bubbles:true,cancelable:true,view:window}))}
const r=row.getBoundingClientRect();return {found:true,left:r.left,right:r.right,mid:r.y+r.height/2}})()`)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := v.(map[string]any)
	if m["found"] != true {
		record("downloadRow", m)
		return
	}
	left, right, mid := num(m, "left"), num(m, "right"), num(m, "mid")
	for x := left - 60; x <= right+5; x += 12 {
		moveTo(x, mid)
		time.Sleep(40 * time.Millisecond)
	}
	time.Sleep(1500)
	if !clickText("纯图PDF") {
		record("pdfEntry", "missing")
		return
	}
	time.Sleep(3000)
	// Wait for the export dialog to finish rendering its pages.
	prepared := false
	for i := 0; i < 120; i++ {
		body := ""
		if v, err := b.EvalReadOnly(`(()=>{return {found:true,body:(document.body?.innerText||'')}})()`); err == nil {
			body = str(v.(map[string]any), "body")
		}
		if strings.Contains(body, "导出设置") && strings.Contains(body, "共") && !strings.Contains(body, "正在准备导出") {
			prepared = true
			break
		}
		time.Sleep(time.Second)
	}
	record("prepared", prepared)
	record("afterPrepare", func() any {
		v, err := b.EvalReadOnly(`(()=>{const vis=e=>e.getClientRects().length>0;
const btns=[...document.querySelectorAll('button')].filter(vis).map(e=>({text:(e.innerText||'').trim(),disabled:e.disabled===true})).filter(x=>x.text);
return {found:true,btns:btns.slice(0,20),body:(document.body?.innerText||'').replace(/\n+/g,'|').slice(0,400)}})()`)
		if err != nil {
			return err.Error()
		}
		return v
	}())
	clickText("下一步")
	var seen []string
	for i := 0; i < 120; i++ {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || strings.HasSuffix(e.Name(), ".crdownload") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			info, err := os.Stat(p)
			if err != nil || info.Size() < 100 {
				continue
			}
			seen = append(seen, e.Name())
			size, sha, verr := verifySheetPDF(p)
			if verr != nil {
				continue
			}
			record("downloaded", map[string]any{"path": p, "name": e.Name(), "size": size, "sha256": sha, "seen": seen})
			_ = b.Capture(os.Getenv("SHIMO_SHEET_PROBE_SHOT"))
			return
		}
		time.Sleep(time.Second)
	}
	record("seen", seen)
	record("downloaded", "timeout")
	_ = b.Capture(os.Getenv("SHIMO_SHEET_PROBE_SHOT"))
}
