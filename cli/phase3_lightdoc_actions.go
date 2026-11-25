package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 轻文档（/docs/）没有编辑器 SDK，需求 18/19/20/21/22 只能走页面操作。
// 这里把每个动作都收敛成「语义定位 + 真实输入事件 + DOM/导出双证据」。

type lightDocFormatSpec struct {
	Paragraph int
	Text      string
	Start     int
	End       int
	Attrs     map[string]any
}

var lightDocFormatAttrs = map[string]string{
	"bold": "data-hk-font-bold", "italic": "data-hk-font-italic",
	"underline": "data-hk-font-underline", "strike": "data-hk-font-strike",
	"fontSize": "data-hk-font-size", "fontFamily": "data-hk-font-family",
	"color": "data-hk-font-color", "highlight": "data-hk-font-bgcolor",
	"verticalAlign": "data-hk-font-vert-align",
}

func prepareLightDocFormat(payload map[string]any) (lightDocFormatSpec, error) {
	spec := lightDocFormatSpec{Start: -1, End: -1}
	spec.Paragraph = int(num(payload, "paragraph"))
	spec.Text = str(payload, "text")
	if spec.Paragraph < 1 {
		return spec, errors.New("payload.paragraph 必须是 1 起的段落序号")
	}
	if strings.TrimSpace(spec.Text) == "" {
		return spec, errors.New("payload.text 必填：必须是目标段落的完整正文，用于写入前核对")
	}
	spec.Start = int(num(payload, "start"))
	if _, ok := payload["start"]; !ok {
		return spec, errors.New("payload.start 必填（未指定时按 0 处理会误改整段）")
	}
	if _, ok := payload["end"]; !ok {
		return spec, errors.New("payload.end 必填（未指定时按整段处理会误改范围）")
	}
	spec.End = int(num(payload, "end"))
	total := len([]rune(spec.Text))
	if spec.Start < 0 || spec.End <= spec.Start || spec.End > total {
		return spec, fmt.Errorf("字符范围 [%d,%d) 超出段落长度 %d", spec.Start, spec.End, total)
	}
	attrs, ok := payload["attrs"].(map[string]any)
	if !ok || len(attrs) == 0 {
		return spec, errors.New("payload.attrs 必填，例如 {\"bold\":true}")
	}
	spec.Attrs = map[string]any{}
	for key, value := range attrs {
		if _, known := lightDocFormatAttrs[key]; !known {
			return spec, fmt.Errorf("不支持的字符格式 %q；可用：bold italic underline strike fontSize fontFamily color highlight verticalAlign", key)
		}
		switch key {
		case "bold", "italic", "underline", "strike":
			if _, ok := value.(bool); !ok {
				return spec, fmt.Errorf("%s 需要布尔值", key)
			}
		case "fontSize":
			size := int(num(map[string]any{"v": value}, "v"))
			if size < 6 || size > 200 {
				return spec, errors.New("fontSize 需要在 6–200 之间")
			}
			value = size
		case "color", "highlight":
			hex, err := normalizeHexColor(fmt.Sprint(value), key == "highlight")
			if err != nil {
				return spec, err
			}
			value = hex
		case "fontFamily":
			name := strings.TrimSpace(fmt.Sprint(value))
			if name == "" {
				return spec, errors.New("fontFamily 不能为空")
			}
			value = name
		case "verticalAlign":
			mode := strings.TrimSpace(fmt.Sprint(value))
			if mode != "superscript" && mode != "subscript" && mode != "baseline" {
				return spec, errors.New("verticalAlign 只接受 superscript / subscript / baseline")
			}
			value = mode
		}
		spec.Attrs[key] = value
	}
	return spec, nil
}

func normalizeHexColor(raw string, allowNone bool) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" || s == "NONE" || s == "CLEAR" {
		if allowNone {
			return "NONE", nil
		}
		return "", errors.New("颜色不能为空")
	}
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return "", fmt.Errorf("颜色 %q 需要 #RRGGBB 形式", raw)
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789ABCDEF", c) {
			return "", fmt.Errorf("颜色 %q 含非法字符", raw)
		}
	}
	return "#" + s, nil
}

// 文本型控件（字号、段落样式、字体）走文本；图标型控件走 svg 类名；字体靠相邻位置锚定。
func lightDocControl(icon, text string, anchorIcon, anchorText string, nth int) (string, string, string, string, int) {
	return icon, text, anchorIcon, anchorText, nth
}

func lightDocControlBoxJS(icon, text, anchorIcon, anchorText string, nth int) string {
	iconRaw, _ := json.Marshal(icon)
	textRaw, _ := json.Marshal(text)
	aIconRaw, _ := json.Marshal(anchorIcon)
	aTextRaw, _ := json.Marshal(anchorText)
	return `(()=>{const icon=` + string(iconRaw) + `,wantText=` + string(textRaw) + `,anchorIcon=` + string(aIconRaw) + `,anchorText=` + string(aTextRaw) + `,nth=` + strconv.Itoa(nth) + `;
const btns=Array.from(document.querySelectorAll('button')).filter(x=>{const r=x.getBoundingClientRect();return r.width>0&&r.height>0&&r.y<120});
const byIcon=(list,ic)=>list.find(x=>{const s=x.querySelector('svg');return s&&(s.getAttribute('class')||'').includes(ic)});
const byText=(list,t)=>{if(t==='FONTSIZE')return list.find(x=>/^\d+$/.test((x.innerText||'').trim()));if(t==='PARAGRAPH_STYLE')return list.find(x=>/^(正文|标题\d?|副标题)$/.test((x.innerText||'').trim()));return list.find(x=>(x.innerText||'').trim()===t)};
let el=null;
if(icon)el=byIcon(btns,icon);
if(!el&&wantText)el=byText(btns,wantText);
if(!el&&(anchorIcon||anchorText)){const base=anchorIcon?byIcon(btns,anchorIcon):byText(btns,anchorText);if(base){const i=btns.indexOf(base);if(i>=0&&i+nth>=0&&i+nth<btns.length)el=btns[i+nth]}}
if(!el)return {ok:false,error:'工具栏未找到目标控件'};const r=el.getBoundingClientRect();return {ok:true,x:r.x+r.width/2,y:r.y+r.height/2,label:(el.innerText||'').trim(),icon:(el.querySelector('svg')?.getAttribute('class')||'')}})()`
}

func (b *phase3EditorBrowser) clickControl(icon, text, anchorIcon, anchorText string, nth int) error {
	v, err := b.Eval(lightDocControlBoxJS(icon, text, anchorIcon, anchorText, nth))
	if err != nil {
		return err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return errors.New("工具栏定位返回结构无效")
	}
	x, _ := m["x"].(float64)
	y, _ := m["y"].(float64)
	if x == 0 && y == 0 {
		return errors.New("工具栏控件没有可见坐标")
	}
	return b.clickPoint(x, y)
}

// 浮层选项按 svg 类名（对齐、上下标这类纯图标项）或文本/色块匹配。
func lightDocPopoverPickJS(text, colorHex, icon string) string {
	textRaw, _ := json.Marshal(text)
	colorRaw, _ := json.Marshal(strings.ToUpper(strings.TrimPrefix(colorHex, "#")))
	iconRaw, _ := json.Marshal(icon)
	return `(()=>{const wantText=` + string(textRaw) + `,wantColor=` + string(colorRaw) + `,wantIcon=` + string(iconRaw) + `;
const layers=Array.from(document.querySelectorAll('div')).filter(e=>{const cls=(e.className||'').toString();const r=e.getBoundingClientRect();return /dropdown--menus|popover|cg-menu(\s|$)/i.test(cls)&&r.height>40&&r.width>40&&e.getClientRects().length});
if(!layers.length)return {ok:false,error:'选项浮层未出现'};
const layer=layers[layers.length-1];
const rgb2hex=rgb=>{const m=(rgb||'').match(/\d+/g);if(!m)return '';return m.slice(0,3).map(x=>Number(x).toString(16).padStart(2,'0')).join('').toUpperCase()};
let hits=[];
if(wantIcon){hits=Array.from(layer.querySelectorAll('*')).filter(e=>{const s=e.tagName==='svg'?e:e.querySelector('svg');return s&&(s.getAttribute('class')||'').includes(wantIcon)&&e.getClientRects().length}).map(s=>s.closest('button')||s.closest('div')||s)}
else if(wantText){const tw=document.createTreeWalker(layer,NodeFilter.SHOW_TEXT);let tn;while(tn=tw.nextNode()){if((tn.textContent||'').trim()!==wantText)continue;const host=tn.parentElement;if(!host||!host.getClientRects().length)continue;const r=host.getBoundingClientRect();if(r.width&&r.height&&r.height<=44)hits.push(host)}}
else if(wantColor){hits=Array.from(layer.querySelectorAll('*')).filter(e=>{if(!e.getClientRects().length)return false;return rgb2hex(getComputedStyle(e).backgroundColor)===wantColor});hits.sort((a,b)=>{const ra=a.getBoundingClientRect(),rb=b.getBoundingClientRect();return ra.width*ra.height-rb.width*rb.height})}
hits=hits.filter((e,i)=>hits.indexOf(e)===i&&e.getClientRects().length);
if(!hits.length){const avail=wantColor?[...new Set(Array.from(layer.querySelectorAll('*')).map(e=>rgb2hex(getComputedStyle(e).backgroundColor)).filter(x=>x))]:[];return {ok:false,error:'浮层中没有匹配的选项'+(wantColor?('；当前色板可用：'+avail.slice(0,40).join(' ')):'；层内文本：'+((layer.innerText||'').replace(/\s+/g,' ').slice(0,120)))}}
const el=hits[0];document.querySelectorAll('[data-shimo-pick]').forEach(x=>x.removeAttribute('data-shimo-pick'));el.setAttribute('data-shimo-pick','1');
try{el.scrollIntoView({block:'center',behavior:'instant'})}catch(e){el.scrollIntoView()}
return {ok:true,mark:true,label:(el.innerText||'').trim(),color:rgb2hex(getComputedStyle(el).backgroundColor)}})()`
}

// 选项定位分两步：先标记并滚入可视区，滚动完成后再取坐标。
// 一步内滚动并取坐标会拿到滚动前的位置，从而点到相邻项。
func (b *phase3EditorBrowser) pickPopover(text, colorHex, icon string) error {
	if _, err := b.Eval(lightDocPopoverPickJS(text, colorHex, icon)); err != nil {
		return err
	}
	time.Sleep(250 * time.Millisecond)
	v, err := b.Eval(`(()=>{const el=document.querySelector('[data-shimo-pick="1"]');if(!el)return {ok:false,error:'标记的选项已消失'};const r=el.getBoundingClientRect();if(!r.width||!r.height)return {ok:false,error:'标记的选项不可见'};
return {ok:true,x:r.x+r.width/2,y:r.y+r.height/2}})()`)
	if err != nil {
		return err
	}
	m, ok := v.(map[string]any)
	if !ok || m["ok"] == false {
		return errors.New("选项点击未返回有效结果")
	}
	x, _ := m["x"].(float64)
	y, _ := m["y"].(float64)
	if x == 0 && y == 0 {
		return errors.New("匹配到的选项没有可见坐标")
	}
	return b.clickPoint(x, y)
}

// 色板是固定集合，找不到时把可用色值带出来，调用方无需再猜。
func lightDocAvailableColors(v any) []string {
	m, _ := v.(map[string]any)
	raw, _ := m["available"].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s := strings.TrimSpace(fmt.Sprint(item)); strings.HasPrefix(s, "#") {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func (b *phase3EditorBrowser) pressEscape() {
	_ = b.pressKey(lightDocKey{"Escape", "Escape", 27, 0})
	time.Sleep(200 * time.Millisecond)
}

// 字符格式的应用顺序：先设性属性（字体、字号、颜色、高亮），再勾选式属性（B/I/U/S）。
func (b *phase3EditorBrowser) applyLightDocAttrs(spec lightDocFormatSpec) (map[string]any, error) {
	applied := map[string]any{}
	setters := []string{"fontFamily", "fontSize", "color", "highlight", "verticalAlign"}
	toggles := []string{"bold", "italic", "underline", "strike"}
	for _, key := range setters {
		value, ok := spec.Attrs[key]
		if !ok {
			continue
		}
		switch key {
		case "fontFamily":
			if err := b.clickControl("", "字体", "PARAGRAPH_STYLE", "", 1); err != nil {
				return applied, fmt.Errorf("打开字体列表失败：%w", err)
			}
			time.Sleep(900 * time.Millisecond)
			if err := b.pickPopover(fmt.Sprint(value), "", ""); err != nil {
				b.pressEscape()
				return applied, fmt.Errorf("选择字体 %q 失败：%w", value, err)
			}
		case "fontSize":
			if err := b.clickControl("", "FONTSIZE", "", "", 0); err != nil {
				return applied, fmt.Errorf("打开字号列表失败：%w", err)
			}
			time.Sleep(900 * time.Millisecond)
			if err := b.pickPopover(strconv.Itoa(value.(int)), "", ""); err != nil {
				b.pressEscape()
				return applied, fmt.Errorf("选择字号 %v 失败：%w", value, err)
			}
		case "color":
			if err := b.clickControl("", "", "SvgCommonFontColor", "", 1); err != nil {
				return applied, fmt.Errorf("打开字色面板失败：%w", err)
			}
			time.Sleep(900 * time.Millisecond)
			if err := b.pickPopover("", fmt.Sprint(value), ""); err != nil {
				b.pressEscape()
				return applied, fmt.Errorf("选择字色 %v 失败：%w", value, err)
			}
		case "highlight":
			if err := b.clickControl("", "", "SvgCommonFontHighlight", "", 1); err != nil {
				return applied, fmt.Errorf("打开高亮面板失败：%w", err)
			}
			time.Sleep(900 * time.Millisecond)
			if err := b.pickPopover("", fmt.Sprint(value), ""); err != nil {
				b.pressEscape()
				return applied, fmt.Errorf("选择高亮色 %v 失败：%w", value, err)
			}
		case "verticalAlign":
			if err := b.clickControl("", "PARAGRAPH_STYLE", "", "", 0); err == nil {
				b.pressEscape()
			}
			return applied, errors.New("verticalAlign 需要工具栏的上下标入口，该入口不在当前工具栏版本中；请改用文稿（document-format）")
		}
		applied[key] = value
		time.Sleep(500 * time.Millisecond)
	}
	for _, key := range toggles {
		want, ok := spec.Attrs[key].(bool)
		if !ok {
			continue
		}
		attr := lightDocFormatAttrs[key]
		current, err := b.lightDocMarkValues(attr, spec)
		if err != nil {
			return applied, err
		}
		on := len(current) == 1 && current[0] == "true"
		if on == want {
			applied[key] = want
			continue
		}
		icon := map[string]string{"bold": "SvgTextBold", "italic": "SvgTextItalics", "underline": "SvgTextUnderline", "strike": "SvgTextStrikeThrough"}[key]
		if err := b.clickControl(icon, "", "", "", 0); err != nil {
			return applied, fmt.Errorf("点击 %s 失败：%w", key, err)
		}
		time.Sleep(400 * time.Millisecond)
		applied[key] = want
	}
	return applied, nil
}

// 轻文档把中文字体名存成英文族名（宋体 → simsun），断言因此按别名集合判等。
func lightDocFontAliases(name string) []string {
	key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", ""))
	table := map[string][]string{
		"宋体":            {"simsun", "宋体"},
		"新宋体":           {"nsimsun", "新宋体"},
		"黑体":            {"simhei", "黑体"},
		"楷体":            {"kaiti", "楷体", "kaitisc", "楷体_gb2312"},
		"仿宋":            {"fangsong", "仿宋"},
		"微软雅黑":          {"microsoftyahei", "微软雅黑"},
		"等线":            {"dengxian", "等线"},
		"timesnewroman": {"timesnewroman"},
		"arial":         {"arial"},
		"calibri":       {"calibri"},
	}
	if list, ok := table[key]; ok {
		return list
	}
	return []string{key}
}

func lightDocExpectAttr(key string, value any) string {
	switch value.(type) {
	case bool:
		return strconv.FormatBool(value.(bool))
	case int:
		return strconv.Itoa(value.(int))
	}
	s := fmt.Sprint(value)
	if strings.HasPrefix(s, "#") {
		return strings.ToUpper(strings.TrimPrefix(s, "#"))
	}
	return s
}

func (b *phase3EditorBrowser) assertLightDocAttrs(spec lightDocFormatSpec) (map[string]any, error) {
	evidence := map[string]any{}
	keys := make([]string, 0, len(spec.Attrs))
	for key := range spec.Attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		attr := lightDocFormatAttrs[key]
		values, err := b.lightDocMarkValues(attr, spec)
		if err != nil {
			return evidence, fmt.Errorf("%s 读取失败：%w", key, err)
		}
		evidence[key] = map[string]any{"attr": attr, "values": values}
		want := lightDocExpectAttr(key, spec.Attrs[key])
		switch key {
		case "bold", "italic", "underline", "strike":
			got := len(values) == 1 && values[0] == "true"
			if got != spec.Attrs[key].(bool) {
				return evidence, fmt.Errorf("%s 未按请求生效（实际标记 %v）", key, values)
			}
		case "color", "highlight":
			if want == "NONE" {
				for _, v := range values {
					if strings.EqualFold(strings.TrimPrefix(v, "#"), want) {
						continue
					}
					if v != "" {
						return evidence, fmt.Errorf("%s 未清除（实际标记 %v）", key, values)
					}
				}
				continue
			}
			hit := false
			for _, v := range values {
				if strings.EqualFold(strings.TrimPrefix(strings.ToUpper(v), "#"), strings.TrimPrefix(want, "#")) {
					hit = true
				}
			}
			if !hit {
				return evidence, fmt.Errorf("%s 未按请求生效（实际标记 %v，期望 %s）", key, values, want)
			}
		case "fontFamily":
			if len(values) != 1 || values[0] == "" {
				return evidence, fmt.Errorf("fontFamily 未按请求生效（实际标记 %v）", values)
			}
			got := strings.ToLower(strings.ReplaceAll(values[0], " ", ""))
			hit := false
			for _, alias := range lightDocFontAliases(want) {
				if got == strings.ToLower(strings.ReplaceAll(alias, " ", "")) {
					hit = true
					break
				}
			}
			if !hit {
				return evidence, fmt.Errorf("fontFamily 未按请求生效（实际标记 %v，期望 %s）", values, want)
			}
		default:
			if len(values) != 1 || !strings.EqualFold(strings.TrimPrefix(values[0], "#"), strings.TrimPrefix(want, "#")) {
				return evidence, fmt.Errorf("%s 未按请求生效（实际标记 %v，期望 %s）", key, values, want)
			}
		}
	}
	return evidence, nil
}

// JSON 数字会变成 float64，直接打印会得到 9.41...e+07，协作名单比对因此失败。
func plainIntString(v any) string {
	switch n := v.(type) {
	case float64:
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case json.Number:
		return n.String()
	case string:
		return strings.TrimSpace(n)
	}
	return ""
}

func (a *App) currentLightDocUserID() (string, error) {
	cookie, err := authCookie()
	if err != nil {
		return "", err
	}
	me, err := a.usersMeWithCookie(cookie)
	if err != nil {
		return "", err
	}
	id := plainIntString(me["id"])
	if id == "" {
		return "", errors.New("无法确认当前账号，拒绝在未确认协作者的情况下写入")
	}
	return id, nil
}

type lightDocRun struct {
	app      *App
	target   string
	guid     string
	dir      string
	browser  *phase3EditorBrowser
	attempt  bool
	runError error
}

func (r *lightDocRun) close() {
	if r.browser != nil {
		r.browser.Close()
	}
}

func (a *App) lightDocStart(action, target, guid, dir string, payload any) (*lightDocRun, error) {
	run := &lightDocRun{app: a, target: target, guid: guid, dir: dir}
	if err := phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"action": action, "target": target, "guid": guid, "payload": payload, "version": version, "at": time.Now().UTC()}); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *lightDocRun) fail(err error) error {
	state := "未执行写入"
	if r.attempt {
		state = "写入已尝试，可能已生效；验收未通过，不要自动重放"
	}
	_ = phase3WriteEvidence(filepath.Join(r.dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": r.attempt, "error": err.Error(), "evidenceDir": r.dir})
	return fmt.Errorf("%s（证据 %s）：%w", state, r.dir, err)
}

// doc-format：字体、字号、颜色、高亮、加粗、斜体、下划线、删除线。
func (a *App) cmdPhase3LightDocFormat() error {
	if len(a.Pos) != 1 {
		return errors.New("doc-format 需要一个轻文档链接（/docs/）")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareLightDocFormat(payload)
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
		return errors.New("目标不是轻文档（/docs/）；文稿请用 document-format，表格请用 sheet-*")
	}
	guid := guidFromURL(target)
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	run, err := a.lightDocStart("doc-format", target, guid, dir, spec)
	if err != nil {
		return err
	}
	defer run.close()

	beforePath, beforeMD, err := a.lightDocExportMarkdown(guid, filepath.Join(dir, "before"))
	if err != nil {
		return run.fail(fmt.Errorf("导出改前内容失败：%w", err))
	}
	userID, err := a.currentLightDocUserID()
	if err != nil {
		return run.fail(err)
	}
	b, err := openLightDoc(target, userID)
	if err != nil {
		return run.fail(err)
	}
	run.browser = b
	if err = b.checkAndSelect(spec); err != nil {
		return run.fail(err)
	}
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return run.fail(err)
	}
	run.attempt = true
	applied, err := b.applyLightDocAttrs(spec)
	if err != nil {
		_ = phase3WriteEvidence(filepath.Join(dir, "submission.json"), applied)
		return run.fail(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), applied); err != nil {
		return run.fail(err)
	}
	b.settleLightDoc()
	marks, err := b.assertLightDocAttrs(spec)
	if err != nil {
		return run.fail(err)
	}
	_ = phase3WriteEvidence(filepath.Join(dir, "marks.json"), marks)
	if err = b.Capture(filepath.Join(dir, "after.png")); err != nil {
		return run.fail(err)
	}
	afterPath, afterMD, err := a.lightDocExportMarkdown(guid, filepath.Join(dir, "after"))
	if err != nil {
		return run.fail(fmt.Errorf("导出写后内容失败：%w", err))
	}
	textBefore, _, _ := b.lightDocParagraphText(spec.Paragraph)
	if err = phase3WriteEvidence(filepath.Join(dir, "content.json"), map[string]any{"before": beforePath, "after": afterPath, "paragraphText": textBefore, "paragraphTextPreserved": textBefore == spec.Text, "markdownChanged": beforeMD != afterMD}); err != nil {
		return run.fail(err)
	}
	if err = b.reloadLightDoc(userID); err != nil {
		return run.fail(err)
	}
	reloaded, err := b.assertLightDocAttrs(spec)
	if err != nil {
		return run.fail(fmt.Errorf("刷新后持久化验收失败：%w", err))
	}
	_ = phase3WriteEvidence(filepath.Join(dir, "marks-reloaded.json"), reloaded)
	if err = b.Capture(filepath.Join(dir, "reload.png")); err != nil {
		return run.fail(err)
	}
	result := map[string]any{
		"ok": true, "verified": true, "action": "doc-format", "target": target, "spec": spec,
		"applied": applied, "marks": marks, "marksAfterReload": reloaded, "afterReload": true,
		"evidenceDir":       dir,
		"paragraphTextKept": textBefore == spec.Text,
		"verificationScope": "请求的字符格式标记与段落正文；轻文档 md 导出不含字号/颜色/高亮，故以 DOM 标记为准",
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}

func isLightDocURL(target string) bool {
	return strings.Contains(target, "/docs/")
}

// 写入前必须完成两件事：协作名单只确认了自己、选区与请求完全一致。
func (b *phase3EditorBrowser) checkAndSelect(spec lightDocFormatSpec) error {
	if _, err := b.Eval(lightDocSelectJS(spec.Paragraph, spec.Text, spec.Start, spec.End)); err != nil {
		return fmt.Errorf("定位目标文字失败：%w", err)
	}
	time.Sleep(300 * time.Millisecond)
	return nil
}
