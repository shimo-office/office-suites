package main

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"reflect"
)

func (s *phase3SDKSession) tableReadRevision() (float64, error) {
	data, err := s.get("table-api/bases/" + url.PathEscape(s.guid) + "/rev")
	if err != nil {
		return 0, err
	}
	value, ok := data.(map[string]any)
	if !ok {
		return 0, errors.New("表格只读revision响应无效")
	}
	rev, ok := value["rev"].(float64)
	permission, _ := value["filePermission"].(map[string]any)
	if !ok || math.IsNaN(rev) || math.IsInf(rev, 0) || rev < 0 || rev != math.Trunc(rev) || !boolv(permission, "canRead") {
		return 0, errors.New("缺少有效只读revision或读取权限")
	}
	return rev, nil
}

// The table endpoint fully hydrates only its selected view. Follow the native
// getViewData route for each lazy view; never infer orders from another view.
func (s *phase3SDKSession) hydrateTableViews(table map[string]any) error {
	fields, records := phase3TableMap(table, "fields"), phase3TableMap(table, "records")
	for i, raw := range phase3TableRows(table, "views") {
		view := raw.(map[string]any)
		_, hasFields := view["fields"].([]any)
		_, hasRecords := view["records"].([]any)
		if hasFields && hasRecords {
			if err := phase3ValidateViewOrders(view, str(table, "guid"), fields, records); err != nil {
				return err
			}
			continue
		}
		id := str(view, "guid")
		data, err := s.get("table-api/tables/" + url.PathEscape(str(table, "guid")) + "/views/" + url.PathEscape(id))
		if err != nil {
			return err
		}
		loaded, ok := data.(map[string]any)
		if !ok || str(loaded, "guid") != id || str(loaded, "tableGuid") != str(table, "guid") {
			return errors.New("延迟加载视图身份与所属表不符")
		}
		for key, old := range view {
			fresh, exists := loaded[key]
			if !exists && key != "creator" {
				return fmt.Errorf("完整视图缺少原有元数据%s", key)
			}
			lazyOrder := (key == "fields" || key == "records") && old == nil
			if exists && !lazyOrder && !reflect.DeepEqual(old, fresh) {
				return fmt.Errorf("延迟视图元数据%s在快照期间改变", key)
			}
		}
		if err = phase3ValidateViewOrders(loaded, str(table, "guid"), fields, records); err != nil {
			return err
		}
		merged := phase3TableCopy(view)
		for key, value := range loaded {
			merged[key] = value
		}
		table["views"].([]any)[i] = merged
	}
	return nil
}

func phase3ValidateViewOrders(view map[string]any, tableGuid string, fields, records map[string]map[string]any) error {
	if str(view, "guid") == "" || str(view, "tableGuid") != tableGuid {
		return errors.New("完整视图身份或所属表不符")
	}
	for key, members := range map[string]map[string]map[string]any{"fields": fields, "records": records} {
		order, ok := view[key].([]any)
		if !ok {
			return fmt.Errorf("完整视图缺少%s顺序", key)
		}
		seen := map[string]bool{}
		for _, raw := range order {
			member, ok := raw.(string)
			if !ok || seen[member] || members[member] == nil {
				return fmt.Errorf("视图%s顺序含未知或重复成员", key)
			}
			seen[member] = true
		}
	}
	return nil
}
