package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

func prepareDocumentMention(p map[string]any) (documentFormatSpec, error) {
	for k := range p {
		if k != "paragraph" && k != "text" {
			return documentFormatSpec{}, fmt.Errorf("document-mention 仅接受 paragraph/text，目标始终为当前本人")
		}
	}
	s, err := prepareDocumentFormat(map[string]any{"paragraph": p["paragraph"], "text": p["text"], "attrs": map[string]any{"bold": true}})
	if err != nil {
		return s, err
	}
	s.Attrs = nil
	s.Start = s.End
	return s, nil
}

func verifyDocumentMention(before, after documentSnapshot, s documentFormatSpec, name string) error {
	if s.Paragraph < 1 || s.Paragraph > len(before.Paragraphs) || s.Paragraph > len(after.Paragraphs) || name == "" {
		return fmt.Errorf("提及目标或段落无效")
	}
	p := after.Paragraphs[s.Paragraph-1]
	if p.Text != s.Text+"@"+name {
		return fmt.Errorf("提及后的正文与指定段落末尾新增本人姓名不符")
	}
	chars := documentExpanded(p)
	n := utf8.RuneCountInString(s.Text)
	if len(chars) < n {
		return fmt.Errorf("导出正文字符不完整")
	}
	structure, err := documentTrimMentionStructure(p.Structure, s.Text, "@"+name)
	if err != nil {
		return err
	}
	trimmed := documentSnapshot{Structure: after.Structure, Assets: after.Assets, StyleResources: after.StyleResources, Paragraphs: append([]documentParagraph(nil), after.Paragraphs...)}
	trimmed.Paragraphs[s.Paragraph-1] = documentParagraph{Structure: structure, Text: s.Text, Chars: chars[:n], Properties: p.Properties, MarkAttrs: p.MarkAttrs}
	return verifyDocumentFormat(before, trimmed, s)
}

// Hyperlink.add is the editor's actual body-mention insertion path. The object
// uses User=1 and atInfo.userId, which the editor persists in writeToModoc.
func documentMentionJS(s documentFormatSpec, id, name string) string {
	user, _ := json.Marshal(map[string]string{"id": id, "name": name})
	return `(async()=>{` + phase3PresenceGuardJS() + `const user=` + string(user) + `;const api=window._sdk?.editor?.docsApi;const plugin=await api?.pluginManager?.getInstanceAsync('Hyperlink');` + documentRangeJS(s) + `
 if(String(window._sdk?.options?.user?.id)!==user.id||typeof plugin?.add!=='function'||typeof plugin?.canAdd!=='function')return {ok:false,error:'本人身份或真实提及插件不可确认'};
 if(!Array.isArray(p.children)||p.children.some(c=>c.atType===1))return {ok:false,error:'目标段落已有提及或模型未知，未重复写入'};
 return await e.runSelectionCallback(range,()=>{` + phase3PresenceGuardJS() + `if(a.isViewMode!==false||e.config?.mode?.role!=='editor'||e.canHandleChanges===false||String(window._sdk?.options?.user?.id)!==user.id)return {ok:false,error:'提交前权限或本人身份变化'};if(!plugin.canAdd())return {ok:false,error:'当前选区禁止插入提及'};
 const props={Text:'@'+user.name,atType:1,atInfo:{userId:Number(user.id)},getHeading:()=>null,getAnchor:()=>'',getValue:()=>'',getToolTip:()=>'',getText:()=>'@'+user.name};plugin.add(props);
 return {ok:true,submitted:true,userId:user.id,displayText:props.Text,paragraph:spec.paragraph,notificationAPIInvoked:false};});})()`
}

func documentMentionReadbackJS(s documentFormatSpec, id, name string) string {
	raw, _ := json.Marshal(map[string]any{"paragraph": s.Paragraph, "text": s.Text + "@" + name, "userId": id, "label": "@" + name})
	// The mention node is not always a direct child of the paragraph: the editor
	// can nest it inside the hyperlink wrapper. Walk the model instead of
	// reading p.children once, and fall back to the document root when the
	// paragraph object itself exposes no children accessor.
	return `(async()=>{const spec=` + string(raw) + `,e=window._sdk?.editor,p=e?.getParagraphs?.()[spec.paragraph-1];if(!p)return {ok:false,error:'提及段落不存在'};const text=(await e.getParagraphRange(p).getText()).replace(/\r?\n$/,'');if(text!==spec.text)return {ok:false,error:'提及段落正文不符'};
const kids=n=>{if(Array.isArray(n?.children))return n.children;for(const k of ['getChildren','getChildNodes','getNodes','getChildrenNodes'])if(typeof n?.[k]==='function'){try{const v=n[k]();if(Array.isArray(v))return v}catch(_){}}return null};
const find=root=>{const out=[],seen=new Set();const walk=(n,d)=>{if(!n||typeof n!=='object'||d>40||seen.has(n))return;seen.add(n);const cs=kids(n);if(!cs)return;for(const c of cs){if(c?.atType===1)out.push(c);walk(c,d+1)}};walk(root,0);return out};
let mentions=find(p),scope='paragraph';if(mentions.length===0){const doc=typeof e.getDocument==='function'?e.getDocument():e.docsApi?.getDocument?.();mentions=find(doc);scope='document'}
if(mentions.length!==1)return {ok:false,error:'真实用户提及节点数不是1（'+mentions.length+'）'};const m=mentions[0];const collected={Text:''};const returned=typeof m.getText==='function'?m.getText(collected):null;const label=typeof returned==='string'?returned:collected.Text;if(String(m.atInfo?.userId)!==spec.userId||label!==spec.label)return {ok:false,error:'真实提及的用户ID或显示名不符'};return {ok:true,userId:String(m.atInfo.userId),atType:m.atType,displayText:label,paragraph:spec.paragraph,scope,nodeId:typeof m.getId==='function'?m.getId():null,text};})()`
}

func documentMentionIdentity(me map[string]any) (string, string, error) {
	id, name := str(me, "id"), str(me, "name")
	if name == "" {
		name = str(me, "displayName")
	}
	if id == "" || name == "" || strings.ContainsAny(name, "\r\n") {
		return "", "", fmt.Errorf("当前登录用户缺少可验证 ID 或显示名")
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return "", "", fmt.Errorf("当前用户 ID 类型不受支持")
		}
	}
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil || n == 0 || n > 9007199254740991 {
		return "", "", fmt.Errorf("当前用户 ID 不能可靠转换为编辑器整数")
	}
	return id, name, nil
}
