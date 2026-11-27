package main

import (
	"encoding/json"
	"os"
	"testing"
)

// Opt-in read-only investigation of the deployed spreadsheet façade. It never
// writes: the expressions only enumerate property names and object shapes.
func TestSheetFacadeProbe(t *testing.T) {
	target := os.Getenv("SHIMO_SHEET_PROBE_TARGET")
	if os.Getenv("SHIMO_SHEET_PROBE") != "1" || target == "" {
		t.Skip("opt-in sheet façade probe")
	}
	b, err := openPhase3EditorWhen(target, `Boolean(window.__SM__?.editor?.workbook||document.querySelector('canvas'))`)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	expr := `(()=>{const keys=o=>{if(!o)return[];const a=[];for(let p=o;p&&p!==Object.prototype;p=Object.getPrototypeOf(p))a.push(...Object.getOwnPropertyNames(p));return[...new Set(a)].sort()};
const sm=window.__SM__;const e=sm?.editor;
const wb=e?.workbook;const s=e?.activeSheet;
const fns=o=>keys(o).filter(k=>{try{return typeof o[k]==='function'}catch{return false}});
return {path:location.pathname,smKeys:keys(sm),editorFns:fns(e),workbookFns:fns(wb),activeSheetFns:fns(s),
activeSheetProps:keys(s).filter(k=>{try{return typeof s[k]!=='function'}catch{return true}}),
activeSheetShape:{name:s?.name,id:s?.id,rowCount:s?.rowCount,columnCount:s?.columnCount,editable:s?.editable},
tnKeys:keys(s?.tn),tnAGS:keys(s?.tn?._AGS),agsFns:fns(s?.tn?._AGS),
workbookShape:{sheetCount:wb?.getWorksheets?.length,worksheets:(wb?.getWorksheets?.()||[]).map(w=>({name:w.name,id:w.id,rows:w.rowCount,cols:w.columnCount,editable:w.editable}))},
globals:Object.keys(window).filter(k=>/SM|sdk|SDK|sheet|Sheet|workbook|Workbook|grid|Grid/.test(k)).slice(0,60),
body:(document.body?.innerText||'').slice(0,400)}})()`
	v, err := b.EvalReadOnly(expr)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	t.Logf("%s", out)
}
