package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"
)

func phase3ChartProofSpec(plan phase3BaseEditPlan) map[string]any {
	c := plan.Dashboard["components"].([]any)[0].(map[string]any)
	axis := c["axis"].(map[string]any)["xAxis"].(map[string]any)
	typ := num(c, "type")
	series, dimension := "bar", "x"
	if typ == 2 {
		series = "line"
	}
	if typ == 3 {
		dimension = "y"
	}
	if typ == 5 {
		series, dimension = "pie", "pie"
	}
	style := c["style"].(map[string]any)
	return map[string]any{"themeColor": str(style, "themeColor"), "dashboardGuid": plan.TableGuid, "componentGuid": c["componentGuid"], "chartType": typ, "tableGuid": plan.SourceGuid, "fieldGuid": axis["fieldGuid"], "nativeSeriesType": series, "axisDimension": dimension}
}

func verifyPhase3ChartIdentity(actual, spec map[string]any, previousOrigin float64) error {
	for _, key := range []string{"dashboardGuid", "componentGuid", "chartType", "tableGuid", "fieldGuid", "nativeSeriesType", "axisDimension"} {
		if !reflect.DeepEqual(actual[key], spec[key]) {
			return fmt.Errorf("原生图表身份或类型不符：%s", key)
		}
	}
	if want, exists := spec["themeColor"]; exists {
		if actual["themeColor"] != want {
			return errors.New("图表渲染绑定的调色板配置不符")
		}
		palette, ok := actual["palette"].([]any)
		if !ok || len(palette) != 6 {
			return errors.New("原生图表调色板未完整读回")
		}
		preset, ok := want.(string)
		expectedColors := phase3ChartPalettes[preset]
		if !ok || len(expectedColors) != len(palette) {
			return errors.New("未知原生图表调色板")
		}
		for i, value := range palette {
			if color, ok := value.(string); !ok || !strings.EqualFold(color, expectedColors[i]) {
				return errors.New("原生图表颜色不符合所选官方调色板")
			}
		}
	}
	origin, ok := actual["timeOrigin"].(float64)
	if !ok || origin <= previousOrigin {
		return errors.New("图表不是本次导航后的新文档，拒绝旧DOM证据")
	}
	return nil
}

// Read only the native render option and its enclosing component's explicit
// identity. A new performance.timeOrigin binds proof to this navigation.
func (g *phase3TablePresence) ChartProof(spec map[string]any, expected map[string]int) (any, error) {
	previous, err := g.browser.EvalReadOnly(`performance.timeOrigin`)
	if err != nil {
		return nil, err
	}
	previousOrigin, ok := previous.(float64)
	if !ok {
		return nil, errors.New("无法读取当前文档generation")
	}
	u, err := url.Parse(g.target)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("table", str(spec, "dashboardGuid"))
	q.Del("view")
	u.RawQuery = q.Encode()
	if _, err = g.browser.ws.Call(701, "Page.navigate", map[string]any{"url": u.String()}, 15*time.Second); err != nil {
		return nil, err
	}
	g.target = u.String()
	specJSON, _ := json.Marshal(spec)
	originJSON, _ := json.Marshal(previousOrigin)
	readiness := `document.readyState==="complete"&&performance.timeOrigin>` + string(originJSON) + `&&new URLSearchParams(location.search).get('table')===` + jsonString(str(spec, "dashboardGuid")) + `&&Boolean(window.__shimoPhase3Presence?.().ready&&window.__shimoPhase3Presence?.().connected)&&!!document.querySelector('[_echarts_instance_]')`
	if err = g.browser.WaitReady(readiness, phase3EditorReadyTimeout); err != nil {
		return nil, err
	}
	if err = g.CheckPresence(); err != nil {
		return nil, err
	}
	v, err := g.browser.EvalReadOnly(`(()=>{const spec=` + string(specJSON) + `;const elements=[...document.querySelectorAll('[_echarts_instance_]')];if(elements.length!==1)return {ok:false,error:"仪表盘未渲染唯一图表"};const el=elements[0],key=Object.keys(el).find(k=>k.startsWith('__reactFiber$'));let fiber=key?el[key]:null,option=null,binding=null;for(let i=0;fiber&&i<60;i++,fiber=fiber.return){const props=fiber.memoizedProps;if(!option&&props?.option?.series)option=props.option;if(props?.config&&props?.dashboard&&typeof props.dashboard.getGuid==='function'){const c=props.config;binding={dashboardGuid:props.dashboard.getGuid(),componentGuid:c.componentGuid,chartType:c.type,tableGuid:c.tableGuid,fieldGuid:c.fieldGuid,themeColor:String(c.themeColor)};if(props.id!==c.componentGuid)return {ok:false,error:"原生图表组件ID不一致"};break}}if(!option||!binding)return {ok:false,error:"未找到绑定身份的原生图表"};for(const key of ['dashboardGuid','componentGuid','chartType','tableGuid','fieldGuid'])if(binding[key]!==spec[key])return {ok:false,error:"原生图表身份不匹配: "+key};const series=Array.isArray(option.series)?option.series:[option.series];if(series.length!==1||!Array.isArray(series[0].data))return {ok:false,error:"图表series数据异常"};const s=series[0];let categories,values,axisDimension;if(s.type==='pie'){categories=s.data.map(x=>String(x.name));values=s.data.map(x=>x.rawValue??x.value);axisDimension='pie'}else{const x=[option.xAxis].flat().filter(Boolean).find(x=>x.type==='category'),y=[option.yAxis].flat().filter(Boolean).find(x=>x.type==='category'),axis=x||y;if(!axis)return {ok:false,error:"图表分类轴缺失"};axisDimension=x?'x':'y';categories=axis.data;values=s.data.map(x=>typeof x==='object'?x.value:x)}return {...binding,palette:option.color,nativeSeriesType:s.type,axisDimension,timeOrigin:performance.timeOrigin,categories,values,source:'native-rendered-option'}})()`)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("图表汇总读回格式无效")
	}
	if err = verifyPhase3ChartIdentity(m, spec, previousOrigin); err != nil {
		return nil, err
	}
	if err = verifyPhase3ChartCounts(m, expected); err != nil {
		return nil, err
	}
	g.readiness = `document.readyState==="complete"&&Boolean(window.__shimoPhase3Presence?.().ready&&window.__shimoPhase3Presence?.().connected)&&[...document.querySelectorAll('[_echarts_instance_]')].some(e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(e).visibility!=="hidden"})`
	return m, nil
}

func jsonString(value string) string { raw, _ := json.Marshal(value); return string(raw) }
