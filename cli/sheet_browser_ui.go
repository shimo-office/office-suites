package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// UI primitives for the spreadsheet page. Some operations exist only in the
// product menu (for example the PDF export wizard), and the grid is a canvas:
// there are no DOM cells to click. These helpers locate a control by visible
// text or test id, then click through CDP input so the page's own handlers run -
// the same path a user takes.

func jsString(v string) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// asObj normalises a CDP return value into an object. A scalar reply is a bug
// in the expression, never a state we can act on.
func asObj(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return m
}

func (b *phase3EditorBrowser) uiBodyText() string {
	v, err := b.EvalReadOnly(`(()=>({text:(document.body?.innerText||'')}))()`)
	if err != nil {
		return ""
	}
	return str(asObj(v), "text")
}

// waitUIText polls the rendered text. Positive and negative conditions are
// checked in the same evaluation so a transient frame cannot satisfy both.
func (b *phase3EditorBrowser) waitUIText(must, forbid []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		// A pending condition is data, not an action rejection. EvalReadOnly
		// treats {ok:false} as an exception and would discard the page text.
		expr := `(()=>{const t=(document.body?.innerText||'');return {matched:Boolean(` + uiTextPredicate(must, forbid) + `),text:t.slice(0,2000)}})()`
		v, err := b.EvalReadOnly(expr)
		if err != nil {
			return fmt.Errorf("读取页面文字失败：%w", err)
		}
		m := asObj(v)
		if boolv(m, "matched") {
			return nil
		}
		if time.Now().After(deadline) {
			last := strings.ReplaceAll(str(m, "text"), "\n", "|")
			return fmt.Errorf("等待页面文字超时（%s）：需要 %v，排除 %v；当前页面：%s", timeout, must, forbid, truncateRunes(last, 300))
		}
		time.Sleep(400 * time.Millisecond)
	}
}

func uiTextPredicate(must, forbid []string) string {
	parts := []string{"t"}
	for _, m := range must {
		parts = append(parts, "t.includes("+jsString(m)+")")
	}
	for _, f := range forbid {
		parts = append(parts, "!t.includes("+jsString(f)+")")
	}
	return strings.Join(parts, "&&")
}

// uiBox resolves the clickable rectangle of a uniquely identified control.
func (b *phase3EditorBrowser) uiBox(name, selector, text string) (float64, float64, error) {
	expr := `(()=>{const sel=` + jsString(selector) + `;const want=` + jsString(text) + `;
const vis=e=>{const r=e.getBoundingClientRect();return e.getClientRects().length>0&&r.width>0&&r.height>0};
let el=null;
if(sel){el=document.querySelector(sel)}
if(!el&&want){const hits=[...document.querySelectorAll('button,[role="button"],div,span,li')].filter(e=>vis(e)&&(e.innerText||'').trim()===want);el=hits[hits.length-1]||null}
if(!el)return {found:false};
const r=el.getBoundingClientRect();
return {found:true,x:r.x+r.width/2,y:r.y+r.height/2,w:Math.round(r.width),h:Math.round(r.height),text:(el.innerText||'').trim().slice(0,40),disabled:el.disabled===true}})()`
	v, err := b.EvalReadOnly(expr)
	if err != nil {
		return 0, 0, fmt.Errorf("定位%s失败：%w", name, err)
	}
	m := asObj(v)
	if !boolv(m, "found") {
		return 0, 0, fmt.Errorf("页面没有找到%s（选择器 %q / 文本 %q）", name, selector, text)
	}
	if boolv(m, "disabled") {
		return 0, 0, fmt.Errorf("%s 当前不可点击", name)
	}
	x, y := num(m, "x"), num(m, "y")
	if x <= 0 || y <= 0 {
		return 0, 0, fmt.Errorf("%s 没有可见坐标", name)
	}
	return x, y, nil
}

func (b *phase3EditorBrowser) uiClick(x, y float64) error { return b.clickPoint(x, y) }

func (b *phase3EditorBrowser) uiClickBox(name, selector, text string) error {
	x, y, err := b.uiBox(name, selector, text)
	if err != nil {
		return err
	}
	return b.uiClick(x, y)
}

// uiHoverRow travels the pointer across a menu row so hover-driven submenus
// open. A single mouseMoved event is not enough: the menu tracks pointer travel.
func (b *phase3EditorBrowser) uiHoverRow(selector string) error {
	v, err := b.EvalReadOnly(`(()=>{const row=document.querySelector(` + jsString(selector) + `);if(!row)return {found:false};
for(const t of ['pointerover','pointerenter','mouseover','mouseenter']){row.dispatchEvent(new MouseEvent(t,{bubbles:true,cancelable:true,view:window}))}
const r=row.getBoundingClientRect();return {found:true,left:r.left,right:r.right,mid:r.y+r.height/2}})()`)
	if err != nil {
		return err
	}
	m := asObj(v)
	if !boolv(m, "found") {
		return errors.New("菜单行未出现")
	}
	left, right, mid := num(m, "left"), num(m, "right"), num(m, "mid")
	for x := left - 60; x <= right+5; x += 12 {
		b.nextID++
		if _, err := b.ws.Call(b.nextID, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": x, "y": mid, "button": "none", "buttons": 0}, 8*time.Second); err != nil {
			return err
		}
		time.Sleep(40 * time.Millisecond)
	}
	return nil
}

func (b *phase3EditorBrowser) uiWaitFound(name, selector string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		v, err := b.EvalReadOnly(`(()=>{const e=document.querySelector(` + jsString(selector) + `);if(!e)return {found:false};
const r=e.getBoundingClientRect();return {found:r.width>0&&r.height>0}})()`)
		if err == nil && boolv(asObj(v), "found") {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s 在 %s 内没有出现", name, timeout)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// uiSwitchSheet activates the sheet by real name. The page identity is checked
// afterwards so a wrong or renamed sheet can never receive the write.
func (b *phase3EditorBrowser) uiSwitchSheet(name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	expr := `(async()=>{const e=window.__SM__?.editor;const want=` + jsString(name) + `;
if(!e?.workbook||typeof e.workbook.getWorksheets!=='function')return {found:false,error:'当前部署没有表格工作簿接口'};
const list=e.workbook.getWorksheets().filter(s=>s.name===want);
if(list.length!==1)return {found:false,error:'真实工作表名不存在或不唯一'};
if(e.activeSheet?.id===list[0].id)return {found:true,already:true};
if(typeof e.workbook.setActiveWorksheet!=='function')return {found:false,error:'工作表定位方法不可用'};
await e.workbook.setActiveWorksheet(list[0].id);
await new Promise(r=>setTimeout(r,600));
if(e.activeSheet?.id!==list[0].id)return {found:false,error:'活动工作表未切换至目标'};
return {found:true,already:false}})()`
	v, err := b.Eval(expr)
	if err != nil {
		return fmt.Errorf("切换工作表失败：%w", err)
	}
	m := asObj(v)
	if !boolv(m, "found") {
		return fmt.Errorf("切换工作表失败：%s", str(m, "error"))
	}
	return nil
}
