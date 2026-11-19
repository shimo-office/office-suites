package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPhase3DocumentGraphicReadOnlyProbe(t *testing.T) {
	mode := os.Getenv("SHIMO_PHASE3_GRAPHIC_PROBE")
	if mode != "1" && mode != "native" {
		t.Skip("opt-in read-only dedicated graphics fixture")
	}
	target := "https://shimo.net/docx/MWn7L07rygswnn1p"
	if mode == "native" {
		target = "https://shimo.net/docx/MWn7L07rXetm8G1p"
		if os.Getenv("SHIMO_PHASE3_GRAPHIC_STABLE") == "1" {
			target = "https://shimo.net/docx/9qK56V5w0QIl5EWb"
		}
	}
	parent := "../tmp/phase3-live/docx-slides/31-graphics"
	if mode == "native" {
		parent = "../tmp/phase3-live/docx-slides/31-native-shapes"
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(parent, "readonly-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("evidence", dir)
	app := newApp([]string{"phase3"})
	beforeArtifact, err := app.phase3OfficeExport(guidFromURL(target), filepath.Join(dir, "before-export"), "docx")
	if err != nil {
		t.Fatal(err)
	}
	b, err := openPhase3EditorWhen(target, documentReadyJS)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	v, err := b.EvalReadOnly(`(()=>{const a=window._sdk.editor.docsApi;const d=a.controller?.docModel?.drawings;const properties=o=>o?Object.fromEntries(Object.entries(o).filter(([k,v])=>/title|descr|page|^id$|wrap|^rot$|^ext[XY]$/i.test(k)&&['string','number','boolean'].includes(typeof v))):null;const methods=o=>{const keys=new Set;for(let p=o;p&&p!==Object.prototype;p=Object.getPrototypeOf(p)){for(const k of Object.getOwnPropertyNames(p)){if(/select|title|descr|page|wrap|rot|props|transform/i.test(k)&&typeof Object.getOwnPropertyDescriptor(p,k)?.value==='function')keys.add(k)}}return [...keys]};if(typeof a.getParaDrawings!=='function')return {ok:false,error:'missing image enumeration'};return {ok:true,api:{applyImgProperties:String(a.applyImgProperties).slice(0,6000),setShapeAlign:typeof a.setShapeAlign,getShapeSelected:typeof a.getShapeSelected},manager:!!d,managerMethods:methods(d),pages:(d?.drawingPages||[]).map((page,index)=>({index,objects:['inlineObjects','wrappingObjects','frontTextObjects','behindTextObjects'].flatMap(kind=>(page?.[kind]||[]).map(o=>({kind,shape:properties(o),shapeKeys:Object.keys(o),methods:methods(o),parent:properties(o.parent),parentKeys:Object.keys(o.parent||{}),docPr:o.parent?.docPr?{...properties(o.parent.docPr),name:o.parent.docPr.name,descr:o.parent.docPr.descr,keys:Object.keys(o.parent.docPr)}:null,nativeNv:typeof o.getNvPr==='function'?properties(o.getNvPr()):null,getNvPrSource:typeof o.getNvPr==='function'?String(o.getNvPr).slice(0,1000):null,shapeSources:{setTitle:String(o.setTitle),setDescription:String(o.setDescription),getTitle:String(o.getTitle),getDescription:String(o.getDescription)},geometry:o.spPr?.geometry?{preset:o.spPr.geometry.preset,keys:Object.keys(o.spPr.geometry)}:null,nv:properties(o.nvSpPr?.cNvPr),xfrm:properties(o.spPr?.xfrm)})))})),pictures:a.getParaDrawings().map(p=>({para:properties(p),paraKeys:Object.keys(p),paraMethods:methods(p),image:properties(p.graphicsObj),imageKeys:Object.keys(p.graphicsObj||{}),imageMethods:methods(p.graphicsObj),parent:properties(p.graphicsObj?.parent),xfrm:properties(p.graphicsObj?.spPr?.xfrm),nv:properties(p.graphicsObj?.nvPicPr?.cNvPr)}))}})()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "objects.json"), v); err != nil {
		t.Fatal(err)
	}

	if mode == "native" && os.Getenv("SHIMO_PHASE3_GRAPHIC_METADATA_ONLY") != "1" {
		spec := documentGraphicSpec{Name: "PHASE3_NATIVE_RECTANGLE", Description: "PHASE3_NATIVE_RECTANGLE_20261118"}
		if os.Getenv("SHIMO_PHASE3_GRAPHIC_POSITION_PROBE") == "1" {
			spec = documentGraphicSpec{Paragraph: 2, ExpectedParagraph: "", Drawing: 1}
		}
		source, err := readDocumentGraphic(beforeArtifact.Path)
		if err != nil {
			t.Fatal(err)
		}
		baseline, err := documentGraphicBaseline(source.Root, spec)
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "selection-baseline.json"), baseline); err != nil {
			t.Fatal(err)
		}
		native, err := b.EvalReadOnly(documentGraphicRuntimeScript(spec, baseline, false, true, ""))
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "selection.json"), native); err != nil {
			t.Fatal(err)
		}
		ui, err := b.EvalReadOnly(`(async()=>{await new Promise(r=>setTimeout(r,400));const visible=e=>e.getClientRects().length>0;return {ok:true,resources:performance.getEntriesByType('resource').map(r=>r.name).filter(s=>/\.js(?:\?|$)/.test(s)).map(s=>{const u=new URL(s);return u.origin+u.pathname}),scripts:[...document.scripts].map(s=>s.src).filter(Boolean).map(s=>{const u=new URL(s);return u.origin+u.pathname}),body:(document.body?.innerText||'').slice(0,5000),buttons:[...document.querySelectorAll('button,[role="button"],[role="menuitem"],[title],[aria-label]')].filter(visible).map(e=>({text:(e.innerText||'').slice(0,100),aria:e.getAttribute('aria-label'),title:e.getAttribute('title'),testid:e.getAttribute('data-testid'),role:e.getAttribute('role'),tag:e.tagName,className:typeof e.className==='string'?e.className:'',disabled:e.disabled===true||e.getAttribute('aria-disabled')==='true'})).slice(0,160)}})()`)
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "toolbar.json"), ui); err != nil {
			t.Fatal(err)
		}

		gallery, err := b.EvalReadOnly(`(()=>{const summary=v=>{if(v===null||['string','number','boolean'].includes(typeof v))return v;if(typeof v==='function')return {functionSource:String(v).slice(0,1800)};if(!v||typeof v!=='object')return typeof v;return {constructor:v.constructor?.name,keys:Object.keys(v).slice(0,50)}};const leaves=[...document.querySelectorAll('*')].filter(e=>e.childElementCount===0&&e.textContent==='Abc'&&e.getClientRects().length);return {ok:true,options:leaves.map(e=>{const parents=[];for(let n=e,depth=0;n&&depth<5;n=n.parentElement,depth++){const key=Object.keys(n).find(k=>k.startsWith('__reactProps'));parents.push({tag:n.tagName,html:n.outerHTML.slice(0,2600),props:key?Object.fromEntries(Object.entries(n[key]).filter(([k])=>k!=='children').map(([k,v])=>[k,summary(v)])):null})}const fibers=[];const fk=Object.keys(e).find(k=>k.startsWith('__reactFiber'));for(let f=fk?e[fk]:null,depth=0;f&&depth<12;f=f.return,depth++)fibers.push({key:f.key,type:typeof f.type==='function'?String(f.type).slice(0,9000):f.type?.displayName||f.type,props:Object.fromEntries(Object.entries(f.memoizedProps||{}).filter(([k])=>k!=='children').map(([k,v])=>[k,summary(v)]))});return {parents,fibers}})}})()`)
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "gallery-elements.json"), gallery); err != nil {
			t.Fatal(err)
		}
	}
	if err = b.Capture(filepath.Join(dir, "objects.png")); err != nil {
		t.Fatal(err)
	}
	afterArtifact, err := app.phase3OfficeExport(guidFromURL(target), filepath.Join(dir, "after-export"), "docx")
	if err != nil {
		t.Fatal(err)
	}
	spec := documentGraphicSpec{Name: "PHASE3_ARROW", Description: "PHASE3_ARROW_20261118"}
	if mode == "native" {
		spec = documentGraphicSpec{Name: "PHASE3_NATIVE_RECTANGLE", Description: "PHASE3_NATIVE_RECTANGLE_20261118"}
		if os.Getenv("SHIMO_PHASE3_GRAPHIC_POSITION_PROBE") == "1" {
			spec = documentGraphicSpec{Paragraph: 2, ExpectedParagraph: "", Drawing: 1}
		}
	}
	if err = verifyDocumentGraphic(beforeArtifact.Path, afterArtifact.Path, spec); err != nil {
		t.Fatal("read-only probe changed content", err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "read-only-proof.json"), map[string]any{"ok": true, "contentMutationAttempted": false, "selectionOnly": mode == "native", "target": target, "beforeArtifact": beforeArtifact, "afterArtifact": afterArtifact, "documentXMLAndAssetsUnchanged": true}); err != nil {
		t.Fatal(err)
	}
}
