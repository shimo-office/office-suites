package main

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

type documentReplaceSpec struct {
	Find          string `json:"find"`
	Replacement   string `json:"replacement"`
	ExpectedCount int    `json:"expectedCount"`
}

func prepareDocumentReplace(p map[string]any) (documentReplaceSpec, error) {
	s := documentReplaceSpec{}
	for k := range p {
		if k != "find" && k != "replacement" && k != "expectedCount" {
			return s, fmt.Errorf("未知替换参数 %s", k)
		}
	}
	var ok bool
	s.Find, ok = p["find"].(string)
	if !ok || s.Find == "" || len(s.Find) > 4000 || strings.ContainsAny(s.Find, "\r\n\t\x00") {
		return s, fmt.Errorf("find 必须为非空单行字面文本")
	}
	s.Replacement, ok = p["replacement"].(string)
	if !ok || len(s.Replacement) > 4000 || strings.ContainsAny(s.Replacement, "\r\n\t\x00") {
		return s, fmt.Errorf("replacement 必须是单行字面文本")
	}
	if s.Find == s.Replacement {
		return s, fmt.Errorf("替换前后文本相同")
	}
	n, ok := p["expectedCount"].(float64)
	if !ok || n < 1 || n > 1000 || math.Trunc(n) != n {
		return s, fmt.Errorf("expectedCount 必须明确为1–1000的整数")
	}
	s.ExpectedCount = int(n)
	return s, nil
}
func documentReplaceOffsets(text, find string) []int {
	offsets := []int{}
	for at := 0; at <= len(text)-len(find); {
		i := strings.Index(text[at:], find)
		if i < 0 {
			break
		}
		i += at
		offsets = append(offsets, utf8.RuneCountInString(text[:i]))
		at = i + len(find)
	}
	return offsets
}
func documentReplaceSteps(before documentLayoutSnapshot, s documentReplaceSpec) ([]documentFormatSpec, error) {
	steps := []documentFormatSpec{}
	count := 0
	for _, p := range before.Paragraphs {
		count += strings.Count(p.Text, s.Find)
	}
	if count != s.ExpectedCount {
		return nil, fmt.Errorf("原文命中%d处，与expectedCount=%d不符；未写入", count, s.ExpectedCount)
	}
	for i := len(before.Paragraphs) - 1; i >= 0; i-- {
		p := before.Paragraphs[i]
		offsets := documentReplaceOffsets(p.Text, s.Find)
		current := p.Text
		for j := len(offsets) - 1; j >= 0; j-- {
			start, end := offsets[j], offsets[j]+utf8.RuneCountInString(s.Find)
			steps = append(steps, documentFormatSpec{Paragraph: i + 1, Text: current, Start: start, End: end})
			runes := []rune(current)
			current = string(runes[:start]) + s.Replacement + string(runes[end:])
		}
	}
	return steps, nil
}
func verifyDocumentReplace(before, after documentLayoutSnapshot, s documentReplaceSpec) error {
	if _, e := documentReplaceSteps(before, s); e != nil {
		return e
	}
	if len(before.Paragraphs) != len(after.Paragraphs) || !reflect.DeepEqual(before.Sections, after.Sections) || before.Background != after.Background {
		return fmt.Errorf("替换意外改变段落/页面结构")
	}
	for i, p := range before.Paragraphs {
		got := after.Paragraphs[i]
		want := strings.ReplaceAll(p.Text, s.Find, s.Replacement)
		if got.Text != want || !reflect.DeepEqual(p.Props, got.Props) {
			return fmt.Errorf("第%d段替换文本或段落格式不符", i+1)
		}
		if i >= len(before.Text.Paragraphs) || i >= len(after.Text.Paragraphs) {
			return fmt.Errorf("缺少逐字符格式证据")
		}
		original := documentExpanded(before.Text.Paragraphs[i])
		actual := documentExpanded(after.Text.Paragraphs[i])
		expected := []documentChar{}
		position := 0
		length := utf8.RuneCountInString(s.Find)
		for _, start := range documentReplaceOffsets(p.Text, s.Find) {
			if start >= len(original) || start+length > len(original) {
				return fmt.Errorf("原文字符模型不符")
			}
			expected = append(expected, original[position:start]...)
			for _, c := range s.Replacement {
				expected = append(expected, documentChar{Text: string(c), Attrs: original[start].Attrs})
			}
			position = start + length
		}
		expected = append(expected, original[position:]...)
		if len(expected) != len(actual) {
			return fmt.Errorf("第%d段字符格式发生非预期变化", i+1)
		}
		for j := range expected {
			if !reflect.DeepEqual(expected[j], actual[j]) {
				return fmt.Errorf("第%d段第%d字符格式发生非预期变化", i+1, j+1)
			}
		}
	}
	return nil
}

func documentReplaceJS(s documentReplaceSpec, before documentLayoutSnapshot, steps []documentFormatSpec) string {
	request, _ := json.Marshal(s)
	rawSteps, _ := json.Marshal(steps)
	texts := []string{}
	for _, p := range before.Paragraphs {
		texts = append(texts, p.Text)
	}
	rawTexts, _ := json.Marshal(texts)
	return `(async()=>{` + phase3PresenceGuardJS() + `const request=` + string(request) + `,steps=` + string(rawSteps) + `,baseline=` + string(rawTexts) + `;const locate=async(spec)=>{` + documentRangeBodyJS() + `return {ok:true,range,selected}};const e=window._sdk.editor;if(typeof e.pasteDataInternal!=='function')return {ok:false,error:'精确替换方法未就绪'};if(e.getParagraphs().length!==baseline.length)return {ok:false,error:'段落数已变化'};for(let i=0;i<baseline.length;i++){if((await e.getParagraphRange(e.getParagraphs()[i]).getText()).replace(/\r?\n$/,'')!==baseline[i])return {ok:false,error:'全文基线变化，未写入'}};for(const s of steps){const r=await locate({...s,text:baseline[s.paragraph-1]});if(!r.ok)return r;if(r.selected!==request.find)return {ok:false,error:'预检选区不匹配，未写入'}};let completed=0;for(const spec of steps){const r=await locate(spec);if(!r.ok)return {ok:false,error:'已替换'+completed+'处；后续选区失败，禁止重放：'+r.error};if(r.selected!==request.find)return {ok:false,error:'已替换'+completed+'处；选区文本变化，禁止重放'};` + `const applied=await e.runSelectionCallback(r.range,async()=>{` + phase3PresenceGuardJS() + `if(e.config?.mode?.role!=='editor'||e.docsApi.isViewMode!==false||e.canHandleChanges===false)return {ok:false,error:'编辑权限已变化'};await e.pasteDataInternal(request.replacement,'text');return {ok:true}});if(!applied?.ok)return {ok:false,error:'已提交'+completed+'处；后续写入中止，禁止重放：'+(applied?.error||'未知状态')};completed++}return {ok:true,submitted:true,completed};})()`
}
func (a *App) cmdPhase3DocumentReplace() (runErr error) {
	if len(a.Pos) != 1 {
		return fmt.Errorf("document-replace 需要一个文稿链接")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	p, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	spec, err := prepareDocumentReplace(p)
	if err != nil {
		return err
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !phase3SameOrigin(target, a.Origin) {
		return fmt.Errorf("目标 origin 不一致")
	}
	guid := guidFromURL(target)
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	attempted := false
	defer func() {
		if runErr != nil {
			runErr = documentLayoutFailure(runErr, attempted, dir)
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": attempted, "mayHaveApplied": attempted, "error": runErr.Error()})
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"action": "document-replace", "spec": spec, "target": target, "guid": guid, "version": version, "at": time.Now().UTC()}); err != nil {
		return err
	}
	artifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "before"), "docx")
	if err != nil {
		return err
	}
	before, err := readDocumentLayout(artifact.Path)
	if err != nil {
		return err
	}
	steps, err := documentReplaceSteps(before, spec)
	if err != nil {
		return err
	}
	if len(before.Paragraphs) == 0 || len(before.Paragraphs) > 1000 {
		return fmt.Errorf("段落数量超出当前精确验证范围")
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "before-structure.json"), before); err != nil {
		return err
	}
	b, err := phase3OpenDocumentEditor(target, documentLayoutReadyJS)
	if err != nil {
		return err
	}
	defer b.Close()
	if err = b.CheckPresence(); err != nil {
		return err
	}
	if err = documentLayoutCapture(b, filepath.Join(dir, "before.png")); err != nil {
		return err
	}
	attempted = true
	submitted, err := b.Eval(documentReplaceJS(spec, before, steps))
	if err != nil {
		return fmt.Errorf("替换可能部分生效，禁止自动重放；%s：%w", dir, err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return err
	}
	if err = b.waitDocumentSaved(); err != nil {
		return err
	}
	var after documentLayoutSnapshot
	var afterArtifact ArtifactEvidence
	var verifyErr error
	for i := 0; i < 5; i++ {
		afterArtifact, verifyErr = a.phase3OfficeExport(guid, filepath.Join(dir, fmt.Sprintf("after-%d", i)), "docx")
		if verifyErr == nil {
			after, verifyErr = readDocumentLayout(afterArtifact.Path)
		}
		if verifyErr == nil {
			verifyErr = verifyDocumentReplace(before, after, spec)
		}
		if verifyErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if verifyErr != nil {
		return fmt.Errorf("已提交替换，但独立DOCX验收失败；禁止重放；%s：%w", dir, verifyErr)
	}
	if err = documentLayoutCapture(b, filepath.Join(dir, "after.png")); err != nil {
		return err
	}
	b.Close()
	fresh, err := phase3OpenDocumentEditor(target, documentLayoutReadyJS)
	if err != nil {
		return err
	}
	defer fresh.Close()
	if err = fresh.CheckPresence(); err != nil {
		return err
	}
	reloadArtifact, err := a.phase3OfficeExport(guid, filepath.Join(dir, "reload"), "docx")
	if err != nil {
		return err
	}
	reload, err := readDocumentLayout(reloadArtifact.Path)
	if err != nil {
		return err
	}
	if err = verifyDocumentReplace(before, reload, spec); err != nil {
		return err
	}
	if err = documentLayoutCapture(fresh, filepath.Join(dir, "reload.png")); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "target-node-assertions.json"), map[string]any{"verified": true, "count": spec.ExpectedCount, "before": before, "after": after, "reload": reload}); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "verified": true, "action": "document-replace", "url": target, "replaced": spec.ExpectedCount, "beforeArtifact": artifact, "artifact": afterArtifact, "reloadArtifact": reloadArtifact, "evidenceDir": dir, "verificationScope": "literal whole-document replacement count and text/format preservation; not full scene acceptance"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
