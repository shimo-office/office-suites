package main

import "errors"

func preparePhase3ReferenceSet(payload map[string]any, baseGuid string, tables map[string]map[string]any) (phase3BaseEditPlan, error) {
	p := phase3BaseEditPlan{phase3TableEditPlan: phase3TableEditPlan{Action: "table-reference-set", TableGuid: str(payload, "tableGuid")}, FieldGuid: str(payload, "fieldGuid")}
	if err := phase3TablePayloadKeys(payload, "tableGuid", "fieldGuid", "recordGuid", "referenceRecordGuids"); err != nil {
		return p, err
	}
	source := tables[p.TableGuid]
	if source == nil || str(source, "baseGuid") != baseGuid {
		return p, errors.New("关联源表不存在")
	}
	field := phase3TableMap(source, "fields")[p.FieldGuid]
	if field == nil || str(field, "type") != "ReferenceRecord" {
		return p, errors.New("必须使用真实 ReferenceRecord 字段")
	}
	props, _ := field["props"].(map[string]any)
	p.ReferenceTableGuid, p.ReferenceFieldGuid = str(props, "tableGuid"), str(props, "foreignTableColumnGuid")
	if str(props, "baseGuid") != "" && str(props, "baseGuid") != baseGuid {
		return p, errors.New("不写跨base关联")
	}
	target := tables[p.ReferenceTableGuid]
	if target == nil || p.ReferenceTableGuid == p.TableGuid || str(target, "baseGuid") != baseGuid {
		return p, errors.New("关联目标不是同base另一已读取数据表")
	}
	back := phase3TableMap(target, "fields")[p.ReferenceFieldGuid]
	backProps, _ := back["props"].(map[string]any)
	if back == nil || str(back, "type") != "ReferenceRecord" || str(backProps, "tableGuid") != p.TableGuid || str(backProps, "foreignTableColumnGuid") != p.FieldGuid {
		return p, errors.New("关联反向字段指针未建立")
	}
	for _, table := range []map[string]any{source, target} {
		if num(table, "tableType") != 0 || str(table, "formGuid") != "" || table["syncedConfig"] != nil {
			return p, errors.New("关联值写入不支持表单、同步或特殊数据表")
		}
	}
	for _, settings := range []map[string]any{props, backProps} {
		if boolv(settings, "isRefToView") || str(settings, "refViewGuid") != "" || str(settings, "error") != "" || (str(settings, "baseGuid") != "" && str(settings, "baseGuid") != baseGuid) {
			return p, errors.New("关联字段存在视图限制、错误或跨base引用，拒绝写入")
		}
	}
	rowID := str(payload, "recordGuid")
	row := phase3TableMap(source, "records")[rowID]
	if row == nil {
		return p, errors.New("源记录不存在")
	}
	selected, ok := payload["referenceRecordGuids"].([]any)
	if !ok || len(selected) > 100 {
		return p, errors.New("关联记录必须为最多100项GUID数组")
	}
	if props["isMultiple"] != true && len(selected) > 1 {
		return p, errors.New("此关联字段不允许多选")
	}
	sourceName, err := phase3ReferenceDisplayName(source, row)
	if err != nil {
		return p, err
	}
	targetRows := phase3TableMap(target, "records")
	oldLinks, err := phase3ExistingReferenceIDs(row["valueMap"].(map[string]any)[p.FieldGuid])
	if err != nil {
		return p, err
	}
	for id := range oldLinks {
		if targetRows[id] == nil {
			return p, errors.New("原关联指向不存在的记录")
		}
	}

	seen := map[string]bool{}
	newValues := []any{}
	for _, raw := range selected {
		id, ok := raw.(string)
		if !ok || seen[id] || targetRows[id] == nil {
			return p, errors.New("关联记录不存在或重复")
		}
		seen[id] = true
		name, e := phase3ReferenceDisplayName(target, targetRows[id])
		if e != nil {
			return p, e
		}
		newValues = append(newValues, map[string]any{"refRecordGuid": id, "refDisplayName": name})
	}
	p.LinkedExpected = map[string]map[string]any{}
	for id, t := range tables {
		p.LinkedExpected[id] = phase3TableCopy(t)
	}
	expectedSource := phase3TableMap(p.LinkedExpected[p.TableGuid], "records")[rowID]
	expectedSource["valueMap"].(map[string]any)[p.FieldGuid] = newValues
	targetChanges := map[string]any{}
	for id, row := range targetRows {
		values := row["valueMap"].(map[string]any)
		old := values[p.ReferenceFieldGuid]
		oldBack, err := phase3ExistingReferenceIDs(old)
		if err != nil {
			return p, err
		}
		if oldBack[rowID] != oldLinks[id] {
			return p, errors.New("已有双向关联不一致，拒绝自动修复或覆盖")
		}
		for linked := range oldBack {
			if phase3TableMap(source, "records")[linked] == nil {
				return p, errors.New("已有反向关联指向不存在记录")
			}
		}
		refs := []any{}
		had := false
		if old != nil {
			entries, ok := old.([]any)
			if !ok {
				return p, errors.New("原关联值不是记录引用数组")
			}
			for _, entry := range entries {
				ref, ok := entry.(map[string]any)
				if !ok || str(ref, "refRecordGuid") == "" {
					return p, errors.New("原关联引用无效")
				}
				if str(ref, "refRecordGuid") == rowID {
					had = true
				} else {
					refs = append(refs, ref)
				}
			}
		}
		if seen[id] {
			refs = append(refs, map[string]any{"refRecordGuid": rowID, "refDisplayName": sourceName})
		}
		if seen[id] || had {
			if backProps["isMultiple"] != true && len(refs) > 1 {
				return p, errors.New("反向关联不允许多选；拒绝覆盖其他引用")
			}
			targetChanges[id] = map[string]any{p.ReferenceFieldGuid: refs}
			phase3TableMap(p.LinkedExpected[p.ReferenceTableGuid], "records")[id]["valueMap"].(map[string]any)[p.ReferenceFieldGuid] = refs
		}
	}
	if len(phase3TableRows(source, "views")) == 0 || len(phase3TableRows(target, "views")) == 0 {
		return p, errors.New("关联表缺少视图")
	}
	sourceView := phase3TableRows(source, "views")[0].(map[string]any)
	targetView := phase3TableRows(target, "views")[0].(map[string]any)
	if err = p.add("UpdateCell", map[string]any{"tableGuid": p.TableGuid, "viewGuid": str(sourceView, "guid"), "rows": map[string]any{rowID: map[string]any{p.FieldGuid: newValues}}}); err != nil {
		return p, err
	}
	if len(targetChanges) > 0 {
		err = p.add("UpdateCell", map[string]any{"tableGuid": p.ReferenceTableGuid, "viewGuid": str(targetView, "guid"), "rows": targetChanges})
	}
	return p, err
}

func phase3ReferenceDisplayName(table, row map[string]any) (string, error) {
	for id, field := range phase3TableMap(table, "fields") {
		if boolv(field, "isPrimary") {
			if str(field, "type") != "SingleLineText" {
				return "", errors.New("关联显示名目前需要文本主字段")
			}
			name, ok := row["valueMap"].(map[string]any)[id].(string)
			if !ok || name == "" {
				return "", errors.New("关联记录的文本主字段为空")
			}
			return name, nil
		}
	}
	return "", errors.New("关联表无主字段")
}

func phase3ExistingReferenceIDs(raw any) (map[string]bool, error) {
	out := map[string]bool{}
	if raw == nil {
		return out, nil
	}
	entries, ok := raw.([]any)
	if !ok {
		return nil, errors.New("原关联值不是记录引用数组")
	}
	for _, entry := range entries {
		ref, ok := entry.(map[string]any)
		id := str(ref, "refRecordGuid")
		if !ok || id == "" || out[id] {
			return nil, errors.New("原关联值存在无效或重复引用")
		}
		out[id] = true
	}
	return out, nil
}
