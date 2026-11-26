package main

import (
	"fmt"
	"path/filepath"
	"time"
)

func (a *App) cmdPhase3DocumentMention() (runErr error) {
	if len(a.Pos) != 1 {
		return fmt.Errorf("document-mention 需要一个文稿链接")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareDocumentMention(payload)
	if err != nil {
		return err
	}
	me, err := a.usersMe()
	if err != nil {
		return err
	}
	userID, userName, err := documentMentionIdentity(me)
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
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
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
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"action": "document-mention", "target": target, "spec": spec, "version": version, "at": time.Now().UTC()}); err != nil {
		return err
	}
	beforeArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "before"), "docx")
	if err != nil {
		return err
	}
	before, err := readDocumentSnapshot(beforeArtifact.Path)
	if err != nil {
		return err
	}
	if spec.Paragraph > len(before.Paragraphs) || before.Paragraphs[spec.Paragraph-1].Text != spec.Text {
		return fmt.Errorf("基线段落不匹配，未写入")
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "before-readback.json"), before); err != nil {
		return err
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
	attempted = true
	submitted, err := b.Eval(documentMentionJS(spec, userID, userName))
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return err
	}
	if err = b.waitDocumentSaved(); err != nil {
		return err
	}
	mention, err := b.Eval(documentMentionReadbackJS(spec, userID, userName))
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "mention-after.json"), mention); err != nil {
		return err
	}
	afterArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "after"), "docx")
	if err != nil {
		return err
	}
	after, err := readDocumentSnapshot(afterArtifact.Path)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "after-readback.json"), after); err != nil {
		return err
	}
	if err = verifyDocumentMention(before, after, spec, userName); err != nil {
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
	reloadedMention, err := b.Eval(documentMentionReadbackJS(spec, userID, userName))
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "mention-readback.json"), reloadedMention); err != nil {
		return err
	}
	reloadArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "reload"), "docx")
	if err != nil {
		return err
	}
	reloaded, err := readDocumentSnapshot(reloadArtifact.Path)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "reload-readback.json"), reloaded); err != nil {
		return err
	}
	if err = verifyDocumentMention(before, reloaded, spec, userName); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "target-node-assertions.json"), map[string]any{"verified": true, "spec": spec, "before": before, "after": after, "reloaded": reloaded}); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "verified": true, "action": "document-mention", "target": target, "spec": spec, "beforeArtifact": beforeArtifact, "artifact": afterArtifact, "reloadArtifact": reloadArtifact, "evidenceDir": dir, "verificationScope": "real self-mention node user ID/display text after reload, exported appended text and original text/style preservation; no separate notification API invoked; platform delivery is not asserted"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
