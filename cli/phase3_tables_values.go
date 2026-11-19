package main

import (
	"errors"
	"time"
)

// The native Date field stores OA days, not Unix milliseconds. Require an
// explicit ISO timestamp with zone at the CLI boundary, then convert to the
// SDK's UTC day origin (1899-12-30); document timezone controls display only.
func phase3NormalizeTableValues(values map[string]any, fields map[string]map[string]any) (map[string]any, error) {
	result := phase3TableCopy(values)
	for id, value := range result {
		if str(fields[id], "type") != "Date" || value == nil {
			continue
		}
		iso, ok := value.(string)
		if !ok {
			return nil, errors.New("Date 输入必须是含时区的 RFC3339 时间字符串")
		}
		instant, err := time.Parse(time.RFC3339Nano, iso)
		if err != nil {
			return nil, errors.New("Date 输入必须是含时区的 RFC3339 时间字符串")
		}
		result[id] = float64(instant.UnixMilli())/86400000 + 25569
	}
	return result, nil
}
