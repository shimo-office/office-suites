package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

// Keep browser creation injectable for offline command integration tests.
// Production uses the same guarded browser constructor.
var phase3OpenDocumentEditor = openPhase3EditorWhen

const documentReadyJS = `typeof window._sdk?.editor?.getParagraphs==="function"&&typeof window._sdk?.editor?.docsApi?.setFontSize==="function"`

const documentEditableReadyJS = `(` + documentReadyJS + `)&&window._sdk.editor.docsApi.isViewMode===false&&window._sdk.editor.config?.mode?.role==="editor"&&window._sdk.editor.canHandleChanges!==false`

// SDK methods become available before the asynchronous edit-role state. This
// read-only wait precedes every mutation and never retries a submitted write.
func (b *phase3EditorBrowser) WaitDocumentEditable() error {
	return b.WaitReady(documentEditableReadyJS, 30*time.Second)
}

func documentRangeJS(s documentFormatSpec) string {
	raw, _ := json.Marshal(s)
	return `const spec=` + string(raw) + `;` + documentRangeBodyJS()
}

func documentRangeBodyJS() string {
	return `const e=window._sdk?.editor,a=e?.docsApi;if(a?.isViewMode!==false||e.config?.mode?.role!=="editor"||e.canHandleChanges===false)return {ok:false,error:"当前文稿编辑权限未确认"};const paragraphs=e.getParagraphs();const p=paragraphs[spec.paragraph-1];if(!p)return {ok:false,error:"目标段落不存在"};let range=e.getParagraphRange(p);const text=(await range.getText()).replace(/\r?\n$/,'');if(text!==spec.text)return {ok:false,error:"目标段落正文不匹配请求"};const selected=Array.from(text).slice(spec.start,spec.end).join('');
if(spec.start!==0||spec.end!==Array.from(text).length){
 const base=atob(range.start).split('|');if(base.length<4||!Array.isArray(p.children))return {ok:false,error:"当前段落位置模型不支持精确字符定位"};
 const runs=[];for(let i=0;i<p.children.length;i++){const r=p.children[i];const t=typeof r.getText==='function'?r.getText():typeof r.GetText==='function'?r.GetText():null;if(typeof t!=='string'||!Array.isArray(r.children))return {ok:false,error:"复杂段落不支持当前字符定位"};const n=Array.from(t).length;if(n===0)continue;if(n!==r.children.length)return {ok:false,error:"字符与模型索引不一致，拒绝猜测位置"};runs.push({index:i,text:t,length:n})}
 if(runs.map(r=>r.text).join('')!==text)return {ok:false,error:"段落模型正文不一致，拒绝猜测位置"};
 const position=offset=>{let used=0;for(const r of runs){if(offset<=used+r.length){const path=base.slice(0,-2);return btoa([...path,r.index,offset-used].join('|'))}used+=r.length}throw Error('字符位置越界')};
 range=e.generateRange({start:position(spec.start),end:position(spec.end)});if(await range.getText()!==selected)return {ok:false,error:"当前选区读回不符请求"};
}
`
}

func documentFormatJS(s documentFormatSpec) string {
	return `(async()=>{` + phase3PresenceGuardJS() + documentRangeJS(s) + `
const method={fontFamily:'setFontFamily',fontSize:'setFontSize',color:'setTextColor',verticalAlign:'setTextVertAlign',bold:'setBold',italic:'setItalic',underline:'setUnderline',strike:'setStrikeout'};
for(const key of Object.keys(spec.attrs)){if(typeof a[method[key]]!=='function')return {ok:false,error:'目标格式方法未就绪：'+key}}
return await e.runSelectionCallback(range,()=>{` + phase3PresenceGuardJS() + `if(a.isViewMode!==false||e.config?.mode?.role!=="editor"||e.canHandleChanges===false)return {ok:false,error:"写入前权限已改变，已中止"};for(const [key,value] of Object.entries(spec.attrs)){let v=value;if(key==='color')v={r:parseInt(value.slice(1,3),16),g:parseInt(value.slice(3,5),16),b:parseInt(value.slice(5,7),16)};if(key==='verticalAlign')v={baseline:0,superscript:1,subscript:2}[value];if(key==='underline')v={value:value?'single':'none',color:value?'auto':undefined};a[method[key]](v)}return {ok:true,selectedText:selected,applied:Object.keys(spec.attrs)}});
})()`
}

func (b *phase3EditorBrowser) waitDocumentSaved() error {
	_, err := b.Eval(`(async()=>{const e=window._sdk?.editor;if(typeof e?.collaboration?.haveUnsavedChange!=='function')return {ok:false,error:'保存状态不可验证'};for(let i=0;i<40;i++){await new Promise(r=>setTimeout(r,500));if(!e.collaboration.haveUnsavedChange()){await new Promise(r=>setTimeout(r,500));if(!e.collaboration.haveUnsavedChange())return {ok:true,saved:true}}}return {ok:false,error:'等待保存超时，可能已写入，禁止自动重放'}})()`)
	return err
}

func (a *App) cmdPhase3Document() (runErr error) {
	if len(a.Pos) != 1 {
		return fmt.Errorf("document-format 需要一个文稿链接")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareDocumentFormat(payload)
	if err != nil {
		return err
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !phase3SameOrigin(target, a.Origin) {
		return fmt.Errorf("目标 origin 不符，未写入")
	}
	guid := guidFromURL(target)
	if guid == "" {
		return fmt.Errorf("目标缺少GUID")
	}
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	attempted := false
	defer func() {
		if runErr != nil {
			if attempted {
				runErr = fmt.Errorf("写入已尝试，可能已生效；验收未通过，不要自动重放（证据 %s）：%w", dir, runErr)
			} else {
				runErr = fmt.Errorf("未执行写入（证据 %s）：%w", dir, runErr)
			}
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": attempted, "error": runErr.Error(), "evidenceDir": dir})
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"action": "document-format", "target": target, "guid": guid, "spec": spec, "version": version, "at": time.Now().UTC()}); err != nil {
		return err
	}
	beforeArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "before"), "docx")
	if err != nil {
		return err
	}
	before, err := readDocumentSnapshot(beforeArtifact.Path)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "before-readback.json"), before); err != nil {
		return err
	}
	if spec.Paragraph > len(before.Paragraphs) || before.Paragraphs[spec.Paragraph-1].Text != spec.Text {
		return fmt.Errorf("导出基线不匹配请求段落，未执行写入")
	}
	if verifyDocumentFormat(before, before, spec) == nil {
		return fmt.Errorf("目标字符已符合请求，未重复写入；现有状态不算本次修改验收")
	}
	b, err := phase3OpenDocumentEditor(target, documentReadyJS)
	if err != nil {
		return err
	}
	defer b.Close()
	if err = b.WaitDocumentEditable(); err != nil {
		return err
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	if _, err = b.Eval(`(async()=>{` + documentRangeJS(spec) + `return {ok:true,selectedText:selected}})()`); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return err
	}
	attempted = true
	submitted, err := b.Eval(documentFormatJS(spec))
	if err != nil {
		return fmt.Errorf("格式写入已尝试，可能部分生效，禁止自动重放：%w", err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return err
	}
	if err = b.waitDocumentSaved(); err != nil {
		return err
	}
	afterArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "after"), "docx")
	if err != nil {
		return fmt.Errorf("写后导出失败，可能已生效：%w", err)
	}
	after, err := readDocumentSnapshot(afterArtifact.Path)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "after-readback.json"), after); err != nil {
		return err
	}
	if err = verifyDocumentFormat(before, after, spec); err != nil {
		return fmt.Errorf("写后物证验收失败，禁止重放：%w", err)
	}
	if err = b.Capture(filepath.Join(dir, "after.png")); err != nil {
		return err
	}
	if _, err = b.Eval(`(()=>{Object.defineProperty(window,"__shimoPhase3BeforeReload",{value:true});return {ok:true}})()`); err != nil {
		return err
	}
	if _, err = b.ws.Call(8000, "Page.reload", map[string]any{"ignoreCache": true}, 15*time.Second); err != nil {
		return err
	}
	if err = b.WaitReady(`!window.__shimoPhase3BeforeReload&&(`+documentReadyJS+`)`, phase3EditorReadyTimeout); err != nil {
		return fmt.Errorf("写后刷新验证失败：%w", err)
	}
	if err = b.WaitDocumentEditable(); err != nil {
		return err
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	if _, err = b.Eval(`(async()=>{` + documentRangeJS(spec) + `return {ok:true,selectedText:selected}})()`); err != nil {
		return err
	}
	reloadedArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "reloaded"), "docx")
	if err != nil {
		return err
	}
	reloaded, err := readDocumentSnapshot(reloadedArtifact.Path)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "reload-readback.json"), reloaded); err != nil {
		return err
	}
	if err = verifyDocumentFormat(before, reloaded, spec); err != nil {
		return fmt.Errorf("刷新后持久化验收失败：%w", err)
	}
	if err = b.Capture(filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	assertions := map[string]any{"verified": true, "paragraph": spec.Paragraph, "start": spec.Start, "end": spec.End, "indexUnit": "Unicode code point", "expectedAttrs": spec.Attrs, "actualCharacters": documentExpanded(reloaded.Paragraphs[spec.Paragraph-1])[spec.Start:spec.End], "paragraphTextPreserved": true, "otherCharacterAndParagraphPropertiesPreserved": true, "afterReload": true}
	if err = phase3WriteEvidence(filepath.Join(dir, "target-node-assertions.json"), assertions); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "verified": true, "action": "document-format", "target": target, "spec": spec, "beforeArtifact": beforeArtifact, "artifact": afterArtifact, "reloadedArtifact": reloadedArtifact, "evidenceDir": dir, "verificationScope": "requested character attributes, preserved paragraph text and other measured character attributes; not complete phase3 scene acceptance"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
