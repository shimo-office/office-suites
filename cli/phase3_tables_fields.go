package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

func preparePhase3TableFieldUpdate(p *phase3TableEditPlan, payload, before map[string]any, viewGuid string) error {
	if err := phase3TablePayloadKeys(payload, "tableGuid", "fieldGuid", "name"); err != nil {
		return err
	}
	id, ok := payload["fieldGuid"].(string)
	if !ok || id == "" {
		return errors.New("fieldGuid 必须是字符串")
	}
	_, ok = phase3TableMap(before, "fields")[id]
	if !ok {
		return errors.New("指定字段不存在")
	}
	name, ok := payload["name"].(string)
	if !ok || strings.TrimSpace(name) == "" || len(name) > 200 {
		return errors.New("字段名称无效")
	}
	for other, m := range phase3TableMap(before, "fields") {
		if other != id && str(m, "name") == name {
			return errors.New("字段名称已存在")
		}
	}
	expected := p.Expected["fields"].(map[string]any)[id].(map[string]any)
	expected["name"] = name
	return p.add("UpdateColumn", map[string]any{"tableGuid": p.TableGuid, "viewGuid": viewGuid, "colGuid": id, "name": name})
}
func preparePhase3TableFieldDelete(p *phase3TableEditPlan, payload, before map[string]any, viewGuid string) error {
	if err := phase3TablePayloadKeys(payload, "tableGuid", "fieldGuids"); err != nil {
		return err
	}
	ids, ok := payload["fieldGuids"].([]any)
	if !ok || len(ids) == 0 || len(ids) > 20 {
		return errors.New("fieldGuids 必须包含 1 至 20 个字段")
	}
	fields := phase3TableMap(before, "fields")
	seen := map[string]bool{}
	for _, v := range ids {
		id, ok := v.(string)
		field, exists := fields[id]
		if !ok || id == "" || !exists || seen[id] {
			return errors.New("删除字段 guid 不存在或重复")
		}
		if boolv(field, "isPrimary") {
			return errors.New("禁止删除主字段")
		}
		if str(field, "type") == "ReferenceRecord" || str(field, "type") == "RefRecordRollUp" || str(field, "formQuestionGuid") != "" {
			return errors.New("关联/汇总/表单字段可能影响其他对象，拒绝局部删除")
		}
		seen[id] = true
		quoted, _ := json.Marshal(id)
		for other, f := range fields {
			if other == id {
				continue
			}
			raw, _ := json.Marshal(f["props"])
			if strings.Contains(string(raw), string(quoted)) {
				return errors.New("该字段被其他字段引用，拒绝隐式破坏依赖")
			}
		}
		for _, v := range phase3TableRows(before, "views") {
			view := phase3TableCopy(v.(map[string]any))
			for _, key := range []string{"fields", "fieldInfo", "records"} {
				delete(view, key)
			}
			raw, _ := json.Marshal(view)
			if strings.Contains(string(raw), string(quoted)) {
				return errors.New("该字段用于视图配置，拒绝隐式改写筛选/分组")
			}
		}
		p.DeletedIDs = append(p.DeletedIDs, id)
	}
	expectedFields := p.Expected["fields"].(map[string]any)
	records := p.Expected["records"].(map[string]any)
	for _, id := range p.DeletedIDs {
		delete(expectedFields, id)
	}
	for rowID, value := range records {
		m, _ := value.(map[string]any)
		copy := phase3TableCopy(m)
		for _, id := range p.DeletedIDs {
			delete(copy, id)
		}
		records[rowID] = copy
	}
	return p.add("DeleteColumn", map[string]any{"tableGuid": p.TableGuid, "viewGuid": viewGuid, "colGuids": p.DeletedIDs})
}

func phase3FieldDeleteDependencies(tables map[string]map[string]any, tableGuid string, payload map[string]any) (map[string]map[string]any, error) {
	ids, ok := payload["fieldGuids"].([]any)
	if !ok {
		return nil, errors.New("fieldGuids 必须为数组")
	}
	unchanged := map[string]map[string]any{}
	for id, table := range tables {
		if id == tableGuid {
			continue
		}
		for _, raw := range ids {
			field, ok := raw.(string)
			if !ok {
				return nil, errors.New("字段 guid 必须为字符串")
			}
			if phase3ContainsTableReference(table, field) {
				return nil, errors.New("字段被其他数据表或仪表盘引用，拒绝删除")
			}
		}
		unchanged[id] = table
	}
	return unchanged, nil
}

func (s *phase3SDKSession) verifyPhase3UnchangedTables(expected map[string]map[string]any, source string) error {
	if expected == nil {
		return nil
	}
	actual, err := s.baseSnapshot()
	if err != nil {
		return err
	}
	if len(actual) != len(expected)+1 || actual[source] == nil {
		return errors.New("字段删除意外改变数据表列表")
	}
	for id, before := range expected {
		after := actual[id]
		if after == nil || !reflect.DeepEqual(phase3BaseComparable(before), phase3BaseComparable(after)) {
			return errors.New("字段删除影响了其他数据表或仪表盘")
		}
	}
	return nil
}
