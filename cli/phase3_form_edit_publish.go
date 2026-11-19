package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

func phase3FormSettingsPayload(req map[string]any, now time.Time) (map[string]any, error) {
	for k := range req {
		if k != "deadline" && k != "timezone" {
			return nil, fmt.Errorf("不支持设置字段 %s（不允许修改填写权限）", k)
		}
	}
	t, err := time.Parse(time.RFC3339, str(req, "deadline"))
	if err != nil || !t.After(now) {
		return nil, errors.New("deadline 必须是未来时间，使用含时区偏移的 RFC3339")
	}
	out := map[string]any{"deadline": t.Format(time.RFC3339)}
	if z, ok := req["timezone"]; ok {
		zone, ok := z.(string)
		if !ok {
			return nil, errors.New("timezone 无效")
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			return nil, errors.New("timezone 不是有效 IANA 时区")
		}
		_, want := t.In(loc).Zone()
		_, got := t.Zone()
		if want != got {
			return nil, errors.New("deadline 偏移与 timezone 不一致")
		}
		out["timezone"] = float64(want) / 3600
	}
	return out, nil
}

// The official form SDK edit.command.publishForm passes rev:1 for both normal
// publication and initialization. This is a publication protocol marker, not
// the collaboration OT revision. See official-publish-command-source.txt in
// the live acceptance evidence; verify the marker AND the full published body.
const phase3FormPublicationRevision = 1

var phase3PublicationFields = []string{"formId", "begin", "deadline", "delayed", "responseDurationLimit", "responseTimeLimitPerDay", "limitCount", "fillLimit", "sheetGuid", "spreadGuid", "tablesGuid", "tableGuid", "teamId", "submitterCondition", "assignedGroup", "showResponseScoreStatus", "timezone", "assignedSubmitters", "assignedDepartments", "channels", "isAnonymous", "collectUniqueUserInfo", "isQuiz", "showTotalScore", "showQuestionTotalScore", "responseUpdatable", "subscribeNotification", "reminder"}
var phase3PublicationAccessFields = []string{"submitterCondition", "assignedSubmitters", "assignedDepartments", "assignedGroup", "isAnonymous", "collectUniqueUserInfo", "teamId", "fillLimit", "subscribeNotification", "reminder"}

func phase3FormPublishPayload(before, form map[string]any, user any) (map[string]any, error) {
	if str(before, "status") != "unpublished" && str(before, "status") != "normal" && str(before, "status") != "stopped" {
		return nil, errors.New("未知原发布状态")
	}
	if before["isQuiz"] == true || before["isQuiz"] == float64(1) {
		return nil, errors.New("暂不支持测验发布，需要独立答案与评分校验")
	}
	qs, ok := form["questions"].([]any)
	if !ok || len(qs) == 0 || strings.TrimSpace(str(form, "title")) == "" {
		return nil, errors.New("发布要求非空标题和题目")
	}
	content, err := json.Marshal(form)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"status": "normal", "publishedContent": string(content), "insensitiveContent": string(content), "publishedRev": phase3FormPublicationRevision, "userId": user}
	for _, k := range phase3PublicationFields {
		if v, ok := before[k]; ok {
			out[k] = v
		}
	}
	// Publishing must never start notifications. Existing configured notification
	// subscriptions require a separate explicitly authorized publication path.
	if v := before["subscribeNotification"]; v != nil && v != "" {
		return nil, errors.New("表单配置了通知订阅；本命令未获发送通知授权")
	}
	if v := before["reminder"]; v != nil && v != "" {
		return nil, errors.New("表单配置了提醒；本命令未获发送提醒授权")
	}
	return out, nil
}
func phase3VerifyPublication(before, after, form map[string]any) error {
	revJSON, _ := json.Marshal(after["publishedRev"])
	if string(revJSON) != "1" {
		return errors.New("服务端publishedRev缺失或与官方发布协议标记1不一致")
	}

	if str(after, "status") != "normal" {
		return errors.New("服务端未处于 normal 发布状态")
	}
	for _, k := range phase3PublicationAccessFields {
		if !reflect.DeepEqual(before[k], after[k]) {
			return fmt.Errorf("发布前后 %s 变化，不能确认原授权范围", k)
		}
	}
	var got map[string]any
	if json.Unmarshal([]byte(str(after, "publishedContent")), &got) != nil {
		return errors.New("发布内容无法解析")
	}
	return phase3VerifyForm(form, got)
}

// The SDK updatePublishStatus endpoint returns void. A successful empty
// transport acknowledgement permits the mandatory independent GET; it never
// verifies the action itself. HTML/error text still follows strict JSON checks.
func phase3FormMutationAck(r *Response) (map[string]any, error) {
	if r != nil && (r.Status == 200 || r.Status == 204) && strings.TrimSpace(string(r.Raw)) == "" && r.Data == nil {
		return map[string]any{"httpStatus": r.Status, "emptyBody": true, "actionVerified": false, "requiresIndependentReadback": true}, nil
	}
	return phase3FormResponseJSON(r)
}

func (s *phase3SDKSession) formPublication() (map[string]any, error) {
	r, err := s.app.authRequestPath(s.prefix+"forms/"+url.PathEscape(s.guid)+"/publications", "GET", nil, nil, s.headers)
	if err != nil {
		return nil, err
	}
	return phase3FormResponseJSON(r)
}

// Tests replace only editor startup with a local fake CDP peer. Production keeps
// the same readiness, permission, publication and fresh-profile verification.
var phase3OpenFormPublishEditor = openPhase3EditorWhen

func (a *App) cmdPhase3FormPublish() (runErr error) {
	if len(a.Pos) != 1 {
		return errors.New("用法：phase3 <表单链接> --action form-settings|form-publish [--payload JSON] --out 证据目录")
	}
	action := a.opt("--action")
	var settings map[string]any
	var err error
	if action == "form-settings" {
		var req map[string]any
		if json.Unmarshal([]byte(a.opt("--payload")), &req) != nil {
			return errors.New("--payload 必须为 JSON 对象")
		}
		settings, err = phase3FormSettingsPayload(req, time.Now())
		if err != nil {
			return err
		}
	} else if action != "form-publish" {
		return errors.New("未知表单发布操作")
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	s, err := a.phase3SDK(target, "form")
	if err != nil {
		return err
	}
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	attempted := false
	stage := "preflight"
	defer func() {
		if runErr != nil {
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "action": action, "stage": stage, "mutationAttempted": attempted, "mayHaveApplied": attempted, "error": runErr.Error(), "url": s.target})
			runErr = fmt.Errorf("表单操作失败（mayHaveApplied=%t，禁止自动重放）；证据 %s：%w", attempted, dir, runErr)
		}
	}()
	b, err := phase3OpenFormPublishEditor(s.target+"?debug=1", phase3FormReady)
	if err != nil {
		return err
	}
	defer b.Close()
	snap, err := phase3FormSnapshot(b)
	if err != nil {
		return err
	}
	if snap["canPublish"] != true {
		return errors.New("没有发布或设置权限")
	}
	if str(snap, "guid") != s.guid {
		return errors.New("表单身份不匹配")
	}
	form, err := s.formDraft(b)
	if err != nil {
		return err
	}
	editorForm, _ := snap["form"].(map[string]any)
	if err = phase3VerifyForm(form, editorForm); err != nil {
		return err
	}
	before, err := s.formPublication()
	if err != nil {
		return err
	}
	user, err := b.Eval(`window.logged.edit.query.permissions.DEBUG_getContext().user.id`)
	if err != nil || user == nil {
		return errors.New("无法核实当前用户")
	}
	body := settings
	method := "PATCH"
	if action == "form-publish" {
		body, err = phase3FormPublishPayload(before, form, user)
		if err != nil {
			return err
		}
		method = "POST"
	} else {
		body["userId"] = user
	}
	for name, value := range map[string]any{"before-structure.json": snap, "server-before.json": form, "publication-before.json": before, "request.json": body} {
		if err = phase3WriteEvidence(filepath.Join(dir, name), value); err != nil {
			return err
		}
	}
	if err = phase3FormScreenshot(b, filepath.Join(dir, "before.png")); err != nil {
		return err
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	current, err := s.formPublication()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, current) {
		return errors.New("发布设置在准备期间发生变化；未写入")
	}
	server, err := s.formDraft(b)
	if err != nil {
		return err
	}
	if err = phase3VerifyForm(form, server); err != nil {
		return err
	}
	// Final browser guard verifies permission, current draft and online-list in a
	// single synchronous JS task immediately before the one REST mutation.
	cookie, err := authCookie()
	if err != nil {
		return err
	}
	prior, _ := json.Marshal(form)
	_, err = b.Eval(`(()=>{` + phase3PresenceGuardJS() + `const e=window.logged.edit;if(!e.query.permissions.canPublishForm())return {ok:false,error:"发布权限已变化"};const stable=v=>v&&typeof v==="object"?(Array.isArray(v)?v.map(stable):Object.fromEntries(Object.keys(v).sort().map(k=>[k,stable(v[k])]))):v;if(JSON.stringify(stable(e.query.getForm()))!==JSON.stringify(stable(` + string(prior) + `)))return {ok:false,error:"草稿已变化"};return {ok:true}})()`)
	if err != nil {
		return err
	}
	stage = "mutation"
	attempted = true
	r, err := a.requestPath(s.prefix+"forms/"+url.PathEscape(s.guid)+"/publications", method, body, nil, s.headers, cookie)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "mutation-transport.json"), map[string]any{"httpStatus": r.Status, "contentType": r.Header.Get("Content-Type"), "bodyBytes": len(r.Raw)}); err != nil {
		return err
	}
	reply, err := phase3FormMutationAck(r)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), reply); err != nil {
		return err
	}
	stage = "readback"
	after, err := s.formPublication()
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "publish-readback.json"), after); err != nil {
		return err
	}
	if action == "form-publish" {
		if err = phase3VerifyPublication(before, after, form); err != nil {
			return err
		}
	} else {
		for _, k := range phase3PublicationAccessFields {
			if !reflect.DeepEqual(before[k], after[k]) {
				return fmt.Errorf("设置后权限/通知字段 %s 意外变化", k)
			}
		}
		want, _ := time.Parse(time.RFC3339, str(settings, "deadline"))
		got, e := time.Parse(time.RFC3339, str(after, "deadline"))
		if e != nil || !got.Equal(want) {
			return errors.New("服务端截止时间与请求不一致")
		}
		if v, ok := settings["timezone"]; ok && after["timezone"] != v {
			return errors.New("服务端时区与请求不一致")
		}
	}
	stage = "fresh-profile"
	b.Close()
	b, err = phase3OpenFormPublishEditor(s.target+"?debug=1", phase3FormReady)
	if err != nil {
		return err
	}
	defer b.Close()
	fresh, err := phase3FormSnapshot(b)
	if err != nil {
		return err
	}
	freshForm, _ := fresh["form"].(map[string]any)
	if err = phase3VerifyForm(form, freshForm); err != nil {
		return err
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "after-structure.json"), fresh); err != nil {
		return err
	}
	if err = phase3FormScreenshot(b, filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "action": action, "url": s.target, "mutationAttempted": true, "actionVerified": true, "evidenceDir": dir, "scope": "verified server publication/settings and fresh editor draft; respondent behavior requires form-preview verification"}
	if action == "form-publish" {
		link, e := b.Eval(`window.logged.edit.query.getFillFormLink()`)
		if e != nil {
			return e
		}
		linkText, ok := link.(string)
		if !ok || linkText == "" {
			return errors.New("未获取真实填写链接")
		}
		result["respondentURL"] = linkText
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
