package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPhase3DocumentLayoutProbe(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_LAYOUT_PROBE") != "1" {
		t.Skip("explicit read-only dedicated fixture probe")
	}
	b, err := openPhase3Editor("https://shimo.net/docx/z18oQ2oXkZSErPW7")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	v, err := b.Eval(`(()=>{const e=window._sdk.editor,a=e.docsApi;const names=['applyStyle','setDocMargins','getDocMargins','setDocPgColor','getDocPgColor','setLineSpacing','setParagraphFirstLineIndent','setParagraphLeftIndent','setParagraphShd','setColumnsProps','setParaNumbering','setCurThemeFonts','search','replace','replaceAll','scrollToPage','applyParagraphProperties','getStylesAry','getCurThemeFonts','getPageCount','getCurPageIndex','insertTable','setTableProps','addTblRowBelow','addTblColumnRight','mergeCells','splitCells','addShapeOnCurPage','addImgUrl','applyImgProperties'];const search=a.pluginManager.getInstance('Search');return {pageSources:{scroll:String(a.controller?.ScrollToPage),visible:String(a.controller?.getCurVisiblePage),visibleValue:a.getCurVisiblePage?.(),pages:a.getPageCount?.()},search:search?{keys:Object.keys(search),methods:Object.getOwnPropertyNames(Object.getPrototypeOf(search)).filter(k=>/search|find|replace/i.test(k)).map(k=>[k,String(search[k])])}:null,editorFind:String(e.find),styles:a.getStylesAry?.().map(x=>({name:x.Name,id:x.Id,keys:Object.keys(x)})),theme:a.getCurThemeFonts?.(),functions:Object.fromEntries(names.map(k=>[k,typeof a[k]==='function'?String(a[k]):null])),editorMethods:Object.getOwnPropertyNames(Object.getPrototypeOf(e)).filter(k=>/find|search|replace|page|style/i.test(k)),apiMethods:Object.getOwnPropertyNames(Object.getPrototypeOf(a)).filter(k=>/find|search|replace|page|style|margin|theme/i.test(k)),isViewMode:a.isViewMode,presence:window.__shimoPhase3Presence?.()};})()`)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	t.Log(string(out))
}
