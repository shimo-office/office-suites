package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

func phase3FormSubmissionAnswers(form, answers map[string]any) ([]any, error) {
	questions, ok := form["questions"].([]any)
	if !ok {
		return nil, errors.New("已发布题目结构无效")
	}
	known := map[string]bool{}
	positions := map[string]int{}
	for i, v := range questions {
		q, _ := v.(map[string]any)
		if err := phase3ValidateQuestion(q); err != nil {
			return nil, fmt.Errorf("提交前题型配置验证失败：%w", err)
		}
		known[str(q, "guid")] = true
		positions[str(q, "guid")] = i
	}
	for k := range answers {
		if !known[k] {
			return nil, fmt.Errorf("未知问题 %s", k)
		}
	}
	hidden := map[string]bool{}
	rules, _ := form["rules"].([]any)
	for _, v := range rules {
		r, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("规则无效")
		}
		if err := phase3ValidateFormRule(form, r); err != nil {
			return nil, err
		}
		detail := r["rule"].(map[string]any)
		for _, id := range detail["effects"].([]any)[0].(map[string]any)["questions"].([]any) {
			hidden[id.(string)] = true
		}
	}
	rules = append([]any{}, rules...)
	sort.SliceStable(rules, func(i, j int) bool {
		a := rules[i].(map[string]any)["rule"].(map[string]any)["conditions"].([]any)[0].(map[string]any)
		b := rules[j].(map[string]any)["rule"].(map[string]any)["conditions"].([]any)[0].(map[string]any)
		return positions[str(a, "question")] < positions[str(b, "question")]
	})
	for _, v := range rules {
		r := v.(map[string]any)["rule"].(map[string]any)
		c := r["conditions"].([]any)[0].(map[string]any)
		trigger := str(c, "question")
		if hidden[trigger] {
			continue
		}
		selected := map[string]bool{}
		switch a := answers[trigger].(type) {
		case string:
			selected[a] = true
		case []any:
			for _, x := range a {
				s, ok := x.(string)
				if ok {
					selected[s] = true
				}
			}
		}
		match := false
		for _, x := range c["options"].([]any) {
			if selected[str(x.(map[string]any), "value")] {
				match = true
			}
		}
		if match {
			for _, id := range r["effects"].([]any)[0].(map[string]any)["questions"].([]any) {
				hidden[id.(string)] = false
			}
		}
	}
	out := []any{}
	for _, v := range questions {
		q, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("题目无效")
		}
		id := str(q, "guid")
		value, exists := answers[id]
		if hidden[id] {
			if exists {
				return nil, fmt.Errorf("隐藏题 %s 不应提交答案", id)
			}
			continue
		}
		if !exists {
			if q["required"] == true {
				return nil, fmt.Errorf("必填题 %s 缺少答案", id)
			}
			continue
		}
		config, _ := q["question"].(map[string]any)
		a := map[string]any{"guid": id}
		switch str(q, "type") {
		case "text", "multiple_text":
			s, ok := value.(string)
			if !ok || len(s) > 20000 || (q["required"] == true && strings.TrimSpace(s) == "") {
				return nil, fmt.Errorf("文本题 %s 答案无效", id)
			}
			a["type"] = 0
			if q["type"] == "multiple_text" {
				a["type"] = 4
			}
			a["text"] = map[string]any{"content": s}
		case "single_choice", "dropdown_list", "multiple_choice":
			opts, _ := config["options"].([]any)
			valid := map[string]bool{}
			for _, x := range opts {
				m, _ := x.(map[string]any)
				if str(m, "type") == "normal" {
					valid[str(m, "value")] = true
				}
			}
			selected := []any{}
			if q["type"] == "multiple_choice" {
				var ok bool
				selected, ok = value.([]any)
				if !ok {
					return nil, errors.New("多选答案必须是数组")
				}
				a["type"] = 2
			} else {
				s, ok := value.(string)
				if !ok {
					return nil, errors.New("单选答案必须是选项value")
				}
				selected = []any{s}
				a["type"] = 1
				if q["type"] == "dropdown_list" {
					a["type"] = 3
				}
			}
			choices := []any{}
			seen := map[string]bool{}
			for _, x := range selected {
				s, ok := x.(string)
				if !ok || !valid[s] || seen[s] {
					return nil, errors.New("选项不存在或重复")
				}
				seen[s] = true
				choices = append(choices, map[string]any{"type": "normal", "value": s})
			}
			if q["required"] == true && len(choices) == 0 {
				return nil, errors.New("必填多选题为空")
			}
			if q["type"] == "multiple_choice" {
				a["choices"] = choices
			} else {
				a["choice"] = choices[0]
			}
		case "rate":
			n, ok := value.(float64)
			max, _ := config["sum"].(float64)
			if !ok || n != math.Trunc(n) || n < 1 || n > max {
				return nil, errors.New("评分答案必须是范围内整数")
			}
			a["type"] = 5
			a["rate"] = map[string]any{"value": n, "sum": max}
		default:
			return nil, fmt.Errorf("暂不支持提交 %s 题型", str(q, "type"))
		}
		out = append(out, a)
	}
	return out, nil
}
func (a *App) cmdPhase3FormSubmit() (runErr error) {
	if len(a.Pos) != 1 {
		return errors.New("用法：phase3 <表单链接> --action form-submit --payload '{\"answers\":{\"题目GUID\":\"答案\"}}' --out 证据目录")
	}
	var req map[string]any
	if json.Unmarshal([]byte(a.opt("--payload")), &req) != nil || len(req) != 1 {
		return errors.New("payload 仅接受 answers 对象")
	}
	answers, ok := req["answers"].(map[string]any)
	if !ok {
		return errors.New("answers 必须为对象")
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	s, err := a.phase3SDK(target, "form")
	if err != nil {
		return err
	}
	before, err := s.formPublication()
	if err != nil {
		return err
	}
	if str(before, "status") != "normal" {
		return errors.New("表单未开放收集")
	}
	if v := before["subscribeNotification"]; v != nil && v != "" {
		return errors.New("配置了通知订阅，未获发送通知授权")
	}
	var form map[string]any
	if json.Unmarshal([]byte(str(before, "publishedContent")), &form) != nil {
		return errors.New("已发布结构无效")
	}
	content, err := phase3FormSubmissionAnswers(form, answers)
	if err != nil {
		return err
	}
	me, err := a.usersMe()
	if err != nil {
		return err
	}
	if str(me, "id") == "" {
		return errors.New("当前用户身份无效")
	}
	rows, total, err := s.formResults()
	if err != nil {
		return err
	}
	prior := map[string]bool{}
	for _, r := range rows {
		prior[str(r, "submitId")] = true
	}
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	attempted := false
	defer func() {
		if runErr != nil {
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": attempted, "mayHaveApplied": attempted, "error": runErr.Error()})
			runErr = fmt.Errorf("提交答卷失败（mayHaveApplied=%t，禁止自动重放）；证据 %s：%w", attempted, dir, runErr)
		}
	}()
	b, err := phase3OpenFormEditor(s.target+"?debug=1", phase3FormReady)
	if err != nil {
		return err
	}
	defer b.Close()
	snap, err := phase3FormSnapshot(b)
	if err != nil {
		return err
	}
	if snap["canPublish"] != true {
		return errors.New("当前CLI测试提交要求管理权限，以便独立读取答卷核验")
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	body := map[string]any{"formRev": before["publishedRev"], "userFinger": str(me, "id"), "userName": str(me, "name"), "responseContent": content}
	for name, v := range map[string]any{"request.json": body, "publication-before.json": before, "responses-before.json": rows} {
		if err = phase3WriteEvidence(filepath.Join(dir, name), v); err != nil {
			return err
		}
	}
	current, err := s.formPublication()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, before) {
		return errors.New("准备期间发布版本或设置变化；未提交")
	}
	cookie, err := authCookie()
	if err != nil {
		return err
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	confirmationText := a.opt("--confirm-text")
	id := ""
	if confirmationText != "" {
		if err = s.formSubmitUI(b, form, answers, before, confirmationText, dir, &attempted); err != nil {
			return err
		}
	} else {
		attempted = true
		reply, e := a.requestPath(s.prefix+"forms/"+url.PathEscape(s.guid)+"/submit", "POST", body, nil, s.headers, cookie)
		if e != nil {
			return e
		}
		submitted, e := phase3FormResponseJSON(reply)
		if e != nil {
			return e
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
			return err
		}
		id = str(submitted, "submitId")
		if id == "" || prior[id] {
			return errors.New("未获得唯一新答卷ID；不得自动重试")
		}
	}
	after, n, readTotals, err := phase3WaitFormSubmission(context.Background(), s.formResultsContext, rows, total, id, time.Second)
	if e := phase3WriteEvidence(filepath.Join(dir, "readback-attempts.json"), map[string]any{"totals": readTotals, "indexLagMarker": -1, "mutationRequests": 1, "scope": "complete read-only result queries; no submit replay"}); e != nil {
		return e
	}
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "responses-after.json"), after); err != nil {
		return err
	}
	if n != total+1 {
		return errors.New("答卷总数未精确增加1")
	}
	if confirmationText != "" {
		for _, r := range after {
			if !prior[str(r, "submitId")] {
				if id != "" {
					return errors.New("UI提交产生多于一个新答卷")
				}
				id = str(r, "submitId")
			}
		}
		if id == "" {
			return errors.New("UI提交后无唯一新答卷")
		}
	}
	var found map[string]any
	for _, r := range after {
		if str(r, "submitId") == id {
			found = r
		}
	}
	if found == nil {
		return errors.New("独立分页读回未找到新答卷")
	}
	byID := map[string]map[string]any{}
	for _, r := range after {
		byID[str(r, "submitId")] = r
	}
	for _, r := range rows {
		if !reflect.DeepEqual(r, byID[str(r, "submitId")]) {
			return errors.New("提交后已有答卷发生变化或缺失")
		}
	}
	actual, err := phase3FormAnswers(found)
	if err != nil {
		return err
	}
	expectedJSON, _ := json.Marshal(content)
	actualJSON, _ := json.Marshal(actual)
	if string(expectedJSON) != string(actualJSON) {
		return errors.New("服务端保存答案与请求不一致")
	}
	result := map[string]any{"ok": true, "action": "form-submit", "url": s.target, "submitId": id, "total": n, "actionVerified": true, "mutationAttempted": true, "evidenceDir": dir, "scope": "one persisted response and exact answer readback; result-page text needs separate UI verification"}
	if confirmationText != "" {
		result["confirmationVerified"] = true
		result["scope"] = "real submit-button confirmation page plus one independently persisted response with exact answers"
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
