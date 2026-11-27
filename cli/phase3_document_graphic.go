package main

import (
	"fmt"
	"path/filepath"
	"time"
)

func (a *App) cmdPhase3DocumentGraphic() (runErr error) {
	if len(a.Pos) != 1 {
		return fmt.Errorf("document-graphic 需要一个文稿链接")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareDocumentGraphic(payload)
	if err != nil {
		return err
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !phase3SameOrigin(target, a.Origin) {
		return fmt.Errorf("目标origin不符")
	}
	guid := guidFromURL(target)
	if guid == "" {
		return fmt.Errorf("缺少文稿GUID")
	}
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	attempted := false
	defer func() {
		if runErr != nil {
			if attempted {
				runErr = fmt.Errorf("图形写入已尝试，可能已生效；验收未通过，禁止自动重放（证据%s）：%w", dir, runErr)
			} else {
				runErr = fmt.Errorf("未执行图形写入（证据%s）：%w", dir, runErr)
			}
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": attempted, "error": runErr.Error()})
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"action": "document-graphic", "target": target, "spec": spec, "version": version, "at": time.Now().UTC()}); err != nil {
		return err
	}
	beforeArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "before"), "docx")
	if err != nil {
		return err
	}
	before, err := readDocumentGraphic(beforeArtifact.Path)
	if err != nil {
		return err
	}
	baseline, err := documentGraphicBaseline(before.Root, spec)
	if err != nil {
		return err
	}
	if _, err = documentGraphicNormalize(before.Root, spec); err != nil {
		return fmt.Errorf("图形基线不支持当前请求: %w", err)
	}
	if verifyDocumentGraphic(beforeArtifact.Path, beforeArtifact.Path, spec) == nil {
		return fmt.Errorf("图形已满足请求，不把未修改状态计为新验收")
	}
	b, err := phase3OpenDocumentEditor(target, documentReadyJS)
	if err != nil {
		return err
	}
	defer b.Close()
	if err = b.WaitDocumentEditable(); err != nil {
		return err
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	preflight, err := b.EvalReadOnly(documentGraphicRuntimeScript(spec, baseline, false, spec.Preset != 0, ""))
	if err != nil {
		return err
	}
	pm, ok := preflight.(map[string]any)
	if !ok || str(pm, "objectId") == "" {
		return fmt.Errorf("未取得真实图形对象ID")
	}
	objectID := str(pm, "objectId")
	if err = phase3WriteEvidence(filepath.Join(dir, "before-native.json"), preflight); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return err
	}
	attempted = true
	submitted, err := b.Eval(documentGraphicRuntimeJS(spec, baseline, true, objectID))
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return err
	}
	if err = b.waitDocumentSaved(); err != nil {
		return err
	}
	afterArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "after"), "docx")
	if err != nil {
		return err
	}
	warnings, err := verifyDocumentGraphicWithWarnings(beforeArtifact.Path, afterArtifact.Path, spec)
	if err != nil {
		return err
	}
	afterNative, err := b.EvalReadOnly(documentGraphicRuntimeJS(spec, baseline, false, objectID))
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "after-native.json"), afterNative); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "after.png")); err != nil {
		return err
	}
	if _, err = b.EvalReadOnly(`(()=>{window.__shimoGraphicBeforeReload=true;return {ok:true}})()`); err != nil {
		return err
	}
	if _, err = b.ws.Call(8000, "Page.reload", map[string]any{"ignoreCache": true}, 15*time.Second); err != nil {
		return err
	}
	if err = b.WaitReady(`!window.__shimoGraphicBeforeReload&&(`+documentReadyJS+`)`, phase3EditorReadyTimeout); err != nil {
		return err
	}
	if err = b.WaitDocumentEditable(); err != nil {
		return err
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	reloadNative, err := b.EvalReadOnly(documentGraphicRuntimeJS(spec, baseline, false, objectID))
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "reload-native.json"), reloadNative); err != nil {
		return err
	}
	reloaded, err := a.phase3OfficeExport(guid, filepath.Join(dir, "reload"), "docx")
	if err != nil {
		return err
	}
	if err = verifyDocumentGraphic(beforeArtifact.Path, reloaded.Path, spec); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "target-node-assertions.json"), map[string]any{"verified": true, "objectId": objectID, "spec": spec, "afterReload": true, "unrequestedXMLAndAssetsPreserved": true}); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "action": "document-graphic", "url": target, "objectId": objectID, "actionVerified": true, "verified": true, "mutationAttempted": true, "beforeArtifact": beforeArtifact, "artifact": afterArtifact, "reloadArtifact": reloaded, "evidenceDir": dir, "verification": "requested graphic properties verified by native identity and two DOCX exports including genuine reload", "warnings": warnings}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
