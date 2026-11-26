package main

import (
	"encoding/json"
	"fmt"
)

// 这一组动作直接调用套件前端 SDK 的根级能力（history / comments / version /
// print），不依赖旧版 sdk.actions.*。不同套件的 SDK 挂在不同的全局对象上：
//   - 文稿（/docx/）：window._sdk（editor / docsApi）
//   - 表格（/sheets/）：window.smsdk.__DRIVE_SDK__.editor 或 globalThis.sdk
//   - 新版统一入口：globalThis.sdk
//
// 因此这里按候选根逐个探测，只调用真实存在的方法，并把实际用到的方法名
// 一并返回，便于核对。
const phase2SDKRrootsJS = `const roots=[window._sdk,window._sdk?.editor,window._sdk?.editor?.docsApi,globalThis.sdk,globalThis.sdk?.editor,window.smsdk?.__DRIVE_SDK__?.editor,window.smsdk?.__DRIVE_SDK__].filter(Boolean);`

func phase2SDKCallExpression(kind string, args any) string {
	raw, _ := json.Marshal(args)
	switch kind {
	case "history-show":
		return `(async()=>{` + phase2SDKRrootsJS + `
let used=null;for(const r of roots){try{if(typeof r.history?.show==='function'){await r.history.show();used='history.show';break}if(typeof r.actions?.showViewHistory==='function'){await r.actions.showViewHistory();used='actions.showViewHistory';break}if(typeof r.editor?.showViewHistory==='function'){await r.editor.showViewHistory();used='editor.showViewHistory';break}}catch(e){}}
if(!used)return {ok:false,error:'当前页面没有可用的历史面板接口（已探测 history.show / actions.showViewHistory）'};
await new Promise(r=>setTimeout(r,1800));
const visible=el=>{const b=el.getBoundingClientRect(),s=getComputedStyle(el);return b.width>200&&b.height>120&&s.visibility!=='hidden'&&s.display!=='none'&&Number(s.opacity||1)>0.1};
const panel=[...document.querySelectorAll('[class*="history"],[class*="History"],[class*="version"],[class*="Version"]')].filter(visible).map(el=>({cls:String(el.className).slice(0,120),text:(el.innerText||'').trim().slice(0,120)})).filter(x=>x.text);
return {ok:true,method:used,panelVisible:panel.length>0,panels:panel.slice(0,5),bodyHasHistoryText:/历史版本|版本记录|历史记录/.test(document.body.innerText||'')}})()`
	case "history-hide":
		return `(async()=>{` + phase2SDKRrootsJS + `
for(const r of roots){try{if(typeof r.history?.hide==='function'){await r.history.hide();return {ok:true,method:'history.hide'}}if(typeof r.actions?.hideViewHistory==='function'){await r.actions.hideViewHistory();return {ok:true,method:'actions.hideViewHistory'}}if(typeof r.editor?.hideViewHistory==='function'){await r.editor.hideViewHistory();return {ok:true,method:'editor.hideViewHistory'}}}catch(e){}}
return {ok:false,error:'当前页面没有可用的历史面板接口'}})()`
	case "comments-show":
		return `(async()=>{` + phase2SDKRrootsJS + `
let used=null;for(const r of roots){try{if(typeof r.comments?.show==='function'){await r.comments.show();used='comments.show';break}if(typeof r.actions?.showViewCommentList==='function'){await r.actions.showViewCommentList();used='actions.showViewCommentList';break}if(typeof r.editor?.showComments==='function'){await r.editor.showComments();used='editor.showComments';break}}catch(e){}}
if(!used)return {ok:false,error:'当前页面没有可用的评论面板接口'};
await new Promise(r=>setTimeout(r,1500));
const visible=el=>{const b=el.getBoundingClientRect(),s=getComputedStyle(el);return b.width>200&&b.height>120&&s.visibility!=='hidden'&&s.display!=='none'};
const panel=[...document.querySelectorAll('[class*="comment"],[class*="Comment"]')].filter(visible).map(el=>String(el.className).slice(0,120)).slice(0,5);
return {ok:true,method:used,panelVisible:panel.length>0,panels:panel,bodyHasCommentText:/全部评论|添加评论|评论/.test(document.body.innerText||'')}})()`
	case "comments-hide":
		return `(async()=>{` + phase2SDKRrootsJS + `
for(const r of roots){try{if(typeof r.comments?.hide==='function'){await r.comments.hide();return {ok:true,method:'comments.hide'}}if(typeof r.actions?.hideViewCommentList==='function'){await r.actions.hideViewCommentList();return {ok:true,method:'actions.hideViewCommentList'}}if(typeof r.editor?.hideComments==='function'){await r.editor.hideComments();return {ok:true,method:'editor.hideComments'}}}catch(e){}}
return {ok:false,error:'当前页面没有可用的评论面板接口'}})()`
	case "save-version":
		return `(async()=>{const args=` + string(raw) + `;` + phase2SDKRrootsJS + `
const opts=args&&Object.keys(args).length?args:undefined;
let last='当前页面没有版本接口（version.createRevision / editor.createRevision）';
for(const r of roots){for(const [owner,label] of [[r.editor,'editor.createRevision'],[r.version,'version.createRevision'],[r,'createRevision']]){if(typeof owner?.createRevision==='function'){try{const value=await owner.createRevision(opts);return {ok:true,method:label,value:value??null}}catch(e){last='创建版本失败：'+(e&&e.message||e)}}}}
return {ok:false,error:last}})()`
	case "print":
		return `(async()=>{` + phase2SDKRrootsJS + `
for(const r of roots){for(const [owner,label] of [[r,'print'],[r.editor,'editor.print'],[r.docsApi,'docsApi.print']]){if(typeof owner?.print==='function'){try{await owner.print();return {ok:true,method:label}}catch(e){return {ok:false,error:'调用打印失败：'+(e&&e.message||e)}}}}}
return {ok:false,error:'当前页面没有打印接口（sdk.print / editor.print）'}})()`
	}
	return fmt.Sprintf(`(()=>({ok:false,error:%q}))()`, "未知 SDK 动作："+kind)
}
