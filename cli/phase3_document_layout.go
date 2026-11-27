package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const documentLayoutReadyJS = `typeof window._sdk?.editor?.getParagraphs==='function'&&typeof window._sdk?.editor?.getParagraphRange==='function'&&typeof window._sdk?.editor?.docsApi?.applyParagraphProperties==='function'`

func documentLayoutJS(s documentLayoutSpec, before documentLayoutSnapshot) string {
	request, _ := json.Marshal(s)
	nativeMargins, _ := json.Marshal(before.NativePageMargins)
	keep, _ := json.Marshal(documentPagePreserveMargins(s, before))
	section := map[string]string{}
	if len(before.SectionValues) == 1 {
		section = before.SectionValues[0]
	}
	rawSection, _ := json.Marshal(section)
	texts := []string{}
	for _, p := range before.Paragraphs {
		texts = append(texts, p.Text)
	}
	baseline, _ := json.Marshal(texts)
	return `(async()=>{` + phase3PresenceGuardJS() + `const nativeMargins=` + string(nativeMargins) + `;const spec=` + string(request) + `,baseline=` + string(baseline) + `,keep=` + string(keep) + `,section=` + string(rawSection) + `;const e=window._sdk.editor,a=e.docsApi;if(e.config?.mode?.role!=='editor'||a.isViewMode!==false||e.canHandleChanges===false)return {ok:false,error:'没有已确认文稿编辑权限'};const ps=e.getParagraphs();if(ps.length!==baseline.length)return {ok:false,error:'段落数量已变化，未写入'};for(let i=0;i<ps.length;i++){if((await e.getParagraphRange(ps[i]).getText()).replace(/\r?\n$/,'')!==baseline[i])return {ok:false,error:'正文基线已变化，未写入'}}
 const methods=spec.action==='document-theme'?['startAction','finalizeAction']:spec.action==='document-page'?[...(Object.keys(spec.marginsCm||{}).length?['updatePageSettings']:[]),...(spec.columns?['setColumnsProps']:[]),...(spec.background?['setDocPgColor']:[])]:Object.keys(spec.attrs).map(k=>({align:'setParaAlign',heading:'applyStyle',firstLineChars:'applyParagraphProperties',lineMultiple:'setLineSpacing',shading:'applyParagraphProperties',numbering:'setParaNumbering'})[k]);for(const m of methods){if(typeof a[m]!=='function')return {ok:false,error:'所需编辑方法未就绪：'+m}};
 const theme=spec.action==='document-theme'?a.controller?.docModel?.getTheme?.():null;if(spec.action==='document-theme'&&typeof theme?.setCustomThemeFont!=='function')return {ok:false,error:'主题原生更新方法未就绪'};
 const columnProps={num:spec.columns,equivalentWidth:true,Space:Number(section['cols.space']||'425')*25.4/1440,sep:section['cols.sep']==='1'||section['cols.sep']==='true'};if(spec.columns&&(!Number.isFinite(columnProps.Space)||columnProps.Space<0))return {ok:false,error:'原分栏间距不可读取，未写入'};const range=e.getParagraphRange(ps[spec.action==='document-paragraph'?spec.paragraph-1:0]);return await e.runSelectionCallback(range,()=>{` + phase3PresenceGuardJS() + `if(e.config?.mode?.role!=='editor'||a.isViewMode!==false||e.canHandleChanges===false)return {ok:false,error:'编辑权限已变化'};for(const [key,value] of Object.entries(nativeMargins||{})){const section=a.getCurPosSectionPr?.(),method=key==='header'?'getPgMarginHeader':'getPgMarginFooter';if(typeof section?.[method]!=='function'||section[method]()!==value)return {ok:false,error:'页眉/页脚距离已变化，未写入'}};const color=s=>({r:parseInt(s.slice(1,3),16),g:parseInt(s.slice(3,5),16),b:parseInt(s.slice(5,7),16)});
 if(spec.action==='document-paragraph'){for(const [key,value]of Object.entries(spec.attrs)){switch(key){case 'align':a.setParaAlign({left:1,center:2,right:0,justify:3}[value]);break;case 'heading':a.applyStyle('Heading '+value);break;case 'firstLineChars':a.applyParagraphProperties({Ind:{FirstLineChars:Math.round(value*100)}});break;case 'lineMultiple':a.setLineSpacing({type:1,value});break;case 'shading':{const c=color(value);a.applyParagraphProperties({Shd:{Value:0,Color:{docOfgetR:()=>c.r,docOfgetG:()=>c.g,docOfgetB:()=>c.b},fill:value.slice(1)}});break}case 'numbering':a.setParaNumbering(1,1);break;default:throw Error('unsupported paragraph property')}}}
 else if(spec.action==='document-theme'){if(a.controller?.docModel?.getTheme?.()!==theme||typeof theme.setCustomThemeFont!=='function')return {ok:false,error:'主题对象已变化，未写入'};a.startAction();try{theme.setCustomThemeFont(spec.themeFonts,false)}finally{a.finalizeAction()}}else{const m=spec.marginsCm||{};if(Object.keys(m).length){const pg={left:m.left*10,top:m.top*10,right:m.right*10,bottom:m.bottom*10};for(const k of Object.keys(keep))pg[k]=keep[k];a.updatePageSettings({scope:1,margins:pg})};if(spec.columns)a.setColumnsProps(columnProps);if(spec.background)a.setDocPgColor(spec.background)}
 return {ok:true,submitted:true,action:spec.action};});})()`
}

func (a *App) cmdPhase3DocumentLayout() (runErr error) {
	if len(a.Pos) != 1 {
		return fmt.Errorf("文稿布局动作需要一个目标链接")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	p, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareDocumentLayout(a.opt("--action"), p)
	if err != nil {
		return err
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !phase3SameOrigin(target, a.Origin) {
		return fmt.Errorf("目标 origin 不一致，未写入")
	}
	guid := guidFromURL(target)
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	attempted := false
	defer func() {
		if runErr != nil {
			runErr = documentLayoutFailure(runErr, attempted, dir)
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": attempted, "mayHaveApplied": attempted, "error": runErr.Error()})
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"spec": spec, "target": target, "guid": guid, "version": version, "at": time.Now().UTC()}); err != nil {
		return err
	}
	beforeArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "before"), "docx")
	if err != nil {
		return err
	}
	before, err := readDocumentLayout(beforeArtifact.Path)
	if err != nil {
		return err
	}
	if len(before.Paragraphs) == 0 || len(before.Paragraphs) > 1000 {
		return fmt.Errorf("段落数量超出当前可验证范围")
	}
	if spec.Action == "document-paragraph" && (spec.Paragraph > len(before.Paragraphs) || before.Paragraphs[spec.Paragraph-1].Text != spec.Text) {
		return fmt.Errorf("原段落序号或正文不匹配，未写入")
	}
	if spec.Action == "document-theme" && before.Theme == nil {
		return fmt.Errorf("原文稿无可验证的主题定义，未写入")
	}
	if spec.Action == "document-page" && len(before.Sections) != 1 {
		return fmt.Errorf("页面设置目前仅支持单节文稿，未写入")
	}
	if verifyDocumentLayout(before, before, spec) == nil {
		return fmt.Errorf("目标已经符合请求，未重复写入")
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "before-structure.json"), before); err != nil {
		return err
	}
	b, err := phase3OpenDocumentEditor(target, documentLayoutReadyJS)
	if err != nil {
		return err
	}
	defer b.Close()
	if err = b.CheckPresence(); err != nil {
		return err
	}
	if err = documentLayoutCapture(b, filepath.Join(dir, "before.png")); err != nil {
		return err
	}
	if documentNeedsNativeMargins(spec, before) {
		native, readErr := b.EvalReadOnly(documentNativePageMarginsJS)
		if readErr != nil {
			return readErr
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "native-page-margins-before.json"), native); err != nil {
			return err
		}
		values, ok := native.(map[string]any)
		if !ok {
			return fmt.Errorf("缺省页眉/页脚读取结构无效，未写入")
		}
		before, err = documentWithNativeMargins(before, values)
		if err != nil {
			return err
		}
	}
	attempted = true
	submitted, err := b.Eval(documentLayoutJS(spec, before))
	if err != nil {
		return fmt.Errorf("文稿布局请求未确认，可能已生效，禁止自动重放；%s：%w", dir, err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return err
	}
	if err = b.waitDocumentSaved(); err != nil {
		return err
	}
	var after documentLayoutSnapshot
	var afterArtifact ArtifactEvidence
	var proofErr error
	keep := documentPagePreserveMargins(spec, before)
	repaired := false
	for i := 0; i < 5; i++ {
		afterArtifact, proofErr = a.phase3OfficeExport(guid, filepath.Join(dir, fmt.Sprintf("after-%d", i)), "docx")
		if proofErr == nil {
			after, proofErr = readDocumentLayout(afterArtifact.Path)
		}
		if proofErr == nil && len(keep) > 0 && !repaired && documentPageMarginsReset(before, after) {
			// The editor's page-settings call silently rewrote header/footer
			// to its own default. Restore the recorded values once, then
			// re-export so the verdict still rests on the document itself.
			repaired = true
			repair, repairErr := b.Eval(documentPagePreserveJS(keep))
			if repairErr == nil {
				repairErr = b.waitDocumentSaved()
			}
			if repairErr != nil {
				proofErr = fmt.Errorf("页边距写入连带重置了页眉/页脚距离，自动恢复失败：%w", repairErr)
			} else {
				_ = phase3WriteEvidence(filepath.Join(dir, "margin-restore.json"), repair)
				time.Sleep(time.Second)
				continue
			}
		}
		if proofErr == nil {
			proofErr = verifyDocumentLayout(before, after, spec)
		}
		if proofErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if proofErr != nil {
		diff := documentLayoutDiff(before, after)
		if len(diff) > 0 {
			_ = phase3WriteEvidence(filepath.Join(dir, "failure-diff.json"), map[string]any{"ok": false, "diff": diff})
			return fmt.Errorf("已提交布局写入，但独立DOCX验收失败，禁止重放；差异：%s；%s：%w", strings.Join(diff, "；"), dir, proofErr)
		}
		return fmt.Errorf("已提交布局写入，但独立DOCX验收失败，禁止重放；%s：%w", dir, proofErr)
	}
	if err = documentLayoutCapture(b, filepath.Join(dir, "after.png")); err != nil {
		return err
	}
	b.Close()
	reopened, err := phase3OpenDocumentEditor(target, documentLayoutReadyJS)
	if err != nil {
		return err
	}
	defer reopened.Close()
	if err = reopened.CheckPresence(); err != nil {
		return err
	}
	reloadArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "reload"), "docx")
	if err != nil {
		return err
	}
	reloadSnapshot, err := readDocumentLayout(reloadArtifact.Path)
	if err != nil {
		return err
	}
	if err = verifyDocumentLayout(before, reloadSnapshot, spec); err != nil {
		return err
	}
	if err = documentLayoutCapture(reopened, filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "target-node-assertions.json"), map[string]any{"verified": true, "before": before, "after": after, "reload": reloadSnapshot}); err != nil {
		return err
	}
	scope := "requested document layout properties, unchanged text and non-target paragraphs; not full scenario acceptance"
	if spec.Action == "document-theme" {
		scope = "all six theme fonts and supplemental reset, inherited character mappings, explicit fonts and non-font content preserved; explicit-font text need not change appearance"
	}
	result := map[string]any{"ok": true, "verified": true, "action": spec.Action, "url": target, "beforeArtifact": beforeArtifact, "artifact": afterArtifact, "reloadArtifact": reloadArtifact, "evidenceDir": dir, "verificationScope": scope}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printPhase3Result(result, fmt.Sprintf("文稿布局动作验收通过：%s", spec.Action))
}

// Inspection provides stable one-based paragraph selection inputs without
// opening or mutating an editor. It does not claim online edit acceptance.
func (a *App) cmdPhase3DocumentLayoutInspect() error {
	if len(a.Pos) != 1 {
		return fmt.Errorf("document-layout-inspect 需要一个文稿链接")
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !phase3SameOrigin(target, a.Origin) {
		return fmt.Errorf("目标 origin 不一致")
	}
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	artifact, err := a.phase3OfficeExport(guidFromURL(target), dir, "docx")
	if err != nil {
		return err
	}
	snapshot, err := readDocumentLayout(artifact.Path)
	if err != nil {
		return err
	}
	result := map[string]any{"ok": true, "action": "document-layout-inspect", "url": target, "artifact": artifact, "structure": snapshot, "paragraphIndexBase": 1, "mutationAttempted": false, "verificationScope": "read-only DOCX paragraph and section structure"}
	if err = phase3WriteEvidence(filepath.Join(dir, "inspection.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}

func documentLayoutFailure(err error, attempted bool, dir string) error {
	if err == nil {
		return nil
	}
	if attempted {
		return fmt.Errorf("文稿动作未完成验收；可能已生效，禁止自动重放；证据：%s：%w", dir, err)
	}
	return fmt.Errorf("文稿动作未写入；证据：%s：%w", dir, err)
}

func documentLayoutCapture(b *phase3EditorBrowser, filename string) error {
	var last error
	deadline := time.Now().Add(10 * time.Second)
	for i := 0; i < 20; i++ {
		last = b.Capture(filename)
		if last == nil {
			return nil
		}
		if !strings.Contains(last.Error(), "截图为空白") {
			return last
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if remaining > 500*time.Millisecond {
			remaining = 500 * time.Millisecond
		}
		time.Sleep(remaining)
	}
	return fmt.Errorf("等候文稿渲染后截图仍为空白：%w", last)
}
