package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"time"
)

func verifyPhase3NativeViewProof(actual, spec map[string]any, previousOrigin float64) error {
	for _, key := range []string{"tableGuid", "viewGuid", "viewType"} {
		if !reflect.DeepEqual(actual[key], spec[key]) {
			return fmt.Errorf("原生视图绑定不符：%s", key)
		}
	}
	if str(spec, "viewType") == "Calendar" {
		for _, key := range []string{"documentTimezone", "browserTimezone"} {
			if str(spec, key) == "" || actual[key] != spec[key] {
				return fmt.Errorf("日历实际时区绑定不符：%s", key)
			}
		}
	}
	origin, ok := actual["timeOrigin"].(float64)
	if !ok || origin <= previousOrigin {
		return errors.New("视图证据不是本次导航的新文档")
	}
	nodes, ok := actual["visibleNativeNodes"].(float64)
	if !ok || nodes < 1 {
		return errors.New("目标视图没有可见原生内容")
	}
	if expected, exists := spec["expectedRecords"].(map[string]any); exists {
		rendered, ok := actual["renderedRecords"].(map[string]any)
		if !ok || len(rendered) == 0 && len(expected) > 0 {
			return errors.New("目标视图没有渲染任何记录")
		}
		if len(expected) == 0 {
			ids, ok := actual["nativeRecordGuids"].([]any)
			if !ok {
				return errors.New("空视图缺少原生筛选后记录集合证明")
			}
			excluded, _ := spec["excludedWhollyEmptyRecordGuids"].(map[string]any)
			seen := map[string]bool{}
			for _, raw := range ids {
				id, ok := raw.(string)
				if !ok || id == "" || seen[id] || excluded[id] != true {
					return errors.New("空视图原生模型仍含未预期的记录")
				}
				seen[id] = true
			}
		}
		// Grid/Kanban virtualise long tables: only the mounted rows exist in the
		// DOM. Demand that every rendered record is a real view record with the
		// exact expected title/group/date, and record how much of the set was
		// actually visible instead of failing on the rows that were never mounted.
		for id, value := range rendered {
			want, ok := expected[id]
			if !ok {
				return fmt.Errorf("视图渲染了不属于该视图的记录 %s", id)
			}
			if !reflect.DeepEqual(value, want) {
				return fmt.Errorf("记录 %s 的可见标题、看板分组或日历日期与独立API预期不一致", id)
			}
		}
		actual["verifiedRecords"] = float64(len(rendered))
		actual["expectedRecordCount"] = float64(len(expected))
		actual["virtualized"] = len(rendered) < len(expected)
		actual["verificationScope"] = "every visible record matched; unmounted rows were not asserted"
		if len(expected) == 0 {
			actual["verificationScope"] = "bound visible native view has no populated filtered records; wholly empty initial rows explicitly excluded"
		}
	}
	return nil
}

// Native view components pass their own viewModel to mounted React children.
// Read that binding without mutating application state or using a guessed global.
func (g *phase3TablePresence) ViewProof(spec map[string]any) (any, error) {
	selector, ok := map[string]string{"Grid": ".sm-grid-view-container .header-field[data-field-guid]", "Kanban": ".sm-kanban-item-name", "Calendar": ".calendar-task-item[data-task-guid] .task-name"}[str(spec, "viewType")]
	if !ok {
		return nil, errors.New("视图渲染证明目前仅支持 Grid/Kanban/Calendar")
	}
	expected, hasExpected := spec["expectedRecords"].(map[string]any)
	emptyExpected := hasExpected && len(expected) == 0
	if emptyExpected {
		selector = "#table-editor"
	}
	if str(spec, "viewType") == "Calendar" {
		// A taller real viewport exposes overlapping month-view events that the
		// native calendar otherwise collapses into '+N records' links.
		if _, err := g.browser.ws.Call(702, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 1440, "height": 1400, "deviceScaleFactor": 1, "mobile": false}, 10*time.Second); err != nil {
			return nil, err
		}
	}
	previous, err := g.browser.EvalReadOnly(`performance.timeOrigin`)
	if err != nil {
		return nil, err
	}
	previousOrigin, ok := previous.(float64)
	if !ok {
		return nil, errors.New("无法读取视图导航generation")
	}
	u, err := url.Parse(g.target)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("table", str(spec, "tableGuid"))
	q.Set("view", str(spec, "viewGuid"))
	u.RawQuery = q.Encode()
	if _, err = g.browser.ws.Call(703, "Page.navigate", map[string]any{"url": u.String()}, 15*time.Second); err != nil {
		return nil, err
	}
	g.target = u.String()
	specJSON, _ := json.Marshal(spec)
	selectorJSON, _ := json.Marshal(selector)
	originJSON, _ := json.Marshal(previousOrigin)
	mounted := `(()=>{const visible=e=>{const r=e.getBoundingClientRect(),s=getComputedStyle(e);return r.width>0&&r.height>0&&s.visibility!=="hidden"&&s.display!=="none"};return [...document.querySelectorAll(` + string(selectorJSON) + `)].some(visible)})()`
	identity := `new URLSearchParams(location.search).get('table')===` + jsonString(str(spec, "tableGuid")) + `&&new URLSearchParams(location.search).get('view')===` + jsonString(str(spec, "viewGuid"))
	stableReadiness := `document.readyState==="complete"&&` + identity + `&&Boolean(window.__shimoPhase3Presence?.().ready&&window.__shimoPhase3Presence?.().connected)&&` + mounted
	if emptyExpected {
		// The shared #table-editor shell appears before the selected renderer
		// binds its model. Empty views have no record cells to signal readiness.
		stableReadiness += `&&(()=>{const proof=(` + phase3ReadNativeView + `)(` + string(specJSON) + `,` + string(selectorJSON) + `);return proof.viewGuid===` + jsonString(str(spec, "viewGuid")) + `&&Array.isArray(proof.nativeRecordGuids)})()`
	}
	readiness := `performance.timeOrigin>` + string(originJSON) + `&&` + stableReadiness
	if err = g.browser.WaitReady(readiness, phase3EditorReadyTimeout); err != nil {
		return nil, err
	}
	if err = g.CheckPresence(); err != nil {
		return nil, err
	}
	if str(spec, "viewType") == "Grid" && !emptyExpected {
		// A freshly created view paints its header before any row. Wait for the
		// first mounted record cell, and nudge the grid once so row
		// virtualisation mounts data instead of reporting an empty view.
		if waitErr := g.browser.WaitReady(phase3GridCellReady, 20*time.Second); waitErr != nil {
			if p, e := g.browser.EvalReadOnly(`(()=>{const c=document.querySelector('.sm-grid-view-container');if(!c)return null;const r=c.getBoundingClientRect();return{x:r.left+Math.min(r.width/2,240),y:r.top+Math.min(r.height/2,200)}})()`); e == nil {
				if point, ok := p.(map[string]any); ok {
					_, _ = g.browser.ws.Call(778, "Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": num(point, "x"), "y": num(point, "y"), "deltaX": 0, "deltaY": 320}, 10*time.Second)
				}
			}
			if e := g.browser.WaitReady(phase3GridCellReady, 15*time.Second); e != nil {
				return nil, fmt.Errorf("目标 Grid 视图在 35 秒内没有渲染任何记录单元格：%w", e)
			}
		}
	}
	v, err := g.browser.EvalReadOnly(`(` + phase3ReadNativeView + `)(` + string(specJSON) + `,` + string(selectorJSON) + `)`)
	if err != nil {
		return nil, err
	}
	actual, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("原生视图证明格式无效")
	}
	if err = verifyPhase3NativeViewProof(actual, spec, previousOrigin); err != nil {
		rendered, _ := actual["renderedRecords"].(map[string]any)
		return actual, fmt.Errorf("%w（可见节点=%v，已渲染记录=%d，主字段=%s，视图=%s）", err, actual["visibleNativeNodes"], len(rendered), str(spec, "primaryFieldGuid"), str(spec, "viewGuid"))
	}
	g.readiness = stableReadiness
	return actual, nil
}

// Fixed read-only native render extraction. Calendar task fields and record IDs
// are taken from the actual mounted task component, not synthesized from spec.
const phase3ReadNativeView = `(spec,selector)=>{
 const visible=e=>{
  if(!e)return false;
  const r=e.getBoundingClientRect();let left=Math.max(0,r.left),right=Math.min(innerWidth,r.right),top=Math.max(0,r.top),bottom=Math.min(innerHeight,r.bottom);
  for(let p=e;p;p=p.parentElement){const s=getComputedStyle(p);if(s.visibility==='hidden'||s.display==='none')return false;const box=p.getBoundingClientRect();if(/hidden|auto|scroll/.test(s.overflowX)){left=Math.max(left,box.left);right=Math.min(right,box.right)}if(/hidden|auto|scroll/.test(s.overflowY)){top=Math.max(top,box.top);bottom=Math.min(bottom,box.bottom)}}
  return r.width>0&&r.height>0&&right>left&&bottom>top;
 };
 const fiberNodes=el=>{const out=[];for(let p=el;p;p=p.parentElement){const key=Object.keys(p).find(k=>k.startsWith('__reactFiber$'));if(!key)continue;let f=p[key];for(let i=0;f&&i<100;i++,f=f.return)out.push(f);break}return out};const fibers=el=>fiberNodes(el).map(f=>f.memoizedProps);
 const nodes=[...document.querySelectorAll(selector)].filter(visible),emptyExpected=!!spec.expectedRecords&&Object.keys(spec.expectedRecords).length===0;let binding=null,boundContainer=null,boundView=null;
 for(const el of emptyExpected?[]:nodes.slice(0,100)){
  for(const props of fibers(el)){const view=props?.viewModel;if(view&&typeof view.getGuid==='function'&&typeof view.getTable==='function'&&typeof view.getType==='function'){
   const candidate={tableGuid:view.getTable().getGuid(),viewGuid:view.getGuid(),viewType:view.getType()};
   if(candidate.tableGuid===spec.tableGuid&&candidate.viewGuid===spec.viewGuid&&candidate.viewType===spec.viewType){if(spec.viewType==='Grid'){boundContainer=el.closest('.sm-grid-view-container');if(!boundContainer)continue}binding=candidate;boundView=view;break}
  }}if(binding)break;
 }
 if(!binding&&(spec.viewType==='Grid'||emptyExpected)){
  for(const el of nodes){const container=emptyExpected?el:el.closest('.sm-grid-view-container');if(!container)continue;
   for(const fiber of fiberNodes(el)){if(typeof fiber.memoizedProps?.resolveTableInst!=='function')continue;
    for(const editor of Object.values(fiber.stateNode||{})){if(typeof editor?.getActiveRenderView!=='function'||typeof editor?.getActiveView!=='function')continue;
     const render=editor.getActiveRenderView(),view=render?.getViewModel?.();
     if(render?.container!==container||!container.contains(el)||view!==editor.getActiveView()||typeof view?.getGuid!=='function'||typeof view?.getType!=='function'||typeof view?.getTable!=='function')continue;
     const candidate={tableGuid:view.getTable().getGuid(),viewGuid:view.getGuid(),viewType:view.getType()};
     if(candidate.tableGuid===spec.tableGuid&&candidate.viewGuid===spec.viewGuid&&candidate.viewType===spec.viewType){boundContainer=container;boundView=view;binding=candidate;break}
    }if(binding)break
   }if(binding)break
  }
 }
 if(!binding)return {ok:false,error:'可见原生DOM未绑定目标视图模型'};
 let nativeRecordGuids;
 if(emptyExpected){const records=boundView?.getRenderRecords?.(true,false);if(!Array.isArray(records?.recordGuids))return {ok:false,error:'原生视图未提供筛选后记录集合'};nativeRecordGuids=records.recordGuids;}
 const recordSelector={Kanban:'.sm-kanban-item-name',Calendar:'.calendar-task-item[data-task-guid] .task-name'}[spec.viewType],recordNodes=emptyExpected&&recordSelector?[...document.querySelectorAll(recordSelector)].filter(visible):nodes;
 const renderedRecords={};
 const add=(id,value)=>{if(!id)throw Error('原生记录GUID缺失');if(renderedRecords[id]&&JSON.stringify(renderedRecords[id])!==JSON.stringify(value))throw Error('同一记录的渲染副本数据不一致');renderedRecords[id]=value};
 if(spec.viewType==='Grid'){
  for(const el of document.querySelectorAll('.sm-grid-view-container .cell[data-record-guid][data-field-guid]')){
   if((!emptyExpected&&el.closest('.sm-grid-view-container')!==boundContainer)||!boundContainer?.contains(el)||el.getAttribute('data-field-guid')!==spec.primaryFieldGuid||!visible(el))continue;
   const title=(el.innerText||'').trim();if(title)add(el.getAttribute('data-record-guid'),{title});
  }
 }else if(spec.viewType==='Kanban'){
  for(const el of recordNodes){const card=el.closest('[data-record-guid]'),id=card?.getAttribute('data-record-guid');const record=fibers(el).map(p=>p?.record).find(r=>r&&r.guid===id&&r.valueMap);
   if(!record)throw Error('可见看板卡片未绑定原生记录');const primary=record.valueMap[spec.primaryFieldGuid];if(primary==null||primary==='')continue;
   if(typeof primary!=='string')throw Error('看板主字段不是文本');
   const lane=fibers(el).find(p=>p?.horizontalGroup&&p?.horizontalField&&Array.isArray(p.recordGuids));
   if(!lane||!lane.recordGuids.includes(id)||typeof lane.horizontalField.getGuid!=='function'||lane.viewModel?.getGuid?.()!==binding.viewGuid||lane.viewModel?.getTable?.()?.getGuid?.()!==binding.tableGuid||Object.keys(lane.verticalGroups||{}).length!==0)throw Error('卡片没有绑定唯一真实单层看板分组');
   const groupFieldGuid=lane.horizontalField.getGuid(),groupValue=lane.horizontalGroup.value;
   if(typeof groupFieldGuid!=='string'||typeof groupValue!=='string'||lane.horizontalGroup.type&&lane.horizontalGroup.type!=='unassigned')throw Error('原生看板分组值不受支持');
   add(id,{title:(el.innerText||'').trim(),groupFieldGuid,groupValue});
  }
 }else{
  const day=d=>{if(!(d instanceof Date)||!Number.isFinite(d.getTime()))throw Error('原生日历事件日期无效');return String(d.getFullYear()).padStart(4,'0')+'-'+String(d.getMonth()+1).padStart(2,'0')+'-'+String(d.getDate()).padStart(2,'0')};
  for(const el of recordNodes){const card=el.closest('.calendar-task-item[data-task-guid]'),id=card?.getAttribute('data-task-guid');const task=fibers(el).map(p=>p?.task).find(t=>t?.guid===id&&t.record);
   if(!task||task.record.getGuid()!==id||task.invalid||!fibers(el).some(p=>p?.viewModel===boundView))throw Error('原生日历事件记录绑定无效');const title=(el.innerText||'').trim();
   if(task.title!==title)throw Error('原生日历事件可见名称不一致');
   add(id,{title,startOA:task.record.getValue(spec.startFieldGuid),endOA:task.record.getValue(spec.endFieldGuid),startDay:day(task.originStartDate),endDay:day(task.originEndDate)});
  }
 }
 const calendarContext=spec.viewType==='Calendar'?{documentTimezone:boundView?.getTable?.()?.getBase?.()?.getDocumentTimezone?.(),browserTimezone:Intl.DateTimeFormat().resolvedOptions().timeZone}:{};
 return {...binding,...calendarContext,...(emptyExpected?{nativeRecordGuids}:{}),visibleNativeNodes:nodes.length,renderedRecords,timeOrigin:performance.timeOrigin,source:'native-view-model-and-visible-records'};
}`

// phase3GridCellReady waits for at least one mounted, visible record cell; a
// grid can be "ready" with headers only while rows are still virtualised out.
const phase3GridCellReady = `(()=>{const visible=e=>{if(!e)return false;const r=e.getBoundingClientRect(),s=getComputedStyle(e);return r.width>0&&r.height>0&&s.visibility!=="hidden"&&s.display!=="none"};return [...document.querySelectorAll(".sm-grid-view-container .cell[data-record-guid][data-field-guid]")].some(visible)})()`
