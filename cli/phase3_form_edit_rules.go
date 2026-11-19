package main

import (
	"errors"
	"fmt"
)

const phase3NewRuleID = "phase3newRule"

func phase3FormRuleExpected(out, req map[string]any) error {
	rules := out["rules"].([]any)
	id := str(req, "guid")
	idx := -1
	for i, v := range rules {
		r, ok := v.(map[string]any)
		if !ok {
			return errors.New("规则不是对象")
		}
		if str(r, "guid") == id {
			idx = i
		}
		if str(r, "guid") == phase3NewRuleID {
			return errors.New("规则 guid 与内部新建标识冲突")
		}
	}
	if str(req, "operation") == "rule-delete" {
		if idx < 0 {
			return errors.New("目标规则不存在")
		}
		out["rules"] = append(append([]any{}, rules[:idx]...), rules[idx+1:]...)
		return nil
	}
	if str(req, "operation") == "rule-add" {
		id = phase3NewRuleID
	} else if idx < 0 {
		return errors.New("目标规则不存在")
	}
	options, ok := req["options"].([]any)
	if !ok || len(options) == 0 {
		return errors.New("规则 options 必须是非空选项value数组")
	}
	encoded := []any{}
	seen := map[string]bool{}
	for _, v := range options {
		s, ok := v.(string)
		if !ok || s == "" || seen[s] {
			return errors.New("规则 option 无效或重复")
		}
		seen[s] = true
		encoded = append(encoded, map[string]any{"type": "normal", "value": s})
	}
	targets, ok := req["targets"].([]any)
	if !ok || len(targets) == 0 {
		return errors.New("规则 targets 必须是非空问题GUID数组")
	}
	rule := map[string]any{"guid": id, "type": "conditional", "rule": map[string]any{"conditions": []any{map[string]any{"type": "select_option", "question": str(req, "trigger"), "options": encoded}}, "effects": []any{map[string]any{"type": "show_question", "questions": targets}, map[string]any{"type": "show_widget", "widgets": []any{}}}}}
	if err := phase3ValidateFormRule(out, rule); err != nil {
		return err
	}
	if idx < 0 {
		out["rules"] = append(rules, rule)
	} else {
		rules[idx] = rule
	}
	return nil
}
func phase3ValidateFormRule(form, rule map[string]any) error {
	if str(rule, "type") != "conditional" {
		return errors.New("包含尚不支持验证的规则类型")
	}
	detail, ok := rule["rule"].(map[string]any)
	if !ok {
		return errors.New("规则配置缺失")
	}
	conds, ok := detail["conditions"].([]any)
	if !ok || len(conds) != 1 {
		return errors.New("当前仅支持单条件规则")
	}
	cond, ok := conds[0].(map[string]any)
	if !ok || str(cond, "type") != "select_option" {
		return errors.New("仅支持 select_option 条件")
	}
	questions := form["questions"].([]any)
	positions := map[string]int{}
	var trigger map[string]any
	for i, v := range questions {
		q, ok := v.(map[string]any)
		if !ok {
			return errors.New("题目结构无效")
		}
		positions[str(q, "guid")] = i
		if str(q, "guid") == str(cond, "question") {
			trigger = q
		}
	}
	if trigger == nil {
		return errors.New("规则触发题不存在")
	}
	switch str(trigger, "type") {
	case "single_choice", "multiple_choice", "dropdown_list":
	default:
		return errors.New("规则触发题必须为选项题")
	}
	config, _ := trigger["question"].(map[string]any)
	opts, _ := config["options"].([]any)
	available := map[string]bool{}
	for _, v := range opts {
		m, _ := v.(map[string]any)
		if str(m, "type") == "normal" {
			available[str(m, "value")] = true
		}
	}
	wanted, ok := cond["options"].([]any)
	if !ok || len(wanted) == 0 {
		return errors.New("规则缺少触发选项")
	}
	for _, v := range wanted {
		o, ok := v.(map[string]any)
		if !ok || str(o, "type") != "normal" || !available[str(o, "value")] {
			return errors.New("规则触发选项不存在")
		}
	}
	effects, ok := detail["effects"].([]any)
	if !ok || len(effects) != 2 {
		return errors.New("规则effects结构无效")
	}
	show, ok := effects[0].(map[string]any)
	if !ok || str(show, "type") != "show_question" {
		return errors.New("仅支持显示问题规则")
	}
	targets, ok := show["questions"].([]any)
	if !ok || len(targets) == 0 {
		return errors.New("规则没有显示目标题")
	}
	seen := map[string]bool{}
	for _, v := range targets {
		id, ok := v.(string)
		pos, exists := positions[id]
		if !ok || !exists || pos <= positions[str(trigger, "guid")] || seen[id] {
			return fmt.Errorf("规则目标题 %v 不存在、重复或位于触发题之前", v)
		}
		seen[id] = true
	}
	widget, ok := effects[1].(map[string]any)
	if !ok || str(widget, "type") != "show_widget" {
		return errors.New("规则widget结构无效")
	}
	widgets, ok := widget["widgets"].([]any)
	if !ok || len(widgets) != 0 {
		return errors.New("当前不支持组件显示规则")
	}
	return nil
}
