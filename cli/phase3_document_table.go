package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

func documentTableJS(s documentTableSpec, before documentTablesSnapshot) string {
	if s.Operation == "merge" || s.Operation == "split" {
		return documentTableSpanJS(s, before)
	}
	if s.Operation == "distribute-rows" || s.Operation == "distribute-columns" {
		return documentTableDistributeJS(s, before)
	}
	return documentTableOperationJS(s, before, documentTableMethods[s.Operation], "", "a[method]();")
}

func documentTableOperationJS(s documentTableSpec, before documentTablesSnapshot, methodName, preflight, mutation string) string {
	anchor, _ := documentTableAnchor(before, s)
	request, _ := json.Marshal(s)
	baseline, _ := json.Marshal(before.Tables)
	method, _ := json.Marshal(methodName)
	return `(async()=>{` + phase3PresenceGuardJS() + documentRangeJS(anchor) + `const request=` + string(request) + `,baseline=` + string(baseline) + `,method=` + string(method) + `;
 const cellFor=p=>p?.getParent?.()?.getParent?.();const cell=cellFor(p),row=cell?.Row,table=row?.Table;const tables=[];for(const p of paragraphs){const t=cellFor(p)?.Row?.Table;if(t&&!tables.includes(t))tables.push(t)}
 if(tables.length!==baseline.length||tables[request.table-1]!==table||row?.Index!==request.row-1||cell?.Index!==request.column-1)return {ok:false,error:'表格或单元格模型位置与导出基线不一致'};
 const distributionAction=request.operation==='distribute-rows'||request.operation==='distribute-columns';
 const stableParagraphs=paragraphs.slice();
 const modelText=p=>{if(!Array.isArray(p?.children))return null;let text='';try{for(const run of p.children){const value=typeof run.getText==='function'?run.getText():typeof run.GetText==='function'?run.GetText():null;if(typeof value!=='string')return null;text+=value}}catch{return null}return text};
 const validate=()=>{const current=distributionAction?e.getParagraphs():paragraphs;if(distributionAction&&(!Array.isArray(current)||current.length!==stableParagraphs.length||current.some((p,i)=>p!==stableParagraphs[i])))return false;for(let ti=0;ti<tables.length;ti++){const t=tables[ti],bt=baseline[ti];if(!Array.isArray(t.children)||t.children.length!==bt.rows.length)return false;for(let ri=0;ri<t.children.length;ri++){const r=t.children[ri];if(!Array.isArray(r.children)||r.children.length!==bt.rows[ri].length)return false;for(let ci=0;ci<r.children.length;ci++){const c=r.children[ci],bc=bt.rows[ri][ci];if(c.getGridSpan?.()!==1||c.getVMergedRows?.()!==1||cellFor(current[bc.paragraph-1])!==c)return false;if(distributionAction){const ps=current.filter(p=>cellFor(p)===c);if(c.Row!==r||c.Index!==ci||ps.length!==bc.paragraphs.length||ps.some((p,i)=>p!==stableParagraphs[bc.paragraphs[i]-1]||modelText(p)!==bc.paragraphTexts[i]))return false}}}}return true};
 if(!validate()||typeof a[method]!=='function')return {ok:false,error:'表格网格或编辑方法不受支持'};
 for(const bt of baseline)for(const r of bt.rows)for(const c of r){const indices=distributionAction?c.paragraphs:[c.paragraph],texts=distributionAction?c.paragraphTexts:[c.text];for(let i=0;i<indices.length;i++){if((await e.getParagraphRange(paragraphs[indices[i]-1]).getText()).replace(/\r?\n$/,'')!==texts[i])return {ok:false,error:'表格正文已变化，未写入'}}}
 ` + preflight + `
 return await e.runSelectionCallback(range,()=>{` + phase3PresenceGuardJS() + `if(a.isViewMode!==false||e.config?.mode?.role!=='editor'||e.canHandleChanges===false)return {ok:false,error:'写入前编辑权限已改变'};if(!validate()||table.curCell!==cell)return {ok:false,error:'当前选区不是目标单元格，未写入'};` + mutation + `return {ok:true,submitted:true,operation:request.operation,table:request.table,row:request.row,column:request.column}});})()`
}

func (a *App) cmdPhase3DocumentTable() (runErr error) {
	if len(a.Pos) != 1 {
		return fmt.Errorf("document-table 需要一个文稿链接")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareDocumentTable(payload)
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
	if guid == "" {
		return fmt.Errorf("目标缺少 GUID")
	}
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	attempted := false
	defer func() {
		if runErr != nil {
			if attempted {
				runErr = fmt.Errorf("已尝试写入，可能已生效；验收未通过，不要自动重放（证据 %s）：%w", dir, runErr)
			} else {
				runErr = fmt.Errorf("未执行写入（证据 %s）：%w", dir, runErr)
			}
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": attempted, "error": runErr.Error()})
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"action": "document-table", "target": target, "spec": spec, "version": version, "at": time.Now().UTC()}); err != nil {
		return err
	}
	beforeArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "before"), "docx")
	if err != nil {
		return err
	}
	before, err := readDocumentTables(beforeArtifact.Path)
	if err != nil {
		return err
	}
	if _, err = documentTableAnchor(before, spec); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "before-readback.json"), before); err != nil {
		return err
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
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return err
	}
	attempted = true
	submitted, err := b.Eval(documentTableJS(spec, before))
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return err
	}
	if err = b.waitDocumentSaved(); err != nil {
		return err
	}
	afterArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "after"), "docx")
	if err != nil {
		return err
	}
	after, err := readDocumentTables(afterArtifact.Path)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "after-readback.json"), after); err != nil {
		return err
	}
	if err = verifyDocumentTable(before, after, spec); err != nil {
		return err
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
		return err
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	reloadArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "reload"), "docx")
	if err != nil {
		return err
	}
	reloaded, err := readDocumentTables(reloadArtifact.Path)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "reload-readback.json"), reloaded); err != nil {
		return err
	}
	if err = verifyDocumentTable(before, reloaded, spec); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "target-node-assertions.json"), map[string]any{"verified": true, "spec": spec, "before": before, "after": after, "reloaded": reloaded}); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "verified": true, "action": "document-table", "target": target, "spec": spec, "beforeArtifact": beforeArtifact, "artifact": afterArtifact, "reloadArtifact": reloadArtifact, "evidenceDir": dir, "verificationScope": "requested table structure or distribution, preserved non-target content/style/resources; merge/split covers horizontal existing equal-width grid only"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
