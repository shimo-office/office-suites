package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

func preparePhase3TableReference(payload map[string]any, baseGuid string, tables map[string]map[string]any) (phase3BaseEditPlan, error) {
	p := phase3BaseEditPlan{phase3TableEditPlan: phase3TableEditPlan{Action: "table-reference-add", TableGuid: str(payload, "tableGuid")}, ReferenceTableGuid: str(payload, "referenceTableGuid"), Name: str(payload, "name")}
	if err := phase3TablePayloadKeys(payload, "tableGuid", "referenceTableGuid", "name"); err != nil {
		return p, err
	}
	source, target := tables[p.TableGuid], tables[p.ReferenceTableGuid]
	if source == nil || target == nil || p.TableGuid == p.ReferenceTableGuid {
		return p, errors.New("关联必须选择本应用表格内两个不同且已读取的数据表")
	}
	if num(source, "tableType") == 4 || num(target, "tableType") == 4 || source["syncedConfig"] != nil || target["syncedConfig"] != nil {
		return p, errors.New("仪表盘/同步表不能作为关联源或目标")
	}
	if str(source, "baseGuid") != baseGuid || str(target, "baseGuid") != baseGuid {
		return p, errors.New("关联数据表与当前 base 不一致")
	}
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 200 {
		return p, errors.New("关联字段名称为空或过长")
	}
	for _, field := range phase3TableMap(source, "fields") {
		if str(field, "name") == p.Name {
			return p, errors.New("字段名称重复")
		}
	}
	views := phase3TableRows(source, "views")
	if len(views) == 0 || len(phase3TableRows(target, "views")) == 0 {
		return p, errors.New("关联表缺少视图，无法验证")
	}
	id, err := phase3TableID()
	if err != nil {
		return p, err
	}
	p.FieldGuid = id[:11]
	id, err = phase3TableID()
	if err != nil {
		return p, err
	}
	p.ReferenceFieldGuid = id[:11]
	p.CreatedIDs = []string{p.FieldGuid, p.ReferenceFieldGuid}
	field := map[string]any{"guid": p.FieldGuid, "name": p.Name, "type": "ReferenceRecord", "isPrimary": false, "props": map[string]any{"baseGuid": "", "tableGuid": p.ReferenceTableGuid, "isMultiple": true, "isRefToView": false, "refViewGuid": "", "error": "", "foreignTableColumnGuid": p.ReferenceFieldGuid}}
	view := views[0].(map[string]any)
	viewGuid := str(view, "guid")
	return p, p.add("CreateColumn", map[string]any{"tableGuid": p.TableGuid, "viewGuid": viewGuid, "viewTargetGuids": phase3TableAppendTargets(source, "fields", viewGuid, []string{p.FieldGuid}), "cols": []any{field}})
}

func verifyPhase3TableReference(before, after map[string]map[string]any, p phase3BaseEditPlan) error {
	for _, side := range []struct {
		table, field, other, reverse string
		source                       bool
	}{{p.TableGuid, p.FieldGuid, p.ReferenceTableGuid, p.ReferenceFieldGuid, true}, {p.ReferenceTableGuid, p.ReferenceFieldGuid, p.TableGuid, p.FieldGuid, false}} {
		old, next := before[side.table], after[side.table]
		if old == nil || next == nil {
			return errors.New("关联的一侧数据表未读回")
		}
		if err := phase3ValidateTableSnapshot(next, str(old, "baseGuid"), side.table); err != nil {
			return err
		}
		fields := phase3TableMap(next, "fields")
		newField := fields[side.field]
		if len(fields) != len(phase3TableMap(old, "fields"))+1 || newField == nil || str(newField, "type") != "ReferenceRecord" || boolv(newField, "isPrimary") {
			return errors.New("双向关联字段未完整建立")
		}
		if side.source && str(newField, "name") != p.Name {
			return errors.New("关联字段名称不符")
		}
		props, ok := newField["props"].(map[string]any)
		if !ok || str(props, "tableGuid") != side.other || str(props, "foreignTableColumnGuid") != side.reverse || str(props, "error") != "" {
			return errors.New("关联字段目标/反向指针无效")
		}
		if foreignBase := str(props, "baseGuid"); foreignBase != "" && foreignBase != str(old, "baseGuid") {
			return errors.New("关联越过当前 base")
		}
		if side.source && (props["isMultiple"] != true || props["isRefToView"] != false || str(props, "refViewGuid") != "") {
			return errors.New("关联字段属性与计划不符")
		}
		stripped := phase3TableCopy(next)
		remaining := []any{}
		for _, raw := range phase3TableRows(stripped, "fields") {
			f := raw.(map[string]any)
			if str(f, "guid") != side.field {
				remaining = append(remaining, raw)
			}
		}
		stripped["fields"] = remaining
		for _, row := range phase3TableMap(stripped, "records") {
			values := row["valueMap"].(map[string]any)
			if v, exists := values[side.field]; exists {
				if v != nil {
					if arr, ok := v.([]any); !ok || len(arr) > 0 {
						return errors.New("新增关联字段意外更改记录值")
					}
				}
				delete(values, side.field)
			}
		}
		for _, view := range phase3TableMap(stripped, "views") {
			original := phase3TableMap(old, "views")[str(view, "guid")]
			if original == nil {
				return errors.New("关联意外新增视图")
			}
			order, ok := view["fields"].([]any)
			if !ok {
				return errors.New("关联后视图无字段顺序")
			}
			kept := []any{}
			found := 0
			for _, id := range order {
				if id == side.field {
					found++
					continue
				}
				kept = append(kept, id)
			}
			if found > 1 || side.source && found != 1 {
				return errors.New("关联字段在视图缺失或重复")
			}
			if !reflect.DeepEqual(kept, original["fields"]) {
				return errors.New("关联改变原有字段顺序")
			}
			if side.source && len(order) > 0 && order[len(order)-1] != side.field {
				return errors.New("关联列没有追加到末尾")
			}
			view["fields"] = kept
			if info, ok := view["fieldInfo"].(map[string]any); ok {
				delete(info, side.field)
			}
		}
		if !reflect.DeepEqual(phase3BaseComparable(old), phase3BaseComparable(stripped)) {
			return fmt.Errorf("关联意外改变数据表 %s 的现有内容或视图", side.table)
		}
	}
	return nil
}
