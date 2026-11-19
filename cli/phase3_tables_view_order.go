package main

import (
	"errors"
	"math"
)

// Native CreateView inserts after targetGuid. Predict the complete ordering
// from the untouched baseline; no server readback values enter this oracle.
func phase3InsertedViewOrders(before map[string]any, anchorID string) (map[string]float64, float64, error) {
	views := phase3TableMap(before, "views")
	orders := map[string]float64{}
	seen := map[float64]bool{}
	for id, view := range views {
		order, ok := view["order"].(float64)
		if !ok || math.IsNaN(order) || math.IsInf(order, 0) || order < 0 || order >= 9007199254740991 || math.Trunc(order) != order || seen[order] {
			return nil, 0, errors.New("现有视图顺序缺失或不唯一，无法预测插入位置")
		}
		orders[id], seen[order] = order, true
	}
	anchor, ok := orders[anchorID]
	if !ok {
		return nil, 0, errors.New("插入锚点视图不存在")
	}
	result := map[string]float64{}
	for id, order := range orders {
		if order > anchor {
			order++
		}
		result[id] = order
	}
	return result, anchor + 1, nil
}
