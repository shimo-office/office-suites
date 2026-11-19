package main

import (
	"errors"
	"math"
	"reflect"
	"time"
)

// Prove the complete small set of named fixture records. Blank initial rows are
// explicitly counted outside this scope; large/virtualized sets fail closed.
func phase3ViewRecordSpec(table, view map[string]any, timezone string, browserTimezone ...string) (map[string]any, error) {
	primary := ""
	for id, field := range phase3TableMap(table, "fields") {
		if boolv(field, "isPrimary") {
			if str(field, "type") != "SingleLineText" {
				return nil, errors.New("视图展示验收要求文本主字段")
			}
			primary = id
		}
	}
	if primary == "" {
		return nil, errors.New("视图缺少文本主字段")
	}
	spec := map[string]any{"tableGuid": str(table, "guid"), "viewGuid": str(view, "guid"), "viewType": str(view, "type"), "primaryFieldGuid": primary, "recordScope": "all populated records matching saved filters; wholly empty initial rows explicitly excluded"}
	groupID := ""
	var groupChoices map[string]any
	if str(view, "type") == "Kanban" {
		groups, ok := view["groups"].([]any)
		if !ok || len(groups) != 1 {
			return nil, errors.New("看板真实分组验收仅支持一个单选分组字段")
		}
		group, ok := groups[0].(map[string]any)
		if !ok {
			return nil, errors.New("看板分组结构无效")
		}
		groupID = str(group, "fieldGuid")
		field := phase3TableMap(table, "fields")[groupID]
		if field == nil || str(field, "type") != "SingleChoice" {
			return nil, errors.New("看板真实分组验收要求单选字段")
		}
		props, _ := field["props"].(map[string]any)
		groupChoices, _ = props["choices"].(map[string]any)
		if groupChoices == nil {
			return nil, errors.New("看板单选分组选项缺失")
		}
	}
	var zone *time.Location
	var err error
	if str(view, "type") == "Calendar" {
		browser := ""
		if len(browserTimezone) == 1 {
			browser = browserTimezone[0]
		}
		var context map[string]any
		zone, context, err = phase3CalendarDisplayZone(timezone, browser)
		if err != nil {
			return nil, errors.New("无法验证日历的文档时区")
		}
		for key, value := range context {
			spec[key] = value
		}
	}
	calendar, _ := view["calendar"].(map[string]any)
	startID, endID := str(calendar, "startFieldGuid"), str(calendar, "endFieldGuid")
	if zone != nil {
		spec["startFieldGuid"], spec["endFieldGuid"] = startID, endID
	}
	expected := map[string]any{}
	excluded := 0
	for id, row := range phase3TableMap(table, "records") {
		values, _ := row["valueMap"].(map[string]any)
		title, ok := values[primary].(string)
		if !ok || title == "" {
			for _, value := range values {
				if value == nil {
					continue
				}
				switch typed := value.(type) {
				case string:
					if typed == "" {
						continue
					}
				case []any:
					if len(typed) == 0 {
						continue
					}
				case map[string]any:
					if len(typed) == 0 {
						continue
					}
				}
				return nil, errors.New("存在主字段为空但其他字段有数据的记录，无法可靠展示定位，拒绝排除")
			}
			excluded++
			continue
		}
		matches, e := phase3ViewFilterMatches(values, view["filters"])
		if e != nil {
			return nil, e
		}
		if !matches {
			continue
		}
		record := map[string]any{"title": title}
		if groupID != "" {
			value := ""
			if raw := values[groupID]; raw != nil {
				var ok bool
				value, ok = raw.(string)
				if !ok || value != "" && groupChoices[value] == nil {
					return nil, errors.New("看板记录的分组值不是有效单选项")
				}
			}
			record["groupFieldGuid"], record["groupValue"] = groupID, value
		}
		if zone != nil {
			start, sok := values[startID].(float64)
			end, eok := values[endID].(float64)
			if !sok || !eok || end < start || math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) {
				return nil, errors.New("命中日历记录缺少有效起止日期，无法证明完整事件集合")
			}
			day := func(v float64) string {
				// Native Kk uses JavaScript TimeClip (truncate toward zero), then
				// advances a 999ms value by 1ms to correct OA floating error.
				instant := time.UnixMilli(int64((v - 25569) * 86400000))
				if instant.Nanosecond()/int(time.Millisecond) == 999 {
					instant = instant.Add(time.Millisecond)
				}
				return instant.In(zone).Format("2006-01-02")
			}
			record["startOA"], record["endOA"], record["startDay"], record["endDay"] = start, end, day(start), day(end)
		}
		expected[id] = record
	}
	if len(expected) == 0 || len(expected) > 20 {
		return nil, errors.New("视图完整展示验收要求1至20条非空文本主字段记录，拒绝空数据或未证明的虚拟化集合")
	}
	spec["expectedRecords"], spec["excludedWhollyEmptyRecords"] = expected, excluded
	return spec, nil
}

func phase3ViewFilterMatches(values map[string]any, raw any) (bool, error) {
	if raw == nil {
		return true, nil
	}
	filter, ok := raw.(map[string]any)
	if !ok {
		return false, errors.New("视图filter格式无效")
	}
	conjunction := str(filter, "conjunction")
	if conjunction != "And" && conjunction != "Or" {
		return false, errors.New("视图filter逻辑未支持")
	}
	conditions, ok := filter["conditions"].([]any)
	if !ok {
		return false, errors.New("视图filter条件无效")
	}
	result := conjunction == "And"
	for _, rawCondition := range conditions {
		condition, ok := rawCondition.(map[string]any)
		if !ok {
			return false, errors.New("视图filter条件无效")
		}
		actual, want := values[str(condition, "fieldGuid")], condition["value"]
		matched := false
		switch str(condition, "operator") {
		case "Equal":
			matched = reflect.DeepEqual(actual, want)
		case "NotEqual":
			matched = !reflect.DeepEqual(actual, want)
		case "GreaterThan", "GreaterThanEqual", "LessThan", "LessThanEqual":
			a, aok := actual.(float64)
			b, bok := want.(float64)
			if !aok || !bok {
				return false, errors.New("数值filter遇到非数值记录，拒绝猜测渲染")
			}
			switch str(condition, "operator") {
			case "GreaterThan":
				matched = a > b
			case "GreaterThanEqual":
				matched = a >= b
			case "LessThan":
				matched = a < b
			case "LessThanEqual":
				matched = a <= b
			}
		default:
			return false, errors.New("视图filter运算符未支持")
		}
		if conjunction == "And" {
			result = result && matched
		} else {
			result = result || matched
		}
	}
	return result, nil
}
