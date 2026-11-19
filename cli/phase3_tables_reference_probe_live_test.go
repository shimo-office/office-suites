package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhase3ReferenceDialogReadOnlyProbe(t *testing.T) {
	out := os.Getenv("SHIMO_PHASE3_REFERENCE_PROBE_OUT")
	if out == "" {
		t.Skip("dedicated fixture only")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	const target = "https://shimo.net/tables/DqdxLrxlDMcw78q4?table=61492df5d31&view=69d0afc1d15"
	b, err := openPhase3TableBrowser(target)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err = b.WaitReady(phase3TableGridReady, 45*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Eval(`(()=>{const matches=[...document.querySelectorAll('button')].filter(e=>e.innerText.trim()==='字段管理');if(matches.length!==1)throw Error('field manager not unique');matches[0].click();return true})()`); err != nil {
		t.Fatal(err)
	}
	if err = b.WaitReady(`document.body.innerText.includes('添加字段')`, 10*time.Second); err != nil {
		_ = phase3WriteEvidence(filepath.Join(out, "wait-error.json"), map[string]any{"error": err.Error()})
	}
	if _, err = b.Eval(`(()=>{const matches=[...document.querySelectorAll('div,span')].filter(e=>e.childElementCount===0&&e.innerText.trim()==='添加字段'&&e.getBoundingClientRect().width>0);if(matches.length!==1)throw Error('add field not unique '+matches.length);matches[0].click();return true})()`); err != nil {
		t.Fatal(err)
	}
	if err = b.WaitReady(`document.querySelector('input[name="fieldName"]')!==null`, 10*time.Second); err != nil {
		_ = phase3WriteEvidence(filepath.Join(out, "dialog-wait-error.json"), map[string]any{"error": err.Error()})
	}
	position, err := b.EvalReadOnly(`(()=>{const menu=[...document.querySelectorAll('.field-type-menu')].find(e=>e.getBoundingClientRect().width>0);if(!menu)throw Error('native field type menu absent');const r=menu.getBoundingClientRect();return{x:r.left+r.width/2,y:r.top+r.height/2}})()`)
	if err != nil {
		t.Fatal(err)
	}
	point := position.(map[string]any)
	if _, err = b.ws.Call(778, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": point["x"], "y": point["y"]}, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err = b.WaitReady(`(()=>{return [...document.querySelectorAll('[data-field-type="ReferenceRecord"]')].some(e=>e.getBoundingClientRect().width>0)})()`, 3*time.Second); err != nil {
		_ = phase3WriteEvidence(filepath.Join(out, "type-hover-error.json"), map[string]any{"error": err.Error()})
	}
	if _, err = b.Eval(`(()=>{const e=[...document.querySelectorAll('[data-field-type="ReferenceRecord"]')].find(e=>e.getBoundingClientRect().width>0);if(e)e.click();return !!e})()`); err != nil {
		t.Fatal(err)
	}
	if err = b.WaitReady(`document.body.innerText.includes('关联表格')||document.body.innerText.includes('关联的')`, 5*time.Second); err != nil {
		_ = phase3WriteEvidence(filepath.Join(out, "reference-wait-error.json"), map[string]any{"error": err.Error()})
	}
	v, err := b.EvalReadOnly(`(()=>({text:document.body.innerText,fieldTypes:[...document.querySelectorAll('[data-field-type]')].map(e=>({type:e.getAttribute('data-field-type'),text:e.innerText,rect:e.getBoundingClientRect().toJSON(),visibility:getComputedStyle(e).visibility})),typeAncestors:[...document.querySelectorAll('div,span')].filter(e=>e.childElementCount===0&&e.innerText.trim()==='多行文本').map(e=>e.parentElement.parentElement.outerHTML.slice(0,8000)),nodes:[...document.querySelectorAll('button,input,[role="button"],[role="menuitem"],[data-tooltip],.header-field,[class*=select],label')].filter(e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0}).map(e=>({text:e.innerText,html:e.outerHTML.slice(0,1200)}))}))()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(out, "visible-ui.json"), v); err != nil {
		t.Fatal(err)
	}
	capabilities, err := b.EvalReadOnly(phase3ReferenceCapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(out, "capabilities.json"), capabilities); err != nil {
		t.Fatal(err)
	}
	if err = b.Capture(filepath.Join(out, "visible-ui.png")); err != nil {
		t.Fatal(err)
	}
}

func TestPhase3ReferenceCapabilityPreflightReadOnly(t *testing.T) {
	out := os.Getenv("SHIMO_PHASE3_REFERENCE_GUARD_OUT")
	if out == "" {
		t.Skip("dedicated fixture only")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	guard, err := phase3OpenTableGuard("https://shimo.net/tables/DqdxLrxlDMcw78q4")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	g, ok := guard.(*phase3TablePresence)
	if !ok {
		t.Fatal("real native guard required")
	}
	if err = g.NavigateTable("61492df5d31"); err != nil {
		t.Fatal(err)
	}
	actual, err := g.ReferenceCapability("61492df5d31")
	if actual != nil {
		if e := phase3WriteEvidence(filepath.Join(out, "reference-capability.json"), actual); e != nil {
			t.Fatal(e)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	denial := verifyPhase3ReferenceCapability(actual, "61492df5d31")
	if actual["referenceTypeOffered"] != false || denial == nil {
		t.Fatal("fixture capability changed; no write was attempted")
	}
	if err = g.browser.Capture(filepath.Join(out, "after-cancel.png")); err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(out, "result.json"), map[string]any{"verified": true, "scope": "native reference capability preflight and unsaved dialog cancellation only", "referenceAvailable": false, "denial": denial.Error(), "mutationAttempted": false, "composeCalls": 0}); err != nil {
		t.Fatal(err)
	}
}
