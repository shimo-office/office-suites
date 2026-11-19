package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

type phase3SlideReplaceSpec struct {
	Old      string `json:"old"`
	New      string `json:"new"`
	Count    int    `json:"expectedOldCount"`
	NewCount int    `json:"expectedNewCount"`
}

func preparePhase3SlideReplace(p map[string]any) (*phase3SlideReplaceSpec, error) {
	old, o := p["old"].(string)
	next, n := p["new"].(string)
	if !o || !n || len(p) != 2 || strings.TrimSpace(old) == "" || strings.TrimSpace(next) == "" || strings.ContainsAny(old+next, "\r\n\t\x00") || utf8.RuneCountInString(old) > 1000 || utf8.RuneCountInString(next) > 1000 {
		return nil, fmt.Errorf("替换只接受非空单行old/new文字，最多1000字符")
	}
	if strings.Contains(old, next) || strings.Contains(next, old) {
		return nil, fmt.Errorf("old/new不得相同或互相包含，以便精确核对替换次数")
	}
	return &phase3SlideReplaceSpec{Old: old, New: next}, nil
}

// Reject text spanning runs before mutation: this bounded action guarantees
// formatting preservation by replacing within each already-identifiable run.
func phase3SlideReplaceCounts(root *artifactNode, s phase3SlideReplaceSpec) ([2]int, error) {
	counts := [2]int{}
	var walk func(*artifactNode) error
	walk = func(n *artifactNode) error {
		if n.Name.Space == artifactDrawingNS && n.Name.Local == "p" {
			expected := strings.Count(phase3SlideText(n), s.Old)
			inside := 0
			var count func(*artifactNode)
			count = func(x *artifactNode) {
				if x.Name.Space == artifactDrawingNS && x.Name.Local == "t" {
					inside += strings.Count(x.Text, s.Old)
				}
				for _, c := range x.Children {
					count(c)
				}
			}
			count(n)
			if expected != inside {
				return fmt.Errorf("旧词跨越格式run边界，未执行可能破坏样式的替换")
			}
		}
		if n.Name.Space == artifactDrawingNS && n.Name.Local == "t" {
			counts[0] += strings.Count(n.Text, s.Old)
			counts[1] += strings.Count(strings.ReplaceAll(n.Text, s.Old, s.New), s.New)
		}
		for _, c := range n.Children {
			if e := walk(c); e != nil {
				return e
			}
		}
		return nil
	}
	e := walk(root)
	return counts, e
}
func phase3SlideReplaceNode(n *artifactNode, s phase3SlideReplaceSpec) *artifactNode {
	out := *n
	out.Children = nil
	if n.Name.Space == artifactDrawingNS && n.Name.Local == "t" {
		out.Text = strings.ReplaceAll(n.Text, s.Old, s.New)
	}
	for _, c := range n.Children {
		out.Children = append(out.Children, phase3SlideReplaceNode(c, s))
	}
	return &out
}
func phase3PrepareSlideReplace(filename string, ids []string, s *phase3SlidesSpec) error {
	if e := phase3PlaceholderBWPreflight(filename); e != nil {
		return e
	}
	total := [2]int{}
	_, e := readPhase3SlideArtifactFiltered(filename, ids, func(i int, n *artifactNode) (*artifactNode, error) {
		c, e := phase3SlideReplaceCounts(n, *s.Replace)
		if e != nil {
			return nil, e
		}
		total[0] += c[0]
		total[1] += c[1]
		return n, nil
	})
	if e != nil {
		return e
	}
	if total[0] == 0 {
		return fmt.Errorf("旧词不存在，未执行无效替换")
	}
	s.Replace.Count = total[0]
	s.Replace.NewCount = total[1]
	return nil
}
func verifyPhase3SlideReplaceArtifacts(beforePath, afterPath string, beforeIDs, afterIDs []string, s phase3SlidesSpec) error {
	if s.Replace == nil || s.Replace.Count < 1 || !reflect.DeepEqual(beforeIDs, afterIDs) {
		return fmt.Errorf("缺少替换预期或页面顺序改变")
	}
	for _, path := range []string{beforePath, afterPath} {
		if e := phase3PlaceholderBWPreflight(path); e != nil {
			return e
		}
	}
	targets := map[int]map[string]bool{}
	expected, e := readPhase3SlideArtifactFiltered(beforePath, beforeIDs, func(i int, n *artifactNode) (*artifactNode, error) {
		targets[i] = map[string]bool{}
		phase3CollectReplaceTargets(n, "", s.Replace.Old, targets[i])
		return phase3NormalizeReplaceTargets(phase3SlideReplaceNode(n, *s.Replace), "", targets[i]), nil
	})
	if e != nil {
		return e
	}
	actual, e := readPhase3SlideArtifactFiltered(afterPath, afterIDs, func(i int, n *artifactNode) (*artifactNode, error) {
		return phase3NormalizeReplaceTargets(n, "", targets[i]), nil
	})
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(expected, actual) {
		return fmt.Errorf("导出文字/次数未按请求替换，或出现格式、媒体等附带变化")
	}
	return nil
}

// Paths are captured from the original package, never inferred from the new
// text. Neutral native defaults on unrelated objects/paragraphs stay exact.
func phase3ReplaceHasRunMatch(n *artifactNode, old string) bool {
	if n.Name.Space == artifactDrawingNS && n.Name.Local == "t" && strings.Contains(n.Text, old) {
		return true
	}
	for _, child := range n.Children {
		if phase3ReplaceHasRunMatch(child, old) {
			return true
		}
	}
	return false
}

func phase3CollectReplaceTargets(n *artifactNode, path, old string, targets map[string]bool) {
	if phase3ReplaceHasRunMatch(n, old) && ((n.Name.Space == artifactSlideNS && n.Name.Local == "sp") || (n.Name.Space == artifactDrawingNS && n.Name.Local == "p")) {
		targets[path] = true
	}
	for i, c := range n.Children {
		phase3CollectReplaceTargets(c, path+"/"+strconv.Itoa(i), old, targets)
	}
}

func phase3NormalizeReplaceTargets(n *artifactNode, path string, targets map[string]bool) *artifactNode {
	out := *n
	out.Children = make([]*artifactNode, len(n.Children))
	for i, c := range n.Children {
		out.Children[i] = phase3NormalizeReplaceTargets(c, path+"/"+strconv.Itoa(i), targets)
	}
	if targets[path] {
		if n.Name.Space == artifactSlideNS && n.Name.Local == "sp" {
			return phase3PlaceholderNativeBW(&out)
		}
		if n.Name.Space == artifactDrawingNS && n.Name.Local == "p" {
			// Preserves existing runs/formatting; does not promise identical
			// default styling for text appended in a later editing session.
			return phase3TableNativeParagraph(&out)
		}
	}
	return &out
}
func phase3SlideReplaceMutationJS(s phase3SlidesSpec, ids []string) string {
	raw, _ := json.Marshal(s.Replace)
	expected, _ := json.Marshal(ids)
	return `(()=>{const p=` + string(raw) + `;const expected=` + string(expected) + `;` + phase3PresenceGuardJS() + `
const sdk=window.presentationSDK;const wb=sdk?.editor?.workbench;if(wb?.isReady()!==true||typeof wb.permissions?.canEdit!=='function'||wb.permissions.canEdit()!==true)return {ok:false,error:'未确认编辑权限'};if(typeof wb.core?.getSlideTextSummary!=='function'||typeof wb.core?.replace!=='function')return {ok:false,error:'缺少已确认的查找替换接口'};if(JSON.stringify(sdk.API.slides.getSlides().map(s=>s.id))!==JSON.stringify(expected))return {ok:false,error:'页面列表变化'};const count=text=>{const rows=wb.core.getSlideTextSummary(wb.editor,{flatten:true,includeHiddenSlide:true});if(!Array.isArray(rows)||rows.some(r=>typeof r.text!=='string'))throw new Error('invalid paragraph snapshot');return rows.reduce((n,r)=>n+r.text.split(text).length-1,0)};const before=count(p.old);if(before!==p.expectedOldCount)return {ok:false,error:'SDK查找次数与PPTX不一致，未写入'};const submitted=wb.core.replace(wb.editor,{text:p.old,replaceWith:p.new,isReplaceAll:true,isCaseSensitive:true});const old=count(p.old);const next=count(p.new);if(submitted?.replaceNum!==p.expectedOldCount||old!==0||next!==p.expectedNewCount)return {ok:false,error:'替换次数或读回不符，禁止重放'};return {ok:true,before,submitted,oldCount:old,newCount:next};})()`
}
