package main

import (
	"errors"
	"fmt"
	"time"
)

// Read the initialized native field menu, whose offered types already apply
// both SDK deployment options and the current table's FieldOptions. Defaults
// from a downloaded bundle are not evidence of this instance's capability.
const phase3ReferenceCapabilityRead = `(()=>{
 const visible=e=>{const r=e.getBoundingClientRect(),s=getComputedStyle(e);return r.width>0&&r.height>0&&s.visibility!=='hidden'&&s.display!=='none'};
 const nodes=[...document.querySelectorAll('[data-field-type]')].filter(visible);
 if(!nodes.length)throw Error('native field menu is not initialized');
 let oi=null;
 for(const el of nodes){
  const key=Object.keys(el).find(k=>k.startsWith('__reactFiber$'));let f=el[key],bound=null;
  for(let i=0;f&&i<100;i++,f=f.return){const candidate=f.memoizedProps?.oi;if(typeof candidate?.Ea?.getConfig==='function'&&typeof candidate.getActiveTable==='function'){bound=candidate;break}}
  if(!bound||oi&&oi!==bound)throw Error('native field menu has missing or mixed table binding');oi=bound;
 }
 const config=oi.Ea.getConfig('FieldOptions'),types=nodes.map(e=>e.getAttribute('data-field-type')).filter(Boolean);
 return {tableGuid:oi.getActiveTable().getGuid(),fieldType:'ReferenceRecord',menuInitialized:true,fieldOptionsConfigPresent:config!=null,fieldOptionsDisableRefField:typeof config?.disableRefField==='boolean'?config.disableRefField:null,renderedFieldTypes:types,referenceTypeOffered:types.includes('ReferenceRecord')};
})()`

// The hover menu can disappear during an asynchronous repaint. Retry only
// its missing visible nodes; invalid binding and unknown capability still fail.
const phase3ReferenceCapabilityReadyRead = `(async()=>{for(let attempt=0;attempt<=25;attempt++){try{return ` + phase3ReferenceCapabilityRead + `}catch(error){if(error.message!=='native field menu is not initialized'||attempt===25)throw error;await new Promise(resolve=>setTimeout(resolve,200))}}})()`

func verifyPhase3ReferenceCapability(v map[string]any, tableGuid string) error {
	if tableGuid == "" || str(v, "tableGuid") != tableGuid || str(v, "fieldType") != "ReferenceRecord" || v["menuInitialized"] != true {
		return errors.New("无法验证当前目标表的原生关联字段能力；未执行写入")
	}
	offered, ok := v["referenceTypeOffered"].(bool)
	if !ok {
		return errors.New("原生关联字段能力未知；未执行写入")
	}
	types, ok := v["renderedFieldTypes"].([]any)
	if !ok || len(types) == 0 {
		return errors.New("原生字段菜单列表缺失；未执行写入")
	}
	found := false
	for _, raw := range types {
		s, ok := raw.(string)
		if !ok || s == "" {
			return errors.New("原生字段菜单列表无效；未执行写入")
		}
		if s == "ReferenceRecord" {
			found = true
		}
	}
	if found != offered {
		return errors.New("原生关联字段菜单证据不一致；未执行写入")
	}
	if !offered || v["fieldOptionsDisableRefField"] == true {
		return errors.New("当前目标表的原生菜单未提供可用关联字段；未执行写入（不依据 HTTP 状态推断部署能力）")
	}
	return nil
}

func (g *phase3TablePresence) ReferenceCapability(tableGuid string) (map[string]any, error) {
	if err := g.browser.WaitReady(phase3TableGridReady, phase3EditorReadyTimeout); err != nil {
		return nil, err
	}
	if err := g.CheckPresence(); err != nil {
		return nil, err
	}
	if _, err := g.browser.Eval(`(()=>{if(document.querySelector('input[name="fieldName"]'))throw Error('existing field dialog prevents capability inspection');const matches=[...document.querySelectorAll('button')].filter(e=>e.innerText.trim()==='字段管理'&&e.getBoundingClientRect().width>0);if(matches.length!==1)throw Error('field manager not unique');matches[0].click();return true})()`); err != nil {
		return nil, err
	}
	if err := g.browser.WaitReady(`(()=>{return [...document.querySelectorAll('div,span')].some(e=>e.childElementCount===0&&e.innerText.trim()==='添加字段'&&e.getBoundingClientRect().width>0)})()`, 10*time.Second); err != nil {
		return nil, err
	}
	if _, err := g.browser.Eval(`(()=>{const matches=[...document.querySelectorAll('div,span')].filter(e=>e.childElementCount===0&&e.innerText.trim()==='添加字段'&&e.getBoundingClientRect().width>0);if(matches.length!==1)throw Error('add field not unique');matches[0].click();return true})()`); err != nil {
		return nil, err
	}
	if err := g.browser.WaitReady(`document.querySelector('input[name="fieldName"]')!==null`, 10*time.Second); err != nil {
		return nil, err
	}
	p, err := g.browser.EvalReadOnly(`(()=>{const menus=[...document.querySelectorAll('.field-type-menu')].filter(e=>e.getBoundingClientRect().width>0);if(menus.length!==1)throw Error('native field type menu not unique');const r=menus[0].getBoundingClientRect();return{x:r.left+r.width/2,y:r.top+r.height/2}})()`)
	if err != nil {
		return nil, err
	}
	point, ok := p.(map[string]any)
	if !ok {
		return nil, errors.New("原生字段类型菜单位置未知")
	}
	if _, err = g.browser.ws.Call(778, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": point["x"], "y": point["y"]}, 10*time.Second); err != nil {
		return nil, err
	}
	if err = g.browser.WaitReady(`(()=>{return [...document.querySelectorAll('[data-field-type]')].some(e=>e.getBoundingClientRect().width>0)})()`, 10*time.Second); err != nil {
		return nil, err
	}
	raw, err := g.browser.EvalReadOnly(phase3ReferenceCapabilityReadyRead)
	if err != nil {
		return nil, err
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("原生字段能力读取结果无效")
	}
	// Cancel the unsaved dialog; never select a field type or invoke its save.
	if _, err = g.browser.Eval(`(()=>{const matches=[...document.querySelectorAll('button')].filter(e=>e.innerText.trim()==='取消'&&e.getBoundingClientRect().width>0);if(matches.length!==1)throw Error('field dialog cancel not unique');matches[0].click();return true})()`); err != nil {
		return value, fmt.Errorf("取消只读字段检查对话框失败：%w", err)
	}
	if err = g.browser.WaitReady(`document.querySelector('input[name="fieldName"]')===null`, 10*time.Second); err != nil {
		return value, err
	}
	return value, nil
}
