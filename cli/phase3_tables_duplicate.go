package main

import (
	"bytes"
	"errors"
)

// Normalize only schema-defined identity locations. User cell values, field
// names, descriptions, option names and filter literals remain byte-for-byte.
func phase3DuplicateComparable(table map[string]any, ids map[string]string) map[string]any {
	mapID := func(id string) string {
		if replacement, ok := ids[id]; ok {
			return replacement
		}
		return id
	}
	copy := phase3TableCopy(table)
	copy["guid"] = mapID(str(copy, "guid"))
	copy["name"] = ""
	for _, field := range phase3TableRows(copy, "fields") {
		f := field.(map[string]any)
		f["guid"] = mapID(str(f, "guid"))
		if id, ok := f["tableGuid"].(string); ok {
			f["tableGuid"] = mapID(id)
		}
	}
	for _, record := range phase3TableRows(copy, "records") {
		r := record.(map[string]any)
		r["guid"] = mapID(str(r, "guid"))
		if id, ok := r["tableGuid"].(string); ok {
			r["tableGuid"] = mapID(id)
		}
		values := map[string]any{}
		for field, value := range r["valueMap"].(map[string]any) {
			values[mapID(field)] = value
		}
		r["valueMap"] = values
	}
	for _, raw := range phase3TableRows(copy, "views") {
		view := raw.(map[string]any)
		for _, key := range []string{"guid", "tableGuid"} {
			if id, ok := view[key].(string); ok {
				view[key] = mapID(id)
			}
		}
		delete(view, "createdBy")
		for _, key := range []string{"fields", "records"} {
			if list, ok := view[key].([]any); ok {
				for i, value := range list {
					if id, ok := value.(string); ok {
						list[i] = mapID(id)
					}
				}
			}
		}
		if info, ok := view["fieldInfo"].(map[string]any); ok {
			mapped := map[string]any{}
			for id, value := range info {
				mapped[mapID(id)] = value
			}
			view["fieldInfo"] = mapped
		}
		for _, key := range []string{"filters", "groups", "sort", "calendar", "gantt", "gallery", "list", "kanban"} {
			if value, exists := view[key]; exists {
				view[key] = phase3DuplicateViewReferences(value, mapID)
			}
		}
	}
	return phase3BaseComparable(copy)
}

func phase3DuplicateViewReferences(value any, mapID func(string) string) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = phase3DuplicateViewReferences(x, mapID)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for key, x := range v {
			switch key {
			case "fieldGuid", "startFieldGuid", "endFieldGuid", "followFieldGuid", "labelFieldGuid", "photoFieldGuid":
				if id, ok := x.(string); ok {
					out[key] = mapID(id)
				} else {
					out[key] = x
				}
			case "showFieldGuids":
				if list, ok := x.([]any); ok {
					mapped := make([]any, len(list))
					for i, id := range list {
						if id, ok := id.(string); ok {
							mapped[i] = mapID(id)
						} else {
							mapped[i] = id
						}
					}
					out[key] = mapped
				} else {
					out[key] = x
				}
			case "conditions", "rules", "color":
				out[key] = phase3DuplicateViewReferences(x, mapID)
			default:
				out[key] = x
			}
		}
		return out
	default:
		return value
	}
}

// The native duplicate endpoint can return an empty 2xx acknowledgement.
// This is submission evidence only: the caller still requires exact base-wide
// readback, revision advancement, and a fresh editor before reporting success.
func phase3DuplicateResponse(r *Response) (any, error) {
	if r.Status >= 200 && r.Status < 300 && r.Data == nil && len(bytes.TrimSpace(r.Raw)) == 0 {
		return map[string]any{"emptyResponse": true, "httpStatus": r.Status, "requiresReadback": true}, nil
	}
	value, err := phase3SDKJSON(r)
	if err != nil {
		return nil, err
	}
	m, ok := value.(map[string]any)
	if !ok || str(m, "code") != "0" {
		return nil, errors.New("复制未返回code=0或明确空2xx确认；禁止自动重试")
	}
	return value, nil
}
