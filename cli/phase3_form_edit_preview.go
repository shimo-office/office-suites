package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"time"
)

func phase3FormLinkEvidence(raw string) map[string]any {
	u, err := url.Parse(raw)
	if err != nil {
		return map[string]any{"valid": false}
	}
	keys := []string{}
	for k := range u.Query() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return map[string]any{"url": u.String(), "redactedQueryKeys": keys}
}

// The official unauthenticated SDK selects previews via path.startsWith(
// "/preview?t="). Re-encoding Query sorts debug before t and produces a blank
// page. Keep the signed route byte order; append debug only at the end.
func phase3FormDebugURL(u *url.URL) string {
	v := *u
	if _, ok := v.Query()["debug"]; !ok {
		if v.RawQuery != "" {
			v.RawQuery += "&"
		}
		v.RawQuery += "debug=1"
	}
	return v.String()
}

func phase3FormNavigationURL(raw, origin string) (*url.URL, error) {
	base, err := url.Parse(origin)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	u = base.ResolveReference(u)
	check := *u
	check.RawQuery = ""
	check.Fragment = ""
	if u.User != nil || !phase3SameOrigin(check.String(), origin) {
		return nil, errors.New("SDK返回非本origin链接，未传递登录态")
	}
	return u, nil
}

func phase3FormPreviewCases(form, req map[string]any) (string, []map[string]any, error) {
	id := str(req, "trigger")
	var trigger map[string]any
	known := map[string]bool{}
	qs, _ := form["questions"].([]any)
	for _, v := range qs {
		q, _ := v.(map[string]any)
		known[str(q, "guid")] = true
		if str(q, "guid") == id {
			trigger = q
		}
	}
	if trigger == nil || str(trigger, "type") != "single_choice" {
		return "", nil, errors.New("预览触发题必须为现存单选题")
	}
	config, _ := trigger["question"].(map[string]any)
	options, _ := config["options"].([]any)
	valid := map[string]bool{}
	for _, v := range options {
		o, _ := v.(map[string]any)
		if str(o, "type") == "normal" {
			valid[str(o, "value")] = true
		}
	}
	cases, ok := req["branches"].([]any)
	if !ok || len(cases) < 1 || len(cases) > 10 {
		return "", nil, errors.New("branches必须为1–10个明确分支断言")
	}
	out := []map[string]any{}
	for _, v := range cases {
		c, ok := v.(map[string]any)
		if !ok || !valid[str(c, "option")] {
			return "", nil, errors.New("分支选项无效")
		}
		seen := map[string]bool{}
		count := 0
		for _, key := range []string{"visible", "hidden"} {
			ids, ok := c[key].([]any)
			if !ok {
				return "", nil, errors.New("分支必须包含visible/hidden数组")
			}
			for _, v := range ids {
				x, ok := v.(string)
				if !ok || !known[x] || seen[x] {
					return "", nil, errors.New("分支目标题不存在或重复")
				}
				seen[x] = true
				count++
			}
		}
		if count == 0 {
			return "", nil, errors.New("分支没有任何显示/隐藏断言")
		}
		if v, exists := c["sequence"]; exists {
			ids, ok := v.([]any)
			if !ok || len(ids) == 0 {
				return "", nil, errors.New("sequence必须是非空题目GUID数组")
			}
			seq := map[string]bool{}
			for _, v := range ids {
				id, ok := v.(string)
				if !ok || !known[id] || seq[id] {
					return "", nil, errors.New("sequence题目不存在或重复")
				}
				seq[id] = true
			}
			for _, v := range c["visible"].([]any) {
				if !seq[v.(string)] {
					return "", nil, errors.New("sequence缺少预期可见题")
				}
			}
			for _, v := range c["hidden"].([]any) {
				if seq[v.(string)] {
					return "", nil, errors.New("sequence包含预期隐藏题")
				}
			}
		}
		out = append(out, c)
	}
	return id, out, nil
}
func (a *App) cmdPhase3FormPreview() (runErr error) {
	if len(a.Pos) != 1 {
		return errors.New("用法：phase3 <表单链接> --action form-preview|form-respondent --payload '{\"trigger\":\"题目GUID\",\"branches\":[{\"option\":\"选项value\",\"visible\":[],\"hidden\":[]}]}' --out 目录")
	}
	var req map[string]any
	if json.Unmarshal([]byte(a.opt("--payload")), &req) != nil {
		return errors.New("payload必须为JSON对象")
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	s, err := a.phase3SDK(target, "form")
	if err != nil {
		return err
	}
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	var b *phase3EditorBrowser
	defer func() {
		if runErr != nil {
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": false, "responseSubmitted": false, "error": runErr.Error()})
			if b != nil {
				diagnostic, _ := b.Eval(`({path:location.pathname,readyState:document.readyState,title:document.title,body:document.body?.innerText.slice(0,3000),loggedKeys:Object.keys(window.logged||{}),scripts:[...document.scripts].map(s=>{try{return new URL(s.src).pathname}catch{return "inline"}}),errors:window.__phase3FormPageErrors||[]})`)
				_ = phase3WriteEvidence(filepath.Join(dir, "page-diagnostic.json"), diagnostic)
				_ = phase3FormScreenshot(b, filepath.Join(dir, "failure.png"))
			}
			runErr = fmt.Errorf("表单预览验证失败；证据 %s：%w", dir, runErr)
		}
		if b != nil {
			b.Close()
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), req); err != nil {
		return err
	}
	b, err = phase3OpenFormEditor(s.target+"?debug=1", phase3FormReady)
	if err != nil {
		return err
	}
	form, err := s.formDraft(b)
	if err != nil {
		return err
	}
	action := a.opt("--action")
	linkMethod := "getPreviewFormLink"
	if action == "form-respondent" {
		publication, err := s.formPublication()
		if err != nil {
			return err
		}
		if str(publication, "status") != "normal" {
			return errors.New("表单未开放收集")
		}
		if json.Unmarshal([]byte(str(publication, "publishedContent")), &form) != nil {
			return errors.New("已发布表单内容无效")
		}
		linkMethod = "getFillFormLink"
	} else if action != "form-preview" {
		return errors.New("未知预览动作")
	}
	trigger, cases, err := phase3FormPreviewCases(form, req)
	if err != nil {
		return err
	}
	v, err := b.Eval(`window.logged.edit.query.` + linkMethod + `()`)
	if err != nil {
		return err
	}
	link, ok := v.(string)
	if !ok {
		return errors.New("SDK未返回真实预览/填写链接")
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "sdk-navigation.json"), map[string]any{"route": phase3FormLinkEvidence(link), "kind": action}); err != nil {
		return err
	}
	u, err := phase3FormNavigationURL(link, a.Origin)
	if err != nil {
		return err
	}

	for name, value := range map[string]any{"form.json": form, "navigation.json": map[string]any{"route": phase3FormLinkEvidence(link), "kind": action}} {
		if err = phase3WriteEvidence(filepath.Join(dir, name), value); err != nil {
			return err
		}
	}
	b.nextID++
	if _, err = b.ws.Call(b.nextID, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": `window.__phase3FormPageErrors=[];addEventListener('error',e=>window.__phase3FormPageErrors.push({type:'error',message:String(e.message||'resource error').replace(/https?:\/\/[^\s]+/g,'[url]').slice(0,400)}));addEventListener('unhandledrejection',e=>window.__phase3FormPageErrors.push({type:'rejection',message:String(e.reason?.message||e.reason).replace(/https?:\/\/[^\s]+/g,'[url]').slice(0,400)}));`}, 10*time.Second); err != nil {
		return err
	}
	b.nextID++
	if _, err = b.ws.Call(b.nextID, "Page.navigate", map[string]any{"url": phase3FormDebugURL(u)}, 15*time.Second); err != nil {
		return err
	}
	idJSON, _ := json.Marshal(trigger + "_" + str(cases[0], "option"))
	if err = b.WaitReady(`Boolean(document.getElementById(`+string(idJSON)+`))`, phase3EditorReadyTimeout); err != nil {
		return err
	}
	pageText, e := b.Eval(`({title:document.title,body:document.body.innerText})`)
	if e != nil {
		return e
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "page-text.json"), pageText); err != nil {
		return err
	}
	if action == "form-respondent" {
		expected, _ := json.Marshal(map[string]any{"title": form["title"], "desc": form["desc"], "guid": s.guid})
		observed, e := b.Eval(`(()=>{const q=` + phase3FormFillQueryJS + `;const expected=` + string(expected) + `;const f=q?.getForm();if(!f||q.getGuid()!==expected.guid||f.title!==expected.title||f.desc!==expected.desc)return {ok:false,error:'实际填写页与发布标题/描述/文件身份不一致'};return {guid:q.getGuid(),rev:q.getRev(),title:f.title,desc:f.desc}})()`)
		if e != nil {
			return e
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "respondent-loaded.json"), observed); err != nil {
			return err
		}
	}
	if err = phase3FormScreenshot(b, filepath.Join(dir, "preview.png")); err != nil {
		return err
	}
	knownIDs := []any{}
	for _, v := range form["questions"].([]any) {
		knownIDs = append(knownIDs, v.(map[string]any)["guid"])
	}
	results := []any{}
	for i, c := range cases {
		spec, _ := json.Marshal(map[string]any{"known": knownIDs, "trigger": trigger, "guid": s.guid, "title": form["title"], "branch": c, "respondent": action == "form-respondent"})
		observationJS := `(` + phase3FormBranchObservationJS + `)(` + string(spec) + `)`
		qid, _ := json.Marshal(trigger + "_" + str(c, "option"))
		_, err = b.Eval(`(()=>{const input=document.getElementById(` + string(qid) + `);if(!input||input.disabled)return {ok:false,error:"真实单选控件不可操作"};input.scrollIntoView({block:'center'});if(!input.checked)input.click();return {ok:true}})()`)
		if err != nil {
			return err
		}
		if err = b.WaitReady(observationJS+`.verified`, 30*time.Second); err != nil {
			state, _ := b.Eval(observationJS)
			_ = phase3WriteEvidence(filepath.Join(dir, fmt.Sprintf("branch-%02d-failure-state.json", i+1)), state)
			return err
		}
		observed, err := b.Eval(observationJS)
		if err != nil {
			return err
		}
		state, ok := observed.(map[string]any)
		if !ok || state["verified"] != true || state["selectionVerified"] != true {
			_ = phase3WriteEvidence(filepath.Join(dir, fmt.Sprintf("branch-%02d-failure-state.json", i+1)), observed)
			return errors.New("最终分支快照的原生选项、DOM或题序未通过核验")
		}
		if err = phase3WriteEvidence(filepath.Join(dir, fmt.Sprintf("branch-%02d.json", i+1)), observed); err != nil {
			return err
		}
		if err = phase3FormScreenshot(b, filepath.Join(dir, fmt.Sprintf("branch-%02d.png", i+1))); err != nil {
			return err
		}
		results = append(results, observed)
	}
	result := map[string]any{"ok": true, "action": action, "actionVerified": true, "route": phase3FormLinkEvidence(link), "branches": results, "responseSubmitted": false, "mutationAttempted": false, "evidenceDir": dir, "verification": "real preview/respondent DOM controls and per-question GUID visibility"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
