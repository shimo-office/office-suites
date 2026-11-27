package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// 这一组动作直接调用套件前端 SDK 的根级能力（history / comments / version /
// print），不依赖旧版 sdk.actions.*。不同套件的 SDK 挂在不同的全局对象上：
//   - 产品页统一入口：window.__SM__.editor（文档 / 文稿 / 表格实测可用，
//     即文档里的 sdk.history.show / sdk.comments.show / sdk.print）
//   - 文稿（/docx/）：window._sdk（editor / docsApi）
//   - 表格（/sheets/）：window.smsdk.__DRIVE_SDK__.editor 或 globalThis.sdk
//
// 因此这里按候选根逐个探测，只调用真实存在的方法，并把实际用到的方法名
// 一并返回，便于核对。__SM__ 必须排在最前：产品页只有它稳定暴露根级 facade，
// 旧入口 _sdk 只有 editor/docsApi，没有 history/comments/version/print。
const phase2SDKRrootsJS = `const roots=[window.__SM__?.sdk,window.__SM__,window.__SM__?.editor,window._sdk,window._sdk?.editor,window._sdk?.editor?.docsApi,globalThis.sdk,globalThis.sdk?.editor,window.smsdk?.__DRIVE_SDK__?.editor,window.smsdk?.__DRIVE_SDK__].filter(Boolean);`

// phase2PanelProbeJS 在动作执行后轮询面板是否真的渲染出来。产品页的面板是
// 异步挂载的，固定 sleep 很容易在慢机器上误判成"调用成功但没面板"。
// 判定同时要求：靠近右边缘、有实际尺寸、且面板文本命中目标关键词。
func phase2PanelProbeJS(want string) string {
	return `
const wanted=` + want + `;
const panelVisible=()=>[...document.querySelectorAll('[class*="history"],[class*="History"],[class*="comment"],[class*="Comment"],[class*="version"],[class*="Version"],[class*="panel"],[class*="Panel"]')].some(el=>{const b=el.getBoundingClientRect(),s=getComputedStyle(el);return b.width>200&&b.height>120&&b.right>innerWidth-40&&s.visibility!=='hidden'&&s.display!=='none'&&Number(s.opacity||1)>0.1&&wanted.test(el.innerText||'')});
let seen=false;for(let i=0;i<14;i++){if(panelVisible()){seen=true;break}await new Promise(r=>setTimeout(r,300))}`
}

// cellHistoryJS 走真实的页面右键菜单：单元格历史没有对外 SDK 方法，只能
// 在表格画布上触发 contextmenu，再点"查看单元格历史"。产品页的菜单挂在
// shadow DOM 里，普通 querySelectorAll 找不到，这里做深度遍历。
const cellHistoryJS = `(async()=>{
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const canvas=document.querySelector('canvas');
if(!canvas)return {ok:false,error:'当前页面没有表格画布，不像是表格页'};
const deep=[];
const walk=root=>{for(const el of root.querySelectorAll('*')){deep.push(el);if(el.shadowRoot)walk(el.shadowRoot)}};
const collect=()=>{deep.length=0;walk(document);return deep};
const b=canvas.getBoundingClientRect();
const x=Math.round(b.left+110),y=Math.round(b.top+70);
const target=document.elementFromPoint(x,y)||canvas;
for(const t of ['mousedown','mouseup','contextmenu']){target.dispatchEvent(new MouseEvent(t,{bubbles:true,cancelable:true,clientX:x,clientY:y,button:2,buttons:2,view:window}))}
await sleep(1000);
let nodes=collect();
const hit=nodes.filter(el=>el.children.length===0&&/查看单元格历史/.test(el.textContent||''));
if(!hit.length)return {ok:false,error:'右键菜单没有出现（未找到“查看单元格历史”），可能该位置不是数据单元格',point:{x,y},sample:(document.body.innerText||'').slice(0,160)};
const item=hit[0];
for(const t of ['mousedown','mouseup','click']){item.dispatchEvent(new MouseEvent(t,{bubbles:true,cancelable:true,view:window}))}
await sleep(1800);
nodes=collect();
const panel=nodes.filter(el=>{const r=el.getBoundingClientRect();return r.width>160&&r.height>60&&/单元格历史/.test(el.innerText||'')}).map(el=>({cls:String(el.className).slice(0,80),text:(el.innerText||'').trim().slice(0,200)}));
return {ok:true,point:{x,y},panelVisible:panel.length>0,panels:panel.slice(0,3),bodyText:/单元格历史/.test(document.body.innerText||'')}})()`

func phase2SDKCallExpression(kind string, args any) string {
	raw, _ := json.Marshal(args)
	switch kind {
	case "history-show":
		return `(async()=>{` + phase2SDKRrootsJS + `
let used=null,returned=null;
// 第一轮只认新根级 facade（sdk.history.show）。旧 actions.showViewHistory
// 在同一页面也存在，但调用后不会渲染面板，因此必须放到第二轮兜底。
for(const r of roots){try{if(typeof r.history?.show==='function'){returned=await r.history.show();used='history.show';break}}catch(e){}}
if(!used){if(window.__SM__)return {ok:false,error:'产品页前端 SDK 尚未就绪（没有 sdk.history.show），请稍后重试'};for(const r of roots){try{if(typeof r.actions?.showViewHistory==='function'){await r.actions.showViewHistory();used='actions.showViewHistory';break}if(typeof r.editor?.showViewHistory==='function'){await r.editor.showViewHistory();used='editor.showViewHistory';break}}catch(e){}}}
if(!used)return {ok:false,error:'当前页面没有可用的历史面板接口（已探测 history.show / actions.showViewHistory）'};
` + phase2PanelProbeJS("/历史|版本/") + `
const panel=[...document.querySelectorAll('[class*="history"],[class*="History"],[class*="version"],[class*="Version"]')].map(el=>({cls:String(el.className).slice(0,120),text:(el.innerText||'').trim().slice(0,120)})).filter(x=>x.text).slice(0,5);
return {ok:true,method:used,returned:returned??null,panelVisible:seen,panels:panel,bodyHasHistoryText:/历史版本|版本记录|历史记录/.test(document.body.innerText||'')}})()`
	case "history-hide":
		return `(async()=>{` + phase2SDKRrootsJS + `
for(const r of roots){try{if(typeof r.history?.hide==='function'){await r.history.hide();return {ok:true,method:'history.hide'}}}catch(e){}}
for(const r of roots){try{if(typeof r.actions?.hideViewHistory==='function'){await r.actions.hideViewHistory();return {ok:true,method:'actions.hideViewHistory'}}if(typeof r.editor?.hideViewHistory==='function'){await r.editor.hideViewHistory();return {ok:true,method:'editor.hideViewHistory'}}}catch(e){}}
return {ok:false,error:'当前页面没有可用的历史面板接口'}})()`
	case "comments-show":
		return `(async()=>{` + phase2SDKRrootsJS + `
let used=null;
for(const r of roots){try{if(typeof r.comments?.show==='function'){await r.comments.show();used='comments.show';break}}catch(e){}}
if(!used){for(const r of roots){try{if(typeof r.actions?.showViewCommentList==='function'){await r.actions.showViewCommentList();used='actions.showViewCommentList';break}if(typeof r.editor?.showComments==='function'){await r.editor.showComments();used='editor.showComments';break}}catch(e){}}}
if(!used)return {ok:false,error:'当前页面没有可用的评论面板接口'};
` + phase2PanelProbeJS("/评论|提及/") + `
const panel=[...document.querySelectorAll('[class*="comment"],[class*="Comment"]')].map(el=>String(el.className).slice(0,120)).slice(0,5);
return {ok:true,method:used,panelVisible:seen,panels:panel,bodyHasCommentText:/全部评论|添加评论|评论/.test(document.body.innerText||'')}})()`
	case "comments-hide":
		return `(async()=>{` + phase2SDKRrootsJS + `
for(const r of roots){try{if(typeof r.comments?.hide==='function'){await r.comments.hide();return {ok:true,method:'comments.hide'}}}catch(e){}}
for(const r of roots){try{if(typeof r.actions?.hideViewCommentList==='function'){await r.actions.hideViewCommentList();return {ok:true,method:'actions.hideViewCommentList'}}if(typeof r.editor?.hideComments==='function'){await r.editor.hideComments();return {ok:true,method:'editor.hideComments'}}}catch(e){}}
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

const phase2EditorReadyTimeout = 60 * time.Second

// Wait for the method that will actually be executed, using the same root list
// as phase2SDKCallExpression. A different mounted method is not readiness.
func phase2ReadyExpression(method string) string {
	var capability string
	switch method {
	case "sdk.history.show":
		capability = `roots.some(r=>typeof r.history?.show==='function')||(!window.__SM__&&roots.some(r=>typeof r.actions?.showViewHistory==='function'||typeof r.editor?.showViewHistory==='function'))`
	case "sdk.history.hide":
		capability = `roots.some(r=>typeof r.history?.hide==='function'||typeof r.actions?.hideViewHistory==='function'||typeof r.editor?.hideViewHistory==='function')`
	case "sdk.comments.show":
		capability = `roots.some(r=>typeof r.comments?.show==='function'||typeof r.actions?.showViewCommentList==='function'||typeof r.editor?.showComments==='function')`
	case "sdk.comments.hide":
		capability = `roots.some(r=>typeof r.comments?.hide==='function'||typeof r.actions?.hideViewCommentList==='function'||typeof r.editor?.hideComments==='function')`
	case "sdk.print":
		capability = `roots.some(r=>typeof r.print==='function'||typeof r.editor?.print==='function'||typeof r.docsApi?.print==='function')`
	case "sdk.version.createRevision":
		capability = `roots.some(r=>typeof r.editor?.createRevision==='function'||typeof r.version?.createRevision==='function'||typeof r.createRevision==='function')`
	case "sdk.cellHistory":
		capability = `Boolean(document.querySelector('canvas'))`
	default:
		if !strings.HasPrefix(method, "actions.") {
			return `false`
		}
		raw, _ := json.Marshal(strings.Split(method, "."))
		capability = `roots.some(r=>{let value=r;for(const key of ` + string(raw) + `){value=value?.[key]}return typeof value==='function'})`
	}
	return `(()=>{` + phase2SDKRrootsJS + `
const ed=window.__SM__?.editor, sdk=window.__SM__?.sdk;
if(ed?.initialized===false||sdk?._rendered===false)return false;
const rendered=roots.some(r=>r.initialized===true||r._rendered===true)||Boolean(document.querySelector('canvas,.ProseMirror,[contenteditable="true"],.ql-editor'));
if(!rendered||!(document.body?.innerText||'').trim())return false;
return Boolean(` + capability + `);})()`
}
