package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"time"
)

var phase3FormID = regexp.MustCompile(`^[A-Za-z0-9_-]{4,64}$`)

// Form requests deliberately expose a small typed mutation surface, not arbitrary
// editor methods. The expected complete draft is computed before any mutation.
func phase3FormExpected(before, req map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(before)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	qs, ok := out["questions"].([]any)
	if !ok {
		return nil, errors.New("草稿缺少 questions 数组")
	}
	if _, ok := out["rules"].([]any); !ok {
		return nil, errors.New("草稿缺少 rules 数组")
	}
	find := func(id string) int {
		for i, v := range qs {
			m, ok := v.(map[string]any)
			if ok && str(m, "guid") == id {
				return i
			}
		}
		return -1
	}
	index := func(key string, max int) (int, error) {
		n, ok := req[key].(float64)
		if !ok || n != math.Trunc(n) || n < 0 || n > float64(max) {
			return 0, fmt.Errorf("%s 必须是有效的零基整数索引", key)
		}
		return int(n), nil
	}
	switch str(req, "operation") {
	case "question-add":
		q, ok := req["question"].(map[string]any)
		if !ok {
			return nil, errors.New("缺少 question 对象")
		}
		if err = phase3ValidateQuestion(q); err != nil {
			return nil, err
		}
		if find(str(q, "guid")) >= 0 {
			return nil, errors.New("问题 guid 已存在")
		}
		i := len(qs)
		if _, ok := req["index"]; ok {
			i, err = index("index", len(qs))
			if err != nil {
				return nil, err
			}
		}
		next := append([]any{}, qs[:i]...)
		next = append(next, q)
		next = append(next, qs[i:]...)
		out["questions"] = next
	case "question-update":
		i := find(str(req, "guid"))
		if i < 0 {
			return nil, errors.New("目标问题不存在")
		}
		patch, ok := req["patch"].(map[string]any)
		if !ok || len(patch) == 0 {
			return nil, errors.New("缺少问题 patch")
		}
		q := qs[i].(map[string]any)
		for k, v := range patch {
			if k != "subject" && k != "description" && k != "required" && k != "question" {
				return nil, fmt.Errorf("不支持修改问题字段 %s", k)
			}
			q[k] = v
		}
		if err = phase3ValidateQuestion(q); err != nil {
			return nil, err
		}
		switch str(q, "type") {
		case "single_choice", "multiple_choice", "dropdown_list", "rate":
			q["question"].(map[string]any)["total"] = float64(0)
		}
	case "question-delete":
		id := str(req, "guid")
		i := find(id)
		if i < 0 {
			return nil, errors.New("目标问题不存在")
		}
		for _, key := range []string{"rules", "widgets"} {
			v, _ := json.Marshal(out[key])
			if strings.Contains(string(v), `"`+id+`"`) {
				return nil, errors.New("目标问题被规则或组件引用；请先调整引用")
			}
		}
		out["questions"] = append(append([]any{}, qs[:i]...), qs[i+1:]...)
	case "question-reorder":
		from, e := index("from", len(qs)-1)
		if e != nil {
			return nil, e
		}
		to, e := index("to", len(qs)-1)
		if e != nil {
			return nil, e
		}
		q := qs[from]
		next := append(append([]any{}, qs[:from]...), qs[from+1:]...)
		next = append(next, nil)
		copy(next[to+1:], next[to:])
		next[to] = q
		out["questions"] = next
	case "rule-add", "rule-update", "rule-delete":
		if err = phase3FormRuleExpected(out, req); err != nil {
			return nil, err
		}
	case "meta":
		patch, ok := req["patch"].(map[string]any)
		if !ok || len(patch) == 0 {
			return nil, errors.New("缺少表单 patch")
		}
		for k, v := range patch {
			switch k {
			case "title", "desc":
				s, ok := v.(string)
				if !ok || len(s) > 20000 || (k == "title" && strings.TrimSpace(s) == "") {
					return nil, errors.New("标题/描述无效")
				}
			case "confirmation":
				if err = phase3ValidateConfirmation(v); err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("不支持修改表单字段 %s", k)
			}
			out[k] = v
		}
	default:
		return nil, errors.New("不支持的表单编辑 operation")
	}
	for _, v := range out["rules"].([]any) {
		r, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("规则结构无效")
		}
		if err = phase3ValidateFormRule(out, r); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func phase3ValidateQuestion(q map[string]any) error {
	for k := range q {
		switch k {
		case "guid", "type", "required", "subject", "description", "question", "response", "tableFieldGuid":
		default:
			return fmt.Errorf("未知问题字段 %s", k)
		}
	}
	if !phase3FormID.MatchString(str(q, "guid")) {
		return errors.New("问题 guid 无效")
	}
	if s, ok := q["subject"].(string); !ok || strings.TrimSpace(s) == "" || len(s) > 20000 {
		return errors.New("问题标题不能为空或过长")
	}
	if _, ok := q["required"].(bool); !ok {
		return errors.New("问题 required 必须为布尔值")
	}
	if v, ok := q["description"]; ok {
		if _, ok := v.(string); !ok {
			return errors.New("description 必须是文本")
		}
	}
	body, ok := q["question"].(map[string]any)
	if !ok {
		return errors.New("question.question 必须为对象")
	}
	if v, ok := body["total"]; ok && v != float64(0) {
		return errors.New("当前仅支持非测验题型total=0")
	}
	switch str(q, "type") {
	case "text", "multiple_text":
		if len(body) > 0 {
			return errors.New("当前文本题仅支持默认题型配置")
		}
	case "rate":
		for k := range body {
			if k != "shape" && k != "sum" && k != "total" {
				return errors.New("未知评分题配置")
			}
		}
		n, ok := body["sum"].(float64)
		if !ok || n < 1 || n > 10 || n != math.Trunc(n) || body["shape"] != "star" || len(body) < 2 || len(body) > 3 {
			return errors.New("评分题仅支持 star，sum为1–10整数")
		}
	case "single_choice", "multiple_choice", "dropdown_list":
		for k := range body {
			if k != "options" && k != "limitCount" && k != "total" {
				return fmt.Errorf("暂不支持选项题配置 %s", k)
			}
		}
		if v, ok := body["limitCount"]; ok && v != false {
			return errors.New("当前仅支持无限制多选题")
		}
		opts, ok := body["options"].([]any)
		if !ok || len(opts) < 1 || len(opts) > 100 {
			return errors.New("选项数必须为 1–100")
		}
		seen := map[string]bool{}
		for _, v := range opts {
			m, ok := v.(map[string]any)
			if !ok || len(m) != 3 || str(m, "type") != "normal" || !phase3FormID.MatchString(str(m, "value")) || strings.TrimSpace(str(m, "label")) == "" || seen[str(m, "value")] {
				return errors.New("选项必须包含唯一 value、normal type 和非空 label")
			}
			seen[str(m, "value")] = true
		}
	default:
		return errors.New("当前支持 text/multiple_text/single_choice/multiple_choice/dropdown_list/rate 题型")
	}
	return nil
}
func phase3ValidateConfirmation(v any) error {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 2 || str(m, "type") != "custom" {
		return errors.New("结果页仅支持 custom 文本内容")
	}
	c, ok := m["confirmation"].(map[string]any)
	if !ok || len(c) != 2 {
		return errors.New("结果页 confirmation 必须有 showTitle/content")
	}
	if _, ok := c["showTitle"].(bool); !ok {
		return errors.New("结果页 showTitle 必须是布尔值")
	}
	s, ok := c["content"].(string)
	if !ok || len(s) > 20000 || strings.TrimSpace(s) == "" {
		return errors.New("结果页 content 无效")
	}
	return nil
}
func phase3VerifyForm(expected, actual map[string]any) error {
	// JSON normalizes map/number representation and does not ignore unrelated fields.
	e, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	a, err := json.Marshal(actual)
	if err != nil {
		return err
	}
	if string(e) != string(a) {
		return errors.New("重载后完整草稿与预期不一致；写入可能已生效，禁止自动重放")
	}
	return nil
}
func phase3FormSnapshot(b *phase3EditorBrowser) (map[string]any, error) {
	v, err := b.Eval(`(()=>{const e=window.logged?.edit;if(!e?.query?.getForm)return {ok:false,error:"表单编辑器未就绪"};return {guid:e.query.getGuid(),form:e.query.getForm(),rev:e.query.getRev()??null,sdkHasUnsavedUnreliable:e.query.hasUnsaved(),canEdit:e.query.permissions.canEditForm(),canPublish:e.query.permissions.canPublishForm(),publication:e.query.getPublishStatus(),presence:window.__shimoPhase3Presence?.()}})()`)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("表单未返回草稿对象")
	}
	return m, nil
}
func phase3FormScreenshot(b *phase3EditorBrowser, path string) error {
	b.nextID++
	r, err := b.ws.Call(b.nextID, "Page.captureScreenshot", map[string]any{"format": "png", "captureBeyondViewport": false}, 20*time.Second)
	if err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(str(r, "data"))
	if err != nil || len(data) < 1000 {
		return errors.New("编辑器截图为空或无效")
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	if !screenshotHasContent(path) {
		return fmt.Errorf("表单截图为空白或无法解码，不能作为验收物证；原图保留：%s；请等待真实表单内容显示后仅重新检查，不要重放写入", path)
	}
	return nil
}
func phase3FormReload(b *phase3EditorBrowser) (map[string]any, error) {
	if _, err := b.Eval(`(()=>{window.__phase3OldFormContext=true;return true})()`); err != nil {
		return nil, err
	}
	b.nextID++
	if _, err := b.ws.Call(b.nextID, "Page.reload", map[string]any{"ignoreCache": true}, 10*time.Second); err != nil {
		return nil, err
	}
	// Wait for navigation before querying; otherwise the old execution context can
	// still answer and falsely masquerade as persistent readback.
	time.Sleep(2 * time.Second)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		fresh, freshErr := b.Eval(`!window.__phase3OldFormContext`)
		m, err := phase3FormSnapshot(b)
		if freshErr == nil && fresh == true && err == nil {
			return m, nil
		}
		time.Sleep(time.Second)
	}
	return nil, errors.New("重载后编辑器未就绪；不能确认持久化")
}
func phase3FormMutationJS(req, expected, before map[string]any) string {
	r, _ := json.Marshal(req)
	e, _ := json.Marshal(expected)
	prior, _ := json.Marshal(before)
	return `(async()=>{` + phase3PresenceGuardJS() + `const e=window.logged.edit;if(!e.query.permissions.canEditForm())return {ok:false,error:"没有表单编辑权限"};const prior=` + string(prior) + `;const stable=v=>v&&typeof v==="object"?(Array.isArray(v)?v.map(stable):Object.fromEntries(Object.keys(v).sort().map(k=>[k,stable(v[k])]))):v;if(JSON.stringify(stable(e.query.getForm()))!==JSON.stringify(stable(prior.form))||(e.query.getRev()??null)!==prior.rev)return {ok:false,error:"草稿在准备后发生变化，未执行写入"};let createdRuleGuid=null;const r=` + string(r) + `, expected=` + string(e) + `;switch(r.operation){case "question-add":e.command.appendQuestion(r.question,r.index);break;case "question-update":e.command.updateQuestion(expected.questions.find(q=>q.guid===r.guid));break;case "question-delete":e.command.deleteQuestion(r.guid);break;case "question-reorder":e.command.reorderQuestion(r.from,r.to);break;case "meta":e.command.updateForm(r.patch);break;case "rule-add":{const count=e.query.getForm().rules.length;e.command.appendRuleByType("conditional");const created=e.query.getForm().rules[count];if(!created?.guid)throw new Error("missing generated rule id");createdRuleGuid=created.guid;const rule=expected.rules.find(x=>x.guid==="phase3newRule");e.command.updateRule({...rule,guid:createdRuleGuid});break;}case "rule-update":e.command.updateRule(expected.rules.find(x=>x.guid===r.guid));break;case "rule-delete":e.command.deleteRule(r.guid);break;default:return {ok:false,error:"不支持的 operation"}}return {ok:true,submitted:true,createdRuleGuid}})()`
}
