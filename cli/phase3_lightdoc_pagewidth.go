package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// doc-page-width：轻文档的「文档样式 / 页面宽度」入口。页面宽度控件只在标题
// 上方 hover 时出现，点击后用真实鼠标事件选择菜单项 data-test=menu-item.<mode>。
// 验收依据是正文纸张容器的 page-width-<mode> 类与宽度变化，而不是菜单是否点开。
var lightDocPageWidthOptions = map[string]string{
	"standard": "menu-item.standard", "标准": "menu-item.standard", "标准页面": "menu-item.standard",
	"wide": "menu-item.wide", "加宽": "menu-item.wide", "加宽页面": "menu-item.wide",
	"full": "menu-item.full", "responsive": "menu-item.full", "自适应": "menu-item.full", "自适应页面": "menu-item.full",
}

func prepareLightDocPageWidth(raw string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return "", errors.New("doc-page-width 需要 width：standard（标准）/ wide（加宽）/ full（自适应）")
	}
	if _, ok := lightDocPageWidthOptions[key]; !ok {
		return "", fmt.Errorf("未知页面宽度 %q；可用：standard / wide / full", raw)
	}
	return key, nil
}

func lightDocPageWidthMode(mode string) string {
	switch mode {
	case "wide", "加宽", "加宽页面":
		return "wide"
	case "full", "responsive", "自适应", "自适应页面":
		return "full"
	}
	return "standard"
}

const lightDocPaperStateJS = `(()=>{const ce=document.querySelector('div[contenteditable="true"]');if(!ce)return {ok:false,error:'正文编辑器未就绪'};let paper=ce;for(let i=0;i<10&&paper;i++){const cls=String(paper.className||'');if(/has-editor-toolbar|page-width-/.test(cls))break;paper=paper.parentElement}const r=paper?.getBoundingClientRect();return {ok:true,paperClass:String(paper?.className||''),width:r?r.width:0}})()`

func (a *App) cmdPhase3LightDocPageWidth() error {
	if len(a.Pos) != 1 {
		return errors.New("doc-page-width 需要一个轻文档链接（/docs/）")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	raw := firstNonEmpty(str(payload, "width"), str(payload, "mode"), str(payload, "value"))
	mode, err := prepareLightDocPageWidth(raw)
	if err != nil {
		return err
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !phase3SameOrigin(target, a.Origin) {
		return errors.New("目标 origin 不符，未写入")
	}
	if !isLightDocURL(target) {
		return errors.New("目标不是轻文档（/docs/）；文稿请用 document-page")
	}
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	run, err := a.lightDocStart("doc-page-width", target, guidFromURL(target), dir, map[string]any{"width": mode})
	if err != nil {
		return err
	}
	defer run.close()
	userID, err := a.currentLightDocUserID()
	if err != nil {
		return run.fail(err)
	}
	b, err := openLightDoc(target, userID)
	if err != nil {
		return run.fail(err)
	}
	run.browser = b
	before, err := b.lightDocPaperState()
	if err != nil {
		return run.fail(err)
	}
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return run.fail(err)
	}
	// 页面宽度入口只在标题上方 hover 时出现：先把鼠标移到纸张顶部再取坐标。
	if rect, e := b.Eval(`(()=>{const ce=document.querySelector('div[contenteditable="true"]');const r=ce?ce.getBoundingClientRect():null;return r?{x:r.x+r.width/2,y:r.y+30}:null})()`); e == nil {
		if m, ok := rect.(map[string]any); ok {
			if x, ok := m["x"].(float64); ok {
				if y, ok := m["y"].(float64); ok {
					b.nextID++
					_, _ = b.ws.Call(b.nextID, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": x, "y": y, "button": "none"}, 10*time.Second)
					time.Sleep(600 * time.Millisecond)
				}
			}
		}
	}
	control, err := b.Eval(`(()=>{const icon=document.querySelector('svg.SvgMainPaper');if(!icon)return {ok:false,error:'未找到页面宽度入口'};const r=icon.getBoundingClientRect();return {ok:true,x:r.x+r.width/2,y:r.y+r.height/2}})()`)
	if err != nil {
		return run.fail(err)
	}
	box, _ := control.(map[string]any)
	x, _ := box["x"].(float64)
	y, _ := box["y"].(float64)
	if x == 0 || y == 0 {
		return run.fail(errors.New("页面宽度入口没有可见坐标"))
	}
	wantClass := "page-width-" + lightDocPageWidthMode(mode)
	if strings.Contains(str(before, "paperClass"), wantClass) {
		result := map[string]any{"ok": true, "verified": true, "alreadySet": true, "action": "doc-page-width", "target": target, "width": mode,
			"before": before, "after": before, "evidenceDir": dir, "screenshot": filepath.Join(dir, "before.png"),
			"verificationScope": "目标页面宽度已生效，未重复写入"}
		if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
			return err
		}
		return a.printJSON(result)
	}
	run.attempt = true
	if err = b.clickPoint(x, y); err != nil {
		return run.fail(fmt.Errorf("打开页面宽度菜单失败：%w", err))
	}
	time.Sleep(700 * time.Millisecond)
	selector := lightDocPageWidthOptions[strings.ToLower(strings.TrimSpace(raw))]
	if selector == "" {
		selector = lightDocPageWidthOptions[mode]
	}
	itemJS := `(()=>{const el=document.querySelector('[data-test="` + selector + `"]');if(!el)return {ok:false,error:'页面宽度菜单项未出现'};const item=el.closest('.cg-menu--item')||el;const r=item.getBoundingClientRect();return {ok:true,x:r.x+r.width/2,y:r.y+r.height/2,text:(item.innerText||'').trim()}})()`
	item, err := b.Eval(itemJS)
	if err != nil {
		return run.fail(err)
	}
	im, _ := item.(map[string]any)
	ix, _ := im["x"].(float64)
	iy, _ := im["y"].(float64)
	if ix == 0 || iy == 0 {
		return run.fail(errors.New("页面宽度菜单项没有可见坐标"))
	}
	if err = b.clickPoint(ix, iy); err != nil {
		return run.fail(fmt.Errorf("选择页面宽度失败：%w", err))
	}
	time.Sleep(900 * time.Millisecond)
	after, err := b.lightDocPaperState()
	if err != nil {
		return run.fail(err)
	}
	afterClass := str(after, "paperClass")
	want := wantClass
	if lightDocPageWidthMode(mode) == "standard" {
		// 标准页面是默认值，页面可能直接移除 page-width-* 类而不是加 page-width-standard。
		if strings.Contains(afterClass, "page-width-wide") || strings.Contains(afterClass, "page-width-full") {
			return run.fail(fmt.Errorf("页面宽度未生效：仍为 %q", afterClass))
		}
	} else if !strings.Contains(afterClass, want) {
		return run.fail(fmt.Errorf("页面宽度未生效：期望 %s，实际容器类 %q", want, afterClass))
	}
	if err = b.Capture(filepath.Join(dir, "after.png")); err != nil {
		return run.fail(err)
	}
	result := map[string]any{"ok": true, "verified": true, "action": "doc-page-width", "target": target, "width": mode,
		"before": before, "after": after, "evidenceDir": dir, "screenshot": filepath.Join(dir, "after.png"),
		"verificationScope": "点击文档样式/页面宽度并用真实鼠标选择菜单项；以正文纸张容器的 page-width-* 类与宽度为验收依据"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}

func (b *phase3EditorBrowser) lightDocPaperState() (map[string]any, error) {
	v, err := b.Eval(lightDocPaperStateJS)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok || !boolv(m, "ok") {
		return nil, errors.New(str(m, "error"))
	}
	return m, nil
}
