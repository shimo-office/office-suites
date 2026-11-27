package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// Grid operations that exist only in the page façade. Each action is a fixed
// expression authored here; callers never supply scripts or editor method names.
// Verification is layered: façade read-back, reload read-back, and an exported
// workbook comparison that must show no non-target change.

const sheetBrowserReadyJS = `Boolean(window.__SM__?.editor?.workbook?.getWorksheets&&window.__SM__?.editor?.activeSheet?.getRange)`

const sheetFacadePrelude = `const e=window.__SM__?.editor;if(!e?.activeSheet)return {found:false,error:'表格编辑器未就绪'};const s=e.activeSheet;const tn=s.tn;const core=tn?._AGS;`

func sheetFreezeStateJS() string {
	return `(()=>{` + sheetFacadePrelude + `
if(typeof core?.frozenRowCount!=='function'||typeof core?.frozenColumnCount!=='function')return {found:false,error:'冻结只读接口不可用'};
return {found:true,sheetId:s.id,sheetName:s.name,rows:core.frozenRowCount(),columns:core.frozenColumnCount()}})()`
}

func sheetFreezeApplyJS(rows, columns *int, before map[string]any) string {
	prior, _ := json.Marshal(before)
	return `(async()=>{` + sheetFacadePrelude + `
const before=` + string(prior) + `;const rows=` + intOrNull(rows) + `,columns=` + intOrNull(columns) + `;
if(s.id!==before.sheetId||s.name!==before.sheetName)return {found:false,error:'目标工作表已变化，未写入'};
if(!s.editable)return {found:false,error:'目标工作表不可编辑'};
if(core.frozenRowCount()!==before.rows||core.frozenColumnCount()!==before.columns)return {found:false,error:'冻结状态已变化，未写入'};
if(typeof e.workbook.save!=='function')return {found:false,error:'保存接口不可用，未写入'};
const wantRows=rows===null?before.rows:rows,wantColumns=columns===null?before.columns:columns;
if(rows!==null&&(typeof s.setFrozenRowCount!=='function'||rows>=s.rowCount))return {found:false,error:'冻结行数超过当前工作表或方法不可用'};
if(columns!==null&&(typeof s.setFrozenColumnCount!=='function'||columns>=s.columnCount))return {found:false,error:'冻结列数超过当前工作表或方法不可用'};
if(wantRows===before.rows&&wantColumns===before.columns)return {found:true,changed:false};
if(rows!==null)s.setFrozenRowCount(rows);
if(columns!==null)s.setFrozenColumnCount(columns);
await e.workbook.save();
return {found:true,changed:true,submitted:true}})()`
}

func sheetFilterStateJS() string {
	return `(()=>{` + sheetFacadePrelude + `
if(typeof tn?.getFilter!=='function')return {found:false,error:'筛选只读接口不可用'};
const f=tn.getFilter();const r=f&&f.range?{row:f.range.row,col:f.range.col,rowCount:f.range.rowCount,colCount:f.range.colCount}:null;
return {found:true,sheetId:s.id,sheetName:s.name,range:r}})()`
}

func sheetFilterApplyJS(spec sheetBrowserSpec, before map[string]any) string {
	prior, _ := json.Marshal(before)
	raw, _ := json.Marshal(map[string]any{"row": spec.Row, "col": spec.Column, "rowCount": spec.RowCount, "colCount": spec.ColumnCount})
	return `(async()=>{` + sheetFacadePrelude + `
const before=` + string(prior) + `,range=` + string(raw) + `;const clear=` + boolJS(spec.Action == "sheet-filter-clear") + `;
if(s.id!==before.sheetId||s.name!==before.sheetName)return {found:false,error:'目标工作表已变化，未写入'};
if(!s.editable)return {found:false,error:'目标工作表不可编辑'};
if(typeof e.workbook.save!=='function')return {found:false,error:'保存接口不可用，未写入'};
if(typeof tn.getFilter!=='function')return {found:false,error:'筛选接口不可用'};
const current=tn.getFilter();
if(clear){
  if(!current)return {found:true,changed:false};
  if(typeof tn.deleteFilter!=='function')return {found:false,error:'清除筛选方法不可用'};
  tn.deleteFilter();await e.workbook.save();return {found:true,changed:true,submitted:true};
}
if(current){
  const c=current.range||{};
  if(c.row===range.row&&c.col===range.col&&c.rowCount===range.rowCount&&c.colCount===range.colCount)return {found:true,changed:false};
  return {found:false,error:'该工作表已有筛选区域，先执行 sheet-filter-clear 再设置新区域'};
}
if(typeof tn.addFilter!=='function')return {found:false,error:'添加筛选方法不可用'};
if(range.row+range.rowCount>s.rowCount||range.col+range.colCount>s.columnCount)return {found:false,error:'筛选区域超出工作表范围'};
tn.addFilter(range);await e.workbook.save();return {found:true,changed:true,submitted:true}})()`
}

func intOrNull(v *int) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprintf("%d", *v)
}

func boolJS(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func (a *App) runSheetBrowser() (runErr error) {
	if len(a.Pos) != 1 {
		return errors.New("用法：shimo-net editor <表格链接> --action sheet-freeze|sheet-filter|sheet-filter-clear --payload '<JSON>' [--out 证据目录] [--json]")
	}
	action := strings.TrimSpace(a.opt("--action"))
	if explicitNoBrowser(a) {
		return fmt.Errorf("%s 通过表格页面前端接口执行，--no-browser 已禁止启动浏览器；本次未写入", action)
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareSheetBrowser(action, payload)
	if err != nil {
		return err
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !sameCredentialOrigin(target, a.Origin) {
		return errors.New("目标与配置 origin 不一致；未写入")
	}
	guid := guidFromURL(target)
	if guid == "" {
		return errors.New("该动作需要包含文件 guid 的表格链接")
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	if str(info, "type") != "spreadsheet" || boolv(info, "isCloudFile") {
		return errors.New("该动作只接受在线电子表格")
	}
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	attempted := false
	stage := "preflight"
	defer func() {
		if runErr != nil {
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "action": action, "stage": stage, "mutationAttempted": attempted, "mayHaveApplied": attempted, "noReplay": attempted, "error": runErr.Error()})
			if attempted {
				runErr = &commandError{Message: "表格写入已尝试，可能已保存；禁止自动重放；证据 " + dir + "：" + runErr.Error(), NoReplay: true, Result: map[string]any{"action": action, "stage": stage, "mutationAttempted": true, "mayHaveApplied": true, "noReplay": true}}
			}
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), spec); err != nil {
		return err
	}
	beforeArtifact, err := a.phase3Export(guid, filepath.Join(dir, "before"))
	if err != nil {
		return err
	}
	beforeCells, err := readArtifactSheet(beforeArtifact.Path, spec.Sheet)
	if err != nil {
		return err
	}
	userID, err := a.currentLightDocUserID()
	if err != nil {
		return err
	}
	b, err := openPhase3EditorWhen(target, sheetBrowserReadyJS)
	if err != nil {
		return err
	}
	defer func() {
		if b != nil {
			b.Close()
		}
	}()
	if err = b.uiSwitchSheet(spec.Sheet); err != nil {
		return err
	}
	if err = b.CheckLightDocPresence(userID); err != nil {
		return err
	}
	readState := sheetFilterStateJS
	if spec.Action == "sheet-freeze" {
		readState = sheetFreezeStateJS
	}
	before, err := evalSheetObject(b, readState())
	if err != nil {
		return err
	}
	if !boolv(before, "found") {
		return fmt.Errorf("%s：%s", action, str(before, "error"))
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "before-state.json"), before); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return err
	}
	var applyJS string
	if spec.Action == "sheet-freeze" {
		applyJS = sheetFreezeApplyJS(spec.FrozenRows, spec.FrozenColumns, before)
	} else {
		applyJS = sheetFilterApplyJS(spec, before)
	}
	stage = "mutation"
	attempted = true
	submittedValue, err := b.Eval(applyJS)
	if err != nil {
		return err
	}
	submitted, ok := submittedValue.(map[string]any)
	if !ok {
		return errors.New("页面写入没有返回可核验结构")
	}
	if !boolv(submitted, "found") {
		return fmt.Errorf("%s 未写入：%s", action, str(submitted, "error"))
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return err
	}
	time.Sleep(1200 * time.Millisecond)
	stage = "reload-verification"
	b.Close()
	b, err = openPhase3EditorWhen(target, sheetBrowserReadyJS)
	if err != nil {
		return err
	}
	if err = b.uiSwitchSheet(spec.Sheet); err != nil {
		return err
	}
	after, err := evalSheetObject(b, readState())
	if err != nil {
		return err
	}
	if !boolv(after, "found") {
		return fmt.Errorf("重开后读取%s失败：%s", action, str(after, "error"))
	}
	if err = verifySheetBrowserState(spec, before, after); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	afterArtifact, err := a.phase3Export(guid, filepath.Join(dir, "after"))
	if err != nil {
		return err
	}
	afterCells, err := readArtifactSheet(afterArtifact.Path, spec.Sheet)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(beforeCells, afterCells) {
		return errors.New("操作后单元格内容、样式或合并结构出现非目标差异")
	}
	result := map[string]any{
		"ok": true, "action": spec.Action, "target": target, "transport": "browser-facade",
		"changed": boolv(submitted, "changed"), "submitted": boolv(submitted, "submitted"),
		"saved": true, "verified": true, "afterReload": true,
		"before": before, "after": after,
		"beforeArtifact": beforeArtifact, "afterArtifact": afterArtifact,
		"evidenceDir": dir, "screenshot": filepath.Join(dir, "reload.png"),
		"view": a.agentView(viewTransportCLIBrowser, target), "auth": a.agentAuth(),
		"verification": map[string]any{
			"level":    "reload",
			"scope":    "目标状态在重开页面后保持；导出工作簿的单元格值、样式与合并结构未改变",
			"evidence": []string{"before-state.json", "submission.json", "reload.png", "after/*.xlsx"},
		},
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	if a.JSON {
		return a.printJSON(result)
	}
	fmt.Printf("%s 已保存并重开核验：%s\n证据：%s\n", action, terminalText(target), terminalText(dir))
	return nil
}

// evalSheetObject requires a structured reply so a partially rendered page can
// never be mistaken for a state.
func evalSheetObject(b *phase3EditorBrowser, expr string) (map[string]any, error) {
	v, err := b.EvalReadOnly(expr)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("页面返回结构无效（期望对象）")
	}
	return m, nil
}

func verifySheetBrowserState(spec sheetBrowserSpec, before, after map[string]any) error {
	if str(before, "sheetId") == "" || str(before, "sheetId") != str(after, "sheetId") || str(after, "sheetName") != spec.Sheet {
		return errors.New("刷新后工作表身份不一致")
	}
	switch spec.Action {
	case "sheet-freeze":
		wantRows, wantCols := num(before, "rows"), num(before, "columns")
		if spec.FrozenRows != nil {
			wantRows = float64(*spec.FrozenRows)
		}
		if spec.FrozenColumns != nil {
			wantCols = float64(*spec.FrozenColumns)
		}
		if num(after, "rows") != wantRows || num(after, "columns") != wantCols {
			return fmt.Errorf("冻结持久化未通过：期望行/列 %.0f/%.0f，实际 %.0f/%.0f", wantRows, wantCols, num(after, "rows"), num(after, "columns"))
		}
	case "sheet-filter":
		want := map[string]any{"row": float64(spec.Row), "col": float64(spec.Column), "rowCount": float64(spec.RowCount), "colCount": float64(spec.ColumnCount)}
		got, _ := after["range"].(map[string]any)
		if got == nil {
			return errors.New("筛选区域未持久化：重开后没有筛选")
		}
		for key, value := range want {
			if num(got, key) != value.(float64) {
				return fmt.Errorf("筛选区域未持久化：%s 期望 %.0f，实际 %.0f", key, value, num(got, key))
			}
		}
	case "sheet-filter-clear":
		if after["range"] != nil {
			return errors.New("筛选未清除：重开后仍存在筛选区域")
		}
	default:
		return fmt.Errorf("未知动作 %q", spec.Action)
	}
	return nil
}
