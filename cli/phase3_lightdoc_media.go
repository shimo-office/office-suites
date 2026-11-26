package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 文内表格与图片：轻文档没有插入接口，只能把光标放好后派发粘贴事件，
// 由编辑器自己的粘贴管线生成原生表格 / 图片节点，再用 DOM 与导出物核对。

const lightDocPasteJSHeader = `(payload=>{const ce=document.querySelector('` + lightDocEditorSelector + `');if(!ce)return {ok:false,error:'正文编辑器未就绪'};const dt=new DataTransfer();`

// 光标定位：position=after 放到段落末尾，before 放到段落开头。
func lightDocCursorJS(paragraph int, text, position string) string {
	spec, _ := json.Marshal(map[string]any{"paragraph": paragraph, "text": text, "position": position})
	return `(()=>{const spec=` + string(spec) + `;const ce=document.querySelector('` + lightDocEditorSelector + `');if(!ce)return {ok:false,error:'正文编辑器未就绪'};
const ps=Array.from(ce.querySelectorAll('p.hk-paragraph-node'));const p=ps[spec.paragraph-1];if(!p)return {ok:false,error:'目标段落不存在'};
const text=(p.innerText||'').replace(/\r?\n$/,'');if(text!==spec.text)return {ok:false,error:'目标段落正文不匹配请求'};
const w=document.createTreeWalker(p,NodeFilter.SHOW_TEXT);const nodes=[];let n,acc=0;
while(n=w.nextNode()){const len=Array.from(n.textContent).length;if(len===0)continue;nodes.push({node:n,start:acc,len:len});acc+=len}
if(!nodes.length)return {ok:false,error:'目标段落没有可定位的文字'};
const off=spec.position==='before'?0:acc;
let target=nodes[nodes.length-1],inner=target.len;
for(const it of nodes){if(off<=it.start+it.len){target=it;inner=off-it.start;break}}
// DOM Range 使用 UTF-16 偏移，不能把码点数直接当作节点内位置。
const offset=Array.from(target.node.textContent).slice(0,Math.max(0,Math.min(inner,target.len))).join('').length;
ce.focus();const r=document.createRange();r.setStart(target.node,offset);r.collapse(true);
const sel=window.getSelection();sel.removeAllRanges();sel.addRange(r);document.dispatchEvent(new Event('selectionchange'));
return {ok:true,offset:off,paragraphText:text}})()`
}

type lightDocTableSpec struct {
	Paragraph int
	Text      string
	Position  string
	Rows      int
	Cols      int
	Values    [][]string
}

func prepareLightDocTable(payload map[string]any) (lightDocTableSpec, error) {
	spec := lightDocTableSpec{Paragraph: int(num(payload, "paragraph")), Text: str(payload, "text"), Position: strings.TrimSpace(str(payload, "position"))}
	if spec.Paragraph < 1 {
		return spec, errors.New("payload.paragraph 必须是 1 起的段落序号")
	}
	if strings.TrimSpace(spec.Text) == "" {
		return spec, errors.New("payload.text 必填：目标段落完整正文，用于插入前核对")
	}
	if spec.Position == "" {
		spec.Position = "after"
	}
	if spec.Position != "after" && spec.Position != "before" {
		return spec, errors.New("position 只接受 after / before")
	}
	spec.Rows = int(num(payload, "rows"))
	spec.Cols = int(num(payload, "cols"))
	if raw, ok := payload["values"].([]any); ok {
		for _, rowRaw := range raw {
			row, ok := rowRaw.([]any)
			if !ok {
				return spec, errors.New("values 必须是二维数组")
			}
			cells := make([]string, 0, len(row))
			for _, cell := range row {
				cells = append(cells, fmt.Sprint(cell))
			}
			spec.Values = append(spec.Values, cells)
		}
	}
	if len(spec.Values) > 0 {
		spec.Rows = len(spec.Values)
		spec.Cols = 0
		for _, row := range spec.Values {
			if len(row) > spec.Cols {
				spec.Cols = len(row)
			}
		}
	}
	if spec.Rows < 1 || spec.Cols < 1 {
		return spec, errors.New("需要 rows/cols，或提供非空 values")
	}
	if spec.Rows > 50 || spec.Cols > 20 {
		return spec, errors.New("单次插入上限 50 行 20 列")
	}
	return spec, nil
}

func lightDocTableHTML(spec lightDocTableSpec) (string, string) {
	var html strings.Builder
	html.WriteString("<table><tbody>")
	var plain strings.Builder
	for r := 0; r < spec.Rows; r++ {
		html.WriteString("<tr>")
		for c := 0; c < spec.Cols; c++ {
			value := ""
			if r < len(spec.Values) && c < len(spec.Values[r]) {
				value = spec.Values[r][c]
			}
			html.WriteString("<td>" + escapeHTMLEntities(value) + "</td>")
			if c > 0 {
				plain.WriteString("\t")
			}
			plain.WriteString(value)
		}
		html.WriteString("</tr>")
		plain.WriteString("\n")
	}
	html.WriteString("</tbody></table>")
	return html.String(), plain.String()
}

func escapeHTMLEntities(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return replacer.Replace(s)
}

// 表格快照：按单元格 GUID 追踪新增表格，避免把已有表格当成结果。
func (b *phase3EditorBrowser) lightDocTableShapes() ([]map[string]any, error) {
	v, err := b.Eval(`(()=>{const ce=document.querySelector('` + lightDocEditorSelector + `');if(!ce)return {ok:false,error:'正文编辑器未就绪'};
const tables=Array.from(ce.querySelectorAll('table.hk-table-node'));
return {ok:true,tables:tables.map(t=>{const cols=(()=>{try{return (JSON.parse(t.getAttribute('data-hk-table-cols')||'[]')||[]).length}catch(e){return -1}})();
return {rows:t.getAttribute('data-hk-table-rows')||'',cols:cols,guids:Array.from(t.querySelectorAll('[data-cell-guid]')).map(c=>c.getAttribute('data-cell-guid')),text:(t.innerText||'').replace(/\s+/g,' ').trim().slice(0,120)}})}})()`)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("表格快照返回结构无效")
	}
	raw, _ := m["tables"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if table, ok := item.(map[string]any); ok {
			out = append(out, table)
		}
	}
	return out, nil
}

func tableCellKey(table map[string]any) string {
	raw, _ := table["guids"].([]any)
	parts := make([]string, 0, len(raw))
	for _, g := range raw {
		parts = append(parts, fmt.Sprint(g))
	}
	return strings.Join(parts, ",")
}

func (a *App) cmdPhase3LightDocTable() error {
	if len(a.Pos) != 1 {
		return errors.New("doc-table 需要一个轻文档链接（/docs/）")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareLightDocTable(payload)
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
		return errors.New("目标不是轻文档（/docs/）；文稿内表格请用 document-table")
	}
	guid := guidFromURL(target)
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	run, err := a.lightDocStart("doc-table", target, guid, dir, spec)
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
	beforeTables, err := b.lightDocTableShapes()
	if err != nil {
		return run.fail(err)
	}
	before := map[string]bool{}
	for _, t := range beforeTables {
		before[tableCellKey(t)] = true
	}
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return run.fail(err)
	}
	if _, err = b.Eval(lightDocCursorJS(spec.Paragraph, spec.Text, spec.Position)); err != nil {
		return run.fail(fmt.Errorf("定位插入光标失败：%w", err))
	}
	time.Sleep(300 * time.Millisecond)
	run.attempt = true
	html, plain := lightDocTableHTML(spec)
	htmlRaw, _ := json.Marshal(html)
	plainRaw, _ := json.Marshal(plain)
	if _, err = b.Eval(lightDocPasteJSHeader + `dt.setData('text/html',` + string(htmlRaw) + `);dt.setData('text/plain',` + string(plainRaw) + `);const ev=new ClipboardEvent('paste',{clipboardData:dt,bubbles:true,cancelable:true});ce.dispatchEvent(ev);return {ok:true}})()`); err != nil {
		return run.fail(fmt.Errorf("派发插入表格粘贴失败：%w", err))
	}
	b.settleLightDoc()
	afterTables, err := b.lightDocTableShapes()
	if err != nil {
		return run.fail(err)
	}
	var added map[string]any
	for _, t := range afterTables {
		if !before[tableCellKey(t)] {
			added = t
			break
		}
	}
	if added == nil {
		return run.fail(errors.New("没有检测到新增表格"))
	}
	if str(added, "rows") != strconv.Itoa(spec.Rows) || int(num(added, "cols")) != spec.Cols {
		return run.fail(fmt.Errorf("新增表格尺寸不符（实际 %s 行 %v 列，期望 %d 行 %d 列）", str(added, "rows"), added["cols"], spec.Rows, spec.Cols))
	}
	wantText := strings.Join(strings.Fields(plain), " ")
	if got := strings.Join(strings.Fields(str(added, "text")), " "); !strings.Contains(got, strings.ReplaceAll(wantText, " ", "")) && got != wantText {
		return run.fail(fmt.Errorf("新增表格内容不符（实际 %q）", str(added, "text")))
	}
	if err = b.Capture(filepath.Join(dir, "after.png")); err != nil {
		return run.fail(err)
	}
	afterPath, _, err := a.lightDocExportMarkdown(guid, filepath.Join(dir, "after"))
	if err != nil {
		return run.fail(fmt.Errorf("导出写后内容失败：%w", err))
	}
	if err = b.reloadLightDoc(userID); err != nil {
		return run.fail(err)
	}
	reloadedTables, err := b.lightDocTableShapes()
	if err != nil {
		return run.fail(err)
	}
	found := false
	for _, t := range reloadedTables {
		if tableCellKey(t) == tableCellKey(added) {
			found = true
			if str(t, "rows") != strconv.Itoa(spec.Rows) {
				return run.fail(fmt.Errorf("刷新后表格行数不符（%s）", str(t, "rows")))
			}
			break
		}
	}
	if !found {
		return run.fail(errors.New("刷新后新增表格消失，插入未持久化"))
	}
	if err = b.Capture(filepath.Join(dir, "reload.png")); err != nil {
		return run.fail(err)
	}
	result := map[string]any{"ok": true, "verified": true, "action": "doc-table", "target": target, "spec": spec,
		"tableBefore": len(beforeTables), "tableAfter": len(afterTables), "addedTable": added,
		"afterArtifact": afterPath, "afterReload": true, "evidenceDir": dir,
		"verificationScope": "新增表格的行列数、单元格内容与刷新后持久化"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}

type lightDocImageSpec struct {
	Paragraph int
	Text      string
	Position  string
	File      string
}

const lightDocImageMaxBytes = 4 << 20

func prepareLightDocImage(payload map[string]any) (lightDocImageSpec, error) {
	spec := lightDocImageSpec{
		Paragraph: int(num(payload, "paragraph")),
		Text:      str(payload, "text"),
		Position:  strings.TrimSpace(str(payload, "position")),
		File:      strings.TrimSpace(str(payload, "file")),
	}
	if spec.Paragraph < 1 {
		return spec, errors.New("payload.paragraph 必须是 1 起的段落序号")
	}
	if strings.TrimSpace(spec.Text) == "" {
		return spec, errors.New("payload.text 必填：目标段落完整正文，用于插入前核对")
	}
	if spec.Position == "" {
		spec.Position = "after"
	}
	if spec.Position != "after" && spec.Position != "before" {
		return spec, errors.New("position 只接受 after / before")
	}
	if spec.File == "" {
		return spec, errors.New("payload.file 必填：本地图片路径")
	}
	info, err := os.Stat(spec.File)
	if err != nil {
		return spec, fmt.Errorf("读取图片文件失败：%w", err)
	}
	if info.Size() > lightDocImageMaxBytes {
		return spec, fmt.Errorf("图片超过 %d MB，轻文档粘贴通道不适合传大图", lightDocImageMaxBytes>>20)
	}
	switch strings.ToLower(filepath.Ext(spec.File)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
	default:
		return spec, errors.New("只接受 png/jpg/jpeg/gif/webp")
	}
	return spec, nil
}

func lightDocImageMIME(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func (b *phase3EditorBrowser) lightDocImageCount() (int, error) {
	v, err := b.Eval(`(()=>{const ce=document.querySelector('` + lightDocEditorSelector + `');if(!ce)return {ok:false,error:'正文编辑器未就绪'};const imgs=Array.from(ce.querySelectorAll('img.hk-inline-image-node'));return {ok:true,count:imgs.length,loaded:imgs.filter(i=>i.naturalWidth>0).length}})()`)
	if err != nil {
		return 0, err
	}
	m, _ := v.(map[string]any)
	return int(num(m, "count")), nil
}

func (a *App) cmdPhase3LightDocImage() error {
	if len(a.Pos) != 1 {
		return errors.New("doc-image 需要一个轻文档链接（/docs/）")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareLightDocImage(payload)
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
		return errors.New("目标不是轻文档（/docs/）；文稿图片请用 document-graphic")
	}
	body, err := os.ReadFile(spec.File)
	if err != nil {
		return fmt.Errorf("读取图片失败：%w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(body)
	guid := guidFromURL(target)
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	run, err := a.lightDocStart("doc-image", target, guid, dir, spec)
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
	before, err := b.lightDocImageCount()
	if err != nil {
		return run.fail(err)
	}
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return run.fail(err)
	}
	if _, err = b.Eval(lightDocCursorJS(spec.Paragraph, spec.Text, spec.Position)); err != nil {
		return run.fail(fmt.Errorf("定位插入光标失败：%w", err))
	}
	time.Sleep(300 * time.Millisecond)
	run.attempt = true
	mimeRaw, _ := json.Marshal(lightDocImageMIME(spec.File))
	b64Raw, _ := json.Marshal(encoded)
	nameRaw, _ := json.Marshal(filepath.Base(spec.File))
	if _, err = b.Eval(lightDocPasteJSHeader + `const b64=` + string(b64Raw) + `;const bin=atob(b64);const u8=new Uint8Array(bin.length);for(let i=0;i<bin.length;i++)u8[i]=bin.charCodeAt(i);
const f=new File([u8],` + string(nameRaw) + `,{type:` + string(mimeRaw) + `});dt.items.add(f);
const ev=new ClipboardEvent('paste',{clipboardData:dt,bubbles:true,cancelable:true});ce.dispatchEvent(ev);return {ok:true,bytes:f.size}})()`); err != nil {
		return run.fail(fmt.Errorf("派发插入图片粘贴失败：%w", err))
	}
	b.settleLightDoc()
	after, err := b.lightDocImageCount()
	if err != nil {
		return run.fail(err)
	}
	if after <= before {
		return run.fail(fmt.Errorf("没有检测到新增图片（改前 %d 张，改后 %d 张）", before, after))
	}
	if err = b.Capture(filepath.Join(dir, "after.png")); err != nil {
		return run.fail(err)
	}
	afterPath, afterMD, err := a.lightDocExportMarkdown(guid, filepath.Join(dir, "after"))
	if err != nil {
		return run.fail(fmt.Errorf("导出写后内容失败：%w", err))
	}
	// 导出服务异步生成快照：刚插入的图片可能还没进这一次的 md。页面新增图片节点
	// 与刷新后数量已能证明写入生效，导出引用只作旁证，缺失时补一次导出再判定。
	exportReferenced := strings.Contains(afterMD, "data:image/") || strings.Contains(afterMD, "![](")
	if !exportReferenced {
		for i := 0; i < 2 && !exportReferenced; i++ {
			time.Sleep(2 * time.Second)
			if p, md, e := a.lightDocExportMarkdown(guid, filepath.Join(dir, fmt.Sprintf("after-retry%d", i+1))); e == nil {
				afterPath, afterMD = p, md
				exportReferenced = strings.Contains(md, "data:image/") || strings.Contains(md, "![](")
			}
		}
	}
	if err = b.reloadLightDoc(userID); err != nil {
		return run.fail(err)
	}
	reloaded, err := b.lightDocImageCount()
	if err != nil {
		return run.fail(err)
	}
	if reloaded < after {
		return run.fail(fmt.Errorf("刷新后图片数量减少（写后 %d，刷新后 %d）", after, reloaded))
	}
	if err = b.Capture(filepath.Join(dir, "reload.png")); err != nil {
		return run.fail(err)
	}
	result := map[string]any{"ok": true, "verified": true, "action": "doc-image", "target": target, "spec": spec,
		"imagesBefore": before, "imagesAfter": after, "imagesAfterReload": reloaded,
		"afterArtifact": afterPath, "evidenceDir": dir, "exportReferencedImage": exportReferenced,
		"verificationScope": "新增图片节点、刷新后数量与导出引用（异步导出可能滞后，缺失时以页面节点+刷新为准）",
		"storageNote":       "图片以内嵌 base64 保存，大图会显著增大文档体积"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
