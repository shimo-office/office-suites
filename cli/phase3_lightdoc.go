package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 轻文档（/docs/，document 类型）没有对外编辑器 SDK：window._sdk 不存在，
// 只有 ProseMirror 风格的 hk-* 内核。因此这里的动作全部走「页面操作」：
// 真实选区 + 真实鼠标/键盘事件，结果用 DOM 标记与导出物双重核对。
// 定位一律走语义锚点（svg 图标类名、按钮文本、段落文本），不写死坐标。

const lightDocEditorSelector = `[contenteditable="true"].hk-editor-content`

const lightDocReadyJS = `(()=>{const ce=document.querySelector('` + lightDocEditorSelector + `');if(!ce||!ce.getClientRects().length)return false;return !!document.querySelector('button svg[class*="SvgTextBold"]')})()`

// 轻文档页面不挂 window.logged，协作名单里的当前用户只能由 CLI 传入。
func lightDocPresenceJS(userID string) string {
	raw, _ := json.Marshal(strings.TrimSpace(userID))
	return `(()=>{const presence=window.__shimoPhase3Presence?.();const expected=` + string(raw) + `;if(!presence)return {ok:false,error:"协作名单未装载：未收到服务端在线名单"};if(!presence.ready||!presence.connected)return {ok:false,error:"协作名单检查未通过：尚未收到服务端在线名单"};if(!expected)return {ok:false,error:"缺少当前账号标识，拒绝在未确认协作者的情况下写入"};const others=presence.ids.filter(id=>String(id)!==expected);if(others.length)return {ok:false,error:"协作名单检查未通过：检测到其他用户正在协作（在线 "+JSON.stringify(presence.ids)+"，本人 "+expected+"）"};return {ok:true,others:0,self:expected}})()`
}

func openLightDoc(target, userID string) (*phase3EditorBrowser, error) {
	b, err := openPhase3EditorWhen(target, lightDocReadyJS)
	if err != nil {
		return nil, err
	}
	if err = b.CheckLightDocPresence(userID); err != nil {
		b.Close()
		return nil, err
	}
	return b, nil
}

func (b *phase3EditorBrowser) CheckLightDocPresence(userID string) error {
	if _, err := b.EvalReadOnly(lightDocPresenceJS(userID)); err != nil {
		return fmt.Errorf("协作名单只读门禁失败：%w", err)
	}
	return nil
}

// 段落定位：全文 p.hk-paragraph-node 按出现顺序编号（1 起），正文必须与请求完全一致。
func lightDocSelectJS(paragraph int, text string, start, end int) string {
	spec, _ := json.Marshal(map[string]any{"paragraph": paragraph, "text": text, "start": start, "end": end})
	return `(()=>{const spec=` + string(spec) + `;const ce=document.querySelector('` + lightDocEditorSelector + `');if(!ce)return {ok:false,error:'正文编辑器未就绪'};
const ps=Array.from(ce.querySelectorAll('p.hk-paragraph-node'));const p=ps[spec.paragraph-1];if(!p)return {ok:false,error:'目标段落不存在'};
const text=(p.innerText||'').replace(/\r?\n$/,'');if(text!==spec.text)return {ok:false,error:'目标段落正文不匹配请求：第'+spec.paragraph+'段实际为「'+text.slice(0,40)+'」，共'+ps.length+'段'};
const nodes=[];const w=document.createTreeWalker(p,NodeFilter.SHOW_TEXT);let n,acc=0;while(n=w.nextNode()){const len=Array.from(n.textContent).length;if(len===0)continue;nodes.push({node:n,start:acc,len:len});acc+=len}
if(acc!==Array.from(text).length)return {ok:false,error:'段落含非文本内容，拒绝猜测字符位置'};
const at=off=>{for(const it of nodes){if(off<=it.start+it.len)return [it.node,off-it.start]}throw new Error('字符位置越界')};
ce.focus();const r=document.createRange();r.setStart(...at(spec.start));r.setEnd(...at(spec.end));const sel=window.getSelection();sel.removeAllRanges();sel.addRange(r);document.dispatchEvent(new Event('selectionchange'));
return {ok:true,selected:sel.toString(),paragraphText:text,lineID:p.getAttribute('data-hk-line-id')}})()`
}

// 断言不依赖页面当前选区：按段落序号与字符范围重新定位目标文本节点，
// 再取每个节点的祖先链取值。页面在保存/重渲染时会折叠选区，靠选区断言会误报。
func lightDocMarkJS(attr string, paragraph int, text string, start, end int) string {
	raw, _ := json.Marshal(attr)
	spec, _ := json.Marshal(map[string]any{"paragraph": paragraph, "text": text, "start": start, "end": end})
	return `(()=>{const attr=` + string(raw) + `;const spec=` + string(spec) + `;
const ce=document.querySelector('` + lightDocEditorSelector + `');if(!ce)return {ok:false,error:'正文编辑器未就绪'};
const ps=Array.from(ce.querySelectorAll('p.hk-paragraph-node'));const p=ps[spec.paragraph-1];if(!p)return {ok:false,error:'目标段落不存在'};
const text=(p.innerText||'').replace(/\r?\n$/,'');if(text!==spec.text)return {ok:false,error:'目标段落正文不匹配请求'};
const w=document.createTreeWalker(p,NodeFilter.SHOW_TEXT);const nodes=[];let n,acc=0;
while(n=w.nextNode()){const len=Array.from(n.textContent).length;if(len===0)continue;nodes.push({node:n,start:acc,len:len});acc+=len}
const vals=[];const hits=[];
for(const it of nodes){const s=it.start,e=it.start+it.len;if(e<=spec.start||s>=spec.end)continue;let el=it.node.parentElement,v='';
while(el&&el!==ce){if(el.hasAttribute(attr)){v=el.getAttribute(attr);break}el=el.parentElement}
vals.push(v);hits.push(it.node.textContent)}
return {ok:true,values:[...new Set(vals.map(x=>String(x).trim()))],covered:hits.join(''),count:vals.length}})()`
}

func (b *phase3EditorBrowser) lightDocMarkValues(attr string, spec lightDocFormatSpec) ([]string, error) {
	v, err := b.Eval(lightDocMarkJS(attr, spec.Paragraph, spec.Text, spec.Start, spec.End))
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("标记读取返回结构无效")
	}
	raw, _ := m["values"].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, strings.TrimSpace(fmt.Sprint(item)))
	}
	sort.Strings(out)
	return out, nil
}

// 工具栏控件按语义定位：优先 svg 类名，其次按钮文本。
func lightDocToolbarBoxJS(icon, text string) string {
	iconRaw, _ := json.Marshal(icon)
	textRaw, _ := json.Marshal(text)
	return `(()=>{const icon=` + string(iconRaw) + `;const wantText=` + string(textRaw) + `;const btns=Array.from(document.querySelectorAll('button')).filter(x=>{const r=x.getBoundingClientRect();return r.width>0&&r.height>0&&r.y<120});
let el=null;
if(icon){el=btns.find(x=>{const s=x.querySelector('svg');return s&&(s.getAttribute('class')||'').includes(icon)})}
if(!el&&wantText){if(wantText==='FONTSIZE'){el=btns.find(x=>/^\d+$/.test((x.innerText||'').trim()))}else if(wantText==='PARAGRAPH_STYLE'){el=btns.find(x=>/^(正文|标题\d?|副标题)$/.test((x.innerText||'').trim()))}else{el=btns.find(x=>(x.innerText||'').trim()===wantText)}}
if(!el)return {ok:false,error:'工具栏未找到目标控件'};const r=el.getBoundingClientRect();return {ok:true,x:r.x+r.width/2,y:r.y+r.height/2,label:(el.innerText||'').trim()}})()`
}

func (b *phase3EditorBrowser) clickPoint(x, y float64) error {
	steps := []map[string]any{
		{"type": "mouseMoved", "x": x, "y": y, "button": "none"},
		{"type": "mousePressed", "x": x, "y": y, "button": "left", "buttons": 1, "clickCount": 1},
		{"type": "mouseReleased", "x": x, "y": y, "button": "left", "buttons": 0, "clickCount": 1},
	}
	for _, p := range steps {
		b.nextID++
		if _, err := b.ws.Call(b.nextID, "Input.dispatchMouseEvent", p, 10*time.Second); err != nil {
			return err
		}
	}
	return nil
}

func (b *phase3EditorBrowser) clickToolbar(icon, text string) error {
	v, err := b.Eval(lightDocToolbarBoxJS(icon, text))
	if err != nil {
		return err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return errors.New("工具栏定位返回结构无效")
	}
	x, _ := m["x"].(float64)
	y, _ := m["y"].(float64)
	return b.clickPoint(x, y)
}

// 弹出层里的选项：文本精确匹配（字号、段落样式、字体）或色块背景色匹配（字色、高亮）。
func lightDocPopoverItemJS(text, colorHex string, last bool) string {
	textRaw, _ := json.Marshal(text)
	colorRaw, _ := json.Marshal(strings.ToUpper(strings.TrimPrefix(colorHex, "#")))
	return `(()=>{const wantText=` + string(textRaw) + `;const wantColor=` + string(colorRaw) + `;const pickLast=` + strconv.FormatBool(last) + `;
const layers=Array.from(document.querySelectorAll('div')).filter(e=>{const cls=(e.className||'').toString();const r=e.getBoundingClientRect();return /dropdown--menus|popover|panel/i.test(cls)&&r.height>60&&r.width>40&&e.getClientRects().length});
if(!layers.length)return {ok:false,error:'选项浮层未出现'};
const layer=layers[layers.length-1];
const hex=rgb=>{const m=(rgb||'').match(/\d+/g);if(!m)return '';return m.slice(0,3).map(x=>Number(x).toString(16).padStart(2,'0')).join('').toUpperCase()};
let hits=[];
if(wantText){hits=Array.from(layer.querySelectorAll('*')).filter(e=>{const t=(e.innerText||'').trim();return t===wantText&&e.getClientRects().length&&e.children.length<=1})}
else if(wantColor){hits=Array.from(layer.querySelectorAll('*')).filter(e=>{if(!e.getClientRects().length||e.children.length>0)return false;const bg=getComputedStyle(e).backgroundColor;return hex(bg)===wantColor})}
if(!hits.length)return {ok:false,error:'浮层中没有匹配的选项'};
const el=pickLast?hits[hits.length-1]:hits[0];el.scrollIntoView({block:'center'});
const r=el.getBoundingClientRect();return {ok:true,x:r.x+r.width/2,y:r.y+r.height/2,text:(el.innerText||'').trim(),color:hex(getComputedStyle(el).backgroundColor)}})()`
}

func (b *phase3EditorBrowser) choosePopoverItem(text, colorHex string) error {
	v, err := b.Eval(lightDocPopoverItemJS(text, colorHex, false))
	if err != nil {
		return err
	}
	m, _ := v.(map[string]any)
	x, _ := m["x"].(float64)
	y, _ := m["y"].(float64)
	if x == 0 && y == 0 {
		return errors.New("匹配到的选项没有可见坐标")
	}
	return b.clickPoint(x, y)
}

type lightDocKey struct {
	key  string
	code string
	vk   int
	mods int
}

func (b *phase3EditorBrowser) pressKey(k lightDocKey) error {
	base := map[string]any{"key": k.key, "code": k.code, "windowsVirtualKeyCode": k.vk, "nativeVirtualKeyCode": k.vk, "modifiers": k.mods}
	down := map[string]any{"type": "keyDown"}
	up := map[string]any{"type": "keyUp"}
	for key, val := range base {
		down[key] = val
		up[key] = val
	}
	b.nextID++
	if _, err := b.ws.Call(b.nextID, "Input.dispatchKeyEvent", down, 10*time.Second); err != nil {
		return err
	}
	b.nextID++
	_, err := b.ws.Call(b.nextID, "Input.dispatchKeyEvent", up, 10*time.Second)
	return err
}

// 轻文档没有可订阅的保存状态，持久化一律用「刷新页面后重新读取」证明：
// 刷新后的 DOM 由服务端内容重建，能区分「本地改了」和「真的存下来了」。
func (b *phase3EditorBrowser) reloadLightDoc(userID string) error {
	// 用 Page.navigate 而不是 Page.reload：reload 会替换 target，
	// 旧连接上的 evaluate 会一直挂到 RPC 上限，表现为裸 i/o timeout。
	b.nextID++
	if _, err := b.ws.Call(b.nextID, "Page.navigate", map[string]any{"url": b.navURL}, 25*time.Second); err != nil {
		return fmt.Errorf("重新打开页面失败：%w", err)
	}
	time.Sleep(2 * time.Second)
	if err := b.WaitReady(lightDocReadyJS, 90*time.Second); err != nil {
		return fmt.Errorf("刷新后编辑器未就绪：%w", err)
	}
	time.Sleep(1500 * time.Millisecond)
	for i := 0; i < 30; i++ {
		v, err := b.EvalReadOnly(`(()=>{const p=window.__shimoPhase3Presence?.();return {ok:Boolean(p&&p.ready&&p.connected)}})()`)
		if err == nil {
			if m, ok := v.(map[string]any); ok && m["ok"] == true {
				return b.CheckLightDocPresence(userID)
			}
		}
		time.Sleep(time.Second)
	}
	return errors.New("刷新后协作名单重连超时")
}

// 保存是异步的，给一小段静默窗口再读数；不以此为通过依据。
func (b *phase3EditorBrowser) settleLightDoc() {
	time.Sleep(1800 * time.Millisecond)
}

func lightDocParagraphTextJS(paragraph int) string {
	return `(()=>{const ce=document.querySelector('` + lightDocEditorSelector + `');if(!ce)return {ok:false,error:'正文编辑器未就绪'};const ps=Array.from(ce.querySelectorAll('p.hk-paragraph-node'));const p=ps[` + strconv.Itoa(paragraph-1) + `];if(!p)return {ok:false,error:'目标段落不存在'};return {ok:true,text:(p.innerText||'').replace(/\r?\n$/,''),heading:p.getAttribute('data-hk-heading')||''}})()`
}

func (b *phase3EditorBrowser) lightDocParagraphText(paragraph int) (string, string, error) {
	v, err := b.Eval(lightDocParagraphTextJS(paragraph))
	if err != nil {
		return "", "", err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return "", "", errors.New("段落读取返回结构无效")
	}
	return str(m, "text"), str(m, "heading"), nil
}

// 导出物只作辅助证据：轻文档的 md 不保留字号、颜色与高亮，字符格式以 DOM 标记为准。
func (a *App) lightDocExportMarkdown(guid, dir string) (string, string, error) {
	art, err := a.exportFile(guid, "md", "document")
	if err != nil {
		return "", "", err
	}
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	body, err := httpGet(art["url"])
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, "content_"+time.Now().Format("150405")+".md")
	if err = os.WriteFile(path, body, 0o600); err != nil {
		return "", "", err
	}
	return path, string(body), nil
}
