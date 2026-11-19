package main

import (
	"errors"
	"math"
	"strconv"
)

func preparePhase3TableConversion(p *phase3TableEditPlan, payload, before map[string]any, viewGuid string) error {
	if err := phase3TablePayloadKeys(payload, "tableGuid", "fieldGuid", "type"); err != nil {
		return err
	}
	id, to := str(payload, "fieldGuid"), str(payload, "type")
	field := phase3TableMap(before, "fields")[id]
	if field == nil {
		return errors.New("待转换字段不存在")
	}
	from := str(field, "type")
	if !((from == "Number" && to == "SingleLineText") || (from == "SingleLineText" && to == "Number")) {
		return errors.New("仅支持可无损回转的 Number 与 SingleLineText 类型转换")
	}
	// Reuse deletion's dependency validation on an isolated plan. This never
	// submits its DeleteColumn delta; conversion emits only UpdateColumn/Cell.
	safety := phase3TableEditPlan{TableGuid: p.TableGuid, Expected: phase3TableSemantic(before)}
	if err := preparePhase3TableFieldDelete(&safety, map[string]any{"tableGuid": p.TableGuid, "fieldGuids": []any{id}}, before, viewGuid); err != nil {
		return err
	}
	expectedField := p.Expected["fields"].(map[string]any)[id].(map[string]any)
	expectedField["type"], expectedField["props"] = to, nil
	changes := map[string]any{}
	for rowID, row := range phase3TableMap(before, "records") {
		values := row["valueMap"].(map[string]any)
		value, exists := values[id]
		if !exists || value == nil {
			continue
		}
		var converted any
		if from == "Number" {
			n, ok := value.(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
				return errors.New("数值字段含无效值，拒绝转换")
			}
			converted = strconv.FormatFloat(n, 'f', -1, 64)
		} else {
			text, ok := value.(string)
			if !ok || text == "" {
				return errors.New("文本字段含空字符串/非文本，拒绝可能丢值的转换")
			}
			n, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || strconv.FormatFloat(n, 'f', -1, 64) != text {
				return errors.New("文本值不能无损回转为数字，未执行写入")
			}
			converted = n
		}
		next := phase3TableCopy(values)
		next[id] = converted
		p.Expected["records"].(map[string]any)[rowID] = next
		changes[rowID] = map[string]any{id: converted}
	}
	if err := p.add("UpdateColumn", map[string]any{"tableGuid": p.TableGuid, "viewGuid": viewGuid, "colGuid": id, "type": to, "props": nil}); err != nil {
		return err
	}
	if len(changes) > 0 {
		return p.add("UpdateCell", map[string]any{"tableGuid": p.TableGuid, "viewGuid": viewGuid, "rows": changes})
	}
	return nil
}
