package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"time"
)

const phase3FormFillQueryJS = `window.logged?.["fill:query"]`
const phase3FormFillCommandJS = `window.logged?.["fill:command"]`

// A second tab in the same isolated browser keeps the editor's authoritative
// online-list connection alive until immediately before the real submit click.
func phase3FormFillTab(editor *phase3EditorBrowser, target string) (*phase3EditorBrowser, func(), error) {
	editor.nextID++
	created, err := editor.ws.Call(editor.nextID, "Target.createTarget", map[string]any{"url": target}, 15*time.Second)
	if err != nil {
		return nil, nil, err
	}
	id := str(created, "targetId")
	cleanup := func() {
		editor.nextID++
		_, _ = editor.ws.Call(editor.nextID, "Target.closeTarget", map[string]any{"targetId": id}, 5*time.Second)
	}
	endpoint, err := url.Parse(editor.debugURL)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	wsURL := "ws://" + endpoint.Host + "/devtools/page/" + id
	conn, err := dialWS(wsURL)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	fill := &phase3EditorBrowser{ws: conn, nextID: 1000, debugURL: wsURL}
	closeTab := func() { conn.Close(); cleanup() }
	if err = fill.WaitReady(`Boolean(`+phase3FormFillQueryJS+`?.getForm()?.questions?.some(q=>document.getElementById(q.guid)) && document.querySelector('button'))`, phase3EditorReadyTimeout); err != nil {
		err = fill.captureReadinessFailure(err)
		closeTab()
		return nil, nil, err
	}
	return fill, closeTab, nil
}

func phase3FormFillResponses(form, answers map[string]any) ([]any, error) {
	if _, err := phase3FormSubmissionAnswers(form, answers); err != nil {
		return nil, err
	}
	out := []any{}
	for _, v := range form["questions"].([]any) {
		q := v.(map[string]any)
		a, ok := answers[str(q, "guid")]
		if !ok {
			continue
		}
		var response any
		switch str(q, "type") {
		case "text", "multiple_text":
			response = map[string]any{"content": a}
		case "single_choice", "dropdown_list":
			response = map[string]any{"choice": map[string]any{"type": "normal", "value": a}}
		case "multiple_choice":
			choices := []any{}
			for _, x := range a.([]any) {
				choices = append(choices, map[string]any{"type": "normal", "value": x})
			}
			response = map[string]any{"choices": choices}
		case "rate":
			config := q["question"].(map[string]any)
			response = map[string]any{"value": a, "sum": config["sum"]}
		default:
			return nil, errors.New("不支持的填写页题型")
		}
		out = append(out, map[string]any{"guid": q["guid"], "response": response})
	}
	return out, nil
}

// Tests replace only creation of the second tab; filling, guards, the single
// submit click and confirmation verification remain the production path.
var phase3OpenFormFillTab = phase3FormFillTab

func (s *phase3SDKSession) formSubmitUI(editor *phase3EditorBrowser, form, answers, publication map[string]any, expectedText, dir string, attempted *bool) error {
	v, err := editor.Eval(`window.logged.edit.query.getFillFormLink()`)
	if err != nil {
		return err
	}
	raw, ok := v.(string)
	if !ok {
		return errors.New("SDK没有填写链接")
	}
	u, err := phase3FormNavigationURL(raw, s.app.Origin)
	if err != nil {
		return err
	}

	fill, closeTab, err := phase3OpenFormFillTab(editor, phase3FormDebugURL(u))
	if err != nil {
		return err
	}
	defer closeTab()
	responses, err := phase3FormFillResponses(form, answers)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(responses)
	guid, _ := json.Marshal(s.guid)
	_, err = fill.Eval(`(()=>{const f={query:` + phase3FormFillQueryJS + `,command:` + phase3FormFillCommandJS + `};if(f.query.getGuid()!==` + string(guid) + `||f.query.getEditingSubmission()||f.query.getPageStatus()!==0)return {ok:false,error:'不是本表单的新答卷填写页'};for(const r of ` + string(data) + `){const q=f.query.getForm().questions.find(q=>q.guid===r.guid);if(!q)return {ok:false,error:'填写题目不匹配'};f.command.updateResponse({...q,response:r.response})}return {ok:true}})()`)
	if err != nil {
		return err
	}
	if err = phase3FormScreenshot(fill, filepath.Join(dir, "filled-before-submit.png")); err != nil {
		return err
	}
	current, e := s.formPublication()
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(current, publication) {
		return errors.New("填写期间发布版本或设置变化，未点击提交")
	}
	if err = editor.CheckPresence(); err != nil {
		return err
	}
	// One real submit-button click runs the application's own check -> submit ->
	// confirmation transition. Never set the page status or manufacture result DOM.
	*attempted = true
	_, err = fill.Eval(`(()=>{const buttons=[...document.querySelectorAll('button')].filter(e=>e.innerText.trim()==='提交'&&e.getClientRects().length>0&&!e.disabled);if(buttons.length!==1)return {ok:false,error:'未定位唯一真实提交按钮'};buttons[0].click();return {ok:true}})()`)
	if err != nil {
		return err
	}
	text, _ := json.Marshal(expectedText)
	if err = fill.WaitReady(phase3FormFillQueryJS+`?.getPageStatus()===2 && document.body.innerText.includes(`+string(text)+`)`, 90*time.Second); err != nil {
		_ = phase3FormScreenshot(fill, filepath.Join(dir, "submit-ui-failure.png"))
		return fmt.Errorf("实际提交未观察到预期结果页；禁止重试：%w", err)
	}
	if err = phase3FormScreenshot(fill, filepath.Join(dir, "confirmation.png")); err != nil {
		return err
	}
	observation, err := fill.Eval(`({pageStatus:` + phase3FormFillQueryJS + `.getPageStatus(),text:document.body.innerText})`)
	if err != nil {
		return err
	}
	return phase3WriteEvidence(filepath.Join(dir, "confirmation.json"), observation)
}
