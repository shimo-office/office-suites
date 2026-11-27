package main

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

func phase3ViewDeleteDependencies(tables map[string]map[string]any, tableGuid, viewGuid string) (map[string]map[string]any, error) {
	unchanged := map[string]map[string]any{}
	for id, table := range tables {
		for _, field := range phase3TableMap(table, "fields") {
			props, _ := field["props"].(map[string]any)
			if str(props, "tableGuid") == tableGuid && props["isRefToView"] == true && str(props, "refViewGuid") == viewGuid {
				return nil, errors.New("视图被关联字段引用，拒绝删除")
			}
		}
		if components, ok := table["components"].([]any); ok {
			for _, raw := range components {
				component, _ := raw.(map[string]any)
				source, _ := component["dataSource"].(map[string]any)
				if str(source, "tableGuid") == tableGuid && str(source, "viewGuid") == viewGuid {
					return nil, errors.New("视图被仪表盘数据源引用，拒绝删除")
				}
			}
		}
		if id != tableGuid {
			unchanged[id] = table
		}
	}
	return unchanged, nil
}

func preparePhase3TableView(p *phase3TableEditPlan, payload, before map[string]any) error {
	id := str(payload, "viewGuid")
	views := phase3TableMap(before, "views")
	view := views[id]
	if view == nil {
		return errors.New("viewGuid 不属于目标数据表")
	}
	if permission, ok := view["permission"].(map[string]any); ok && str(permission, "type") != "" && str(permission, "type") != "Shared" {
		return errors.New("个人或锁定视图不在此写动作范围")
	}
	fields := phase3TableMap(before, "fields")
	expected := p.Expected["views"].(map[string]any)
	if p.Action == "table-view-delete" {
		if err := phase3TablePayloadKeys(payload, "tableGuid", "viewGuid"); err != nil {
			return err
		}
		if len(views) < 2 {
			return errors.New("不能删除最后一个视图")
		}
		sharedRemaining := false
		for other, candidate := range views {
			permission, _ := candidate["permission"].(map[string]any)
			if other != id && (str(permission, "type") == "" || str(permission, "type") == "Shared") {
				sharedRemaining = true
			}
		}
		if !sharedRemaining {
			return errors.New("不能删除最后一个共享视图")
		}
		if id == str(before, "defaultViewGuid") {
			return errors.New("不能删除默认引用视图")
		}
		if str(view, "shareGuid") != "" {
			return errors.New("已分享视图不能通过此动作删除")
		}
		orders, err := phase3DeletedViewOrders(before, id)
		if err != nil {
			return err
		}
		delete(expected, id)
		for remaining, order := range orders {
			expected[remaining].(map[string]any)["order"] = order
		}
		p.DeletedViewIDs = []string{id}
		return p.add("DeleteView", map[string]any{"tableGuid": p.TableGuid, "views": []any{map[string]any{"viewGuid": id, "viewType": str(view, "type")}}})
	}
	if p.Action == "table-view-create" {
		return preparePhase3ViewCreate(p, payload, before, view)
	}
	if err := phase3TablePayloadKeys(payload, "tableGuid", "viewGuid", "name", "filters", "groups", "sort"); err != nil {
		return err
	}
	next := expected[id].(map[string]any)
	for _, key := range []string{"name", "filters", "groups", "sort"} {
		value, exists := payload[key]
		if !exists {
			continue
		}
		native := map[string]string{"name": "UpdateViewName", "filters": "UpdateViewFilters", "groups": "UpdateViewGroups", "sort": "UpdateViewSort"}[key]
		data := map[string]any{"tableGuid": p.TableGuid, "viewGuid": id, key: value}
		switch key {
		case "name":
			name, ok := value.(string)
			if !ok || strings.TrimSpace(name) == "" || len(name) > 200 {
				return errors.New("视图名称为空或过长")
			}
			for other, v := range views {
				if other != id && str(v, "name") == name {
					return errors.New("视图名称重复")
				}
			}
		case "filters":
			if err := phase3ValidateViewFilter(value, fields); err != nil {
				return err
			}
			if value != nil {
				filter := phase3TableCopy(value.(map[string]any))
				filter["includeSubRecord"] = false
				value = filter
			}
		case "groups":
			if err := phase3ValidateViewRules(value, fields, false); err != nil {
				return err
			}
			groups := []any{}
			for _, raw := range value.([]any) {
				group := phase3TableCopy(raw.(map[string]any))
				group["groupBy"] = nil
				groups = append(groups, group)
			}
			value = groups
		case "sort":
			config, ok := value.(map[string]any)
			if !ok {
				return errors.New("sort 必须为对象")
			}
			if err := phase3TablePayloadKeys(config, "rules", "keepSorted"); err != nil {
				return err
			}
			if _, ok := config["keepSorted"].(bool); !ok {
				return errors.New("keepSorted 必须为布尔值")
			}
			if err := phase3ValidateViewRules(config["rules"], fields, true); err != nil {
				return err
			}
			order, err := phase3SortedViewRecords(before, view, config["rules"].([]any))
			if err != nil {
				return err
			}
			p.ViewLayouts = map[string]map[string]any{id: {"records": order}}
			data["records"] = order
		}
		data[key] = value
		next[key] = value
		if err := p.add(native, data); err != nil {
			return err
		}
	}
	return nil
}

func phase3ValidateViewFilter(value any, fields map[string]map[string]any) error {
	if value == nil {
		return nil
	}
	f, ok := value.(map[string]any)
	if !ok {
		return errors.New("filters 必须为对象或 null")
	}
	if err := phase3TablePayloadKeys(f, "conjunction", "conditions"); err != nil {
		return err
	}
	if c := str(f, "conjunction"); c != "And" && c != "Or" {
		return errors.New("筛选 conjunction 仅支持 And/Or")
	}
	conditions, ok := f["conditions"].([]any)
	if !ok || len(conditions) == 0 || len(conditions) > 20 {
		return errors.New("筛选条件需含1至20项；清空使用 null")
	}
	for _, raw := range conditions {
		c, ok := raw.(map[string]any)
		if !ok {
			return errors.New("筛选条件必须为对象")
		}
		if err := phase3TablePayloadKeys(c, "fieldGuid", "operator", "value"); err != nil {
			return err
		}
		id := str(c, "fieldGuid")
		field := fields[id]
		if field == nil {
			return errors.New("筛选字段不存在")
		}
		op := str(c, "operator")
		switch op {
		case "Equal", "NotEqual", "GreaterThan", "GreaterThanEqual", "LessThan", "LessThanEqual":
		default:
			return errors.New("筛选运算符未支持")
		}
		if op != "Equal" && op != "NotEqual" && str(field, "type") != "Number" {
			return errors.New("大小比较仅支持 Number 字段")
		}
		if c["value"] == nil {
			return errors.New("筛选值不能为空")
		}
		if err := phase3TableValues(map[string]any{id: c["value"]}, fields); err != nil {
			return err
		}
	}
	return nil
}

func phase3ValidateViewRules(value any, fields map[string]map[string]any, numericOnly bool) error {
	rules, ok := value.([]any)
	if !ok || len(rules) > 3 {
		return errors.New("分组/排序规则必须为最多3项数组")
	}
	seen := map[string]bool{}
	for _, raw := range rules {
		rule, ok := raw.(map[string]any)
		if !ok {
			return errors.New("规则必须为对象")
		}
		if err := phase3TablePayloadKeys(rule, "fieldGuid", "order"); err != nil {
			return err
		}
		id := str(rule, "fieldGuid")
		field := fields[id]
		if field == nil || seen[id] {
			return errors.New("规则字段不存在或重复")
		}
		seen[id] = true
		if order := str(rule, "order"); order != "Asc" && order != "Desc" {
			return errors.New("规则 order 仅支持 Asc/Desc")
		}
		typ := str(field, "type")
		if numericOnly && typ != "Number" {
			return errors.New("精确排序目前仅支持 Number 字段")
		}
		if !numericOnly && typ != "Number" && typ != "SingleChoice" && typ != "SingleLineText" {
			return errors.New("分组字段类型未支持")
		}
	}
	return nil
}

func phase3SortedViewRecords(table, view map[string]any, rules []any) ([]any, error) {
	old, ok := view["records"].([]any)
	if !ok {
		return nil, errors.New("视图缺少记录顺序")
	}
	result := append([]any{}, old...)
	records := phase3TableMap(table, "records")
	for _, id := range result {
		row := records[fmt.Sprint(id)]
		if row == nil {
			return nil, errors.New("视图包含未知记录")
		}
		values, _ := row["valueMap"].(map[string]any)
		for _, raw := range rules {
			rule := raw.(map[string]any)
			value := values[str(rule, "fieldGuid")]
			if value == nil {
				continue
			}
			n, ok := value.(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
				return nil, errors.New("排序字段含非数值，拒绝猜测排序规则")
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		a := records[fmt.Sprint(result[i])]["valueMap"].(map[string]any)
		b := records[fmt.Sprint(result[j])]["valueMap"].(map[string]any)
		for _, raw := range rules {
			r := raw.(map[string]any)
			araw, braw := a[str(r, "fieldGuid")], b[str(r, "fieldGuid")]
			// SDK Table.applySort -> applyGroup removes its null/empty group before
			// sorting nonempty values, then appends that group for both directions.
			if araw == nil && braw == nil {
				continue
			}
			if araw == nil {
				return false
			}
			if braw == nil {
				return true
			}
			av, bv := araw.(float64), braw.(float64)
			if av == bv {
				continue
			}
			if str(r, "order") == "Desc" {
				return av > bv
			}
			return av < bv
		}
		return false
	})
	return result, nil
}

func preparePhase3ViewCreate(p *phase3TableEditPlan, payload, before, source map[string]any) error {
	if err := phase3TablePayloadKeys(payload, "tableGuid", "viewGuid", "name", "type", "groupFieldGuid", "startFieldGuid", "endFieldGuid"); err != nil {
		return err
	}
	name, ok := payload["name"].(string)
	if !ok || strings.TrimSpace(name) == "" || len(name) > 200 {
		return errors.New("视图名称为空或过长")
	}
	orders, newOrder, err := phase3InsertedViewOrders(before, str(source, "guid"))
	if err != nil {
		return err
	}
	for _, v := range phase3TableMap(before, "views") {
		if str(v, "name") == name {
			return errors.New("视图名称重复")
		}
	}
	typ := str(payload, "type")
	fields := phase3TableMap(before, "fields")
	groups := []any{}
	metaKey := ""
	var meta map[string]any
	switch typ {
	case "Grid":
		if payload["groupFieldGuid"] != nil || payload["startFieldGuid"] != nil || payload["endFieldGuid"] != nil {
			return errors.New("Grid 不接受看板/日历字段配置")
		}
	case "Kanban":
		if payload["startFieldGuid"] != nil || payload["endFieldGuid"] != nil {
			return errors.New("Kanban 不接受日期字段配置")
		}
		field := str(payload, "groupFieldGuid")
		if fields[field] == nil || str(fields[field], "type") != "SingleChoice" {
			return errors.New("Kanban 需要已有 SingleChoice 分组字段")
		}
		groups = []any{map[string]any{"fieldGuid": field, "order": "Asc", "groupBy": nil}}
		metaKey = "kanban"
		meta = map[string]any{"fieldNameVisible": true, "emptyValueVisible": false, "subRecordInvisible": false}
	case "Calendar":
		if payload["groupFieldGuid"] != nil {
			return errors.New("Calendar 不接受看板分组字段")
		}
		start, end := str(payload, "startFieldGuid"), str(payload, "endFieldGuid")
		if fields[start] == nil || fields[end] == nil || str(fields[start], "type") != "Date" || str(fields[end], "type") != "Date" {
			return errors.New("Calendar 起止字段必须是已有 Date 字段")
		}
		metaKey = "calendar"
		meta = map[string]any{"startFieldGuid": start, "endFieldGuid": end, "color": map[string]any{"type": "custom", "customColor": "#fff", "followFieldGuid": ""}, "display": map[string]any{"showLunarCalendar": true, "showHoliday": true}}
	default:
		return errors.New("视图类型仅支持 Grid/Kanban/Calendar")
	}
	id, err := phase3TableID()
	if err != nil {
		return err
	}
	id = id[:11]
	fieldOrder, ok := source["fields"].([]any)
	if !ok {
		return errors.New("源视图无字段顺序")
	}
	rows, ok := source["records"].([]any)
	if !ok {
		return errors.New("源视图无记录顺序")
	}
	fieldInfo := map[string]any{}
	for _, id := range fieldOrder {
		fieldInfo[fmt.Sprint(id)] = map[string]any{"visible": true}
	}
	for viewID, order := range orders {
		p.Expected["views"].(map[string]any)[viewID].(map[string]any)["order"] = order
	}
	p.NewViews = map[string]map[string]any{id: {"guid": id, "tableGuid": p.TableGuid, "name": name, "type": typ, "order": newOrder, "desc": "", "fields": fieldOrder, "records": rows, "filters": nil, "groups": groups, "sort": map[string]any{"rules": []any{}, "keepSorted": true}, "permission": nil}}
	p.CreatedIDs = append(p.CreatedIDs, id)
	data := map[string]any{"tableGuid": p.TableGuid, "viewGuid": id, "type": typ, "targetGuid": str(source, "guid"), "fromViewGuid": str(source, "guid"), "name": name, "desc": "", "groups": groups, "fieldInfo": fieldInfo, "colsOrder": fieldOrder, "permission": nil, "blockId": ""}
	if metaKey != "" {
		data[metaKey] = meta
		p.NewViews[id][metaKey] = meta
	}
	if err := p.add("CreateView", data); err != nil {
		return err
	}
	// CreateView does not carry sort in the native wire protocol, and server
	// defaults vary by view type. Declare it explicitly in the same submission.
	return p.add("UpdateViewSort", map[string]any{"tableGuid": p.TableGuid, "viewGuid": id, "sort": map[string]any{"rules": []any{}, "keepSorted": true}, "records": rows})
}

func verifyPhase3NewViews(after map[string]any, p phase3TableEditPlan) error {
	actual := phase3TableMap(after, "views")
	for id, want := range p.NewViews {
		view := actual[id]
		if view == nil {
			return errors.New("新增视图未读回")
		}
		for key, value := range want {
			// Native getGroup() ?? [] represents absent/null grouping as no groups.
			if key == "groups" && view[key] == nil {
				if groups, ok := value.([]any); ok && len(groups) == 0 {
					continue
				}
			}
			if !reflect.DeepEqual(view[key], value) {
				return fmt.Errorf("新视图 %s 的 %s 不符合预期", id, key)
			}
		}
		info, _ := view["fieldInfo"].(map[string]any)
		for _, field := range want["fields"].([]any) {
			attrs, _ := info[fmt.Sprint(field)].(map[string]any)
			if attrs["visible"] != true {
				return errors.New("新增视图字段未显示")
			}
		}
	}
	return nil
}
