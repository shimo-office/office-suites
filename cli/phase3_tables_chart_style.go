package main

import (
	"errors"
	"math"
	"strconv"
)

// The native dashboard exposes eleven palette presets. UpdateComponent sends
// the complete existing component, preserving data source, layout and axes.
func preparePhase3ChartStyle(payload map[string]any, baseGuid string, tables map[string]map[string]any) (phase3BaseEditPlan, error) {
	p := phase3BaseEditPlan{phase3TableEditPlan: phase3TableEditPlan{Action: "table-chart-style", TableGuid: str(payload, "dashboardGuid")}}
	if err := phase3TablePayloadKeys(payload, "dashboardGuid", "componentGuid", "themeColor"); err != nil {
		return p, err
	}
	d := tables[p.TableGuid]
	if d == nil || num(d, "tableType") != 4 || str(d, "baseGuid") != baseGuid {
		return p, errors.New("目标仪表盘不存在")
	}
	components, ok := d["components"].([]any)
	if !ok || len(components) != 1 {
		return p, errors.New("样式验收目前要求仪表盘只有一个图表")
	}
	c, ok := components[0].(map[string]any)
	if !ok || str(c, "componentGuid") != str(payload, "componentGuid") || str(payload, "componentGuid") == "" {
		return p, errors.New("目标原生图表不存在")
	}
	palette, ok := payload["themeColor"].(float64)
	if !ok || palette < 0 || palette > 10 || palette != math.Trunc(palette) {
		return p, errors.New("themeColor 必须为 0 至 10 的原生调色板编号")
	}
	style, ok := c["style"].(map[string]any)
	if !ok || str(style, "themeColor") == strconv.Itoa(int(palette)) {
		return p, errors.New("样式缺失或调色板没有变化")
	}
	ds, _ := c["dataSource"].(map[string]any)
	axis, _ := c["axis"].(map[string]any)
	x, _ := axis["xAxis"].(map[string]any)
	y, _ := axis["yAxis"].(map[string]any)
	p.SourceGuid, p.FieldGuid = str(ds, "tableGuid"), str(x, "fieldGuid")
	if str(ds, "viewGuid") != "" || str(y, "mode") != "count" || tables[p.SourceGuid] == nil {
		return p, errors.New("样式验收目前要求全表计数图表")
	}
	if num(c, "type") != 1 && num(c, "type") != 2 && num(c, "type") != 3 && num(c, "type") != 5 {
		return p, errors.New("样式验收不支持此图表类型")
	}
	counts, err := phase3ChartExpectedCounts(tables[p.SourceGuid], p.FieldGuid)
	if err != nil {
		return p, err
	}
	if len(counts) == 0 || (len(counts) == 1 && counts[""] > 0) {
		return p, errors.New("样式验收源数据为空")
	}
	p.LinkedExpected = map[string]map[string]any{}
	for id, table := range tables {
		p.LinkedExpected[id] = phase3TableCopy(table)
	}
	p.Dashboard = p.LinkedExpected[p.TableGuid]
	updated := p.Dashboard["components"].([]any)[0].(map[string]any)
	updated["style"].(map[string]any)["themeColor"] = strconv.Itoa(int(palette))
	data := phase3TableCopy(updated)
	data["baseGuid"], data["dashboardGuid"] = baseGuid, p.TableGuid
	return p, p.add("UpdateComponent", data)
}

// Exact native SDK preset colors, modules 49858 (Rg) and 21516. A changed
// upstream theme fails closed until its mapping is independently verified.
var phase3ChartPalettes = map[string][]string{
	"0":  {"#DE7F83", "#F3833A", "#3861AF", "#7AE6F5", "#9C8EC1", "#80BE7F"},
	"1":  {"#7AE6F5", "#00C9E8", "#008FA0", "#FAD2D8", "#E0504E", "#AF3A37"},
	"2":  {"#E0504E", "#E9A2A5", "#ED6B00", "#E9D45C", "#66B165", "#1E541D"},
	"3":  {"#31218B", "#59439E", "#73B1EB", "#008FA0", "#66B165", "#EBE361"},
	"4":  {"#CD4747", "#F3833A", "#98C7F1", "#7AE6F5", "#80BE7F", "#EBE361"},
	"5":  {"#E96465", "#9C8EC1", "#4583D4", "#00B9D4", "#3A8438", "#EBE361"},
	"6":  {"#221A83", "#7562AB", "#ED6B00", "#006C72", "#00B9D4", "#EBE361"},
	"7":  {"#30732E", "#66B165", "#CF4900", "#FAA573", "#006C72", "#00B9D4"},
	"8":  {"#30732E", "#A4D0A3", "#E0A849", "#EBE361", "#3861AF", "#73B1EB"},
	"9":  {"#31218B", "#EBE361", "#7AE6F5", "#A4D0A3", "#9C8EC1", "#73B1EB"},
	"10": {"#9C8EC1", "#E9A2A5", "#FAA573", "#66B165", "#7AE6F5", "#98C7F1"},
}
