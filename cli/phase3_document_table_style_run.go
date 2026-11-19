package main

import (
	"fmt"
	"path/filepath"
	"time"
)

func (a *App) cmdPhase3DocumentTableStyle() (runErr error) {
	if len(a.Pos) != 1 {
		return fmt.Errorf("document-table-style 需要一个文稿链接")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareDocumentTableStyle(payload)
	if err != nil {
		return err
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !phase3SameOrigin(target, a.Origin) {
		return fmt.Errorf("目标 origin 不一致，未写入")
	}
	guid := guidFromURL(target)
	if guid == "" {
		return fmt.Errorf("目标缺少 GUID")
	}
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	attempted := false
	defer func() {
		if runErr != nil {
			if attempted {
				runErr = fmt.Errorf("已尝试写入，可能已生效；验收未通过，不要自动重放（证据 %s）：%w", dir, runErr)
			} else {
				runErr = fmt.Errorf("未执行写入（证据 %s）：%w", dir, runErr)
			}
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": attempted, "error": runErr.Error()})
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"action": "document-table-style", "target": target, "spec": spec, "version": version, "at": time.Now().UTC()}); err != nil {
		return err
	}
	beforeArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "before"), "docx")
	if err != nil {
		return err
	}
	before, err := readDocumentTables(beforeArtifact.Path)
	if err != nil {
		return err
	}
	if _, err = documentTableAnchor(before, spec.documentTableSpec); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "before-readback.json"), before); err != nil {
		return err
	}
	if verifyDocumentTableStyle(beforeArtifact.Path, beforeArtifact.Path, spec) == nil {
		return fmt.Errorf("目标样式已符合请求，未重复写入")
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
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return err
	}
	preflight, err := b.Eval(documentTableStylePreflightJS(spec, before))
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "selection-preflight.json"), preflight); err != nil {
		return err
	}
	attempted = true
	submitted, err := b.Eval(documentTableStyleJS(spec, before))
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
	after, err := readDocumentTables(afterArtifact.Path)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "after-readback.json"), after); err != nil {
		return err
	}
	if err = verifyDocumentTableStyle(beforeArtifact.Path, afterArtifact.Path, spec); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "after.png")); err != nil {
		return err
	}
	if _, err = b.Eval(`(()=>{Object.defineProperty(window,"__shimoPhase3BeforeReload",{value:true});return {ok:true}})()`); err != nil {
		return err
	}
	if _, err = b.ws.Call(8000, "Page.reload", map[string]any{"ignoreCache": true}, 15*time.Second); err != nil {
		return err
	}
	if err = b.WaitReady(`!window.__shimoPhase3BeforeReload&&(`+documentReadyJS+`)`, phase3EditorReadyTimeout); err != nil {
		return err
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	reloadArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "reload"), "docx")
	if err != nil {
		return err
	}
	reloaded, err := readDocumentTables(reloadArtifact.Path)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "reload-readback.json"), reloaded); err != nil {
		return err
	}
	if err = verifyDocumentTableStyle(beforeArtifact.Path, reloadArtifact.Path, spec); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "target-node-assertions.json"), map[string]any{"verified": true, "spec": spec, "before": before, "after": after, "reloaded": reloaded}); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "verified": true, "action": "document-table-style", "target": target, "spec": spec, "beforeArtifact": beforeArtifact, "artifact": afterArtifact, "reloadArtifact": reloadArtifact, "evidenceDir": dir, "verificationScope": "requested single-cell fill/text color and whole-table margins/alignment/outer borders; unchanged other document XML and effective character attributes after reload"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
