package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

func phase3FormRequests(payload string) ([]map[string]any, error) {
	var req map[string]any
	if json.Unmarshal([]byte(payload), &req) != nil || req == nil {
		return nil, errors.New("--payload 必须是 JSON 对象")
	}
	if v, ok := req["operations"]; ok {
		if len(req) != 1 {
			return nil, errors.New("批量payload仅接受operations")
		}
		list, ok := v.([]any)
		if !ok || len(list) == 0 || len(list) > 20 {
			return nil, errors.New("operations必须包含1–20个动作")
		}
		out := []map[string]any{}
		for _, v := range list {
			m, ok := v.(map[string]any)
			if !ok || str(m, "operation") == "" {
				return nil, errors.New("批量动作必须是typed operation对象")
			}
			out = append(out, m)
		}
		return out, nil
	}
	return []map[string]any{req}, nil
}

func (a *App) cmdPhase3FormEdit() (runErr error) {
	if len(a.Pos) != 1 {
		return errors.New("用法：phase3 <表单链接> --action form-edit --payload JSON --out 证据目录")
	}
	requests, err := phase3FormRequests(a.opt("--payload"))
	if err != nil {
		return err
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
	completed := []any{}
	currentStep := 0
	currentMayHaveApplied := false
	stage := "preflight"
	defer func() {
		if runErr != nil {
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "action": "form-edit", "stage": stage, "completedSteps": completed, "currentStep": currentStep, "currentMayHaveApplied": currentMayHaveApplied, "mayHaveApplied": currentMayHaveApplied || len(completed) > 0, "error": runErr.Error(), "url": s.target})
			runErr = &commandError{Message: fmt.Sprintf("表单编辑失败（已完成%d步，当前步mayHaveApplied=%t，禁止重放整个批次）；证据 %s：%v", len(completed), currentMayHaveApplied, dir, runErr), Result: map[string]any{"stage": stage, "completedSteps": completed, "currentStep": currentStep, "currentMayHaveApplied": currentMayHaveApplied, "mayHaveApplied": currentMayHaveApplied || len(completed) > 0, "evidenceDir": dir}, NoReplay: currentMayHaveApplied || len(completed) > 0}
		}
	}()
	b, err := phase3OpenFormEditor(s.target+"?debug=1", phase3FormReady)
	if err != nil {
		return err
	}
	defer func() {
		if b != nil {
			b.Close()
		}
	}()
	if err = phase3FormShowQuestions(b); err != nil {
		return err
	}
	snap, err := phase3FormSnapshot(b)
	if err != nil {
		return err
	}
	form, ok := snap["form"].(map[string]any)
	if !ok || str(snap, "guid") != s.guid {
		return errors.New("草稿目标身份不匹配")
	}
	if snap["canEdit"] != true {
		return errors.New("没有表单编辑权限")
	}
	// Validate the complete requested sequence before the first write. A generated
	// rule ID may only be referenced by a later, separate invocation.
	planned := form
	for i, r := range requests {
		planned, err = phase3FormExpected(planned, r)
		if err != nil {
			return fmt.Errorf("第%d步预检：%w", i+1, err)
		}
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), requests); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "before-structure.json"), snap); err != nil {
		return err
	}
	// File title and form title are one native product setting. Capture the
	// metadata baseline before the first draft write for a concurrent-name guard.
	titleRequested := false
	for _, request := range requests {
		patch, _ := request["patch"].(map[string]any)
		if str(request, "operation") == "meta" && patch["title"] != nil {
			titleRequested = true
		}
	}
	baselineName := ""
	if titleRequested {
		info, e := a.fileInfo(s.guid)
		if e != nil {
			return e
		}
		if str(info, "type") != "form" {
			return errors.New("表单标题同步目标不是在线表单；未写入")
		}
		// A missing name is not a reason to drop the whole batch: the title is
		// still written, but the file-name half of the contract is reported as
		// unverified instead of being silently assumed.
		baselineName = str(info, "name")
	}
	var expected map[string]any
	for i, r := range requests {
		currentStep = i + 1
		currentMayHaveApplied = false
		stage = "operation"
		stepDir := dir
		if len(requests) > 1 {
			stepDir = filepath.Join(dir, fmt.Sprintf("step-%02d", i+1))
			if err = os.Mkdir(stepDir, 0700); err != nil {
				return err
			}
		}
		expected, currentMayHaveApplied, err = phase3FormRunOne(b, s, stepDir, r)
		if err != nil {
			return err
		}
		item := map[string]any{"step": i + 1, "operation": r["operation"], "serverVerified": true, "evidenceDir": stepDir}
		completed = append(completed, item)
		if err = phase3WriteEvidence(filepath.Join(dir, "progress.json"), map[string]any{"completedSteps": completed, "freshProfileVerified": false}); err != nil {
			return err
		}
	}
	var titleSync map[string]any
	if titleRequested {
		stage = "file-title-sync"
		titleSync, err = a.phase3SyncFormTitle(s.guid, baselineName, str(expected, "title"))
		if e := phase3WriteEvidence(filepath.Join(dir, "title-sync.json"), titleSync); e != nil {
			return e
		}
		if err != nil {
			return err
		}
	}
	stage = "fresh-profile"
	b.Close()
	b, err = phase3OpenFormEditor(s.target+"?debug=1", phase3FormReady)
	if err != nil {
		return err
	}
	if err = phase3FormShowQuestions(b); err != nil {
		return err
	}
	after, err := phase3FormSnapshot(b)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "after-structure.json"), after); err != nil {
		return err
	}
	if str(after, "guid") != s.guid {
		return errors.New("重新打开后GUID不一致")
	}
	got, _ := after["form"].(map[string]any)
	if err = phase3VerifyForm(expected, got); err != nil {
		return err
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	if err = phase3FormScreenshot(b, filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "progress.json"), map[string]any{"completedSteps": completed, "freshProfileVerified": true}); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "action": "form-edit", "url": s.target, "actionVerified": true, "mutationAttempted": true, "completedSteps": completed, "verification": "each operation verified against signed server draft; final complete draft verified in fresh Chrome profile", "evidenceDir": dir, "scope": "requested draft mutations only; publication and respondent branch behavior require separate verification"}
	if titleSync != nil {
		result["titleSync"] = titleSync
	}
	if len(requests) == 1 {
		result["operation"] = requests[0]["operation"]
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}

func phase3FormRunOne(b *phase3EditorBrowser, s *phase3SDKSession, dir string, req map[string]any) (expected map[string]any, attempted bool, runErr error) {
	stage := "preflight"
	defer func() {
		if runErr != nil {
			_ = phase3WriteEvidence(filepath.Join(dir, "step-failure.json"), map[string]any{"ok": false, "stage": stage, "mutationAttempted": attempted, "mayHaveApplied": attempted, "error": runErr.Error(), "url": s.target})
		}
	}()
	before, err := phase3FormSnapshot(b)
	if err != nil {
		return nil, false, err
	}
	form, ok := before["form"].(map[string]any)
	if !ok || str(before, "guid") != s.guid {
		return nil, false, errors.New("草稿身份不匹配")
	}
	if before["canEdit"] != true {
		return nil, false, errors.New("没有编辑权限")
	}
	before, serverBefore, err := phase3FormSyncedBefore(b, s, dir, before)
	if err != nil {
		return nil, false, err
	}
	expected, err = phase3FormExpected(form, req)
	if err != nil {
		return nil, false, err
	}
	if reflect.DeepEqual(expected, form) {
		return nil, false, errors.New("请求不改变草稿，不能计为修改成功")
	}
	for name, v := range map[string]any{"request.json": req, "before-structure.json": before, "server-before.json": serverBefore, "expected.json": expected} {
		if err = phase3WriteEvidence(filepath.Join(dir, name), v); err != nil {
			return nil, false, err
		}
	}
	if err = phase3FormScreenshot(b, filepath.Join(dir, "before.png")); err != nil {
		return nil, false, err
	}
	if err = b.CheckPresence(); err != nil {
		return nil, false, err
	}
	stage = "mutation"
	attempted = true
	submitted, err := b.Eval(phase3FormMutationJS(req, expected, before))
	if err != nil {
		return nil, true, err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return nil, true, err
	}
	if str(req, "operation") == "rule-add" {
		m, _ := submitted.(map[string]any)
		id := str(m, "createdRuleGuid")
		if !phase3FormID.MatchString(id) {
			return nil, true, errors.New("规则生成ID无效")
		}
		for _, v := range form["rules"].([]any) {
			if str(v.(map[string]any), "guid") == id {
				return nil, true, errors.New("规则生成ID重复")
			}
		}
		for _, v := range expected["rules"].([]any) {
			r := v.(map[string]any)
			if r["guid"] == phase3NewRuleID {
				r["guid"] = id
			}
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "expected.json"), expected); err != nil {
			return nil, true, err
		}
	}
	stage = "server-save"
	deadline := time.Now().Add(60 * time.Second)
	saved := false
	var lastServer map[string]any
	var lastReadError string
	for time.Now().Before(deadline) {
		got, e := s.formDraft(b)
		if e != nil {
			lastReadError = e.Error()
		} else {
			lastServer = got
			lastReadError = ""
		}
		if e == nil && phase3VerifyForm(expected, got) == nil {
			if err = phase3WriteEvidence(filepath.Join(dir, "server-after.json"), got); err != nil {
				return nil, true, err
			}
			saved = true
			break
		}
		time.Sleep(time.Second)
	}
	if !saved {
		_ = phase3WriteEvidence(filepath.Join(dir, "server-last.json"), map[string]any{"form": lastServer, "error": lastReadError})
		return nil, true, errors.New("服务端完整草稿保存验证超时；SDK hasUnsaved恒false未作为判据")
	}
	stage = "rendered-readback"
	after, err := phase3FormSnapshot(b)
	if err != nil {
		return nil, true, err
	}
	got, _ := after["form"].(map[string]any)
	if err = phase3VerifyForm(expected, got); err != nil {
		return nil, true, err
	}
	if err = b.CheckPresence(); err != nil {
		return nil, true, err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "after-structure.json"), after); err != nil {
		return nil, true, err
	}
	if err = phase3FormScreenshot(b, filepath.Join(dir, "after.png")); err != nil {
		return nil, true, err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "step-result.json"), map[string]any{"ok": true, "operation": req["operation"], "serverVerified": true, "mutationAttempted": true, "evidenceDir": dir}); err != nil {
		return nil, true, err
	}
	return expected, true, nil
}

// On first open the native editor initializes the draft title from the file
// name. Its asynchronous save may lag behind the rendered editor. Wait only
// for the unchanged browser baseline to reach the server; never ignore a diff
// or resubmit an edit. The mutation still checks this returned snapshot/rev.
func phase3FormSyncedBefore(b *phase3EditorBrowser, s *phase3SDKSession, dir string, before map[string]any) (map[string]any, map[string]any, error) {
	baseline := before["form"].(map[string]any)
	for attempt := 0; attempt < 6; attempt++ {
		server, err := s.formDraft(b)
		if err != nil {
			return nil, nil, err
		}
		if phase3VerifyForm(baseline, server) == nil {
			return before, server, nil
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "preflight-draft-mismatch.json"), map[string]any{"browser": before, "server": server, "readAttempt": attempt + 1, "mutationAttempted": false}); err != nil {
			return nil, nil, err
		}
		if attempt == 5 {
			break
		}
		if err = b.CheckPresence(); err != nil {
			return nil, nil, err
		}
		time.Sleep(time.Second)
		current, err := phase3FormSnapshot(b)
		if err != nil {
			return nil, nil, err
		}
		form, ok := current["form"].(map[string]any)
		if !ok || str(current, "guid") != s.guid || current["canEdit"] != true || phase3VerifyForm(baseline, form) != nil {
			return nil, nil, errors.New("等待草稿保存时目标、权限或正文发生变化；未写入")
		}
		before = current
	}
	return nil, nil, errors.New("等待保存后浏览器草稿与服务端快照仍不一致；未写入；差异见 preflight-draft-mismatch.json")
}
