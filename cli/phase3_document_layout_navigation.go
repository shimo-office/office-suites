package main

import (
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"time"
)

func prepareDocumentPageNavigation(p map[string]any) (int, error) {
	n, ok := p["page"].(float64)
	if len(p) != 1 || !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 1 || n > 10000 || math.Trunc(n) != n {
		return 0, fmt.Errorf("document-goto 仅接受从1开始的 page 整数")
	}
	return int(n), nil
}

// GoToPage only moves the native cursor if the target has been calculated.
// The physical total may already be available from cached rendering.
func documentPageCalculationStateJS(page int) string {
	return fmt.Sprintf(`(()=>{const c=window._sdk?.editor?.docsApi?.controller;const total=c?.paintingModel?.totalPages,calculated=c?.calcEngine?.getPageCount?.();return {ready:Number.isInteger(total)&&total>=%d&&Number.isInteger(calculated)&&calculated>=%d,totalPages:total??null,calculatedPages:calculated??null};})()`, page, page)
}

func documentNavigateCalculatedPage(page int, read, navigate func(string) (any, error), pause func(time.Duration), maxAttempts int) ([]any, any, error) {
	states := []any{}
	for i := 0; i < maxAttempts; i++ {
		state, err := read(documentPageCalculationStateJS(page))
		if err != nil {
			return states, nil, err
		}
		states = append(states, state)
		if s, ok := state.(map[string]any); ok && s["ready"] == true {
			// Recheck in the same task as navigation, then call exactly once.
			submitted, err := navigate(fmt.Sprintf(`(()=>{const state=%s;if(!state.ready)return {ok:false,error:'目标页计算状态已变化，未执行导航',state};const a=window._sdk.editor.docsApi;a.controller.GoToPage(%d);return {ok:true,page:%d,pageCount:state.totalPages,calculatedPages:state.calculatedPages}})()`, documentPageCalculationStateJS(page), page-1, page))
			if err != nil {
				return states, submitted, err
			}
			if result, ok := submitted.(map[string]any); !ok || result["ok"] != true {
				return states, submitted, fmt.Errorf("目标页计算状态已变化，未执行导航")
			}
			return states, submitted, nil
		}
		if i+1 < maxAttempts {
			pause(250 * time.Millisecond)
		}
	}
	return states, nil, fmt.Errorf("等待目标页计算完成超时，未执行导航")
}

// The footer is the actual editor DOM, never a fabricated event or counter.
// Physical viewport, model cursor and the visible page indicator must agree.
func documentPageNavigationStateJS(page int) string {
	return fmt.Sprintf(`(()=>{const a=window._sdk.editor.docsApi,c=a.controller,p=c.paintingModel;const footerText=document.getElementById('shark-footer-container')?.innerText||'';const match=footerText.match(/(?:页数|页码|Pages?)\s*[:：]?\s*(\d+)\s*[/／]\s*(\d+)/i);const state={ok:true,page:a.getCurVisiblePage()+1,pageCount:p.totalPages,cursorPage:Number.isInteger(p.cursorInPage)?p.cursorInPage+1:null,documentPage:Number.isInteger(c.docModel?.CurPage)?c.docModel.CurPage+1:null,footerPage:match?Number(match[1]):null,footerPageCount:match?Number(match[2]):null,footerText:footerText.slice(0,500)};return {...state,verified:state.page===%d&&state.cursorPage===%d&&state.documentPage===%d&&state.footerPage===%d&&state.footerPageCount===state.pageCount};})()`, page, page, page, page)
}
func (a *App) cmdPhase3DocumentGoto() (runErr error) {
	if len(a.Pos) != 1 {
		return fmt.Errorf("document-goto 需要一个目标链接")
	}
	args, e := phase2Args(a.opt("--payload"))
	if e != nil {
		return e
	}
	p, e := phase2ObjectArg(args)
	if e != nil {
		return e
	}
	page, e := prepareDocumentPageNavigation(p)
	if e != nil {
		return e
	}
	target, e := a.phase2Target(a.Pos[0])
	if e != nil {
		return e
	}
	if !phase3SameOrigin(target, a.Origin) {
		return fmt.Errorf("目标 origin 不一致")
	}
	dir, e := a.phase3EvidenceDir(a.opt("--out"))
	if e != nil {
		return e
	}
	defer func() {
		if runErr != nil {
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": false, "error": runErr.Error()})
		}
	}()
	if e = phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"action": "document-goto", "page": page, "url": target, "pageIndexBase": 1, "scope": "physical page navigation; does not edit or persist cursor position"}); e != nil {
		return e
	}
	original, e := a.phase3OfficeExport(guidFromURL(target), filepath.Join(dir, "before"), "docx")
	if e != nil {
		return e
	}
	before, e := readDocumentLayout(original.Path)
	if e != nil {
		return e
	}
	b, e := phase3OpenDocumentEditor(target, `typeof window._sdk?.editor?.docsApi?.controller?.GoToPage==='function'&&typeof window._sdk?.editor?.docsApi?.getCurVisiblePage==='function'&&window._sdk.editor.docsApi.controller?.paintingModel?.totalPages>0`)
	if e != nil {
		return e
	}
	defer b.Close()
	if e = documentLayoutCapture(b, filepath.Join(dir, "before.png")); e != nil {
		return e
	}
	readiness, submitted, navigationErr := documentNavigateCalculatedPage(page, b.EvalReadOnly, b.Eval, time.Sleep, 120)
	if e = phase3WriteEvidence(filepath.Join(dir, "navigation-readiness.json"), map[string]any{"attempts": readiness, "navigationSubmitted": submitted}); e != nil {
		return e
	}
	if navigationErr != nil {
		return navigationErr
	}
	if e = phase3WriteEvidence(filepath.Join(dir, "navigation.json"), submitted); e != nil {
		return e
	}
	var state map[string]any
	states := []map[string]any{}
	for i := 0; i < 20; i++ {
		value, err := b.EvalReadOnly(documentPageNavigationStateJS(page))
		if err != nil {
			return err
		}
		var ok bool
		state, ok = value.(map[string]any)
		if !ok {
			return fmt.Errorf("页面状态返回格式无法验证")
		}
		states = append(states, state)
		if state["verified"] == true {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if e = phase3WriteEvidence(filepath.Join(dir, "navigation-state.json"), map[string]any{"attempts": states, "final": state}); e != nil {
		return e
	}
	if e = documentLayoutCapture(b, filepath.Join(dir, "after.png")); e != nil {
		return e
	}
	artifact, e := a.phase3OfficeExport(guidFromURL(target), filepath.Join(dir, "after"), "docx")
	if e != nil {
		return e
	}
	after, e := readDocumentLayout(artifact.Path)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("导航前后文稿结构出现变化，无法宣称只读导航验收通过")
	}
	if state["verified"] != true {
		return fmt.Errorf("页面视口、模型光标与真实页脚计数不一致，导航未完成验收；内容导出未变，证据：%s", dir)
	}
	result := map[string]any{"ok": true, "verified": true, "action": "document-goto", "page": page, "visiblePage": state["page"], "footerPage": state["footerPage"], "cursorPage": state["cursorPage"], "documentPage": state["documentPage"], "pageCount": state["pageCount"], "url": target, "mutationAttempted": false, "evidenceDir": dir, "artifact": artifact, "verificationScope": "physical viewport, native cursor and visible footer agree in current browser session; content unchanged; cursor position is not persisted"}
	if e = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); e != nil {
		return e
	}
	return a.printJSON(result)
}
