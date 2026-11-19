package main

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strings"
)

func phase3ChartExpectedCounts(source map[string]any, fieldID string) (map[string]int, error) {
	field := phase3TableMap(source, "fields")[fieldID]
	if field == nil {
		return nil, errors.New("图表字段不存在")
	}
	props, _ := field["props"].(map[string]any)
	choices, _ := props["choices"].(map[string]any)
	counts := map[string]int{}
	for _, r := range phase3TableMap(source, "records") {
		values := r["valueMap"].(map[string]any)
		v := values[fieldID]
		label := ""
		if v != nil {
			switch str(field, "type") {
			case "SingleLineText":
				var ok bool
				label, ok = v.(string)
				if !ok {
					return nil, errors.New("图表文本字段含异常值")
				}
			case "SingleChoice":
				id, ok := v.(string)
				choice, _ := choices[id].(map[string]any)
				if !ok || choice == nil {
					return nil, errors.New("图表单选值不是有效选项")
				}
				label = str(choice, "name")
			case "Number":
				n, ok := v.(float64)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
					return nil, errors.New("图表数值无效")
				}
				label = fmt.Sprint(n)
			default:
				return nil, errors.New("图表字段无法独立统计")
			}
		}
		counts[label]++
	}
	return counts, nil
}

func verifyPhase3ChartCounts(rendered map[string]any, expected map[string]int) error {
	categories, ok := rendered["categories"].([]any)
	if !ok {
		return errors.New("原生图表未返回分类")
	}
	values, ok := rendered["values"].([]any)
	if !ok || len(categories) != len(values) || len(categories) != len(expected) {
		return errors.New("原生图表分组数量与源记录不一致")
	}
	seen := map[string]bool{}
	blankUsed := false
	for i, raw := range categories {
		label, ok := raw.(string)
		if !ok || seen[label] {
			return errors.New("原生图表分类缺失或重复")
		}
		seen[label] = true
		count, ok := values[i].(float64)
		if !ok || math.Trunc(count) != count {
			return errors.New("原生图表计数不是整数")
		}
		want, exists := expected[label]
		if !exists && label == "(空)" && expected[""] > 0 && !blankUsed {
			want = expected[""]
			blankUsed = true
			exists = true
		}
		if !exists || count != float64(want) {
			return fmt.Errorf("原生图表分类 %q 汇总值与独立源记录计数不一致", label)
		}
	}
	for label := range expected {
		if label != "" && !seen[label] {
			return errors.New("原生图表缺少非空分类")
		}
	}
	return nil
}

func preparePhase3TableChart(payload map[string]any, baseGuid string, tables map[string]map[string]any) (phase3BaseEditPlan, error) {
	p := phase3BaseEditPlan{phase3TableEditPlan: phase3TableEditPlan{Action: "table-chart-create"}, SourceGuid: str(payload, "tableGuid"), Name: str(payload, "name")}
	if err := phase3TablePayloadKeys(payload, "tableGuid", "fieldGuid", "name", "type"); err != nil {
		return p, err
	}
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 200 {
		return p, errors.New("图表名称为空或过长")
	}
	for _, t := range tables {
		if str(t, "name") == p.Name {
			return p, errors.New("同名数据表/仪表盘已经存在")
		}
	}
	source := tables[p.SourceGuid]
	if source == nil || num(source, "tableType") == 4 {
		return p, errors.New("图表源数据表不存在")
	}
	fieldID := str(payload, "fieldGuid")
	p.FieldGuid = fieldID
	field := phase3TableMap(source, "fields")[fieldID]
	if field == nil {
		return p, errors.New("图表分组字段不存在")
	}
	switch str(field, "type") {
	case "SingleLineText", "SingleChoice", "Number":
	default:
		return p, errors.New("图表分组字段类型尚未支持")
	}
	chartType, ok := map[string]int{"Column": 1, "Line": 2, "Bar": 3, "Pie": 5}[str(payload, "type")]
	if !ok {
		return p, errors.New("图表类型仅支持 Column/Line/Bar/Pie")
	}
	if len(phase3TableRows(source, "records")) == 0 {
		return p, errors.New("源数据表没有记录，拒绝空数据图表验收")
	}
	counts, err := phase3ChartExpectedCounts(source, fieldID)
	if err != nil {
		return p, err
	}
	if len(counts) == 1 && counts[""] > 0 {
		return p, errors.New("图表分组字段全为空，拒绝空数据图表验收")
	}
	id, err := phase3TableID()
	if err != nil {
		return p, err
	}
	p.TableGuid = id[:11]
	id, err = phase3TableID()
	if err != nil {
		return p, err
	}
	componentID := id[:11]
	component := phase3TableCopy(map[string]any{"componentGuid": componentID, "name": p.Name, "grid": map[string]any{"x": 0, "y": 0, "w": 3, "h": 2}, "type": chartType, "subType": 1, "dataSource": map[string]any{"tableGuid": p.SourceGuid, "viewGuid": ""}, "axis": map[string]any{"xAxis": map[string]any{"fieldGuid": fieldID, "orderBy": 0, "order": 0}, "yAxis": map[string]any{"mode": "count", "series": nil}}, "style": map[string]any{"themeColor": "0", "legend": map[string]any{"enable": true}, "label": map[string]any{"enable": true}, "statNumber": map[string]any{"decimalPlace": 0, "formatType": 0, "description": ""}}})
	// Explicit values for fields observed in the persisted protocol. These are
	// new request parameters, never an oracle retrofitted to an earlier response.
	component["baseGuid"], component["dashboardGuid"] = baseGuid, p.TableGuid
	component["dataRange"] = map[string]any{"type": float64(0), "filter": nil}
	yAxis := component["axis"].(map[string]any)["yAxis"].(map[string]any)
	yAxis["countStyle"], yAxis["seriesStyle"] = map[string]any{"color": ""}, map[string]any{"color": ""}
	style := component["style"].(map[string]any)
	style["grid"] = false
	style["label"] = map[string]any{"enable": true, "columns": nil, "fontBold": nil, "fontFamily": nil, "fontItalic": nil, "fontSize": nil, "position": nil}
	p.Dashboard = map[string]any{"guid": p.TableGuid, "baseGuid": baseGuid, "name": p.Name, "desc": "", "tableType": float64(4), "components": []any{component},
		"positions":       []any{map[string]any{"componentGuid": componentID, "x": float64(0), "y": float64(0), "w": float64(3), "h": float64(2)}},
		"refreshInterval": float64(0), "shareConfig": map[string]any{"enable": false, "shareGuid": "", "shareMode": ""}}
	p.CreatedIDs = []string{p.TableGuid, componentID}
	return p, p.add("CreateDashboard", map[string]any{"baseGuid": baseGuid, "dashboardGuid": p.TableGuid, "name": p.Name, "desc": "", "targetGuid": p.SourceGuid, "components": []any{component}, "positions": p.Dashboard["positions"], "refreshInterval": p.Dashboard["refreshInterval"], "shareConfig": p.Dashboard["shareConfig"]})
}

func phase3DashboardComparable(d map[string]any) map[string]any {
	out := phase3TableCopy(d)
	// The server changes this timestamp for any accepted component update. All
	// other data, including unknown settings, ownership and order, is protected.
	delete(out, "updatedAt")
	return out
}

func verifyPhase3Dashboard(actual, expected map[string]any) error {
	if actual == nil {
		return errors.New("新增仪表盘未读回")
	}
	for _, key := range []string{"guid", "baseGuid", "name", "desc", "tableType", "positions", "refreshInterval", "shareConfig"} {
		if !reflect.DeepEqual(actual[key], expected[key]) {
			return fmt.Errorf("仪表盘 %s 未符合预期", key)
		}
	}
	got, ok := actual["components"].([]any)
	want, _ := expected["components"].([]any)
	if !ok || len(got) != len(want) {
		return errors.New("图表组件数量不一致")
	}
	for i, raw := range want {
		w := raw.(map[string]any)
		g, ok := got[i].(map[string]any)
		if !ok {
			return errors.New("图表组件不是对象")
		}
		if !reflect.DeepEqual(g, w) {
			return errors.New("图表完整配置与明确预期不一致；不能忽略默认或未知字段")
		}
	}
	return nil
}

func (s *phase3SDKSession) dashboardSnapshot(meta map[string]any) (map[string]any, error) {
	id, typedID := meta["guid"].(string)
	if !typedID || id == "" || meta["baseGuid"] != s.guid || meta["tableType"] != float64(4) {
		return nil, errors.New("仪表盘 base 元数据身份或类型无效")
	}
	raw, err := s.get("table-api/bases/" + url.PathEscape(s.guid) + "/dashboards/" + url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	data, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("仪表盘内容不是对象")
	}
	if code, wrapped := data["code"]; wrapped {
		if code != float64(0) {
			return nil, errors.New("仪表盘 envelope 未明确返回数值 code=0")
		}
		data, ok = data["data"].(map[string]any)
		if !ok {
			return nil, errors.New("仪表盘 envelope data 不是对象")
		}
	}
	if data["dashboardGuid"] != id || data["baseGuid"] != s.guid {
		return nil, errors.New("仪表盘 dashboardGuid/baseGuid 不匹配或缺失")
	}
	if actual, exists := data["guid"]; exists && actual != id {
		return nil, errors.New("仪表盘 guid 与 dashboardGuid 冲突")
	}
	for _, key := range []string{"name", "desc"} {
		value, typed := data[key].(string)
		if !typed || value != meta[key] {
			return nil, fmt.Errorf("仪表盘 %s 与 base 元数据不一致", key)
		}
	}
	if value, exists := data["tableType"]; exists && value != float64(4) {
		return nil, errors.New("仪表盘响应 tableType 不一致")
	}
	components, ok := data["components"].([]any)
	if !ok {
		return nil, errors.New("仪表盘缺少 components 数组")
	}
	seen := map[string]bool{}
	for _, raw := range components {
		c, ok := raw.(map[string]any)
		id, typedID := c["componentGuid"].(string)
		if !ok || !typedID || id == "" || seen[id] {
			return nil, errors.New("图表组件身份缺失/重复")
		}
		if c["baseGuid"] != s.guid || c["dashboardGuid"] != meta["guid"] {
			return nil, errors.New("图表组件所属 base/dashboard 身份不一致")
		}
		seen[id] = true
	}
	if err = phase3ValidateDashboardPositions(data, meta); err != nil {
		return nil, err
	}
	out := phase3TableCopy(meta)
	for key, value := range phase3TableCopy(data) {
		out[key] = value
	}
	out["guid"], out["baseGuid"] = id, s.guid
	return out, nil
}

func phase3ValidateDashboardPositions(data, meta map[string]any) error {
	positions, ok := data["positions"].([]any)
	components, _ := data["components"].([]any)
	if !ok || len(positions) != len(components) {
		return errors.New("仪表盘 positions 缺失或数量不符")
	}
	grids := map[string]map[string]any{}
	for _, raw := range components {
		c := raw.(map[string]any)
		grid, ok := c["grid"].(map[string]any)
		if !ok || len(grid) != 4 {
			return errors.New("图表组件 grid 结构无效")
		}
		grids[str(c, "componentGuid")] = grid
	}
	seen := map[string]bool{}
	for _, raw := range positions {
		p, ok := raw.(map[string]any)
		id, typedID := p["componentGuid"].(string)
		if !ok || !typedID || seen[id] || grids[id] == nil || len(p) != 5 {
			return errors.New("仪表盘 position 身份或结构无效")
		}
		seen[id] = true
		for _, key := range []string{"x", "y", "w", "h"} {
			v, typed := p[key].(float64)
			if !typed || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v != math.Trunc(v) || ((key == "w" || key == "h") && v == 0) || v != grids[id][key] {
				return errors.New("仪表盘 positions 与组件 grid 不一致")
			}
		}
	}
	if raw, exists := meta["positions"]; exists && raw != nil {
		basePositions, ok := raw.([]any)
		if !ok || len(basePositions) != len(grids) {
			return errors.New("base 仪表盘位置数量不一致")
		}
		seenBase := map[string]bool{}
		for _, raw := range basePositions {
			p, ok := raw.(map[string]any)
			id, typedID := p["componentGuid"].(string)
			if !ok || !typedID || seenBase[id] || grids[id] == nil || len(p) != 2 || !reflect.DeepEqual(p["grid"], grids[id]) {
				return errors.New("base 仪表盘位置与完整响应不一致")
			}
			seenBase[id] = true
		}
	}
	return nil
}
