package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

func phase3SlidesRead(b *phase3EditorBrowser) ([]string, error) {
	v, err := b.Eval(`(()=>{const sdk=window.presentationSDK;if(sdk?.editor?.workbench?.isReady()!==true)return {ok:false,error:"幻灯片尚未就绪"};const api=sdk.API?.slides;if(!api)return {ok:false,error:"缺少真实幻灯片 SDK"};return {ok:true,ids:api.getSlides().map(s=>s.id)}})()`)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("页面快照返回无效")
	}
	raw, ok := m["ids"].([]any)
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("页面快照为空或未就绪")
	}
	ids := make([]string, len(raw))
	seen := map[string]bool{}
	for i, v := range raw {
		ids[i], ok = v.(string)
		if !ok || ids[i] == "" || seen[ids[i]] {
			return nil, fmt.Errorf("页面 ID 无效")
		}
		seen[ids[i]] = true
	}
	return ids, nil
}
func phase3SlidesSelect(b *phase3EditorBrowser, id string) error {
	raw, _ := json.Marshal(id)
	_, err := b.Eval(`(()=>{const id=` + string(raw) + `;const api=window.presentationSDK?.API?.slides;if(!api?.getSlides().some(s=>s.id===id))return {ok:false,error:"截图目标页面不存在"};api.setCurrentSlideIndex(id);api.setSelectedSlides([id]);return {ok:true}})()`)
	return err
}
func phase3SlideScreenshot(b *phase3EditorBrowser, filename string) error {
	if _, err := b.Eval(`(async()=>{const frame=()=>Promise.race([new Promise(requestAnimationFrame),new Promise(r=>setTimeout(r,250))]);await frame();await frame();const sdk=window.presentationSDK;if(sdk?.editor?.workbench?.isReady()!==true||!sdk.API?.slides?.getSlides()?.length)return {ok:false,error:"幻灯片未就绪，不能截图验收"};const visible=[...document.querySelectorAll('canvas,svg')].some(n=>{const r=n.getBoundingClientRect();return r.width>200&&r.height>100});if(!visible)return {ok:false,error:"未发现可见幻灯片画布，不能截图验收"};return {ok:true}})()`); err != nil {
		return err
	}
	return b.Capture(filename)
}
func phase3SlidesMutationJS(s phase3SlidesSpec, expected []string) string {
	if s.Media != nil {
		return phase3SlideMediaMutationJS(s, expected)
	}
	if s.Theme != nil {
		return phase3SlideThemeMutationJS(s, expected)
	}
	if s.Show != nil {
		return phase3SlideShowMutationJS(s, expected)
	}
	if s.Animation != nil {
		return phase3SlideAnimationMutationJS(s, expected)
	}
	if s.Table != nil {
		return phase3SlideTableMutationJS(s, expected)
	}
	if s.Placeholder != nil {
		return phase3SlidePlaceholderMutationJS(s, expected)
	}
	if s.Shape != nil {
		return phase3SlideShapeMutationJS(s, expected)
	}
	if s.Replace != nil {
		return phase3SlideReplaceMutationJS(s, expected)
	}
	if s.Action == "slide-text-format" {
		return phase3SlideTextFormatMutationJS(s, expected)
	}
	if s.Action == "slide-layout" {
		return phase3SlideLayoutMutationJS(s, expected)
	}
	if s.Action == "slide-text-insert" {
		return phase3SlideTextMutationJS(s, expected)
	}
	request, _ := json.Marshal(s)
	ids, _ := json.Marshal(expected)
	return `(()=>{const request=` + string(request) + `;const expected=` + string(ids) + `;` + phase3PresenceGuardJS() + `
const sdk=window.presentationSDK;const wb=sdk?.editor?.workbench;if(wb?.isReady()!==true||typeof wb.permissions?.canEdit!=="function"||wb.permissions.canEdit()!==true)return {ok:false,error:"未确认编辑权限或页面未就绪，未写入"};const api=sdk?.API?.slides;if(!api)return {ok:false,error:"缺少幻灯片 SDK"};const before=api.getSlides().map(s=>s.id);if(JSON.stringify(before)!==JSON.stringify(expected)||!before.includes(request.slideId))return {ok:false,error:"页面列表已变化，未写入"};
if(request.action==='slide-delete'&&before.length<2)return {ok:false,error:"不能删除最后一页"};
let added;switch(request.action){case 'slide-add':api.setCurrentSlideIndex(request.slideId);api.setSelectedSlides([request.slideId]);added=api.addSlide();break;case 'slide-duplicate':added=api.duplicateSlide(request.slideId);break;case 'slide-hide':api.hideSlide(request.slideId);break;case 'slide-delete':api.deleteSlide(request.slideId);break;default:return {ok:false,error:"未知动作"};}
const after=api.getSlides().map(s=>s.id);if(added&&(!added.id||before.includes(added.id)))return {ok:false,error:"SDK 未返回新增页面 ID；禁止重放"};return {ok:true,before,after,addedId:added?.id||null,presenceSource:'COLLABROOM/RECONNECT_READY'};})()`
}

// Tests replace only startup with a local CDP peer; all command guards,
// exports, verification and independent reopen still run in production code.
var phase3OpenSlidesEditor = openPhase3Editor

func (a *App) cmdPhase3Slides() error {
	return a.cmdPhase3SlidesWithOpen(phase3OpenSlidesEditor)
}

func (a *App) cmdPhase3SlidesWithOpen(openEditor func(string) (*phase3EditorBrowser, error)) (runErr error) {
	action := a.opt("--action")
	var spec phase3SlidesSpec
	if action != "slide-inspect" && action != "slide-design-inspect" {
		args, err := phase2Args(a.opt("--payload"))
		if err != nil {
			return err
		}
		p, err := phase2ObjectArg(args)
		if err != nil {
			return err
		}
		spec, err = preparePhase3Slides(action, p)
		if err != nil {
			return err
		}
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !phase3SameOrigin(target, a.Origin) {
		return fmt.Errorf("目标与登录 origin 不一致")
	}
	guid := guidFromURL(target)
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	if str(info, "type") != "presentation" || boolv(info, "isCloudFile") || boolv(info, "isFolder") {
		return fmt.Errorf("页面动作仅接受在线幻灯片")
	}
	u, err := url.Parse(target)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("dbg", "1")
	u.RawQuery = q.Encode()
	editorURL := u.String()
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	attempted := false
	started := time.Now()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	commandRecord := map[string]any{"argv": os.Args, "action": action, "targetGuid": guid, "startedUTC": started.UTC().Format(time.RFC3339), "startedShanghai": started.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format(time.RFC3339), "binarySHA256": fmt.Sprintf("%x", sha256.Sum256(binary))}
	defer func() {
		runErr = phase3SlideMutationError(runErr, attempted, dir)
		if runErr != nil && !attempted {
			runErr = fmt.Errorf("幻灯片动作未提交写入（证据 %s）：%w", dir, runErr)
		}
		commandRecord["mutationAttempted"] = attempted
		commandRecord["finishedUTC"] = time.Now().UTC().Format(time.RFC3339)
		commandRecord["exitCode"] = 0
		if runErr != nil {
			commandRecord["exitCode"] = 1
		}
		_ = phase3WriteEvidence(filepath.Join(dir, "command-log.json"), commandRecord)
		if runErr != nil {
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": attempted, "error": runErr.Error(), "doNotReplay": attempted})
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "command-log.json"), commandRecord); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"action": action, "spec": spec, "url": target}); err != nil {
		return err
	}
	b, err := openEditor(editorURL)
	if err != nil {
		return err
	}
	defer func() {
		if b != nil {
			b.Close()
		}
	}()
	beforeIDs, err := phase3SlidesRead(b)
	if err != nil {
		return err
	}
	if spec.Replace != nil || spec.Show != nil || spec.Theme != nil {
		spec.SlideID = beforeIDs[0]
	}
	if action == "slide-design-inspect" {
		v, e := b.Eval(phase3SlideDesignJS())
		if e != nil {
			return e
		}
		return a.printJSON(v)
	}
	if action == "slide-inspect" {
		return a.printJSON(map[string]any{"ok": true, "url": target, "slideIds": beforeIDs, "verificationScope": "read-only editor page IDs"})
	}
	if spec.Layout != nil {
		if err = phase3ResolveSlideLayout(b, &spec); err != nil {
			return err
		}
	}
	if err = b.CheckPresence(); err != nil {
		return err
	}
	beforeArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "before"), "pptx")
	if err != nil {
		return err
	}
	if spec.Layout != nil {
		spec.Layout.Expected, err = phase3SlideExpectedLayout(beforeArtifact.Path, beforeIDs, spec)
		if err != nil {
			return err
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "layout-expected.json"), spec.Layout); err != nil {
			return err
		}
	}
	if spec.Replace != nil {
		if err = phase3PrepareSlideReplace(beforeArtifact.Path, beforeIDs, &spec); err != nil {
			return err
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "replace-expected.json"), spec.Replace); err != nil {
			return err
		}
	}
	if spec.Media != nil {
		if _, err = readPhase3SlideArtifactFiltered(beforeArtifact.Path, beforeIDs, phase3MediaTargetFilter(beforeIDs, spec, nil, false)); err != nil {
			return err
		}
		if err = phase3PrepareSlideMediaBrowser(b, spec.Media, spec.SlideID); err != nil {
			return err
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "input-media-sha256.json"), spec.Media); err != nil {
			return err
		}
	}
	if spec.Theme != nil {
		if _, _, err = phase3ReadThemeLocalPages(beforeArtifact.Path, beforeIDs, nil, nil); err != nil {
			return err
		}
		if err = phase3PrepareSlideTheme(b, spec.Theme); err != nil {
			return err
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "theme-expected.json"), spec.Theme); err != nil {
			return err
		}
	}
	if spec.Show != nil {
		if _, err = phase3ReadShowProperties(beforeArtifact.Path); err != nil {
			return err
		}
		if _, err = phase3PrepareNativeShow(b, target, spec.Show); err != nil {
			return err
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "show-expected.json"), spec.Show); err != nil {
			return err
		}
	}
	if spec.Animation != nil {
		if err = phase3PrepareSlideAnimation(beforeArtifact.Path, beforeIDs, spec); err != nil {
			return err
		}
	}
	if spec.Table != nil {
		if err = phase3PrepareSlideTable(beforeArtifact.Path, beforeIDs, &spec); err != nil {
			return err
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "table-expected.json"), spec.Table); err != nil {
			return err
		}
	}
	if spec.Shape != nil {
		if err = phase3PrepareSlideShape(beforeArtifact.Path, beforeIDs, &spec); err != nil {
			return err
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "shape-expected.json"), spec.Shape); err != nil {
			return err
		}
	}
	before, err := readPhase3SlideArtifact(beforeArtifact.Path, beforeIDs)
	if err != nil {
		return err
	}
	found := false
	for _, p := range before {
		if p.ID == spec.SlideID {
			found = true
			if spec.Layout != nil && p.Layout == spec.Layout.Expected {
				return fmt.Errorf("导出版式已经相同，未执行无效写入")
			}
			if spec.Action == "slide-hide" && p.Hidden {
				return fmt.Errorf("目标页已经隐藏，未执行重复写入")
			}
		}
	}
	if !found {
		return fmt.Errorf("目标页面 ID 不存在")
	}
	if spec.Action == "slide-delete" && len(before) < 2 {
		return fmt.Errorf("不能删除最后一页")
	}
	if spec.Placeholder != nil {
		if err = phase3PlaceholderBWPreflight(beforeArtifact.Path); err != nil {
			return err
		}
		if _, err = readPhase3SlideArtifactFiltered(beforeArtifact.Path, beforeIDs, phase3PlaceholderFilter(beforeIDs, spec, false)); err != nil {
			return err
		}
	}
	if spec.Format != nil {
		if _, ok := spec.Format.Attrs["outlineColor"]; ok {
			if err = phase3CheckNewOutlineArtifact(beforeArtifact.Path, beforeIDs, spec); err != nil {
				return err
			}
		}
		if _, err = readPhase3SlideArtifactFiltered(beforeArtifact.Path, beforeIDs, phase3SlideTextFormatFilter(beforeIDs, spec, false, true)); err != nil {
			return err
		}
	}
	if spec.Text != nil {
		if err = phase3SlideTextBounds(beforeArtifact.Path, *spec.Text); err != nil {
			return err
		}
		if _, err = readPhase3SlideArtifactFiltered(beforeArtifact.Path, beforeIDs, phase3SlideTextTargetFilter(beforeIDs, spec, false)); err != nil {
			return err
		}
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "before-readback.json"), before); err != nil {
		return err
	}
	if err = phase3SlidesSelect(b, spec.SlideID); err != nil {
		return err
	}
	if err = phase3SlideScreenshot(b, filepath.Join(dir, "before.png")); err != nil {
		return err
	}
	if err = phase3SlideCaptureBatch(b, dir, "before", beforeIDs, spec); err != nil {
		return err
	}
	attempted = true
	submission, err := b.Eval(phase3SlidesMutationJS(spec, beforeIDs))
	if err != nil {
		return fmt.Errorf("动作未获确认，禁止自动重放；%s：%w", dir, err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submission); err != nil {
		return err
	}
	afterIDs, err := phase3SlidesRead(b)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "after-readback.json"), map[string]any{"ids": afterIDs}); err != nil {
		return err
	}
	shotID := spec.SlideID
	if spec.Action == "slide-add" || spec.Action == "slide-duplicate" {
		for _, id := range afterIDs {
			exists := false
			for _, old := range beforeIDs {
				if id == old {
					exists = true
				}
			}
			if !exists {
				shotID = id
			}
		}
	}
	if spec.Action == "slide-delete" {
		shotID = afterIDs[0]
	}
	if err = phase3SlidesSelect(b, shotID); err != nil {
		return err
	}
	if err = phase3SlideScreenshot(b, filepath.Join(dir, "after.png")); err != nil {
		return err
	}
	if err = phase3SlideCaptureBatch(b, dir, "after", afterIDs, spec); err != nil {
		return err
	}
	// Poll only independent reads/exports; the mutation above is never replayed.
	var after []phase3SlidePage
	var artifact ArtifactEvidence
	var proofErr error
	for i := 0; i < 4; i++ {
		time.Sleep(2 * time.Second)
		artifact, proofErr = a.phase3OfficeExport(guid, filepath.Join(dir, fmt.Sprintf("after-%d", i)), "pptx")
		if proofErr == nil {
			after, proofErr = readPhase3SlideArtifact(artifact.Path, afterIDs)
		}
		if proofErr == nil {
			if spec.Media != nil {
				proofErr = verifyPhase3SlideMediaArtifacts(beforeArtifact.Path, artifact.Path, beforeIDs, afterIDs, spec)
			} else if spec.Theme != nil {
				proofErr = verifyPhase3SlideThemeArtifacts(beforeArtifact.Path, artifact.Path, beforeIDs, afterIDs, spec)
			} else if spec.Show != nil {
				proofErr = verifyPhase3SlideShowArtifacts(beforeArtifact.Path, artifact.Path, beforeIDs, afterIDs, spec)
			} else if spec.Animation != nil {
				proofErr = verifyPhase3SlideAnimationArtifacts(beforeArtifact.Path, artifact.Path, beforeIDs, afterIDs, spec)
			} else if spec.Table != nil {
				proofErr = verifyPhase3SlideTableArtifacts(beforeArtifact.Path, artifact.Path, beforeIDs, afterIDs, spec)
			} else if spec.Placeholder != nil {
				proofErr = verifyPhase3SlidePlaceholderArtifacts(beforeArtifact.Path, artifact.Path, beforeIDs, afterIDs, spec)
			} else if spec.Shape != nil {
				proofErr = verifyPhase3SlideShapeArtifacts(beforeArtifact.Path, artifact.Path, beforeIDs, afterIDs, spec)
			} else if spec.Text != nil {
				proofErr = verifyPhase3SlideTextArtifacts(beforeArtifact.Path, artifact.Path, beforeIDs, afterIDs, spec)
			} else if spec.Replace != nil {
				proofErr = verifyPhase3SlideReplaceArtifacts(beforeArtifact.Path, artifact.Path, beforeIDs, afterIDs, spec)
			} else if spec.Format != nil {
				proofErr = verifyPhase3SlideTextFormatArtifacts(beforeArtifact.Path, artifact.Path, beforeIDs, afterIDs, spec)
			} else if spec.Layout != nil {
				proofErr = verifyPhase3SlideLayout(before, after, spec)
			} else {
				proofErr = verifyPhase3Slides(before, after, spec)
			}
		}
		if proofErr == nil {
			break
		}
	}
	if proofErr != nil {
		return fmt.Errorf("写入后 PPTX 验收失败，禁止重放；%s：%w", dir, proofErr)
	}
	b.Close()
	b = nil
	reload, err := openEditor(editorURL)
	if err != nil {
		return fmt.Errorf("重开验证失败：%w", err)
	}
	defer reload.Close()
	reloadedIDs, err := phase3SlidesRead(reload)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(reloadedIDs, afterIDs) {
		return fmt.Errorf("重开后页面 ID/顺序未持久化；禁止重放")
	}
	if spec.Media != nil {
		v, e := phase3VerifyReloadedMedia(reload, spec, dir)
		if e != nil {
			return e
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "playback-evidence.json"), v); err != nil {
			return err
		}
	}
	if spec.Theme != nil {
		v, e := reload.Eval(phase3SlideThemeReloadJS())
		if e != nil {
			return e
		}
		// Preserve the actual reopened state even when source comparison fails.
		if err = phase3WriteEvidence(filepath.Join(dir, "reload-theme.json"), v); err != nil {
			return err
		}
		if e = phase3VerifySlideThemeNative(v, reloadedIDs, *spec.Theme); e != nil {
			return e
		}
		native, _ := v.(map[string]any)
		if e = phase3VerifyThemeDelta(native["delta"], *spec.Theme); e != nil {
			return e
		}
	}
	if spec.Show != nil {
		v, e := phase3VerifyReloadedShow(reload, *spec.Show)
		if v != nil {
			if err = phase3WriteEvidence(filepath.Join(dir, "reload-show-settings.json"), v); err != nil {
				return err
			}
		}
		if e != nil {
			return e
		}
	}
	if spec.Animation != nil {
		v, e := phase3VerifyReloadedAnimation(reload, spec)
		if e != nil {
			return e
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "reload-animation.json"), v); err != nil {
			return err
		}

	}
	if err = phase3SlideCaptureBatch(reload, dir, "reload", reloadedIDs, spec); err != nil {
		return err
	}
	if spec.Layout != nil {
		if err = phase3VerifyReloadedLayout(reload, spec); err != nil {
			return err
		}
	}
	if err = phase3SlidesSelect(reload, shotID); err != nil {
		return err
	}
	if err = phase3SlideScreenshot(reload, filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "target-node-assertions.json"), map[string]any{"before": before, "after": after, "reloadIds": reloadedIDs, "verified": true}); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "verified": true, "action": action, "url": target, "beforeArtifact": beforeArtifact, "artifact": artifact, "slides": after, "evidenceDir": dir, "verificationScope": "requested slide action: editor IDs/order, independent reload, PPTX target nodes and unchanged non-target content; theme operations also compare complete official master/layout source; not full scene acceptance"}
	if spec.Show != nil {
		matched, e := phase3ShowExportMatches(artifact.Path, *spec.Show)
		if e != nil {
			return e
		}
		result["verificationScope"] = "saved Shimo native PresentationPr settings after fresh-profile reload; unchanged PPTX slide content/resources; not portable-export settings acceptance"
		result["pptxShowSettingsMatchRequest"] = matched
		if !matched {
			result["exportLimitation"] = "当前石墨 PPTX 导出未包含已保存的循环/计时设置"
		}
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printPhase3Result(result, fmt.Sprintf("幻灯片页面动作验收通过：%s", action))
}

func phase3SlideMutationError(err error, attempted bool, dir string) error {
	if err == nil || !attempted {
		return err
	}
	return fmt.Errorf("写入已尝试，可能已生效；验收未通过，禁止自动重放（证据 %s）：%w", dir, err)
}

func phase3SlideCaptureBatch(b *phase3EditorBrowser, dir, prefix string, ids []string, s phase3SlidesSpec) error {
	targets := []string{}
	if s.Theme != nil {
		targets = append(targets, ids...)
	}
	if s.Animation != nil {
		for _, t := range s.Animation.Targets {
			targets = append(targets, t.SlideID)
		}
	}
	for i, id := range targets {
		if e := phase3SlidesSelect(b, id); e != nil {
			return e
		}
		if e := phase3SlideScreenshot(b, filepath.Join(dir, fmt.Sprintf("%s-page-%d.png", prefix, i+1))); e != nil {
			return e
		}
	}
	return nil
}
