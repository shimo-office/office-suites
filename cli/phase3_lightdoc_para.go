package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 段落格式：标题级别、对齐、列表、缩进、行距。全部通过工具栏控件下发。
type lightDocParaSpec struct {
	Paragraph int
	Text      string
	Attrs     map[string]any
}

func prepareLightDocParagraph(payload map[string]any) (lightDocParaSpec, error) {
	spec := lightDocParaSpec{Paragraph: int(num(payload, "paragraph")), Text: str(payload, "text")}
	if spec.Paragraph < 1 {
		return spec, errors.New("payload.paragraph 必须是 1 起的段落序号")
	}
	if strings.TrimSpace(spec.Text) == "" {
		return spec, errors.New("payload.text 必填：必须是目标段落的完整正文，用于写入前核对")
	}
	attrs, ok := payload["attrs"].(map[string]any)
	if !ok || len(attrs) == 0 {
		return spec, errors.New("payload.attrs 必填，例如 {\"heading\":1}")
	}
	spec.Attrs = map[string]any{}
	for key, value := range attrs {
		switch key {
		case "heading":
			switch v := value.(type) {
			case string:
				name := strings.TrimSpace(v)
				switch name {
				case "正文", "body", "normal":
					spec.Attrs[key] = 0
				case "标题", "title":
					spec.Attrs[key] = "title"
				case "副标题", "subtitle":
					spec.Attrs[key] = "subtitle"
				default:
					n, err := strconv.Atoi(strings.TrimPrefix(name, "标题"))
					if err != nil || n < 1 || n > 6 {
						return spec, fmt.Errorf("heading %q 不支持；用 0–6、title 或 subtitle", name)
					}
					spec.Attrs[key] = n
				}
			default:
				n := int(num(map[string]any{"v": value}, "v"))
				if n < 0 || n > 6 {
					return spec, errors.New("heading 只接受 0–6（0 为正文）")
				}
				spec.Attrs[key] = n
			}
		case "align":
			mode := strings.TrimSpace(fmt.Sprint(value))
			if _, ok := map[string]string{"left": "SvgTextAlignLeft", "center": "SvgTextAlignCenter", "right": "SvgTextAlignRight", "justify": "SvgTextAlignJustify"}[mode]; !ok {
				return spec, errors.New("align 只接受 left / center / right / justify")
			}
			spec.Attrs[key] = mode
		case "list":
			mode := strings.TrimSpace(fmt.Sprint(value))
			if _, ok := map[string]string{"ordered": "SvgTextListOrder", "unordered": "SvgTextListUnorder", "todo": "SvgCommonToDoList", "none": ""}[mode]; !ok {
				return spec, errors.New("list 只接受 ordered / unordered / todo / none")
			}
			spec.Attrs[key] = mode
		case "indent":
			n := int(num(map[string]any{"v": value}, "v"))
			if n == 0 || n > 8 || n < -8 {
				return spec, errors.New("indent 需要 1–8 或 -1 到 -8（正为增加，负为减少）")
			}
			spec.Attrs[key] = n
		case "lineHeight":
			v := strings.TrimSpace(fmt.Sprint(value))
			if v == "" {
				return spec, errors.New("lineHeight 不能为空")
			}
			spec.Attrs[key] = v
		default:
			return spec, fmt.Errorf("不支持的段落格式 %q；可用：heading align list indent lineHeight", key)
		}
	}
	return spec, nil
}

func (a *App) cmdPhase3LightDocParagraph() error {
	if len(a.Pos) != 1 {
		return errors.New("doc-paragraph 需要一个轻文档链接（/docs/）")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareLightDocParagraph(payload)
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
		return errors.New("目标不是轻文档（/docs/）；文稿请用 document-paragraph")
	}
	guid := guidFromURL(target)
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	run, err := a.lightDocStart("doc-paragraph", target, guid, dir, spec)
	if err != nil {
		return err
	}
	defer run.close()

	if _, _, err = a.lightDocExportMarkdown(guid, filepath.Join(dir, "before")); err != nil {
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
	if err = b.checkAndSelectText(spec.Paragraph, spec.Text); err != nil {
		return run.fail(err)
	}
	beforeShape, err := b.lightDocParaShape(spec.Paragraph)
	if err != nil {
		return run.fail(err)
	}
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return run.fail(err)
	}
	run.attempt = true
	if err = b.applyLightDocPara(spec); err != nil {
		return run.fail(err)
	}
	b.settleLightDoc()
	afterShape, err := b.lightDocParaShape(spec.Paragraph)
	if err != nil {
		return run.fail(err)
	}
	if err = b.assertLightDocPara(spec, afterShape); err != nil {
		return run.fail(err)
	}
	if _, ok := spec.Attrs["lineHeight"]; ok {
		if str(afterShape, "lineHeight") == str(beforeShape, "lineHeight") {
			return run.fail(fmt.Errorf("lineHeight 未发生变化（改前 %q，改后 %q）", str(beforeShape, "lineHeight"), str(afterShape, "lineHeight")))
		}
	}
	_ = phase3WriteEvidence(filepath.Join(dir, "shape.json"), map[string]any{"before": beforeShape, "after": afterShape})
	if err = b.Capture(filepath.Join(dir, "after.png")); err != nil {
		return run.fail(err)
	}
	if err = b.reloadLightDoc(userID); err != nil {
		return run.fail(err)
	}
	reloadedShape, err := b.lightDocParaShape(spec.Paragraph)
	if err != nil {
		return run.fail(fmt.Errorf("刷新后读取段落形态失败：%w", err))
	}
	if err = b.assertLightDocPara(spec, reloadedShape); err != nil {
		return run.fail(fmt.Errorf("刷新后持久化验收失败：%w", err))
	}
	if _, ok := spec.Attrs["lineHeight"]; ok && str(reloadedShape, "lineHeight") != str(afterShape, "lineHeight") {
		return run.fail(fmt.Errorf("刷新后行距未保持（写后 %q，刷新后 %q）", str(afterShape, "lineHeight"), str(reloadedShape, "lineHeight")))
	}
	if err = b.Capture(filepath.Join(dir, "reload.png")); err != nil {
		return run.fail(err)
	}
	result := map[string]any{"ok": true, "verified": true, "action": "doc-paragraph", "target": target, "spec": spec, "before": beforeShape, "after": afterShape, "afterReload": reloadedShape, "evidencedAfterReload": true, "evidenceDir": dir, "verificationScope": "请求的段落属性与段落正文（刷新后复验）"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}

func (b *phase3EditorBrowser) checkAndSelectText(paragraph int, text string) error {
	actual, _, err := b.lightDocParagraphText(paragraph)
	if err != nil {
		return fmt.Errorf("读取目标段落失败：%w", err)
	}
	if actual != text {
		return fmt.Errorf("目标段落正文不匹配请求：第%d段实际为「%s」", paragraph, truncateRunes(actual, 40))
	}
	if _, err = b.Eval(lightDocSelectJS(paragraph, text, 0, len([]rune(text)))); err != nil {
		return fmt.Errorf("定位目标段落失败：%w", err)
	}
	time.Sleep(250 * time.Millisecond)
	return nil
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func (b *phase3EditorBrowser) applyLightDocPara(spec lightDocParaSpec) error {
	b.pressEscape()
	if value, ok := spec.Attrs["heading"]; ok {
		label := "正文"
		switch v := value.(type) {
		case int:
			if v > 0 {
				label = "标题" + strconv.Itoa(v)
			}
		case string:
			if v == "title" {
				label = "标题"
			} else if v == "subtitle" {
				label = "副标题"
			}
		}
		if err := b.clickControl("", "PARAGRAPH_STYLE", "", "", 0); err != nil {
			return fmt.Errorf("打开段落样式列表失败：%w", err)
		}
		time.Sleep(900 * time.Millisecond)
		if err := b.pickPopover(label, "", ""); err != nil {
			b.pressEscape()
			return fmt.Errorf("应用段落样式 %s 失败：%w", label, err)
		}
		time.Sleep(400 * time.Millisecond)
	}
	if value, ok := spec.Attrs["align"]; ok {
		icon := map[string]string{"left": "SvgTextAlignLeft", "center": "SvgTextAlignCenter", "right": "SvgTextAlignRight", "justify": "SvgTextAlignJustify"}[fmt.Sprint(value)]
		if err := b.clickControl("SvgTextAlignLeft", "", "", "", 0); err != nil {
			return fmt.Errorf("打开对齐列表失败：%w", err)
		}
		time.Sleep(900 * time.Millisecond)
		if err := b.pickPopover("", "", icon); err != nil {
			b.pressEscape()
			return fmt.Errorf("应用对齐 %v 失败：%w", value, err)
		}
		time.Sleep(400 * time.Millisecond)
	}
	if value, ok := spec.Attrs["list"]; ok {
		mode := fmt.Sprint(value)
		if mode != "none" {
			icon := map[string]string{"ordered": "SvgTextListOrder", "unordered": "SvgTextListUnorder", "todo": "SvgCommonToDoList"}[mode]
			if err := b.clickControl(icon, "", "", "", 0); err != nil {
				return fmt.Errorf("切换列表失败：%w", err)
			}
			time.Sleep(400 * time.Millisecond)
		}
	}
	if value, ok := spec.Attrs["indent"]; ok {
		n := value.(int)
		icon := "SvgTextIndentIncrease"
		if n < 0 {
			icon = "SvgTextIndentDecrease"
			n = -n
		}
		for i := 0; i < n; i++ {
			if err := b.clickControl(icon, "", "", "", 0); err != nil {
				return fmt.Errorf("调整缩进失败：%w", err)
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
	if value, ok := spec.Attrs["lineHeight"]; ok {
		if err := b.clickControl("SvgTextLineHeight", "", "", "", 0); err != nil {
			return fmt.Errorf("打开行距列表失败：%w", err)
		}
		time.Sleep(900 * time.Millisecond)
		if err := b.pickPopover(fmt.Sprint(value), "", ""); err != nil {
			b.pressEscape()
			return fmt.Errorf("设置行距 %v 失败：%w", value, err)
		}
	}
	return nil
}

// 段落形态：标题、对齐、列表、缩进都落在 class 上，行距落在 style 上。
func (b *phase3EditorBrowser) lightDocParaShape(paragraph int) (map[string]any, error) {
	v, err := b.Eval(`(()=>{const ce=document.querySelector('` + lightDocEditorSelector + `');const ps=Array.from(ce.querySelectorAll('p.hk-paragraph-node'));const p=ps[` + strconv.Itoa(paragraph-1) + `];if(!p)return {ok:false,error:'目标段落不存在'};
const chain=[];let e=p;for(let i=0;i<5&&e&&e!==ce;i++){chain.push({tag:e.tagName,cls:(e.className||'').toString(),style:e.getAttribute('style')||''});e=e.parentElement}
const all=chain.map(x=>x.cls).join(' ');
const pick=(re)=>{const m=all.match(re);return m?m[1]:''};
const lh=(p.getAttribute('style')||'').match(/line-height:\s*([\d.]+)/);
return {ok:true,heading:pick(/hk-heading-h(\d)/),align:pick(/hk-alignment-(\w+)/),list:pick(/hk-(ordered|unordered|todo)-list-node/),indent:pick(/hk-indent-(\d+)/),lineHeight:lh?lh[1]:'',chain:chain,text:(p.innerText||'').replace(/\r?\n$/,'')}})()`)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("段落形态读取返回结构无效")
	}
	return m, nil
}

// 轻文档把行距落成换算后的 line-height 数值（1.5 倍显示为 2.5），
// 因此行距只用「面板里当前选中的倍数」对照，不比较 style 数值。
func (b *phase3EditorBrowser) lightDocLineHeightLabel() (string, error) {
	if err := b.clickControl("SvgTextLineHeight", "", "", "", 0); err != nil {
		return "", err
	}
	time.Sleep(1200 * time.Millisecond)
	v, err := b.Eval(`(()=>{const items=Array.from(document.querySelectorAll('.cg-editor-select--item'));if(!items.length)return {ok:false,error:'行距面板选项未出现'};
const picked=items.filter(e=>e.children.length>0||/active|selected|checked/i.test((e.className||'').toString()));
if(!picked.length)return {ok:false,error:'行距面板没有选中项'};
return {ok:true,label:(picked[0].textContent||'').trim()}})()`)
	b.pressEscape()
	if err != nil {
		return "", err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return "", errors.New("行距面板读取返回结构无效")
	}
	return str(m, "label"), nil
}

func (b *phase3EditorBrowser) assertLightDocPara(spec lightDocParaSpec, shape map[string]any) error {
	if want, ok := spec.Attrs["heading"]; ok {
		got := str(shape, "heading")
		wantLevel := ""
		switch v := want.(type) {
		case int:
			if v != 0 {
				wantLevel = strconv.Itoa(v)
			}
		}
		if got != wantLevel {
			return fmt.Errorf("heading 未按请求生效（实际 hk-heading-h%s，期望 h%s）", got, wantLevel)
		}
	}
	if want, ok := spec.Attrs["align"]; ok {
		got := str(shape, "align")
		if got == "" {
			got = "left"
		}
		if got != fmt.Sprint(want) {
			return fmt.Errorf("align 未按请求生效（实际 %s，期望 %v）", got, want)
		}
	}
	if want, ok := spec.Attrs["list"]; ok {
		got := str(shape, "list")
		expect := fmt.Sprint(want)
		if expect == "none" {
			expect = ""
		}
		if got != expect {
			return fmt.Errorf("list 未按请求生效（实际 %s，期望 %s）", got, expect)
		}
	}
	if _, ok := spec.Attrs["indent"]; ok {
		level, _ := strconv.Atoi(str(shape, "indent"))
		if level < 1 {
			return fmt.Errorf("indent 未按请求生效（实际缩进级别 %d）", level)
		}
	}
	// 行距的 style 数值是编辑器换算后的结果（1.5 倍落成 2.5），
	// 不与请求值直接比较，改由调用方核对「是否发生变化」与「刷新后是否保持」。
	if _, ok := spec.Attrs["lineHeight"]; ok && str(shape, "lineHeight") == "" {
		return errors.New("lineHeight 未生效：段落上没有行距标记")
	}
	return nil
}

// 导出物里带强调标记（~~ ~~、* *），计数必须按纯文本进行，
// 否则「XYZ-12345」会被格式标记切断而误判为不存在。
func lightDocPlainMarkdown(md string) string {
	replacer := strings.NewReplacer("~~", "", "**", "", "__", "", "*", "", "_", "", "`", "", "\\", "")
	return replacer.Replace(md)
}

// 查找替换：走页面查找替换面板，结果以替换前后全文计数为证。
type lightDocReplaceSpec struct {
	Find    string
	Replace string
	Scope   string
}

func prepareLightDocReplace(payload map[string]any) (lightDocReplaceSpec, error) {
	spec := lightDocReplaceSpec{Find: str(payload, "find"), Replace: str(payload, "replace"), Scope: strings.TrimSpace(str(payload, "scope"))}
	if spec.Find == "" {
		return spec, errors.New("payload.find 不能为空")
	}
	if _, ok := payload["replace"]; !ok {
		return spec, errors.New("payload.replace 必填（清空也要显式给空字符串）")
	}
	if spec.Scope == "" {
		spec.Scope = "all"
	}
	if spec.Scope != "all" && spec.Scope != "first" {
		return spec, errors.New("scope 只接受 all / first")
	}
	return spec, nil
}

func (a *App) cmdPhase3LightDocReplace() error {
	if len(a.Pos) != 1 {
		return errors.New("doc-replace 需要一个轻文档链接（/docs/）")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareLightDocReplace(payload)
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
		return errors.New("目标不是轻文档（/docs/）；文稿请用 document-replace")
	}
	guid := guidFromURL(target)
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	run, err := a.lightDocStart("doc-replace", target, guid, dir, spec)
	if err != nil {
		return err
	}
	defer run.close()

	beforePath, beforeMD, err := a.lightDocExportMarkdown(guid, filepath.Join(dir, "before"))
	if err != nil {
		return run.fail(fmt.Errorf("导出改前内容失败：%w", err))
	}
	occurrences := strings.Count(lightDocPlainMarkdown(beforeMD), spec.Find)
	if occurrences == 0 {
		return run.fail(fmt.Errorf("改前导出中找不到 %q，未执行替换", spec.Find))
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
	run.attempt = true
	if err = b.runLightDocReplace(spec); err != nil {
		return run.fail(err)
	}
	b.settleLightDoc()
	// 服务端落库是异步的：导出是服务端视角，连续导出直到命中或确认未命中。
	var afterPath, afterMD string
	for attempt := 0; attempt < 4; attempt++ {
		afterPath, afterMD, err = a.lightDocExportMarkdown(guid, filepath.Join(dir, "after"))
		if err != nil {
			return run.fail(fmt.Errorf("导出写后内容失败：%w", err))
		}
		if spec.Scope != "all" || strings.Count(lightDocPlainMarkdown(afterMD), spec.Find) == 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	remaining := strings.Count(lightDocPlainMarkdown(afterMD), spec.Find)
	added := strings.Count(lightDocPlainMarkdown(afterMD), spec.Replace) - strings.Count(lightDocPlainMarkdown(beforeMD), spec.Replace)
	if err = b.Capture(filepath.Join(dir, "after.png")); err != nil {
		return run.fail(err)
	}
	if spec.Scope == "all" && remaining != 0 {
		return run.fail(fmt.Errorf("全文替换未完成：改后仍剩 %d 处 %q", remaining, spec.Find))
	}
	if spec.Scope == "first" && remaining != occurrences-1 {
		return run.fail(fmt.Errorf("单次替换数量不符：改前 %d 处，改后剩 %d 处", occurrences, remaining))
	}
	if spec.Replace != "" && added <= 0 {
		return run.fail(fmt.Errorf("改后没有出现替换文本 %q，替换可能未生效", spec.Replace))
	}
	result := map[string]any{"ok": true, "verified": true, "action": "doc-replace", "target": target, "spec": spec,
		"beforeOccurrences": occurrences, "afterOccurrences": remaining, "replaceAdded": added,
		"before": beforePath, "after": afterPath, "evidenceDir": dir,
		"verificationScope": "替换前后导出全文的命中计数"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}

func (b *phase3EditorBrowser) runLightDocReplace(spec lightDocReplaceSpec) error {
	if err := b.clickControl("SvgCommonFindReplace", "", "", "", 0); err != nil {
		return fmt.Errorf("打开查找替换面板失败：%w", err)
	}
	time.Sleep(900 * time.Millisecond)
	if _, err := b.Eval(`(()=>{const panel=Array.from(document.querySelectorAll('input')).find(i=>/查找/.test(i.placeholder||''));if(!panel)return {ok:false,error:'查找输入框未出现'};return {ok:true}})()`); err != nil {
		return fmt.Errorf("查找替换面板未就绪：%w", err)
	}
	if err := b.fillLightDocInput("查找", spec.Find); err != nil {
		return err
	}
	if err := b.clickTextButton("更多选项"); err != nil {
		return fmt.Errorf("展开替换输入失败：%w", err)
	}
	time.Sleep(800 * time.Millisecond)
	if err := b.fillLightDocInput("替换", spec.Replace); err != nil {
		return err
	}
	label := "全部替换"
	if spec.Scope == "first" {
		label = "替换"
	}
	if _, err := b.Eval(`(()=>{const b=Array.from(document.querySelectorAll('button')).find(x=>(x.innerText||'').trim()==='` + label + `');if(!b)return {ok:false,error:'未找到「` + label + `」按钮'};b.click();return {ok:true}})()`); err != nil {
		return fmt.Errorf("执行替换失败：%w", err)
	}
	time.Sleep(1200 * time.Millisecond)
	return nil
}

// 按钮一律用真实鼠标点击：element.click() 对部分 React 组件不触发处理逻辑。
func (b *phase3EditorBrowser) clickTextButton(label string) error {
	raw, _ := json.Marshal(label)
	v, err := b.Eval(`(()=>{const want=` + string(raw) + `;const btns=Array.from(document.querySelectorAll('button')).filter(x=>x.getClientRects().length);
let el=btns.find(x=>(x.innerText||'').trim()===want);
if(!el)el=btns.find(x=>(x.getAttribute('aria-label')||'')===want||(x.getAttribute('title')||'')===want);
if(!el&&want==='更多选项')el=btns.find(x=>x.hasAttribute('aria-expanded')&&x.getAttribute('aria-expanded')!=='true');
if(!el)return {ok:false,error:'未找到按钮：'+want+'；现有：'+btns.map(x=>((x.innerText||'').trim()||x.getAttribute('aria-label')||x.getAttribute('title')||'?')).slice(0,20).join('/')};const r=el.getBoundingClientRect();return {ok:true,x:r.x+r.width/2,y:r.y+r.height/2}})()`)
	if err != nil {
		return err
	}
	m, _ := v.(map[string]any)
	x, _ := m["x"].(float64)
	y, _ := m["y"].(float64)
	if x == 0 && y == 0 {
		return errors.New("按钮没有可见坐标")
	}
	return b.clickPoint(x, y)
}

// 受控输入框必须走原生 setter + input 事件，否则 React 侧状态不会更新。
func (b *phase3EditorBrowser) fillLightDocInput(placeholder, value string) error {
	phRaw, _ := json.Marshal(placeholder)
	valRaw, _ := json.Marshal(value)
	if _, err := b.Eval(`(()=>{const el=Array.from(document.querySelectorAll('input,textarea')).find(i=>(i.placeholder||'').includes(` + string(phRaw) + `));if(!el)return {ok:false,error:'找不到输入框：` + placeholder + `'};
el.focus();const proto=el.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;const setter=Object.getOwnPropertyDescriptor(proto,'value').set;setter.call(el,` + string(valRaw) + `);el.dispatchEvent(new Event('input',{bubbles:true}));return {ok:true,value:el.value}})()`); err != nil {
		return fmt.Errorf("填写「%s」失败：%w", placeholder, err)
	}
	time.Sleep(300 * time.Millisecond)
	return nil
}
