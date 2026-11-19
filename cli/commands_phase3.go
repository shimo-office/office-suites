package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var phase3MutationActions = map[string]bool{
	"sheet-update": true, "sheet-append": true, "sheet-insert": true,
	"sheet-delete": true, "sheet-format": true, "sheet-merge": true,
}

// Phase3 validates a request before any mutation, records both exports, and
// reports success only after proving the requested postcondition. No inference
// from HTTP status, metadata strings, or unrelated workbook styles is allowed.
func (a *App) cmdPhase3() (runErr error) {
	if len(a.Pos) != 1 && !(len(a.Pos) == 2 && a.Pos[0] == "artifact") {
		return errors.New("用法：phase3 <链接> --action <动作> --payload JSON --read-range 工作表!A1:D4 [--out 目录]；phase3 artifact <文件>")
	}
	if a.Pos[0] == "artifact" {
		return a.cmdPhase3Artifact()
	}
	action := strings.TrimSpace(a.opt("--action"))
	if action == "document-format" {
		return a.cmdPhase3Document()
	}
	if action == "document-table" {
		return a.cmdPhase3DocumentTable()
	}
	if action == "document-table-style" {
		return a.cmdPhase3DocumentTableStyle()
	}
	if action == "document-mention" {
		return a.cmdPhase3DocumentMention()
	}
	if action == "document-graphic" {
		return a.cmdPhase3DocumentGraphic()
	}
	if action == "document-paragraph" || action == "document-page" || action == "document-theme" {
		return a.cmdPhase3DocumentLayout()
	}
	if action == "document-replace" {
		return a.cmdPhase3DocumentReplace()
	}
	if action == "document-layout-inspect" {
		return a.cmdPhase3DocumentLayoutInspect()
	}
	if action == "document-goto" {
		return a.cmdPhase3DocumentGoto()
	}
	if action == "table-inspect" {
		return a.cmdPhase3TableRead()
	}
	if action == "table-create" || action == "table-duplicate" || action == "table-delete" || action == "table-chart-create" || action == "table-chart-style" || action == "table-reference-add" || action == "table-reference-set" {
		return a.cmdPhase3BaseEdit()
	}
	if action == "table-update" || action == "table-field-add" || action == "table-field-update" || action == "table-field-convert" || action == "table-field-delete" || action == "table-record-add" || action == "table-record-update" || action == "table-view-update" || action == "table-view-create" || action == "table-view-delete" {
		return a.cmdPhase3TableEdit()
	}
	if action == "form-inspect" || action == "form-results" {
		return a.cmdPhase3FormRead()
	}
	if action == "form-edit" {
		return a.cmdPhase3FormEdit()
	}
	if action == "form-draft" {
		return a.cmdPhase3FormDraft()
	}
	if action == "form-settings" || action == "form-publish" {
		return a.cmdPhase3FormPublish()
	}
	if action == "form-submit" {
		return a.cmdPhase3FormSubmit()
	}
	if action == "form-preview" || action == "form-respondent" {
		return a.cmdPhase3FormPreview()
	}
	if action == "slide-print-pdf" {
		return a.cmdPhase3SlidesPrint()
	}
	if action == "slide-title-animate" || action == "slide-show-settings" || action == "slide-theme" || action == "slide-media-insert" {
		return a.cmdPhase3Slides()
	}
	if action == "slide-inspect" || action == "slide-design-inspect" || action == "slide-add" || action == "slide-duplicate" || action == "slide-hide" || action == "slide-delete" || action == "slide-text-insert" || action == "slide-text-format" || action == "slide-layout" || action == "slide-replace-all" || action == "slide-shape-format" || action == "slide-placeholder-fill" || action == "slide-table-edit" {
		return a.cmdPhase3Slides()
	}
	if operation, ok := phase2ManualOnlyActions[action]; ok {
		return manualOnlyError(operation)
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := preparePhase3(action, payload, strings.TrimSpace(a.opt("--read-range")))
	if err != nil {
		return err
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !phase3SameOrigin(target, a.Origin) {
		return errors.New("目标链接与已配置 origin 不一致；未执行任何写入")
	}
	guid := guidFromURL(target)
	if guid == "" {
		return errors.New("目标链接没有有效文件 guid")
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	if str(info, "type") != "spreadsheet" || boolv(info, "isCloudFile") || boolv(info, "isFolder") {
		return errors.New("phase3 表格动作只接受在线 spreadsheet；未执行写入")
	}
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	mutationAttempted := false
	stage := "request-evidence"
	defer func() {
		if runErr != nil {
			original := runErr.Error()
			if mutationAttempted {
				runErr = fmt.Errorf("写入已提交或可能已提交；禁止自动重放；证据目录 %s：%w", dir, runErr)
			}
			if evidenceErr := phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{
				"ok": false, "mutationAttempted": mutationAttempted, "mayHaveApplied": mutationAttempted,
				"stage": stage, "originalError": original, "error": runErr.Error(),
			}); evidenceErr != nil {
				runErr = fmt.Errorf("%w；失败证据无法保存：%v", runErr, evidenceErr)
			}
		}
	}()
	requestRecord := map[string]any{"action": action, "url": target, "payload": spec.Payload, "readRange": spec.ReadRange}
	if err := phase3WriteEvidence(filepath.Join(dir, "request.json"), requestRecord); err != nil {
		return err
	}
	stage = "before-export"
	beforeArtifact, err := a.phase3Export(guid, filepath.Join(dir, "before"))
	if err != nil {
		return fmt.Errorf("前置导出失败，未执行写入：%w", err)
	}
	before, err := readArtifactSheet(beforeArtifact.Path, spec.Sheet)
	if err != nil {
		return err
	}
	if err := phase3ValidateSnapshot(before, &spec); err != nil {
		return err
	}
	// Fail closed on unknown readback schemas before a mutating request.
	stage = "before-readback"
	beforeRead, err := a.runPhase2HTTP(guid, phase2Actions["sheet-values"], []any{map[string]any{"range": spec.ReadRange}})
	if err != nil {
		return fmt.Errorf("前置读回失败，未执行写入：%w", err)
	}
	if err := phase3VerifyReadback(beforeRead, before, spec.ReadRange); err != nil {
		return fmt.Errorf("前置读回与物证不一致，未执行写入：%w", err)
	}
	if err := phase3WriteEvidence(filepath.Join(dir, "before-readback.json"), beforeRead); err != nil {
		return err
	}
	var submitted any
	stage = "mutation"
	mutationAttempted = true
	if action == "sheet-format" {
		submitted, err = a.runSheetFormat(guid, []any{spec.Payload})
	} else {
		submitted, err = a.runPhase2HTTP(guid, phase2Actions[action], []any{spec.Payload})
	}
	if err != nil {
		return fmt.Errorf("写请求失败：%w", err)
	}
	stage = "submission-evidence"
	if err := phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return fmt.Errorf("记录提交物证失败：%w", err)
	}
	result, err := a.finishPhase3(guid, target, dir, spec, before, beforeArtifact, submitted, &stage)
	if err != nil {
		return fmt.Errorf("写后验收失败：%w", err)
	}
	stage = "result-evidence"
	if err := phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return fmt.Errorf("验收结果保存失败：%w", err)
	}
	if a.JSON {
		return a.printJSON(result)
	}
	fmt.Printf("动作验收通过：%s\n证据：%s\n", action, dir)
	return nil
}

func (a *App) finishPhase3(guid, target, dir string, spec phase3Spec, before artifactSheetSnapshot, beforeArtifact ArtifactEvidence, submitted any, stage *string) (map[string]any, error) {
	*stage = "after-readback"
	readback, err := a.runPhase2HTTP(guid, phase2Actions["sheet-values"], []any{map[string]any{"range": spec.ReadRange}})
	if err != nil {
		return nil, err
	}
	if err := phase3WriteEvidence(filepath.Join(dir, "after-readback.json"), readback); err != nil {
		return nil, err
	}
	*stage = "after-export"
	artifact, err := a.phase3Export(guid, filepath.Join(dir, "after"))
	if err != nil {
		return nil, err
	}
	*stage = "artifact-proof"
	artifact, err = verifyArtifactExpected(artifact.Path, "xlsx", spec.Expected)
	if err != nil {
		return nil, err
	}
	after, err := readArtifactSheet(artifact.Path, spec.Sheet)
	if err != nil {
		return nil, err
	}
	*stage = "readback-proof"
	if err := phase3VerifyReadback(readback, after, spec.ReadRange); err != nil {
		return nil, err
	}
	if err := phase3VerifyRequestedReadback(readback, after, spec); err != nil {
		return nil, err
	}
	*stage = "structural-proof"
	if err := phase3VerifyStructural(before, after, spec); err != nil {
		return nil, err
	}
	result := map[string]any{"ok": true, "action": spec.Action, "method": phase2Actions[spec.Action], "url": target,
		"result": submitted, "readback": readback, "beforeArtifact": beforeArtifact, "artifact": artifact, "evidenceDir": dir,
		"verified": true, "verifyWith": phase3ProofSummary(spec), "verificationScope": "requested spreadsheet action only",
		"numericExportNote": "API 数值类型单独核对；当前 XLSX 导出会将数值保存为文本，不宣称 XLSX 数值类型保真"}
	if cliBrowserVerification(a) {
		*stage = "screenshot-proof"
		shot := filepath.Join(dir, "after.png")
		if _, err := runPhase2Browser(target, "browser.verify", nil, shot); err != nil {
			return nil, err
		}
		result["screenshot"] = shot
	}
	return result, nil
}

func phase3EvidenceDir(parent string) (string, error) {
	if parent == "" {
		return os.MkdirTemp("", "shimo-phase3-")
	}
	abs, err := filepath.Abs(expandHome(parent))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0700); err != nil {
		return "", err
	}
	return os.MkdirTemp(abs, "phase3-")
}

func (a *App) phase3Export(guid, dir string) (ArtifactEvidence, error) {
	info, err := a.fileInfo(guid)
	if err != nil {
		return ArtifactEvidence{}, err
	}
	if str(info, "type") != "spreadsheet" || boolv(info, "isCloudFile") {
		return ArtifactEvidence{}, errors.New("目标类型不再是在线表格")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ArtifactEvidence{}, err
	}
	got, err := a.downloadOne(withGUID(info, guid), dir)
	if err != nil {
		return ArtifactEvidence{}, err
	}
	if str(got, "exportType") != "xlsx" || str(got, "file") == "" {
		return ArtifactEvidence{}, errors.New("未取得 XLSX 导出文件")
	}
	return verifyArtifact(str(got, "file"), "xlsx", nil)
}

func (a *App) cmdPhase3Artifact() error {
	if len(a.Pos) != 2 {
		return errors.New("用法：phase3 artifact <文件.docx|xlsx|pptx> [--kind 类型]")
	}
	artifact, err := verifyArtifact(expandHome(a.Pos[1]), a.opt("--kind"), nil)
	if err != nil {
		return err
	}
	result := map[string]any{"ok": true, "method": "local:artifact-inspect", "artifact": artifact,
		"verificationScope": "package structure only", "actionVerified": false}
	if a.JSON {
		return a.printJSON(result)
	}
	fmt.Printf("文件结构检查通过（不代表线上动作验收）：%s\n", artifact.Path)
	return nil
}
